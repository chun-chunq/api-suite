// Package client wraps the SEC EDGAR REST API.
// EDGAR = Electronic Data Gathering, Analysis, and Retrieval
// Docs: https://www.sec.gov/developer
// Auth: none required — public open data
// Rate limit: 10 req/sec (be polite — include User-Agent with contact email)
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

const (
	defaultBaseURL     = "https://data.sec.gov"
	searchBaseURL      = "https://efts.sec.gov"
	companiesURL       = "https://www.sec.gov/files/company_tickers.json"
)

// Company is a US public company registered with the SEC.
type Company struct {
	CIK        string `json:"cik"`        // 10-digit Central Index Key (padded)
	Name       string `json:"name"`
	Ticker     string `json:"ticker,omitempty"`
	Exchange   string `json:"exchange,omitempty"` // "NYSE", "NASDAQ", etc.
	SIC        string `json:"sic,omitempty"`      // Standard Industrial Classification code
	SICDesc    string `json:"sicDescription,omitempty"`
	StateInc   string `json:"stateOfIncorporation,omitempty"`
	FiscalYearEnd string `json:"fiscalYearEnd,omitempty"` // e.g. "1231" = Dec 31
	URL        string `json:"url"`
}

// Filing is a single SEC filing (10-K, 10-Q, 8-K, etc.)
type Filing struct {
	AccessionNum  string `json:"accessionNumber"`
	FilingDate    string `json:"filingDate"`
	Form          string `json:"form"`        // "10-K", "10-Q", "8-K", "DEF 14A", etc.
	ReportDate    string `json:"reportDate,omitempty"`
	Description   string `json:"description,omitempty"`
	DocumentURL   string `json:"documentUrl"` // link to filing index
	Size          int64  `json:"size,omitempty"`
	IsXBRL        bool   `json:"isXBRL"`
}

// FinancialFact is a single financial data point from XBRL filings.
type FinancialFact struct {
	Concept       string  `json:"concept"`       // e.g. "us-gaap/NetIncomeLoss"
	Label         string  `json:"label"`
	Value         float64 `json:"value"`
	Unit          string  `json:"unit"`          // "USD", "shares", etc.
	Period        string  `json:"period"`        // e.g. "2023-12-31" or "2023-01-01/2023-12-31"
	Form          string  `json:"form"`
	FiledDate     string  `json:"filed"`
	AccessionNum  string  `json:"accessionNumber"`
}

// Client wraps SEC EDGAR APIs.
type Client struct {
	http        *http.Client
	baseURL     string
	searchURL   string
	userAgent   string // required by SEC: "Name Email"
}

// New creates a new EDGAR client.
// userAgent should be "YourName contact@email.com" (SEC requirement)
func New(userAgent string) *Client {
	if userAgent == "" {
		userAgent = "Research-API contact@example.com"
	}
	return &Client{
		http:      &http.Client{Timeout: 20 * time.Second},
		baseURL:   defaultBaseURL,
		searchURL: searchBaseURL,
		userAgent: userAgent,
	}
}

func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	// SEC asks for a small delay between requests — handled by callers

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("EDGAR request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("EDGAR HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// SearchCompanies full-text searches SEC company names.
func (c *Client) SearchCompanies(ctx context.Context, query string, maxResults int) ([]Company, int, error) {
	if query == "" {
		return nil, 0, fmt.Errorf("query is required")
	}
	if maxResults <= 0 || maxResults > 100 {
		maxResults = 20
	}

	params := url.Values{}
	params.Set("q", `"`+query+`"`)
	params.Set("dateRange", "custom")
	params.Set("category", "form-type")
	// Use the full-text search for company names via submissions search
	u := fmt.Sprintf("%s/LATEST/search-index?q=%s&entity=%s&hits.hits.total.value=true&hits.hits._source.period_of_report=true&hits.hits._source.entity_name=true&hits.hits._source.file_date=true&dateRange=custom&forms=10-K",
		c.searchURL, url.QueryEscape(query), url.QueryEscape(query))
	_ = u

	// Better: use the company search endpoint
	searchU := fmt.Sprintf("%s/LATEST/search-index?q=%s&entity=%s",
		c.searchURL,
		url.QueryEscape(`"`+query+`"`),
		url.QueryEscape(query))

	body, err := c.get(ctx, searchU)
	if err != nil || body == nil {
		// Fall back to static tickers list for company name matching
		return c.searchTickerList(ctx, query, maxResults)
	}

	var raw edgarSearchResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return c.searchTickerList(ctx, query, maxResults)
	}

	companies := make([]Company, 0, len(raw.Hits.Hits))
	seen := map[string]bool{}
	for _, hit := range raw.Hits.Hits {
		cik := normalizeCIK(hit.Source.EntityID)
		if seen[cik] {
			continue
		}
		seen[cik] = true
		companies = append(companies, Company{
			CIK:  cik,
			Name: hit.Source.EntityName,
			URL:  fmt.Sprintf("https://www.sec.gov/cgi-bin/browse-edgar?action=getcompany&CIK=%s&type=10-K&dateb=&owner=include&count=10", cik),
		})
		if len(companies) >= maxResults {
			break
		}
	}
	return companies, raw.Hits.Total.Value, nil
}

// searchTickerList is a fallback that searches the static company tickers file.
func (c *Client) searchTickerList(ctx context.Context, query string, maxResults int) ([]Company, int, error) {
	body, err := c.get(ctx, companiesURL)
	if err != nil || body == nil {
		return nil, 0, fmt.Errorf("could not load company tickers")
	}

	var raw map[string]tickerEntry
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, 0, err
	}

	queryLower := strings.ToLower(query)
	var matches []Company
	for _, entry := range raw {
		if strings.Contains(strings.ToLower(entry.Title), queryLower) ||
			strings.EqualFold(entry.Ticker, query) {
			cik := normalizeCIK(fmt.Sprintf("%d", entry.CIKStr))
			matches = append(matches, Company{
				CIK:    cik,
				Name:   entry.Title,
				Ticker: entry.Ticker,
				URL:    fmt.Sprintf("https://www.sec.gov/cgi-bin/browse-edgar?action=getcompany&CIK=%s&type=10-K&dateb=&owner=include&count=10", cik),
			})
		}
		if len(matches) >= maxResults {
			break
		}
	}
	return matches, len(matches), nil
}

// GetCompanyProfile fetches company details by CIK.
func (c *Client) GetCompanyProfile(ctx context.Context, cik string) (*Company, error) {
	cik = normalizeCIK(cik)
	if cik == "" {
		return nil, fmt.Errorf("CIK is required")
	}

	u := fmt.Sprintf("%s/submissions/CIK%s.json", c.baseURL, cik)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, nil
	}

	var raw edgarSubmissions
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode submissions: %w", err)
	}

	co := mapCompanyProfile(raw)
	return &co, nil
}

// GetFilings fetches recent filings for a company by CIK.
func (c *Client) GetFilings(ctx context.Context, cik string, formType string, maxResults int) ([]Filing, error) {
	cik = normalizeCIK(cik)
	if cik == "" {
		return nil, fmt.Errorf("CIK is required")
	}
	if maxResults <= 0 || maxResults > 100 {
		maxResults = 20
	}

	u := fmt.Sprintf("%s/submissions/CIK%s.json", c.baseURL, cik)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, nil
	}

	var raw edgarSubmissions
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode submissions: %w", err)
	}

	return mapFilings(raw, cik, formType, maxResults), nil
}

// GetFinancialFacts fetches structured financial data (XBRL) for a company.
// concept: e.g. "us-gaap/NetIncomeLoss", "us-gaap/Revenues", "us-gaap/Assets"
func (c *Client) GetFinancialFacts(ctx context.Context, cik, concept string, maxResults int) ([]FinancialFact, error) {
	cik = normalizeCIK(cik)
	if cik == "" || concept == "" {
		return nil, fmt.Errorf("CIK and concept are required")
	}
	if maxResults <= 0 || maxResults > 200 {
		maxResults = 20
	}

	// concept format: "us-gaap/NetIncomeLoss" → taxonomy="us-gaap", concept="NetIncomeLoss"
	parts := strings.SplitN(concept, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("concept must be in 'taxonomy/ConceptName' format e.g. 'us-gaap/NetIncomeLoss'")
	}
	taxonomy, conceptName := parts[0], parts[1]

	u := fmt.Sprintf("%s/api/xbrl/companyconcept/CIK%s/%s/%s.json",
		c.baseURL, cik, taxonomy, conceptName)
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, nil
	}

	var raw edgarConceptResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode concept: %w", err)
	}

	return mapFacts(raw, concept, maxResults), nil
}

// ── Raw SEC EDGAR JSON structures ──────────────────────────────────────────────

type tickerEntry struct {
	CIKStr int    `json:"cik_str"`
	Ticker string `json:"ticker"`
	Title  string `json:"title"`
}

type edgarSearchResponse struct {
	Hits struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []struct {
			Source struct {
				EntityID   string `json:"entity_id"`
				EntityName string `json:"entity_name"`
				FormType   string `json:"form_type"`
				FileDate   string `json:"file_date"`
			} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

type edgarSubmissions struct {
	CIK          string `json:"cik"`
	EntityType   string `json:"entityType"`
	SIC          string `json:"sic"`
	SICDesc      string `json:"sicDescription"`
	Name         string `json:"name"`
	Tickers      []string `json:"tickers"`
	Exchanges    []string `json:"exchanges"`
	StateOfInc   string `json:"stateOfIncorporation"`
	FiscalYearEnd string `json:"fiscalYearEnd"`
	Filings      struct {
		Recent struct {
			AccessionNumber []string `json:"accessionNumber"`
			FilingDate      []string `json:"filingDate"`
			Form            []string `json:"form"`
			ReportDate      []string `json:"reportDate"`
			Size            []int64  `json:"size"`
			IsXBRL          []int    `json:"isXBRL"`
			PrimaryDocument []string `json:"primaryDocument"`
			PrimaryDocDescription []string `json:"primaryDocDescription"`
		} `json:"recent"`
	} `json:"filings"`
}

type edgarConceptResponse struct {
	CIK     int    `json:"cik"`
	Concept string `json:"concept"`
	Label   string `json:"label"`
	Units   map[string][]struct {
		End    string  `json:"end"`
		Start  string  `json:"start"`
		Val    float64 `json:"val"`
		Form   string  `json:"form"`
		Filed  string  `json:"filed"`
		Accn   string  `json:"accn"`
	} `json:"units"`
}

func mapCompanyProfile(r edgarSubmissions) Company {
	ticker := ""
	exchange := ""
	if len(r.Tickers) > 0 {
		ticker = r.Tickers[0]
	}
	if len(r.Exchanges) > 0 {
		exchange = r.Exchanges[0]
	}
	cik := normalizeCIK(r.CIK)
	return Company{
		CIK:           cik,
		Name:          r.Name,
		Ticker:        ticker,
		Exchange:      exchange,
		SIC:           r.SIC,
		SICDesc:       r.SICDesc,
		StateInc:      r.StateOfInc,
		FiscalYearEnd: r.FiscalYearEnd,
		URL:           fmt.Sprintf("https://www.sec.gov/cgi-bin/browse-edgar?action=getcompany&CIK=%s&type=10-K&dateb=&owner=include&count=10", cik),
	}
}

func mapFilings(r edgarSubmissions, cik, formFilter string, maxResults int) []Filing {
	recent := r.Filings.Recent
	n := len(recent.AccessionNumber)
	filings := make([]Filing, 0, maxResults)

	for i := 0; i < n && len(filings) < maxResults; i++ {
		form := ""
		if i < len(recent.Form) {
			form = recent.Form[i]
		}
		if formFilter != "" && !strings.EqualFold(form, formFilter) {
			continue
		}
		accn := ""
		if i < len(recent.AccessionNumber) {
			accn = recent.AccessionNumber[i]
		}
		filingDate := ""
		if i < len(recent.FilingDate) {
			filingDate = recent.FilingDate[i]
		}
		reportDate := ""
		if i < len(recent.ReportDate) {
			reportDate = recent.ReportDate[i]
		}
		desc := ""
		if i < len(recent.PrimaryDocDescription) {
			desc = recent.PrimaryDocDescription[i]
		}
		var size int64
		if i < len(recent.Size) {
			size = recent.Size[i]
		}
		isXBRL := false
		if i < len(recent.IsXBRL) {
			isXBRL = recent.IsXBRL[i] == 1
		}

		// Build document URL
		accnForURL := strings.ReplaceAll(accn, "-", "")
		docURL := fmt.Sprintf("https://www.sec.gov/Archives/edgar/data/%s/%s/", strings.TrimLeft(cik, "0"), accnForURL)

		filings = append(filings, Filing{
			AccessionNum: accn,
			FilingDate:   filingDate,
			Form:         form,
			ReportDate:   reportDate,
			Description:  desc,
			DocumentURL:  docURL,
			Size:         size,
			IsXBRL:       isXBRL,
		})
	}
	return filings
}

func mapFacts(r edgarConceptResponse, concept string, maxResults int) []FinancialFact {
	facts := make([]FinancialFact, 0, maxResults)

	for unit, entries := range r.Units {
		// Take most recent entries first (they come sorted newest last in EDGAR)
		for i := len(entries) - 1; i >= 0 && len(facts) < maxResults; i-- {
			e := entries[i]
			period := e.End
			if e.Start != "" {
				period = e.Start + "/" + e.End
			}
			facts = append(facts, FinancialFact{
				Concept:      concept,
				Label:        r.Label,
				Value:        e.Val,
				Unit:         unit,
				Period:       period,
				Form:         e.Form,
				FiledDate:    e.Filed,
				AccessionNum: e.Accn,
			})
		}
	}
	return facts
}

func normalizeCIK(cik string) string {
	cik = strings.TrimSpace(cik)
	if cik == "" {
		return ""
	}
	// Remove leading "CIK" prefix if present
	cik = strings.TrimPrefix(strings.ToUpper(cik), "CIK")
	// Pad to 10 digits
	for len(cik) < 10 {
		cik = "0" + cik
	}
	if len(cik) > 10 {
		cik = cik[len(cik)-10:]
	}
	return cik
}
