// Package client provides a GDPR enforcement tracker.
// Data source: GDPR Enforcement Tracker (enforcementtracker.com) — public dataset
// The tracker aggregates official DPA decisions from all EU/EEA member states.
// Data is cached locally and refreshed every 24 hours.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	// Primary source: GDPR enforcement tracker JSON export
	trackerURL    = "https://www.enforcementtracker.com/?export=json"
	cacheMaxAge   = 24 * time.Hour
)

// Fine is a single GDPR enforcement action / fine.
type Fine struct {
	ID             string  `json:"id"`
	Authority      string  `json:"authority"`      // supervisory authority e.g. "CNIL (France)"
	Country        string  `json:"country"`        // ISO code e.g. "FR"
	Entity         string  `json:"entity"`         // company/person fined
	EntityType     string  `json:"entityType"`     // "Private company", "Public body", etc.
	Sector         string  `json:"sector"`         // industry sector
	Amount         float64 `json:"amount"`         // fine amount in EUR
	AmountStr      string  `json:"amountStr"`      // formatted e.g. "€1,200,000"
	Year           int     `json:"year"`
	Date           string  `json:"date,omitempty"`
	Type           string  `json:"type"`           // "Fine", "Reprimand", "Warning", etc.
	Violation      string  `json:"violation"`      // GDPR articles violated
	Summary        string  `json:"summary"`        // brief description
	Source         string  `json:"source,omitempty"` // link to official decision
	ArticleViolated []string `json:"articlesViolated,omitempty"`
}

// SearchQuery for filtering fines.
type SearchQuery struct {
	Country    string  // ISO country code e.g. "DE", "FR"
	Authority  string  // DPA name substring
	Entity     string  // company name substring (case-insensitive)
	MinAmount  float64 // minimum fine amount in EUR
	MaxAmount  float64 // maximum fine amount in EUR
	YearFrom   int
	YearTo     int
	Article    string  // GDPR article e.g. "5", "6", "17"
	Sector     string  // industry sector substring
	MaxResults int
	Offset     int
}

// SearchResult is the paginated response.
type SearchResult struct {
	Total   int    `json:"total"`
	Offset  int    `json:"offset"`
	Results []Fine `json:"results"`
	Stats   *Stats `json:"stats,omitempty"`
}

// Stats provides aggregate statistics on a result set.
type Stats struct {
	TotalFines    int     `json:"totalFines"`
	TotalAmount   float64 `json:"totalAmountEUR"`
	AvgAmount     float64 `json:"avgAmountEUR"`
	MaxFine       float64 `json:"maxFineEUR"`
	TopCountry    string  `json:"topCountry"`
	TopAuthority  string  `json:"topAuthority"`
}

// Client holds the GDPR fines cache and provides query methods.
type Client struct {
	mu         sync.RWMutex
	fines      []Fine
	cachedAt   time.Time
	http       *http.Client
	trackerURL string // overridable for tests
}

// New creates a new GDPR client.
func New() *Client {
	return &Client{
		http:       &http.Client{Timeout: 30 * time.Second},
		trackerURL: trackerURL,
	}
}

// ensureCache loads (or refreshes) the fines dataset.
func (c *Client) ensureCache(ctx context.Context) error {
	c.mu.RLock()
	fresh := len(c.fines) > 0 && time.Since(c.cachedAt) < cacheMaxAge
	c.mu.RUnlock()
	if fresh {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	// Double-check after acquiring write lock
	if len(c.fines) > 0 && time.Since(c.cachedAt) < cacheMaxAge {
		return nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.trackerURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; GDPR-API/1.0)")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("enforcement tracker request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("enforcement tracker HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	fines, err := parseTrackerJSON(body)
	if err != nil {
		return err
	}

	c.fines = fines
	c.cachedAt = time.Now()
	return nil
}

// Search filters the fines dataset.
func (c *Client) Search(ctx context.Context, q SearchQuery) (*SearchResult, error) {
	if err := c.ensureCache(ctx); err != nil {
		return nil, err
	}

	if q.MaxResults <= 0 || q.MaxResults > 500 {
		q.MaxResults = 50
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	var matched []Fine
	for _, f := range c.fines {
		if q.Country != "" && !strings.EqualFold(f.Country, q.Country) {
			continue
		}
		if q.Authority != "" && !strings.Contains(strings.ToLower(f.Authority), strings.ToLower(q.Authority)) {
			continue
		}
		if q.Entity != "" && !strings.Contains(strings.ToLower(f.Entity), strings.ToLower(q.Entity)) {
			continue
		}
		if q.MinAmount > 0 && f.Amount < q.MinAmount {
			continue
		}
		if q.MaxAmount > 0 && f.Amount > q.MaxAmount {
			continue
		}
		if q.YearFrom > 0 && f.Year < q.YearFrom {
			continue
		}
		if q.YearTo > 0 && f.Year > q.YearTo {
			continue
		}
		if q.Sector != "" && !strings.Contains(strings.ToLower(f.Sector), strings.ToLower(q.Sector)) {
			continue
		}
		if q.Article != "" {
			found := false
			for _, art := range f.ArticleViolated {
				if strings.Contains(art, q.Article) {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		matched = append(matched, f)
	}

	total := len(matched)

	// Apply pagination
	start := q.Offset
	if start > total {
		start = total
	}
	end := start + q.MaxResults
	if end > total {
		end = total
	}
	page := matched[start:end]

	// Compute stats on full matched set
	stats := computeStats(matched)

	return &SearchResult{
		Total:   total,
		Offset:  q.Offset,
		Results: page,
		Stats:   stats,
	}, nil
}

// GetTopFines returns the N largest fines overall or by country.
func (c *Client) GetTopFines(ctx context.Context, country string, n int) ([]Fine, error) {
	if n <= 0 || n > 100 {
		n = 10
	}
	result, err := c.Search(ctx, SearchQuery{
		Country:    country,
		MaxResults: n,
		// We'd want sorted by amount desc — handled by parseTrackerJSON which sorts on load
	})
	if err != nil {
		return nil, err
	}
	return result.Results, nil
}

// CacheInfo returns metadata about the loaded cache.
func (c *Client) CacheInfo() (count int, cachedAt time.Time) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.fines), c.cachedAt
}

// ── Raw JSON parsing ───────────────────────────────────────────────────────────

type trackerRecord struct {
	ID          string `json:"id"`
	Date        string `json:"date"`
	Country     string `json:"country"`
	Authority   string `json:"authority"`
	Sector      string `json:"sector"`
	Quoted      string `json:"quoted"`    // fine amount as string e.g. "1200000"
	Type        string `json:"type"`
	Violated    string `json:"violated"`  // e.g. "Art. 5, Art. 6"
	Controller  string `json:"controller"`
	Summary     string `json:"summary"`
	Source      string `json:"source"`
	EntityType  string `json:"controllerType"`
}

func parseTrackerJSON(body []byte) ([]Fine, error) {
	// The tracker exports as an array of records OR wrapped in an object
	// Try array first
	var records []trackerRecord
	if err := json.Unmarshal(body, &records); err != nil {
		// Try wrapped format {"data": [...]}
		var wrapper struct {
			Data []trackerRecord `json:"data"`
		}
		if err2 := json.Unmarshal(body, &wrapper); err2 != nil {
			return nil, fmt.Errorf("parse tracker JSON: %w", err)
		}
		records = wrapper.Data
	}

	fines := make([]Fine, 0, len(records))
	for _, r := range records {
		f := mapRecord(r)
		fines = append(fines, f)
	}

	// Sort by amount descending in-place
	for i := 1; i < len(fines); i++ {
		for j := i; j > 0 && fines[j].Amount > fines[j-1].Amount; j-- {
			fines[j], fines[j-1] = fines[j-1], fines[j]
		}
	}

	return fines, nil
}

func mapRecord(r trackerRecord) Fine {
	// Parse amount — remove non-numeric chars
	amountStr := strings.TrimSpace(r.Quoted)
	amount := parseAmount(amountStr)

	// Parse year from date
	year := 0
	if len(r.Date) >= 4 {
		for i, ch := range r.Date[:4] {
			if ch >= '0' && ch <= '9' {
				year = year*10 + int(ch-'0')
				_ = i
			}
		}
	}

	// Parse articles violated
	articles := parseArticles(r.Violated)

	// Format amount string
	amountFormatted := ""
	if amount > 0 {
		amountFormatted = formatEUR(amount)
	} else {
		amountFormatted = r.Quoted
	}

	// Truncate summary
	summary := r.Summary
	if len(summary) > 400 {
		summary = summary[:397] + "..."
	}

	return Fine{
		ID:              r.ID,
		Authority:       r.Authority,
		Country:         r.Country,
		Entity:          r.Controller,
		EntityType:      r.EntityType,
		Sector:          r.Sector,
		Amount:          amount,
		AmountStr:       amountFormatted,
		Year:            year,
		Date:            r.Date,
		Type:            r.Type,
		Violation:       r.Violated,
		Summary:         summary,
		Source:          r.Source,
		ArticleViolated: articles,
	}
}

func parseAmount(s string) float64 {
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, ".", "")
	s = strings.TrimSpace(s)
	var amount float64
	fmt.Sscanf(s, "%f", &amount)
	return amount
}

func formatEUR(amount float64) string {
	if amount >= 1_000_000 {
		return fmt.Sprintf("€%.1fM", amount/1_000_000)
	}
	if amount >= 1_000 {
		return fmt.Sprintf("€%.0fK", amount/1_000)
	}
	return fmt.Sprintf("€%.0f", amount)
}

func parseArticles(violated string) []string {
	if violated == "" {
		return nil
	}
	parts := strings.Split(violated, ",")
	articles := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		// Extract just the article number from "Art. 5 GDPR" → "5"
		p = strings.TrimPrefix(p, "Art. ")
		p = strings.TrimPrefix(p, "Art.")
		p = strings.Split(p, " ")[0]
		p = strings.TrimSpace(p)
		if p != "" {
			articles = append(articles, p)
		}
	}
	return articles
}

func computeStats(fines []Fine) *Stats {
	if len(fines) == 0 {
		return &Stats{}
	}
	var total float64
	var maxFine float64
	countryCounts := map[string]int{}
	authorityCounts := map[string]int{}

	for _, f := range fines {
		total += f.Amount
		if f.Amount > maxFine {
			maxFine = f.Amount
		}
		if f.Country != "" {
			countryCounts[f.Country]++
		}
		if f.Authority != "" {
			authorityCounts[f.Authority]++
		}
	}

	topCountry := topKey(countryCounts)
	topAuthority := topKey(authorityCounts)

	return &Stats{
		TotalFines:   len(fines),
		TotalAmount:  total,
		AvgAmount:    total / float64(len(fines)),
		MaxFine:      maxFine,
		TopCountry:   topCountry,
		TopAuthority: topAuthority,
	}
}

func topKey(m map[string]int) string {
	best := ""
	bestCount := 0
	for k, v := range m {
		if v > bestCount {
			bestCount = v
			best = k
		}
	}
	return best
}
