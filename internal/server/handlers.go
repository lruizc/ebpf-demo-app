package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/luisruiz/weather-app/internal/cache"
	"github.com/luisruiz/weather-app/internal/config"
	"github.com/luisruiz/weather-app/internal/weather"
	"github.com/luisruiz/weather-app/web"
)

// IndexPage is the data model passed to the HTML dashboard template.
type IndexPage struct {
	Cities     []config.City
	PodName    string
	AppVersion string
}

// Handlers groups the HTTP handler dependencies.
type Handlers struct {
	cfg     *config.Config
	store   *cache.Store // nil when Redis is unavailable (degraded mode)
	wc      *weather.Client
	metrics *Metrics
	log     *slog.Logger
	tmpl    *template.Template
	version string
}

// NewHandlers constructs Handlers. store may be nil (degraded/no-cache mode).
func NewHandlers(
	cfg *config.Config,
	store *cache.Store,
	wc *weather.Client,
	m *Metrics,
	log *slog.Logger,
	version string,
) (*Handlers, error) {
	tmpl, err := template.ParseFS(web.FS, "templates/index.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &Handlers{
		cfg: cfg, store: store, wc: wc,
		metrics: m, log: log, tmpl: tmpl, version: version,
	}, nil
}

// Index serves the HTML dashboard.
func (h *Handlers) Index(w http.ResponseWriter, r *http.Request) {
	hostname, _ := os.Hostname()
	page := IndexPage{Cities: h.cfg.Cities, PodName: hostname, AppVersion: h.version}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.tmpl.Execute(w, page); err != nil {
		h.log.Error("template render", "err", err)
	}
}

// Weather resolves and returns the weather JSON for the requested city.
func (h *Handlers) Weather(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	reqID := newRequestID()

	slug := r.URL.Query().Get("city")
	if slug == "" {
		writeJSON(w, http.StatusBadRequest, errBody("missing_city", "query parameter 'city' is required"))
		return
	}

	city, ok := h.cfg.CityBySlug[slug]
	if !ok {
		writeJSON(w, http.StatusNotFound, errBody("unknown_city", fmt.Sprintf("city %q not in catalog", slug)))
		return
	}

	// Attempt cache read for non-bypass cities.
	var cached *weather.Snapshot
	if h.store != nil {
		snap, hit, err := h.store.Get(r.Context(), slug)
		if err != nil {
			h.log.Warn("cache_get_error", "city", slug, "err", err.Error(), "request_id", reqID)
			h.metrics.CacheOpsTotal.WithLabelValues("get", "error").Inc()
		} else if hit {
			h.metrics.CacheOpsTotal.WithLabelValues("get", "hit").Inc()
			cached = &snap
		} else {
			h.metrics.CacheOpsTotal.WithLabelValues("get", "miss").Inc()
		}
	}

	dec := Decide(h.cfg, slug, cached, time.Now())

	var snap weather.Snapshot
	var source string

	switch dec.Action {
	case ActionCache:
		snap = *cached
		source = "cache"

	case ActionBypass, ActionOrigin:
		if dec.Action == ActionBypass {
			source = "bypass"
		} else {
			source = "origin"
		}
		var fetchErr error
		snap, fetchErr = h.wc.Fetch(r.Context(), city)
		if fetchErr != nil {
			h.log.Error("origin_fetch_error", "city", slug, "err", fetchErr.Error(), "request_id", reqID)
			h.metrics.OriginRequestTotal.WithLabelValues(slug, "error").Inc()
			h.recordRequest(slug, source, http.StatusBadGateway, start)
			writeJSON(w, http.StatusBadGateway, errBody("origin_unavailable", "could not reach the weather provider"))
			return
		}
		h.metrics.OriginRequestTotal.WithLabelValues(slug, "success").Inc()

		// Write to cache only for non-bypass cache misses.
		if dec.Action == ActionOrigin && h.store != nil {
			if err := h.store.SetWithTTL(r.Context(), snap, h.cfg.TTL); err != nil {
				h.log.Warn("cache_set_error", "city", slug, "err", err.Error(), "request_id", reqID)
				h.metrics.CacheOpsTotal.WithLabelValues("set", "error").Inc()
			} else {
				h.metrics.CacheOpsTotal.WithLabelValues("set", "success").Inc()
			}
		}

	default:
		// ActionUnknown — unreachable after the catalog check above, but kept for safety.
		writeJSON(w, http.StatusNotFound, errBody("unknown_city", fmt.Sprintf("city %q not in catalog", slug)))
		return
	}

	snap.Source = source
	h.log.Info("request",
		"city", slug,
		"source", source,
		"status_code", http.StatusOK,
		"latency_ms", time.Since(start).Milliseconds(),
		"request_id", reqID,
	)
	h.recordRequest(slug, source, http.StatusOK, start)

	w.Header().Set("X-Source", source)
	w.Header().Set("X-Request-ID", reqID)
	writeJSON(w, http.StatusOK, snap)
}

// Health returns 200 ok for liveness and readiness probes.
func Health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintln(w, "ok") //nolint:errcheck
}

// StaticFS returns the sub-filesystem rooted at web/static for the file server.
func StaticFS() (fs.FS, error) {
	return fs.Sub(web.FS, "static")
}

func (h *Handlers) recordRequest(city, source string, status int, start time.Time) {
	latency := time.Since(start).Seconds()
	h.metrics.RequestsTotal.WithLabelValues(city, source, strconv.Itoa(status)).Inc()
	h.metrics.RequestDuration.WithLabelValues(city, source).Observe(latency)
}

// newRequestID generates a 16-hex-char random correlation ID.
func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func errBody(code, msg string) map[string]string {
	return map[string]string{"error": code, "message": msg}
}
