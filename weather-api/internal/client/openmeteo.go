// Package client wraps the Open-Meteo API (open-meteo.com).
// Free, no auth, no rate-limit stated. Covers global coordinates.
// Docs: https://open-meteo.com/en/docs
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.open-meteo.com/v1"
const geocodeBaseURL = "https://geocoding-api.open-meteo.com/v1"

// ── Public types ─────────────────────────────────────────────────────────────

// CurrentWeather holds the current weather at a location.
type CurrentWeather struct {
	Latitude       float64   `json:"latitude"`
	Longitude      float64   `json:"longitude"`
	Timezone       string    `json:"timezone"`
	Time           time.Time `json:"time"`
	Temperature    float64   `json:"temperatureC"`
	FeelsLike      float64   `json:"feelsLikeC"`
	Humidity       int       `json:"humidityPercent"`
	WindSpeed      float64   `json:"windSpeedKmh"`
	WindDirection  int       `json:"windDirectionDeg"`
	WeatherCode    int       `json:"weatherCode"`
	WeatherDesc    string    `json:"weatherDescription"`
	IsDay          bool      `json:"isDay"`
	Precipitation  float64   `json:"precipitationMm"`
	CloudCover     int       `json:"cloudCoverPercent"`
	Pressure       float64   `json:"pressureHpa"`
	Visibility     float64   `json:"visibilityM,omitempty"`
}

// DailyForecast holds one day of forecast.
type DailyForecast struct {
	Date             string  `json:"date"`
	TempMax          float64 `json:"tempMaxC"`
	TempMin          float64 `json:"tempMinC"`
	PrecipSum        float64 `json:"precipSumMm"`
	PrecipProbMax    int     `json:"precipProbMaxPercent"`
	WindSpeedMax     float64 `json:"windSpeedMaxKmh"`
	WeatherCode      int     `json:"weatherCode"`
	WeatherDesc      string  `json:"weatherDescription"`
	SunriseUTC       string  `json:"sunriseUTC"`
	SunsetUTC        string  `json:"sunsetUTC"`
	UVIndexMax       float64 `json:"uvIndexMax"`
}

// HourlyPoint holds one hourly forecast step.
type HourlyPoint struct {
	Time          time.Time `json:"time"`
	Temperature   float64   `json:"temperatureC"`
	Humidity      int       `json:"humidityPercent"`
	PrecipProb    int       `json:"precipProbPercent"`
	Precipitation float64   `json:"precipitationMm"`
	WindSpeed     float64   `json:"windSpeedKmh"`
	WeatherCode   int       `json:"weatherCode"`
	WeatherDesc   string    `json:"weatherDescription"`
	IsDay         bool      `json:"isDay"`
}

// Forecast bundles current conditions plus hourly/daily arrays.
type Forecast struct {
	Latitude  float64         `json:"latitude"`
	Longitude float64         `json:"longitude"`
	Timezone  string          `json:"timezone"`
	Current   *CurrentWeather `json:"current,omitempty"`
	Hourly    []HourlyPoint   `json:"hourly,omitempty"`
	Daily     []DailyForecast `json:"daily,omitempty"`
}

// Location is a geocoded place.
type Location struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Country     string  `json:"country"`
	CountryCode string  `json:"countryCode"`
	Admin1      string  `json:"admin1,omitempty"` // state / region
	Timezone    string  `json:"timezone"`
}

// ── Client ────────────────────────────────────────────────────────────────────

type Client struct {
	http        *http.Client
	baseURL     string
	geocodeURL  string
}

func New() *Client {
	return &Client{
		http:       &http.Client{Timeout: 15 * time.Second},
		baseURL:    defaultBaseURL,
		geocodeURL: geocodeBaseURL,
	}
}

// ── Geocoding ─────────────────────────────────────────────────────────────────

// SearchLocations converts a place name to coordinates.
func (c *Client) SearchLocations(ctx context.Context, name string, maxResults int) ([]Location, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if maxResults <= 0 || maxResults > 20 {
		maxResults = 5
	}

	params := url.Values{}
	params.Set("name", name)
	params.Set("count", strconv.Itoa(maxResults))
	params.Set("language", "en")
	params.Set("format", "json")

	u := c.geocodeURL + "/search?" + params.Encode()

	var raw struct {
		Results []struct {
			ID          int     `json:"id"`
			Name        string  `json:"name"`
			Latitude    float64 `json:"latitude"`
			Longitude   float64 `json:"longitude"`
			Country     string  `json:"country"`
			CountryCode string  `json:"country_code"`
			Admin1      string  `json:"admin1"`
			Timezone    string  `json:"timezone"`
		} `json:"results"`
	}
	if err := c.get(ctx, u, &raw); err != nil {
		return nil, err
	}

	locs := make([]Location, 0, len(raw.Results))
	for _, r := range raw.Results {
		locs = append(locs, Location{
			ID:          r.ID,
			Name:        r.Name,
			Latitude:    r.Latitude,
			Longitude:   r.Longitude,
			Country:     r.Country,
			CountryCode: r.CountryCode,
			Admin1:      r.Admin1,
			Timezone:    r.Timezone,
		})
	}
	return locs, nil
}

// ── Current weather ───────────────────────────────────────────────────────────

// GetCurrent fetches the current weather at lat/lon.
func (c *Client) GetCurrent(ctx context.Context, lat, lon float64, timezone string) (*CurrentWeather, error) {
	if timezone == "" {
		timezone = "auto"
	}

	params := url.Values{}
	params.Set("latitude", strconv.FormatFloat(lat, 'f', 6, 64))
	params.Set("longitude", strconv.FormatFloat(lon, 'f', 6, 64))
	params.Set("timezone", timezone)
	params.Set("current", strings.Join([]string{
		"temperature_2m",
		"apparent_temperature",
		"relative_humidity_2m",
		"wind_speed_10m",
		"wind_direction_10m",
		"weather_code",
		"is_day",
		"precipitation",
		"cloud_cover",
		"surface_pressure",
		"visibility",
	}, ","))

	u := c.baseURL + "/forecast?" + params.Encode()

	var raw struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Timezone  string  `json:"timezone"`
		Current   struct {
			Time                string  `json:"time"`
			Temperature2m       float64 `json:"temperature_2m"`
			ApparentTemp        float64 `json:"apparent_temperature"`
			RelativeHumidity2m  int     `json:"relative_humidity_2m"`
			WindSpeed10m        float64 `json:"wind_speed_10m"`
			WindDirection10m    int     `json:"wind_direction_10m"`
			WeatherCode         int     `json:"weather_code"`
			IsDay               int     `json:"is_day"`
			Precipitation       float64 `json:"precipitation"`
			CloudCover          int     `json:"cloud_cover"`
			SurfacePressure     float64 `json:"surface_pressure"`
			Visibility          float64 `json:"visibility"`
		} `json:"current"`
	}
	if err := c.get(ctx, u, &raw); err != nil {
		return nil, err
	}

	t, _ := time.Parse("2006-01-02T15:04", raw.Current.Time)

	return &CurrentWeather{
		Latitude:      raw.Latitude,
		Longitude:     raw.Longitude,
		Timezone:      raw.Timezone,
		Time:          t,
		Temperature:   raw.Current.Temperature2m,
		FeelsLike:     raw.Current.ApparentTemp,
		Humidity:      raw.Current.RelativeHumidity2m,
		WindSpeed:     raw.Current.WindSpeed10m,
		WindDirection: raw.Current.WindDirection10m,
		WeatherCode:   raw.Current.WeatherCode,
		WeatherDesc:   wmoDescription(raw.Current.WeatherCode),
		IsDay:         raw.Current.IsDay == 1,
		Precipitation: raw.Current.Precipitation,
		CloudCover:    raw.Current.CloudCover,
		Pressure:      raw.Current.SurfacePressure,
		Visibility:    raw.Current.Visibility,
	}, nil
}

// ── Forecast ─────────────────────────────────────────────────────────────────

// GetForecast returns up to `days` days of daily forecast plus the first
// `hourlyHours` hours of hourly data (max 168 = 7 days).
func (c *Client) GetForecast(ctx context.Context, lat, lon float64, timezone string, days int, hourlyHours int) (*Forecast, error) {
	if timezone == "" {
		timezone = "auto"
	}
	if days <= 0 || days > 16 {
		days = 7
	}
	if hourlyHours < 0 {
		hourlyHours = 0
	}
	if hourlyHours > 168 {
		hourlyHours = 168
	}

	params := url.Values{}
	params.Set("latitude", strconv.FormatFloat(lat, 'f', 6, 64))
	params.Set("longitude", strconv.FormatFloat(lon, 'f', 6, 64))
	params.Set("timezone", timezone)
	params.Set("forecast_days", strconv.Itoa(days))
	params.Set("daily", strings.Join([]string{
		"temperature_2m_max",
		"temperature_2m_min",
		"precipitation_sum",
		"precipitation_probability_max",
		"wind_speed_10m_max",
		"weather_code",
		"sunrise",
		"sunset",
		"uv_index_max",
	}, ","))
	if hourlyHours > 0 {
		params.Set("hourly", strings.Join([]string{
			"temperature_2m",
			"relative_humidity_2m",
			"precipitation_probability",
			"precipitation",
			"wind_speed_10m",
			"weather_code",
			"is_day",
		}, ","))
	}
	// also get current conditions
	params.Set("current", strings.Join([]string{
		"temperature_2m",
		"apparent_temperature",
		"relative_humidity_2m",
		"wind_speed_10m",
		"wind_direction_10m",
		"weather_code",
		"is_day",
		"precipitation",
		"cloud_cover",
		"surface_pressure",
	}, ","))

	u := c.baseURL + "/forecast?" + params.Encode()

	var raw struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
		Timezone  string  `json:"timezone"`
		Current   struct {
			Time               string  `json:"time"`
			Temperature2m      float64 `json:"temperature_2m"`
			ApparentTemp       float64 `json:"apparent_temperature"`
			RelativeHumidity2m int     `json:"relative_humidity_2m"`
			WindSpeed10m       float64 `json:"wind_speed_10m"`
			WindDirection10m   int     `json:"wind_direction_10m"`
			WeatherCode        int     `json:"weather_code"`
			IsDay              int     `json:"is_day"`
			Precipitation      float64 `json:"precipitation"`
			CloudCover         int     `json:"cloud_cover"`
			SurfacePressure    float64 `json:"surface_pressure"`
		} `json:"current"`
		Daily struct {
			Time                    []string  `json:"time"`
			TempMax                 []float64 `json:"temperature_2m_max"`
			TempMin                 []float64 `json:"temperature_2m_min"`
			PrecipSum               []float64 `json:"precipitation_sum"`
			PrecipProbMax           []int     `json:"precipitation_probability_max"`
			WindSpeedMax            []float64 `json:"wind_speed_10m_max"`
			WeatherCode             []int     `json:"weather_code"`
			Sunrise                 []string  `json:"sunrise"`
			Sunset                  []string  `json:"sunset"`
			UVIndexMax              []float64 `json:"uv_index_max"`
		} `json:"daily"`
		Hourly struct {
			Time          []string  `json:"time"`
			Temp          []float64 `json:"temperature_2m"`
			Humidity      []int     `json:"relative_humidity_2m"`
			PrecipProb    []int     `json:"precipitation_probability"`
			Precip        []float64 `json:"precipitation"`
			WindSpeed     []float64 `json:"wind_speed_10m"`
			WeatherCode   []int     `json:"weather_code"`
			IsDay         []int     `json:"is_day"`
		} `json:"hourly"`
	}
	if err := c.get(ctx, u, &raw); err != nil {
		return nil, err
	}

	// build current
	ct, _ := time.Parse("2006-01-02T15:04", raw.Current.Time)
	cur := &CurrentWeather{
		Latitude:      raw.Latitude,
		Longitude:     raw.Longitude,
		Timezone:      raw.Timezone,
		Time:          ct,
		Temperature:   raw.Current.Temperature2m,
		FeelsLike:     raw.Current.ApparentTemp,
		Humidity:      raw.Current.RelativeHumidity2m,
		WindSpeed:     raw.Current.WindSpeed10m,
		WindDirection: raw.Current.WindDirection10m,
		WeatherCode:   raw.Current.WeatherCode,
		WeatherDesc:   wmoDescription(raw.Current.WeatherCode),
		IsDay:         raw.Current.IsDay == 1,
		Precipitation: raw.Current.Precipitation,
		CloudCover:    raw.Current.CloudCover,
		Pressure:      raw.Current.SurfacePressure,
	}

	// build daily
	daily := make([]DailyForecast, 0, len(raw.Daily.Time))
	for i, dateStr := range raw.Daily.Time {
		d := DailyForecast{Date: dateStr}
		if i < len(raw.Daily.TempMax) {
			d.TempMax = raw.Daily.TempMax[i]
		}
		if i < len(raw.Daily.TempMin) {
			d.TempMin = raw.Daily.TempMin[i]
		}
		if i < len(raw.Daily.PrecipSum) {
			d.PrecipSum = raw.Daily.PrecipSum[i]
		}
		if i < len(raw.Daily.PrecipProbMax) {
			d.PrecipProbMax = raw.Daily.PrecipProbMax[i]
		}
		if i < len(raw.Daily.WindSpeedMax) {
			d.WindSpeedMax = raw.Daily.WindSpeedMax[i]
		}
		if i < len(raw.Daily.WeatherCode) {
			d.WeatherCode = raw.Daily.WeatherCode[i]
			d.WeatherDesc = wmoDescription(d.WeatherCode)
		}
		if i < len(raw.Daily.Sunrise) {
			d.SunriseUTC = raw.Daily.Sunrise[i]
		}
		if i < len(raw.Daily.Sunset) {
			d.SunsetUTC = raw.Daily.Sunset[i]
		}
		if i < len(raw.Daily.UVIndexMax) {
			d.UVIndexMax = raw.Daily.UVIndexMax[i]
		}
		daily = append(daily, d)
	}

	// build hourly (capped at hourlyHours)
	var hourly []HourlyPoint
	if hourlyHours > 0 {
		limit := hourlyHours
		if limit > len(raw.Hourly.Time) {
			limit = len(raw.Hourly.Time)
		}
		hourly = make([]HourlyPoint, 0, limit)
		for i := 0; i < limit; i++ {
			ht, _ := time.Parse("2006-01-02T15:04", raw.Hourly.Time[i])
			hp := HourlyPoint{Time: ht}
			if i < len(raw.Hourly.Temp) {
				hp.Temperature = raw.Hourly.Temp[i]
			}
			if i < len(raw.Hourly.Humidity) {
				hp.Humidity = raw.Hourly.Humidity[i]
			}
			if i < len(raw.Hourly.PrecipProb) {
				hp.PrecipProb = raw.Hourly.PrecipProb[i]
			}
			if i < len(raw.Hourly.Precip) {
				hp.Precipitation = raw.Hourly.Precip[i]
			}
			if i < len(raw.Hourly.WindSpeed) {
				hp.WindSpeed = raw.Hourly.WindSpeed[i]
			}
			if i < len(raw.Hourly.WeatherCode) {
				hp.WeatherCode = raw.Hourly.WeatherCode[i]
				hp.WeatherDesc = wmoDescription(hp.WeatherCode)
			}
			if i < len(raw.Hourly.IsDay) {
				hp.IsDay = raw.Hourly.IsDay[i] == 1
			}
			hourly = append(hourly, hp)
		}
	}

	return &Forecast{
		Latitude:  raw.Latitude,
		Longitude: raw.Longitude,
		Timezone:  raw.Timezone,
		Current:   cur,
		Hourly:    hourly,
		Daily:     daily,
	}, nil
}

// ── HTTP helper ───────────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, u string, dest interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "api-suite/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// Open-Meteo returns {"error":true,"reason":"..."} on errors
		var apiErr struct {
			Error  bool   `json:"error"`
			Reason string `json:"reason"`
		}
		if jsonErr := json.Unmarshal(body, &apiErr); jsonErr == nil && apiErr.Error {
			return fmt.Errorf("open-meteo error: %s", apiErr.Reason)
		}
		return fmt.Errorf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}

// ── WMO weather interpretation codes → human description ─────────────────────
// https://open-meteo.com/en/docs#weathervariables

func wmoDescription(code int) string {
	switch code {
	case 0:
		return "Clear sky"
	case 1:
		return "Mainly clear"
	case 2:
		return "Partly cloudy"
	case 3:
		return "Overcast"
	case 45:
		return "Foggy"
	case 48:
		return "Icy fog"
	case 51:
		return "Light drizzle"
	case 53:
		return "Moderate drizzle"
	case 55:
		return "Dense drizzle"
	case 56:
		return "Light freezing drizzle"
	case 57:
		return "Heavy freezing drizzle"
	case 61:
		return "Slight rain"
	case 63:
		return "Moderate rain"
	case 65:
		return "Heavy rain"
	case 66:
		return "Light freezing rain"
	case 67:
		return "Heavy freezing rain"
	case 71:
		return "Slight snowfall"
	case 73:
		return "Moderate snowfall"
	case 75:
		return "Heavy snowfall"
	case 77:
		return "Snow grains"
	case 80:
		return "Slight rain showers"
	case 81:
		return "Moderate rain showers"
	case 82:
		return "Violent rain showers"
	case 85:
		return "Slight snow showers"
	case 86:
		return "Heavy snow showers"
	case 95:
		return "Thunderstorm"
	case 96:
		return "Thunderstorm with slight hail"
	case 99:
		return "Thunderstorm with heavy hail"
	default:
		return fmt.Sprintf("Weather code %d", code)
	}
}
