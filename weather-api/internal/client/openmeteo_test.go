package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(srv *httptest.Server) *Client {
	c := New()
	c.http = srv.Client()
	c.baseURL = srv.URL + "/v1"
	c.geocodeURL = srv.URL + "/v1"
	return c
}

// ── wmoDescription ────────────────────────────────────────────────────────────

func TestWmoDescription(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{0, "Clear sky"},
		{3, "Overcast"},
		{61, "Slight rain"},
		{95, "Thunderstorm"},
		{999, "Weather code 999"},
	}
	for _, tt := range tests {
		got := wmoDescription(tt.code)
		if got != tt.want {
			t.Errorf("wmoDescription(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

// ── SearchLocations ───────────────────────────────────────────────────────────

func TestSearchLocations_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/search" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []map[string]interface{}{
				{
					"id":           1234,
					"name":         "Berlin",
					"latitude":     52.52,
					"longitude":    13.405,
					"country":      "Germany",
					"country_code": "DE",
					"admin1":       "Berlin",
					"timezone":     "Europe/Berlin",
				},
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	locs, err := c.SearchLocations(context.Background(), "Berlin", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(locs) != 1 {
		t.Fatalf("expected 1 location, got %d", len(locs))
	}
	if locs[0].Name != "Berlin" {
		t.Errorf("expected Berlin, got %s", locs[0].Name)
	}
	if locs[0].CountryCode != "DE" {
		t.Errorf("expected DE, got %s", locs[0].CountryCode)
	}
}

func TestSearchLocations_EmptyName(t *testing.T) {
	c := New()
	_, err := c.SearchLocations(context.Background(), "", 5)
	if err == nil {
		t.Error("expected error for empty name")
	}
}

// ── GetCurrent ────────────────────────────────────────────────────────────────

func currentResponse() map[string]interface{} {
	return map[string]interface{}{
		"latitude":  52.52,
		"longitude": 13.405,
		"timezone":  "Europe/Berlin",
		"current": map[string]interface{}{
			"time":                   "2024-01-15T14:00",
			"temperature_2m":         8.5,
			"apparent_temperature":   5.2,
			"relative_humidity_2m":   75,
			"wind_speed_10m":         15.3,
			"wind_direction_10m":     225,
			"weather_code":           3,
			"is_day":                 1,
			"precipitation":          0.0,
			"cloud_cover":            85,
			"surface_pressure":       1013.2,
			"visibility":             10000.0,
		},
	}
}

func TestGetCurrent_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(currentResponse())
	}))
	defer srv.Close()

	c := newTestClient(srv)
	cur, err := c.GetCurrent(context.Background(), 52.52, 13.405, "Europe/Berlin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cur.Temperature != 8.5 {
		t.Errorf("temperature: got %v, want 8.5", cur.Temperature)
	}
	if cur.WeatherDesc != "Overcast" {
		t.Errorf("weather desc: got %q, want Overcast", cur.WeatherDesc)
	}
	if !cur.IsDay {
		t.Error("expected IsDay=true")
	}
}

func TestGetCurrent_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error":  true,
			"reason": "Latitude must be in range of -90 to 90",
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.GetCurrent(context.Background(), 999, 0, "auto")
	if err == nil {
		t.Error("expected error for bad coordinates")
	}
}

// ── GetForecast ───────────────────────────────────────────────────────────────

func forecastResponse() map[string]interface{} {
	return map[string]interface{}{
		"latitude":  52.52,
		"longitude": 13.405,
		"timezone":  "Europe/Berlin",
		"current": map[string]interface{}{
			"time":                   "2024-01-15T14:00",
			"temperature_2m":         8.5,
			"apparent_temperature":   5.2,
			"relative_humidity_2m":   75,
			"wind_speed_10m":         15.3,
			"wind_direction_10m":     225,
			"weather_code":           3,
			"is_day":                 1,
			"precipitation":          0.0,
			"cloud_cover":            85,
			"surface_pressure":       1013.2,
		},
		"daily": map[string]interface{}{
			"time":                          []string{"2024-01-15", "2024-01-16", "2024-01-17"},
			"temperature_2m_max":            []float64{10.1, 12.3, 9.5},
			"temperature_2m_min":            []float64{3.2, 5.0, 2.8},
			"precipitation_sum":             []float64{0.0, 2.5, 0.8},
			"precipitation_probability_max": []int{10, 80, 40},
			"wind_speed_10m_max":            []float64{20.0, 35.0, 15.0},
			"weather_code":                  []int{1, 63, 61},
			"sunrise":                       []string{"2024-01-15T08:15", "2024-01-16T08:14", "2024-01-17T08:13"},
			"sunset":                        []string{"2024-01-15T16:30", "2024-01-16T16:31", "2024-01-17T16:32"},
			"uv_index_max":                  []float64{1.5, 0.5, 2.0},
		},
		"hourly": map[string]interface{}{
			"time":                     []string{"2024-01-15T00:00", "2024-01-15T01:00"},
			"temperature_2m":           []float64{7.0, 6.5},
			"relative_humidity_2m":     []int{80, 82},
			"precipitation_probability": []int{5, 5},
			"precipitation":            []float64{0.0, 0.0},
			"wind_speed_10m":           []float64{12.0, 11.0},
			"weather_code":             []int{0, 1},
			"is_day":                   []int{0, 0},
		},
	}
}

func TestGetForecast_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(forecastResponse())
	}))
	defer srv.Close()

	c := newTestClient(srv)
	fc, err := c.GetForecast(context.Background(), 52.52, 13.405, "Europe/Berlin", 3, 24)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fc.Current == nil {
		t.Fatal("expected current weather")
	}
	if len(fc.Daily) != 3 {
		t.Errorf("expected 3 daily, got %d", len(fc.Daily))
	}
	if fc.Daily[1].WeatherDesc != "Moderate rain" {
		t.Errorf("day 2 desc: got %q", fc.Daily[1].WeatherDesc)
	}
	if len(fc.Hourly) != 2 {
		t.Errorf("expected 2 hourly, got %d", len(fc.Hourly))
	}
	if fc.Hourly[0].WeatherDesc != "Clear sky" {
		t.Errorf("hour 0 desc: got %q", fc.Hourly[0].WeatherDesc)
	}
}

func TestGetForecast_NoHourly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// check hourly param NOT sent when hourlyHours=0
		if r.URL.Query().Get("hourly") != "" {
			t.Error("hourly param should be absent when hourlyHours=0")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(forecastResponse())
	}))
	defer srv.Close()

	c := newTestClient(srv)
	fc, err := c.GetForecast(context.Background(), 52.52, 13.405, "auto", 3, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fc.Hourly) != 0 {
		t.Errorf("expected 0 hourly, got %d", len(fc.Hourly))
	}
}
