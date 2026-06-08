// Package client wraps the European Central Bank (ECB) Euro Foreign Exchange
// Reference Rates. Free, no auth. Updated daily around 16:00 CET.
//
// Endpoints used:
//   - Daily:   https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml
//   - 90-day:  https://www.ecb.europa.eu/stats/eurofxref/eurofxref-hist-90d.xml
//   - History: https://www.ecb.europa.eu/stats/eurofxref/eurofxref-hist.xml  (large, ~3MB)
//
// All rates are EUR-based (i.e. 1 EUR = X CCY). To get CCY1→CCY2 cross-rates:
// rate = rate(EUR→CCY2) / rate(EUR→CCY1).
package client

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	dailyURL  = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-daily.xml"
	hist90URL = "https://www.ecb.europa.eu/stats/eurofxref/eurofxref-hist-90d.xml"
)

// ── Types ─────────────────────────────────────────────────────────────────────

// RateSet holds all EUR-based rates for a single date.
type RateSet struct {
	Date  string             `json:"date"`
	Rates map[string]float64 `json:"rates"` // currency code → rate vs EUR
}

// ConversionResult is the result of a currency conversion.
type ConversionResult struct {
	From     string  `json:"from"`
	To       string  `json:"to"`
	Amount   float64 `json:"amount"`
	Result   float64 `json:"result"`
	Rate     float64 `json:"rate"`
	Date     string  `json:"date"`
	Inverted float64 `json:"invertedRate"` // 1/rate
}

// HistoryPoint is one date in a rate history.
type HistoryPoint struct {
	Date string  `json:"date"`
	Rate float64 `json:"rate"`
}

// ── Client ────────────────────────────────────────────────────────────────────

type Client struct {
	http *http.Client

	mu          sync.RWMutex
	daily       *RateSet
	dailyAt     time.Time
	hist90      []RateSet
	hist90At    time.Time
}

func New() *Client {
	return &Client{
		http: &http.Client{Timeout: 20 * time.Second},
	}
}

// ── Cache helpers ─────────────────────────────────────────────────────────────

func (c *Client) ensureDaily(ctx context.Context) error {
	c.mu.RLock()
	fresh := c.daily != nil && time.Since(c.dailyAt) < 4*time.Hour
	c.mu.RUnlock()
	if fresh {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	// double check
	if c.daily != nil && time.Since(c.dailyAt) < 4*time.Hour {
		return nil
	}
	rates, err := c.fetchRates(ctx, dailyURL)
	if err != nil {
		return err
	}
	if len(rates) == 0 {
		return fmt.Errorf("ECB daily feed returned no rates")
	}
	c.daily = &rates[0]
	c.dailyAt = time.Now()
	return nil
}

func (c *Client) ensureHist90(ctx context.Context) error {
	c.mu.RLock()
	fresh := c.hist90 != nil && time.Since(c.hist90At) < 6*time.Hour
	c.mu.RUnlock()
	if fresh {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.hist90 != nil && time.Since(c.hist90At) < 6*time.Hour {
		return nil
	}
	rates, err := c.fetchRates(ctx, hist90URL)
	if err != nil {
		return err
	}
	c.hist90 = rates
	c.hist90At = time.Now()
	return nil
}

// ── Public methods ────────────────────────────────────────────────────────────

// GetLatestRates returns all EUR-based rates for today.
func (c *Client) GetLatestRates(ctx context.Context) (*RateSet, error) {
	if err := c.ensureDaily(ctx); err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.daily, nil
}

// Convert converts an amount from one currency to another using today's rates.
// Supports EUR as from/to directly. Cross rates are computed via EUR.
func (c *Client) Convert(ctx context.Context, from, to string, amount float64) (*ConversionResult, error) {
	if err := c.ensureDaily(ctx); err != nil {
		return nil, err
	}
	c.mu.RLock()
	daily := c.daily
	c.mu.RUnlock()

	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))
	if from == "" || to == "" {
		return nil, fmt.Errorf("from and to currency codes are required")
	}

	rate, err := crossRate(daily.Rates, from, to)
	if err != nil {
		return nil, err
	}

	result := amount * rate
	inv := 0.0
	if rate != 0 {
		inv = 1.0 / rate
	}
	return &ConversionResult{
		From:     from,
		To:       to,
		Amount:   amount,
		Result:   roundTo(result, 6),
		Rate:     roundTo(rate, 6),
		Date:     daily.Date,
		Inverted: roundTo(inv, 6),
	}, nil
}

// GetCurrencies returns a sorted list of all supported currency codes.
func (c *Client) GetCurrencies(ctx context.Context) ([]string, error) {
	if err := c.ensureDaily(ctx); err != nil {
		return nil, err
	}
	c.mu.RLock()
	daily := c.daily
	c.mu.RUnlock()

	codes := make([]string, 0, len(daily.Rates)+1)
	codes = append(codes, "EUR")
	for code := range daily.Rates {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes, nil
}

// GetHistory returns the rate of from→to for the last available dates (up to 90).
// Returns newest-first.
func (c *Client) GetHistory(ctx context.Context, from, to string, limit int) ([]HistoryPoint, error) {
	if err := c.ensureHist90(ctx); err != nil {
		return nil, err
	}
	c.mu.RLock()
	hist := c.hist90
	c.mu.RUnlock()

	from = strings.ToUpper(strings.TrimSpace(from))
	to = strings.ToUpper(strings.TrimSpace(to))

	if limit <= 0 || limit > 90 {
		limit = 30
	}

	points := make([]HistoryPoint, 0, limit)
	for _, rs := range hist {
		rate, err := crossRate(rs.Rates, from, to)
		if err != nil {
			continue // date may not have both currencies
		}
		points = append(points, HistoryPoint{Date: rs.Date, Rate: roundTo(rate, 6)})
		if len(points) >= limit {
			break
		}
	}
	return points, nil
}

// ── XML parsing ───────────────────────────────────────────────────────────────

// ECB XML structure:
// <gesmes:Envelope>
//   <Cube>
//     <Cube time="2024-01-15">
//       <Cube currency="USD" rate="1.0942"/>
//       ...
//     </Cube>
//   </Cube>
// </gesmes:Envelope>

type ecbEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Cubes   []ecbDay `xml:"Cube>Cube"`
}

type ecbDay struct {
	Time  string    `xml:"time,attr"`
	Rates []ecbRate `xml:"Cube"`
}

type ecbRate struct {
	Currency string  `xml:"currency,attr"`
	Rate     float64 `xml:"rate,attr"`
}

func (c *Client) fetchRates(ctx context.Context, url string) ([]RateSet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/xml")
	req.Header.Set("User-Agent", "api-suite/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ECB upstream %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // max 8MB
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	var env ecbEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("parse XML: %w", err)
	}

	sets := make([]RateSet, 0, len(env.Cubes))
	for _, day := range env.Cubes {
		if day.Time == "" {
			continue
		}
		rates := make(map[string]float64, len(day.Rates))
		for _, r := range day.Rates {
			if r.Currency != "" && r.Rate > 0 {
				rates[strings.ToUpper(r.Currency)] = r.Rate
			}
		}
		sets = append(sets, RateSet{Date: day.Time, Rates: rates})
	}
	// newest first
	sort.Slice(sets, func(i, j int) bool {
		return sets[i].Date > sets[j].Date
	})
	return sets, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// crossRate computes the from→to rate via EUR.
// rates map is EUR-based (1 EUR = X CCY).
// from→to = rates[to] / rates[from]  (when neither is EUR)
// EUR→CCY = rates[CCY]
// CCY→EUR = 1 / rates[CCY]
func crossRate(rates map[string]float64, from, to string) (float64, error) {
	if from == to {
		return 1.0, nil
	}
	// get EUR→from
	var eurFrom float64
	if from == "EUR" {
		eurFrom = 1.0
	} else {
		r, ok := rates[from]
		if !ok {
			return 0, fmt.Errorf("unsupported currency: %s", from)
		}
		eurFrom = r
	}
	// get EUR→to
	var eurTo float64
	if to == "EUR" {
		eurTo = 1.0
	} else {
		r, ok := rates[to]
		if !ok {
			return 0, fmt.Errorf("unsupported currency: %s", to)
		}
		eurTo = r
	}
	// from→to = eurTo / eurFrom
	if eurFrom == 0 {
		return 0, fmt.Errorf("zero rate for %s", from)
	}
	return eurTo / eurFrom, nil
}

func roundTo(val float64, decimals int) float64 {
	pow := math.Pow(10, float64(decimals))
	return math.Round(val*pow) / pow
}

// parsedFloat is used in tests
func parsedFloat(s string) float64 {
	v, _ := strconv.ParseFloat(s, 64)
	return v
}
