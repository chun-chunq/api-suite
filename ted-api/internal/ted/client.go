// Package ted wraps the official TED EU Procurement API (api.ted.europa.eu v3).
// No authentication is required for the search endpoint.
package ted

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	tedAPIBase  = "https://api.ted.europa.eu"
	searchPath  = "/v3/notices/search"
	httpTimeout = 30 * time.Second

	// Fields that the TED API actually supports (validated against live API).
	// The API uses eForms SDK field names — "cpv", "estimated-value" etc. are NOT valid;
	// only the fields below are confirmed working for the search endpoint.
	defaultFields = "publication-number,notice-title,notice-type,buyer-name,publication-date"
)

// Client calls the TED API search endpoint.
type Client struct {
	http    *http.Client
	baseURL string
}

// NewClient creates a ready TED API client.
func NewClient() *Client {
	return &Client{
		http:    &http.Client{Timeout: httpTimeout},
		baseURL: tedAPIBase,
	}
}

// SearchRequest is the body sent to POST /v3/notices/search.
type SearchRequest struct {
	Query          string   `json:"query"`
	Fields         []string `json:"fields"`
	Limit          int      `json:"limit"`
	Page           int      `json:"page,omitempty"`
	Scope          string   `json:"scope"`
	PaginationMode string   `json:"paginationMode"`
}

// RawSearchResponse is the JSON response from the TED API.
type RawSearchResponse struct {
	Notices           []RawNotice `json:"notices"`
	TotalNoticeCount  int         `json:"totalNoticeCount"`
	TimedOut          bool        `json:"timedOut"`
}

// RawNotice is a single notice as returned by the TED API.
// Multilingual fields are maps from language code → value.
type RawNotice struct {
	PublicationNumber string                 `json:"publication-number"`
	NoticeType        string                 `json:"notice-type"`
	PublicationDate   string                 `json:"publication-date"`
	BuyerName         map[string][]string    `json:"buyer-name"`
	NoticeTitle       map[string]string      `json:"notice-title"`
	Links             map[string]interface{} `json:"links"`
}

// Search executes a search against the TED API.
func (c *Client) Search(ctx context.Context, req SearchRequest) (*RawSearchResponse, error) {
	if len(req.Fields) == 0 {
		req.Fields = strings.Split(defaultFields, ",")
	}
	if req.Limit <= 0 {
		req.Limit = 10
	}
	if req.Limit > 100 {
		req.Limit = 100
	}
	if req.Scope == "" {
		req.Scope = "ACTIVE"
	}
	if req.PaginationMode == "" {
		req.PaginationMode = "PAGE_NUMBER"
	}
	if req.Page < 1 {
		req.Page = 1
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+searchPath, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		snippet := string(raw)
		if len(snippet) > 300 {
			snippet = snippet[:300]
		}
		return nil, fmt.Errorf("TED API %d: %s", resp.StatusCode, snippet)
	}

	var result RawSearchResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}
	return &result, nil
}
