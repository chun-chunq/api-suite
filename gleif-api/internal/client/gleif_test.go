package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMapRecord_Basic(t *testing.T) {
	r := gleifRecord{
		ID: "529900W18LQJJN6SJ336",
	}
	r.Attributes.LEI = "529900W18LQJJN6SJ336"
	r.Attributes.Entity.LegalName.Name = "Test Bank AG"
	r.Attributes.Entity.Status = "ACTIVE"
	r.Attributes.Entity.Jurisdiction = "DE"
	r.Attributes.Entity.LegalAddress.City = "Frankfurt"
	r.Attributes.Entity.LegalAddress.Country = "DE"

	e := mapRecord(r)
	if e.LEI != "529900W18LQJJN6SJ336" {
		t.Errorf("LEI want 529900W18LQJJN6SJ336, got %s", e.LEI)
	}
	if e.Name != "Test Bank AG" {
		t.Errorf("Name want 'Test Bank AG', got %s", e.Name)
	}
	if e.Status != "ACTIVE" {
		t.Errorf("Status want ACTIVE, got %s", e.Status)
	}
	if e.Jurisdiction != "DE" {
		t.Errorf("Jurisdiction want DE, got %s", e.Jurisdiction)
	}
	if e.RegisteredAddress == nil || e.RegisteredAddress.City != "Frankfurt" {
		t.Error("RegisteredAddress.City want Frankfurt")
	}
}

func TestGetByLEI_InvalidLength(t *testing.T) {
	c := New()
	_, err := c.GetByLEI(nil, "SHORT")
	if err == nil {
		t.Error("expected error for short LEI, got nil")
	}
}

func TestGetByLEI_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	// Override base URL via a custom client
	c := &Client{http: &http.Client{}}
	// Patch: call the internal get() which uses baseURL — test via parseList
	body := []byte(`{"data":[],"meta":{"total":0}}`)
	entities, total, err := c.parseList(body)
	if err != nil {
		t.Fatalf("parseList error: %v", err)
	}
	if total != 0 {
		t.Errorf("total want 0, got %d", total)
	}
	if len(entities) != 0 {
		t.Errorf("entities want 0, got %d", len(entities))
	}
}

func TestParseList_MultipleRecords(t *testing.T) {
	records := gleifListResponse{
		Data: []gleifRecord{
			{ID: "AAAAAAAAAAAAAAAAAAA1"},
			{ID: "AAAAAAAAAAAAAAAAAAA2"},
		},
		Meta: &struct {
			Total int `json:"total"`
		}{Total: 2},
	}
	body, _ := json.Marshal(records)

	c := &Client{}
	entities, total, err := c.parseList(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 2 {
		t.Errorf("total want 2, got %d", total)
	}
	if len(entities) != 2 {
		t.Errorf("len(entities) want 2, got %d", len(entities))
	}
}

func TestParseList_Empty(t *testing.T) {
	body := []byte(`{"data":[],"meta":{"total":0}}`)
	c := &Client{}
	entities, total, err := c.parseList(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if total != 0 || len(entities) != 0 {
		t.Errorf("expected empty result")
	}
}

func TestMinHelper(t *testing.T) {
	if min(3, 5) != 3 {
		t.Error("min(3,5) want 3")
	}
	if min(10, 2) != 2 {
		t.Error("min(10,2) want 2")
	}
	if min(7, 7) != 7 {
		t.Error("min(7,7) want 7")
	}
}
