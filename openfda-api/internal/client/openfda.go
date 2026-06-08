// Package client wraps the OpenFDA API (api.fda.gov).
// Free, no auth required (rate-limited to 240 req/min without API key).
// Covers: drug labels, adverse events (FAERS), drug recalls.
// Docs: https://open.fda.gov/apis/
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const baseURL = "https://api.fda.gov"

// ── Types ─────────────────────────────────────────────────────────────────────

// DrugLabel is a simplified FDA drug label entry.
type DrugLabel struct {
	ID                  string   `json:"id"`
	BrandName           string   `json:"brandName,omitempty"`
	GenericName         string   `json:"genericName,omitempty"`
	LabelerName         string   `json:"labelerName,omitempty"`
	ProductType         string   `json:"productType,omitempty"`
	Route               []string `json:"route,omitempty"`
	Substance           []string `json:"substance,omitempty"`
	Indications         string   `json:"indicationsAndUsage,omitempty"`
	Warnings            string   `json:"warnings,omitempty"`
	Dosage              string   `json:"dosageAndAdministration,omitempty"`
	Contraindications   string   `json:"contraindications,omitempty"`
	AdverseReactions    string   `json:"adverseReactions,omitempty"`
	EffectiveDate       string   `json:"effectiveDate,omitempty"`
}

// AdverseEvent is a simplified FAERS adverse event report.
type AdverseEvent struct {
	ReportID       string   `json:"reportId"`
	ReceiveDate    string   `json:"receiveDate"`
	Serious        bool     `json:"serious"`
	SeriousReasons []string `json:"seriousReasons,omitempty"`
	Drugs          []string `json:"drugs,omitempty"`
	Reactions      []string `json:"reactions,omitempty"`
	Country        string   `json:"country,omitempty"`
	Age            *float64 `json:"patientAge,omitempty"`
	Sex            string   `json:"patientSex,omitempty"`
}

// DrugRecall is a simplified drug recall record.
type DrugRecall struct {
	RecallNumber    string `json:"recallNumber"`
	Status          string `json:"status"`
	Classification  string `json:"classification"` // Class I, II, III
	RecallingFirm   string `json:"recallingFirm"`
	ProductDesc     string `json:"productDescription"`
	Reason          string `json:"reasonForRecall"`
	Country         string `json:"country"`
	InitiationDate  string `json:"initiationDate"`
	TerminationDate string `json:"terminationDate,omitempty"`
}

// SearchResult is a generic paginated result.
type SearchResult[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
	Skip  int `json:"skip"`
	Limit int `json:"limit"`
}

// ── Client ────────────────────────────────────────────────────────────────────

type Client struct {
	http    *http.Client
	baseURL string
	apiKey  string // optional
}

func New(apiKey string) *Client {
	return &Client{
		http:    &http.Client{Timeout: 20 * time.Second},
		baseURL: baseURL,
		apiKey:  apiKey,
	}
}

// ── Drug Labels ───────────────────────────────────────────────────────────────

// SearchDrugLabels searches FDA drug labels by brand name, generic name, or substance.
func (c *Client) SearchDrugLabels(ctx context.Context, query string, limit, skip int) (*SearchResult[DrugLabel], error) {
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	limit = clamp(limit, 1, 100)
	if skip < 0 {
		skip = 0
	}

	// Search across brand name, generic name, and substance name
	searchStr := fmt.Sprintf(
		`(openfda.brand_name:"%s"+openfda.generic_name:"%s"+openfda.substance_name:"%s")`,
		query, query, query,
	)

	params := url.Values{}
	params.Set("search", searchStr)
	params.Set("limit", strconv.Itoa(limit))
	params.Set("skip", strconv.Itoa(skip))

	var raw struct {
		Meta struct {
			Results struct {
				Total int `json:"total"`
				Skip  int `json:"skip"`
				Limit int `json:"limit"`
			} `json:"results"`
		} `json:"meta"`
		Results []map[string]interface{} `json:"results"`
	}
	if err := c.get(ctx, "/drug/label.json", params, &raw); err != nil {
		return nil, err
	}

	items := make([]DrugLabel, 0, len(raw.Results))
	for _, r := range raw.Results {
		items = append(items, mapDrugLabel(r))
	}
	return &SearchResult[DrugLabel]{
		Items: items,
		Total: raw.Meta.Results.Total,
		Skip:  raw.Meta.Results.Skip,
		Limit: raw.Meta.Results.Limit,
	}, nil
}

// ── Adverse Events ────────────────────────────────────────────────────────────

// SearchAdverseEvents searches FAERS (FDA Adverse Event Reporting System).
func (c *Client) SearchAdverseEvents(ctx context.Context, drugName string, limit, skip int) (*SearchResult[AdverseEvent], error) {
	if drugName == "" {
		return nil, fmt.Errorf("drugName is required")
	}
	limit = clamp(limit, 1, 100)
	if skip < 0 {
		skip = 0
	}

	params := url.Values{}
	params.Set("search", fmt.Sprintf(`patient.drug.medicinalproduct:"%s"`, drugName))
	params.Set("limit", strconv.Itoa(limit))
	params.Set("skip", strconv.Itoa(skip))

	var raw struct {
		Meta struct {
			Results struct {
				Total int `json:"total"`
				Skip  int `json:"skip"`
				Limit int `json:"limit"`
			} `json:"results"`
		} `json:"meta"`
		Results []map[string]interface{} `json:"results"`
	}
	if err := c.get(ctx, "/drug/event.json", params, &raw); err != nil {
		return nil, err
	}

	items := make([]AdverseEvent, 0, len(raw.Results))
	for _, r := range raw.Results {
		items = append(items, mapAdverseEvent(r))
	}
	return &SearchResult[AdverseEvent]{
		Items: items,
		Total: raw.Meta.Results.Total,
		Skip:  raw.Meta.Results.Skip,
		Limit: raw.Meta.Results.Limit,
	}, nil
}

// ── Drug Recalls ──────────────────────────────────────────────────────────────

// SearchRecalls searches FDA drug enforcement/recall records.
func (c *Client) SearchRecalls(ctx context.Context, query string, classification string, limit, skip int) (*SearchResult[DrugRecall], error) {
	limit = clamp(limit, 1, 100)
	if skip < 0 {
		skip = 0
	}

	searchParts := []string{}
	if query != "" {
		searchParts = append(searchParts, fmt.Sprintf(`(recalling_firm:"%s"+product_description:"%s")`, query, query))
	}
	if classification != "" {
		searchParts = append(searchParts, fmt.Sprintf(`classification:"Class %s"`, strings.ToUpper(classification)))
	}
	if len(searchParts) == 0 {
		searchParts = append(searchParts, `status:"Ongoing"`)
	}

	params := url.Values{}
	params.Set("search", strings.Join(searchParts, "+AND+"))
	params.Set("limit", strconv.Itoa(limit))
	params.Set("skip", strconv.Itoa(skip))

	var raw struct {
		Meta struct {
			Results struct {
				Total int `json:"total"`
				Skip  int `json:"skip"`
				Limit int `json:"limit"`
			} `json:"results"`
		} `json:"meta"`
		Results []map[string]interface{} `json:"results"`
	}
	if err := c.get(ctx, "/drug/enforcement.json", params, &raw); err != nil {
		return nil, err
	}

	items := make([]DrugRecall, 0, len(raw.Results))
	for _, r := range raw.Results {
		items = append(items, mapDrugRecall(r))
	}
	return &SearchResult[DrugRecall]{
		Items: items,
		Total: raw.Meta.Results.Total,
		Skip:  raw.Meta.Results.Skip,
		Limit: raw.Meta.Results.Limit,
	}, nil
}

// ── Mappers ───────────────────────────────────────────────────────────────────

func mapDrugLabel(r map[string]interface{}) DrugLabel {
	dl := DrugLabel{}

	// openfda sub-object
	if openfda, ok := r["openfda"].(map[string]interface{}); ok {
		dl.BrandName = firstStr(openfda, "brand_name")
		dl.GenericName = firstStr(openfda, "generic_name")
		dl.LabelerName = firstStr(openfda, "manufacturer_name")
		dl.ProductType = firstStr(openfda, "product_type")
		dl.Route = strSlice(openfda, "route")
		dl.Substance = strSlice(openfda, "substance_name")
	}

	dl.ID = strField(r, "id")
	dl.Indications = truncate(firstArrStr(r, "indications_and_usage"), 600)
	dl.Warnings = truncate(firstArrStr(r, "warnings"), 600)
	dl.Dosage = truncate(firstArrStr(r, "dosage_and_administration"), 400)
	dl.Contraindications = truncate(firstArrStr(r, "contraindications"), 400)
	dl.AdverseReactions = truncate(firstArrStr(r, "adverse_reactions"), 400)
	dl.EffectiveDate = strField(r, "effective_time")
	return dl
}

func mapAdverseEvent(r map[string]interface{}) AdverseEvent {
	ae := AdverseEvent{}
	ae.ReportID = strField(r, "safetyreportid")
	ae.ReceiveDate = strField(r, "receivedate")
	ae.Serious = strField(r, "serious") == "1"
	ae.Country = strField(r, "primarysource.reportercountry")

	// serious reasons
	reasons := []string{}
	for _, field := range []string{"seriousnessdeath", "seriousnesshospitalization", "seriousnesslifethreatening", "seriousnessdisabling"} {
		if strField(r, field) == "1" {
			reasons = append(reasons, strings.TrimPrefix(field, "seriousness"))
		}
	}
	ae.SeriousReasons = reasons

	// patient info
	if patient, ok := r["patient"].(map[string]interface{}); ok {
		if age := strField(patient, "patientonsetage"); age != "" {
			if v, err := strconv.ParseFloat(age, 64); err == nil {
				ae.Age = &v
			}
		}
		if sex := strField(patient, "patientsex"); sex != "" {
			switch sex {
			case "1":
				ae.Sex = "male"
			case "2":
				ae.Sex = "female"
			default:
				ae.Sex = "unknown"
			}
		}

		// drugs
		if drugs, ok := patient["drug"].([]interface{}); ok {
			drugNames := make([]string, 0, len(drugs))
			for _, d := range drugs {
				if dm, ok := d.(map[string]interface{}); ok {
					if name := strField(dm, "medicinalproduct"); name != "" {
						drugNames = append(drugNames, name)
					}
				}
			}
			if len(drugNames) > 10 {
				drugNames = drugNames[:10]
			}
			ae.Drugs = drugNames
		}

		// reactions
		if reactions, ok := patient["reaction"].([]interface{}); ok {
			rxNames := make([]string, 0, len(reactions))
			for _, rx := range reactions {
				if rxm, ok := rx.(map[string]interface{}); ok {
					if name := strField(rxm, "reactionmeddrapt"); name != "" {
						rxNames = append(rxNames, name)
					}
				}
			}
			if len(rxNames) > 10 {
				rxNames = rxNames[:10]
			}
			ae.Reactions = rxNames
		}
	}
	return ae
}

func mapDrugRecall(r map[string]interface{}) DrugRecall {
	return DrugRecall{
		RecallNumber:    strField(r, "recall_number"),
		Status:          strField(r, "status"),
		Classification:  strField(r, "classification"),
		RecallingFirm:   strField(r, "recalling_firm"),
		ProductDesc:     truncate(strField(r, "product_description"), 400),
		Reason:          truncate(strField(r, "reason_for_recall"), 400),
		Country:         strField(r, "country"),
		InitiationDate:  strField(r, "recall_initiation_date"),
		TerminationDate: strField(r, "termination_date"),
	}
}

// ── HTTP helper ───────────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, path string, params url.Values, dest interface{}) error {
	if c.apiKey != "" {
		params.Set("api_key", c.apiKey)
	}

	u := c.baseURL + path + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "api-suite/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		// OpenFDA returns 404 with {"error":{"code":"NOT_FOUND",...}} when no results
		return fmt.Errorf("no results found")
	}
	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if jsonErr := json.Unmarshal(body, &apiErr); jsonErr == nil && apiErr.Error.Message != "" {
			return fmt.Errorf("openFDA error: %s", apiErr.Error.Message)
		}
		return fmt.Errorf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}

// ── Field extraction helpers ──────────────────────────────────────────────────

func strField(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	}
	return fmt.Sprintf("%v", v)
}

func firstStr(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch x := v.(type) {
	case []interface{}:
		if len(x) > 0 {
			if s, ok := x[0].(string); ok {
				return s
			}
		}
	case string:
		return x
	}
	return ""
}

func strSlice(m map[string]interface{}, key string) []string {
	v, ok := m[key]
	if !ok {
		return nil
	}
	arr, ok := v.([]interface{})
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

func firstArrStr(m map[string]interface{}, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	switch x := v.(type) {
	case []interface{}:
		if len(x) > 0 {
			if s, ok := x[0].(string); ok {
				return s
			}
		}
	case string:
		return x
	}
	return ""
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}

func clamp(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
