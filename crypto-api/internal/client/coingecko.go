// Package client wraps the CoinGecko API (api.coingecko.com/api/v3).
// Free public tier: 30 calls/min, no auth required.
// Demo API key available for free at https://www.coingecko.com/en/api
// (x-cg-demo-api-key header, 30 calls/min — same as free, but more stable).
// Docs: https://docs.coingecko.com/reference/introduction
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const baseURL = "https://api.coingecko.com/api/v3"

// ── Types ─────────────────────────────────────────────────────────────────────

// CoinPrice holds current price data for one coin.
type CoinPrice struct {
	ID                   string   `json:"id"`
	Symbol               string   `json:"symbol"`
	Name                 string   `json:"name"`
	CurrentPrice         float64  `json:"currentPrice"`
	MarketCap            float64  `json:"marketCap"`
	MarketCapRank        int      `json:"marketCapRank"`
	TotalVolume          float64  `json:"totalVolume"`
	High24h              float64  `json:"high24h"`
	Low24h               float64  `json:"low24h"`
	PriceChange24h       float64  `json:"priceChange24h"`
	PriceChangePct24h    float64  `json:"priceChangePct24h"`
	CirculatingSupply    float64  `json:"circulatingSupply,omitempty"`
	TotalSupply          *float64 `json:"totalSupply,omitempty"`
	ATH                  float64  `json:"ath"`
	ATHDate              string   `json:"athDate,omitempty"`
	LastUpdated          string   `json:"lastUpdated,omitempty"`
	Currency             string   `json:"currency"`
}

// CoinDetail holds full coin profile info.
type CoinDetail struct {
	ID          string   `json:"id"`
	Symbol      string   `json:"symbol"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Categories  []string `json:"categories,omitempty"`
	HomePage    string   `json:"homePage,omitempty"`
	GenesisDate string   `json:"genesisDate,omitempty"`
	SentimentUp float64  `json:"sentimentVotesUpPct,omitempty"`
	Rank        int      `json:"marketCapRank"`
	Price       *CoinPrice `json:"currentData,omitempty"`
}

// TrendingCoin is a trending coin result.
type TrendingCoin struct {
	ID            string  `json:"id"`
	Symbol        string  `json:"symbol"`
	Name          string  `json:"name"`
	MarketCapRank int     `json:"marketCapRank"`
	Score         int     `json:"score"` // trending rank (0 = #1)
	ThumbURL      string  `json:"thumbUrl,omitempty"`
	PriceBTC      float64 `json:"priceBtc,omitempty"`
}

// OHLCPoint is one candlestick data point.
type OHLCPoint struct {
	Timestamp int64   `json:"timestamp"`
	Open      float64 `json:"open"`
	High      float64 `json:"high"`
	Low       float64 `json:"low"`
	Close     float64 `json:"close"`
}

// ── Client ────────────────────────────────────────────────────────────────────

type Client struct {
	http    *http.Client
	baseURL string
	apiKey  string // optional demo key
}

func New(apiKey string) *Client {
	return &Client{
		http:    &http.Client{Timeout: 15 * time.Second},
		baseURL: baseURL,
		apiKey:  apiKey,
	}
}

// ── Prices ────────────────────────────────────────────────────────────────────

// GetPrices fetches prices for one or more coins (by CoinGecko ID) in a given currency.
// coinIDs: e.g. ["bitcoin", "ethereum", "solana"]
// currency: e.g. "usd", "eur", "gbp"
func (c *Client) GetPrices(ctx context.Context, coinIDs []string, currency string) ([]CoinPrice, error) {
	if len(coinIDs) == 0 {
		return nil, fmt.Errorf("at least one coin ID is required")
	}
	if currency == "" {
		currency = "usd"
	}
	if len(coinIDs) > 50 {
		coinIDs = coinIDs[:50]
	}

	params := url.Values{}
	params.Set("ids", strings.Join(coinIDs, ","))
	params.Set("vs_currency", currency)
	params.Set("order", "market_cap_desc")
	params.Set("per_page", "250")
	params.Set("page", "1")
	params.Set("sparkline", "false")
	params.Set("price_change_percentage", "24h")

	var raw []map[string]interface{}
	if err := c.get(ctx, "/coins/markets", params, &raw); err != nil {
		return nil, err
	}

	result := make([]CoinPrice, 0, len(raw))
	for _, r := range raw {
		result = append(result, mapCoinPrice(r, currency))
	}
	return result, nil
}

// GetTopCoins fetches the top N coins by market cap.
func (c *Client) GetTopCoins(ctx context.Context, currency string, limit int) ([]CoinPrice, error) {
	if currency == "" {
		currency = "usd"
	}
	if limit <= 0 || limit > 250 {
		limit = 10
	}

	params := url.Values{}
	params.Set("vs_currency", currency)
	params.Set("order", "market_cap_desc")
	params.Set("per_page", fmt.Sprintf("%d", limit))
	params.Set("page", "1")
	params.Set("sparkline", "false")

	var raw []map[string]interface{}
	if err := c.get(ctx, "/coins/markets", params, &raw); err != nil {
		return nil, err
	}

	result := make([]CoinPrice, 0, len(raw))
	for _, r := range raw {
		result = append(result, mapCoinPrice(r, currency))
	}
	return result, nil
}

// ── Trending ──────────────────────────────────────────────────────────────────

// GetTrending returns the top 7 trending coins on CoinGecko (24h).
func (c *Client) GetTrending(ctx context.Context) ([]TrendingCoin, error) {
	var raw struct {
		Coins []struct {
			Item struct {
				ID            string  `json:"id"`
				Symbol        string  `json:"symbol"`
				Name          string  `json:"name"`
				MarketCapRank int     `json:"market_cap_rank"`
				Score         int     `json:"score"`
				Thumb         string  `json:"thumb"`
				PriceBTC      float64 `json:"price_btc"`
			} `json:"item"`
		} `json:"coins"`
	}
	if err := c.get(ctx, "/search/trending", url.Values{}, &raw); err != nil {
		return nil, err
	}

	result := make([]TrendingCoin, 0, len(raw.Coins))
	for _, c := range raw.Coins {
		result = append(result, TrendingCoin{
			ID:            c.Item.ID,
			Symbol:        c.Item.Symbol,
			Name:          c.Item.Name,
			MarketCapRank: c.Item.MarketCapRank,
			Score:         c.Item.Score,
			ThumbURL:      c.Item.Thumb,
			PriceBTC:      c.Item.PriceBTC,
		})
	}
	return result, nil
}

// ── Coin Detail ───────────────────────────────────────────────────────────────

// GetCoinDetail fetches profile data for a single coin.
func (c *Client) GetCoinDetail(ctx context.Context, coinID, currency string) (*CoinDetail, error) {
	if coinID == "" {
		return nil, fmt.Errorf("coinID is required")
	}
	if currency == "" {
		currency = "usd"
	}

	params := url.Values{}
	params.Set("localization", "false")
	params.Set("tickers", "false")
	params.Set("market_data", "true")
	params.Set("community_data", "false")
	params.Set("developer_data", "false")

	var raw map[string]interface{}
	if err := c.get(ctx, "/coins/"+coinID, params, &raw); err != nil {
		return nil, err
	}

	return mapCoinDetail(raw, currency), nil
}

// ── Mappers ───────────────────────────────────────────────────────────────────

func mapCoinPrice(r map[string]interface{}, currency string) CoinPrice {
	cp := CoinPrice{Currency: currency}
	cp.ID, _ = r["id"].(string)
	cp.Symbol, _ = r["symbol"].(string)
	cp.Name, _ = r["name"].(string)
	cp.CurrentPrice, _ = r["current_price"].(float64)
	cp.MarketCap, _ = r["market_cap"].(float64)
	if rank, ok := r["market_cap_rank"].(float64); ok {
		cp.MarketCapRank = int(rank)
	}
	cp.TotalVolume, _ = r["total_volume"].(float64)
	cp.High24h, _ = r["high_24h"].(float64)
	cp.Low24h, _ = r["low_24h"].(float64)
	cp.PriceChange24h, _ = r["price_change_24h"].(float64)
	cp.PriceChangePct24h, _ = r["price_change_percentage_24h"].(float64)
	cp.CirculatingSupply, _ = r["circulating_supply"].(float64)
	if ts, ok := r["total_supply"].(float64); ok {
		cp.TotalSupply = &ts
	}
	cp.ATH, _ = r["ath"].(float64)
	cp.ATHDate, _ = r["ath_date"].(string)
	cp.LastUpdated, _ = r["last_updated"].(string)
	return cp
}

func mapCoinDetail(r map[string]interface{}, currency string) *CoinDetail {
	cd := &CoinDetail{}
	cd.ID, _ = r["id"].(string)
	cd.Symbol, _ = r["symbol"].(string)
	cd.Name, _ = r["name"].(string)
	cd.GenesisDate, _ = r["genesis_date"].(string)

	if rank, ok := r["market_cap_rank"].(float64); ok {
		cd.Rank = int(rank)
	}
	if vUp, ok := r["sentiment_votes_up_percentage"].(float64); ok {
		cd.SentimentUp = vUp
	}

	// description
	if desc, ok := r["description"].(map[string]interface{}); ok {
		if en, ok := desc["en"].(string); ok {
			// strip HTML tags and truncate
			cd.Description = truncate(stripHTML(en), 500)
		}
	}

	// categories
	if cats, ok := r["categories"].([]interface{}); ok {
		for _, cat := range cats {
			if s, ok := cat.(string); ok && s != "" {
				cd.Categories = append(cd.Categories, s)
				if len(cd.Categories) >= 5 {
					break
				}
			}
		}
	}

	// homepage
	if links, ok := r["links"].(map[string]interface{}); ok {
		if hp, ok := links["homepage"].([]interface{}); ok && len(hp) > 0 {
			cd.HomePage, _ = hp[0].(string)
		}
	}

	// current market data
	if md, ok := r["market_data"].(map[string]interface{}); ok {
		price := &CoinPrice{ID: cd.ID, Symbol: cd.Symbol, Name: cd.Name, Currency: currency}
		cur := strings.ToLower(currency)

		if cp, ok := md["current_price"].(map[string]interface{}); ok {
			price.CurrentPrice, _ = cp[cur].(float64)
		}
		if mc, ok := md["market_cap"].(map[string]interface{}); ok {
			price.MarketCap, _ = mc[cur].(float64)
		}
		if tv, ok := md["total_volume"].(map[string]interface{}); ok {
			price.TotalVolume, _ = tv[cur].(float64)
		}
		if h, ok := md["high_24h"].(map[string]interface{}); ok {
			price.High24h, _ = h[cur].(float64)
		}
		if l, ok := md["low_24h"].(map[string]interface{}); ok {
			price.Low24h, _ = l[cur].(float64)
		}
		if pc, ok := md["price_change_24h"].(float64); ok {
			price.PriceChange24h = pc
		}
		if pcp, ok := md["price_change_percentage_24h"].(float64); ok {
			price.PriceChangePct24h = pcp
		}
		if cs, ok := md["circulating_supply"].(float64); ok {
			price.CirculatingSupply = cs
		}
		if ts, ok := md["total_supply"].(float64); ok {
			price.TotalSupply = &ts
		}
		if ath, ok := md["ath"].(map[string]interface{}); ok {
			price.ATH, _ = ath[cur].(float64)
		}
		if rank, ok := md["market_cap_rank"].(float64); ok {
			price.MarketCapRank = int(rank)
			cd.Rank = price.MarketCapRank
		}
		cd.Price = price
	}

	return cd
}

// ── HTTP helper ───────────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, path string, params url.Values, dest interface{}) error {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "api-suite/1.0")
	if c.apiKey != "" {
		req.Header.Set("x-cg-demo-api-key", c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode == 429 {
		return fmt.Errorf("rate limited by CoinGecko — please retry in a moment")
	}
	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Error string `json:"error"`
		}
		if jsonErr := json.Unmarshal(body, &apiErr); jsonErr == nil && apiErr.Error != "" {
			return fmt.Errorf("CoinGecko error: %s", apiErr.Error)
		}
		return fmt.Errorf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func stripHTML(s string) string {
	// Simple HTML tag stripping (sufficient for CoinGecko descriptions)
	var result strings.Builder
	inTag := false
	for _, ch := range s {
		if ch == '<' {
			inTag = true
			continue
		}
		if ch == '>' {
			inTag = false
			continue
		}
		if !inTag {
			result.WriteRune(ch)
		}
	}
	// Collapse multiple spaces/newlines
	return strings.Join(strings.Fields(result.String()), " ")
}
