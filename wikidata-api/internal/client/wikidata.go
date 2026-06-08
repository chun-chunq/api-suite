// Package client wraps the Wikidata and MediaWiki APIs.
// Free, no auth required. Rate limit: be polite (1 req/sec max without key).
//
// Uses:
//   - Wikidata search:  https://www.wikidata.org/w/api.php?action=wbsearchentities
//   - Wikidata entity:  https://www.wikidata.org/w/api.php?action=wbgetentities
//   - SPARQL endpoint:  https://query.wikidata.org/sparql  (for complex queries)
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

const (
	wikidataBase  = "https://www.wikidata.org/w/api.php"
	sparqlBase    = "https://query.wikidata.org/sparql"
)

// ── Types ─────────────────────────────────────────────────────────────────────

// SearchResult is a brief entity match from search.
type SearchResult struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Aliases     []string `json:"aliases,omitempty"`
	URL         string `json:"url"`
	Type        string `json:"entityType"` // item or property
}

// Entity holds the full simplified entity data.
type Entity struct {
	ID          string            `json:"id"`
	Label       string            `json:"label"`
	Description string            `json:"description,omitempty"`
	Aliases     []string          `json:"aliases,omitempty"`
	URL         string            `json:"url"`
	Claims      map[string][]Claim `json:"claims,omitempty"`
	// Most useful resolved claims:
	InstanceOf   []string `json:"instanceOf,omitempty"`
	Subclass     []string `json:"subclassOf,omitempty"`
	Country      string   `json:"country,omitempty"`
	OfficialSite string   `json:"officialWebsite,omitempty"`
	Image        string   `json:"image,omitempty"`
	Coordinates  *LatLon  `json:"coordinates,omitempty"`
}

// Claim is a simplified statement value.
type Claim struct {
	Property    string      `json:"property"`
	Value       interface{} `json:"value"`
	ValueType   string      `json:"valueType"`
}

// LatLon is a geographic coordinate.
type LatLon struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// ── Client ────────────────────────────────────────────────────────────────────

type Client struct {
	http        *http.Client
	wikidataURL string
	sparqlURL   string
	userAgent   string
}

func New(contactEmail string) *Client {
	ua := "api-suite/1.0"
	if contactEmail != "" {
		ua = fmt.Sprintf("api-suite/1.0 (%s)", contactEmail)
	}
	return &Client{
		http:        &http.Client{Timeout: 20 * time.Second},
		wikidataURL: wikidataBase,
		sparqlURL:   sparqlBase,
		userAgent:   ua,
	}
}

// ── Search ────────────────────────────────────────────────────────────────────

// Search searches Wikidata for entities matching a label string.
func (c *Client) Search(ctx context.Context, query, language, entityType string, limit int) ([]SearchResult, int, error) {
	if query == "" {
		return nil, 0, fmt.Errorf("query is required")
	}
	if language == "" {
		language = "en"
	}
	if entityType == "" {
		entityType = "item"
	}
	if limit <= 0 || limit > 50 {
		limit = 10
	}

	params := url.Values{}
	params.Set("action", "wbsearchentities")
	params.Set("search", query)
	params.Set("language", language)
	params.Set("type", entityType)
	params.Set("limit", strconv.Itoa(limit))
	params.Set("format", "json")
	params.Set("uselang", language)

	var raw struct {
		Search      []struct {
			ID          string   `json:"id"`
			Label       string   `json:"label"`
			Description string   `json:"description"`
			Aliases     []struct {
				Value string `json:"value"`
			} `json:"aliases"`
			URL        string `json:"url"`
			EntityType string `json:"entityType"`
		} `json:"search"`
		SearchInfo struct {
			Search string `json:"search"`
		} `json:"searchinfo"`
		SearchContinue int `json:"search-continue"`
	}

	if err := c.get(ctx, c.wikidataURL, params, &raw); err != nil {
		return nil, 0, err
	}

	results := make([]SearchResult, 0, len(raw.Search))
	for _, r := range raw.Search {
		aliases := make([]string, 0, len(r.Aliases))
		for _, a := range r.Aliases {
			aliases = append(aliases, a.Value)
		}
		results = append(results, SearchResult{
			ID:          r.ID,
			Label:       r.Label,
			Description: r.Description,
			Aliases:     aliases,
			URL:         "https://www.wikidata.org/wiki/" + r.ID,
			Type:        r.EntityType,
		})
	}
	return results, raw.SearchContinue, nil
}

// ── Get Entity ────────────────────────────────────────────────────────────────

// GetEntity fetches detailed data for a Wikidata entity by ID (e.g. "Q42").
func (c *Client) GetEntity(ctx context.Context, entityID, language string) (*Entity, error) {
	if entityID == "" {
		return nil, fmt.Errorf("entityID is required")
	}
	// normalize: "q42" → "Q42"
	entityID = strings.ToUpper(entityID)
	if language == "" {
		language = "en"
	}

	params := url.Values{}
	params.Set("action", "wbgetentities")
	params.Set("ids", entityID)
	params.Set("languages", language)
	params.Set("format", "json")
	params.Set("props", "labels|descriptions|aliases|claims|sitelinks")

	var raw struct {
		Entities map[string]map[string]interface{} `json:"entities"`
	}
	if err := c.get(ctx, c.wikidataURL, params, &raw); err != nil {
		return nil, err
	}

	ent, ok := raw.Entities[entityID]
	if !ok {
		return nil, fmt.Errorf("entity %s not found", entityID)
	}

	// check if entity is missing
	if missing, _ := ent["missing"].(string); missing == "" {
		if _, isMissing := ent["missing"]; isMissing {
			return nil, fmt.Errorf("entity %s not found", entityID)
		}
	}

	return mapEntity(ent, entityID, language), nil
}

// ── Mappers ───────────────────────────────────────────────────────────────────

func mapEntity(raw map[string]interface{}, id, lang string) *Entity {
	e := &Entity{
		ID:  id,
		URL: "https://www.wikidata.org/wiki/" + id,
	}

	// labels
	if labels, ok := raw["labels"].(map[string]interface{}); ok {
		if langLabel, ok := labels[lang].(map[string]interface{}); ok {
			e.Label, _ = langLabel["value"].(string)
		}
	}

	// descriptions
	if descs, ok := raw["descriptions"].(map[string]interface{}); ok {
		if langDesc, ok := descs[lang].(map[string]interface{}); ok {
			e.Description, _ = langDesc["value"].(string)
		}
	}

	// aliases
	if aliases, ok := raw["aliases"].(map[string]interface{}); ok {
		if langAliases, ok := aliases[lang].([]interface{}); ok {
			for _, a := range langAliases {
				if am, ok := a.(map[string]interface{}); ok {
					if v, ok := am["value"].(string); ok {
						e.Aliases = append(e.Aliases, v)
					}
				}
			}
		}
	}

	// claims — simplified extraction of key properties
	if claims, ok := raw["claims"].(map[string]interface{}); ok {
		e.Claims = map[string][]Claim{}

		// P31 = instance of
		e.InstanceOf = extractStringValues(claims, "P31")
		// P279 = subclass of
		e.Subclass = extractStringValues(claims, "P279")
		// P17 = country
		if vals := extractStringValues(claims, "P17"); len(vals) > 0 {
			e.Country = vals[0]
		}
		// P856 = official website
		if vals := extractURLValues(claims, "P856"); len(vals) > 0 {
			e.OfficialSite = vals[0]
		}
		// P18 = image
		if vals := extractStringValues(claims, "P18"); len(vals) > 0 {
			e.Image = "https://commons.wikimedia.org/wiki/Special:FilePath/" + url.QueryEscape(vals[0])
		}
		// P625 = coordinates
		if coord := extractCoordinates(claims, "P625"); coord != nil {
			e.Coordinates = coord
		}
	}

	return e
}

// extractStringValues extracts entity IDs or string values from a claim property.
func extractStringValues(claims map[string]interface{}, prop string) []string {
	stmts, ok := claims[prop].([]interface{})
	if !ok {
		return nil
	}
	result := []string{}
	for _, s := range stmts {
		sm, ok := s.(map[string]interface{})
		if !ok {
			continue
		}
		mainsnak, ok := sm["mainsnak"].(map[string]interface{})
		if !ok {
			continue
		}
		dv, ok := mainsnak["datavalue"].(map[string]interface{})
		if !ok {
			continue
		}
		switch dv["type"] {
		case "wikibase-entityid":
			if val, ok := dv["value"].(map[string]interface{}); ok {
				if id, ok := val["id"].(string); ok {
					result = append(result, id)
				}
			}
		case "string":
			if val, ok := dv["value"].(string); ok {
				result = append(result, val)
			}
		}
		if len(result) >= 5 {
			break
		}
	}
	return result
}

func extractURLValues(claims map[string]interface{}, prop string) []string {
	return extractStringValues(claims, prop) // URLs are stored as strings
}

func extractCoordinates(claims map[string]interface{}, prop string) *LatLon {
	stmts, ok := claims[prop].([]interface{})
	if !ok || len(stmts) == 0 {
		return nil
	}
	sm, ok := stmts[0].(map[string]interface{})
	if !ok {
		return nil
	}
	mainsnak, ok := sm["mainsnak"].(map[string]interface{})
	if !ok {
		return nil
	}
	dv, ok := mainsnak["datavalue"].(map[string]interface{})
	if !ok {
		return nil
	}
	if dv["type"] != "globecoordinate" {
		return nil
	}
	val, ok := dv["value"].(map[string]interface{})
	if !ok {
		return nil
	}
	lat, _ := val["latitude"].(float64)
	lon, _ := val["longitude"].(float64)
	if lat == 0 && lon == 0 {
		return nil
	}
	return &LatLon{Lat: lat, Lon: lon}
}

// ── HTTP helper ───────────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, baseURL string, params url.Values, dest interface{}) error {
	u := baseURL + "?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	return nil
}
