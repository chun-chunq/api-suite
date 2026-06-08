package scraper

import "time"

// SearchQuery defines the trademark search parameters.
type SearchQuery struct {
	// Markenbezeichnung — trademark name or keyword
	Name string `json:"name"`
	// Aktenzeichen/Registernummer — exact registration number (e.g. "30010285")
	RegistrationNumber string `json:"registrationNumber"`
	// Inhaber/Anmelder — owner or applicant name
	Owner string `json:"owner"`
	// Klassen — Nice Classification classes (1–45), comma-separated or as slice
	Classes []int `json:"classes"`
	// Status filter: "registered", "applied", "expired", "deleted", "" = all
	Status string `json:"status"`
	// Markenform — mark type: "word", "figurative", "combined", "3d", "" = all
	MarkType string `json:"markType"`
	// DateFrom/DateTo filter filing date (YYYY-MM-DD)
	DateFrom string `json:"dateFrom"`
	DateTo   string `json:"dateTo"`
	// MaxResults — max entries to return (default 50, max 200)
	MaxResults int `json:"maxResults"`
}

// Trademark is a single result from a trademark search.
type Trademark struct {
	RegistrationNumber string    `json:"registrationNumber"`
	Name               string    `json:"name"`
	Owner              string    `json:"owner"`
	Representative     string    `json:"representative,omitempty"`
	Classes            []int     `json:"classes"`
	GoodsAndServices   string    `json:"goodsAndServices,omitempty"`
	Status             string    `json:"status"`
	MarkType           string    `json:"markType"`
	FilingDate         string    `json:"filingDate,omitempty"`
	RegistrationDate   string    `json:"registrationDate,omitempty"`
	ExpiryDate         string    `json:"expiryDate,omitempty"`
	DetailURL          string    `json:"detailUrl,omitempty"`
	Country            string    `json:"country,omitempty"` // DE, IR (international), EM (EU)
}

// SearchResult is the response from a trademark search.
type SearchResult struct {
	Results    []Trademark `json:"results"`
	TotalCount int         `json:"totalCount"`
	Page       int         `json:"page"`
	Query      SearchQuery `json:"query"`
	ScrapedAt  time.Time   `json:"scrapedAt"`
}

// StatusValues maps human-readable status strings to DPMA form values.
var StatusValues = map[string]string{
	"registered": "IR",
	"applied":    "AN",
	"expired":    "EX",
	"deleted":    "LO",
}

// MarkTypeValues maps human-readable mark types to DPMA form values.
var MarkTypeValues = map[string]string{
	"word":       "W",
	"figurative": "B",
	"combined":   "WB",
	"3d":         "D",
	"sound":      "K",
	"color":      "F",
}
