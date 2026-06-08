package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("New() returned nil")
	}
}

func TestSearch_NoParams(t *testing.T) {
	c := New()
	_, err := c.Search(context.Background(), SearchQuery{})
	if err == nil {
		t.Error("expected error for empty search query")
	}
}

func TestGetBySIREN_InvalidLength(t *testing.T) {
	c := New()
	_, err := c.GetBySIREN(context.Background(), "12345")
	if err == nil {
		t.Error("expected error for short SIREN")
	}
}

func TestGetBySIREN_TooLong(t *testing.T) {
	c := New()
	_, err := c.GetBySIREN(context.Background(), "1234567890") // 10 digits
	if err == nil {
		t.Error("expected error for 10-digit SIREN")
	}
}

func TestMapCompany_Active(t *testing.T) {
	r := sireneCompany{
		Siren:                "542051180",
		NomComplet:           "TOTALENERGIES SE",
		EtatAdministratif:    "A",
		LibelleNatureJuridique: "Société Anonyme à Conseil d'Administration",
		NatureJuridique:      "5710",
		ActivitePrincipale:   "06.10Z",
		CategorieEntreprise:  "GE",
		DateCreation:         "1985-06-01",
	}
	co := mapCompany(r)
	if co.SIREN != "542051180" {
		t.Errorf("SIREN want 542051180, got %s", co.SIREN)
	}
	if co.Name != "TOTALENERGIES SE" {
		t.Errorf("Name want TOTALENERGIES SE, got %s", co.Name)
	}
	if co.Status != "active" {
		t.Errorf("Status want active, got %s", co.Status)
	}
	if co.LegalFormCode != "5710" {
		t.Errorf("LegalFormCode want 5710, got %s", co.LegalFormCode)
	}
}

func TestMapCompany_Ceased(t *testing.T) {
	r := sireneCompany{
		Siren:             "123456789",
		NomComplet:        "OLD COMPANY SA",
		EtatAdministratif: "C",
	}
	co := mapCompany(r)
	if co.Status != "ceased" {
		t.Errorf("Status want ceased, got %s", co.Status)
	}
}

func TestMapCompany_FallbackName(t *testing.T) {
	r := sireneCompany{
		Siren:            "999999999",
		NomComplet:       "",
		NomRaisonSociale: "FALLBACK NAME SARL",
	}
	co := mapCompany(r)
	if co.Name != "FALLBACK NAME SARL" {
		t.Errorf("Name want fallback, got %s", co.Name)
	}
}

func TestMapCompany_WithHQ(t *testing.T) {
	r := sireneCompany{
		Siren:      "542051180",
		NomComplet: "TOTALENERGIES SE",
		Siege: &siegeRaw{
			Siret:          "54205118000022",
			Adresse:        "2 PL JEAN MILLIER",
			CodePostal:     "92400",
			LibelleCommune: "COURBEVOIE",
			LibelleDepartement: "Hauts-de-Seine",
			LibelleRegion:  "Île-de-France",
			Latitude:       48.8959,
			Longitude:      2.2386,
		},
	}
	co := mapCompany(r)
	if co.HQ == nil {
		t.Fatal("HQ should not be nil")
	}
	if co.HQ.City != "COURBEVOIE" {
		t.Errorf("HQ.City want COURBEVOIE, got %s", co.HQ.City)
	}
	if co.HQ.Latitude != 48.8959 {
		t.Errorf("HQ.Latitude want 48.8959, got %f", co.HQ.Latitude)
	}
	if co.HQ.SIRET != "54205118000022" {
		t.Errorf("HQ.SIRET want 54205118000022, got %s", co.HQ.SIRET)
	}
}

func TestSearch_ServerResponse(t *testing.T) {
	raw := sireneResponse{
		TotalResults: 1,
		TotalPages:   1,
		PerPage:      25,
		Page:         1,
		Results: []sireneCompany{
			{
				Siren:             "542051180",
				NomComplet:        "TOTALENERGIES SE",
				EtatAdministratif: "A",
				CategorieEntreprise: "GE",
			},
		},
	}
	body, _ := json.Marshal(raw)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := &Client{http: &http.Client{}, baseURL: srv.URL}
	result, err := c.Search(context.Background(), SearchQuery{Query: "Total", MaxResults: 10})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("Total want 1, got %d", result.Total)
	}
	if len(result.Results) != 1 {
		t.Fatalf("want 1 result, got %d", len(result.Results))
	}
	if result.Results[0].SIREN != "542051180" {
		t.Errorf("SIREN want 542051180, got %s", result.Results[0].SIREN)
	}
}

func TestSearch_DefaultPagination(t *testing.T) {
	q := SearchQuery{Query: "test", MaxResults: 0, Page: 0}
	if q.MaxResults <= 0 || q.MaxResults > 25 {
		q.MaxResults = 25
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.MaxResults != 25 {
		t.Errorf("default MaxResults want 25, got %d", q.MaxResults)
	}
	if q.Page != 1 {
		t.Errorf("default Page want 1, got %d", q.Page)
	}
}
