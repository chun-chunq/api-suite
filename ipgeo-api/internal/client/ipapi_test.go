package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestClient(srv *httptest.Server) *Client {
	c := New("")
	c.http = srv.Client()
	c.baseURL = srv.URL
	return c
}

// ── Lookup ────────────────────────────────────────────────────────────────────

func geoResponse() map[string]interface{} {
	return map[string]interface{}{
		"status":      "success",
		"country":     "United States",
		"countryCode": "US",
		"region":      "CA",
		"regionName":  "California",
		"city":        "San Francisco",
		"zip":         "94102",
		"lat":         float64(37.7749),
		"lon":         float64(-122.4194),
		"timezone":    "America/Los_Angeles",
		"isp":         "Cloudflare, Inc.",
		"org":         "Cloudflare",
		"as":          "AS13335 Cloudflare, Inc.",
		"asname":      "CLOUDFLARENET",
		"mobile":      false,
		"proxy":       false,
		"hosting":     true,
		"query":       "1.1.1.1",
	}
}

func TestLookup_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/json/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(geoResponse())
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.Lookup(context.Background(), "1.1.1.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Country != "United States" {
		t.Errorf("country: %s", result.Country)
	}
	if result.City != "San Francisco" {
		t.Errorf("city: %s", result.City)
	}
	if !result.Hosting {
		t.Error("expected hosting=true")
	}
	if result.IP != "1.1.1.1" {
		t.Errorf("IP: %s (expected from query field)", result.IP)
	}
}

func TestLookup_Self(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// "self" lookup hits /json/ with no IP
		if r.URL.Path != "/json/" {
			t.Errorf("expected /json/, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(geoResponse())
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Lookup(context.Background(), "self")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLookup_InvalidIP(t *testing.T) {
	c := New("")
	_, err := c.Lookup(context.Background(), "not a valid ip")
	if err == nil {
		t.Error("expected error for invalid IP with spaces")
	}
}

func TestLookup_FailStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "fail",
			"message": "private range",
			"query":   "192.168.1.1",
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Lookup(context.Background(), "192.168.1.1")
	if err == nil {
		t.Error("expected error for fail status")
	}
	if !strings.Contains(err.Error(), "private range") {
		t.Errorf("expected 'private range' in error, got: %v", err)
	}
}

func TestLookup_RateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Lookup(context.Background(), "8.8.8.8")
	if err == nil {
		t.Error("expected error for rate limit")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("expected rate limited error, got: %v", err)
	}
}

// ── LookupBatch ───────────────────────────────────────────────────────────────

func TestLookupBatch_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]interface{}{
			map[string]interface{}{
				"status":      "success",
				"country":     "United States",
				"countryCode": "US",
				"city":        "San Francisco",
				"query":       "1.1.1.1",
				"proxy":       false,
				"mobile":      false,
				"hosting":     true,
			},
			map[string]interface{}{
				"status":      "success",
				"country":     "United States",
				"countryCode": "US",
				"city":        "Mountain View",
				"query":       "8.8.8.8",
				"proxy":       false,
				"mobile":      false,
				"hosting":     true,
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	// Override batch URL
	results, err := c.LookupBatch(context.Background(), []string{"1.1.1.1", "8.8.8.8"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].IP != "1.1.1.1" {
		t.Errorf("result[0].IP: %s", results[0].IP)
	}
	if results[1].City != "Mountain View" {
		t.Errorf("result[1].City: %s", results[1].City)
	}
}

func TestLookupBatch_EmptyIPs(t *testing.T) {
	c := New("")
	_, err := c.LookupBatch(context.Background(), []string{})
	if err == nil {
		t.Error("expected error for empty IP list")
	}
}
