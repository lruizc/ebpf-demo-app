package config

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var slugRe = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)

// Config holds the fully-resolved application configuration.
type Config struct {
	Cities       []City
	CityBySlug   map[string]City
	BypassCities map[string]bool
	TTL          time.Duration
	RedisAddr    string
	OpenMeteoURL string
	ListenAddr   string
	LogLevel     string
}

// Load resolves configuration from environment variables, applying baked-in defaults.
// Returns an error if any value is invalid or a bypass city slug is not in the catalog.
func Load() (*Config, error) {
	cities := DefaultCities()
	if raw := os.Getenv("CITIES"); raw != "" {
		var parsed []struct {
			Slug string  `json:"slug"`
			Name string  `json:"name"`
			Lat  float64 `json:"lat"`
			Lon  float64 `json:"lon"`
		}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			return nil, fmt.Errorf("CITIES: invalid JSON: %w", err)
		}
		cities = make([]City, 0, len(parsed))
		for _, p := range parsed {
			cities = append(cities, City{Slug: p.Slug, DisplayName: p.Name, Latitude: p.Lat, Longitude: p.Lon})
		}
	}
	if len(cities) == 0 {
		return nil, fmt.Errorf("city catalog is empty")
	}

	bySlug := make(map[string]City, len(cities))
	for _, c := range cities {
		if !slugRe.MatchString(c.Slug) {
			return nil, fmt.Errorf("city slug %q does not match required pattern ^[a-z][a-z0-9-]*[a-z0-9]$", c.Slug)
		}
		if c.Latitude < -90 || c.Latitude > 90 {
			return nil, fmt.Errorf("city %q: latitude %.4f out of [-90, 90]", c.Slug, c.Latitude)
		}
		if c.Longitude < -180 || c.Longitude > 180 {
			return nil, fmt.Errorf("city %q: longitude %.4f out of [-180, 180]", c.Slug, c.Longitude)
		}
		bySlug[c.Slug] = c
	}

	bypassCities := make(map[string]bool)
	for _, s := range strings.Split(envOr("BYPASS_CITIES", "sao-paulo"), ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := bySlug[s]; !ok {
			return nil, fmt.Errorf("BYPASS_CITIES: slug %q not found in city catalog", s)
		}
		bypassCities[s] = true
	}

	ttlSecs := 3600
	if raw := os.Getenv("TTL_SECONDS"); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			return nil, fmt.Errorf("TTL_SECONDS: must be a positive integer, got %q", raw)
		}
		ttlSecs = v
	}

	return &Config{
		Cities:       cities,
		CityBySlug:   bySlug,
		BypassCities: bypassCities,
		TTL:          time.Duration(ttlSecs) * time.Second,
		RedisAddr:    envOr("REDIS_ADDR", "redis:6379"),
		OpenMeteoURL: envOr("OPEN_METEO_URL", "https://api.open-meteo.com/v1/forecast"),
		ListenAddr:   envOr("LISTEN_ADDR", ":8080"),
		LogLevel:     envOr("LOG_LEVEL", "info"),
	}, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
