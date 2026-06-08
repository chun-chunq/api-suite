package scraper

// Institution represents a BaFin-licensed financial institution.
type Institution struct {
	BaFinID     string   `json:"bafinId"`
	Name        string   `json:"name"`
	LegalForm   string   `json:"legalForm,omitempty"`
	Status      string   `json:"status"`       // "Active" | "Revoked" | "Withdrawn"
	LicenseType string   `json:"licenseType"`  // "Bank" | "InvestmentFirm" | "PaymentInstitution" | "CryptoAssets" | "Insurance" | "FundManager" | ...
	LicenseTypes []string `json:"licenseTypes,omitempty"` // all license types if multiple
	Address     string   `json:"address,omitempty"`
	City        string   `json:"city,omitempty"`
	Country     string   `json:"country,omitempty"`
	LEI         string   `json:"lei,omitempty"`  // Legal Entity Identifier if available
	DetailURL   string   `json:"detailUrl,omitempty"`
}

// SearchResult is the response for a BaFin institution search.
type SearchResult struct {
	Total        int           `json:"total"`
	Results      []Institution `json:"results"`
	Query        SearchQuery   `json:"query"`
}

// SearchQuery holds the parsed search parameters.
type SearchQuery struct {
	Name         string `json:"name,omitempty"`
	LicenseType  string `json:"licenseType,omitempty"` // filter by type
	Country      string `json:"country,omitempty"`
	StatusFilter string `json:"status,omitempty"`   // "active" | "revoked" | ""
	MaxResults   int    `json:"maxResults,omitempty"`
}
