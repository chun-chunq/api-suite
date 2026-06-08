// Package client wraps the OpenSky Network REST API.
// OpenSky provides live and historical ADS-B flight tracking data.
// Docs: https://openskynetwork.github.io/opensky-api/
// Auth: optional (anonymous = 400 API credits/day, registered = more)
// License: OpenSky Network Data License — freely usable for non-commercial research
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://opensky-network.org/api"

// Aircraft represents a live aircraft state vector from ADS-B.
type Aircraft struct {
	ICAO24       string   `json:"icao24"`        // unique ICAO 24-bit address (hex)
	Callsign     string   `json:"callsign"`      // flight callsign e.g. "DLH123"
	OriginCountry string  `json:"originCountry"`
	LastContact  int64    `json:"lastContact"`   // Unix timestamp
	Longitude    *float64 `json:"longitude,omitempty"`
	Latitude     *float64 `json:"latitude,omitempty"`
	AltBaro      *float64 `json:"altitudeBaro,omitempty"` // barometric altitude in meters
	AltGeo       *float64 `json:"altitudeGeo,omitempty"`  // geometric altitude in meters
	OnGround     bool     `json:"onGround"`
	Velocity     *float64 `json:"velocityMs,omitempty"`   // ground speed m/s
	Heading      *float64 `json:"heading,omitempty"`      // true track degrees
	VertRate     *float64 `json:"verticalRate,omitempty"` // climb/descend rate m/s
	Squawk       string   `json:"squawk,omitempty"`       // transponder code
	PositionSource int    `json:"positionSource"`         // 0=ADS-B, 1=ASTERIX, 2=MLAT
}

// Airport represents an airport from the OurAirports open dataset.
type Airport struct {
	ICAO      string  `json:"icao"`
	IATA      string  `json:"iata,omitempty"`
	Name      string  `json:"name"`
	City      string  `json:"city,omitempty"`
	Country   string  `json:"country"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Altitude  int     `json:"altitudeFt,omitempty"`
	Type      string  `json:"type"` // large_airport, medium_airport, small_airport
}

// Flight represents a completed flight record.
type Flight struct {
	ICAO24         string `json:"icao24"`
	Callsign       string `json:"callsign"`
	FirstSeen      int64  `json:"firstSeen"`
	LastSeen       int64  `json:"lastSeen"`
	DepartureAirport string `json:"departureAirport,omitempty"`
	ArrivalAirport   string `json:"arrivalAirport,omitempty"`
}

// BoundingBox for geographic filtering.
type BoundingBox struct {
	MinLat float64
	MaxLat float64
	MinLon float64
	MaxLon float64
}

// Client wraps OpenSky Network and OurAirports data.
type Client struct {
	http    *http.Client
	baseURL string
	username string // optional OpenSky account
	password string
}

// New creates a new aviation client.
// username/password are optional — provide for higher rate limits.
func New(username, password string) *Client {
	return &Client{
		http:     &http.Client{Timeout: 15 * time.Second},
		baseURL:  defaultBaseURL,
		username: username,
		password: password,
	}
}

func (c *Client) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenSky request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("OpenSky rate limit exceeded — try again in 10 seconds")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenSky HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// GetAllStates returns all live aircraft currently in the air (or within bounding box).
func (c *Client) GetAllStates(ctx context.Context, box *BoundingBox) ([]Aircraft, int64, error) {
	params := url.Values{}
	if box != nil {
		params.Set("lamin", fmt.Sprintf("%f", box.MinLat))
		params.Set("lamax", fmt.Sprintf("%f", box.MaxLat))
		params.Set("lomin", fmt.Sprintf("%f", box.MinLon))
		params.Set("lomax", fmt.Sprintf("%f", box.MaxLon))
	}

	body, err := c.get(ctx, "/states/all", params)
	if err != nil {
		return nil, 0, err
	}
	if body == nil {
		return nil, 0, nil
	}

	return c.parseStateVectors(body)
}

// GetAircraftByICAO returns the current state of a specific aircraft.
func (c *Client) GetAircraftByICAO(ctx context.Context, icao24 string) (*Aircraft, error) {
	icao24 = strings.ToLower(strings.TrimSpace(icao24))
	if icao24 == "" {
		return nil, fmt.Errorf("icao24 is required")
	}

	params := url.Values{}
	params.Set("icao24", icao24)

	body, err := c.get(ctx, "/states/all", params)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, nil
	}

	aircraft, _, err := c.parseStateVectors(body)
	if err != nil {
		return nil, err
	}
	if len(aircraft) == 0 {
		return nil, nil
	}
	return &aircraft[0], nil
}

// GetFlightsByAircraft returns recent flights for an aircraft by ICAO24.
// begin/end are Unix timestamps. Max time range: 30 days.
func (c *Client) GetFlightsByAircraft(ctx context.Context, icao24 string, begin, end int64) ([]Flight, error) {
	icao24 = strings.ToLower(strings.TrimSpace(icao24))
	if icao24 == "" {
		return nil, fmt.Errorf("icao24 is required")
	}

	params := url.Values{}
	params.Set("icao24", icao24)
	params.Set("begin", fmt.Sprintf("%d", begin))
	params.Set("end", fmt.Sprintf("%d", end))

	body, err := c.get(ctx, "/flights/aircraft", params)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return []Flight{}, nil
	}

	var raw []oskyFlight
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode flights: %w", err)
	}

	flights := make([]Flight, 0, len(raw))
	for _, r := range raw {
		flights = append(flights, Flight{
			ICAO24:           r.ICAO24,
			Callsign:         strings.TrimSpace(r.Callsign),
			FirstSeen:        r.FirstSeen,
			LastSeen:         r.LastSeen,
			DepartureAirport: r.EstDepartureAirport,
			ArrivalAirport:   r.EstArrivalAirport,
		})
	}
	return flights, nil
}

// ── Raw OpenSky JSON structures ────────────────────────────────────────────────

type oskyStatesResponse struct {
	Time   int64           `json:"time"`
	States [][]interface{} `json:"states"`
}

type oskyFlight struct {
	ICAO24              string `json:"icao24"`
	Callsign            string `json:"callsign"`
	FirstSeen           int64  `json:"firstSeen"`
	LastSeen            int64  `json:"lastSeen"`
	EstDepartureAirport string `json:"estDepartureAirport"`
	EstArrivalAirport   string `json:"estArrivalAirport"`
}

func (c *Client) parseStateVectors(body []byte) ([]Aircraft, int64, error) {
	var raw oskyStatesResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, 0, fmt.Errorf("decode state vectors: %w", err)
	}

	aircraft := make([]Aircraft, 0, len(raw.States))
	for _, s := range raw.States {
		a := parseStateVector(s)
		aircraft = append(aircraft, a)
	}
	return aircraft, raw.Time, nil
}

// parseStateVector maps an OpenSky state vector array to Aircraft struct.
// OpenSky returns state vectors as ordered arrays:
// [icao24, callsign, origin_country, time_position, last_contact, longitude,
//  latitude, baro_altitude, on_ground, velocity, true_track, vertical_rate,
//  sensors, geo_altitude, squawk, spi, position_source]
func parseStateVector(s []interface{}) Aircraft {
	a := Aircraft{}
	getString := func(i int) string {
		if i < len(s) && s[i] != nil {
			if v, ok := s[i].(string); ok {
				return strings.TrimSpace(v)
			}
		}
		return ""
	}
	getFloat := func(i int) *float64 {
		if i < len(s) && s[i] != nil {
			if v, ok := s[i].(float64); ok {
				return &v
			}
		}
		return nil
	}
	getBool := func(i int) bool {
		if i < len(s) && s[i] != nil {
			if v, ok := s[i].(bool); ok {
				return v
			}
		}
		return false
	}
	getInt64 := func(i int) int64 {
		if i < len(s) && s[i] != nil {
			if v, ok := s[i].(float64); ok {
				return int64(v)
			}
		}
		return 0
	}
	getInt := func(i int) int {
		if i < len(s) && s[i] != nil {
			if v, ok := s[i].(float64); ok {
				return int(v)
			}
		}
		return 0
	}

	a.ICAO24 = getString(0)
	a.Callsign = getString(1)
	a.OriginCountry = getString(2)
	a.LastContact = getInt64(4)
	a.Longitude = getFloat(5)
	a.Latitude = getFloat(6)
	a.AltBaro = getFloat(7)
	a.OnGround = getBool(8)
	a.Velocity = getFloat(9)
	a.Heading = getFloat(10)
	a.VertRate = getFloat(11)
	a.AltGeo = getFloat(13)
	a.Squawk = getString(14)
	a.PositionSource = getInt(16)
	return a
}
