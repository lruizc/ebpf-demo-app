// Package cache provides a Redis-backed store for WeatherSnapshot values.
package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/luisruiz/weather-app/internal/weather"
)

const keyPrefix = "weather:v1:"

// Store wraps a Redis client with typed get/set operations for weather.Snapshot.
type Store struct {
	rdb *redis.Client
	log *slog.Logger
}

// New dials Redis at addr, pings it, and returns a Store.
// Returns an error if the initial connection or ping fails.
func New(addr string, log *slog.Logger) (*Store, error) {
	rdb := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis dial %s: %w", addr, err)
	}
	return &Store{rdb: rdb, log: log}, nil
}

// Get retrieves a cached Snapshot for slug.
// Returns (snap, true, nil) on hit, (zero, false, nil) on miss or corrupt entry.
func (s *Store) Get(ctx context.Context, slug string) (weather.Snapshot, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()

	raw, err := s.rdb.Get(ctx, keyPrefix+slug).Bytes()
	if errors.Is(err, redis.Nil) {
		return weather.Snapshot{}, false, nil
	}
	if err != nil {
		return weather.Snapshot{}, false, fmt.Errorf("redis GET %s: %w", slug, err)
	}

	var snap weather.Snapshot
	if err := json.Unmarshal(raw, &snap); err != nil {
		s.log.Warn("cache_corrupt", "slug", slug, "err", err.Error())
		return weather.Snapshot{}, false, nil
	}
	return snap, true, nil
}

// SetWithTTL stores snap in Redis under its city slug with the given TTL.
func (s *Store) SetWithTTL(ctx context.Context, snap weather.Snapshot, ttl time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()

	raw, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}
	return s.rdb.Set(ctx, keyPrefix+snap.City, raw, ttl).Err()
}
