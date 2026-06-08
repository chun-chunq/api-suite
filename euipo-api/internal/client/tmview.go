// Package client wraps the TMview REST API — the official EU trademark database.
// TMview is operated by EUIPO and covers 60+ national IP offices.
// API docs: https://www.tmdn.org/tmview/#/
// License: open data, no auth required
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

const defaultBaseURL = "https://www.tmdn.org/tmview/api"

// Trademark represents a single trademark record from TMview.
type Trademark struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`           // word mark / figurative mark name
	ApplicationNum string   `json:"applicationNum"` // filing number
	RegistrationNum string  `json:"registrationNum,omitempty"`
	Status         string   `json:"status"`         // REGISTERED, PENDING, EXPIRED, etc.
	FilingDate     string   `json:"filingDate,omitempty"`
	ExpiryDate     string   `json:"expiryDate,omitempty"`
	Office         string   `json:"office"`   // e.g. "EM" = EUIPO, "DE" = DPMA, "FR" = INPI
	OfficeName     string   `json:"officeName"`
	Applicant      string   `json:"applicant,omitempty"`
	NiceClasses    []int    `json:"niceClasses,omitempty"` // Nice classification classes (1–45)
	Goods          string   `json:"goods,omitempty"`       // goods & services description
	ImageURL       string   `json:"imageUrl,omitempty"`    // figurative mark thumbnail
	Type           string   `json:"type"`                  // "word", "figurative", "combined"
	URL            string   `json:"url"`                   // link to TMview entry
}

// SearchQuery holds search parameters.
type SearchQuery struct {
	Query       string   // free-text search (trademark name)
	Territories []string // e.g. ["EM","DE","FR"] — empty = all
	Classes     []int    // Nice classes filter — empty = all
	Status      string   // "REGISTERED", "PENDING", "EXPIRED" — empty = all
	Holder      string   // applicant/holder name filter
	MaxResults  int      // 1–100, default 25
	Offset      int      // pagination offset
}

// SearchResult is the response for a trademark search.
type SearchResult struct {
	Total   int         `json:"total"`
	Offset  int         `json:"offset"`
	Results []Trademark `json:"results"`
	Query   SearchQuery `json:"query"`
}

// Client wraps the TMview REST API.
type Client struct {
	http    *http.Client
	baseURL string
}

// New creates a new TMview client.
func New() *Client {
	return &Client{
		http:    &http.Client{Timeout: 20 * time.Second},
		baseURL: defaultBaseURL,
	}
}

// Search searches for trademarks across EU and national offices.
func (c *Client) Search(ctx context.Context, q SearchQuery) (*SearchResult, error) {
	if q.Query == "" && q.Holder == "" {
		return nil, fmt.Errorf("query or holder must be provided")
	}
	if q.MaxResults <= 0 || q.MaxResults > 100 {
		q.MaxResults = 25
	}

	params := url.Values{}
	if q.Query != "" {
		params.Set("query", q.Query)
	}
	if q.Holder != "" {
		params.Set("holder", q.Holder)
	}
	if len(q.Territories) > 0 {
		params.Set("territory", strings.Join(q.Territories, ","))
	}
	if q.Status != "" {
		params.Set("status", q.Status)
	}
	if len(q.Classes) > 0 {
		classes := make([]string, len(q.Classes))
		for i, cl := range q.Classes {
			classes[i] = fmt.Sprintf("%d", cl)
		}
		params.Set("niceClass", strings.Join(classes, ","))
	}
	params.Set("offset", fmt.Sprintf("%d", q.Offset))
	params.Set("limit", fmt.Sprintf("%d", q.MaxResults))
	params.Set("language", "en")

	endpoint := fmt.Sprintf("%s/search/basic?%s", c.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; EU-Trademark-API/1.0)")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TMview request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMview HTTP %d: %s", resp.StatusCode, string(body))
	}

	return c.parseResponse(body, q)
}

// GetByID fetches a single trademark by its TMview ID (office code + application number).
// id format: "EM/018123456" or "DE/302023012345"
func (c *Client) GetByID(ctx context.Context, officeCode, appNum string) (*Trademark, error) {
	officeCode = strings.ToUpper(strings.TrimSpace(officeCode))
	appNum = strings.TrimSpace(appNum)
	if officeCode == "" || appNum == "" {
		return nil, fmt.Errorf("officeCode and appNum are required")
	}

	endpoint := fmt.Sprintf("%s/trademark/%s/%s", c.baseURL, officeCode, url.PathEscape(appNum))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("TMview HTTP %d", resp.StatusCode)
	}

	var raw tmviewSingle
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode TMview single: %w", err)
	}
	tm := mapTrademark(raw.TrademarkData)
	return &tm, nil
}

// ── Raw TMview JSON structures ─────────────────────────────────────────────────

type tmviewSearchResponse struct {
	TotalResults int              `json:"totalResults"`
	Results      []tmviewRaw      `json:"trademarks"`
}

type tmviewSingle struct {
	TrademarkData tmviewRaw `json:"trademarkData"`
}

type tmviewRaw struct {
	ST13               string `json:"st13"`               // unique ID: office+number e.g. "EM018123456"
	TrademarkName      string `json:"trademarkName"`
	ApplicationNumber  string `json:"applicationNumber"`
	RegistrationNumber string `json:"registrationNumber"`
	TrademarkStatus    string `json:"trademarkStatus"`
	FilingDate         string `json:"filingDate"`
	ExpiryDate         string `json:"expiryDate"`
	MarkFeature        string `json:"markFeature"` // WORD, FIGURATIVE, COMBINED
	Office             string `json:"office"`      // EM, DE, FR, etc.
	OfficeName         string `json:"officeName"`
	Holders            []struct {
		Name string `json:"name"`
	} `json:"holders"`
	NiceClasses []struct {
		NiceClass int    `json:"niceClass"`
		GoodsAndServices string `json:"goodsAndServices"`
	} `json:"niceClassifications"`
	ImageURI string `json:"imageURI"`
}

func (c *Client) parseResponse(body []byte, q SearchQuery) (*SearchResult, error) {
	var raw tmviewSearchResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse TMview response: %w", err)
	}

	trademarks := make([]Trademark, 0, len(raw.Results))
	for _, r := range raw.Results {
		trademarks = append(trademarks, mapTrademark(r))
	}

	return &SearchResult{
		Total:   raw.TotalResults,
		Offset:  q.Offset,
		Results: trademarks,
		Query:   q,
	}, nil
}

func mapTrademark(r tmviewRaw) Trademark {
	// collect Nice classes
	classes := make([]int, 0, len(r.NiceClasses))
	goods := make([]string, 0, len(r.NiceClasses))
	for _, nc := range r.NiceClasses {
		classes = append(classes, nc.NiceClass)
		if nc.GoodsAndServices != "" {
			goods = append(goods, fmt.Sprintf("[%d] %s", nc.NiceClass, nc.GoodsAndServices))
		}
	}

	// first holder
	applicant := ""
	if len(r.Holders) > 0 {
		applicant = r.Holders[0].Name
	}

	// image URL
	imageURL := ""
	if r.ImageURI != "" {
		imageURL = "https://www.tmdn.org" + r.ImageURI
	}

	// tmview deep-link
	tmURL := ""
	if r.ST13 != "" {
		tmURL = fmt.Sprintf("https://www.tmdn.org/tmview/#/trademark/%s", r.ST13)
	}

	// clean up type name
	tmType := strings.ToLower(r.MarkFeature)

	goodsStr := ""
	if len(goods) > 0 {
		full := strings.Join(goods, " | ")
		if len(full) > 500 {
			full = full[:497] + "..."
		}
		goodsStr = full
	}

	return Trademark{
		ID:              r.ST13,
		Name:            r.TrademarkName,
		ApplicationNum:  r.ApplicationNumber,
		RegistrationNum: r.RegistrationNumber,
		Status:          r.TrademarkStatus,
		FilingDate:      r.FilingDate,
		ExpiryDate:      r.ExpiryDate,
		Office:          r.Office,
		OfficeName:      r.OfficeName,
		Applicant:       applicant,
		NiceClasses:     classes,
		Goods:           goodsStr,
		ImageURL:        imageURL,
		Type:            tmType,
		URL:             tmURL,
	}
}
