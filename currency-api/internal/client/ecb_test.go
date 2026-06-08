package client

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func makeSrv(xmlBody string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(xmlBody))
	}))
}

func sampleXML(date string) string {
	type xmlRate struct {
		Currency string  `xml:"currency,attr"`
		Rate     float64 `xml:"rate,attr"`
	}
	type xmlDay struct {
		Time  string    `xml:"time,attr"`
		Rates []xmlRate `xml:"Cube"`
	}
	type xmlOuter struct {
		Day xmlDay `xml:"Cube"`
	}
	type xmlEnv struct {
		XMLName xml.Name `xml:"Envelope"`
		Cube    xmlOuter `xml:"Cube"`
	}
	env := xmlEnv{
		Cube: xmlOuter{Day: xmlDay{
			Time: date,
			Rates: []xmlRate{
				{Currency: "USD", Rate: 1.0942},
				{Currency: "GBP", Rate: 0.8603},
				{Currency: "JPY", Rate: 161.26},
				{Currency: "CHF", Rate: 0.9321},
			},
		}},
	}
	b, _ := xml.Marshal(env)
	return string(b)
}

func newTestECBClient(srv *httptest.Server) *Client {
	c := New()
	c.http = srv.Client()
	// inject cached data directly to avoid HTTP calls in unit tests
	rates, _ := c.fetchRates(context.Background(), srv.URL)
	c.daily = &rates[0]
	c.dailyAt = time.Now() // mark as fresh so ensureDaily short-circuits
	return c
}

// ── crossRate ─────────────────────────────────────────────────────────────────

func TestCrossRate_SameCurrency(t *testing.T) {
	r, err := crossRate(map[string]float64{"USD": 1.0942}, "USD", "USD")
	if err != nil {
		t.Fatal(err)
	}
	if r != 1.0 {
		t.Errorf("expected 1.0, got %v", r)
	}
}

func TestCrossRate_EURtoUSD(t *testing.T) {
	rates := map[string]float64{"USD": 1.0942, "GBP": 0.8603}
	r, err := crossRate(rates, "EUR", "USD")
	if err != nil {
		t.Fatal(err)
	}
	if r != 1.0942 {
		t.Errorf("expected 1.0942, got %v", r)
	}
}

func TestCrossRate_USDtoEUR(t *testing.T) {
	rates := map[string]float64{"USD": 1.0942}
	r, err := crossRate(rates, "USD", "EUR")
	if err != nil {
		t.Fatal(err)
	}
	want := 1.0 / 1.0942
	if abs(r-want) > 0.000001 {
		t.Errorf("expected ~%.6f, got %.6f", want, r)
	}
}

func TestCrossRate_USDtoGBP(t *testing.T) {
	rates := map[string]float64{"USD": 1.0942, "GBP": 0.8603}
	r, err := crossRate(rates, "USD", "GBP")
	if err != nil {
		t.Fatal(err)
	}
	want := 0.8603 / 1.0942
	if abs(r-want) > 0.000001 {
		t.Errorf("expected ~%.6f, got %.6f", want, r)
	}
}

func TestCrossRate_UnknownCurrency(t *testing.T) {
	rates := map[string]float64{"USD": 1.0942}
	_, err := crossRate(rates, "USD", "FAKE")
	if err == nil {
		t.Error("expected error for unknown currency")
	}
}

// ── fetchRates XML parsing ────────────────────────────────────────────────────

func TestFetchRates_ParsesXML(t *testing.T) {
	srv := makeSrv(sampleXML("2024-01-15"))
	defer srv.Close()

	c := New()
	c.http = srv.Client()
	rates, err := c.fetchRates(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rates) != 1 {
		t.Fatalf("expected 1 RateSet, got %d", len(rates))
	}
	if rates[0].Date != "2024-01-15" {
		t.Errorf("expected date 2024-01-15, got %s", rates[0].Date)
	}
	if rates[0].Rates["USD"] != 1.0942 {
		t.Errorf("USD rate: got %v, want 1.0942", rates[0].Rates["USD"])
	}
	if rates[0].Rates["GBP"] != 0.8603 {
		t.Errorf("GBP rate: got %v, want 0.8603", rates[0].Rates["GBP"])
	}
}

// ── Convert ───────────────────────────────────────────────────────────────────

func TestConvert_EURtoUSD(t *testing.T) {
	srv := makeSrv(sampleXML("2024-01-15"))
	defer srv.Close()

	c := newTestECBClient(srv)
	res, err := c.Convert(context.Background(), "EUR", "USD", 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if abs(res.Result-109.42) > 0.01 {
		t.Errorf("convert 100 EUR→USD: got %.4f, want ~109.42", res.Result)
	}
	if res.From != "EUR" || res.To != "USD" {
		t.Errorf("from/to mismatch: %s/%s", res.From, res.To)
	}
}

func TestConvert_USDtoGBP(t *testing.T) {
	srv := makeSrv(sampleXML("2024-01-15"))
	defer srv.Close()

	c := newTestECBClient(srv)
	res, err := c.Convert(context.Background(), "USD", "GBP", 200)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := 200 * (0.8603 / 1.0942)
	if abs(res.Result-want) > 0.01 {
		t.Errorf("convert 200 USD→GBP: got %.4f, want %.4f", res.Result, want)
	}
}

func TestConvert_EmptyCurrency(t *testing.T) {
	srv := makeSrv(sampleXML("2024-01-15"))
	defer srv.Close()

	c := newTestECBClient(srv)
	_, err := c.Convert(context.Background(), "", "USD", 100)
	if err == nil {
		t.Error("expected error for empty from")
	}
}

// ── roundTo ──────────────────────────────────────────────────────────────────

func TestRoundTo(t *testing.T) {
	if roundTo(1.23456789, 4) != 1.2346 {
		t.Errorf("roundTo(1.23456789, 4) = %v", roundTo(1.23456789, 4))
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
