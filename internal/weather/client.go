package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/luisruiz/weather-app/internal/config"
)

// codeToCondition maps WMO weather interpretation codes to human-readable strings.
var codeToCondition = map[int]string{
	0:  "Clear sky",
	1:  "Mainly clear",
	2:  "Partly cloudy",
	3:  "Overcast",
	45: "Foggy",
	48: "Icy fog",
	51: "Light drizzle",
	53: "Moderate drizzle",
	55: "Dense drizzle",
	61: "Slight rain",
	63: "Moderate rain",
	65: "Heavy rain",
	71: "Slight snow",
	73: "Moderate snow",
	75: "Heavy snow",
	80: "Slight showers",
	81: "Moderate showers",
	82: "Heavy showers",
	95: "Thunderstorm",
	96: "Thunderstorm with hail",
	99: "Thunderstorm with heavy hail",
}

// Client fetches current weather data from Open-Meteo.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// New constructs a Client. If httpClient is nil, a default with a 3-second timeout is used.
func New(baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 3 * time.Second}
	}
	return &Client{baseURL: baseURL, httpClient: httpClient}
}

type openMeteoResponse struct {
	CurrentWeather struct {
		Temperature float64 `json:"temperature"`
		Weathercode int     `json:"weathercode"`
		Time        string  `json:"time"`
	} `json:"current_weather"`
}

// Fetch retrieves the current weather snapshot for city from the Open-Meteo API.
func (c *Client) Fetch(ctx context.Context, city config.City) (Snapshot, error) {
	url := fmt.Sprintf(
		"%s?latitude=%.4f&longitude=%.4f&current_weather=true&timezone=auto",
		c.baseURL, city.Latitude, city.Longitude,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("fetch weather: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Snapshot{}, fmt.Errorf("open-meteo returned HTTP %d", resp.StatusCode)
	}

	var omr openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&omr); err != nil {
		return Snapshot{}, fmt.Errorf("decode response: %w", err)
	}

	condition, ok := codeToCondition[omr.CurrentWeather.Weathercode]
	if !ok {
		condition = fmt.Sprintf("Code %d", omr.CurrentWeather.Weathercode)
	}

	fetchedAt := time.Now().UTC()
	if t, err := time.Parse("2006-01-02T15:04", omr.CurrentWeather.Time); err == nil {
		fetchedAt = t.UTC()
	}

	return Snapshot{
		City:         city.Slug,
		DisplayName:  city.DisplayName,
		TemperatureC: omr.CurrentWeather.Temperature,
		Condition:    condition,
		WeatherCode:  omr.CurrentWeather.Weathercode,
		FetchedAt:    fetchedAt,
	}, nil
}
