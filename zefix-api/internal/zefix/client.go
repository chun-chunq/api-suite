// Package zefix wraps the official Swiss Zefix REST API.
// Official API: https://www.zefix.admin.ch/ZefixPublicREST/swagger-ui/index.html
// License: Swiss Open Government Data (OGD) — freely reusable
package zefix

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

const defaultBaseURL = "https://www.zefix.admin.ch/ZefixPublicREST"

// Company represents a Swiss registered company from Zefix.
type Company struct {
	UID         string   `json:"uid"`          // e.g. "CHE-100.000.123"
	Name        string   `json:"name"`
	LegalForm   string   `json:"legalForm,omitempty"`
	Status      string   `json:"status"`       // "ACTIVE" | "DELETED" | "MOVED"
	Canton      string   `json:"canton"`       // 2-letter e.g. "ZH", "BE"
	Municipality string  `json:"municipality,omitempty"`
	Address     *Address `json:"address,omitempty"`
	RegisteredAt string  `json:"registeredAt,omitempty"` // canton register date
	PurposeDE   string   `json:"purposeDe,omitempty"` // company purpose (German)
	PurposeFR   string   `json:"purposeFr,omitempty"`
	SHABPublications []SHABPublication `json:"shabPublications,omitempty"`
}

// Address is a structured Swiss company address.
type Address struct {
	Street     string `json:"street,omitempty"`
	HouseNr    string `json:"houseNumber,omitempty"`
	ZIP        string `json:"swissZipCode,omitempty"`
	City       string `json:"city,omitempty"`
	Country    string `json:"country,omitempty"`
}

// SHABPublication is a Swiss Federal Gazette announcement.
type SHABPublication struct {
	Date     string `json:"date"`
	Nr       string `json:"nr"`
	Title    string `json:"title,omitempty"`
	Category string `json:"category,omitempty"`
}

// SearchResult is the response for a company search.
type SearchResult struct {
	Total     int       `json:"total"`
	Results   []Company `json:"results"`
	Query     string    `json:"query"`
	DataSource string   `json:"dataSource"`
}

// Client wraps the Zefix public REST API.
type Client struct {
	http    *http.Client
	baseURL string // overridable for tests
}

// New creates a new Zefix client.
func New() *Client {
	return &Client{
		http:    &http.Client{Timeout: 15 * time.Second},
		baseURL: defaultBaseURL,
	}
}

// Search searches for companies by name.
// lang: "de" | "fr" | "it" | "en"
// activeOnly: if true, only returns ACTIVE companies
func (c *Client) Search(ctx context.Context, name, lang string, activeOnly bool, maxResults int) ([]Company, error) {
	if lang == "" {
		lang = "de"
	}
	if maxResults <= 0 || maxResults > 200 {
		maxResults = 50
	}

	// Zefix search endpoint
	endpoint := fmt.Sprintf("%s/company.json", c.baseURL)
	params := url.Values{}
	params.Set("name", name)
	params.Set("lang", lang)
	params.Set("maxEntries", fmt.Sprintf("%d", maxResults))
	if activeOnly {
		params.Set("searchType", "exact") // Zefix: "exact" filters better
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Zefix API HTTP %d: %s", resp.StatusCode, string(body))
	}

	var raw []zefixCompanyRaw
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode Zefix response: %w", err)
	}

	companies := make([]Company, 0, len(raw))
	for _, r := range raw {
		co := mapCompany(r)
		if activeOnly && !strings.EqualFold(co.Status, "ACTIVE") {
			continue
		}
		companies = append(companies, co)
	}
	return companies, nil
}

// GetByUID fetches a single company by its Swiss UID (CHE-xxx.xxx.xxx).
func (c *Client) GetByUID(ctx context.Context, uid string) (*Company, error) {
	// Normalize UID format
	uid = strings.ReplaceAll(uid, " ", "")
	if !strings.HasPrefix(strings.ToUpper(uid), "CHE") {
		return nil, fmt.Errorf("invalid UID format — expected CHE-xxx.xxx.xxx")
	}

	endpoint := fmt.Sprintf("%s/company/%s.json", c.baseURL, url.PathEscape(uid))
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Zefix API HTTP %d", resp.StatusCode)
	}

	var raw zefixCompanyRaw
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	co := mapCompany(raw)
	return &co, nil
}

// GetPublications fetches SHAB gazette publications for a company UID.
func (c *Client) GetPublications(ctx context.Context, uid string) ([]SHABPublication, error) {
	uid = strings.ReplaceAll(uid, " ", "")
	endpoint := fmt.Sprintf("%s/company/%s/shab.json", c.baseURL, url.PathEscape(uid))
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Zefix SHAB HTTP %d", resp.StatusCode)
	}

	var raw []shabRaw
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}

	pubs := make([]SHABPublication, 0, len(raw))
	for _, r := range raw {
		pubs = append(pubs, SHABPublication{
			Date:     r.PublicationDate,
			Nr:       r.ShabNr,
			Title:    r.Title,
			Category: r.Category,
		})
	}
	return pubs, nil
}

// ── Raw Zefix JSON structures ──────────────────────────────────────────────────

type zefixCompanyRaw struct {
	UID       string `json:"uid"`
	Name      string `json:"name"`
	LegalForm struct {
		NameDE string `json:"nameDe"`
	} `json:"legalForm"`
	Status    string `json:"status"`
	Canton    string `json:"canton"`
	Address   *struct {
		Street      string `json:"street"`
		HouseNumber string `json:"houseNumber"`
		SwissZip    string `json:"swissZipCode"`
		City        string `json:"city"`
		CountryCode string `json:"countryCode"`
	} `json:"address"`
	RegisteredOffice struct {
		Municipality string `json:"municipalityDe"`
	} `json:"registeredOffice"`
	Purpose struct {
		DE string `json:"de"`
		FR string `json:"fr"`
	} `json:"purpose"`
	DateOfRegistration string `json:"dateOfRegistration"`
}

type shabRaw struct {
	PublicationDate string `json:"publicationDate"`
	ShabNr          string `json:"shabNr"`
	Title           string `json:"title"`
	Category        string `json:"category"`
}

func mapCompany(r zefixCompanyRaw) Company {
	co := Company{
		UID:          r.UID,
		Name:         r.Name,
		LegalForm:    r.LegalForm.NameDE,
		Status:       r.Status,
		Canton:       r.Canton,
		Municipality: r.RegisteredOffice.Municipality,
		RegisteredAt: r.DateOfRegistration,
		PurposeDE:    r.Purpose.DE,
		PurposeFR:    r.Purpose.FR,
	}
	if r.Address != nil {
		co.Address = &Address{
			Street:  r.Address.Street,
			HouseNr: r.Address.HouseNumber,
			ZIP:     r.Address.SwissZip,
			City:    r.Address.City,
			Country: r.Address.CountryCode,
		}
	}
	return co
}
