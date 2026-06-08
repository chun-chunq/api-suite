package zefix

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.http == nil {
		t.Fatal("http client is nil")
	}
}

func TestSearch_EmptyName(t *testing.T) {
	c := New()
	_, err := c.Search(context.Background(), "", "DE", true, 10)
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestSearch_ServerResponse(t *testing.T) {
	// Serve raw Zefix JSON format (what the actual API returns)
	rawJSON := `[{"uid":"CHE-116.281.710","name":"Nestlé S.A.","legalForm":{"nameDe":"Aktiengesellschaft"},"status":"ACTIVE","canton":"VD"}]`
	payload := []byte(rawJSON)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(payload)
	}))
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	results, err := c.Search(context.Background(), "Nestlé", "DE", true, 5)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Name != "Nestlé S.A." {
		t.Errorf("name want 'Nestlé S.A.', got '%s'", results[0].Name)
	}
}

func TestGetByUID_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	result, err := c.GetByUID(context.Background(), "CHE-000.000.000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for 404")
	}
}

func TestGetByUID_ValidResponse(t *testing.T) {
	rawJSON := `{"uid":"CHE-116.281.710","name":"Nestlé S.A.","legalForm":{"nameDe":"AG"},"status":"ACTIVE","canton":"VD"}`
	payload := []byte(rawJSON)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(payload)
	}))
	defer srv.Close()

	c := New()
	c.baseURL = srv.URL

	result, err := c.GetByUID(context.Background(), "CHE-116.281.710")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.UID != "CHE-116.281.710" {
		t.Errorf("UID want CHE-116.281.710, got %s", result.UID)
	}
}
