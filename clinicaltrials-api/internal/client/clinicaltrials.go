// Package client wraps the ClinicalTrials.gov API v2.
// Docs: https://clinicaltrials.gov/data-api/api
// No API key required. Free public API from NIH/NLM.
// Returns clinical trial data (studies, conditions, interventions, locations, phases, status).
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBase    = "https://clinicaltrials.gov/api/v2"
	defaultTimeout = 15 * time.Second
	maxPageSize    = 100
)

// Study is a summarized clinical trial record.
type Study struct {
	NCTId           string   `json:"nct_id"`
	Title           string   `json:"title"`
	Status          string   `json:"status"`           // "RECRUITING", "COMPLETED", etc.
	Phase           []string `json:"phase"`            // e.g. ["PHASE2", "PHASE3"]
	StartDate       string   `json:"start_date"`
	CompletionDate  string   `json:"completion_date"`
	Conditions      []string `json:"conditions"`
	Interventions   []string `json:"interventions"`
	Sponsor         string   `json:"sponsor"`
	BriefSummary    string   `json:"brief_summary"`
	Locations       []string `json:"locations"` // first 5 country names
	URL             string   `json:"url"`
}

// SearchResult wraps a list of studies with pagination info.
type SearchResult struct {
	Total      int     `json:"total"`
	Count      int     `json:"count"`
	NextToken  string  `json:"next_token,omitempty"`
	Studies    []Study `json:"studies"`
}

// Client wraps the ClinicalTrials.gov API.
type Client struct {
	http    *http.Client
	baseURL string
}

// New returns a new ClinicalTrials client.
func New() *Client {
	return &Client{
		http:    &http.Client{Timeout: defaultTimeout},
		baseURL: defaultBase,
	}
}

// ── Internal helpers ──────────────────────────────────────────────────────────

func (c *Client) getJSON(ctx context.Context, path string, out interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("not_found")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// extractStudy normalizes the raw API response into our Study struct.
func extractStudy(raw map[string]interface{}) Study {
	getStr := func(m map[string]interface{}, keys ...string) string {
		cur := m
		for i, k := range keys {
			if v, ok := cur[k]; ok {
				if i == len(keys)-1 {
					if s, ok := v.(string); ok {
						return strings.TrimSpace(s)
					}
					return fmt.Sprintf("%v", v)
				}
				if next, ok := v.(map[string]interface{}); ok {
					cur = next
				} else {
					return ""
				}
			} else {
				return ""
			}
		}
		return ""
	}

	getList := func(m map[string]interface{}, key string) []string {
		if v, ok := m[key]; ok {
			if arr, ok := v.([]interface{}); ok {
				result := make([]string, 0, len(arr))
				for _, item := range arr {
					if s, ok := item.(string); ok {
						result = append(result, s)
					}
				}
				return result
			}
		}
		return nil
	}

	proto := getMap(raw, "protocolSection")
	if proto == nil {
		return Study{}
	}

	idMod := getMap(proto, "identificationModule")
	statusMod := getMap(proto, "statusModule")
	descMod := getMap(proto, "descriptionModule")
	condMod := getMap(proto, "conditionsModule")
	armsMod := getMap(proto, "armsInterventionsModule")
	sponsMod := getMap(proto, "sponsorCollaboratorsModule")
	designMod := getMap(proto, "designModule")
	contactMod := getMap(proto, "contactsLocationsModule")

	study := Study{}

	if idMod != nil {
		study.NCTId = getStr(idMod, "nctId")
		study.Title = getStr(idMod, "briefTitle")
		if study.Title == "" {
			study.Title = getStr(idMod, "officialTitle")
		}
	}

	if statusMod != nil {
		study.Status = getStr(statusMod, "overallStatus")
		if startDate := getMap(statusMod, "startDateStruct"); startDate != nil {
			study.StartDate = getStr(startDate, "date")
		}
		if compDate := getMap(statusMod, "completionDateStruct"); compDate != nil {
			study.CompletionDate = getStr(compDate, "date")
		}
		if primDate := getMap(statusMod, "primaryCompletionDateStruct"); primDate != nil && study.CompletionDate == "" {
			study.CompletionDate = getStr(primDate, "date")
		}
	}

	if descMod != nil {
		summary := getStr(descMod, "briefSummary")
		// Truncate to 500 chars
		if len(summary) > 500 {
			summary = summary[:497] + "..."
		}
		study.BriefSummary = summary
	}

	if condMod != nil {
		study.Conditions = getList(condMod, "conditions")
	}

	if armsMod != nil {
		if interventions, ok := armsMod["interventions"]; ok {
			if arr, ok := interventions.([]interface{}); ok {
				for _, item := range arr {
					if m, ok := item.(map[string]interface{}); ok {
						if name, ok := m["name"].(string); ok && name != "" {
							itype, _ := m["type"].(string)
							if itype != "" {
								study.Interventions = append(study.Interventions, itype+": "+name)
							} else {
								study.Interventions = append(study.Interventions, name)
							}
						}
					}
				}
			}
		}
	}

	if sponsMod != nil {
		if lead := getMap(sponsMod, "leadSponsor"); lead != nil {
			study.Sponsor = getStr(lead, "name")
		}
	}

	if designMod != nil {
		if phases, ok := designMod["phases"]; ok {
			if arr, ok := phases.([]interface{}); ok {
				for _, p := range arr {
					if s, ok := p.(string); ok {
						study.Phase = append(study.Phase, s)
					}
				}
			}
		}
	}

	if contactMod != nil {
		if locs, ok := contactMod["locations"]; ok {
			if arr, ok := locs.([]interface{}); ok {
				seen := map[string]bool{}
				for _, item := range arr {
					if m, ok := item.(map[string]interface{}); ok {
						country, _ := m["country"].(string)
						if country != "" && !seen[country] && len(study.Locations) < 5 {
							study.Locations = append(study.Locations, country)
							seen[country] = true
						}
					}
				}
			}
		}
	}

	if study.NCTId != "" {
		study.URL = "https://clinicaltrials.gov/study/" + study.NCTId
	}

	return study
}

func getMap(m map[string]interface{}, key string) map[string]interface{} {
	if v, ok := m[key]; ok {
		if m2, ok := v.(map[string]interface{}); ok {
			return m2
		}
	}
	return nil
}

// ── Public API ────────────────────────────────────────────────────────────────

// Search searches for clinical trials.
// query: free text search term
// status: "RECRUITING", "COMPLETED", "NOT_YET_RECRUITING", "" = all
// phase: "PHASE1", "PHASE2", "PHASE3", "PHASE4", "" = all
// limit: 1-100 (default 10)
// nextToken: pagination token from previous response
func (c *Client) Search(ctx context.Context, query, status, phase string, limit int, nextToken string) (*SearchResult, error) {
	if limit <= 0 || limit > maxPageSize {
		limit = 10
	}

	params := url.Values{
		"format":   []string{"json"},
		"pageSize": []string{fmt.Sprintf("%d", limit)},
		"fields":   []string{"protocolSection"},
	}

	if query = strings.TrimSpace(query); query != "" {
		params.Set("query.term", query)
	}
	if status = strings.ToUpper(strings.TrimSpace(status)); status != "" {
		params.Set("filter.overallStatus", status)
	}
	if phase = strings.ToUpper(strings.TrimSpace(phase)); phase != "" {
		params.Set("filter.phase", phase)
	}
	if nextToken != "" {
		params.Set("pageToken", nextToken)
	}

	path := "/studies?" + params.Encode()

	var raw struct {
		Studies    []map[string]interface{} `json:"studies"`
		NextToken  string                   `json:"nextPageToken"`
		TotalCount int                      `json:"totalCount"`
	}
	if err := c.getJSON(ctx, path, &raw); err != nil {
		return nil, err
	}

	studies := make([]Study, 0, len(raw.Studies))
	for _, r := range raw.Studies {
		s := extractStudy(r)
		if s.NCTId != "" {
			studies = append(studies, s)
		}
	}

	return &SearchResult{
		Total:     raw.TotalCount,
		Count:     len(studies),
		NextToken: raw.NextToken,
		Studies:   studies,
	}, nil
}

// GetStudy fetches details for a specific NCT ID.
func (c *Client) GetStudy(ctx context.Context, nctID string) (*Study, error) {
	nctID = strings.TrimSpace(strings.ToUpper(nctID))
	if nctID == "" {
		return nil, fmt.Errorf("NCT ID is required")
	}

	params := url.Values{
		"format": []string{"json"},
		"fields": []string{"protocolSection"},
	}
	path := fmt.Sprintf("/studies/%s?%s", url.PathEscape(nctID), params.Encode())

	var raw map[string]interface{}
	if err := c.getJSON(ctx, path, &raw); err != nil {
		return nil, err
	}

	study := extractStudy(raw)
	if study.NCTId == "" {
		return nil, fmt.Errorf("not_found: no study with ID %s", nctID)
	}
	return &study, nil
}
