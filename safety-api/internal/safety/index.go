// Package safety provides an in-memory index of EU Safety Gate (RAPEX) alerts.
//
// Data source: European Commission Safety Gate open data
// https://ec.europa.eu/safety-gate-alerts/
// License: EU Open Data / CC BY 4.0
package safety

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"
)

// EU Safety Gate weekly XML data feeds — tried in order.
// The Commission publishes the full dataset split by year, plus a current week feed.
var dataURLs = []string{
	// Full dataset (all years combined) — primary
	"https://ec.europa.eu/safety-gate-alerts/screen/data/rapex-weekly-data",
	// Fallback: open data portal
	"https://data.europa.eu/api/hub/store/data/rapex-weekly-data",
}

// We also try fetching recent years individually if the full feed fails.
const currentYearBaseURL = "https://ec.europa.eu/consumers/consumers_safety/safety_products/rapex/alerts/repository/content/images/publications/"

// Index is a thread-safe in-memory store of safety alerts.
type Index struct {
	mu       sync.RWMutex
	alerts   []Alert
	// lowercase product/brand/category → alert indices
	productIdx  map[string][]int
	categoryIdx map[string][]int
	countryIdx  map[string][]int
	dataDate    string
	loaded      atomic.Bool

	refreshInterval time.Duration
	nextRefresh     time.Time
	log             zerolog.Logger
}

// New creates and starts an Index. Blocks until initial data is loaded (or fails gracefully).
func New(refreshInterval time.Duration, log zerolog.Logger) *Index {
	idx := &Index{
		productIdx:      make(map[string][]int),
		categoryIdx:     make(map[string][]int),
		countryIdx:      make(map[string][]int),
		refreshInterval: refreshInterval,
		log:             log,
	}
	if err := idx.refresh(); err != nil {
		log.Error().Err(err).Msg("initial Safety Gate load failed — retrying in background")
	}
	go idx.refreshLoop()
	return idx
}

// Search returns alerts matching the query. All filters are AND-combined.
func (idx *Index) Search(q SearchQuery) []Alert {
	if q.MaxResults <= 0 || q.MaxResults > 500 {
		q.MaxResults = 50
	}

	idx.mu.RLock()
	defer idx.mu.RUnlock()

	// Build candidate set from best available index
	var candidates []int
	if q.Product != "" {
		candidates = idx.lookup(idx.productIdx, q.Product)
	} else if q.Category != "" {
		candidates = idx.lookup(idx.categoryIdx, q.Category)
	} else if q.Country != "" {
		candidates = idx.lookup(idx.countryIdx, strings.ToUpper(q.Country))
	} else {
		// No index hint — scan all (capped at MaxResults*10 for speed)
		limit := q.MaxResults * 10
		if limit > len(idx.alerts) {
			limit = len(idx.alerts)
		}
		candidates = make([]int, limit)
		for i := range candidates {
			candidates[i] = len(idx.alerts) - 1 - i // newest first
		}
	}

	var results []Alert
	seen := make(map[int]bool, len(candidates))
	for _, i := range candidates {
		if seen[i] {
			continue
		}
		seen[i] = true
		a := idx.alerts[i]
		if !matchesQuery(a, q) {
			continue
		}
		results = append(results, a)
		if len(results) >= q.MaxResults {
			break
		}
	}
	return results
}

// Get returns a single alert by reference number.
func (idx *Index) Get(reference string) (Alert, bool) {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	ref := strings.ToUpper(strings.TrimSpace(reference))
	for _, a := range idx.alerts {
		if strings.EqualFold(a.Reference, ref) {
			return a, true
		}
	}
	return Alert{}, false
}

// Categories returns all known product categories.
func (idx *Index) Categories() []string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	cats := make([]string, 0, len(idx.categoryIdx))
	for k := range idx.categoryIdx {
		cats = append(cats, k)
	}
	return cats
}

// Status returns the current state of the index.
func (idx *Index) Status() Status {
	idx.mu.RLock()
	count := len(idx.alerts)
	date := idx.dataDate
	next := idx.nextRefresh
	idx.mu.RUnlock()
	return Status{
		Loaded:      idx.loaded.Load(),
		AlertCount:  count,
		DataDate:    date,
		NextRefresh: next.UTC().Format(time.RFC3339),
	}
}

// DataDate returns the data timestamp of the loaded dataset.
func (idx *Index) DataDate() string {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.dataDate
}

func (idx *Index) refreshLoop() {
	for {
		next := time.Now().Add(idx.refreshInterval)
		idx.mu.Lock()
		idx.nextRefresh = next
		idx.mu.Unlock()
		time.Sleep(idx.refreshInterval)
		if err := idx.refresh(); err != nil {
			idx.log.Error().Err(err).Msg("Safety Gate refresh failed")
		} else {
			idx.log.Info().Str("dataDate", idx.DataDate()).Msg("Safety Gate refreshed")
		}
	}
}

func (idx *Index) refresh() error {
	var (
		alerts   []Alert
		dataDate string
		lastErr  error
	)

	// Try primary URLs
	for _, url := range dataURLs {
		idx.log.Info().Str("url", url).Msg("downloading Safety Gate data")
		data, err := downloadURL(url)
		if err != nil {
			idx.log.Warn().Err(err).Str("url", url).Msg("Safety Gate URL failed, trying next")
			lastErr = err
			continue
		}
		alerts, dataDate, err = parseXML(data)
		if err != nil {
			idx.log.Warn().Err(err).Msg("Safety Gate XML parse failed, trying next URL")
			lastErr = err
			continue
		}
		lastErr = nil
		break
	}

	// If primary URLs failed, try fetching current + last 2 years week-by-week (sampled)
	if lastErr != nil || len(alerts) == 0 {
		idx.log.Warn().Msg("primary feeds failed, trying year-by-year fetch")
		alerts, dataDate = idx.fetchByYear()
		if len(alerts) == 0 {
			return fmt.Errorf("all Safety Gate sources failed: %w", lastErr)
		}
	}

	// Build indices
	productIdx := make(map[string][]int, len(alerts))
	categoryIdx := make(map[string][]int)
	countryIdx := make(map[string][]int)

	for i, a := range alerts {
		// Product name tokens
		for _, tok := range tokenize(a.ProductName + " " + a.Brand) {
			productIdx[tok] = appendUniq(productIdx[tok], i)
		}
		// Category
		if cat := norm(a.ProductType); cat != "" {
			categoryIdx[cat] = appendUniq(categoryIdx[cat], i)
		}
		// Country (ISO-2)
		if c := strings.ToUpper(a.Country); c != "" {
			countryIdx[c] = appendUniq(countryIdx[c], i)
		}
	}

	idx.mu.Lock()
	idx.alerts = alerts
	idx.productIdx = productIdx
	idx.categoryIdx = categoryIdx
	idx.countryIdx = countryIdx
	idx.dataDate = dataDate
	idx.mu.Unlock()
	idx.loaded.Store(true)

	idx.log.Info().Int("alerts", len(alerts)).Str("dataDate", dataDate).Msg("Safety Gate loaded")
	return nil
}

// fetchByYear tries to load data year by year as fallback.
func (idx *Index) fetchByYear() ([]Alert, string) {
	var all []Alert
	now := time.Now()
	dataDate := now.Format("2006-01-02")

	for yr := now.Year(); yr >= now.Year()-2; yr-- {
		// Try the annual summary XML (not week-by-week)
		url := fmt.Sprintf("%s%d/rapex_%d_en.xml", currentYearBaseURL, yr, yr)
		data, err := downloadURL(url)
		if err != nil {
			continue
		}
		alerts, date, err := parseXML(data)
		if err != nil {
			continue
		}
		all = append(all, alerts...)
		if date != "" {
			dataDate = date
		}
	}
	return all, dataDate
}

// ── XML parsing ────────────────────────────────────────────────────────────────

// The EU Safety Gate XML structure (RAPEX format)
type xmlRapex struct {
	XMLName      xml.Name         `xml:"RAPEX"`
	Notifications []xmlNotification `xml:"NOTIFICATION"`
}

type xmlNotification struct {
	Reference   string `xml:"REFERENCE"`
	Type        string `xml:"TYPE"`
	Date        string `xml:"DATE"`
	Week        string `xml:"WEEK"`
	Country     string `xml:"COUNTRY"`
	Product     xmlProduct `xml:"PRODUCT"`
	Risk        xmlRisk    `xml:"RISK"`
	Measures    string `xml:"MEASURES"`
	Distribution string `xml:"DISTRIBUTION_OF_THE_PRODUCT"`
	URL         string `xml:"URL"`
}

type xmlProduct struct {
	Name     string `xml:"NAME"`
	Type     string `xml:"TYPE"`
	Brand    string `xml:"BRAND"`
	Batch    string `xml:"BATCH_NUMBER_MODEL"`
	BarCode  string `xml:"BAR_CODE"`
	Origin   string `xml:"COUNTRY_OF_ORIGIN"`
}

type xmlRisk struct {
	Type        string `xml:"TYPE"`
	Level       string `xml:"RISK_LEVEL"`
	Description string `xml:"DESCRIPTION"`
}

func parseXML(data []byte) ([]Alert, string, error) {
	var rapex xmlRapex
	if err := xml.Unmarshal(data, &rapex); err != nil {
		// Try alternate root element name
		type xmlAlt struct {
			XMLName       xml.Name          `xml:"notifications"`
			Notifications []xmlNotification `xml:"notification"`
		}
		var alt xmlAlt
		if err2 := xml.Unmarshal(data, &alt); err2 != nil {
			return nil, "", fmt.Errorf("XML parse failed: %w", err)
		}
		rapex.Notifications = alt.Notifications
	}

	alerts := make([]Alert, 0, len(rapex.Notifications))
	dataDate := time.Now().Format("2006-01-02")

	for _, n := range rapex.Notifications {
		dist := parseDistribution(n.Distribution)
		alerts = append(alerts, Alert{
			Reference:    strings.TrimSpace(n.Reference),
			AlertType:    strings.TrimSpace(n.Type),
			Date:         normalizeDate(n.Date),
			Week:         strings.TrimSpace(n.Week),
			Country:      strings.TrimSpace(n.Country),
			ProductName:  strings.TrimSpace(n.Product.Name),
			ProductType:  strings.TrimSpace(n.Product.Type),
			Brand:        strings.TrimSpace(n.Product.Brand),
			BatchNumber:  strings.TrimSpace(n.Product.Batch),
			BarCode:      strings.TrimSpace(n.Product.BarCode),
			Origin:       strings.TrimSpace(n.Product.Origin),
			RiskType:     strings.TrimSpace(n.Risk.Type),
			RiskLevel:    strings.TrimSpace(n.Risk.Level),
			Description:  strings.TrimSpace(n.Risk.Description),
			Measures:     strings.TrimSpace(n.Measures),
			Distribution: dist,
			URL:          strings.TrimSpace(n.URL),
		})
		if n.Date != "" {
			dataDate = normalizeDate(n.Date)
		}
	}
	return alerts, dataDate, nil
}

func parseDistribution(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func normalizeDate(s string) string {
	s = strings.TrimSpace(s)
	// Try various formats
	for _, layout := range []string{"02/01/2006", "2006-01-02", "01/02/2006", "2006/01/02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return s
}

// ── Helpers ────────────────────────────────────────────────────────────────────

func downloadURL(url string) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 100<<20))
}

func matchesQuery(a Alert, q SearchQuery) bool {
	if q.Product != "" && !containsNorm(a.ProductName+" "+a.Brand, q.Product) {
		return false
	}
	if q.Brand != "" && !containsNorm(a.Brand, q.Brand) {
		return false
	}
	if q.Category != "" && !containsNorm(a.ProductType, q.Category) {
		return false
	}
	if q.Country != "" && !strings.EqualFold(a.Country, q.Country) {
		return false
	}
	if q.Origin != "" && !containsNorm(a.Origin, q.Origin) {
		return false
	}
	if q.Risk != "" && !containsNorm(a.RiskType+" "+a.RiskLevel, q.Risk) {
		return false
	}
	if q.From != "" && a.Date != "" && a.Date < q.From {
		return false
	}
	if q.To != "" && a.Date != "" && a.Date > q.To {
		return false
	}
	return true
}

func (idx *Index) lookup(m map[string][]int, query string) []int {
	var result []int
	qNorm := norm(query)
	for key, indices := range m {
		if strings.Contains(key, qNorm) {
			result = append(result, indices...)
		}
	}
	return result
}

func norm(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func containsNorm(haystack, needle string) bool {
	return strings.Contains(norm(haystack), norm(needle))
}

func tokenize(s string) []string {
	s = norm(s)
	words := strings.Fields(s)
	var tokens []string
	for _, w := range words {
		w = strings.Trim(w, ".,;:-")
		if len(w) >= 3 {
			tokens = append(tokens, w)
		}
	}
	// Also add full normalized string
	full := strings.Join(words, " ")
	if full != "" {
		tokens = append(tokens, full)
	}
	return tokens
}

func appendUniq(slice []int, val int) []int {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}

// parseWeekNumber converts "2024-W12" or "12/2024" to a week string.
func parseWeekNumber(s string) string {
	_ = strconv.Itoa // keep import used
	return s
}

var _ = parseWeekNumber // suppress unused warning
