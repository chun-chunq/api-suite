package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(srv *httptest.Server) *Client {
	return &Client{
		http:    srv.Client(),
		baseURL: srv.URL,
	}
}

// ── AQI category tests ────────────────────────────────────────────────────────

func TestEuropeanAQICategory(t *testing.T) {
	cases := []struct {
		val      float64
		expected string
	}{
		{0, "Good"},
		{20, "Good"},
		{21, "Fair"},
		{40, "Fair"},
		{41, "Moderate"},
		{60, "Moderate"},
		{61, "Poor"},
		{80, "Poor"},
		{81, "Very Poor"},
		{100, "Very Poor"},
		{101, "Extremely Poor"},
	}
	for _, tc := range cases {
		cat := europeanAQICategory(tc.val)
		if cat == nil {
			t.Errorf("nil category for value %f", tc.val)
			continue
		}
		if cat.Category != tc.expected {
			t.Errorf("value %f: expected %q, got %q", tc.val, tc.expected, cat.Category)
		}
		if cat.Color == "" {
			t.Errorf("value %f: missing color", tc.val)
		}
	}
}

func TestUSAQICategory(t *testing.T) {
	cases := []struct {
		val      float64
		expected string
	}{
		{0, "Good"},
		{50, "Good"},
		{51, "Moderate"},
		{100, "Moderate"},
		{101, "Unhealthy for Sensitive Groups"},
		{150, "Unhealthy for Sensitive Groups"},
		{151, "Unhealthy"},
		{200, "Unhealthy"},
		{201, "Very Unhealthy"},
		{300, "Very Unhealthy"},
		{301, "Hazardous"},
	}
	for _, tc := range cases {
		cat := usAQICategory(tc.val)
		if cat == nil {
			t.Errorf("nil category for value %f", tc.val)
			continue
		}
		if cat.Category != tc.expected {
			t.Errorf("value %f: expected %q, got %q", tc.val, tc.expected, cat.Category)
		}
	}
}

func TestEuropeanAQICategory_Negative(t *testing.T) {
	if europeanAQICategory(-1) != nil {
		t.Error("expected nil for negative value")
	}
}

func TestUSAQICategory_Negative(t *testing.T) {
	if usAQICategory(-1) != nil {
		t.Error("expected nil for negative value")
	}
}

// ── GetCurrent tests ──────────────────────────────────────────────────────────

func TestGetCurrent_OK(t *testing.T) {
	raw := map[string]interface{}{
		"latitude": 52.52, "longitude": 13.41, "timezone": "Europe/Berlin",
		"current": map[string]interface{}{
			"time":                    "2024-01-15T12:00",
			"pm2_5":                   float64(8.3),
			"pm10":                    float64(12.1),
			"carbon_monoxide":         float64(220.0),
			"nitrogen_dioxide":        float64(15.2),
			"sulphur_dioxide":         float64(2.1),
			"ozone":                   float64(88.5),
			"aerosol_optical_depth":   float64(0.15),
			"dust":                    float64(5.0),
			"uv_index":                float64(3.2),
			"uv_index_clear_sky":      float64(3.5),
			"european_aqi":            float64(18.0),
			"us_aqi":                  float64(35.0),
		},
	}
	b, _ := json.Marshal(raw)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.GetCurrent(context.Background(), 52.52, 13.41, "Europe/Berlin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Current == nil {
		t.Fatal("expected current data")
	}
	if result.Current.PM2_5 != 8.3 {
		t.Errorf("unexpected PM2.5: %f", result.Current.PM2_5)
	}
	if result.Current.EuropeanAQI == nil {
		t.Fatal("expected European AQI")
	}
	if result.Current.EuropeanAQI.Category != "Good" {
		t.Errorf("expected Good category for AQI=18, got %q", result.Current.EuropeanAQI.Category)
	}
	if result.Current.USAQI == nil {
		t.Fatal("expected US AQI")
	}
	if result.Current.USAQI.Category != "Good" {
		t.Errorf("expected Good US AQI for 35, got %q", result.Current.USAQI.Category)
	}
}

func TestGetCurrent_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":true,"reason":"Latitude must be in range of -90 to 90"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.GetCurrent(context.Background(), 999, 999, "auto")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
}

// ── GetForecast tests ─────────────────────────────────────────────────────────

func TestGetForecast_OK(t *testing.T) {
	raw := map[string]interface{}{
		"latitude": 48.8566, "longitude": 2.3522, "timezone": "Europe/Paris",
		"hourly": map[string]interface{}{
			"time":              []string{"2024-01-15T00:00", "2024-01-15T01:00", "2024-01-15T02:00"},
			"pm2_5":             []float64{5.1, 5.3, 4.9},
			"pm10":              []float64{8.0, 8.2, 7.8},
			"ozone":             []float64{70.1, 71.0, 69.5},
			"nitrogen_dioxide":  []float64{12.0, 13.1, 11.5},
			"uv_index":          []float64{0.0, 0.0, 0.0},
		},
	}
	b, _ := json.Marshal(raw)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify forecast_hours param is passed
		fh := r.URL.Query().Get("forecast_hours")
		if fh == "" {
			http.Error(w, "missing forecast_hours", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.GetForecast(context.Background(), 48.8566, 2.3522, "Europe/Paris", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Hourly) != 3 {
		t.Errorf("expected 3 hourly entries, got %d", len(result.Hourly))
	}
	if result.Hourly[0].PM2_5 != 5.1 {
		t.Errorf("unexpected PM2.5: %f", result.Hourly[0].PM2_5)
	}
	if result.Hourly[0].Time != "2024-01-15T00:00" {
		t.Errorf("unexpected time: %q", result.Hourly[0].Time)
	}
}

func TestGetForecast_DefaultHours(t *testing.T) {
	// hours=0 should default to 24
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fh := r.URL.Query().Get("forecast_hours")
		if fh != "24" {
			http.Error(w, "expected forecast_hours=24, got "+fh, 400)
			return
		}
		raw := map[string]interface{}{
			"latitude": 0.0, "longitude": 0.0, "timezone": "UTC",
			"hourly": map[string]interface{}{
				"time": []string{},
			},
		}
		b, _ := json.Marshal(raw)
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.GetForecast(context.Background(), 0, 0, "UTC", 0) // 0 → default 24
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
