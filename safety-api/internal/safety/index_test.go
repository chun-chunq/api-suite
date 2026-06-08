package safety

import (
	"testing"
)

func TestNorm(t *testing.T) {
	if got := norm("  Toys  "); got != "toys" {
		t.Errorf("norm = %q, want 'toys'", got)
	}
}

func TestParseXML_Empty(t *testing.T) {
	xml := `<RAPEX></RAPEX>`
	alerts, _, err := parseXML([]byte(xml))
	if err != nil {
		t.Fatalf("parseXML error: %v", err)
	}
	if len(alerts) != 0 {
		t.Errorf("expected 0 alerts, got %d", len(alerts))
	}
}

func TestParseXML_SingleAlert(t *testing.T) {
	xmlData := `<RAPEX>
  <NOTIFICATION>
    <REFERENCE>A12/0001/24</REFERENCE>
    <TYPE>RAPEX</TYPE>
    <DATE>15/01/2024</DATE>
    <COUNTRY>DE</COUNTRY>
    <PRODUCT>
      <NAME>Fidget Spinner XL</NAME>
      <TYPE>Toys</TYPE>
      <BRAND>ToyBrand</BRAND>
      <COUNTRY_OF_ORIGIN>CN</COUNTRY_OF_ORIGIN>
    </PRODUCT>
    <RISK>
      <TYPE>Injuries</TYPE>
      <RISK_LEVEL>Serious</RISK_LEVEL>
      <DESCRIPTION>Risk of choking due to detachable small parts</DESCRIPTION>
    </RISK>
    <MEASURES>Withdrawal from the market</MEASURES>
  </NOTIFICATION>
</RAPEX>`
	alerts, _, err := parseXML([]byte(xmlData))
	if err != nil {
		t.Fatalf("parseXML error: %v", err)
	}
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert, got %d", len(alerts))
	}
	a := alerts[0]
	if a.Reference != "A12/0001/24" {
		t.Errorf("Reference = %q, want A12/0001/24", a.Reference)
	}
	if a.ProductName != "Fidget Spinner XL" {
		t.Errorf("ProductName = %q", a.ProductName)
	}
	if a.Date != "2024-01-15" {
		t.Errorf("Date = %q, want 2024-01-15", a.Date)
	}
	if a.Country != "DE" {
		t.Errorf("Country = %q, want DE", a.Country)
	}
	if a.RiskLevel != "Serious" {
		t.Errorf("RiskLevel = %q, want Serious", a.RiskLevel)
	}
	if a.Origin != "CN" {
		t.Errorf("Origin = %q, want CN", a.Origin)
	}
}

func TestMatchesQuery(t *testing.T) {
	a := Alert{
		ProductName: "Fidget Spinner",
		ProductType: "Toys",
		Brand:       "ToyBrand",
		Country:     "DE",
		Origin:      "CN",
		RiskType:    "Injuries",
		RiskLevel:   "Serious",
		Date:        "2024-01-15",
	}

	if !matchesQuery(a, SearchQuery{Product: "fidget"}) {
		t.Error("should match 'fidget' in product name")
	}
	if !matchesQuery(a, SearchQuery{Category: "toy"}) {
		t.Error("should match 'toy' in category")
	}
	if !matchesQuery(a, SearchQuery{Country: "DE"}) {
		t.Error("should match country DE")
	}
	if matchesQuery(a, SearchQuery{Country: "FR"}) {
		t.Error("should NOT match country FR")
	}
	if !matchesQuery(a, SearchQuery{From: "2024-01-01", To: "2024-12-31"}) {
		t.Error("should match date range")
	}
	if matchesQuery(a, SearchQuery{From: "2025-01-01"}) {
		t.Error("should NOT match future from-date")
	}
	if !matchesQuery(a, SearchQuery{Risk: "injur"}) {
		t.Error("should match partial risk type")
	}
}

func TestNormalizeDate(t *testing.T) {
	cases := []struct{ in, want string }{
		{"15/01/2024", "2024-01-15"},
		{"2024-01-15", "2024-01-15"},
		{"", ""},
	}
	for _, c := range cases {
		got := normalizeDate(c.in)
		if c.want != "" && got != c.want {
			t.Errorf("normalizeDate(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
