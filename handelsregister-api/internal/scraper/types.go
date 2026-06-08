package scraper

import "time"

// CompanyStatus represents the registration state of a company.
type CompanyStatus string

const (
	StatusActive  CompanyStatus = "aktiv"
	StatusDeleted CompanyStatus = "gelöscht"
	StatusUnknown CompanyStatus = "unbekannt"
)

// Person represents a managing director (Geschäftsführer) or board member (Vorstand).
type Person struct {
	Name string `json:"name"`
	// Role e.g. "Geschäftsführer", "Vorstand", "Prokurist".
	Role string `json:"role,omitempty"`
	City string `json:"city,omitempty"`
}

// Address is the registered seat (Sitz) of the company.
type Address struct {
	Street     string `json:"street,omitempty"`
	PostalCode string `json:"postal_code,omitempty"`
	City       string `json:"city,omitempty"`
	Country    string `json:"country,omitempty"`
}

// CompanyData is the normalized result returned to API consumers.
type CompanyData struct {
	Firmenname    string        `json:"firmenname"`
	Rechtsform    string        `json:"rechtsform,omitempty"`
	HRBNummer     string        `json:"hrb_nummer"`
	Amtsgericht   string        `json:"amtsgericht,omitempty"`
	Register      string        `json:"register,omitempty"` // e.g. "HRB", "HRA"
	State         string        `json:"state,omitempty"`    // Bundesland
	Management    []Person      `json:"management,omitempty"`
	Sitz          Address       `json:"sitz"`
	Gruendung     string        `json:"gruendungsdatum,omitempty"`
	Status        CompanyStatus `json:"status"`
	ScrapedAt     time.Time     `json:"scraped_at"`
	SourceURL     string        `json:"source_url,omitempty"`
}

// SearchResult is a lightweight hit returned from a name search.
type SearchResult struct {
	Firmenname  string `json:"firmenname"`
	HRBNummer   string `json:"hrb_nummer"`
	Amtsgericht string `json:"amtsgericht,omitempty"`
	Sitz        string `json:"sitz,omitempty"`
	State       string `json:"state,omitempty"`
}
