// Package client wraps the official GLEIF REST API (free, no auth needed).
// API docs: https://api.gleif.org/api/v1/
// LEI (Legal Entity Identifier) is required for all MiFID II financial transactions.
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

const baseURL = "https://api.gleif.org/api/v1"

// Entity is a GLEIF legal entity (company with an LEI code).
type Entity struct {
	LEI          string   `json:"lei"`
	Name         string   `json:"name"`
	LegalName    string   `json:"legalName"`
	Status       string   `json:"status"`         // "ACTIVE" | "INACTIVE" | "PENDING_TRANSFER" | "PENDING_ARCHIVAL"
	LegalForm    string   `json:"legalForm,omitempty"`
	Jurisdiction string   `json:"jurisdiction,omitempty"` // e.g. "DE", "US-NY"
	Category     string   `json:"category,omitempty"`     // "BRANCH" | "FUND" | "SOLE_PROPRIETOR" | "GENERAL"
	RegisteredAddress  *Address `json:"registeredAddress,omitempty"`
	HeadquartersAddress *Address `json:"headquartersAddress,omitempty"`
	RegistrationAuthority *RegistrationAuthority `json:"registrationAuthority,omitempty"`
	BICCodes     []string `json:"bicCodes,omitempty"` // SWIFT/BIC codes if linked
	CreatedAt    string   `json:"createdAt,omitempty"`
	UpdatedAt    string   `json:"updatedAt,omitempty"`
	NextRenewalDate string `json:"nextRenewalDate,omitempty"`
	ManagingOU   string   `json:"managingOU,omitempty"` // Local Operating Unit managing this LEI
}

// Address is a structured entity address.
type Address struct {
	Lines   []string `json:"lines,omitempty"`
	City    string   `json:"city,omitempty"`
	Region  string   `json:"region,omitempty"`
	Country string   `json:"country,omitempty"` // ISO 2-letter
	PostalCode string `json:"postalCode,omitempty"`
}

// RegistrationAuthority is the register where the company is incorporated.
type RegistrationAuthority struct {
	RegistrationAuthorityID   string `json:"id,omitempty"`
	RegistrationAuthorityName string `json:"name,omitempty"`
	RegistrationID            string `json:"registrationId,omitempty"` // company number in that register
}

// RelationshipSummary shows parent/child relationships.
type RelationshipSummary struct {
	DirectParent    *Entity `json:"directParent,omitempty"`
	UltimateParent  *Entity `json:"ultimateParent,omitempty"`
	DirectChildren  []Entity `json:"directChildren,omitempty"`
}

// SearchResult is the response for a name/country search.
type SearchResult struct {
	Total      int      `json:"total"`
	Results    []Entity `json:"results"`
	Query      string   `json:"query"`
	DataSource string   `json:"dataSource"`
}

// Client wraps the GLEIF REST API.
type Client struct {
	http *http.Client
}

// New creates a GLEIF client.
func New() *Client {
	return &Client{
		http: &http.Client{Timeout: 15 * time.Second},
	}
}

// SearchByName finds entities by legal name.
// country: optional ISO-2 filter (e.g. "DE")
// activeOnly: if true, only ACTIVE LEIs returned
func (c *Client) SearchByName(ctx context.Context, name, country string, activeOnly bool, maxResults int) ([]Entity, int, error) {
	if maxResults <= 0 || maxResults > 200 {
		maxResults = 50
	}
	params := url.Values{}
	params.Set("filter[fulltext]", name)
	params.Set("page[size]", fmt.Sprintf("%d", maxResults))
	params.Set("page[number]", "1")
	if activeOnly {
		params.Set("filter[entity.status]", "ACTIVE")
	}
	if country != "" {
		params.Set("filter[entity.legalAddress.country]", strings.ToUpper(country))
	}

	return c.searchRaw(ctx, "/lei-records?"+params.Encode())
}

// GetByLEI fetches a single entity by its LEI code.
func (c *Client) GetByLEI(ctx context.Context, lei string) (*Entity, error) {
	lei = strings.ToUpper(strings.TrimSpace(lei))
	if len(lei) != 20 {
		return nil, fmt.Errorf("invalid LEI: must be exactly 20 characters, got %d", len(lei))
	}

	body, err := c.get(ctx, fmt.Sprintf("/lei-records/%s", lei))
	if err != nil {
		return nil, err
	}

	var resp gleifSingleResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("decode GLEIF response: %w", err)
	}
	if resp.Data == nil {
		return nil, nil
	}
	e := mapRecord(*resp.Data)
	return &e, nil
}

// GetRelationships fetches the ownership chain for a given LEI.
func (c *Client) GetRelationships(ctx context.Context, lei string) (*RelationshipSummary, error) {
	lei = strings.ToUpper(strings.TrimSpace(lei))
	summary := &RelationshipSummary{}

	// Direct parent
	dpBody, err := c.get(ctx, fmt.Sprintf("/lei-records/%s/direct-parent", lei))
	if err == nil {
		var resp gleifSingleResponse
		if json.Unmarshal(dpBody, &resp) == nil && resp.Data != nil {
			e := mapRecord(*resp.Data)
			summary.DirectParent = &e
		}
	}
	// Ultimate parent
	upBody, err := c.get(ctx, fmt.Sprintf("/lei-records/%s/ultimate-parent", lei))
	if err == nil {
		var resp gleifSingleResponse
		if json.Unmarshal(upBody, &resp) == nil && resp.Data != nil {
			e := mapRecord(*resp.Data)
			summary.UltimateParent = &e
		}
	}
	// Direct children (first 10)
	dcBody, err := c.get(ctx, fmt.Sprintf("/lei-records/%s/direct-children?page[size]=10", lei))
	if err == nil {
		children, _, err := c.parseList(dcBody)
		if err == nil {
			summary.DirectChildren = children
		}
	}
	return summary, nil
}

func (c *Client) searchRaw(ctx context.Context, path string) ([]Entity, int, error) {
	body, err := c.get(ctx, path)
	if err != nil {
		return nil, 0, err
	}
	return c.parseList(body)
}

func (c *Client) parseList(body []byte) ([]Entity, int, error) {
	var resp gleifListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("decode GLEIF list: %w", err)
	}
	total := 0
	if resp.Meta != nil {
		total = resp.Meta.Total
	}
	entities := make([]Entity, 0, len(resp.Data))
	for _, r := range resp.Data {
		entities = append(entities, mapRecord(r))
	}
	return entities, total, nil
}

func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	u := baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.api+json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GLEIF API: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GLEIF API HTTP %d: %s", resp.StatusCode, string(body[:min(200, len(body))]))
	}
	return body, nil
}

// ── Raw GLEIF JSON-API response structures ─────────────────────────────────

type gleifListResponse struct {
	Data []gleifRecord `json:"data"`
	Meta *struct {
		Total int `json:"total"`
	} `json:"meta"`
}

type gleifSingleResponse struct {
	Data *gleifRecord `json:"data"`
}

type gleifRecord struct {
	ID         string `json:"id"` // = LEI
	Type       string `json:"type"`
	Attributes struct {
		LEI    string `json:"lei"`
		Entity struct {
			LegalName struct {
				Name string `json:"name"`
			} `json:"legalName"`
			OtherNames []struct {
				Name     string `json:"name"`
				Language string `json:"language"`
			} `json:"otherNames"`
			LegalAddress struct {
				Lines      []string `json:"addressLines"`
				City       string   `json:"city"`
				Region     string   `json:"region"`
				PostalCode string   `json:"postalCode"`
				Country    string   `json:"country"`
			} `json:"legalAddress"`
			HeadquartersAddress struct {
				Lines      []string `json:"addressLines"`
				City       string   `json:"city"`
				Region     string   `json:"region"`
				PostalCode string   `json:"postalCode"`
				Country    string   `json:"country"`
			} `json:"headquartersAddress"`
			Status       string `json:"status"`
			LegalForm    struct{ ID string `json:"id"` } `json:"legalForm"`
			EntityCategory string `json:"entityCategory"`
			Jurisdiction   string `json:"jurisdiction"`
		} `json:"entity"`
		Registration struct {
			InitialRegistrationDate  string `json:"initialRegistrationDate"`
			LastUpdateDate           string `json:"lastUpdateDate"`
			NextRenewalDate          string `json:"nextRenewalDate"`
			ManagingLOU              string `json:"managingLou"`
		} `json:"registration"`
		BICCodes []string `json:"bic"`
	} `json:"attributes"`
}

func mapRecord(r gleifRecord) Entity {
	attr := r.Attributes
	ent := attr.Entity

	regAddr := &Address{
		Lines:      ent.LegalAddress.Lines,
		City:       ent.LegalAddress.City,
		Region:     ent.LegalAddress.Region,
		Country:    ent.LegalAddress.Country,
		PostalCode: ent.LegalAddress.PostalCode,
	}
	hqAddr := &Address{
		Lines:      ent.HeadquartersAddress.Lines,
		City:       ent.HeadquartersAddress.City,
		Region:     ent.HeadquartersAddress.Region,
		Country:    ent.HeadquartersAddress.Country,
		PostalCode: ent.HeadquartersAddress.PostalCode,
	}

	lei := r.ID
	if attr.LEI != "" {
		lei = attr.LEI
	}

	return Entity{
		LEI:                  lei,
		Name:                 ent.LegalName.Name,
		LegalName:            ent.LegalName.Name,
		Status:               ent.Status,
		LegalForm:            ent.LegalForm.ID,
		Jurisdiction:         ent.Jurisdiction,
		Category:             ent.EntityCategory,
		RegisteredAddress:    regAddr,
		HeadquartersAddress:  hqAddr,
		BICCodes:             attr.BICCodes,
		CreatedAt:            attr.Registration.InitialRegistrationDate,
		UpdatedAt:            attr.Registration.LastUpdateDate,
		NextRenewalDate:      attr.Registration.NextRenewalDate,
		ManagingOU:           attr.Registration.ManagingLOU,
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
