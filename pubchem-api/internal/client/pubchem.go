// Package client wraps the PubChem PUG REST API.
// Docs: https://pubchemdocs.ncbi.nlm.nih.gov/pug-rest
// No API key required. Free NIH/NCBI service.
// Rate limit: 5 requests/second, 400 requests/minute (we stay well under).
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBase    = "https://pubchem.ncbi.nlm.nih.gov/rest/pug"
	defaultTimeout = 20 * time.Second
)

// Compound holds essential chemical compound information.
type Compound struct {
	CID         int64    `json:"cid"`           // PubChem Compound ID
	Name        string   `json:"name"`          // preferred IUPAC name
	MolFormula  string   `json:"molecular_formula,omitempty"`
	MolWeight   float64  `json:"molecular_weight,omitempty"` // g/mol
	SMILES      string   `json:"smiles,omitempty"`           // canonical SMILES
	InChI       string   `json:"inchi,omitempty"`
	InChIKey    string   `json:"inchikey,omitempty"`
	XLogP       float64  `json:"xlogp,omitempty"`            // lipophilicity
	HBondDonors int      `json:"hbond_donors,omitempty"`
	HBondAccept int      `json:"hbond_acceptors,omitempty"`
	RotatableBonds int   `json:"rotatable_bonds,omitempty"`
	TPSA        float64  `json:"tpsa,omitempty"`            // topological polar surface area (Å²)
	Charge      int      `json:"charge,omitempty"`
	Synonyms    []string `json:"synonyms,omitempty"`        // common names, trade names
	Description string   `json:"description,omitempty"`
}

// SearchResult wraps a list of CIDs from a name search.
type SearchResult struct {
	Query     string  `json:"query"`
	CIDs      []int64 `json:"cids"`
	Total     int     `json:"total"`
}

// Client is the PubChem REST client.
type Client struct {
	http    *http.Client
	baseURL string
}

// New returns a new PubChem client.
func New() *Client {
	return &Client{
		http:    &http.Client{Timeout: defaultTimeout},
		baseURL: defaultBase,
	}
}

// ── Internal helpers ─────────────────────────────────────────────────────────

func (c *Client) get(ctx context.Context, path string, out interface{}) error {
	u := c.baseURL + path
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
	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("rate_limit_exceeded")
	}
	if resp.StatusCode != http.StatusOK {
		// Try to get PubChem error message
		var fault struct {
			Fault struct {
				Message string `json:"Message"`
				Details []string `json:"Details"`
			} `json:"Fault"`
		}
		if jsonErr := json.NewDecoder(resp.Body).Decode(&fault); jsonErr == nil && fault.Fault.Message != "" {
			return fmt.Errorf("pubchem error: %s", fault.Fault.Message)
		}
		return fmt.Errorf("upstream returned HTTP %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	return nil
}

// ── Public API ───────────────────────────────────────────────────────────────

// SearchByName finds compounds matching a name/keyword.
// Returns up to limit CIDs (max 20).
func (c *Client) SearchByName(ctx context.Context, name string, limit int) (*SearchResult, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	if limit <= 0 || limit > 20 {
		limit = 10
	}

	// PUG REST: /compound/name/{name}/cids/JSON
	path := fmt.Sprintf("/compound/name/%s/cids/JSON", url.PathEscape(name))

	var raw struct {
		IdentifierList struct {
			CID []int64 `json:"CID"`
		} `json:"IdentifierList"`
	}
	if err := c.get(ctx, path, &raw); err != nil {
		return nil, err
	}

	cids := raw.IdentifierList.CID
	if len(cids) > limit {
		cids = cids[:limit]
	}

	return &SearchResult{
		Query: name,
		CIDs:  cids,
		Total: len(raw.IdentifierList.CID),
	}, nil
}

// GetByCID fetches full compound details for a given PubChem CID.
func (c *Client) GetByCID(ctx context.Context, cid int64) (*Compound, error) {
	if cid <= 0 {
		return nil, fmt.Errorf("invalid CID: must be positive integer")
	}

	// Fetch properties in one call
	props := "IUPACName,MolecularFormula,MolecularWeight,CanonicalSMILES,InChI,InChIKey,XLogP,HBondDonorCount,HBondAcceptorCount,RotatableBondCount,TPSA,Charge"
	path := fmt.Sprintf("/compound/cid/%d/property/%s/JSON", cid, props)

	var propResp struct {
		PropertyTable struct {
			Properties []map[string]interface{} `json:"Properties"`
		} `json:"PropertyTable"`
	}
	if err := c.get(ctx, path, &propResp); err != nil {
		return nil, err
	}

	if len(propResp.PropertyTable.Properties) == 0 {
		return nil, fmt.Errorf("not_found")
	}

	p := propResp.PropertyTable.Properties[0]
	compound := &Compound{CID: cid}
	compound.Name = strVal(p, "IUPACName")
	compound.MolFormula = strVal(p, "MolecularFormula")
	compound.MolWeight = floatVal(p, "MolecularWeight")
	compound.SMILES = strVal(p, "CanonicalSMILES")
	compound.InChI = strVal(p, "InChI")
	compound.InChIKey = strVal(p, "InChIKey")
	compound.XLogP = floatVal(p, "XLogP")
	compound.HBondDonors = intVal(p, "HBondDonorCount")
	compound.HBondAccept = intVal(p, "HBondAcceptorCount")
	compound.RotatableBonds = intVal(p, "RotatableBondCount")
	compound.TPSA = floatVal(p, "TPSA")
	compound.Charge = intVal(p, "Charge")

	return compound, nil
}

// GetByName fetches compound details for the best match of a name search.
func (c *Client) GetByName(ctx context.Context, name string) (*Compound, error) {
	sr, err := c.SearchByName(ctx, name, 1)
	if err != nil {
		return nil, err
	}
	if len(sr.CIDs) == 0 {
		return nil, fmt.Errorf("not_found")
	}
	compound, err := c.GetByCID(ctx, sr.CIDs[0])
	if err != nil {
		return nil, err
	}
	// Use the search name as fallback display name
	if compound.Name == "" {
		compound.Name = name
	}
	return compound, nil
}

// GetSynonyms returns synonym names for a compound (trade names, common names, etc.).
// Returns up to limit synonyms (max 50).
func (c *Client) GetSynonyms(ctx context.Context, cid int64, limit int) ([]string, error) {
	if cid <= 0 {
		return nil, fmt.Errorf("invalid CID")
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	path := fmt.Sprintf("/compound/cid/%d/synonyms/JSON", cid)

	var raw struct {
		InformationList struct {
			Information []struct {
				Synonym []string `json:"Synonym"`
			} `json:"Information"`
		} `json:"InformationList"`
	}
	if err := c.get(ctx, path, &raw); err != nil {
		return nil, err
	}

	var synonyms []string
	if len(raw.InformationList.Information) > 0 {
		synonyms = raw.InformationList.Information[0].Synonym
	}
	if len(synonyms) > limit {
		synonyms = synonyms[:limit]
	}
	return synonyms, nil
}

// GetDescription returns the textual description of a compound.
func (c *Client) GetDescription(ctx context.Context, cid int64) (string, error) {
	if cid <= 0 {
		return "", fmt.Errorf("invalid CID")
	}

	path := fmt.Sprintf("/compound/cid/%d/description/JSON", cid)

	var raw struct {
		InformationList struct {
			Information []struct {
				Title       string `json:"Title"`
				Description string `json:"Description"`
			} `json:"Information"`
		} `json:"InformationList"`
	}
	if err := c.get(ctx, path, &raw); err != nil {
		if err.Error() == "not_found" {
			return "", nil // no description available
		}
		return "", err
	}

	for _, info := range raw.InformationList.Information {
		if info.Description != "" {
			return strings.TrimSpace(info.Description), nil
		}
	}
	return "", nil
}

// GetProperties returns selected molecular properties for a CID.
// Convenience wrapper around GetByCID for lighter responses.
func (c *Client) GetProperties(ctx context.Context, cid int64) (*Compound, error) {
	return c.GetByCID(ctx, cid)
}

// ── Value extraction helpers ─────────────────────────────────────────────────

func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func floatVal(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case int:
			return float64(n)
		case string:
			f, _ := strconv.ParseFloat(n, 64)
			return f
		}
	}
	return 0
}

func intVal(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case int:
			return n
		}
	}
	return 0
}
