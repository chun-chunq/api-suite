package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestClient(srv *httptest.Server) *Client {
	c := New("")
	c.http = srv.Client()
	c.baseURL = srv.URL
	return c
}

// ── helpers ───────────────────────────────────────────────────────────────────

func TestTruncate(t *testing.T) {
	if got := truncate("hello", 10); got != "hello" {
		t.Errorf("got %q", got)
	}
	if got := truncate("hello world", 5); got != "hello…" {
		t.Errorf("got %q", got)
	}
}

func TestClamp(t *testing.T) {
	if clamp(0, 1, 10) != 1 {
		t.Error("below min")
	}
	if clamp(20, 1, 10) != 10 {
		t.Error("above max")
	}
	if clamp(5, 1, 10) != 5 {
		t.Error("in range")
	}
}

// ── SearchDrugLabels ──────────────────────────────────────────────────────────

func labelResponse() map[string]interface{} {
	return map[string]interface{}{
		"meta": map[string]interface{}{
			"results": map[string]interface{}{
				"total": float64(1),
				"skip":  float64(0),
				"limit": float64(10),
			},
		},
		"results": []interface{}{
			map[string]interface{}{
				"id": "abc-123",
				"openfda": map[string]interface{}{
					"brand_name":        []interface{}{"Aspirin"},
					"generic_name":      []interface{}{"aspirin"},
					"manufacturer_name": []interface{}{"Bayer"},
					"product_type":      []interface{}{"HUMAN OTC DRUG"},
					"route":             []interface{}{"ORAL"},
					"substance_name":    []interface{}{"ASPIRIN"},
				},
				"indications_and_usage": []interface{}{"For the temporary relief of minor aches and pains."},
				"warnings":              []interface{}{"Reye's syndrome warning."},
				"effective_time":        "20240101",
			},
		},
	}
}

func TestSearchDrugLabels_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/drug/label.json" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(labelResponse())
	}))
	defer srv.Close()

	c := newTestClient(srv)
	res, err := c.SearchDrugLabels(context.Background(), "aspirin", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Total != 1 {
		t.Errorf("expected total=1, got %d", res.Total)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}
	if res.Items[0].BrandName != "Aspirin" {
		t.Errorf("expected Aspirin, got %s", res.Items[0].BrandName)
	}
	if res.Items[0].GenericName != "aspirin" {
		t.Errorf("expected aspirin, got %s", res.Items[0].GenericName)
	}
}

func TestSearchDrugLabels_EmptyQuery(t *testing.T) {
	c := New("")
	_, err := c.SearchDrugLabels(context.Background(), "", 10, 0)
	if err == nil {
		t.Error("expected error for empty query")
	}
}

func TestSearchDrugLabels_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{
				"code":    "NOT_FOUND",
				"message": "No matches found!",
			},
		})
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.SearchDrugLabels(context.Background(), "xyzxyzxyz", 10, 0)
	if err == nil {
		t.Error("expected error for 404")
	}
}

// ── SearchAdverseEvents ───────────────────────────────────────────────────────

func adverseEventResponse() map[string]interface{} {
	return map[string]interface{}{
		"meta": map[string]interface{}{
			"results": map[string]interface{}{
				"total": float64(5),
				"skip":  float64(0),
				"limit": float64(5),
			},
		},
		"results": []interface{}{
			map[string]interface{}{
				"safetyreportid": "12345678",
				"receivedate":    "20240115",
				"serious":        "1",
				"seriousnessdeath": "0",
				"seriousnesshospitalization": "1",
				"patient": map[string]interface{}{
					"patientonsetage": "65",
					"patientsex":      "1",
					"drug": []interface{}{
						map[string]interface{}{
							"medicinalproduct": "IBUPROFEN",
						},
					},
					"reaction": []interface{}{
						map[string]interface{}{
							"reactionmeddrapt": "Gastrointestinal haemorrhage",
						},
					},
				},
			},
		},
	}
}

func TestSearchAdverseEvents_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(adverseEventResponse())
	}))
	defer srv.Close()

	c := newTestClient(srv)
	res, err := c.SearchAdverseEvents(context.Background(), "ibuprofen", 5, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}
	ae := res.Items[0]
	if ae.ReportID != "12345678" {
		t.Errorf("expected reportId 12345678, got %s", ae.ReportID)
	}
	if !ae.Serious {
		t.Error("expected serious=true")
	}
	if ae.Sex != "male" {
		t.Errorf("expected male, got %s", ae.Sex)
	}
	if len(ae.Drugs) != 1 || ae.Drugs[0] != "IBUPROFEN" {
		t.Errorf("drugs: got %v", ae.Drugs)
	}
	if len(ae.Reactions) != 1 {
		t.Errorf("reactions: got %v", ae.Reactions)
	}
}

func TestSearchAdverseEvents_EmptyDrug(t *testing.T) {
	c := New("")
	_, err := c.SearchAdverseEvents(context.Background(), "", 10, 0)
	if err == nil {
		t.Error("expected error for empty drug name")
	}
}

// ── SearchRecalls ─────────────────────────────────────────────────────────────

func recallResponse() map[string]interface{} {
	return map[string]interface{}{
		"meta": map[string]interface{}{
			"results": map[string]interface{}{
				"total": float64(2),
				"skip":  float64(0),
				"limit": float64(10),
			},
		},
		"results": []interface{}{
			map[string]interface{}{
				"recall_number":        "D-0001-2024",
				"status":               "Ongoing",
				"classification":       "Class II",
				"recalling_firm":       "Acme Pharma",
				"product_description":  "Aspirin 325mg tablets",
				"reason_for_recall":    "Microbial contamination",
				"country":              "US",
				"recall_initiation_date": "20240101",
			},
		},
	}
}

func TestSearchRecalls_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(recallResponse())
	}))
	defer srv.Close()

	c := newTestClient(srv)
	res, err := c.SearchRecalls(context.Background(), "aspirin", "", 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Total != 2 {
		t.Errorf("expected total=2, got %d", res.Total)
	}
	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}
	if res.Items[0].Classification != "Class II" {
		t.Errorf("classification: got %s", res.Items[0].Classification)
	}
}
