// Package client wraps the EU CORDIS open data REST API.
// CORDIS = Community Research and Development Information Service
// API docs: https://cordis.europa.eu/dataextractionformat/en
// REST search: https://cordis.europa.eu/search/results_en?q=...&format=json
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

const baseURL = "https://cordis.europa.eu"

// Project is a Horizon Europe / FP7 / H2020 funded research project.
type Project struct {
	ID              string   `json:"id"`
	RCN             string   `json:"rcn,omitempty"`
	Acronym         string   `json:"acronym,omitempty"`
	Title           string   `json:"title"`
	Status          string   `json:"status,omitempty"` // "CLOSED" | "ACTIVE" | "PENDING"
	Programme       string   `json:"programme,omitempty"` // e.g. "H2020", "HORIZON"
	Topics          []string `json:"topics,omitempty"`
	StartDate       string   `json:"startDate,omitempty"`
	EndDate         string   `json:"endDate,omitempty"`
	TotalCost       float64  `json:"totalCostEur,omitempty"`
	ECContribution  float64  `json:"ecContributionEur,omitempty"`
	Objective       string   `json:"objective,omitempty"`
	Coordinator     *Organization `json:"coordinator,omitempty"`
	Participants    []Organization `json:"participants,omitempty"`
	URL             string   `json:"url,omitempty"`
}

// Organization is a project participant or coordinator.
type Organization struct {
	Name        string  `json:"name"`
	ShortName   string  `json:"shortName,omitempty"`
	Country     string  `json:"country,omitempty"`
	City        string  `json:"city,omitempty"`
	Role        string  `json:"role,omitempty"` // "coordinator" | "participant"
	Contribution float64 `json:"contributionEur,omitempty"`
}

// SearchQuery holds search parameters.
type SearchQuery struct {
	Keywords   string // full-text search
	Country    string // ISO-2 country of coordinator (e.g. "DE", "FR")
	Programme  string // "HORIZON" | "H2020" | "FP7"
	FromYear   int    // project start year >=
	ToYear     int    // project start year <=
	Status     string // "ACTIVE" | "CLOSED"
	MaxResults int    // 1-100, default 25
	Page       int    // page number (1-based)
}

// SearchResult wraps a paginated list of projects.
type SearchResult struct {
	Total    int       `json:"total"`
	Page     int       `json:"page"`
	PerPage  int       `json:"perPage"`
	Results  []Project `json:"results"`
	Query    SearchQuery `json:"-"`
}

// Client wraps the CORDIS API.
type Client struct {
	http *http.Client
}

// New creates a CORDIS client.
func New() *Client {
	return &Client{
		http: &http.Client{Timeout: 20 * time.Second},
	}
}

// Search finds projects matching the query.
func (c *Client) Search(ctx context.Context, q SearchQuery) (*SearchResult, error) {
	if q.MaxResults <= 0 || q.MaxResults > 100 {
		q.MaxResults = 25
	}
	if q.Page < 1 {
		q.Page = 1
	}

	// Build CORDIS query string (Lucene-style)
	parts := []string{}
	if q.Keywords != "" {
		parts = append(parts, fmt.Sprintf("(title:%s OR objective:%s OR acronym:%s)",
			escapeQ(q.Keywords), escapeQ(q.Keywords), escapeQ(q.Keywords)))
	}
	if q.Country != "" {
		parts = append(parts, fmt.Sprintf("coordinatorCountry:%s", strings.ToUpper(q.Country)))
	}
	if q.Programme != "" {
		parts = append(parts, fmt.Sprintf("programme/code:%s", strings.ToUpper(q.Programme)))
	}
	if q.Status != "" {
		parts = append(parts, fmt.Sprintf("status:%s", strings.ToUpper(q.Status)))
	}
	if q.FromYear > 0 {
		parts = append(parts, fmt.Sprintf("startDate:[%d-01-01+TO+*]", q.FromYear))
	}
	if q.ToYear > 0 {
		parts = append(parts, fmt.Sprintf("startDate:[*+TO+%d-12-31]", q.ToYear))
	}

	queryStr := "*"
	if len(parts) > 0 {
		queryStr = strings.Join(parts, " AND ")
	}

	params := url.Values{}
	params.Set("q", queryStr)
	params.Set("format", "json")
	params.Set("p", fmt.Sprintf("%d", q.Page))
	params.Set("num", fmt.Sprintf("%d", q.MaxResults))
	params.Set("srt", "score,DESC")

	path := "/search/results_en?" + params.Encode()
	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}

	return parseCordisResponse(body, q)
}

// GetProject fetches a single project by its CORDIS ID (numeric string, e.g. "101016775").
func (c *Client) GetProject(ctx context.Context, projectID string) (*Project, error) {
	projectID = strings.TrimSpace(projectID)
	params := url.Values{}
	params.Set("q", "id:"+projectID)
	params.Set("format", "json")
	params.Set("num", "1")

	body, err := c.get(ctx, "/search/results_en?"+params.Encode())
	if err != nil {
		return nil, err
	}
	result, err := parseCordisResponse(body, SearchQuery{MaxResults: 1})
	if err != nil {
		return nil, err
	}
	if len(result.Results) == 0 {
		return nil, nil
	}
	return &result.Results[0], nil
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	u := baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "CORDIS-API-Wrapper/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("CORDIS API: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CORDIS API HTTP %d", resp.StatusCode)
	}
	return body, nil
}

// parseCordisResponse converts the CORDIS JSON-API response into our model.
func parseCordisResponse(body []byte, q SearchQuery) (*SearchResult, error) {
	// CORDIS returns a somewhat flexible structure
	var raw struct {
		Header struct {
			Total int `json:"numFound"`
		} `json:"header"`
		Results []struct {
			// The CORDIS search result wraps each item
			Project *cordisProject `json:"project"`
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		// Try flat array format (different CORDIS endpoint versions)
		var flat struct {
			Header struct {
				Total int `json:"numFound"`
			} `json:"header"`
			Projects []cordisProject `json:"project"`
		}
		if err2 := json.Unmarshal(body, &flat); err2 != nil {
			return nil, fmt.Errorf("parse CORDIS response: %w", err)
		}
		projects := make([]Project, 0, len(flat.Projects))
		for _, p := range flat.Projects {
			projects = append(projects, mapProject(p))
		}
		return &SearchResult{
			Total:   flat.Header.Total,
			Page:    q.Page,
			PerPage: q.MaxResults,
			Results: projects,
		}, nil
	}

	projects := make([]Project, 0, len(raw.Results))
	for _, r := range raw.Results {
		if r.Project != nil {
			projects = append(projects, mapProject(*r.Project))
		}
	}
	return &SearchResult{
		Total:   raw.Header.Total,
		Page:    q.Page,
		PerPage: q.MaxResults,
		Results: projects,
	}, nil
}

// cordisProject is the raw CORDIS project structure.
type cordisProject struct {
	ID             string  `json:"id"`
	RCN            string  `json:"rcn"`
	Acronym        string  `json:"acronym"`
	Title          string  `json:"title"`
	Status         string  `json:"status"`
	Programme      string  `json:"programmeFundingScheme"`
	StartDate      string  `json:"startDate"`
	EndDate        string  `json:"endDate"`
	TotalCost      float64 `json:"totalCost"`
	ECContribution float64 `json:"ecMaxContribution"`
	Objective      string  `json:"objective"`
	Topics         string  `json:"topics"`
	Coordinator    struct {
		Name        string  `json:"name"`
		ShortName   string  `json:"shortName"`
		Country     string  `json:"country"`
		City        string  `json:"city"`
		Contribution float64 `json:"ecContribution"`
	} `json:"coordinator"`
	Participants []struct {
		Name        string  `json:"name"`
		ShortName   string  `json:"shortName"`
		Country     string  `json:"country"`
		City        string  `json:"city"`
		Role        string  `json:"activityType"`
		Contribution float64 `json:"ecContribution"`
	} `json:"participant"`
}

func mapProject(p cordisProject) Project {
	proj := Project{
		ID:             p.ID,
		RCN:            p.RCN,
		Acronym:        p.Acronym,
		Title:          p.Title,
		Status:         p.Status,
		Programme:      p.Programme,
		StartDate:      p.StartDate,
		EndDate:        p.EndDate,
		TotalCost:      p.TotalCost,
		ECContribution: p.ECContribution,
		URL:            fmt.Sprintf("https://cordis.europa.eu/project/id/%s", p.ID),
	}
	// Objective can be very long — truncate to 500 chars for list view
	obj := strings.TrimSpace(p.Objective)
	if len(obj) > 500 {
		obj = obj[:497] + "..."
	}
	proj.Objective = obj

	// Topics (comma-separated string → slice)
	if p.Topics != "" {
		for _, t := range strings.Split(p.Topics, ";") {
			t = strings.TrimSpace(t)
			if t != "" {
				proj.Topics = append(proj.Topics, t)
			}
		}
	}

	// Coordinator
	if p.Coordinator.Name != "" {
		proj.Coordinator = &Organization{
			Name:        p.Coordinator.Name,
			ShortName:   p.Coordinator.ShortName,
			Country:     p.Coordinator.Country,
			City:        p.Coordinator.City,
			Role:        "coordinator",
			Contribution: p.Coordinator.Contribution,
		}
	}

	// Participants
	for _, part := range p.Participants {
		if part.Name != "" {
			proj.Participants = append(proj.Participants, Organization{
				Name:        part.Name,
				ShortName:   part.ShortName,
				Country:     part.Country,
				City:        part.City,
				Role:        "participant",
				Contribution: part.Contribution,
			})
		}
	}
	return proj
}

func escapeQ(s string) string {
	// Basic Lucene escaping — replace spaces with +, quote multi-word
	s = strings.TrimSpace(s)
	if strings.ContainsAny(s, " \t") {
		return fmt.Sprintf(`"%s"`, s)
	}
	return s
}
