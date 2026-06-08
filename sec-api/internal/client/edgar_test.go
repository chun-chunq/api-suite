package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	c := New("Test user@test.com")
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.userAgent != "Test user@test.com" {
		t.Errorf("userAgent mismatch: %s", c.userAgent)
	}
}

func TestNew_DefaultUserAgent(t *testing.T) {
	c := New("")
	if c.userAgent == "" {
		t.Error("default userAgent should not be empty")
	}
}

func TestNormalizeCIK(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"789019", "0000789019"},
		{"0000789019", "0000789019"},
		{"CIK0000789019", "0000789019"},
		{"1", "0000000001"},
		{"10000000000", "0000000000"}, // truncate if longer
	}
	for _, tc := range cases {
		got := normalizeCIK(tc.input)
		if got != tc.want {
			t.Errorf("normalizeCIK(%q) want %s, got %s", tc.input, tc.want, got)
		}
	}
}

func TestSearchCompanies_EmptyQuery(t *testing.T) {
	c := New("test@test.com")
	_, _, err := c.SearchCompanies(context.Background(), "", 10)
	if err == nil {
		t.Error("expected error for empty query")
	}
}

func TestGetCompanyProfile_EmptyCIK(t *testing.T) {
	c := New("test@test.com")
	_, err := c.GetCompanyProfile(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty CIK")
	}
}

func TestGetFilings_EmptyCIK(t *testing.T) {
	c := New("test@test.com")
	_, err := c.GetFilings(context.Background(), "", "10-K", 10)
	if err == nil {
		t.Error("expected error for empty CIK")
	}
}

func TestGetFinancialFacts_InvalidConcept(t *testing.T) {
	c := New("test@test.com")
	_, err := c.GetFinancialFacts(context.Background(), "0000789019", "NetIncomeLoss", 10) // missing taxonomy/
	if err == nil {
		t.Error("expected error for concept without taxonomy prefix")
	}
}

func TestGetCompanyProfile_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := &Client{http: &http.Client{}, baseURL: srv.URL, searchURL: srv.URL, userAgent: "test"}
	result, err := c.GetCompanyProfile(context.Background(), "9999999999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil for 404")
	}
}

func TestGetCompanyProfile_ValidResponse(t *testing.T) {
	raw := edgarSubmissions{
		CIK:         "789019",
		Name:        "MICROSOFT CORP",
		SIC:         "7372",
		SICDesc:     "Prepackaged Software",
		StateOfInc:  "WA",
		FiscalYearEnd: "0630",
		Tickers:     []string{"MSFT"},
		Exchanges:   []string{"Nasdaq"},
	}
	body, _ := json.Marshal(raw)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := &Client{http: &http.Client{}, baseURL: srv.URL, searchURL: srv.URL, userAgent: "test"}
	co, err := c.GetCompanyProfile(context.Background(), "789019")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if co == nil {
		t.Fatal("expected non-nil company")
	}
	if co.Name != "MICROSOFT CORP" {
		t.Errorf("Name want MICROSOFT CORP, got %s", co.Name)
	}
	if co.Ticker != "MSFT" {
		t.Errorf("Ticker want MSFT, got %s", co.Ticker)
	}
	if co.SIC != "7372" {
		t.Errorf("SIC want 7372, got %s", co.SIC)
	}
}

func TestMapFilings_FormFilter(t *testing.T) {
	r := edgarSubmissions{}
	r.Filings.Recent.AccessionNumber = []string{"0001193125-23-001", "0001193125-23-002", "0001193125-23-003"}
	r.Filings.Recent.Form = []string{"10-K", "10-Q", "10-K"}
	r.Filings.Recent.FilingDate = []string{"2023-10-01", "2023-07-01", "2022-10-01"}
	r.Filings.Recent.Size = []int64{1000, 500, 900}

	filings := mapFilings(r, "0000789019", "10-K", 10)
	if len(filings) != 2 {
		t.Errorf("want 2 10-K filings, got %d", len(filings))
	}
	for _, f := range filings {
		if f.Form != "10-K" {
			t.Errorf("expected only 10-K, got %s", f.Form)
		}
	}
}

func TestMapFilings_NoFilter(t *testing.T) {
	r := edgarSubmissions{}
	r.Filings.Recent.AccessionNumber = []string{"A", "B", "C"}
	r.Filings.Recent.Form = []string{"10-K", "10-Q", "8-K"}
	r.Filings.Recent.FilingDate = []string{"2023-10-01", "2023-07-01", "2023-05-01"}

	filings := mapFilings(r, "0000789019", "", 10)
	if len(filings) != 3 {
		t.Errorf("want 3 filings (no filter), got %d", len(filings))
	}
}

func TestMapCompanyProfile_Basic(t *testing.T) {
	r := edgarSubmissions{
		CIK:       "789019",
		Name:      "MICROSOFT CORP",
		Tickers:   []string{"MSFT"},
		Exchanges: []string{"Nasdaq"},
		SIC:       "7372",
	}
	co := mapCompanyProfile(r)
	if co.CIK != "0000789019" {
		t.Errorf("CIK should be padded, got %s", co.CIK)
	}
	if !strings.Contains(co.URL, "0000789019") {
		t.Errorf("URL should contain CIK, got %s", co.URL)
	}
}
