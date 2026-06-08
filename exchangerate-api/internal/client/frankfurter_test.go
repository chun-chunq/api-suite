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

// ── GetLatest tests ───────────────────────────────────────────────────────────

func TestGetLatest_OK(t *testing.T) {
	resp := LatestRates{
		Amount: 1.0,
		Base:   "EUR",
		Date:   "2024-01-15",
		Rates:  map[string]float64{"USD": 1.0876, "GBP": 0.8612},
	}
	b, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/latest" {
			http.Error(w, "wrong path", 404)
			return
		}
		if r.URL.Query().Get("base") != "EUR" {
			http.Error(w, "wrong base", 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.GetLatest(context.Background(), "EUR", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Base != "EUR" {
		t.Errorf("expected EUR, got %q", result.Base)
	}
	if result.Rates["USD"] != 1.0876 {
		t.Errorf("unexpected USD rate: %f", result.Rates["USD"])
	}
}

func TestGetLatest_DefaultBase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("base") != "EUR" {
			http.Error(w, "expected EUR as default base", 400)
			return
		}
		resp := LatestRates{Amount: 1.0, Base: "EUR", Date: "2024-01-15", Rates: map[string]float64{"USD": 1.09}}
		b, _ := json.Marshal(resp)
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.GetLatest(context.Background(), "", nil) // empty → default EUR
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── GetHistorical tests ───────────────────────────────────────────────────────

func TestGetHistorical_OK(t *testing.T) {
	resp := HistoricalRates{
		Amount: 1.0, Base: "USD", Date: "2020-01-15",
		Rates: map[string]float64{"EUR": 0.9012, "GBP": 0.7654},
	}
	b, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/2020-01-15" {
			http.Error(w, "wrong path: "+r.URL.Path, 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.GetHistorical(context.Background(), "2020-01-15", "USD", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Base != "USD" {
		t.Errorf("unexpected base: %q", result.Base)
	}
}

func TestGetHistorical_InvalidDate(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused"}
	_, err := c.GetHistorical(context.Background(), "not-a-date", "EUR", nil)
	if err == nil {
		t.Fatal("expected error for invalid date")
	}
}

// ── GetTimeSeries tests ───────────────────────────────────────────────────────

func TestGetTimeSeries_OK(t *testing.T) {
	raw := struct {
		Amount    float64                       `json:"amount"`
		Base      string                        `json:"base"`
		StartDate string                        `json:"start_date"`
		EndDate   string                        `json:"end_date"`
		Rates     map[string]map[string]float64 `json:"rates"`
	}{
		Amount: 1.0, Base: "EUR",
		StartDate: "2024-01-01", EndDate: "2024-01-03",
		Rates: map[string]map[string]float64{
			"2024-01-01": {"USD": 1.10},
			"2024-01-02": {"USD": 1.11},
			"2024-01-03": {"USD": 1.09},
		},
	}
	b, _ := json.Marshal(raw)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.GetTimeSeries(context.Background(), "2024-01-01", "2024-01-03", "EUR", []string{"USD"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.DataPoints != 3 {
		t.Errorf("expected 3 data points, got %d", result.DataPoints)
	}
	// Dates should be sorted
	if result.Dates[0] != "2024-01-01" {
		t.Errorf("first date should be 2024-01-01, got %q", result.Dates[0])
	}
}

func TestGetTimeSeries_EndBeforeStart(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused"}
	_, err := c.GetTimeSeries(context.Background(), "2024-01-10", "2024-01-01", "EUR", nil)
	if err == nil {
		t.Fatal("expected error for end_date before start_date")
	}
}

func TestGetTimeSeries_TooLarge(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused"}
	_, err := c.GetTimeSeries(context.Background(), "2020-01-01", "2022-01-01", "EUR", nil)
	if err == nil {
		t.Fatal("expected error for range > 365 days")
	}
}

// ── Convert tests ─────────────────────────────────────────────────────────────

func TestConvert_OK(t *testing.T) {
	resp := LatestRates{
		Amount: 100.0, Base: "USD", Date: "2024-01-15",
		Rates: map[string]float64{"EUR": 91.89},
	}
	b, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.Convert(context.Background(), "USD", "EUR", 100.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.From != "USD" || result.To != "EUR" {
		t.Errorf("unexpected from/to: %q/%q", result.From, result.To)
	}
	if result.Rate != 91.89 {
		t.Errorf("unexpected rate: %f", result.Rate)
	}
}

func TestConvert_SameCurrency(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused"}
	result, err := c.Convert(context.Background(), "EUR", "EUR", 42.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Result != 42.0 {
		t.Errorf("same-currency conversion should return same amount, got %f", result.Result)
	}
	if result.Rate != 1.0 {
		t.Errorf("same-currency rate should be 1.0, got %f", result.Rate)
	}
}

func TestConvert_ZeroAmount(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused"}
	_, err := c.Convert(context.Background(), "USD", "EUR", 0)
	if err == nil {
		t.Fatal("expected error for amount=0")
	}
}

func TestConvert_EmptyCurrency(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused"}
	_, err := c.Convert(context.Background(), "", "EUR", 100)
	if err == nil {
		t.Fatal("expected error for empty from currency")
	}
}

// ── GetCurrencies tests ───────────────────────────────────────────────────────

func TestGetCurrencies_OK(t *testing.T) {
	raw := map[string]string{
		"EUR": "Euro",
		"USD": "United States Dollar",
		"GBP": "British Pound",
	}
	b, _ := json.Marshal(raw)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/currencies" {
			http.Error(w, "wrong path", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	currencies, err := c.GetCurrencies(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(currencies) != 3 {
		t.Errorf("expected 3 currencies, got %d", len(currencies))
	}
	// Should be sorted by code
	if currencies[0].Code != "EUR" {
		t.Errorf("expected EUR first, got %q", currencies[0].Code)
	}
}

func TestValidateDate(t *testing.T) {
	if err := validateDate("2024-01-15"); err != nil {
		t.Errorf("valid date should not error: %v", err)
	}
	if err := validateDate("not-a-date"); err == nil {
		t.Error("invalid date should error")
	}
	if err := validateDate("2024/01/15"); err == nil {
		t.Error("wrong format should error")
	}
}

func TestNormalizeCurrency(t *testing.T) {
	if normalizeCurrency("eur") != "EUR" {
		t.Error("should uppercase")
	}
	if normalizeCurrency("  USD  ") != "USD" {
		t.Error("should trim")
	}
}
