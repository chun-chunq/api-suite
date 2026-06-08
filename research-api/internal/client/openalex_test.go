package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	c := New("test@example.com")
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.email != "test@example.com" {
		t.Errorf("email want test@example.com, got %s", c.email)
	}
}

func TestSearchWorks_NoParams(t *testing.T) {
	c := New("")
	_, err := c.SearchWorks(context.Background(), WorkSearchQuery{})
	if err == nil {
		t.Error("expected error for empty query")
	}
}

func TestGetWorkByDOI_Empty(t *testing.T) {
	c := New("")
	_, err := c.GetWorkByDOI(context.Background(), "")
	if err == nil {
		t.Error("expected error for empty DOI")
	}
}

func TestReconstructAbstract_Basic(t *testing.T) {
	inverted := map[string][]int{
		"Hello":  {0},
		"world":  {1},
		"test":   {2},
	}
	result := reconstructAbstract(inverted)
	if result == "" {
		t.Error("expected non-empty abstract")
	}
	// should contain our words
	if !strings.Contains(result, "Hello") && !strings.Contains(result, "world") {
		t.Errorf("abstract should contain words, got: %s", result)
	}
}

func TestReconstructAbstract_Empty(t *testing.T) {
	result := reconstructAbstract(nil)
	if result != "" {
		t.Errorf("nil map should give empty string, got %s", result)
	}
}

func TestReconstructAbstract_Truncation(t *testing.T) {
	inverted := map[string][]int{}
	// create a very long abstract by putting many words at sequential positions
	for i := 0; i < 200; i++ {
		inverted[strings.Repeat("word", 3)] = append(inverted[strings.Repeat("word", 3)], i)
	}
	result := reconstructAbstract(inverted)
	if len(result) > 603 {
		t.Errorf("abstract should be truncated to <=603 chars, got %d", len(result))
	}
}

func TestMapWork_Basic(t *testing.T) {
	r := oaWork{
		ID:              "https://openalex.org/W2741809807",
		Title:           "Attention Is All You Need",
		PublicationYear: 2017,
		Type:            "journal-article",
		CitedByCount:    50000,
	}
	r.OpenAccess.IsOA = true
	r.OpenAccess.OAURL = "https://arxiv.org/pdf/1706.03762"

	w := mapWork(r)
	if w.ID != "W2741809807" {
		t.Errorf("ID want W2741809807, got %s", w.ID)
	}
	if w.Title != "Attention Is All You Need" {
		t.Errorf("Title mismatch: %s", w.Title)
	}
	if w.CitedByCount != 50000 {
		t.Errorf("CitedByCount want 50000, got %d", w.CitedByCount)
	}
	if !w.OpenAccess {
		t.Error("OpenAccess should be true")
	}
	if w.OAUrl == "" {
		t.Error("OAUrl should not be empty")
	}
}

func TestMapWork_Authors(t *testing.T) {
	r := oaWork{
		ID:    "https://openalex.org/W1",
		Title: "Test Paper",
		Authorships: []struct {
			Author struct {
				ID          string `json:"id"`
				DisplayName string `json:"display_name"`
				Orcid       string `json:"orcid"`
			} `json:"author"`
			Institutions []struct {
				DisplayName string `json:"display_name"`
			} `json:"institutions"`
		}{
			{
				Author: struct {
					ID          string `json:"id"`
					DisplayName string `json:"display_name"`
					Orcid       string `json:"orcid"`
				}{
					ID: "https://openalex.org/A1", DisplayName: "John Doe",
				},
				Institutions: []struct {
					DisplayName string `json:"display_name"`
				}{
					{DisplayName: "MIT"},
				},
			},
		},
	}
	w := mapWork(r)
	if len(w.Authors) != 1 {
		t.Fatalf("want 1 author, got %d", len(w.Authors))
	}
	if w.Authors[0].Name != "John Doe" {
		t.Errorf("Author name want John Doe, got %s", w.Authors[0].Name)
	}
	if w.Authors[0].Institution != "MIT" {
		t.Errorf("Institution want MIT, got %s", w.Authors[0].Institution)
	}
	if w.Authors[0].ID != "A1" {
		t.Errorf("Author ID want A1, got %s", w.Authors[0].ID)
	}
}

func TestMapWork_ConceptsLimit(t *testing.T) {
	r := oaWork{ID: "https://openalex.org/W1", Title: "T"}
	for i := 0; i < 10; i++ {
		r.Concepts = append(r.Concepts, struct {
			ID          string  `json:"id"`
			DisplayName string  `json:"display_name"`
			Score       float64 `json:"score"`
			Level       int     `json:"level"`
		}{
			ID: fmt.Sprintf("https://openalex.org/C%d", i), DisplayName: fmt.Sprintf("Concept%d", i), Score: 0.9,
		})
	}
	w := mapWork(r)
	if len(w.Concepts) > 5 {
		t.Errorf("want at most 5 concepts, got %d", len(w.Concepts))
	}
}

func TestMapInstitution_Basic(t *testing.T) {
	r := oaInstitution{
		ID:          "https://openalex.org/I27837315",
		DisplayName: "Massachusetts Institute of Technology",
		CountryCode: "US",
		Type:        "education",
		WorksCount:  200000,
		CitedByCount: 15000000,
	}
	r.IDs.ROR = "https://ror.org/042nb2s44"
	inst := mapInstitution(r)
	if inst.ID != "I27837315" {
		t.Errorf("ID want I27837315, got %s", inst.ID)
	}
	if inst.Country != "US" {
		t.Errorf("Country want US, got %s", inst.Country)
	}
	if inst.ROR == "" {
		t.Error("ROR should not be empty")
	}
}

func TestSearchWorks_ServerResponse(t *testing.T) {
	raw := oaListResponse{}
	raw.Meta.Count = 1
	raw.Meta.Page = 1
	raw.Meta.PerPage = 25
	raw.Results = []oaWork{
		{
			ID:              "https://openalex.org/W2741809807",
			Title:           "Attention Is All You Need",
			PublicationYear: 2017,
			CitedByCount:    50000,
		},
	}
	body, _ := json.Marshal(raw)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := &Client{http: &http.Client{}, baseURL: srv.URL}
	result, err := c.SearchWorks(context.Background(), WorkSearchQuery{Query: "transformer", MaxResults: 10})
	if err != nil {
		t.Fatalf("SearchWorks error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("Total want 1, got %d", result.Total)
	}
	if result.Results[0].Title != "Attention Is All You Need" {
		t.Errorf("Title mismatch: %s", result.Results[0].Title)
	}
}

