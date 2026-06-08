// Package client wraps the GBIF (Global Biodiversity Information Facility) API.
// Docs: https://www.gbif.org/developer/summary
// No API key required for read-only access. Free and open.
// Rate limit: be polite, max ~60 req/min.
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
	defaultBase    = "https://api.gbif.org/v1"
	defaultTimeout = 15 * time.Second
)

// Species holds taxonomic information about a species.
type Species struct {
	Key           int      `json:"key"`
	NubKey        int      `json:"nub_key,omitempty"`
	NameKey       int      `json:"name_key,omitempty"`
	TaxonID       string   `json:"taxon_id,omitempty"`
	Kingdom       string   `json:"kingdom,omitempty"`
	Phylum        string   `json:"phylum,omitempty"`
	Class         string   `json:"class,omitempty"`
	Order         string   `json:"order,omitempty"`
	Family        string   `json:"family,omitempty"`
	Genus         string   `json:"genus,omitempty"`
	Species       string   `json:"species,omitempty"`
	CanonicalName string   `json:"canonical_name,omitempty"`
	ScientificName string  `json:"scientific_name"`
	AuthorShip    string   `json:"authorship,omitempty"`
	NameType      string   `json:"name_type,omitempty"`
	Rank          string   `json:"rank,omitempty"`
	TaxonomicStatus string `json:"taxonomic_status,omitempty"`
	Extinct       bool     `json:"extinct"`
	VernacularNames []string `json:"vernacular_names,omitempty"` // common names in English
	NumDescendants int      `json:"num_descendants,omitempty"`
}

// Occurrence is a single observation record.
type Occurrence struct {
	Key          int     `json:"key"`
	DatasetKey   string  `json:"dataset_key,omitempty"`
	ScientificName string `json:"scientific_name"`
	Kingdom      string  `json:"kingdom,omitempty"`
	Family       string  `json:"family,omitempty"`
	Genus        string  `json:"genus,omitempty"`
	Species      string  `json:"species,omitempty"`
	CountryCode  string  `json:"country_code,omitempty"`
	Country      string  `json:"country,omitempty"`
	StateProvince string `json:"state_province,omitempty"`
	Locality     string  `json:"locality,omitempty"`
	Latitude     *float64 `json:"latitude,omitempty"`
	Longitude    *float64 `json:"longitude,omitempty"`
	Year         int     `json:"year,omitempty"`
	Month        int     `json:"month,omitempty"`
	Day          int     `json:"day,omitempty"`
	EventDate    string  `json:"event_date,omitempty"`
	BasisOfRecord string `json:"basis_of_record,omitempty"`
	MediaType    []string `json:"media_type,omitempty"` // "StillImage", "MovingImage", etc.
	MediaURLs    []string `json:"media_urls,omitempty"`
}

// OccurrenceResult is a paginated list of occurrences.
type OccurrenceResult struct {
	Query      map[string]string `json:"query"`
	Total      int               `json:"total"`
	Offset     int               `json:"offset"`
	Limit      int               `json:"limit"`
	HasMore    bool              `json:"has_more"`
	Results    []Occurrence      `json:"results"`
}

// SpeciesSearchResult is a paginated list of species.
type SpeciesSearchResult struct {
	Query   string    `json:"query"`
	Total   int       `json:"total"`
	Offset  int       `json:"offset"`
	Limit   int       `json:"limit"`
	HasMore bool      `json:"has_more"`
	Results []Species `json:"results"`
}

// Client is the GBIF API client.
type Client struct {
	http    *http.Client
	baseURL string
}

// New returns a new GBIF client.
func New() *Client {
	return &Client{
		http:    &http.Client{Timeout: defaultTimeout},
		baseURL: defaultBase,
	}
}

// ── Internal helpers ─────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, path string, params url.Values, out interface{}) error {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
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
	if resp.StatusCode == http.StatusBadRequest {
		var errResp struct {
			Message string `json:"message"`
		}
		if jsonErr := json.NewDecoder(resp.Body).Decode(&errResp); jsonErr == nil && errResp.Message != "" {
			return fmt.Errorf("GBIF error: %s", errResp.Message)
		}
		return fmt.Errorf("upstream returned HTTP 400")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

// ── Public API ───────────────────────────────────────────────────────────────

// SearchSpecies searches the GBIF backbone taxonomy by name.
func (c *Client) SearchSpecies(ctx context.Context, query string, rank string, limit, offset int) (*SpeciesSearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	params := url.Values{
		"q":      []string{query},
		"limit":  []string{fmt.Sprintf("%d", limit)},
		"offset": []string{fmt.Sprintf("%d", offset)},
	}
	if rank != "" {
		params.Set("rank", strings.ToUpper(rank))
	}

	var raw struct {
		Count   int `json:"count"`
		Offset  int `json:"offset"`
		Limit   int `json:"limit"`
		Results []struct {
			Key            int    `json:"key"`
			NubKey         int    `json:"nubKey"`
			NameKey        int    `json:"nameKey"`
			TaxonID        string `json:"taxonID"`
			Kingdom        string `json:"kingdom"`
			Phylum         string `json:"phylum"`
			Class          string `json:"class"`
			Order          string `json:"order"`
			Family         string `json:"family"`
			Genus          string `json:"genus"`
			Species        string `json:"species"`
			CanonicalName  string `json:"canonicalName"`
			ScientificName string `json:"scientificName"`
			Authorship     string `json:"authorship"`
			NameType       string `json:"nameType"`
			Rank           string `json:"rank"`
			TaxonomicStatus string `json:"taxonomicStatus"`
			Extinct        bool   `json:"extinct"`
			NumDescendants int    `json:"numDescendants"`
		} `json:"results"`
	}

	if err := c.get(ctx, "/species/search", params, &raw); err != nil {
		return nil, err
	}

	results := make([]Species, len(raw.Results))
	for i, r := range raw.Results {
		results[i] = Species{
			Key:             r.Key,
			NubKey:          r.NubKey,
			NameKey:         r.NameKey,
			TaxonID:         r.TaxonID,
			Kingdom:         r.Kingdom,
			Phylum:          r.Phylum,
			Class:           r.Class,
			Order:           r.Order,
			Family:          r.Family,
			Genus:           r.Genus,
			Species:         r.Species,
			CanonicalName:   r.CanonicalName,
			ScientificName:  r.ScientificName,
			AuthorShip:      r.Authorship,
			NameType:        r.NameType,
			Rank:            r.Rank,
			TaxonomicStatus: r.TaxonomicStatus,
			Extinct:         r.Extinct,
			NumDescendants:  r.NumDescendants,
		}
	}

	return &SpeciesSearchResult{
		Query:   query,
		Total:   raw.Count,
		Offset:  raw.Offset,
		Limit:   raw.Limit,
		HasMore: raw.Offset+raw.Limit < raw.Count,
		Results: results,
	}, nil
}

// GetSpecies returns a species by its GBIF taxon key.
func (c *Client) GetSpecies(ctx context.Context, key int) (*Species, error) {
	if key <= 0 {
		return nil, fmt.Errorf("key must be a positive integer")
	}

	var raw struct {
		Key            int    `json:"key"`
		NubKey         int    `json:"nubKey"`
		TaxonID        string `json:"taxonID"`
		Kingdom        string `json:"kingdom"`
		Phylum         string `json:"phylum"`
		Class          string `json:"class"`
		Order          string `json:"order"`
		Family         string `json:"family"`
		Genus          string `json:"genus"`
		Species        string `json:"species"`
		CanonicalName  string `json:"canonicalName"`
		ScientificName string `json:"scientificName"`
		Authorship     string `json:"authorship"`
		Rank           string `json:"rank"`
		TaxonomicStatus string `json:"taxonomicStatus"`
		Extinct        bool   `json:"extinct"`
		NumDescendants int    `json:"numDescendants"`
	}

	if err := c.get(ctx, fmt.Sprintf("/species/%d", key), nil, &raw); err != nil {
		return nil, err
	}

	return &Species{
		Key:             raw.Key,
		NubKey:          raw.NubKey,
		TaxonID:         raw.TaxonID,
		Kingdom:         raw.Kingdom,
		Phylum:          raw.Phylum,
		Class:           raw.Class,
		Order:           raw.Order,
		Family:          raw.Family,
		Genus:           raw.Genus,
		Species:         raw.Species,
		CanonicalName:   raw.CanonicalName,
		ScientificName:  raw.ScientificName,
		AuthorShip:      raw.Authorship,
		Rank:            raw.Rank,
		TaxonomicStatus: raw.TaxonomicStatus,
		Extinct:         raw.Extinct,
		NumDescendants:  raw.NumDescendants,
	}, nil
}

// GetVernacularNames returns common (vernacular) names for a species key.
func (c *Client) GetVernacularNames(ctx context.Context, key int, lang string) ([]string, error) {
	if key <= 0 {
		return nil, fmt.Errorf("key must be a positive integer")
	}

	var raw struct {
		Results []struct {
			VernacularName string `json:"vernacularName"`
			Language       string `json:"language"`
		} `json:"results"`
	}

	if err := c.get(ctx, fmt.Sprintf("/species/%d/vernacularNames", key), url.Values{"limit": []string{"50"}}, &raw); err != nil {
		return nil, err
	}

	lang = strings.ToLower(strings.TrimSpace(lang))
	var names []string
	seen := make(map[string]bool)
	for _, r := range raw.Results {
		name := strings.TrimSpace(r.VernacularName)
		if name == "" || seen[name] {
			continue
		}
		if lang == "" || lang == "en" || lang == "eng" {
			if r.Language == "" || strings.ToLower(r.Language) == "eng" || strings.ToLower(r.Language) == "en" {
				seen[name] = true
				names = append(names, name)
			}
		} else {
			if strings.ToLower(r.Language) == lang {
				seen[name] = true
				names = append(names, name)
			}
		}
	}
	return names, nil
}

// SearchOccurrences searches occurrence records (observations in the wild).
// speciesKey: GBIF taxon key (use SearchSpecies to find it)
// countryCode: ISO 2-letter country code (optional)
func (c *Client) SearchOccurrences(ctx context.Context, speciesKey int, countryCode string, year int, limit, offset int) (*OccurrenceResult, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	params := url.Values{
		"limit":  []string{fmt.Sprintf("%d", limit)},
		"offset": []string{fmt.Sprintf("%d", offset)},
		"hasCoordinate": []string{"true"},
	}

	query := make(map[string]string)
	if speciesKey > 0 {
		params.Set("speciesKey", fmt.Sprintf("%d", speciesKey))
		query["species_key"] = fmt.Sprintf("%d", speciesKey)
	}
	if countryCode != "" {
		cc := strings.ToUpper(strings.TrimSpace(countryCode))
		params.Set("country", cc)
		query["country"] = cc
	}
	if year > 0 {
		params.Set("year", fmt.Sprintf("%d", year))
		query["year"] = fmt.Sprintf("%d", year)
	}

	var raw struct {
		Count   int `json:"count"`
		Offset  int `json:"offset"`
		Limit   int `json:"limit"`
		Results []struct {
			Key            int      `json:"key"`
			DatasetKey     string   `json:"datasetKey"`
			ScientificName string   `json:"scientificName"`
			Kingdom        string   `json:"kingdom"`
			Family         string   `json:"family"`
			Genus          string   `json:"genus"`
			Species        string   `json:"species"`
			CountryCode    string   `json:"countryCode"`
			Country        string   `json:"country"`
			StateProvince  string   `json:"stateProvince"`
			Locality       string   `json:"locality"`
			DecimalLatitude  *float64 `json:"decimalLatitude"`
			DecimalLongitude *float64 `json:"decimalLongitude"`
			Year           int      `json:"year"`
			Month          int      `json:"month"`
			Day            int      `json:"day"`
			EventDate      string   `json:"eventDate"`
			BasisOfRecord  string   `json:"basisOfRecord"`
			Media          []struct {
				Type       string `json:"type"`
				Identifier string `json:"identifier"`
			} `json:"media"`
		} `json:"results"`
	}

	if err := c.get(ctx, "/occurrence/search", params, &raw); err != nil {
		return nil, err
	}

	results := make([]Occurrence, len(raw.Results))
	for i, r := range raw.Results {
		occ := Occurrence{
			Key:            r.Key,
			DatasetKey:     r.DatasetKey,
			ScientificName: r.ScientificName,
			Kingdom:        r.Kingdom,
			Family:         r.Family,
			Genus:          r.Genus,
			Species:        r.Species,
			CountryCode:    r.CountryCode,
			Country:        r.Country,
			StateProvince:  r.StateProvince,
			Locality:       r.Locality,
			Latitude:       r.DecimalLatitude,
			Longitude:      r.DecimalLongitude,
			Year:           r.Year,
			Month:          r.Month,
			Day:            r.Day,
			EventDate:      r.EventDate,
			BasisOfRecord:  r.BasisOfRecord,
		}
		for _, m := range r.Media {
			occ.MediaType = append(occ.MediaType, m.Type)
			if m.Identifier != "" {
				occ.MediaURLs = append(occ.MediaURLs, m.Identifier)
			}
		}
		results[i] = occ
	}

	return &OccurrenceResult{
		Query:   query,
		Total:   raw.Count,
		Offset:  raw.Offset,
		Limit:   raw.Limit,
		HasMore: raw.Offset+raw.Limit < raw.Count,
		Results: results,
	}, nil
}
