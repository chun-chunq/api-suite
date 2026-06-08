package sanctions

// Entity represents a sanctioned person, company, or other subject.
type Entity struct {
	ID           string   `json:"id"`
	SubjectType  string   `json:"subjectType"`  // "person" | "entity" | "ship" | "other"
	Names        []Name   `json:"names"`
	Addresses    []string `json:"addresses,omitempty"`
	BirthDates   []string `json:"birthDates,omitempty"`
	Nationalities []string `json:"nationalities,omitempty"`
	Passports    []string `json:"passports,omitempty"`
	Remark       string   `json:"remark,omitempty"`
	Regulation   string   `json:"regulation,omitempty"` // legal basis
	Programme    string   `json:"programme,omitempty"`  // sanction programme (e.g. "RUSSIA")
}

// Name is a name variant for a sanctioned entity.
type Name struct {
	FirstName  string `json:"firstName,omitempty"`
	MiddleName string `json:"middleName,omitempty"`
	LastName   string `json:"lastName,omitempty"`
	WholeName  string `json:"wholeName"` // always set — full normalized name
	Language   string `json:"language,omitempty"`
}

// CheckResult is the response for a yes/no sanctions check.
type CheckResult struct {
	Sanctioned bool      `json:"sanctioned"`
	Matches    []Entity  `json:"matches,omitempty"`
	Query      string    `json:"query"`
	CheckedAt  string    `json:"checkedAt"`
	ListDate   string    `json:"listDate"` // when the list was last updated
}

// SearchResult is the full search response.
type SearchResult struct {
	Total     int      `json:"total"`
	Results   []Entity `json:"results"`
	Query     string   `json:"query"`
	ListDate  string   `json:"listDate"`
}

// Status reports the health of the sanctions data.
type Status struct {
	Loaded    bool   `json:"loaded"`
	Count     int    `json:"count"`
	ListDate  string `json:"listDate"`
	NextRefresh string `json:"nextRefresh"`
}
