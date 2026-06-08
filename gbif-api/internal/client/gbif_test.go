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

// ── SearchSpecies tests ───────────────────────────────────────────────────────

func TestSearchSpecies_OK(t *testing.T) {
	raw := map[string]interface{}{
		"count": float64(5),
		"offset": float64(0),
		"limit": float64(20),
		"results": []map[string]interface{}{
			{
				"key": float64(5219404),
				"scientificName": "Panthera leo (Linnaeus, 1758)",
				"canonicalName": "Panthera leo",
				"kingdom": "Animalia",
				"class": "Mammalia",
				"order": "Carnivora",
				"family": "Felidae",
				"genus": "Panthera",
				"rank": "SPECIES",
				"taxonomicStatus": "ACCEPTED",
				"extinct": false,
				"numDescendants": float64(12),
			},
		},
	}
	b, _ := json.Marshal(raw)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/species/search" {
			http.Error(w, "wrong path: "+r.URL.Path, 404)
			return
		}
		if r.URL.Query().Get("q") != "lion" {
			http.Error(w, "wrong query param", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.SearchSpecies(context.Background(), "lion", "", 20, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 5 {
		t.Errorf("expected total=5, got %d", result.Total)
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if result.Results[0].Kingdom != "Animalia" {
		t.Errorf("unexpected kingdom: %q", result.Results[0].Kingdom)
	}
	if result.Results[0].CanonicalName != "Panthera leo" {
		t.Errorf("unexpected canonical name: %q", result.Results[0].CanonicalName)
	}
	if result.Results[0].NumDescendants != 12 {
		t.Errorf("unexpected num_descendants: %d", result.Results[0].NumDescendants)
	}
}

func TestSearchSpecies_EmptyQuery(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused"}
	_, err := c.SearchSpecies(context.Background(), "", "", 20, 0)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
}

func TestSearchSpecies_HasMore(t *testing.T) {
	raw := map[string]interface{}{
		"count": float64(100), "offset": float64(0), "limit": float64(20),
		"results": []map[string]interface{}{},
	}
	b, _ := json.Marshal(raw)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.SearchSpecies(context.Background(), "bird", "", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !result.HasMore {
		t.Error("expected HasMore=true when count(100) > offset(0)+limit(20)")
	}
}

// ── GetSpecies tests ──────────────────────────────────────────────────────────

func TestGetSpecies_OK(t *testing.T) {
	raw := map[string]interface{}{
		"key": float64(5219404),
		"scientificName": "Panthera leo",
		"canonicalName": "Panthera leo",
		"kingdom": "Animalia",
		"rank": "SPECIES",
		"taxonomicStatus": "ACCEPTED",
		"extinct": false,
	}
	b, _ := json.Marshal(raw)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/species/5219404" {
			http.Error(w, "wrong path: "+r.URL.Path, 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	species, err := c.GetSpecies(context.Background(), 5219404)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if species.Key != 5219404 {
		t.Errorf("unexpected key: %d", species.Key)
	}
	if species.Kingdom != "Animalia" {
		t.Errorf("unexpected kingdom: %q", species.Kingdom)
	}
}

func TestGetSpecies_InvalidKey(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused"}
	_, err := c.GetSpecies(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error for key=0")
	}
}

func TestGetSpecies_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.GetSpecies(context.Background(), 99999999)
	if err == nil {
		t.Fatal("expected not_found error")
	}
}

// ── GetVernacularNames tests ──────────────────────────────────────────────────

func TestGetVernacularNames_OK(t *testing.T) {
	raw := map[string]interface{}{
		"results": []map[string]interface{}{
			{"vernacularName": "Lion", "language": "eng"},
			{"vernacularName": "African Lion", "language": "eng"},
			{"vernacularName": "Lion d'Afrique", "language": "fra"},
		},
	}
	b, _ := json.Marshal(raw)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	names, err := c.GetVernacularNames(context.Background(), 5219404, "eng")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(names) != 2 { // only English names
		t.Errorf("expected 2 English names, got %d: %v", len(names), names)
	}
}

func TestGetVernacularNames_InvalidKey(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused"}
	_, err := c.GetVernacularNames(context.Background(), -1, "")
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

// ── SearchOccurrences tests ───────────────────────────────────────────────────

func TestSearchOccurrences_OK(t *testing.T) {
	lat := 48.8566
	lon := 2.3522
	raw := map[string]interface{}{
		"count": float64(150),
		"offset": float64(0),
		"limit": float64(20),
		"results": []map[string]interface{}{
			{
				"key": float64(1234567),
				"scientificName": "Panthera leo",
				"kingdom": "Animalia",
				"countryCode": "KE",
				"country": "Kenya",
				"decimalLatitude": lat,
				"decimalLongitude": lon,
				"year": float64(2022),
				"basisOfRecord": "HUMAN_OBSERVATION",
			},
		},
	}
	b, _ := json.Marshal(raw)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/occurrence/search" {
			http.Error(w, "wrong path", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.SearchOccurrences(context.Background(), 5219404, "KE", 2022, 20, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 150 {
		t.Errorf("expected total=150, got %d", result.Total)
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result.Results))
	}
	if result.Results[0].CountryCode != "KE" {
		t.Errorf("unexpected country: %q", result.Results[0].CountryCode)
	}
	if result.Results[0].Latitude == nil || *result.Results[0].Latitude != lat {
		t.Errorf("unexpected latitude")
	}
}

func TestSearchOccurrences_DefaultLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		limit := r.URL.Query().Get("limit")
		if limit != "20" {
			http.Error(w, "expected default limit=20, got "+limit, 400)
			return
		}
		raw := map[string]interface{}{"count": float64(0), "offset": float64(0), "limit": float64(20), "results": []interface{}{}}
		b, _ := json.Marshal(raw)
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.SearchOccurrences(context.Background(), 0, "", 0, 0, 0) // 0 → default 20
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
