package server

import (
	"testing"
	"time"

	"github.com/luisruiz/weather-app/internal/config"
	"github.com/luisruiz/weather-app/internal/weather"
)

func testConfig() *config.Config {
	cities := []config.City{
		{Slug: "bogota", DisplayName: "Bogotá", Latitude: 4.711, Longitude: -74.072},
		{Slug: "sao-paulo", DisplayName: "São Paulo", Latitude: -23.55, Longitude: -46.63},
	}
	return &config.Config{
		Cities: cities,
		CityBySlug: map[string]config.City{
			"bogota":    cities[0],
			"sao-paulo": cities[1],
		},
		BypassCities: map[string]bool{"sao-paulo": true},
		TTL:          time.Hour,
	}
}

// TestDecideUnknownCity covers the "unknown city" row of the side-effects table.
func TestDecideUnknownCity(t *testing.T) {
	d := Decide(testConfig(), "atlantis", nil, time.Now())
	if d.Action != ActionUnknown {
		t.Errorf("got %q, want %q", d.Action, ActionUnknown)
	}
}

// TestDecideBypassCity covers the "bypass city" row (no Redis read, no Redis write).
func TestDecideBypassCity(t *testing.T) {
	d := Decide(testConfig(), "sao-paulo", nil, time.Now())
	if d.Action != ActionBypass {
		t.Errorf("got %q, want %q", d.Action, ActionBypass)
	}
}

// TestDecideCacheHit covers the "non-bypass cache hit" row.
func TestDecideCacheHit(t *testing.T) {
	now := time.Now()
	cached := &weather.Snapshot{FetchedAt: now.Add(-30 * time.Minute)}
	d := Decide(testConfig(), "bogota", cached, now)
	if d.Action != ActionCache {
		t.Errorf("got %q, want %q", d.Action, ActionCache)
	}
}

// TestDecideCacheMiss covers the "non-bypass cache miss" row (nil cached).
func TestDecideCacheMiss(t *testing.T) {
	d := Decide(testConfig(), "bogota", nil, time.Now())
	if d.Action != ActionOrigin {
		t.Errorf("got %q, want %q", d.Action, ActionOrigin)
	}
}

// TestDecideCacheExpired verifies an expired entry is treated as a miss.
func TestDecideCacheExpired(t *testing.T) {
	now := time.Now()
	expired := &weather.Snapshot{FetchedAt: now.Add(-2 * time.Hour)} // beyond 1h TTL
	d := Decide(testConfig(), "bogota", expired, now)
	if d.Action != ActionOrigin {
		t.Errorf("got %q, want %q", d.Action, ActionOrigin)
	}
}
