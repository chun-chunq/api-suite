// Package client wraps the EU VIES VAT validation REST API.
// Docs: https://ec.europa.eu/taxation_customs/vies/#/technical-information
// No API key required — official EU public service.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	viesBase       = "https://ec.europa.eu/taxation_customs/vies/rest-api"
	defaultTimeout = 15 * time.Second
)

// EU member state codes accepted by VIES.
var validCountryCodes = map[string]bool{
	"AT": true, "BE": true, "BG": true, "CY": true, "CZ": true,
	"DE": true, "DK": true, "EE": true, "EL": true, "ES": true,
	"FI": true, "FR": true, "HR": true, "HU": true, "IE": true,
	"IT": true, "LT": true, "LU": true, "LV": true, "MT": true,
	"NL": true, "PL": true, "PT": true, "RO": true, "SE": true,
	"SI": true, "SK": true,
	// Greece uses EL in VIES (alias for GR)
	"GR": true,
}

// VATResult is the validated result for one VAT number.
type VATResult struct {
	CountryCode string `json:"country_code"`
	VATNumber   string `json:"vat_number"`   // without country prefix
	FullVAT     string `json:"full_vat"`     // e.g. "DE123456789"
	Valid        bool   `json:"valid"`
	Name        string `json:"name,omitempty"`
	Address     string `json:"address,omitempty"`
	RequestDate string `json:"request_date,omitempty"`
	UserError   string `json:"user_error,omitempty"`
}

// viesResponse mirrors the VIES REST API JSON response.
type viesResponse struct {
	IsValid     bool   `json:"isValid"`
	RequestDate string `json:"requestDate"`
	UserError   string `json:"userError"`
	Name        string `json:"name"`
	Address     string `json:"address"`
	VATNumber   string `json:"vat_number"`
	TraderName  string `json:"traderName"`
	TraderAddr  string `json:"traderAddress"`
}

// viesErrorResponse is returned by VIES on invalid requests.
type viesErrorResponse struct {
	ActionSucceed bool   `json:"actionSucceed"`
	ErrorWrappers []struct {
		Error string `json:"error"`
	} `json:"errorWrappers"`
}

// Client is the VIES API client.
type Client struct {
	http    *http.Client
	baseURL string
}

// New returns a new VIES client.
func New() *Client {
	return &Client{
		http:    &http.Client{Timeout: defaultTimeout},
		baseURL: viesBase,
	}
}

// ValidateVAT validates a VAT number via the VIES REST API.
// vatNumber may include or omit the country prefix (e.g. "DE123456789" or "123456789").
// countryCode is the 2-letter EU country code (e.g. "DE", "FR", "LU").
func (c *Client) ValidateVAT(ctx context.Context, countryCode, vatNumber string) (*VATResult, error) {
	cc := strings.ToUpper(strings.TrimSpace(countryCode))

	// Normalize GR → EL (VIES uses EL for Greece)
	if cc == "GR" {
		cc = "EL"
	}
	if !validCountryCodes[cc] {
		return nil, fmt.Errorf("unsupported country code: %q (must be EU member state)", countryCode)
	}

	// Strip country prefix from VAT number if present
	vat := strings.ToUpper(strings.TrimSpace(vatNumber))
	vat = strings.TrimSpace(strings.TrimPrefix(vat, cc))
	// Also strip GR prefix for Greece
	if countryCode == "GR" || countryCode == "gr" {
		vat = strings.TrimPrefix(vat, "GR")
	}
	vat = strings.ReplaceAll(vat, " ", "")
	vat = strings.ReplaceAll(vat, "-", "")
	vat = strings.ReplaceAll(vat, ".", "")

	if vat == "" {
		return nil, fmt.Errorf("vat_number is required")
	}

	url := fmt.Sprintf("%s/ms/%s/vat/%s", c.baseURL, cc, vat)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("VIES request: %w", err)
	}
	defer resp.Body.Close()

	// VIES returns 200 even for invalid VAT numbers, but uses non-200 for bad requests
	if resp.StatusCode >= 400 {
		var errResp viesErrorResponse
		if jsonErr := json.NewDecoder(resp.Body).Decode(&errResp); jsonErr == nil && len(errResp.ErrorWrappers) > 0 {
			return nil, fmt.Errorf("VIES error: %s", errResp.ErrorWrappers[0].Error)
		}
		return nil, fmt.Errorf("VIES returned HTTP %d", resp.StatusCode)
	}

	var raw viesResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode VIES response: %w", err)
	}

	// Prefer traderName/traderAddress (more consistent) over name/address
	name := strings.TrimSpace(raw.TraderName)
	if name == "" || name == "---" {
		name = strings.TrimSpace(raw.Name)
	}
	if name == "---" {
		name = ""
	}

	addr := strings.TrimSpace(raw.TraderAddr)
	if addr == "" || addr == "---" {
		addr = strings.TrimSpace(raw.Address)
	}
	if addr == "---" {
		addr = ""
	}
	// Clean up address: collapse multiple newlines / leading spaces
	addr = strings.Join(strings.Fields(addr), " ")

	// Parse request date (strip timezone suffix like "+01:00")
	reqDate := raw.RequestDate
	if idx := strings.Index(reqDate, "+"); idx > 0 {
		reqDate = reqDate[:idx]
	}

	return &VATResult{
		CountryCode: cc,
		VATNumber:   vat,
		FullVAT:     cc + vat,
		Valid:        raw.IsValid,
		Name:        name,
		Address:     addr,
		RequestDate: reqDate,
		UserError:   raw.UserError,
	}, nil
}

// BatchResult wraps results for batch validation.
type BatchResult struct {
	Results []BatchItem `json:"results"`
	Total   int         `json:"total"`
	Valid   int         `json:"valid"`
	Invalid int         `json:"invalid"`
}

// BatchItem is a single item in a batch validation response.
type BatchItem struct {
	Input  string     `json:"input"`
	Result *VATResult `json:"result,omitempty"`
	Error  string     `json:"error,omitempty"`
}

// ValidateBatch validates up to 10 VAT numbers concurrently.
// Each entry in vatNumbers should be a full VAT number like "DE123456789".
func (c *Client) ValidateBatch(ctx context.Context, vatNumbers []string) (*BatchResult, error) {
	if len(vatNumbers) == 0 {
		return nil, fmt.Errorf("at_least_one_vat_number_required")
	}
	if len(vatNumbers) > 10 {
		return nil, fmt.Errorf("batch limited to 10 VAT numbers per request")
	}

	type result struct {
		idx  int
		item BatchItem
	}
	ch := make(chan result, len(vatNumbers))

	for i, vn := range vatNumbers {
		i, vn := i, strings.TrimSpace(vn)
		go func() {
			item := BatchItem{Input: vn}
			cc, number, err := splitVAT(vn)
			if err != nil {
				item.Error = err.Error()
			} else {
				r, err := c.ValidateVAT(ctx, cc, number)
				if err != nil {
					item.Error = err.Error()
				} else {
					item.Result = r
				}
			}
			ch <- result{idx: i, item: item}
		}()
	}

	items := make([]BatchItem, len(vatNumbers))
	for range vatNumbers {
		r := <-ch
		items[r.idx] = r.item
	}

	br := &BatchResult{
		Results: items,
		Total:   len(items),
	}
	for _, it := range items {
		if it.Result != nil && it.Result.Valid {
			br.Valid++
		} else if it.Error == "" && it.Result != nil {
			br.Invalid++
		}
	}

	return br, nil
}

// splitVAT splits a full VAT string like "DE123456789" into country code + number.
func splitVAT(vat string) (cc, number string, err error) {
	vat = strings.ToUpper(strings.TrimSpace(vat))
	vat = strings.ReplaceAll(vat, " ", "")
	vat = strings.ReplaceAll(vat, "-", "")
	if len(vat) < 3 {
		return "", "", fmt.Errorf("VAT number too short: %q", vat)
	}
	cc = vat[:2]
	number = vat[2:]
	if !validCountryCodes[cc] {
		return "", "", fmt.Errorf("unknown country code %q in VAT %q", cc, vat)
	}
	return cc, number, nil
}

// ValidCountryCodes returns the list of supported EU member state codes.
func ValidCountryCodes() []string {
	codes := make([]string, 0, len(validCountryCodes))
	for cc := range validCountryCodes {
		codes = append(codes, cc)
	}
	return codes
}
