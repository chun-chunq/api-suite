package client

import (
	"strings"
	"testing"
)

func TestEscapeQ_SingleWord(t *testing.T) {
	r := escapeQ("robotics")
	if r != "robotics" {
		t.Errorf("want 'robotics', got '%s'", r)
	}
}

func TestEscapeQ_MultiWord(t *testing.T) {
	r := escapeQ("artificial intelligence")
	if !strings.HasPrefix(r, `"`) || !strings.HasSuffix(r, `"`) {
		t.Errorf("multi-word should be quoted, got '%s'", r)
	}
}

func TestEscapeQ_Empty(t *testing.T) {
	r := escapeQ("")
	if r != "" {
		t.Errorf("empty string should stay empty, got '%s'", r)
	}
}

func TestMapProject_Basic(t *testing.T) {
	p := cordisProject{
		ID:      "101016775",
		Acronym: "TESTROB",
		Title:   "Test Robotics Project",
		Status:  "ACTIVE",
	}
	proj := mapProject(p)
	if proj.ID != "101016775" {
		t.Errorf("ID want 101016775, got %s", proj.ID)
	}
	if proj.Acronym != "TESTROB" {
		t.Errorf("Acronym want TESTROB, got %s", proj.Acronym)
	}
	if proj.URL != "https://cordis.europa.eu/project/id/101016775" {
		t.Errorf("URL mismatch: %s", proj.URL)
	}
}

func TestMapProject_TopicsString(t *testing.T) {
	p := cordisProject{
		ID:     "123",
		Topics: "AI;robotics; climate",
	}
	proj := mapProject(p)
	if len(proj.Topics) != 3 {
		t.Errorf("want 3 topics, got %d: %v", len(proj.Topics), proj.Topics)
	}
}

func TestMapProject_ObjectiveTruncation(t *testing.T) {
	longObj := strings.Repeat("x", 600)
	p := cordisProject{ID: "1", Objective: longObj}
	proj := mapProject(p)
	if len(proj.Objective) > 503 {
		t.Errorf("objective should be truncated to <=503 chars, got %d", len(proj.Objective))
	}
	if !strings.HasSuffix(proj.Objective, "...") {
		t.Error("truncated objective should end with ...")
	}
}

func TestMapProject_ShortObjective(t *testing.T) {
	p := cordisProject{ID: "1", Objective: "Short description"}
	proj := mapProject(p)
	if proj.Objective != "Short description" {
		t.Errorf("short objective should be unchanged, got %s", proj.Objective)
	}
}

func TestParseCordisResponse_Empty(t *testing.T) {
	body := []byte(`{"header":{"numFound":0},"results":[]}`)
	result, err := parseCordisResponse(body, SearchQuery{MaxResults: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Total != 0 {
		t.Errorf("total want 0, got %d", result.Total)
	}
	if len(result.Results) != 0 {
		t.Errorf("results want 0, got %d", len(result.Results))
	}
}

func TestSearchQuery_Defaults(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("New() returned nil")
	}
	// Verify default max results clamping
	q := SearchQuery{MaxResults: 0}
	if q.MaxResults <= 0 {
		q.MaxResults = 25
	}
	if q.MaxResults != 25 {
		t.Errorf("default MaxResults want 25, got %d", q.MaxResults)
	}
}
