// Package client wraps the OpenAlex REST API.
// OpenAlex is a free, fully open index of scholarly works, authors, and institutions.
// Docs: https://docs.openalex.org/
// License: CC0 (public domain) — fully free
// Rate limit: polite pool = 100,000 req/day (no auth) — use email param for higher limits
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

const defaultBaseURL = "https://api.openalex.org"

// Work represents a scholarly publication.
type Work struct {
	ID             string    `json:"id"`             // OpenAlex ID e.g. "W2741809807"
	DOI            string    `json:"doi,omitempty"`
	Title          string    `json:"title"`
	Abstract       string    `json:"abstract,omitempty"`
	PublicationYear int      `json:"publicationYear,omitempty"`
	PublicationDate string   `json:"publicationDate,omitempty"`
	Type           string    `json:"type,omitempty"` // "journal-article", "book", "dataset", etc.
	CitedByCount   int       `json:"citedByCount"`
	OpenAccess     bool      `json:"openAccess"`
	OAUrl          string    `json:"oaUrl,omitempty"`   // open access PDF URL if available
	Journal        string    `json:"journal,omitempty"`
	Authors        []Author  `json:"authors,omitempty"`
	Concepts       []Concept `json:"concepts,omitempty"`
	URL            string    `json:"url"`
}

// Author is a contributor to a scholarly work.
type Author struct {
	ID          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Orcid       string `json:"orcid,omitempty"`
	Institution string `json:"institution,omitempty"`
}

// Concept is a topic/field tag on a work (from OpenAlex's ML tagging).
type Concept struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Score float64 `json:"score"` // 0.0–1.0 relevance
	Level int     `json:"level"` // 0=broadest, 5=most specific
}

// Institution represents a research institution/university.
type Institution struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Country     string   `json:"country,omitempty"`
	Type        string   `json:"type,omitempty"` // "education", "healthcare", "company", etc.
	Homepage    string   `json:"homepage,omitempty"`
	WorksCount  int      `json:"worksCount"`
	CitedCount  int      `json:"citedCount"`
	ROR         string   `json:"ror,omitempty"` // Research Organization Registry ID
}

// WorkSearchQuery controls work search parameters.
type WorkSearchQuery struct {
	Query         string   // free-text search
	Author        string   // author name filter
	Institution   string   // institution name filter
	ConceptID     string   // OpenAlex concept ID filter
	Year          int      // publication year filter
	YearFrom      int      // min publication year
	YearTo        int      // max publication year
	OpenAccessOnly bool    // only return open access works
	Type          string   // work type filter
	SortBy        string   // "cited_by_count", "publication_date" (default: relevance)
	MaxResults    int      // 1–200, default 25
	Page          int      // 1-based
}

// SearchResult is a paginated list of works.
type SearchResult struct {
	Total      int    `json:"total"`
	Page       int    `json:"page"`
	PerPage    int    `json:"perPage"`
	Results    []Work `json:"results"`
}

// Client wraps the OpenAlex API.
type Client struct {
	http    *http.Client
	baseURL string
	email   string // polite pool: higher rate limits when you provide email
}

// New creates a new OpenAlex client.
// email is optional but increases rate limits (polite pool).
func New(email string) *Client {
	return &Client{
		http:    &http.Client{Timeout: 20 * time.Second},
		baseURL: defaultBaseURL,
		email:   email,
	}
}

func (c *Client) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	if c.email != "" {
		params.Set("mailto", c.email)
	}
	u := c.baseURL + path + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Research-API/1.0 (mailto:"+c.email+")")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenAlex request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("OpenAlex HTTP %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// SearchWorks searches scholarly publications.
func (c *Client) SearchWorks(ctx context.Context, q WorkSearchQuery) (*SearchResult, error) {
	if q.Query == "" && q.Author == "" && q.Institution == "" && q.ConceptID == "" {
		return nil, fmt.Errorf("at least one search parameter required (query, author, institution, or conceptId)")
	}
	if q.MaxResults <= 0 || q.MaxResults > 200 {
		q.MaxResults = 25
	}
	if q.Page <= 0 {
		q.Page = 1
	}

	params := url.Values{}
	if q.Query != "" {
		params.Set("search", q.Query)
	}

	// Build filter string
	filters := []string{}
	if q.OpenAccessOnly {
		filters = append(filters, "is_oa:true")
	}
	if q.Type != "" {
		filters = append(filters, "type:"+q.Type)
	}
	if q.Year > 0 {
		filters = append(filters, fmt.Sprintf("publication_year:%d", q.Year))
	} else {
		if q.YearFrom > 0 {
			filters = append(filters, fmt.Sprintf("publication_year:>%d", q.YearFrom-1))
		}
		if q.YearTo > 0 {
			filters = append(filters, fmt.Sprintf("publication_year:<%d", q.YearTo+1))
		}
	}
	if q.ConceptID != "" {
		filters = append(filters, "concepts.id:"+q.ConceptID)
	}
	if len(filters) > 0 {
		params.Set("filter", strings.Join(filters, ","))
	}

	// Sort
	if q.SortBy == "cited_by_count" {
		params.Set("sort", "cited_by_count:desc")
	} else if q.SortBy == "publication_date" {
		params.Set("sort", "publication_date:desc")
	}

	params.Set("per-page", fmt.Sprintf("%d", q.MaxResults))
	params.Set("page", fmt.Sprintf("%d", q.Page))
	params.Set("select", "id,doi,title,abstract_inverted_index,publication_year,publication_date,type,cited_by_count,open_access,primary_location,authorships,concepts,best_oa_location")

	body, err := c.get(ctx, "/works", params)
	if err != nil {
		return nil, err
	}

	return c.parseWorksResponse(body, q)
}

// GetWorkByDOI fetches a specific work by DOI.
func (c *Client) GetWorkByDOI(ctx context.Context, doi string) (*Work, error) {
	doi = strings.TrimSpace(doi)
	if doi == "" {
		return nil, fmt.Errorf("doi is required")
	}
	// Normalize DOI — remove https://doi.org/ prefix if present
	doi = strings.TrimPrefix(doi, "https://doi.org/")
	doi = strings.TrimPrefix(doi, "http://doi.org/")

	params := url.Values{}
	params.Set("select", "id,doi,title,abstract_inverted_index,publication_year,publication_date,type,cited_by_count,open_access,primary_location,authorships,concepts,best_oa_location")

	body, err := c.get(ctx, "/works/https://doi.org/"+url.PathEscape(doi), params)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil, nil
		}
		return nil, err
	}

	var raw oaWork
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode work: %w", err)
	}
	work := mapWork(raw)
	return &work, nil
}

// SearchInstitutions searches for research institutions.
func (c *Client) SearchInstitutions(ctx context.Context, query string, maxResults int) ([]Institution, int, error) {
	if query == "" {
		return nil, 0, fmt.Errorf("query is required")
	}
	if maxResults <= 0 || maxResults > 100 {
		maxResults = 10
	}
	params := url.Values{}
	params.Set("search", query)
	params.Set("per-page", fmt.Sprintf("%d", maxResults))
	params.Set("select", "id,display_name,country_code,type,homepage_url,works_count,cited_by_count,ids")

	body, err := c.get(ctx, "/institutions", params)
	if err != nil {
		return nil, 0, err
	}

	var raw struct {
		Meta struct {
			Count int `json:"count"`
		} `json:"meta"`
		Results []oaInstitution `json:"results"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, 0, fmt.Errorf("decode institutions: %w", err)
	}

	institutions := make([]Institution, 0, len(raw.Results))
	for _, r := range raw.Results {
		institutions = append(institutions, mapInstitution(r))
	}
	return institutions, raw.Meta.Count, nil
}

// ── Raw OpenAlex JSON structures ───────────────────────────────────────────────

type oaListResponse struct {
	Meta struct {
		Count       int `json:"count"`
		DBResponseTime int `json:"db_response_time_ms"`
		Page        int `json:"page"`
		PerPage     int `json:"per_page"`
	} `json:"meta"`
	Results []oaWork `json:"results"`
}

type oaWork struct {
	ID              string  `json:"id"`
	DOI             string  `json:"doi"`
	Title           string  `json:"title"`
	PublicationYear int     `json:"publication_year"`
	PublicationDate string  `json:"publication_date"`
	Type            string  `json:"type"`
	CitedByCount    int     `json:"cited_by_count"`
	OpenAccess      struct {
		IsOA  bool   `json:"is_oa"`
		OAURL string `json:"oa_url"`
	} `json:"open_access"`
	BestOALocation *struct {
		PDFURL string `json:"pdf_url"`
	} `json:"best_oa_location"`
	PrimaryLocation *struct {
		Source *struct {
			DisplayName string `json:"display_name"`
		} `json:"source"`
	} `json:"primary_location"`
	Authorships []struct {
		Author struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
			Orcid       string `json:"orcid"`
		} `json:"author"`
		Institutions []struct {
			DisplayName string `json:"display_name"`
		} `json:"institutions"`
	} `json:"authorships"`
	Concepts []struct {
		ID          string  `json:"id"`
		DisplayName string  `json:"display_name"`
		Score       float64 `json:"score"`
		Level       int     `json:"level"`
	} `json:"concepts"`
	AbstractInvertedIndex map[string][]int `json:"abstract_inverted_index"`
}

type oaInstitution struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	CountryCode string `json:"country_code"`
	Type        string `json:"type"`
	HomepageURL string `json:"homepage_url"`
	WorksCount  int    `json:"works_count"`
	CitedByCount int   `json:"cited_by_count"`
	IDs         struct {
		ROR string `json:"ror"`
	} `json:"ids"`
}

func (c *Client) parseWorksResponse(body []byte, q WorkSearchQuery) (*SearchResult, error) {
	var raw oaListResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse works response: %w", err)
	}

	works := make([]Work, 0, len(raw.Results))
	for _, r := range raw.Results {
		works = append(works, mapWork(r))
	}

	return &SearchResult{
		Total:   raw.Meta.Count,
		Page:    q.Page,
		PerPage: q.MaxResults,
		Results: works,
	}, nil
}

// reconstructAbstract rebuilds the abstract from OpenAlex's inverted index format.
func reconstructAbstract(invertedIndex map[string][]int) string {
	if len(invertedIndex) == 0 {
		return ""
	}
	// find max position
	maxPos := 0
	for _, positions := range invertedIndex {
		for _, pos := range positions {
			if pos > maxPos {
				maxPos = pos
			}
		}
	}
	words := make([]string, maxPos+1)
	for word, positions := range invertedIndex {
		for _, pos := range positions {
			if pos <= maxPos {
				words[pos] = word
			}
		}
	}
	abstract := strings.Join(words, " ")
	// truncate to 600 chars
	if len(abstract) > 600 {
		abstract = abstract[:597] + "..."
	}
	return strings.TrimSpace(abstract)
}

func mapWork(r oaWork) Work {
	w := Work{
		ID:              strings.TrimPrefix(r.ID, "https://openalex.org/"),
		DOI:             r.DOI,
		Title:           r.Title,
		PublicationYear: r.PublicationYear,
		PublicationDate: r.PublicationDate,
		Type:            r.Type,
		CitedByCount:    r.CitedByCount,
		OpenAccess:      r.OpenAccess.IsOA,
		OAUrl:           r.OpenAccess.OAURL,
		URL:             r.ID,
	}

	// abstract from inverted index
	if len(r.AbstractInvertedIndex) > 0 {
		w.Abstract = reconstructAbstract(r.AbstractInvertedIndex)
	}

	// journal/venue
	if r.PrimaryLocation != nil && r.PrimaryLocation.Source != nil {
		w.Journal = r.PrimaryLocation.Source.DisplayName
	}

	// PDF URL from best OA location
	if w.OAUrl == "" && r.BestOALocation != nil && r.BestOALocation.PDFURL != "" {
		w.OAUrl = r.BestOALocation.PDFURL
	}

	// authors
	authors := make([]Author, 0, len(r.Authorships))
	for _, a := range r.Authorships {
		inst := ""
		if len(a.Institutions) > 0 {
			inst = a.Institutions[0].DisplayName
		}
		authors = append(authors, Author{
			ID:          strings.TrimPrefix(a.Author.ID, "https://openalex.org/"),
			Name:        a.Author.DisplayName,
			Orcid:       a.Author.Orcid,
			Institution: inst,
		})
	}
	w.Authors = authors

	// concepts (top 5 by score)
	concepts := make([]Concept, 0, 5)
	for i, co := range r.Concepts {
		if i >= 5 {
			break
		}
		concepts = append(concepts, Concept{
			ID:    strings.TrimPrefix(co.ID, "https://openalex.org/"),
			Name:  co.DisplayName,
			Score: co.Score,
			Level: co.Level,
		})
	}
	w.Concepts = concepts

	return w
}

func mapInstitution(r oaInstitution) Institution {
	return Institution{
		ID:         strings.TrimPrefix(r.ID, "https://openalex.org/"),
		Name:       r.DisplayName,
		Country:    r.CountryCode,
		Type:       r.Type,
		Homepage:   r.HomepageURL,
		WorksCount: r.WorksCount,
		CitedCount: r.CitedByCount,
		ROR:        r.IDs.ROR,
	}
}
