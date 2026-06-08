package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(srv *httptest.Server) *Client {
	c := New("")
	c.http = srv.Client()
	c.baseURL = srv.URL
	return c
}

// ── stripHTML ─────────────────────────────────────────────────────────────────

func TestStripHTML(t *testing.T) {
	got := stripHTML("<p>Hello <b>world</b>!</p>")
	if got != "Hello world!" {
		t.Errorf("got %q", got)
	}
}

func TestTruncate(t *testing.T) {
	if truncate("hello", 10) != "hello" {
		t.Error("should not truncate")
	}
	if truncate("hello world", 5) != "hello…" {
		t.Error("should truncate")
	}
}

// ── GetTopCoins ───────────────────────────────────────────────────────────────

func marketsResponse() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"id":                           "bitcoin",
			"symbol":                       "btc",
			"name":                         "Bitcoin",
			"current_price":                float64(45000),
			"market_cap":                   float64(880000000000),
			"market_cap_rank":              float64(1),
			"total_volume":                 float64(30000000000),
			"high_24h":                     float64(46000),
			"low_24h":                      float64(44000),
			"price_change_24h":             float64(500),
			"price_change_percentage_24h":  float64(1.12),
			"circulating_supply":           float64(19600000),
			"ath":                          float64(69000),
			"ath_date":                     "2021-11-10T14:24:11.849Z",
			"last_updated":                 "2024-01-15T12:00:00.000Z",
		},
	}
}

func TestGetTopCoins_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/coins/markets" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(marketsResponse())
	}))
	defer srv.Close()

	c := newTestClient(srv)
	coins, err := c.GetTopCoins(context.Background(), "usd", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(coins) != 1 {
		t.Fatalf("expected 1 coin, got %d", len(coins))
	}
	if coins[0].ID != "bitcoin" {
		t.Errorf("expected bitcoin, got %s", coins[0].ID)
	}
	if coins[0].CurrentPrice != 45000 {
		t.Errorf("price: got %v, want 45000", coins[0].CurrentPrice)
	}
	if coins[0].MarketCapRank != 1 {
		t.Errorf("rank: got %d", coins[0].MarketCapRank)
	}
}

// ── GetPrices ─────────────────────────────────────────────────────────────────

func TestGetPrices_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(marketsResponse())
	}))
	defer srv.Close()

	c := newTestClient(srv)
	coins, err := c.GetPrices(context.Background(), []string{"bitcoin"}, "usd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(coins) != 1 {
		t.Fatalf("expected 1 coin, got %d", len(coins))
	}
	if coins[0].Currency != "usd" {
		t.Errorf("currency: got %s", coins[0].Currency)
	}
}

func TestGetPrices_EmptyIDs(t *testing.T) {
	c := New("")
	_, err := c.GetPrices(context.Background(), []string{}, "usd")
	if err == nil {
		t.Error("expected error for empty IDs")
	}
}

// ── GetTrending ───────────────────────────────────────────────────────────────

func trendingResponse() map[string]interface{} {
	return map[string]interface{}{
		"coins": []interface{}{
			map[string]interface{}{
				"item": map[string]interface{}{
					"id":              "solana",
					"symbol":          "SOL",
					"name":            "Solana",
					"market_cap_rank": float64(5),
					"score":           float64(0),
					"thumb":           "https://example.com/sol.png",
					"price_btc":       float64(0.002),
				},
			},
		},
	}
}

func TestGetTrending_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search/trending" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(trendingResponse())
	}))
	defer srv.Close()

	c := newTestClient(srv)
	coins, err := c.GetTrending(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(coins) != 1 {
		t.Fatalf("expected 1 coin, got %d", len(coins))
	}
	if coins[0].ID != "solana" {
		t.Errorf("expected solana, got %s", coins[0].ID)
	}
}

// ── GetCoinDetail ─────────────────────────────────────────────────────────────

func coinDetailResponse() map[string]interface{} {
	return map[string]interface{}{
		"id":          "bitcoin",
		"symbol":      "btc",
		"name":        "Bitcoin",
		"genesis_date": "2009-01-03",
		"market_cap_rank": float64(1),
		"sentiment_votes_up_percentage": float64(82.5),
		"description": map[string]interface{}{
			"en": "<p>Bitcoin is the first cryptocurrency.</p>",
		},
		"categories": []interface{}{"Cryptocurrency", "Layer 1"},
		"links": map[string]interface{}{
			"homepage": []interface{}{"https://bitcoin.org"},
		},
		"market_data": map[string]interface{}{
			"current_price": map[string]interface{}{"usd": float64(45000)},
			"market_cap":    map[string]interface{}{"usd": float64(880000000000)},
			"total_volume":  map[string]interface{}{"usd": float64(30000000000)},
			"high_24h":      map[string]interface{}{"usd": float64(46000)},
			"low_24h":       map[string]interface{}{"usd": float64(44000)},
			"ath":           map[string]interface{}{"usd": float64(69000)},
			"price_change_24h":            float64(500),
			"price_change_percentage_24h": float64(1.12),
			"circulating_supply":          float64(19600000),
			"market_cap_rank":             float64(1),
		},
	}
}

func TestGetCoinDetail_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(coinDetailResponse())
	}))
	defer srv.Close()

	c := newTestClient(srv)
	detail, err := c.GetCoinDetail(context.Background(), "bitcoin", "usd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.ID != "bitcoin" {
		t.Errorf("expected bitcoin, got %s", detail.ID)
	}
	if detail.Description != "Bitcoin is the first cryptocurrency." {
		t.Errorf("description: %q", detail.Description)
	}
	if len(detail.Categories) != 2 {
		t.Errorf("expected 2 categories, got %d", len(detail.Categories))
	}
	if detail.HomePage != "https://bitcoin.org" {
		t.Errorf("homepage: %s", detail.HomePage)
	}
	if detail.Price == nil {
		t.Fatal("expected price data")
	}
	if detail.Price.CurrentPrice != 45000 {
		t.Errorf("price: %v", detail.Price.CurrentPrice)
	}
}

func TestGetCoinDetail_EmptyID(t *testing.T) {
	c := New("")
	_, err := c.GetCoinDetail(context.Background(), "", "usd")
	if err == nil {
		t.Error("expected error for empty coinID")
	}
}
