package cache

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/luisruiz/weather-app/internal/weather"
)

func newTestStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	store, err := New(mr.Addr(), log)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return store, mr
}

func TestGetMiss(t *testing.T) {
	store, _ := newTestStore(t)
	_, hit, err := store.Get(context.Background(), "bogota")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hit {
		t.Error("expected miss, got hit")
	}
}

func TestSetAndGet(t *testing.T) {
	store, _ := newTestStore(t)
	snap := weather.Snapshot{
		City:         "bogota",
		DisplayName:  "Bogotá",
		TemperatureC: 14.5,
		Condition:    "Partly cloudy",
		WeatherCode:  2,
		FetchedAt:    time.Now().UTC().Truncate(time.Second),
	}
	if err := store.SetWithTTL(context.Background(), snap, time.Hour); err != nil {
		t.Fatalf("SetWithTTL: %v", err)
	}
	got, hit, err := store.Get(context.Background(), "bogota")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !hit {
		t.Fatal("expected hit, got miss")
	}
	if got.TemperatureC != snap.TemperatureC {
		t.Errorf("TemperatureC: got %v, want %v", got.TemperatureC, snap.TemperatureC)
	}
	if got.Condition != snap.Condition {
		t.Errorf("Condition: got %q, want %q", got.Condition, snap.Condition)
	}
}

func TestTTLExpiry(t *testing.T) {
	store, mr := newTestStore(t)
	snap := weather.Snapshot{City: "lima", DisplayName: "Lima", TemperatureC: 20.0}
	if err := store.SetWithTTL(context.Background(), snap, time.Hour); err != nil {
		t.Fatalf("SetWithTTL: %v", err)
	}
	mr.FastForward(2 * time.Hour)
	_, hit, err := store.Get(context.Background(), "lima")
	if err != nil {
		t.Fatalf("Get after expiry: %v", err)
	}
	if hit {
		t.Error("expected miss after TTL expiry, got hit")
	}
}

func TestCorruptJSONTreatedAsMiss(t *testing.T) {
	store, mr := newTestStore(t)
	mr.Set(keyPrefix+"santiago", "not-valid-json")
	_, hit, err := store.Get(context.Background(), "santiago")
	if err != nil {
		t.Fatalf("unexpected error on corrupt entry: %v", err)
	}
	if hit {
		t.Error("expected miss for corrupt JSON, got hit")
	}
}
