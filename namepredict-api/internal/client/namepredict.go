// Package client wraps Agify, Genderize, and Nationalize APIs.
// Docs:
//   - https://agify.io — predict age from first name
//   - https://genderize.io — predict gender from first name
//   - https://nationalize.io — predict nationality from first name
// No API key required (free tier: 100 requests/day per IP).
// These three APIs share the same URL pattern: /{api}?name={name}[&country_id={cc}]
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

const defaultTimeout = 10 * time.Second

// AgeResult is the result from agify.io.
type AgeResult struct {
	Name  string `json:"name"`
	Age   *int   `json:"age"`   // nil if unknown
	Count int    `json:"count"` // number of data points
}

// GenderResult is the result from genderize.io.
type GenderResult struct {
	Name        string  `json:"name"`
	Gender      string  `json:"gender"`      // "male", "female", or "" if unknown
	Probability float64 `json:"probability"` // 0.0 - 1.0
	Count       int     `json:"count"`
}

// NationalityEntry is one nationality prediction.
type NationalityEntry struct {
	CountryID   string  `json:"country_id"`   // ISO 3166-1 alpha-2
	Probability float64 `json:"probability"`  // 0.0 - 1.0
}

// NationalityResult is the result from nationalize.io.
type NationalityResult struct {
	Name        string             `json:"name"`
	Count       int                `json:"count"`
	Countries   []NationalityEntry `json:"countries"` // sorted by probability desc
}

// FullPrediction combines all three predictions for one name.
type FullPrediction struct {
	Name        string             `json:"name"`
	Age         *AgeResult         `json:"age,omitempty"`
	Gender      *GenderResult      `json:"gender,omitempty"`
	Nationality *NationalityResult `json:"nationality,omitempty"`
}

// Client wraps all three name prediction APIs.
type Client struct {
	http *http.Client
	// Base URLs — separate so tests can override them
	agifyURL       string
	genderizeURL   string
	nationalizeURL string
}

// New returns a new name prediction client.
func New() *Client {
	return &Client{
		http:           &http.Client{Timeout: defaultTimeout},
		agifyURL:       "https://api.agify.io",
		genderizeURL:   "https://api.genderize.io",
		nationalizeURL: "https://api.nationalize.io",
	}
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, baseURL, name, countryID string, out interface{}) error {
	params := url.Values{"name": []string{strings.TrimSpace(name)}}
	if countryID != "" {
		params.Set("country_id", strings.ToUpper(strings.TrimSpace(countryID)))
	}
	u := baseURL + "?" + params.Encode()

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

	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("rate_limit: daily limit reached (100 free requests/day)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

// ── Public API ────────────────────────────────────────────────────────────────

// GetAge predicts the age for a given name.
// countryID is optional (ISO 3166-1 alpha-2, e.g. "US", "DE") for country-specific predictions.
func (c *Client) GetAge(ctx context.Context, name, countryID string) (*AgeResult, error) {
	if name = strings.TrimSpace(name); name == "" {
		return nil, fmt.Errorf("name is required")
	}
	var raw struct {
		Name  string `json:"name"`
		Age   *int   `json:"age"`
		Count int    `json:"count"`
	}
	if err := c.get(ctx, c.agifyURL, name, countryID, &raw); err != nil {
		return nil, err
	}
	return &AgeResult{
		Name:  raw.Name,
		Age:   raw.Age,
		Count: raw.Count,
	}, nil
}

// GetGender predicts the gender for a given name.
func (c *Client) GetGender(ctx context.Context, name, countryID string) (*GenderResult, error) {
	if name = strings.TrimSpace(name); name == "" {
		return nil, fmt.Errorf("name is required")
	}
	var raw struct {
		Name        string  `json:"name"`
		Gender      string  `json:"gender"`
		Probability float64 `json:"probability"`
		Count       int     `json:"count"`
	}
	if err := c.get(ctx, c.genderizeURL, name, countryID, &raw); err != nil {
		return nil, err
	}
	return &GenderResult{
		Name:        raw.Name,
		Gender:      raw.Gender,
		Probability: raw.Probability,
		Count:       raw.Count,
	}, nil
}

// GetNationality predicts the nationality for a given name.
func (c *Client) GetNationality(ctx context.Context, name string) (*NationalityResult, error) {
	if name = strings.TrimSpace(name); name == "" {
		return nil, fmt.Errorf("name is required")
	}
	var raw struct {
		Name    string `json:"name"`
		Count   int    `json:"count"`
		Country []struct {
			CountryID   string  `json:"country_id"`
			Probability float64 `json:"probability"`
		} `json:"country"`
	}
	if err := c.get(ctx, c.nationalizeURL, name, "", &raw); err != nil {
		return nil, err
	}

	entries := make([]NationalityEntry, len(raw.Country))
	for i, cc := range raw.Country {
		entries[i] = NationalityEntry{
			CountryID:   cc.CountryID,
			Probability: cc.Probability,
		}
	}
	return &NationalityResult{
		Name:      raw.Name,
		Count:     raw.Count,
		Countries: entries,
	}, nil
}

// GetAll returns age, gender, and nationality predictions concurrently.
// countryID is optional — used for age and gender (nationalize.io doesn't support it).
func (c *Client) GetAll(ctx context.Context, name, countryID string) (*FullPrediction, error) {
	if name = strings.TrimSpace(name); name == "" {
		return nil, fmt.Errorf("name is required")
	}

	var (
		mu          sync.Mutex
		ageResult   *AgeResult
		genderRes   *GenderResult
		natResult   *NationalityResult
		ageErr      error
		genderErr   error
		natErr      error
	)

	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		res, err := c.GetAge(ctx, name, countryID)
		mu.Lock()
		ageResult, ageErr = res, err
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		res, err := c.GetGender(ctx, name, countryID)
		mu.Lock()
		genderRes, genderErr = res, err
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		res, err := c.GetNationality(ctx, name)
		mu.Lock()
		natResult, natErr = res, err
		mu.Unlock()
	}()

	wg.Wait()

	// Return best-effort results; if all failed, return an error
	if ageErr != nil && genderErr != nil && natErr != nil {
		return nil, fmt.Errorf("all predictions failed: age=%v gender=%v nationality=%v", ageErr, genderErr, natErr)
	}

	pred := &FullPrediction{Name: name}
	if ageErr == nil {
		pred.Age = ageResult
	}
	if genderErr == nil {
		pred.Gender = genderRes
	}
	if natErr == nil {
		pred.Nationality = natResult
	}
	return pred, nil
}
