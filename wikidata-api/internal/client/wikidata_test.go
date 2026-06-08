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
	c.wikidataURL = srv.URL + "/api"
	return c
}

// ── Search ────────────────────────────────────────────────────────────────────

func searchResponse() map[string]interface{} {
	return map[string]interface{}{
		"searchinfo": map[string]interface{}{"search": "Douglas Adams"},
		"search": []interface{}{
			map[string]interface{}{
				"id":          "Q42",
				"label":       "Douglas Adams",
				"description": "English author and humourist",
				"aliases": []interface{}{
					map[string]interface{}{"value": "Douglas Noel Adams"},
				},
				"url":        "//www.wikidata.org/wiki/Q42",
				"entityType": "item",
			},
		},
		"search-continue": float64(0),
	}
}

func TestSearch_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") != "wbsearchentities" {
			t.Errorf("unexpected action: %s", r.URL.Query().Get("action"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(searchResponse())
	}))
	defer srv.Close()

	c := newTestClient(srv)
	results, _, err := c.Search(context.Background(), "Douglas Adams", "en", "item", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].ID != "Q42" {
		t.Errorf("expected Q42, got %s", results[0].ID)
	}
	if results[0].Label != "Douglas Adams" {
		t.Errorf("expected Douglas Adams, got %s", results[0].Label)
	}
	if len(results[0].Aliases) != 1 {
		t.Errorf("expected 1 alias, got %d", len(results[0].Aliases))
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	c := New("")
	_, _, err := c.Search(context.Background(), "", "en", "item", 5)
	if err == nil {
		t.Error("expected error for empty query")
	}
}

// ── GetEntity ─────────────────────────────────────────────────────────────────

func entityResponse() map[string]interface{} {
	return map[string]interface{}{
		"entities": map[string]interface{}{
			"Q42": map[string]interface{}{
				"id":   "Q42",
				"type": "item",
				"labels": map[string]interface{}{
					"en": map[string]interface{}{
						"language": "en",
						"value":    "Douglas Adams",
					},
				},
				"descriptions": map[string]interface{}{
					"en": map[string]interface{}{
						"language": "en",
						"value":    "English author and humourist",
					},
				},
				"aliases": map[string]interface{}{
					"en": []interface{}{
						map[string]interface{}{"language": "en", "value": "Douglas Noel Adams"},
					},
				},
				"claims": map[string]interface{}{
					"P31": []interface{}{
						map[string]interface{}{
							"mainsnak": map[string]interface{}{
								"snaktype": "value",
								"property": "P31",
								"datavalue": map[string]interface{}{
									"type": "wikibase-entityid",
									"value": map[string]interface{}{
										"entity-type": "item",
										"id":          "Q5",
									},
								},
							},
						},
					},
					"P856": []interface{}{
						map[string]interface{}{
							"mainsnak": map[string]interface{}{
								"snaktype": "value",
								"property": "P856",
								"datavalue": map[string]interface{}{
									"type":  "string",
									"value": "https://www.douglasadams.com",
								},
							},
						},
					},
				},
			},
		},
	}
}

func TestGetEntity_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("action") != "wbgetentities" {
			t.Errorf("unexpected action: %s", r.URL.Query().Get("action"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entityResponse())
	}))
	defer srv.Close()

	c := newTestClient(srv)
	ent, err := c.GetEntity(context.Background(), "Q42", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ent.ID != "Q42" {
		t.Errorf("expected Q42, got %s", ent.ID)
	}
	if ent.Label != "Douglas Adams" {
		t.Errorf("expected Douglas Adams, got %s", ent.Label)
	}
	if ent.Description != "English author and humourist" {
		t.Errorf("description: %s", ent.Description)
	}
	if len(ent.Aliases) != 1 {
		t.Errorf("expected 1 alias, got %d", len(ent.Aliases))
	}
	if len(ent.InstanceOf) != 1 || ent.InstanceOf[0] != "Q5" {
		t.Errorf("instanceOf: %v", ent.InstanceOf)
	}
	if ent.OfficialSite != "https://www.douglasadams.com" {
		t.Errorf("officialSite: %s", ent.OfficialSite)
	}
}

func TestGetEntity_EmptyID(t *testing.T) {
	c := New("")
	_, err := c.GetEntity(context.Background(), "", "en")
	if err == nil {
		t.Error("expected error for empty ID")
	}
}

func TestGetEntity_Normalizes_Lowercase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids := r.URL.Query().Get("ids")
		if ids != "Q42" {
			t.Errorf("expected Q42 after normalization, got %s", ids)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entityResponse())
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.GetEntity(context.Background(), "q42", "en") // lowercase
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── extractStringValues ───────────────────────────────────────────────────────

func TestExtractStringValues_EntityID(t *testing.T) {
	claims := map[string]interface{}{
		"P31": []interface{}{
			map[string]interface{}{
				"mainsnak": map[string]interface{}{
					"datavalue": map[string]interface{}{
						"type":  "wikibase-entityid",
						"value": map[string]interface{}{"id": "Q5"},
					},
				},
			},
		},
	}
	vals := extractStringValues(claims, "P31")
	if len(vals) != 1 || vals[0] != "Q5" {
		t.Errorf("got %v", vals)
	}
}

func TestExtractStringValues_Missing(t *testing.T) {
	vals := extractStringValues(map[string]interface{}{}, "P999")
	if len(vals) != 0 {
		t.Errorf("expected empty, got %v", vals)
	}
}
