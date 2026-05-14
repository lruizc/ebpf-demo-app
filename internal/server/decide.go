package server

import (
	"time"

	"github.com/luisruiz/weather-app/internal/config"
	"github.com/luisruiz/weather-app/internal/weather"
)

// Action describes the resolved cache/origin/bypass decision.
type Action string

const (
	ActionCache   Action = "cache"
	ActionOrigin  Action = "origin"
	ActionBypass  Action = "bypass"
	ActionUnknown Action = "unknown"
)

// Decision is the result of Decide.
type Decision struct {
	Action Action
	Reason string
}

// Decide resolves the cache/origin/bypass action for slug with zero I/O.
// cached must be nil when there is no Redis entry (miss, expired, or store error).
// The decision is based purely on cfg, the cached snapshot, and the current time.
func Decide(cfg *config.Config, slug string, cached *weather.Snapshot, now time.Time) Decision {
	if _, ok := cfg.CityBySlug[slug]; !ok {
		return Decision{Action: ActionUnknown, Reason: "city not in catalog"}
	}
	if cfg.BypassCities[slug] {
		return Decision{Action: ActionBypass, Reason: "city in bypass policy"}
	}
	if cached != nil && now.Sub(cached.FetchedAt) < cfg.TTL {
		return Decision{Action: ActionCache, Reason: "fresh cache entry"}
	}
	return Decision{Action: ActionOrigin, Reason: "cache miss or expired"}
}
