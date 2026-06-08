package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func makeRaw(cca2, name, region string, pop int64) rawCountry {
	r := rawCountry{
		CCA2:       cca2,
		CCA3:       cca2 + "X",
		Region:     region,
		Population: pop,
		Independent: true,
	}
	r.Name.Common = name
	r.Name.Official = "Republic of " + name
	r.Languages = map[string]string{"eng": "English"}
	r.Currencies = map[string]struct {
		Name   string `json:"name"`
		Symbol string `json:"symbol"`
	}{
		"USD": {Name: "United States dollar", Symbol: "$"},
	}
	r.Flag = "🏳"
	return r
}

func newTestClient(srv *httptest.Server) *Client {
	return &Client{
		http:    srv.Client(),
		baseURL: srv.URL,
	}
}

func TestGetAll_OK(t *testing.T) {
	raws := []rawCountry{
		makeRaw("DE", "Germany", "Europe", 83000000),
		makeRaw("FR", "France", "Europe", 67000000),
	}
	b, _ := json.Marshal(raws)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	countries, err := c.GetAll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(countries) != 2 {
		t.Errorf("expected 2 countries, got %d", len(countries))
	}
}

func TestGetAll_Caching(t *testing.T) {
	calls := 0
	raws := []rawCountry{makeRaw("DE", "Germany", "Europe", 83000000)}
	b, _ := json.Marshal(raws)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	c.GetAll(context.Background())
	c.GetAll(context.Background())
	c.GetAll(context.Background())

	if calls != 1 {
		t.Errorf("expected 1 upstream call (caching), got %d", calls)
	}
}

func TestGetByCode_FromCache(t *testing.T) {
	raws := []rawCountry{
		makeRaw("DE", "Germany", "Europe", 83000000),
		makeRaw("FR", "France", "Europe", 67000000),
	}
	b, _ := json.Marshal(raws)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	// Prime the cache
	c.GetAll(context.Background())

	ct, err := c.GetByCode(context.Background(), "DE")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ct.Name != "Germany" {
		t.Errorf("expected Germany, got %q", ct.Name)
	}
}

func TestGetByCode_NotFound(t *testing.T) {
	raws := []rawCountry{makeRaw("DE", "Germany", "Europe", 83000000)}
	b, _ := json.Marshal(raws)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	c.GetAll(context.Background())

	_, err := c.GetByCode(context.Background(), "ZZ")
	if err == nil {
		t.Fatal("expected not_found error")
	}
}

func TestSearchByName_Match(t *testing.T) {
	raws := []rawCountry{
		makeRaw("DE", "Germany", "Europe", 83000000),
		makeRaw("FR", "France", "Europe", 67000000),
	}
	b, _ := json.Marshal(raws)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	c.GetAll(context.Background())

	results, err := c.SearchByName(context.Background(), "germ", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].CCA2 != "DE" {
		t.Errorf("expected Germany, got %+v", results)
	}
}

func TestSearchByName_NotFound(t *testing.T) {
	raws := []rawCountry{makeRaw("DE", "Germany", "Europe", 83000000)}
	b, _ := json.Marshal(raws)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	c.GetAll(context.Background())

	_, err := c.SearchByName(context.Background(), "Atlantis", false)
	if err == nil {
		t.Fatal("expected not_found error")
	}
}

func TestGetByRegion_Match(t *testing.T) {
	raws := []rawCountry{
		makeRaw("DE", "Germany", "Europe", 83000000),
		makeRaw("JP", "Japan", "Asia", 125000000),
	}
	b, _ := json.Marshal(raws)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	c.GetAll(context.Background())

	results, err := c.GetByRegion(context.Background(), "europe")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].CCA2 != "DE" {
		t.Errorf("expected 1 European country (Germany), got %+v", results)
	}
}

func TestGetByLanguage_Match(t *testing.T) {
	raws := []rawCountry{
		makeRaw("DE", "Germany", "Europe", 83000000),
	}
	raws[0].Languages = map[string]string{"deu": "German"}
	b, _ := json.Marshal(raws)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	c.GetAll(context.Background())

	results, err := c.GetByLanguage(context.Background(), "German")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestGetByCurrency_Match(t *testing.T) {
	raws := []rawCountry{
		makeRaw("DE", "Germany", "Europe", 83000000),
		makeRaw("FR", "France", "Europe", 67000000),
	}
	for i := range raws {
		raws[i].Currencies = map[string]struct {
			Name   string `json:"name"`
			Symbol string `json:"symbol"`
		}{
			"EUR": {Name: "Euro", Symbol: "€"},
		}
	}
	b, _ := json.Marshal(raws)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	c.GetAll(context.Background())

	results, err := c.GetByCurrency(context.Background(), "EUR")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 EUR countries, got %d", len(results))
	}
}

func TestNormalize_CallingCodes(t *testing.T) {
	r := makeRaw("DE", "Germany", "Europe", 83000000)
	r.IDD.Root = "+4"
	r.IDD.Suffixes = []string{"9"}

	ct := normalize(r)
	if len(ct.CallingCodes) == 0 {
		t.Fatal("expected calling codes")
	}
	if ct.CallingCodes[0] != "+49" {
		t.Errorf("expected +49, got %q", ct.CallingCodes[0])
	}
}

func TestCacheExpiry(t *testing.T) {
	raws := []rawCountry{makeRaw("DE", "Germany", "Europe", 83000000)}
	b, _ := json.Marshal(raws)

	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	// Manually set an expired cache
	c.cacheTime = time.Now().Add(-25 * time.Hour)
	c.allCache = []Country{{Name: "Old"}}

	c.GetAll(context.Background())
	if calls != 1 {
		t.Errorf("expected 1 upstream call after cache expiry, got %d", calls)
	}
}
