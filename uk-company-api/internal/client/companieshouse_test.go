package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew(t *testing.T) {
	c := New("test-api-key")
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.apiKey != "test-api-key" {
		t.Errorf("apiKey want test-api-key, got %s", c.apiKey)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	c := New("key")
	_, err := c.Search(context.Background(), "", 10, 0)
	if err == nil {
		t.Error("expected error for empty query")
	}
}

func TestGetByNumber_Empty(t *testing.T) {
	c := New("key")
	_, err := c.GetByNumber(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty company number")
	}
}

func TestGetByNumber_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := &Client{http: &http.Client{}, baseURL: srv.URL, apiKey: "test"}
	result, err := c.GetByNumber(context.Background(), "99999999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil for 404")
	}
}

func TestMapSearchItem_Basic(t *testing.T) {
	item := chSearchItem{
		CompanyNumber:  "00102498",
		Title:          "BARCLAYS BANK PLC",
		CompanyStatus:  "active",
		CompanyType:    "plc",
		DateOfCreation: "1896-01-10",
	}
	co := mapSearchItem(item)
	if co.CompanyNumber != "00102498" {
		t.Errorf("CompanyNumber want 00102498, got %s", co.CompanyNumber)
	}
	if co.Name != "BARCLAYS BANK PLC" {
		t.Errorf("Name want BARCLAYS BANK PLC, got %s", co.Name)
	}
	if co.Status != "active" {
		t.Errorf("Status want active, got %s", co.Status)
	}
	if co.URL == "" {
		t.Error("URL should not be empty")
	}
}

func TestMapCompanyProfile_WithAddress(t *testing.T) {
	profile := chCompanyProfile{
		CompanyNumber: "00102498",
		CompanyName:   "BARCLAYS BANK PLC",
		CompanyStatus: "active",
		Type:          "plc",
		Jurisdiction:  "england-wales",
		SICCodes:      []string{"6420"},
		RegisteredOfficeAddress: &chAddress{
			AddressLine1: "1 Churchill Place",
			Locality:     "London",
			PostalCode:   "E14 5HP",
			Country:      "England",
		},
	}
	co := mapCompanyProfile(profile)
	if co.Jurisdiction != "england-wales" {
		t.Errorf("Jurisdiction want england-wales, got %s", co.Jurisdiction)
	}
	if len(co.SICCodes) != 1 || co.SICCodes[0] != "6420" {
		t.Errorf("SICCodes want [6420], got %v", co.SICCodes)
	}
	if co.RegisteredAddress == nil {
		t.Fatal("RegisteredAddress should not be nil")
	}
	if co.RegisteredAddress.PostalCode != "E14 5HP" {
		t.Errorf("PostalCode want E14 5HP, got %s", co.RegisteredAddress.PostalCode)
	}
}

func TestMapOfficer_Basic(t *testing.T) {
	o := chOfficer{
		Name:        "SMITH, John",
		OfficerRole: "director",
		AppointedOn: "2010-01-15",
		Nationality: "British",
	}
	officer := mapOfficer(o)
	if officer.Name != "SMITH, John" {
		t.Errorf("Name want SMITH, John, got %s", officer.Name)
	}
	if officer.Role != "director" {
		t.Errorf("Role want director, got %s", officer.Role)
	}
	if officer.DateOfBirth != nil {
		t.Error("DateOfBirth should be nil when not set")
	}
}

func TestMapOfficer_WithDOB(t *testing.T) {
	dob := struct {
		Month int `json:"month"`
		Year  int `json:"year"`
	}{Month: 6, Year: 1980}

	o := chOfficer{
		Name:        "DOE, Jane",
		OfficerRole: "secretary",
		DateOfBirth: &dob,
	}
	officer := mapOfficer(o)
	if officer.DateOfBirth == nil {
		t.Fatal("DateOfBirth should not be nil")
	}
	if officer.DateOfBirth.Month != 6 {
		t.Errorf("DOB.Month want 6, got %d", officer.DateOfBirth.Month)
	}
}

func TestSearch_ServerResponse(t *testing.T) {
	raw := chSearchResponse{
		TotalResults: 1,
		StartIndex:   0,
		ItemsPerPage: 20,
		Items: []chSearchItem{
			{
				CompanyNumber: "00102498",
				Title:         "BARCLAYS BANK PLC",
				CompanyStatus: "active",
				CompanyType:   "plc",
			},
		},
	}
	body, _ := json.Marshal(raw)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// verify Basic Auth header is set
		user, _, ok := r.BasicAuth()
		if !ok || user == "" {
			t.Error("expected Basic Auth header")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := &Client{http: &http.Client{}, baseURL: srv.URL, apiKey: "test-key"}
	result, err := c.Search(context.Background(), "Barclays", 10, 0)
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("Total want 1, got %d", result.Total)
	}
	if result.Results[0].CompanyNumber != "00102498" {
		t.Errorf("CompanyNumber want 00102498, got %s", result.Results[0].CompanyNumber)
	}
}
