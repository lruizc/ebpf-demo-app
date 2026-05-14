package server

import (
	"fmt"
	"net/http"
)

// NewMux builds and returns the HTTP ServeMux for all application routes.
// Uses Go 1.22+ method+path routing patterns.
func NewMux(h *Handlers, metrics *Metrics) (*http.ServeMux, error) {
	staticFS, err := StaticFS()
	if err != nil {
		return nil, fmt.Errorf("build static filesystem: %w", err)
	}

	mux := http.NewServeMux()

	// Exact root match — Go 1.22 pattern {$} anchors to /
	mux.HandleFunc("GET /{$}", h.Index)

	// JSON weather API
	mux.HandleFunc("GET /api/weather", h.Weather)

	// Operational endpoints
	mux.HandleFunc("GET /healthz", Health)
	mux.Handle("GET /metrics", metrics.Handler())

	// Static assets — no method constraint so HEAD is also served
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	return mux, nil
}
