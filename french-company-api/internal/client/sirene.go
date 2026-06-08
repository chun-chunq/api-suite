// Package client wraps the French government's Recherche Entreprises API.
// Source: https://recherche-entreprises.api.gouv.fr (official, open data, no auth)
// Data: SIRENE registry — all French companies and establishments (legal + natural persons)
// License: Licence Ouverte / Open Licence v2.0 (freely reusable)
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

const defaultBaseURL = "https://recherche-entreprises.api.gouv.fr"

// Company represents a French registered company (from SIRENE registry).
type Company struct {
	SIREN            string      `json:"siren"`            // 9-digit national company identifier
	Name             string      `json:"name"`             // full legal name
	Acronym          string      `json:"acronym,omitempty"`
	LegalForm        string      `json:"legalForm,omitempty"`       // e.g. "Société Anonyme à Conseil d'Administration"
	LegalFormCode    string      `json:"legalFormCode,omitempty"`   // INSEE code e.g. "5710"
	Status           string      `json:"status"`                    // "A" = active, "C" = ceased
	Category         string      `json:"category,omitempty"`        // "GE", "ETI", "PME", "MI"
	ActivityCode     string      `json:"activityCode,omitempty"`    // NAF/APE code e.g. "62.01Z"
	ActivityLabel    string      `json:"activityLabel,omitempty"`
	EmployeeRange    string      `json:"employeeRange,omitempty"`
	CreatedAt        string      `json:"createdAt,omitempty"`
	UpdatedAt        string      `json:"updatedAt,omitempty"`
	EstablishmentCount       int `json:"establishmentCount,omitempty"`
	ActiveEstablishments     int `json:"activeEstablishments,omitempty"`
	HQ               *Establishment `json:"headquarters,omitempty"`
}

// Establishment is the registered office (siège social) address.
type Establishment struct {
	SIRET       string  `json:"siret,omitempty"`        // 14-digit establishment identifier
	Address     string  `json:"address,omitempty"`
	PostalCode  string  `json:"postalCode,omitempty"`
	City        string  `json:"city,omitempty"`
	Department  string  `json:"department,omitempty"`
	Region      string  `json:"region,omitempty"`
	Latitude    float64 `json:"latitude,omitempty"`
	Longitude   float64 `json:"longitude,omitempty"`
	ActivityCode string `json:"activityCode,omitempty"`
	ActivityLabel string `json:"activityLabel,omitempty"`
}

// SearchQuery holds search parameters for the SIRENE registry.
type SearchQuery struct {
	Query        string // free-text: company name, SIREN, trade name
	PostalCode   string // 5-digit French postal code
	Department   string // 2-digit department code e.g. "75" (Paris)
	ActivityCode string // NAF/APE code filter e.g. "62.01Z"
	LegalForm    string // INSEE legal form code e.g. "5710"
	ActiveOnly   bool   // filter to active companies only
	MaxResults   int    // 1–25 (API max per page), default 25
	Page         int    // 1-based page number
}

// SearchResult is the paginated response.
type SearchResult struct {
	Total      int       `json:"total"`
	Page       int       `json:"page"`
	PerPage    int       `json:"perPage"`
	TotalPages int       `json:"totalPages"`
	Results    []Company `json:"results"`
}

// Client wraps the Recherche Entreprises REST API.
type Client struct {
	http    *http.Client
	baseURL string
}

// New creates a new SIRENE client.
func New() *Client {
	return &Client{
		http:    &http.Client{Timeout: 15 * time.Second},
		baseURL: defaultBaseURL,
	}
}

// Search searches the SIRENE registry.
func (c *Client) Search(ctx context.Context, q SearchQuery) (*SearchResult, error) {
	if q.Query == "" && q.PostalCode == "" && q.Department == "" && q.ActivityCode == "" {
		return nil, fmt.Errorf("at least one search parameter is required (q, postalCode, department, or activityCode)")
	}
	if q.MaxResults <= 0 || q.MaxResults > 25 {
		q.MaxResults = 25
	}
	if q.Page <= 0 {
		q.Page = 1
	}

	params := url.Values{}
	if q.Query != "" {
		params.Set("q", q.Query)
	}
	if q.PostalCode != "" {
		params.Set("code_postal", q.PostalCode)
	}
	if q.Department != "" {
		params.Set("departement", q.Department)
	}
	if q.ActivityCode != "" {
		params.Set("activite_principale", q.ActivityCode)
	}
	if q.LegalForm != "" {
		params.Set("nature_juridique", q.LegalForm)
	}
	if q.ActiveOnly {
		params.Set("etat_administratif", "A")
	}
	params.Set("per_page", fmt.Sprintf("%d", q.MaxResults))
	params.Set("page", fmt.Sprintf("%d", q.Page))

	endpoint := fmt.Sprintf("%s/search?%s", c.baseURL, params.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("SIRENE request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("SIRENE API HTTP %d: %s", resp.StatusCode, string(body))
	}

	return c.parseResponse(body, q)
}

// GetBySIREN fetches a company by its 9-digit SIREN number.
func (c *Client) GetBySIREN(ctx context.Context, siren string) (*Company, error) {
	siren = strings.TrimSpace(strings.ReplaceAll(siren, " ", ""))
	if len(siren) != 9 {
		return nil, fmt.Errorf("invalid SIREN: must be exactly 9 digits, got %d chars", len(siren))
	}

	// Use the search endpoint with SIREN as the query — most reliable
	result, err := c.Search(ctx, SearchQuery{Query: siren, MaxResults: 1})
	if err != nil {
		return nil, err
	}
	if len(result.Results) == 0 {
		return nil, nil
	}
	// Verify SIREN matches exactly
	for _, co := range result.Results {
		if co.SIREN == siren {
			return &co, nil
		}
	}
	return nil, nil
}

// ── Raw API response structures ────────────────────────────────────────────────

type sireneResponse struct {
	TotalResults int             `json:"total_results"`
	TotalPages   int             `json:"total_pages"`
	PerPage      int             `json:"per_page"`
	Page         int             `json:"page"`
	Results      []sireneCompany `json:"results"`
}

type sireneCompany struct {
	Siren                        string `json:"siren"`
	NomComplet                   string `json:"nom_complet"`
	NomRaisonSociale             string `json:"nom_raison_sociale"`
	Sigle                        string `json:"sigle"`
	NombreEtablissements         int    `json:"nombre_etablissements"`
	NombreEtablissementsOuverts  int    `json:"nombre_etablissements_ouverts"`
	ActivitePrincipale           string `json:"activite_principale"`
	LibelleActivitePrincipale    string `json:"libelle_activite_principale"`
	CategorieEntreprise          string `json:"categorie_entreprise"`
	EtatAdministratif            string `json:"etat_administratif"`
	NatureJuridique              string `json:"nature_juridique"`
	LibelleNatureJuridique       string `json:"libelle_nature_juridique"`
	TrancheEffectifSalarie       string `json:"tranche_effectif_salarie"`
	AnneeTrancheEffectif         string `json:"annee_tranche_effectif_salarie"`
	DateCreation                 string `json:"date_creation"`
	DateMiseAJour                string `json:"date_mise_a_jour"`
	Siege                        *siegeRaw `json:"siege"`
}

type siegeRaw struct {
	Siret                     string  `json:"siret"`
	SiretFormate              string  `json:"siret_formate"`
	CodePostal                string  `json:"code_postal"`
	Commune                   string  `json:"commune"`
	LibelleCommune            string  `json:"libelle_commune"`
	Departement               string  `json:"departement"`
	LibelleDepartement        string  `json:"libelle_departement"`
	Region                    string  `json:"region"`
	LibelleRegion             string  `json:"libelle_region"`
	Adresse                   string  `json:"adresse"`
	Latitude                  float64 `json:"latitude"`
	Longitude                 float64 `json:"longitude"`
	ActivitePrincipale        string  `json:"activite_principale"`
	LibelleActivitePrincipale string  `json:"libelle_activite_principale"`
}

func (c *Client) parseResponse(body []byte, q SearchQuery) (*SearchResult, error) {
	var raw sireneResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse SIRENE response: %w", err)
	}

	companies := make([]Company, 0, len(raw.Results))
	for _, r := range raw.Results {
		companies = append(companies, mapCompany(r))
	}

	return &SearchResult{
		Total:      raw.TotalResults,
		Page:       raw.Page,
		PerPage:    raw.PerPage,
		TotalPages: raw.TotalPages,
		Results:    companies,
	}, nil
}

func mapCompany(r sireneCompany) Company {
	status := "active"
	if r.EtatAdministratif == "C" {
		status = "ceased"
	}

	name := r.NomComplet
	if name == "" {
		name = r.NomRaisonSociale
	}

	co := Company{
		SIREN:                r.Siren,
		Name:                 name,
		Acronym:              r.Sigle,
		LegalForm:            r.LibelleNatureJuridique,
		LegalFormCode:        r.NatureJuridique,
		Status:               status,
		Category:             r.CategorieEntreprise,
		ActivityCode:         r.ActivitePrincipale,
		ActivityLabel:        r.LibelleActivitePrincipale,
		EmployeeRange:        r.TrancheEffectifSalarie,
		CreatedAt:            r.DateCreation,
		UpdatedAt:            r.DateMiseAJour,
		EstablishmentCount:   r.NombreEtablissements,
		ActiveEstablishments: r.NombreEtablissementsOuverts,
	}

	if r.Siege != nil {
		co.HQ = &Establishment{
			SIRET:         r.Siege.Siret,
			Address:       r.Siege.Adresse,
			PostalCode:    r.Siege.CodePostal,
			City:          r.Siege.LibelleCommune,
			Department:    r.Siege.LibelleDepartement,
			Region:        r.Siege.LibelleRegion,
			Latitude:      r.Siege.Latitude,
			Longitude:     r.Siege.Longitude,
			ActivityCode:  r.Siege.ActivitePrincipale,
			ActivityLabel: r.Siege.LibelleActivitePrincipale,
		}
	}

	return co
}
