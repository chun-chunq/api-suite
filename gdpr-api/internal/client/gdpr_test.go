package client

import (
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("New() returned nil")
	}
}

func TestParseAmount(t *testing.T) {
	cases := []struct {
		input string
		want  float64
	}{
		{"1200000", 1200000},
		{"1,200,000", 1200000},
		{"0", 0},
		{"", 0},
		{"500000", 500000},
	}
	for _, tc := range cases {
		got := parseAmount(tc.input)
		if got != tc.want {
			t.Errorf("parseAmount(%q) want %f, got %f", tc.input, tc.want, got)
		}
	}
}

func TestFormatEUR(t *testing.T) {
	if s := formatEUR(1200000); !strings.Contains(s, "M") {
		t.Errorf("1.2M should contain M, got %s", s)
	}
	if s := formatEUR(500000); !strings.Contains(s, "K") {
		t.Errorf("500K should contain K, got %s", s)
	}
	if s := formatEUR(100); s != "€100" {
		t.Errorf("100 want €100, got %s", s)
	}
}

func TestParseArticles(t *testing.T) {
	arts := parseArticles("Art. 5, Art. 6 GDPR, Art. 17")
	if len(arts) != 3 {
		t.Errorf("want 3 articles, got %d: %v", len(arts), arts)
	}
	// should contain "5", "6", "17"
	found5 := false
	for _, a := range arts {
		if a == "5" {
			found5 = true
		}
	}
	if !found5 {
		t.Errorf("articles should contain '5', got %v", arts)
	}
}

func TestParseArticles_Empty(t *testing.T) {
	arts := parseArticles("")
	if arts != nil {
		t.Errorf("empty violated should return nil, got %v", arts)
	}
}

func TestMapRecord_Basic(t *testing.T) {
	r := trackerRecord{
		ID:         "ET-1",
		Date:       "2023-01-15",
		Country:    "DE",
		Authority:  "BfDI",
		Controller: "Deutsche Telekom AG",
		Quoted:     "9550000",
		Type:       "Fine",
		Violated:   "Art. 5, Art. 6",
		Summary:    "Inadequate data processing",
	}
	f := mapRecord(r)
	if f.ID != "ET-1" {
		t.Errorf("ID want ET-1, got %s", f.ID)
	}
	if f.Country != "DE" {
		t.Errorf("Country want DE, got %s", f.Country)
	}
	if f.Amount != 9550000 {
		t.Errorf("Amount want 9550000, got %f", f.Amount)
	}
	if f.Year != 2023 {
		t.Errorf("Year want 2023, got %d", f.Year)
	}
	if !strings.Contains(f.AmountStr, "M") {
		t.Errorf("AmountStr should be formatted, got %s", f.AmountStr)
	}
}

func TestMapRecord_SummaryTruncation(t *testing.T) {
	r := trackerRecord{
		ID:      "ET-2",
		Summary: strings.Repeat("x", 600),
		Quoted:  "1000",
	}
	f := mapRecord(r)
	if len(f.Summary) > 400 {
		t.Errorf("Summary should be truncated to <=400 chars, got %d", len(f.Summary))
	}
	if !strings.HasSuffix(f.Summary, "...") {
		t.Error("truncated summary should end with ...")
	}
}

func TestParseTrackerJSON_ArrayFormat(t *testing.T) {
	body := `[
		{"id":"ET-1","country":"DE","authority":"BfDI","controller":"Telekom","quoted":"1000000","type":"Fine","date":"2023-01-01","violated":"Art. 5"},
		{"id":"ET-2","country":"FR","authority":"CNIL","controller":"Orange","quoted":"500000","type":"Fine","date":"2022-06-01","violated":"Art. 6"}
	]`
	fines, err := parseTrackerJSON([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fines) != 2 {
		t.Errorf("want 2 fines, got %d", len(fines))
	}
	// should be sorted by amount desc (1M > 500K)
	if fines[0].Amount < fines[1].Amount {
		t.Error("fines should be sorted by amount descending")
	}
}

func TestParseTrackerJSON_WrappedFormat(t *testing.T) {
	body := `{"data":[
		{"id":"ET-1","country":"DE","authority":"BfDI","controller":"Test","quoted":"100","type":"Fine","date":"2023-01-01","violated":"Art. 5"}
	]}`
	fines, err := parseTrackerJSON([]byte(body))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fines) != 1 {
		t.Errorf("want 1 fine, got %d", len(fines))
	}
}

func TestComputeStats_Basic(t *testing.T) {
	fines := []Fine{
		{Country: "DE", Authority: "BfDI", Amount: 1000000},
		{Country: "DE", Authority: "BfDI", Amount: 500000},
		{Country: "FR", Authority: "CNIL", Amount: 200000},
	}
	stats := computeStats(fines)
	if stats.TotalFines != 3 {
		t.Errorf("TotalFines want 3, got %d", stats.TotalFines)
	}
	if stats.TotalAmount != 1700000 {
		t.Errorf("TotalAmount want 1700000, got %f", stats.TotalAmount)
	}
	if stats.MaxFine != 1000000 {
		t.Errorf("MaxFine want 1000000, got %f", stats.MaxFine)
	}
	if stats.TopCountry != "DE" {
		t.Errorf("TopCountry want DE, got %s", stats.TopCountry)
	}
}

func TestComputeStats_Empty(t *testing.T) {
	stats := computeStats(nil)
	if stats == nil {
		t.Error("stats should not be nil for empty slice")
	}
}
