package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(agify, genderize, nationalize string) *Client {
	return &Client{
		http:           http.DefaultClient,
		agifyURL:       agify,
		genderizeURL:   genderize,
		nationalizeURL: nationalize,
	}
}

// TestGetAge_OK tests successful age prediction
func TestGetAge_OK(t *testing.T) {
	age := 32
	body, _ := json.Marshal(map[string]interface{}{
		"name":  "Michael",
		"age":   age,
		"count": 12345,
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.URL, srv.URL)
	c.http = srv.Client()
	result, err := c.GetAge(context.Background(), "Michael", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Name != "Michael" {
		t.Errorf("expected name 'Michael', got %q", result.Name)
	}
	if result.Age == nil || *result.Age != 32 {
		t.Errorf("expected age 32, got %v", result.Age)
	}
	if result.Count != 12345 {
		t.Errorf("expected count 12345, got %d", result.Count)
	}
}

// TestGetAge_EmptyName tests that empty name returns error
func TestGetAge_EmptyName(t *testing.T) {
	c := New()
	_, err := c.GetAge(context.Background(), "", "")
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

// TestGetAge_NullAge tests when API returns null age (unknown)
func TestGetAge_NullAge(t *testing.T) {
	body, _ := json.Marshal(map[string]interface{}{
		"name":  "Zzxxx",
		"age":   nil,
		"count": 0,
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.URL, srv.URL)
	c.http = srv.Client()
	result, err := c.GetAge(context.Background(), "Zzxxx", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Age != nil {
		t.Errorf("expected nil age for unknown name, got %v", *result.Age)
	}
}

// TestGetGender_OK tests successful gender prediction
func TestGetGender_OK(t *testing.T) {
	body, _ := json.Marshal(map[string]interface{}{
		"name":        "Emma",
		"gender":      "female",
		"probability": 0.98,
		"count":       54321,
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.URL, srv.URL)
	c.http = srv.Client()
	result, err := c.GetGender(context.Background(), "Emma", "DE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Gender != "female" {
		t.Errorf("expected gender 'female', got %q", result.Gender)
	}
	if result.Probability < 0.9 {
		t.Errorf("expected probability >= 0.9, got %f", result.Probability)
	}
}

// TestGetGender_EmptyName tests that empty name returns error
func TestGetGender_EmptyName(t *testing.T) {
	c := New()
	_, err := c.GetGender(context.Background(), "   ", "")
	if err == nil {
		t.Fatal("expected error for whitespace-only name")
	}
}

// TestGetNationality_OK tests successful nationality prediction
func TestGetNationality_OK(t *testing.T) {
	body, _ := json.Marshal(map[string]interface{}{
		"name":  "Zhang",
		"count": 9876,
		"country": []map[string]interface{}{
			{"country_id": "CN", "probability": 0.85},
			{"country_id": "TW", "probability": 0.10},
		},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.URL, srv.URL)
	c.http = srv.Client()
	result, err := c.GetNationality(context.Background(), "Zhang")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Countries) != 2 {
		t.Errorf("expected 2 countries, got %d", len(result.Countries))
	}
	if result.Countries[0].CountryID != "CN" {
		t.Errorf("expected first country CN, got %q", result.Countries[0].CountryID)
	}
}

// TestGetAll_Concurrent tests that GetAll fetches all three predictions concurrently
func TestGetAll_Concurrent(t *testing.T) {
	ageBody, _ := json.Marshal(map[string]interface{}{"name": "Alex", "age": 28, "count": 1000})
	genderBody, _ := json.Marshal(map[string]interface{}{"name": "Alex", "gender": "male", "probability": 0.7, "count": 2000})
	natBody, _ := json.Marshal(map[string]interface{}{"name": "Alex", "count": 3000, "country": []map[string]interface{}{
		{"country_id": "US", "probability": 0.5},
	}})

	ageSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(ageBody)
	}))
	genderSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(genderBody)
	}))
	natSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(natBody)
	}))
	defer ageSrv.Close()
	defer genderSrv.Close()
	defer natSrv.Close()

	c := &Client{
		http:           ageSrv.Client(),
		agifyURL:       ageSrv.URL,
		genderizeURL:   genderSrv.URL,
		nationalizeURL: natSrv.URL,
	}

	result, err := c.GetAll(context.Background(), "Alex", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Age == nil {
		t.Error("expected age result")
	}
	if result.Gender == nil {
		t.Error("expected gender result")
	}
	if result.Nationality == nil {
		t.Error("expected nationality result")
	}
	if result.Name != "Alex" {
		t.Errorf("expected name 'Alex', got %q", result.Name)
	}
}

// TestRateLimit tests 429 handling
func TestRateLimit_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newTestClient(srv.URL, srv.URL, srv.URL)
	c.http = srv.Client()
	_, err := c.GetAge(context.Background(), "Test", "")
	if err == nil {
		t.Fatal("expected error on rate limit")
	}
	if err.Error()[:10] != "rate_limit" {
		t.Errorf("expected rate_limit error, got: %v", err)
	}
}
