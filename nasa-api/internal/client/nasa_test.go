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
		apiKey:  "TEST_KEY",
	}
}

// ── APOD tests ────────────────────────────────────────────────────────────────

func TestGetAPOD_Today(t *testing.T) {
	entry := APODEntry{
		Date:        "2024-01-15",
		Title:       "The Orion Nebula",
		Explanation: "A beautiful nebula.",
		URL:         "https://apod.nasa.gov/apod/image/test.jpg",
		MediaType:   "image",
	}
	b, _ := json.Marshal(entry)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/planetary/apod" {
			http.Error(w, "wrong path", 404)
			return
		}
		// Verify api_key is passed
		if r.URL.Query().Get("api_key") == "" {
			http.Error(w, "missing api_key", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.GetAPOD(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Title != "The Orion Nebula" {
		t.Errorf("unexpected title: %q", result.Title)
	}
}

func TestGetAPOD_WithDate(t *testing.T) {
	entry := APODEntry{Date: "2023-06-15", Title: "Galaxy M87", MediaType: "image"}
	b, _ := json.Marshal(entry)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("date")
		if date != "2023-06-15" {
			http.Error(w, "wrong date", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.GetAPOD(context.Background(), "2023-06-15")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Date != "2023-06-15" {
		t.Errorf("unexpected date: %q", result.Date)
	}
}

func TestGetAPOD_InvalidDate(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused", apiKey: "KEY"}
	_, err := c.GetAPOD(context.Background(), "not-a-date")
	if err == nil {
		t.Fatal("expected error for invalid date format")
	}
}

func TestGetAPODRange_OK(t *testing.T) {
	entries := []APODEntry{
		{Date: "2024-01-01", Title: "Day 1", MediaType: "image"},
		{Date: "2024-01-02", Title: "Day 2", MediaType: "image"},
	}
	b, _ := json.Marshal(entries)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, err := c.GetAPODRange(context.Background(), "2024-01-01", "2024-01-02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 entries, got %d", len(results))
	}
}

func TestGetAPODRange_TooLarge(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused", apiKey: "KEY"}
	_, err := c.GetAPODRange(context.Background(), "2024-01-01", "2024-01-31")
	if err == nil {
		t.Fatal("expected error for range > 7 days")
	}
}

func TestGetAPODRange_EndBeforeStart(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused", apiKey: "KEY"}
	_, err := c.GetAPODRange(context.Background(), "2024-01-10", "2024-01-05")
	if err == nil {
		t.Fatal("expected error for end_date before start_date")
	}
}

// ── Mars Photos tests ─────────────────────────────────────────────────────────

func TestGetMarsPhotos_OK(t *testing.T) {
	raw := map[string]interface{}{
		"photos": []map[string]interface{}{
			{
				"id":         float64(1),
				"sol":        float64(1000),
				"earth_date": "2015-05-30",
				"img_src":    "https://mars.nasa.gov/photo1.jpg",
				"camera":     map[string]string{"name": "FHAZ", "full_name": "Front Hazard Avoidance Camera"},
				"rover":      map[string]string{"name": "Curiosity", "status": "active"},
			},
		},
	}
	b, _ := json.Marshal(raw)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.GetMarsPhotos(context.Background(), "curiosity", "", 1000, "", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Photos) != 1 {
		t.Errorf("expected 1 photo, got %d", len(result.Photos))
	}
	if result.Rover != "curiosity" {
		t.Errorf("unexpected rover: %q", result.Rover)
	}
}

func TestGetMarsPhotos_InvalidRover(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused", apiKey: "KEY"}
	_, err := c.GetMarsPhotos(context.Background(), "voyager", "", 100, "", 5)
	if err == nil {
		t.Fatal("expected error for invalid rover")
	}
}

func TestGetMarsPhotos_Limit(t *testing.T) {
	photos := make([]map[string]interface{}, 15)
	for i := range photos {
		photos[i] = map[string]interface{}{
			"id": float64(i), "sol": float64(1000), "earth_date": "2015-05-30",
			"img_src": "https://mars.nasa.gov/photo.jpg",
			"camera":  map[string]string{"name": "FHAZ", "full_name": "Front Hazard Avoidance"},
			"rover":   map[string]string{"name": "Curiosity", "status": "active"},
		}
	}
	raw := map[string]interface{}{"photos": photos}
	b, _ := json.Marshal(raw)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.GetMarsPhotos(context.Background(), "curiosity", "", 1000, "", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Photos) != 5 {
		t.Errorf("expected 5 photos (limit), got %d", len(result.Photos))
	}
	if result.Total != 15 {
		t.Errorf("expected total=15, got %d", result.Total)
	}
}

// ── NEO Feed tests ────────────────────────────────────────────────────────────

func TestGetNEOFeed_OK(t *testing.T) {
	raw := map[string]interface{}{
		"element_count": 3,
		"near_earth_objects": map[string]interface{}{
			"2024-01-15": []map[string]interface{}{
				{
					"id":   "12345",
					"name": "(2024 AB)",
					"links": map[string]string{"self": "https://api.nasa.gov/neo/1"},
					"absolute_magnitude_h": 22.1,
					"estimated_diameter": map[string]interface{}{
						"kilometers": map[string]float64{
							"estimated_diameter_min": 0.05,
							"estimated_diameter_max": 0.12,
						},
					},
					"is_potentially_hazardous_asteroid": false,
					"close_approach_data": []map[string]interface{}{
						{
							"close_approach_date": "2024-01-15",
							"relative_velocity":   map[string]string{"kilometers_per_hour": "45000"},
							"miss_distance":       map[string]string{"kilometers": "500000"},
						},
					},
				},
			},
		},
	}
	b, _ := json.Marshal(raw)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.GetNEOFeed(context.Background(), "2024-01-15", "2024-01-15")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TotalObjects != 3 {
		t.Errorf("expected 3 total, got %d", result.TotalObjects)
	}
	if len(result.NEOs) != 1 {
		t.Errorf("expected 1 NEO, got %d", len(result.NEOs))
	}
	if result.NEOs[0].MissDistanceKM != "500000" {
		t.Errorf("unexpected miss distance: %q", result.NEOs[0].MissDistanceKM)
	}
}

func TestGetNEOFeed_RangeTooLarge(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused", apiKey: "KEY"}
	_, err := c.GetNEOFeed(context.Background(), "2024-01-01", "2024-01-31")
	if err == nil {
		t.Fatal("expected error for range > 7 days")
	}
}

func TestNew_DefaultKey(t *testing.T) {
	c := New("")
	if c.apiKey != "DEMO_KEY" {
		t.Errorf("expected DEMO_KEY, got %q", c.apiKey)
	}
}
