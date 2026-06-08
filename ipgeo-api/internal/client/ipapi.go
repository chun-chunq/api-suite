// Package client wraps the ip-api.com IP geolocation service.
// Free tier: 45 requests/minute, no auth.
// Paid key (optional): removes rate limit, adds HTTPS, batch endpoint.
// Docs: https://ip-api.com/docs
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	baseURL     = "http://ip-api.com"  // free tier is HTTP only
	baseURLPro  = "https://pro.ip-api.com" // paid key uses HTTPS
	batchURL    = "http://ip-api.com/batch"
	fields      = "status,message,country,countryCode,region,regionName,city,zip,lat,lon,timezone,isp,org,as,asname,mobile,proxy,hosting,query"
)

// ── Types ─────────────────────────────────────────────────────────────────────

// GeoResult holds geolocation data for one IP address.
type GeoResult struct {
	IP          string  `json:"ip"`
	Status      string  `json:"status"`           // "success" or "fail"
	Message     string  `json:"message,omitempty"` // error message when status=fail
	Country     string  `json:"country,omitempty"`
	CountryCode string  `json:"countryCode,omitempty"`
	Region      string  `json:"region,omitempty"`
	RegionName  string  `json:"regionName,omitempty"`
	City        string  `json:"city,omitempty"`
	ZIP         string  `json:"zip,omitempty"`
	Lat         float64 `json:"lat,omitempty"`
	Lon         float64 `json:"lon,omitempty"`
	Timezone    string  `json:"timezone,omitempty"`
	ISP         string  `json:"isp,omitempty"`
	Org         string  `json:"org,omitempty"`
	AS          string  `json:"as,omitempty"`      // ASN
	ASName      string  `json:"asname,omitempty"`
	Mobile      bool    `json:"mobile"`
	Proxy       bool    `json:"proxy"`   // VPN/proxy/Tor detected
	Hosting     bool    `json:"hosting"` // datacenter/hosting IP
}

// ── Client ────────────────────────────────────────────────────────────────────

type Client struct {
	http    *http.Client
	baseURL string
	apiKey  string // optional paid key
}

func New(apiKey string) *Client {
	base := baseURL
	if apiKey != "" {
		base = baseURLPro
	}
	return &Client{
		http:    &http.Client{Timeout: 10 * time.Second},
		baseURL: base,
		apiKey:  apiKey,
	}
}

// ── Lookup single IP ──────────────────────────────────────────────────────────

// Lookup geolocates a single IP address. Pass "" or "self" to look up the caller's IP.
func (c *Client) Lookup(ctx context.Context, ip string) (*GeoResult, error) {
	ip = strings.TrimSpace(ip)
	if ip == "self" {
		ip = ""
	}

	// Validate if non-empty
	if ip != "" {
		if net.ParseIP(ip) == nil {
			// could be a hostname — ip-api.com supports that too, but validate format
			if strings.Contains(ip, " ") {
				return nil, fmt.Errorf("invalid IP address: %q", ip)
			}
		}
	}

	path := "/json/"
	if ip != "" {
		path += ip
	}

	params := url.Values{}
	params.Set("fields", fields)
	if c.apiKey != "" {
		params.Set("key", c.apiKey)
	}

	u := c.baseURL + path + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("rate limited (45 req/min free tier) — retry in a moment")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream %d", resp.StatusCode)
	}

	var result GeoResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if result.Status == "fail" {
		return nil, fmt.Errorf("ip-api error: %s", result.Message)
	}

	// Ensure IP field is populated
	if result.IP == "" {
		result.IP = result.query(body)
	}

	return &result, nil
}

// query extracts the "query" field from raw JSON (ip-api returns the resolved IP as "query").
func (r *GeoResult) query(body []byte) string {
	var raw struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(body, &raw); err == nil {
		return raw.Query
	}
	return ""
}

// ── Batch lookup ──────────────────────────────────────────────────────────────

// LookupBatch geolocates up to 100 IPs in one request (requires free tier too, but
// only available without key on HTTP). Returns results in same order as input.
func (c *Client) LookupBatch(ctx context.Context, ips []string) ([]GeoResult, error) {
	if len(ips) == 0 {
		return nil, fmt.Errorf("at least one IP is required")
	}
	if len(ips) > 100 {
		ips = ips[:100]
	}

	// Build request body: [{"query":"IP","fields":"..."},...]
	type batchItem struct {
		Query  string `json:"query"`
		Fields string `json:"fields"`
	}
	items := make([]batchItem, len(ips))
	for i, ip := range ips {
		items[i] = batchItem{Query: strings.TrimSpace(ip), Fields: fields}
	}

	body, err := json.Marshal(items)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	params := url.Values{}
	params.Set("fields", fields)
	if c.apiKey != "" {
		params.Set("key", c.apiKey)
	}
	// batch endpoint is always /batch under the configured base URL
	batchEndpoint := c.baseURL + "/batch"
	u := batchEndpoint + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u,
		strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("rate limited — retry in a moment")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	// Parse as array of raw maps first to extract "query" field
	var rawResults []json.RawMessage
	if err := json.Unmarshal(respBody, &rawResults); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	results := make([]GeoResult, 0, len(rawResults))
	for _, raw := range rawResults {
		var r GeoResult
		if err := json.Unmarshal(raw, &r); err != nil {
			continue
		}
		// Extract "query" field for IP
		var q struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(raw, &q); err == nil && r.IP == "" {
			r.IP = q.Query
		}
		results = append(results, r)
	}
	return results, nil
}
