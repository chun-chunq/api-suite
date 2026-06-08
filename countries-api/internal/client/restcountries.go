// Package client wraps the REST Countries v3.1 API.
// Docs: https://restcountries.com/#endpoints
// No API key required. Free and open source.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultBase    = "https://restcountries.com/v3.1"
	cacheTTL       = 24 * time.Hour // country data rarely changes
	defaultTimeout = 15 * time.Second
)

// Country is our normalized country representation.
type Country struct {
	Name         string            `json:"name"`
	OfficialName string            `json:"official_name"`
	CCA2         string            `json:"cca2"`          // 2-letter ISO code (e.g. "DE")
	CCA3         string            `json:"cca3"`          // 3-letter ISO code (e.g. "DEU")
	CCN3         string            `json:"ccn3,omitempty"` // numeric ISO code
	Region       string            `json:"region"`
	Subregion    string            `json:"subregion,omitempty"`
	Capital      []string          `json:"capital,omitempty"`
	Population   int64             `json:"population"`
	Area         float64           `json:"area,omitempty"` // km²
	Landlocked   bool              `json:"landlocked"`
	Independent  bool              `json:"independent"`
	Currencies   []Currency        `json:"currencies,omitempty"`
	Languages    map[string]string `json:"languages,omitempty"` // code → name
	CallingCodes []string          `json:"calling_codes,omitempty"`
	Timezones    []string          `json:"timezones,omitempty"`
	Borders      []string          `json:"borders,omitempty"` // CCA3 codes
	Continents   []string          `json:"continents,omitempty"`
	Flag         FlagInfo          `json:"flag"`
	Maps         MapLinks          `json:"maps,omitempty"`
	TLD          []string          `json:"tld,omitempty"` // top-level domains
	Translations map[string]string `json:"translations,omitempty"` // lang → common name
}

// Currency represents a currency used in a country.
type Currency struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Symbol string `json:"symbol,omitempty"`
}

// FlagInfo holds emoji and SVG URL for the country's flag.
type FlagInfo struct {
	Emoji  string `json:"emoji,omitempty"`
	SVG    string `json:"svg,omitempty"`
	PNG    string `json:"png,omitempty"`
	Alt    string `json:"alt,omitempty"`
}

// MapLinks holds Google Maps and OpenStreetMap URLs.
type MapLinks struct {
	GoogleMaps      string `json:"google_maps,omitempty"`
	OpenStreetMaps  string `json:"open_street_maps,omitempty"`
}

// rawCountry mirrors the REST Countries v3.1 JSON structure.
type rawCountry struct {
	Name struct {
		Common   string            `json:"common"`
		Official string            `json:"official"`
		NativeName map[string]struct {
			Official string `json:"official"`
			Common   string `json:"common"`
		} `json:"nativeName"`
	} `json:"name"`
	CCA2        string   `json:"cca2"`
	CCA3        string   `json:"cca3"`
	CCN3        string   `json:"ccn3"`
	Independent bool     `json:"independent"`
	Status      string   `json:"status"`
	Region      string   `json:"region"`
	Subregion   string   `json:"subregion"`
	Capital     []string `json:"capital"`
	Population  int64    `json:"population"`
	Area        float64  `json:"area"`
	Landlocked  bool     `json:"landlocked"`
	Borders     []string `json:"borders"`
	Continents  []string `json:"continents"`
	Timezones   []string `json:"timezones"`
	TLD         []string `json:"tld"`
	Currencies  map[string]struct {
		Name   string `json:"name"`
		Symbol string `json:"symbol"`
	} `json:"currencies"`
	Languages map[string]string `json:"languages"`
	IDD       struct {
		Root     string   `json:"root"`
		Suffixes []string `json:"suffixes"`
	} `json:"idd"`
	Flags struct {
		PNG string `json:"png"`
		SVG string `json:"svg"`
		Alt string `json:"alt"`
	} `json:"flags"`
	Flag  string `json:"flag"` // emoji
	Maps  struct {
		GoogleMaps     string `json:"googleMaps"`
		OpenStreetMaps string `json:"openStreetMaps"`
	} `json:"maps"`
	Translations map[string]struct {
		Official string `json:"official"`
		Common   string `json:"common"`
	} `json:"translations"`
}

// Client is the REST Countries client with in-memory caching.
type Client struct {
	http    *http.Client
	baseURL string

	mu        sync.RWMutex
	allCache  []Country
	cacheTime time.Time
}

// New returns a new REST Countries client.
func New() *Client {
	return &Client{
		http:    &http.Client{Timeout: defaultTimeout},
		baseURL: defaultBase,
	}
}

// ── Internal helpers ─────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, path string, fields []string) ([]rawCountry, error) {
	u := c.baseURL + path
	if len(fields) > 0 {
		u += "?fields=" + url.QueryEscape(strings.Join(fields, ","))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("not_found")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}

	var raws []rawCountry
	if err := json.NewDecoder(resp.Body).Decode(&raws); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return raws, nil
}

func normalize(r rawCountry) Country {
	// Build calling codes from IDD root + suffixes
	var callingCodes []string
	if r.IDD.Root != "" {
		if len(r.IDD.Suffixes) > 0 {
			for _, s := range r.IDD.Suffixes {
				callingCodes = append(callingCodes, r.IDD.Root+s)
			}
		} else {
			callingCodes = []string{r.IDD.Root}
		}
	}

	// Cap calling codes at 3 (some small island nations have many)
	if len(callingCodes) > 3 {
		callingCodes = callingCodes[:3]
	}

	// Build currencies list (sorted by code for consistency)
	var currencies []Currency
	for code, cur := range r.Currencies {
		currencies = append(currencies, Currency{
			Code:   code,
			Name:   cur.Name,
			Symbol: cur.Symbol,
		})
	}
	sort.Slice(currencies, func(i, j int) bool {
		return currencies[i].Code < currencies[j].Code
	})

	// Build translations map: lang → common name
	translations := make(map[string]string)
	for lang, t := range r.Translations {
		translations[lang] = t.Common
	}

	return Country{
		Name:         r.Name.Common,
		OfficialName: r.Name.Official,
		CCA2:         r.CCA2,
		CCA3:         r.CCA3,
		CCN3:         r.CCN3,
		Region:       r.Region,
		Subregion:    r.Subregion,
		Capital:      r.Capital,
		Population:   r.Population,
		Area:         r.Area,
		Landlocked:   r.Landlocked,
		Independent:  r.Independent,
		Currencies:   currencies,
		Languages:    r.Languages,
		CallingCodes: callingCodes,
		Timezones:    r.Timezones,
		Borders:      r.Borders,
		Continents:   r.Continents,
		TLD:          r.TLD,
		Flag:         FlagInfo{Emoji: r.Flag, SVG: r.Flags.SVG, PNG: r.Flags.PNG, Alt: r.Flags.Alt},
		Maps:         MapLinks{GoogleMaps: r.Maps.GoogleMaps, OpenStreetMaps: r.Maps.OpenStreetMaps},
		Translations: translations,
	}
}

// ── Public API ───────────────────────────────────────────────────────────────

// GetAll returns all countries (cached 24h).
func (c *Client) GetAll(ctx context.Context) ([]Country, error) {
	c.mu.RLock()
	if time.Since(c.cacheTime) < cacheTTL && len(c.allCache) > 0 {
		out := make([]Country, len(c.allCache))
		copy(out, c.allCache)
		c.mu.RUnlock()
		return out, nil
	}
	c.mu.RUnlock()

	raws, err := c.get(ctx, "/all", nil)
	if err != nil {
		return nil, err
	}

	countries := make([]Country, 0, len(raws))
	for _, r := range raws {
		countries = append(countries, normalize(r))
	}
	sort.Slice(countries, func(i, j int) bool {
		return countries[i].Name < countries[j].Name
	})

	c.mu.Lock()
	c.allCache = countries
	c.cacheTime = time.Now()
	c.mu.Unlock()

	out := make([]Country, len(countries))
	copy(out, countries)
	return out, nil
}

// GetByCode looks up a country by CCA2 or CCA3 code (e.g. "DE" or "DEU").
func (c *Client) GetByCode(ctx context.Context, code string) (*Country, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return nil, fmt.Errorf("code is required")
	}

	// Try cache first
	all, err := c.GetAll(ctx)
	if err == nil {
		for _, ct := range all {
			if ct.CCA2 == code || ct.CCA3 == code || ct.CCN3 == code {
				cp := ct
				return &cp, nil
			}
		}
		return nil, fmt.Errorf("not_found")
	}

	// Fallback to direct API call
	raws, err2 := c.get(ctx, "/alpha/"+url.PathEscape(code), nil)
	if err2 != nil {
		return nil, err2
	}
	if len(raws) == 0 {
		return nil, fmt.Errorf("not_found")
	}
	ct := normalize(raws[0])
	return &ct, nil
}

// SearchByName searches countries by common or official name (case-insensitive).
func (c *Client) SearchByName(ctx context.Context, name string, fullText bool) ([]Country, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	// Search from cache if available
	all, err := c.GetAll(ctx)
	if err == nil {
		query := strings.ToLower(name)
		var results []Country
		for _, ct := range all {
			if fullText {
				if strings.EqualFold(ct.Name, name) || strings.EqualFold(ct.OfficialName, name) {
					results = append(results, ct)
				}
			} else {
				if strings.Contains(strings.ToLower(ct.Name), query) ||
					strings.Contains(strings.ToLower(ct.OfficialName), query) {
					results = append(results, ct)
				}
			}
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("not_found")
		}
		return results, nil
	}

	// Fallback to API
	path := "/name/" + url.PathEscape(name)
	if fullText {
		path += "?fullText=true"
	}
	raws, err2 := c.get(ctx, path, nil)
	if err2 != nil {
		return nil, err2
	}
	results := make([]Country, 0, len(raws))
	for _, r := range raws {
		results = append(results, normalize(r))
	}
	return results, nil
}

// GetByRegion returns all countries in a region (e.g. "Europe", "Asia", "Americas").
func (c *Client) GetByRegion(ctx context.Context, region string) ([]Country, error) {
	region = strings.TrimSpace(region)
	if region == "" {
		return nil, fmt.Errorf("region is required")
	}

	all, err := c.GetAll(ctx)
	if err == nil {
		query := strings.ToLower(region)
		var results []Country
		for _, ct := range all {
			if strings.ToLower(ct.Region) == query || strings.ToLower(ct.Subregion) == query {
				results = append(results, ct)
			}
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("not_found")
		}
		return results, nil
	}

	raws, err2 := c.get(ctx, "/region/"+url.PathEscape(region), nil)
	if err2 != nil {
		return nil, err2
	}
	results := make([]Country, 0, len(raws))
	for _, r := range raws {
		results = append(results, normalize(r))
	}
	return results, nil
}

// GetByLanguage returns all countries that use a given language code or name.
func (c *Client) GetByLanguage(ctx context.Context, language string) ([]Country, error) {
	all, err := c.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	query := strings.ToLower(strings.TrimSpace(language))
	var results []Country
	for _, ct := range all {
		for code, name := range ct.Languages {
			if strings.ToLower(code) == query || strings.Contains(strings.ToLower(name), query) {
				results = append(results, ct)
				break
			}
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("not_found")
	}
	return results, nil
}

// GetByCurrency returns all countries that use a given currency code (e.g. "EUR").
func (c *Client) GetByCurrency(ctx context.Context, currency string) ([]Country, error) {
	all, err := c.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	query := strings.ToUpper(strings.TrimSpace(currency))
	var results []Country
	for _, ct := range all {
		for _, cur := range ct.Currencies {
			if cur.Code == query {
				results = append(results, ct)
				break
			}
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("not_found")
	}
	return results, nil
}
