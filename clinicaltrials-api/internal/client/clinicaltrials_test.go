package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(srv *httptest.Server) *Client {
	return &Client{
		http:    srv.Client(),
		baseURL: srv.URL,
	}
}

// makeRawStudy builds the nested ClinicalTrials API structure.
func makeRawStudy(nctID, title, status string) map[string]interface{} {
	return map[string]interface{}{
		"protocolSection": map[string]interface{}{
			"identificationModule": map[string]interface{}{
				"nctId":      nctID,
				"briefTitle": title,
			},
			"statusModule": map[string]interface{}{
				"overallStatus": status,
				"startDateStruct": map[string]interface{}{
					"date": "2022-01",
				},
				"completionDateStruct": map[string]interface{}{
					"date": "2024-12",
				},
			},
			"descriptionModule": map[string]interface{}{
				"briefSummary": "A clinical study to evaluate the safety and efficacy of the treatment.",
			},
			"conditionsModule": map[string]interface{}{
				"conditions": []interface{}{"Diabetes", "Hypertension"},
			},
			"armsInterventionsModule": map[string]interface{}{
				"interventions": []interface{}{
					map[string]interface{}{"type": "DRUG", "name": "TestDrug"},
				},
			},
			"sponsorCollaboratorsModule": map[string]interface{}{
				"leadSponsor": map[string]interface{}{
					"name": "Pharma Inc",
				},
			},
			"designModule": map[string]interface{}{
				"phases": []interface{}{"PHASE2", "PHASE3"},
			},
			"contactsLocationsModule": map[string]interface{}{
				"locations": []interface{}{
					map[string]interface{}{"country": "United States"},
					map[string]interface{}{"country": "Germany"},
				},
			},
		},
	}
}

// ── Search tests ──────────────────────────────────────────────────────────────

func TestSearch_OK(t *testing.T) {
	study1 := makeRawStudy("NCT00000001", "Study Alpha", "RECRUITING")
	study2 := makeRawStudy("NCT00000002", "Study Beta", "COMPLETED")

	body, _ := json.Marshal(map[string]interface{}{
		"studies":    []interface{}{study1, study2},
		"totalCount": 2,
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/studies" {
			http.Error(w, "expected /studies", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.Search(context.Background(), "diabetes", "", "", 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Count != 2 {
		t.Errorf("expected 2 studies, got %d", result.Count)
	}
	if result.Studies[0].NCTId != "NCT00000001" {
		t.Errorf("expected NCT00000001, got %q", result.Studies[0].NCTId)
	}
	if result.Studies[0].Title != "Study Alpha" {
		t.Errorf("expected 'Study Alpha', got %q", result.Studies[0].Title)
	}
	if len(result.Studies[0].Conditions) != 2 {
		t.Errorf("expected 2 conditions, got %d", len(result.Studies[0].Conditions))
	}
	if len(result.Studies[0].Phase) != 2 {
		t.Errorf("expected 2 phases, got %d", len(result.Studies[0].Phase))
	}
	if len(result.Studies[0].Locations) != 2 {
		t.Errorf("expected 2 locations, got %d", len(result.Studies[0].Locations))
	}
}

func TestSearch_DefaultLimit(t *testing.T) {
	body, _ := json.Marshal(map[string]interface{}{
		"studies":    []interface{}{},
		"totalCount": 0,
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageSize := r.URL.Query().Get("pageSize")
		if pageSize != "10" {
			http.Error(w, "expected pageSize=10, got "+pageSize, 400)
			return
		}
		w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Search(context.Background(), "cancer", "", "", -5, "") // -5 → default 10
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearch_WithFilters(t *testing.T) {
	body, _ := json.Marshal(map[string]interface{}{
		"studies":    []interface{}{},
		"totalCount": 0,
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("filter.overallStatus")
		phase := r.URL.Query().Get("filter.phase")
		if status != "RECRUITING" {
			http.Error(w, "expected status RECRUITING, got "+status, 400)
			return
		}
		if phase != "PHASE3" {
			http.Error(w, "expected phase PHASE3, got "+phase, 400)
			return
		}
		w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.Search(context.Background(), "cancer", "recruiting", "phase3", 5, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSearch_Pagination(t *testing.T) {
	body, _ := json.Marshal(map[string]interface{}{
		"studies":       []interface{}{},
		"totalCount":    100,
		"nextPageToken": "tok_page2",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	result, err := c.Search(context.Background(), "test", "", "", 10, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.NextToken != "tok_page2" {
		t.Errorf("expected next token 'tok_page2', got %q", result.NextToken)
	}
	if result.Total != 100 {
		t.Errorf("expected total 100, got %d", result.Total)
	}
}

// ── GetStudy tests ────────────────────────────────────────────────────────────

func TestGetStudy_OK(t *testing.T) {
	study := makeRawStudy("NCT12345678", "Specific Study", "COMPLETED")
	body, _ := json.Marshal(study)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/studies/NCT12345678" {
			http.Error(w, "expected /studies/NCT12345678", 404)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	s, err := c.GetStudy(context.Background(), "NCT12345678")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.NCTId != "NCT12345678" {
		t.Errorf("expected NCT12345678, got %q", s.NCTId)
	}
	if s.URL != "https://clinicaltrials.gov/study/NCT12345678" {
		t.Errorf("unexpected URL: %q", s.URL)
	}
	if s.Sponsor != "Pharma Inc" {
		t.Errorf("expected sponsor 'Pharma Inc', got %q", s.Sponsor)
	}
}

func TestGetStudy_EmptyID(t *testing.T) {
	c := New()
	_, err := c.GetStudy(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty NCT ID")
	}
}

func TestGetStudy_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.GetStudy(context.Background(), "NCT99999999")
	if err == nil {
		t.Fatal("expected not_found error")
	}
}

// ── extractStudy edge case ────────────────────────────────────────────────────

func TestExtractStudy_BriefSummaryTruncation(t *testing.T) {
	longSummary := make([]byte, 600)
	for i := range longSummary {
		longSummary[i] = 'a'
	}

	raw := makeRawStudy("NCT00000003", "Long Summary Study", "RECRUITING")
	raw["protocolSection"].(map[string]interface{})["descriptionModule"] = map[string]interface{}{
		"briefSummary": string(longSummary),
	}

	study := extractStudy(raw)
	if len(study.BriefSummary) > 503 { // 500 + "..." = 503
		t.Errorf("expected summary truncated to max 503 chars, got %d", len(study.BriefSummary))
	}
}
