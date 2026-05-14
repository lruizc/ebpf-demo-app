package weather

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/luisruiz/weather-app/internal/config"
)

var bogota = config.City{Slug: "bogota", DisplayName: "Bogotá", Latitude: 4.711, Longitude: -74.072}

func weatherServer(code int, weathercode int, temp float64) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if code != http.StatusOK {
			http.Error(w, "upstream error", code)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck
			"current_weather": map[string]any{
				"temperature": temp,
				"weathercode": weathercode,
				"time":        "2026-05-14T15:00",
			},
		})
	}))
}

func TestFetchHappyPath(t *testing.T) {
	srv := weatherServer(http.StatusOK, 2, 14.5)
	defer srv.Close()

	snap, err := New(srv.URL, nil).Fetch(context.Background(), bogota)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.TemperatureC != 14.5 {
		t.Errorf("TemperatureC: got %v, want 14.5", snap.TemperatureC)
	}
	if snap.Condition != "Partly cloudy" {
		t.Errorf("Condition: got %q, want %q", snap.Condition, "Partly cloudy")
	}
	if snap.WeatherCode != 2 {
		t.Errorf("WeatherCode: got %d, want 2", snap.WeatherCode)
	}
	if snap.City != "bogota" {
		t.Errorf("City: got %q, want %q", snap.City, "bogota")
	}
	if snap.DisplayName != "Bogotá" {
		t.Errorf("DisplayName: got %q, want %q", snap.DisplayName, "Bogotá")
	}
}

func TestFetchWMOCodes(t *testing.T) {
	cases := []struct {
		code      int
		condition string
	}{
		{0, "Clear sky"},
		{63, "Moderate rain"},
		{95, "Thunderstorm"},
	}
	for _, tc := range cases {
		srv := weatherServer(http.StatusOK, tc.code, 20.0)
		snap, err := New(srv.URL, nil).Fetch(context.Background(), bogota)
		srv.Close()
		if err != nil {
			t.Errorf("code %d: unexpected error: %v", tc.code, err)
			continue
		}
		if snap.Condition != tc.condition {
			t.Errorf("code %d: Condition = %q, want %q", tc.code, snap.Condition, tc.condition)
		}
	}
}

func TestFetchHTTPError(t *testing.T) {
	srv := weatherServer(http.StatusServiceUnavailable, 0, 0)
	defer srv.Close()

	_, err := New(srv.URL, nil).Fetch(context.Background(), bogota)
	if err == nil {
		t.Fatal("expected error for 503 response, got nil")
	}
}

func TestFetchMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json")) //nolint:errcheck
	}))
	defer srv.Close()

	_, err := New(srv.URL, nil).Fetch(context.Background(), bogota)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestFetchTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]any{"current_weather": map[string]any{}}) //nolint:errcheck
	}))
	defer srv.Close()

	hc := &http.Client{Timeout: 10 * time.Millisecond}
	_, err := New(srv.URL, hc).Fetch(context.Background(), bogota)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}
