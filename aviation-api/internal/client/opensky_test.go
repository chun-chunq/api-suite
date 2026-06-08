package client

import (
	"testing"
)

func TestNew(t *testing.T) {
	c := New("", "")
	if c == nil {
		t.Fatal("New() returned nil")
	}
}

func TestGetAircraftByICAO_Empty(t *testing.T) {
	c := New("", "")
	_, err := c.GetAircraftByICAO(nil, "")
	if err == nil {
		t.Error("expected error for empty icao24")
	}
}

func TestGetFlightsByAircraft_Empty(t *testing.T) {
	c := New("", "")
	_, err := c.GetFlightsByAircraft(nil, "", 0, 0)
	if err == nil {
		t.Error("expected error for empty icao24")
	}
}

func TestParseStateVector_Full(t *testing.T) {
	lon := 13.4050
	lat := 52.5200
	alt := 10000.0
	vel := 250.0
	hdg := 180.0

	s := []interface{}{
		"3c675a",      // icao24
		"DLH123 ",     // callsign (with trailing space)
		"Germany",     // origin_country
		float64(1700000000), // time_position
		float64(1700000001), // last_contact
		lon,           // longitude
		lat,           // latitude
		alt,           // baro_altitude
		false,         // on_ground
		vel,           // velocity
		hdg,           // true_track
		nil,           // vertical_rate
		nil,           // sensors
		alt,           // geo_altitude
		"7700",        // squawk
		false,         // spi
		float64(0),    // position_source
	}

	a := parseStateVector(s)
	if a.ICAO24 != "3c675a" {
		t.Errorf("ICAO24 want 3c675a, got %s", a.ICAO24)
	}
	if a.Callsign != "DLH123" {
		t.Errorf("Callsign want DLH123, got %s", a.Callsign)
	}
	if a.OriginCountry != "Germany" {
		t.Errorf("OriginCountry want Germany, got %s", a.OriginCountry)
	}
	if a.OnGround {
		t.Error("OnGround should be false")
	}
	if a.Longitude == nil || *a.Longitude != lon {
		t.Errorf("Longitude want %f, got %v", lon, a.Longitude)
	}
	if a.Latitude == nil || *a.Latitude != lat {
		t.Errorf("Latitude want %f, got %v", lat, a.Latitude)
	}
	if a.Squawk != "7700" {
		t.Errorf("Squawk want 7700, got %s", a.Squawk)
	}
	if a.VertRate != nil {
		t.Error("VertRate should be nil for nil input")
	}
}

func TestParseStateVector_OnGround(t *testing.T) {
	s := []interface{}{
		"abc123", "TEST123 ", "France",
		nil, float64(1700000000),
		float64(2.35), float64(48.86),
		float64(0), true, // on_ground = true
		float64(0), float64(0), nil, nil, nil, "1200", false, float64(0),
	}
	a := parseStateVector(s)
	if !a.OnGround {
		t.Error("OnGround should be true")
	}
}

func TestParseStateVector_Empty(t *testing.T) {
	// Should not panic on empty slice
	a := parseStateVector([]interface{}{})
	if a.ICAO24 != "" {
		t.Errorf("empty ICAO24 expected, got %s", a.ICAO24)
	}
}

func TestParseStateVector_ShortSlice(t *testing.T) {
	// Partial data — should not panic
	s := []interface{}{"abc", "CALL"}
	a := parseStateVector(s)
	if a.ICAO24 != "abc" {
		t.Errorf("ICAO24 want abc, got %s", a.ICAO24)
	}
	if a.Latitude != nil {
		t.Error("Latitude should be nil for missing data")
	}
}

func TestParseStatesResponse(t *testing.T) {
	body := []byte(`{
		"time": 1700000000,
		"states": [
			["3c675a", "DLH123 ", "Germany", 1700000000, 1700000001, 13.405, 52.52, 10000.0, false, 250.0, 180.0, null, null, 10000.0, "7700", false, 0]
		]
	}`)

	c := &Client{http: nil}
	aircraft, ts, err := c.parseStateVectors(body)
	if err != nil {
		t.Fatalf("parseStateVectors error: %v", err)
	}
	if ts != 1700000000 {
		t.Errorf("timestamp want 1700000000, got %d", ts)
	}
	if len(aircraft) != 1 {
		t.Fatalf("want 1 aircraft, got %d", len(aircraft))
	}
	if aircraft[0].ICAO24 != "3c675a" {
		t.Errorf("ICAO24 want 3c675a, got %s", aircraft[0].ICAO24)
	}
}
