// Package client wraps the Frankfurter exchange rate API.
// Docs: https://www.frankfurter.app/docs/
// Free, no auth, data from European Central Bank (ECB).
// Supports: latest rates, historical rates, time series, currency list.
// History goes back to 1999-01-04.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const (
	defaultBase    = "https://api.frankfurter.app"
	defaultTimeout = 15 * time.Second
)

// LatestRates holds the latest exchange rates for a base currency.
type LatestRates struct {
	Amount float64            `json:"amount"`
	Base   string             `json:"base"`
	Date   string             `json:"date"`
	Rates  map[string]float64 `json:"rates"`
}

// HistoricalRates holds rates for a specific past date.
type HistoricalRates struct {
	Amount float64            `json:"amount"`
	Base   string             `json:"base"`
	Date   string             `json:"date"`
	Rates  map[string]float64 `json:"rates"`
}

// TimeSeriesRates holds exchange rate time series for a date range.
type TimeSeriesRates struct {
	Amount    float64                       `json:"amount"`
	Base      string                        `json:"base"`
	StartDate string                        `json:"start_date"`
	EndDate   string                        `json:"end_date"`
	Rates     map[string]map[string]float64 `json:"rates"` // date → currency → rate
	Dates     []string                      `json:"dates"` // sorted date list
	DataPoints int                          `json:"data_points"`
}

// ConversionResult holds a currency conversion result.
type ConversionResult struct {
	From     string  `json:"from"`
	To       string  `json:"to"`
	Amount   float64 `json:"amount"`
	Result   float64 `json:"result"`
	Rate     float64 `json:"rate"`
	Date     string  `json:"date"`
}

// CurrencyInfo describes a supported currency.
type CurrencyInfo struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// Client is the Frankfurter API client.
type Client struct {
	http    *http.Client
	baseURL string
}

// New returns a new Frankfurter client.
func New() *Client {
	return &Client{
		http:    &http.Client{Timeout: defaultTimeout},
		baseURL: defaultBase,
	}
}

// ── Internal helpers ─────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, path string, params url.Values, out interface{}) error {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("not_found")
	}
	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Message string `json:"message"`
		}
		if jsonErr := json.NewDecoder(resp.Body).Decode(&errResp); jsonErr == nil && errResp.Message != "" {
			return fmt.Errorf("API error: %s", errResp.Message)
		}
		return fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

func validateDate(d string) error {
	if _, err := time.Parse("2006-01-02", d); err != nil {
		return fmt.Errorf("invalid date %q: use YYYY-MM-DD", d)
	}
	return nil
}

func normalizeCurrency(c string) string {
	return strings.ToUpper(strings.TrimSpace(c))
}

// ── Public API ───────────────────────────────────────────────────────────────

// GetLatest returns the latest exchange rates for a base currency.
// base: e.g. "EUR", "USD" (default "EUR")
// symbols: specific currencies to include (empty = all)
func (c *Client) GetLatest(ctx context.Context, base string, symbols []string) (*LatestRates, error) {
	if base == "" {
		base = "EUR"
	}
	base = normalizeCurrency(base)

	params := url.Values{"base": []string{base}}
	if len(symbols) > 0 {
		normalized := make([]string, len(symbols))
		for i, s := range symbols {
			normalized[i] = normalizeCurrency(s)
		}
		params.Set("symbols", strings.Join(normalized, ","))
	}

	var result LatestRates
	if err := c.get(ctx, "/latest", params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetHistorical returns exchange rates for a specific date (back to 1999-01-04).
func (c *Client) GetHistorical(ctx context.Context, date, base string, symbols []string) (*HistoricalRates, error) {
	if err := validateDate(date); err != nil {
		return nil, err
	}
	if base == "" {
		base = "EUR"
	}
	base = normalizeCurrency(base)

	params := url.Values{"base": []string{base}}
	if len(symbols) > 0 {
		normalized := make([]string, len(symbols))
		for i, s := range symbols {
			normalized[i] = normalizeCurrency(s)
		}
		params.Set("symbols", strings.Join(normalized, ","))
	}

	var result HistoricalRates
	if err := c.get(ctx, "/"+date, params, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetTimeSeries returns exchange rates for a date range (max 365 days to stay polite).
// Both startDate and endDate must be in YYYY-MM-DD format.
func (c *Client) GetTimeSeries(ctx context.Context, startDate, endDate, base string, symbols []string) (*TimeSeriesRates, error) {
	if err := validateDate(startDate); err != nil {
		return nil, err
	}
	if err := validateDate(endDate); err != nil {
		return nil, err
	}

	start, _ := time.Parse("2006-01-02", startDate)
	end, _ := time.Parse("2006-01-02", endDate)

	if end.Before(start) {
		return nil, fmt.Errorf("end_date must be after start_date")
	}
	if end.Sub(start) > 365*24*time.Hour {
		return nil, fmt.Errorf("date range too large: max 365 days per request")
	}

	if base == "" {
		base = "EUR"
	}
	base = normalizeCurrency(base)

	params := url.Values{"base": []string{base}}
	if len(symbols) > 0 {
		normalized := make([]string, len(symbols))
		for i, s := range symbols {
			normalized[i] = normalizeCurrency(s)
		}
		params.Set("symbols", strings.Join(normalized, ","))
	}

	path := fmt.Sprintf("/%s..%s", startDate, endDate)

	var raw struct {
		Amount float64                       `json:"amount"`
		Base   string                        `json:"base"`
		StartDate string                     `json:"start_date"`
		EndDate   string                     `json:"end_date"`
		Rates     map[string]map[string]float64 `json:"rates"`
	}
	if err := c.get(ctx, path, params, &raw); err != nil {
		return nil, err
	}

	// Sort dates
	dates := make([]string, 0, len(raw.Rates))
	for d := range raw.Rates {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	return &TimeSeriesRates{
		Amount:     raw.Amount,
		Base:       raw.Base,
		StartDate:  raw.StartDate,
		EndDate:    raw.EndDate,
		Rates:      raw.Rates,
		Dates:      dates,
		DataPoints: len(dates),
	}, nil
}

// Convert converts an amount from one currency to another using latest rates.
func (c *Client) Convert(ctx context.Context, from, to string, amount float64) (*ConversionResult, error) {
	from = normalizeCurrency(from)
	to = normalizeCurrency(to)

	if from == "" || to == "" {
		return nil, fmt.Errorf("from and to currencies are required")
	}
	if amount <= 0 {
		return nil, fmt.Errorf("amount must be positive")
	}
	if from == to {
		return &ConversionResult{
			From: from, To: to,
			Amount: amount, Result: amount, Rate: 1.0,
		}, nil
	}

	params := url.Values{
		"base":    []string{from},
		"symbols": []string{to},
		"amount":  []string{fmt.Sprintf("%.6f", amount)},
	}

	var raw LatestRates
	if err := c.get(ctx, "/latest", params, &raw); err != nil {
		return nil, err
	}

	rate, ok := raw.Rates[to]
	if !ok {
		return nil, fmt.Errorf("currency not found: %s", to)
	}

	return &ConversionResult{
		From:   from,
		To:     to,
		Amount: amount,
		Result: rate * amount,
		Rate:   rate,
		Date:   raw.Date,
	}, nil
}

// GetCurrencies returns the list of all supported currencies.
func (c *Client) GetCurrencies(ctx context.Context) ([]CurrencyInfo, error) {
	var raw map[string]string
	if err := c.get(ctx, "/currencies", nil, &raw); err != nil {
		return nil, err
	}

	currencies := make([]CurrencyInfo, 0, len(raw))
	for code, name := range raw {
		currencies = append(currencies, CurrencyInfo{Code: code, Name: name})
	}
	sort.Slice(currencies, func(i, j int) bool {
		return currencies[i].Code < currencies[j].Code
	})
	return currencies, nil
}
