// Package client wraps the Open-Meteo Air Quality API.
// Docs: https://open-meteo.com/en/docs/air-quality-api
// No API key required. Free and open source.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBase    = "https://air-quality-api.open-meteo.com/v1"
	defaultTimeout = 15 * time.Second
)

// AQILevel maps a numeric AQI to a label.
type AQILevel struct {
	Value       float64 `json:"value"`
	Category    string  `json:"category"`    // Good / Moderate / Unhealthy etc.
	Color       string  `json:"color"`       // hex color for UI
}

// Current holds current air quality conditions.
type Current struct {
	Time             string  `json:"time"`
	PM2_5            float64 `json:"pm2_5,omitempty"`        // μg/m³
	PM10             float64 `json:"pm10,omitempty"`         // μg/m³
	CO               float64 `json:"carbon_monoxide,omitempty"` // μg/m³
	NO2              float64 `json:"nitrogen_dioxide,omitempty"` // μg/m³
	SO2              float64 `json:"sulphur_dioxide,omitempty"` // μg/m³
	Ozone            float64 `json:"ozone,omitempty"`        // μg/m³
	AerosolOD        float64 `json:"aerosol_optical_depth,omitempty"`
	Dust             float64 `json:"dust,omitempty"`
	UVIndex          float64 `json:"uv_index,omitempty"`
	UVIndexClearSky  float64 `json:"uv_index_clear_sky,omitempty"`
	EuropeanAQI      *AQILevel `json:"european_aqi,omitempty"`
	USAQI            *AQILevel `json:"us_aqi,omitempty"`
}

// HourlyEntry is a single hour's air quality data.
type HourlyEntry struct {
	Time    string  `json:"time"`
	PM2_5   float64 `json:"pm2_5,omitempty"`
	PM10    float64 `json:"pm10,omitempty"`
	Ozone   float64 `json:"ozone,omitempty"`
	NO2     float64 `json:"nitrogen_dioxide,omitempty"`
	UVIndex float64 `json:"uv_index,omitempty"`
}

// AirQualityResult is the complete response for a location.
type AirQualityResult struct {
	Latitude  float64      `json:"latitude"`
	Longitude float64      `json:"longitude"`
	Timezone  string       `json:"timezone"`
	Current   *Current     `json:"current,omitempty"`
	Hourly    []HourlyEntry `json:"hourly,omitempty"`
}

// Client is the Open-Meteo Air Quality API client.
type Client struct {
	http    *http.Client
	baseURL string
}

// New returns a new air quality client.
func New() *Client {
	return &Client{
		http:    &http.Client{Timeout: defaultTimeout},
		baseURL: defaultBase,
	}
}

// ── AQI category helpers ─────────────────────────────────────────────────────

func europeanAQICategory(v float64) *AQILevel {
	if v < 0 {
		return nil
	}
	var cat, color string
	switch {
	case v <= 20:
		cat, color = "Good", "#50f0e6"
	case v <= 40:
		cat, color = "Fair", "#50ccaa"
	case v <= 60:
		cat, color = "Moderate", "#f0e641"
	case v <= 80:
		cat, color = "Poor", "#ff5050"
	case v <= 100:
		cat, color = "Very Poor", "#960032"
	default:
		cat, color = "Extremely Poor", "#7d2181"
	}
	return &AQILevel{Value: v, Category: cat, Color: color}
}

func usAQICategory(v float64) *AQILevel {
	if v < 0 {
		return nil
	}
	var cat, color string
	switch {
	case v <= 50:
		cat, color = "Good", "#00e400"
	case v <= 100:
		cat, color = "Moderate", "#ffff00"
	case v <= 150:
		cat, color = "Unhealthy for Sensitive Groups", "#ff7e00"
	case v <= 200:
		cat, color = "Unhealthy", "#ff0000"
	case v <= 300:
		cat, color = "Very Unhealthy", "#8f3f97"
	default:
		cat, color = "Hazardous", "#7e0023"
	}
	return &AQILevel{Value: v, Category: cat, Color: color}
}

// ── Internal helpers ─────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, params url.Values, out interface{}) error {
	u := c.baseURL + "/air-quality?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Reason string `json:"reason"`
			Error  bool   `json:"error"`
		}
		if jsonErr := json.NewDecoder(resp.Body).Decode(&errResp); jsonErr == nil && errResp.Reason != "" {
			return fmt.Errorf("API error: %s", errResp.Reason)
		}
		return fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

// ── Public API ───────────────────────────────────────────────────────────────

// GetCurrent returns current air quality conditions for a lat/lon.
// timezone: IANA timezone string or "auto"
func (c *Client) GetCurrent(ctx context.Context, lat, lon float64, timezone string) (*AirQualityResult, error) {
	if timezone == "" {
		timezone = "auto"
	}

	params := url.Values{
		"latitude":  []string{fmt.Sprintf("%.4f", lat)},
		"longitude": []string{fmt.Sprintf("%.4f", lon)},
		"timezone":  []string{timezone},
		"current": []string{strings.Join([]string{
			"pm2_5", "pm10", "carbon_monoxide", "nitrogen_dioxide",
			"sulphur_dioxide", "ozone", "aerosol_optical_depth", "dust",
			"uv_index", "uv_index_clear_sky",
			"european_aqi", "us_aqi",
		}, ",")},
	}

	var raw struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Timezone  string  `json:"timezone"`
		Current   struct {
			Time            string  `json:"time"`
			PM2_5           float64 `json:"pm2_5"`
			PM10            float64 `json:"pm10"`
			CO              float64 `json:"carbon_monoxide"`
			NO2             float64 `json:"nitrogen_dioxide"`
			SO2             float64 `json:"sulphur_dioxide"`
			Ozone           float64 `json:"ozone"`
			AerosolOD       float64 `json:"aerosol_optical_depth"`
			Dust            float64 `json:"dust"`
			UVIndex         float64 `json:"uv_index"`
			UVIndexClearSky float64 `json:"uv_index_clear_sky"`
			EuropeanAQI     float64 `json:"european_aqi"`
			USAQI           float64 `json:"us_aqi"`
		} `json:"current"`
	}

	if err := c.get(ctx, params, &raw); err != nil {
		return nil, err
	}

	cur := &Current{
		Time:            raw.Current.Time,
		PM2_5:           raw.Current.PM2_5,
		PM10:            raw.Current.PM10,
		CO:              raw.Current.CO,
		NO2:             raw.Current.NO2,
		SO2:             raw.Current.SO2,
		Ozone:           raw.Current.Ozone,
		AerosolOD:       raw.Current.AerosolOD,
		Dust:            raw.Current.Dust,
		UVIndex:         raw.Current.UVIndex,
		UVIndexClearSky: raw.Current.UVIndexClearSky,
		EuropeanAQI:     europeanAQICategory(raw.Current.EuropeanAQI),
		USAQI:           usAQICategory(raw.Current.USAQI),
	}

	return &AirQualityResult{
		Latitude:  raw.Latitude,
		Longitude: raw.Longitude,
		Timezone:  raw.Timezone,
		Current:   cur,
	}, nil
}

// GetForecast returns hourly air quality forecast for the next N hours (max 48).
func (c *Client) GetForecast(ctx context.Context, lat, lon float64, timezone string, hours int) (*AirQualityResult, error) {
	if timezone == "" {
		timezone = "auto"
	}
	if hours <= 0 || hours > 48 {
		hours = 24
	}

	params := url.Values{
		"latitude":     []string{fmt.Sprintf("%.4f", lat)},
		"longitude":    []string{fmt.Sprintf("%.4f", lon)},
		"timezone":     []string{timezone},
		"forecast_hours": []string{fmt.Sprintf("%d", hours)},
		"hourly": []string{strings.Join([]string{
			"pm2_5", "pm10", "ozone", "nitrogen_dioxide", "uv_index",
		}, ",")},
	}

	var raw struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Timezone  string  `json:"timezone"`
		Hourly    struct {
			Time  []string  `json:"time"`
			PM2_5 []float64 `json:"pm2_5"`
			PM10  []float64 `json:"pm10"`
			Ozone []float64 `json:"ozone"`
			NO2   []float64 `json:"nitrogen_dioxide"`
			UV    []float64 `json:"uv_index"`
		} `json:"hourly"`
	}

	if err := c.get(ctx, params, &raw); err != nil {
		return nil, err
	}

	entries := make([]HourlyEntry, len(raw.Hourly.Time))
	for i, t := range raw.Hourly.Time {
		entry := HourlyEntry{Time: t}
		if i < len(raw.Hourly.PM2_5) {
			entry.PM2_5 = raw.Hourly.PM2_5[i]
		}
		if i < len(raw.Hourly.PM10) {
			entry.PM10 = raw.Hourly.PM10[i]
		}
		if i < len(raw.Hourly.Ozone) {
			entry.Ozone = raw.Hourly.Ozone[i]
		}
		if i < len(raw.Hourly.NO2) {
			entry.NO2 = raw.Hourly.NO2[i]
		}
		if i < len(raw.Hourly.UV) {
			entry.UVIndex = raw.Hourly.UV[i]
		}
		entries[i] = entry
	}

	return &AirQualityResult{
		Latitude:  raw.Latitude,
		Longitude: raw.Longitude,
		Timezone:  raw.Timezone,
		Hourly:    entries,
	}, nil
}
