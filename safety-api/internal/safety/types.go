package safety

// Alert represents a single EU Safety Gate / RAPEX product alert.
type Alert struct {
	Reference   string   `json:"reference"`   // e.g. "A12/0123/24"
	AlertType   string   `json:"alertType"`   // "RAPEX" | "INFORMATION"
	Date        string   `json:"date"`        // YYYY-MM-DD
	Week        string   `json:"week"`        // "2024-W12"
	Country     string   `json:"country"`     // notifying country
	ProductName string   `json:"productName"`
	ProductType string   `json:"productType"` // category e.g. "Toys"
	Brand       string   `json:"brand,omitempty"`
	BatchNumber string   `json:"batchNumber,omitempty"`
	BarCode     string   `json:"barCode,omitempty"`
	Origin      string   `json:"origin,omitempty"` // country of origin/manufacture
	RiskType    string   `json:"riskType"`    // "Injuries", "Chemical", "Electrical hazard"…
	RiskLevel   string   `json:"riskLevel"`   // "Serious" | "High" | "Unknown"
	Description string   `json:"description,omitempty"`
	Measures    string   `json:"measures,omitempty"`   // actions taken
	Distribution []string `json:"distribution,omitempty"` // countries where sold
	URL         string   `json:"url,omitempty"`        // link to EU Safety Gate page
}

// SearchResult is the response for a search query.
type SearchResult struct {
	Total   int     `json:"total"`
	Results []Alert `json:"results"`
	Query   SearchQuery `json:"query"`
	DataDate string  `json:"dataDate"` // when the alert data was last updated
}

// SearchQuery holds the parsed query parameters.
type SearchQuery struct {
	Product  string `json:"product,omitempty"`
	Brand    string `json:"brand,omitempty"`
	Category string `json:"category,omitempty"`
	Country  string `json:"country,omitempty"`  // notifying country (DE, FR, …)
	Origin   string `json:"origin,omitempty"`   // manufacturing country
	Risk     string `json:"risk,omitempty"`
	From     string `json:"from,omitempty"`     // YYYY-MM-DD
	To       string `json:"to,omitempty"`
	MaxResults int  `json:"maxResults,omitempty"`
}

// Status reports the state of the in-memory database.
type Status struct {
	Loaded      bool   `json:"loaded"`
	AlertCount  int    `json:"alertCount"`
	DataDate    string `json:"dataDate"`
	NextRefresh string `json:"nextRefresh"`
}
