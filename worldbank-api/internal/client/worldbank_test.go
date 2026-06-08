package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestClient(srv *httptest.Server) *Client {
	return &Client{
		http:    srv.Client(),
		baseURL: srv.URL,
		cache:   make(map[string]cacheEntry),
	}
}

// Build a typical World Bank API response envelope: [meta, data]
func wbResponse(data interface{}) []byte {
	meta := map[string]interface{}{
		"page":     1,
		"pages":    1,
		"per_page": 50,
		"total":    1,
	}
	raw, _ := json.Marshal([]interface{}{meta, data})
	return raw
}

// ── GetCountry tests ──────────────────────────────────────────────────────────

func TestGetCountry_OK(t *testing.T) {
	countries := []map[string]interface{}{
		{
			"id":       "DEU",
			"iso2Code": "DE",
			"name":     "Germany",
			"region":   map[string]string{"value": "Europe & Central Asia"},
			"incomeLevel": map[string]string{"value": "High income"},
			"lendingType": map[string]string{"value": "Not classified"},
			"capitalCity": "Berlin",
			"longitude":   "10.4515",
			"latitude":    "51.1657",
		},
	}
	body := wbResponse(countries)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	country, err := c.GetCountry(context.Background(), "DE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if country.ISO2Code != "DE" {
		t.Errorf("expected ISO2Code 'DE', got %q", country.ISO2Code)
	}
	if country.Name != "Germany" {
		t.Errorf("expected name 'Germany', got %q", country.Name)
	}
	if country.Capital != "Berlin" {
		t.Errorf("expected capital 'Berlin', got %q", country.Capital)
	}
}

func TestGetCountry_EmptyCode(t *testing.T) {
	c := New()
	_, err := c.GetCountry(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty country code")
	}
}

func TestGetCountry_NotFound(t *testing.T) {
	// API returns empty array
	body := wbResponse([]interface{}{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.GetCountry(context.Background(), "XX")
	if err == nil {
		t.Fatal("expected not_found error")
	}
}

// ── GetIndicator tests ────────────────────────────────────────────────────────

func makeIndicatorData(countryCode, countryName, indicatorID, indicatorName string, years []struct{ Year string; Value *float64 }) []map[string]interface{} {
	result := make([]map[string]interface{}, len(years))
	for i, y := range years {
		result[i] = map[string]interface{}{
			"date":  y.Year,
			"value": y.Value,
			"country": map[string]string{
				"id": countryCode, "value": countryName,
			},
			"indicator": map[string]string{
				"id": indicatorID, "value": indicatorName,
			},
		}
	}
	return result
}

func TestGetIndicator_OK(t *testing.T) {
	v1, v2 := 3.9e12, 4.1e12
	data := makeIndicatorData("DE", "Germany", "NY.GDP.MKTP.CD", "GDP (current US$)",
		[]struct{ Year string; Value *float64 }{
			{"2022", &v1},
			{"2021", &v2},
		},
	)
	body := wbResponse(data)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.GetIndicator(context.Background(), "de", "NY.GDP.MKTP.CD", 0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CountryName != "Germany" {
		t.Errorf("expected country Germany, got %q", result.CountryName)
	}
	if result.IndicatorID != "NY.GDP.MKTP.CD" {
		t.Errorf("expected indicator ID, got %q", result.IndicatorID)
	}
	if len(result.Data) != 2 {
		t.Errorf("expected 2 data points, got %d", len(result.Data))
	}
	if result.Data[0].Value == nil {
		t.Error("expected non-nil value for first data point")
	}
}

func TestGetIndicator_Caching(t *testing.T) {
	calls := 0
	v := 1.0
	data := makeIndicatorData("US", "United States", "SP.POP.TOTL", "Population", []struct{ Year string; Value *float64 }{
		{"2022", &v},
	})
	body := wbResponse(data)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	c.GetIndicator(context.Background(), "us", "SP.POP.TOTL", 0, 0)
	c.GetIndicator(context.Background(), "us", "SP.POP.TOTL", 0, 0)
	c.GetIndicator(context.Background(), "us", "SP.POP.TOTL", 0, 0)

	if calls != 1 {
		t.Errorf("expected 1 upstream call (caching), got %d", calls)
	}
}

func TestGetIndicator_CacheExpiry(t *testing.T) {
	calls := 0
	v := 1.0
	data := makeIndicatorData("US", "United States", "SP.POP.TOTL", "Population", []struct{ Year string; Value *float64 }{
		{"2022", &v},
	})
	body := wbResponse(data)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	c.GetIndicator(context.Background(), "us", "SP.POP.TOTL", 0, 0)

	// Expire the cache
	c.mu.Lock()
	for k, v := range c.cache {
		v.expiresAt = time.Now().Add(-time.Hour)
		c.cache[k] = v
	}
	c.mu.Unlock()

	c.GetIndicator(context.Background(), "us", "SP.POP.TOTL", 0, 0)

	if calls != 2 {
		t.Errorf("expected 2 upstream calls after cache expiry, got %d", calls)
	}
}

func TestGetIndicator_EmptyCountry(t *testing.T) {
	c := New()
	_, err := c.GetIndicator(context.Background(), "", "NY.GDP.MKTP.CD", 0, 0)
	if err == nil {
		t.Fatal("expected error for empty country code")
	}
}

func TestGetIndicator_EmptyIndicator(t *testing.T) {
	c := New()
	_, err := c.GetIndicator(context.Background(), "DE", "", 0, 0)
	if err == nil {
		t.Fatal("expected error for empty indicator")
	}
}

// ── CommonIndicators tests ────────────────────────────────────────────────────

func TestCommonIndicators(t *testing.T) {
	c := New()
	indicators := c.CommonIndicators()
	if len(indicators) == 0 {
		t.Fatal("expected non-empty indicators list")
	}
	for _, ind := range indicators {
		if ind.ID == "" {
			t.Error("indicator ID should not be empty")
		}
		if ind.Name == "" {
			t.Error("indicator name should not be empty")
		}
	}
	// Check GDP is in there
	foundGDP := false
	for _, ind := range indicators {
		if ind.ID == "NY.GDP.MKTP.CD" {
			foundGDP = true
		}
	}
	if !foundGDP {
		t.Error("expected GDP indicator in list")
	}
}
