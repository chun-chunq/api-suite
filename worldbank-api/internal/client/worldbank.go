// Package client wraps the World Bank Open Data API.
// Docs: https://datahelpdesk.worldbank.org/knowledgebase/articles/889392
// No API key required. Free public API.
// Returns country data, economic indicators (GDP, population, inflation, etc.).
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	defaultBase    = "https://api.worldbank.org/v2"
	defaultTimeout = 15 * time.Second
	cacheMinutes   = 60 // cache indicator data for 1h
)

// Country is a World Bank country record.
type Country struct {
	ID           string `json:"id"`
	ISO2Code     string `json:"iso2code"`
	Name         string `json:"name"`
	Region       string `json:"region"`
	IncomeLevel  string `json:"income_level"`
	LendingType  string `json:"lending_type"`
	Capital      string `json:"capital"`
	Longitude    string `json:"longitude"`
	Latitude     string `json:"latitude"`
}

// IndicatorEntry is a single data point for an indicator time series.
type IndicatorEntry struct {
	Year  string   `json:"year"`
	Value *float64 `json:"value"` // nil = no data for this year
}

// IndicatorResult is the full result for an indicator query.
type IndicatorResult struct {
	CountryID   string           `json:"country_id"`
	CountryName string           `json:"country_name"`
	Indicator   string           `json:"indicator"`
	IndicatorID string           `json:"indicator_id"`
	Data        []IndicatorEntry `json:"data"`
}

// Indicator describes an available indicator.
type Indicator struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"`
	Topics []string `json:"topics"`
}

// cacheEntry holds cached indicator results
type cacheEntry struct {
	result    *IndicatorResult
	expiresAt time.Time
}

// Client wraps the World Bank API with an in-memory cache.
type Client struct {
	http    *http.Client
	baseURL string

	mu    sync.RWMutex
	cache map[string]cacheEntry
}

// New returns a new World Bank client.
func New() *Client {
	return &Client{
		http:    &http.Client{Timeout: defaultTimeout},
		baseURL: defaultBase,
		cache:   make(map[string]cacheEntry),
	}
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func (c *Client) getJSON(ctx context.Context, path string, out interface{}) error {
	u := c.baseURL + path
	if !strings.Contains(u, "?") {
		u += "?"
	} else {
		u += "&"
	}
	u += "format=json"

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

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

// ── Public API ────────────────────────────────────────────────────────────────

// GetCountry returns country metadata for an ISO2 country code (e.g. "DE", "US").
func (c *Client) GetCountry(ctx context.Context, code string) (*Country, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, fmt.Errorf("country code is required")
	}

	var raw []json.RawMessage
	if err := c.getJSON(ctx, fmt.Sprintf("/country/%s", url.PathEscape(code)), &raw); err != nil {
		return nil, err
	}
	if len(raw) < 2 {
		return nil, fmt.Errorf("not_found: no country with code %s", code)
	}

	var countries []struct {
		ID       string `json:"id"`
		Iso2Code string `json:"iso2Code"`
		Name     string `json:"name"`
		Region   struct {
			Value string `json:"value"`
		} `json:"region"`
		IncomeLevel struct {
			Value string `json:"value"`
		} `json:"incomeLevel"`
		LendingType struct {
			Value string `json:"value"`
		} `json:"lendingType"`
		CapitalCity string `json:"capitalCity"`
		Longitude   string `json:"longitude"`
		Latitude    string `json:"latitude"`
	}
	if err := json.Unmarshal(raw[1], &countries); err != nil {
		return nil, fmt.Errorf("decode countries: %w", err)
	}
	if len(countries) == 0 {
		return nil, fmt.Errorf("not_found: no country with code %s", code)
	}
	cty := countries[0]
	return &Country{
		ID:          cty.ID,
		ISO2Code:    cty.Iso2Code,
		Name:        cty.Name,
		Region:      cty.Region.Value,
		IncomeLevel: cty.IncomeLevel.Value,
		LendingType: cty.LendingType.Value,
		Capital:     cty.CapitalCity,
		Longitude:   cty.Longitude,
		Latitude:    cty.Latitude,
	}, nil
}

// GetIndicator fetches time series data for a World Bank indicator.
// countryCode: ISO2 code or "all"
// indicatorID: e.g. "NY.GDP.MKTP.CD" (GDP), "SP.POP.TOTL" (population)
// startYear, endYear: optional year range (0 = not specified)
func (c *Client) GetIndicator(ctx context.Context, countryCode, indicatorID string, startYear, endYear int) (*IndicatorResult, error) {
	countryCode = strings.ToLower(strings.TrimSpace(countryCode))
	indicatorID = strings.TrimSpace(indicatorID)
	if countryCode == "" {
		return nil, fmt.Errorf("country code is required")
	}
	if indicatorID == "" {
		return nil, fmt.Errorf("indicator ID is required")
	}

	cacheKey := fmt.Sprintf("%s|%s|%d|%d", countryCode, indicatorID, startYear, endYear)

	// Check cache
	c.mu.RLock()
	if entry, ok := c.cache[cacheKey]; ok && time.Now().Before(entry.expiresAt) {
		c.mu.RUnlock()
		return entry.result, nil
	}
	c.mu.RUnlock()

	// Build path with optional date range
	path := fmt.Sprintf("/country/%s/indicator/%s",
		url.PathEscape(countryCode),
		url.PathEscape(indicatorID),
	)
	if startYear > 0 && endYear > 0 {
		path += fmt.Sprintf("?date=%d:%d&per_page=100", startYear, endYear)
	} else {
		path += "?per_page=50&mrv=10" // most recent 10 values
	}

	var raw []json.RawMessage
	if err := c.getJSON(ctx, path, &raw); err != nil {
		return nil, err
	}
	if len(raw) < 2 {
		return nil, fmt.Errorf("not_found: no data for %s/%s", countryCode, indicatorID)
	}

	var entries []struct {
		Date      string          `json:"date"`
		Value     *float64        `json:"value"`
		Country   struct{ Value string `json:"value"` } `json:"country"`
		Indicator struct{ Value string `json:"value"`; ID string `json:"id"` } `json:"indicator"`
	}
	if err := json.Unmarshal(raw[1], &entries); err != nil {
		return nil, fmt.Errorf("decode indicator data: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("not_found: no data for %s/%s", countryCode, indicatorID)
	}

	data := make([]IndicatorEntry, len(entries))
	countryName := ""
	indicatorName := ""
	actualIndicatorID := ""
	for i, e := range entries {
		data[i] = IndicatorEntry{Year: e.Date, Value: e.Value}
		if countryName == "" {
			countryName = e.Country.Value
		}
		if indicatorName == "" {
			indicatorName = e.Indicator.Value
			actualIndicatorID = e.Indicator.ID
		}
	}

	result := &IndicatorResult{
		CountryID:   strings.ToUpper(countryCode),
		CountryName: countryName,
		Indicator:   indicatorName,
		IndicatorID: actualIndicatorID,
		Data:        data,
	}

	// Cache result
	c.mu.Lock()
	c.cache[cacheKey] = cacheEntry{result: result, expiresAt: time.Now().Add(time.Duration(cacheMinutes) * time.Minute)}
	c.mu.Unlock()

	return result, nil
}

// CommonIndicators returns a list of commonly used World Bank indicators.
func (c *Client) CommonIndicators() []Indicator {
	return []Indicator{
		{ID: "NY.GDP.MKTP.CD", Name: "GDP (current US$)", Source: "World Development Indicators"},
		{ID: "NY.GDP.PCAP.CD", Name: "GDP per capita (current US$)", Source: "World Development Indicators"},
		{ID: "NY.GDP.MKTP.KD.ZG", Name: "GDP growth (annual %)", Source: "World Development Indicators"},
		{ID: "SP.POP.TOTL", Name: "Population, total", Source: "World Development Indicators"},
		{ID: "SP.POP.GROW", Name: "Population growth (annual %)", Source: "World Development Indicators"},
		{ID: "FP.CPI.TOTL.ZG", Name: "Inflation, consumer prices (annual %)", Source: "World Development Indicators"},
		{ID: "SL.UEM.TOTL.ZS", Name: "Unemployment, total (% of labor force)", Source: "World Development Indicators"},
		{ID: "NE.TRD.GNFS.ZS", Name: "Trade (% of GDP)", Source: "World Development Indicators"},
		{ID: "BX.KLT.DINV.WD.GD.ZS", Name: "Foreign direct investment (% of GDP)", Source: "World Development Indicators"},
		{ID: "SE.ADT.LITR.ZS", Name: "Literacy rate, adult total (%)", Source: "World Development Indicators"},
		{ID: "SP.DYN.LE00.IN", Name: "Life expectancy at birth (years)", Source: "World Development Indicators"},
		{ID: "EN.ATM.CO2E.PC", Name: "CO2 emissions (metric tons per capita)", Source: "World Development Indicators"},
		{ID: "EG.USE.ELEC.KH.PC", Name: "Electric power consumption (kWh per capita)", Source: "World Development Indicators"},
		{ID: "IT.NET.USER.ZS", Name: "Individuals using the Internet (% of population)", Source: "World Development Indicators"},
		{ID: "GC.DOD.TOTL.GD.ZS", Name: "Central government debt (% of GDP)", Source: "World Development Indicators"},
	}
}
