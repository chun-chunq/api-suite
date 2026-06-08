package scraper

import (
	"strings"
	"time"
)

// stateDropdownValue resolves a Bundesland given either as a two-letter code
// ("BY") or a full name ("Bayern") to the portal dropdown value. Returns ""
// when the input is empty or unknown (i.e. "Alle Bundesländer").
func stateDropdownValue(state string) string {
	s := strings.TrimSpace(state)
	if s == "" {
		return ""
	}
	if idx, ok := stateCodeToIndex[strings.ToUpper(s)]; ok {
		return idx
	}
	// Try matching by full name (case-insensitive).
	for code, name := range Bundeslaender {
		if strings.EqualFold(name, s) {
			return stateCodeToIndex[code]
		}
	}
	return ""
}

// DebtorAddress holds the structured address of an insolvency debtor.
type DebtorAddress struct {
	Street string `json:"street"`
	City   string `json:"city"`
	PLZ    string `json:"plz"`
}

// Administrator holds details about the appointed insolvency administrator
// (Insolvenzverwalter).
type Administrator struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
	Email string `json:"email"`
}

// Record is a single insolvency announcement.
type Record struct {
	Aktenzeichen    string        `json:"aktenzeichen"`
	Court           string        `json:"court"`
	DebtorName      string        `json:"debtorName"`
	DebtorAddress   DebtorAddress `json:"debtorAddress"`
	PublicationDate string        `json:"publicationDate"` // ISO 8601 (YYYY-MM-DD)
	Subject         string        `json:"subject"`
	RegisterCourt   string        `json:"registerCourt"`
	RegisterType    string        `json:"registerType"`
	RegisterNumber  string        `json:"registerNumber"`
	Administrator   Administrator `json:"administrator"`
	FullText        string        `json:"fullText"`
	SourceURL       string        `json:"sourceUrl"`
}

// SearchQuery describes the parameters of an insolvency search.
type SearchQuery struct {
	Name           string
	State          string // Bundesland code, e.g. "BY"
	DateFrom       time.Time
	DateTo         time.Time
	RegisterType   string // HRB, HRA, ...
	RegisterNumber string
	Subject        string // Gegenstand / proceeding type: "all"|"opening"|"rejection"|"termination"|"security"
	MaxPages       int    // safety cap on pagination, 0 => default

	FirstName  string // Vorname (frm_suche:litx_vorname:text)
	City       string // Sitz/Wohnsitz (frm_suche:litx_sitzWohnsitz:text)
	MatchMode  string // "exact"|"startswith"|"contains" -> default "startswith"
	CaseNumber string // Aktenzeichen as string, e.g. "15 IN 80/23"
	Court      string // Gericht (frm_suche:lsom_gericht:lsom), depends on State
}

// stateCodeToIndex maps two-letter Bundesland codes to the portal dropdown
// values used by frm_suche:lsom_bundesland:lsom.
var stateCodeToIndex = map[string]string{
	"BW": "0",
	"BY": "1",
	"BE": "2",
	"BB": "3",
	"HB": "4",
	"HH": "5",
	"HE": "6",
	"MV": "7",
	"NI": "8",
	"NW": "9",
	"RP": "10",
	"SL": "11",
	"SN": "12",
	"ST": "13",
	"SH": "14",
	"TH": "15",
}

// SearchResult is the outcome of a paginated search.
type SearchResult struct {
	Records    []Record `json:"records"`
	Totalfound int      `json:"totalFound"`
	Pages      int      `json:"pages"`
}

// Bundesländer maps the official two-letter state codes used by the portal.
var Bundeslaender = map[string]string{
	"BW": "Baden-Württemberg",
	"BY": "Bayern",
	"BE": "Berlin",
	"BB": "Brandenburg",
	"HB": "Bremen",
	"HH": "Hamburg",
	"HE": "Hessen",
	"MV": "Mecklenburg-Vorpommern",
	"NI": "Niedersachsen",
	"NW": "Nordrhein-Westfalen",
	"RP": "Rheinland-Pfalz",
	"SL": "Saarland",
	"SN": "Sachsen",
	"ST": "Sachsen-Anhalt",
	"SH": "Schleswig-Holstein",
	"TH": "Thüringen",
}
