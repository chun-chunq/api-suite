// Package client wraps the UK Companies House REST API.
// Docs: https://developer-specs.company-information.service.gov.uk/
// Auth: free API key from https://developer.company-information.service.gov.uk/
// License: Open Government Licence v3.0 (freely reusable)
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

const defaultBaseURL = "https://api.company-information.service.gov.uk"

// Company is a UK registered company from Companies House.
type Company struct {
	CompanyNumber  string   `json:"companyNumber"`  // e.g. "00102498"
	Name           string   `json:"name"`
	Status         string   `json:"status"`         // "active", "dissolved", "liquidation", etc.
	Type           string   `json:"type"`           // "ltd", "plc", "llp", etc.
	Jurisdiction   string   `json:"jurisdiction"`   // "england-wales", "scotland", etc.
	IncorporatedOn string   `json:"incorporatedOn,omitempty"`
	RegisteredAddress *Address `json:"registeredAddress,omitempty"`
	SICCodes       []string `json:"sicCodes,omitempty"`  // Standard Industrial Classification codes
	CanFile        bool     `json:"canFile"`
	URL            string   `json:"url"`
}

// Address is a UK registered office address.
type Address struct {
	AddressLine1 string `json:"addressLine1,omitempty"`
	AddressLine2 string `json:"addressLine2,omitempty"`
	Locality     string `json:"locality,omitempty"`  // town/city
	Region       string `json:"region,omitempty"`
	PostalCode   string `json:"postalCode,omitempty"`
	Country      string `json:"country,omitempty"`
}

// Officer is a director, secretary, or person with significant control.
type Officer struct {
	Name           string `json:"name"`
	Role           string `json:"role"`           // "director", "secretary", etc.
	AppointedOn    string `json:"appointedOn,omitempty"`
	ResignedOn     string `json:"resignedOn,omitempty"`
	Nationality    string `json:"nationality,omitempty"`
	Occupation     string `json:"occupation,omitempty"`
	CountryOfResidence string `json:"countryOfResidence,omitempty"`
	DateOfBirth    *DOB   `json:"dateOfBirth,omitempty"`
}

// DOB is a partial date of birth (month/year only, as returned by API).
type DOB struct {
	Month int `json:"month"`
	Year  int `json:"year"`
}

// SearchResult is paginated search results.
type SearchResult struct {
	Total      int       `json:"total"`
	StartIndex int       `json:"startIndex"`
	ItemsPerPage int     `json:"itemsPerPage"`
	Results    []Company `json:"results"`
}

// Client wraps the Companies House REST API.
type Client struct {
	http    *http.Client
	apiKey  string
	baseURL string
}

// New creates a new Companies House client.
// apiKey is required — get one free at developer.company-information.service.gov.uk
func New(apiKey string) *Client {
	return &Client{
		http:    &http.Client{Timeout: 15 * time.Second},
		apiKey:  apiKey,
		baseURL: defaultBaseURL,
	}
}

func (c *Client) get(ctx context.Context, path string, params url.Values) ([]byte, int, error) {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	// Companies House API uses HTTP Basic Auth: API key as username, empty password
	req.SetBasicAuth(c.apiKey, "")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("Companies House request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}

// Search searches for companies by name or company number.
func (c *Client) Search(ctx context.Context, query string, maxResults, startIndex int) (*SearchResult, error) {
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if maxResults <= 0 || maxResults > 100 {
		maxResults = 20
	}

	params := url.Values{}
	params.Set("q", query)
	params.Set("items_per_page", fmt.Sprintf("%d", maxResults))
	params.Set("start_index", fmt.Sprintf("%d", startIndex))

	body, status, err := c.get(ctx, "/search/companies", params)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("Companies House HTTP %d: %s", status, string(body))
	}

	var raw chSearchResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}

	companies := make([]Company, 0, len(raw.Items))
	for _, item := range raw.Items {
		companies = append(companies, mapSearchItem(item))
	}

	return &SearchResult{
		Total:        raw.TotalResults,
		StartIndex:   raw.StartIndex,
		ItemsPerPage: raw.ItemsPerPage,
		Results:      companies,
	}, nil
}

// GetByNumber fetches a company profile by its company number.
func (c *Client) GetByNumber(ctx context.Context, companyNumber string) (*Company, error) {
	companyNumber = strings.ToUpper(strings.TrimSpace(companyNumber))
	if companyNumber == "" {
		return nil, fmt.Errorf("companyNumber is required")
	}

	body, status, err := c.get(ctx, "/company/"+url.PathEscape(companyNumber), nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("Companies House HTTP %d", status)
	}

	var raw chCompanyProfile
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode company profile: %w", err)
	}

	co := mapCompanyProfile(raw)
	return &co, nil
}

// GetOfficers fetches current and resigned officers for a company.
func (c *Client) GetOfficers(ctx context.Context, companyNumber string, activeOnly bool) ([]Officer, error) {
	companyNumber = strings.ToUpper(strings.TrimSpace(companyNumber))
	params := url.Values{}
	if activeOnly {
		params.Set("register_type", "directors")
	}
	params.Set("items_per_page", "50")

	body, status, err := c.get(ctx, "/company/"+url.PathEscape(companyNumber)+"/officers", params)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("Companies House officers HTTP %d", status)
	}

	var raw chOfficerList
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode officers: %w", err)
	}

	officers := make([]Officer, 0, len(raw.Items))
	for _, item := range raw.Items {
		if activeOnly && item.ResignedOn != "" {
			continue
		}
		officers = append(officers, mapOfficer(item))
	}
	return officers, nil
}

// ── Raw Companies House JSON structures ────────────────────────────────────────

type chSearchResponse struct {
	TotalResults int              `json:"total_results"`
	StartIndex   int              `json:"start_index"`
	ItemsPerPage int              `json:"items_per_page"`
	Items        []chSearchItem   `json:"items"`
}

type chSearchItem struct {
	CompanyNumber  string `json:"company_number"`
	Title          string `json:"title"`
	CompanyStatus  string `json:"company_status"`
	CompanyType    string `json:"company_type"`
	Kind           string `json:"kind"`
	DateOfCreation string `json:"date_of_creation"`
	Description    string `json:"description"`
	Snippet        string `json:"snippet"`
	RegisteredOfficeAddress *chAddress `json:"registered_office_address"`
}

type chCompanyProfile struct {
	CompanyNumber         string     `json:"company_number"`
	CompanyName           string     `json:"company_name"`
	CompanyStatus         string     `json:"company_status"`
	Type                  string     `json:"type"`
	Jurisdiction          string     `json:"jurisdiction"`
	DateOfCreation        string     `json:"date_of_creation"`
	CanFile               bool       `json:"can_file"`
	RegisteredOfficeAddress *chAddress `json:"registered_office_address"`
	SICCodes              []string   `json:"sic_codes"`
}

type chAddress struct {
	AddressLine1 string `json:"address_line_1"`
	AddressLine2 string `json:"address_line_2"`
	Locality     string `json:"locality"`
	Region       string `json:"region"`
	PostalCode   string `json:"postal_code"`
	Country      string `json:"country"`
}

type chOfficerList struct {
	ActiveCount   int         `json:"active_count"`
	ResignedCount int         `json:"resigned_count"`
	TotalResults  int         `json:"total_results"`
	Items         []chOfficer `json:"items"`
}

type chOfficer struct {
	Name        string `json:"name"`
	OfficerRole string `json:"officer_role"`
	AppointedOn string `json:"appointed_on"`
	ResignedOn  string `json:"resigned_on"`
	Nationality string `json:"nationality"`
	Occupation  string `json:"occupation"`
	CountryOfResidence string `json:"country_of_residence"`
	DateOfBirth *struct {
		Month int `json:"month"`
		Year  int `json:"year"`
	} `json:"date_of_birth"`
}

func mapSearchItem(r chSearchItem) Company {
	co := Company{
		CompanyNumber:  r.CompanyNumber,
		Name:           r.Title,
		Status:         r.CompanyStatus,
		Type:           r.CompanyType,
		IncorporatedOn: r.DateOfCreation,
		URL:            fmt.Sprintf("https://find-and-update.company-information.service.gov.uk/company/%s", r.CompanyNumber),
	}
	if r.RegisteredOfficeAddress != nil {
		co.RegisteredAddress = mapAddress(r.RegisteredOfficeAddress)
	}
	return co
}

func mapCompanyProfile(r chCompanyProfile) Company {
	co := Company{
		CompanyNumber:  r.CompanyNumber,
		Name:           r.CompanyName,
		Status:         r.CompanyStatus,
		Type:           r.Type,
		Jurisdiction:   r.Jurisdiction,
		IncorporatedOn: r.DateOfCreation,
		SICCodes:       r.SICCodes,
		CanFile:        r.CanFile,
		URL:            fmt.Sprintf("https://find-and-update.company-information.service.gov.uk/company/%s", r.CompanyNumber),
	}
	if r.RegisteredOfficeAddress != nil {
		co.RegisteredAddress = mapAddress(r.RegisteredOfficeAddress)
	}
	return co
}

func mapAddress(a *chAddress) *Address {
	if a == nil {
		return nil
	}
	return &Address{
		AddressLine1: a.AddressLine1,
		AddressLine2: a.AddressLine2,
		Locality:     a.Locality,
		Region:       a.Region,
		PostalCode:   a.PostalCode,
		Country:      a.Country,
	}
}

func mapOfficer(r chOfficer) Officer {
	o := Officer{
		Name:               r.Name,
		Role:               r.OfficerRole,
		AppointedOn:        r.AppointedOn,
		ResignedOn:         r.ResignedOn,
		Nationality:        r.Nationality,
		Occupation:         r.Occupation,
		CountryOfResidence: r.CountryOfResidence,
	}
	if r.DateOfBirth != nil {
		o.DateOfBirth = &DOB{Month: r.DateOfBirth.Month, Year: r.DateOfBirth.Year}
	}
	return o
}
