package scraper

// SearchQuery holds all optional filter parameters for a ZVG search.
type SearchQuery struct {
	State          string   // Bundesland abbreviation: "by", "nw", "be", etc.
	CourtID        string   // Amtsgericht ID: "D2601" (München), "D3310" (Nürnberg), etc.
	CaseNumber     string   // Aktenzeichen free-text (az1 field)
	ProcedureType  string   // Verfahrensart: "", "-1", "0"..8 (see artValues)
	ObjectTypes    []string // Objektarten: "1"=Reihenhaus, "3"=Einfamilienhaus, etc.
	PostalCode     string   // PLZ
	City           string   // Ort
	Street         string   // Straße
	ObjectText     string   // free-text object search
	SortBy         string   // "2"=Termin (default), "1"=Aktualisierung, "3"=Aktenzeichen
	MaxResults     int      // 0 = first page only (10 results)
}

// Auction represents a single foreclosure auction entry.
type Auction struct {
	ZvgID             string `json:"zvgId"`
	CaseNumber        string `json:"caseNumber"`        // Aktenzeichen
	Court             string `json:"court"`             // Amtsgericht
	State             string `json:"state"`             // Bundesland abbreviation
	ObjectDescription string `json:"objectDescription"` // Objekt/Lage (description + address)
	MarketValue       string `json:"marketValue"`       // Verkehrswert in €
	AuctionDate       string `json:"auctionDate"`       // ISO date
	AuctionTime       string `json:"auctionTime"`       // HH:MM
	AuctionDateRaw    string `json:"auctionDateRaw"`    // original German text
	DetailURL         string `json:"detailUrl"`
	LastUpdated       string `json:"lastUpdated"`
}

// SearchResult wraps the list of auctions returned by a search.
type SearchResult struct {
	Auctions   []Auction `json:"auctions"`
	TotalFound int       `json:"totalFound"`
	State      string    `json:"state,omitempty"`
}

// Bundesland values understood by zvg-portal.de.
var StateValues = map[string]string{
	"BW": "bw", "BY": "by", "BE": "be", "BR": "br",
	"HB": "hb", "HH": "hh", "HE": "he", "MV": "mv",
	"NI": "ni", "NW": "nw", "RP": "rp", "SL": "sl",
	"SN": "sn", "ST": "st", "SH": "sh", "TH": "th",
	// also accept lowercase
	"bw": "bw", "by": "by", "be": "be", "br": "br",
	"hb": "hb", "hh": "hh", "he": "he", "mv": "mv",
	"ni": "ni", "nw": "nw", "rp": "rp", "sl": "sl",
	"sn": "sn", "st": "st", "sh": "sh", "th": "th",
	// long names
	"Bayern":                  "by",
	"NordrheinWestfalen":      "nw",
	"Nordrhein-Westfalen":     "nw",
	"BadenWuerttemberg":       "bw",
	"Baden-Württemberg":       "bw",
	"Berlin":                  "be",
	"Brandenburg":             "br",
	"Bremen":                  "hb",
	"Hamburg":                 "hh",
	"Hessen":                  "he",
	"MecklenburgVorpommern":   "mv",
	"Mecklenburg-Vorpommern":  "mv",
	"Niedersachsen":           "ni",
	"RheinlandPfalz":          "rp",
	"Rheinland-Pfalz":         "rp",
	"Saarland":                "sl",
	"Sachsen":                 "sn",
	"SachsenAnhalt":           "st",
	"Sachsen-Anhalt":          "st",
	"SchleswigHolstein":       "sh",
	"Schleswig-Holstein":      "sh",
	"Thueringen":              "th",
	"Thüringen":               "th",
}
