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

func TestSearchByName_OK(t *testing.T) {
	resp := map[string]interface{}{
		"IdentifierList": map[string]interface{}{
			"CID": []int64{2519, 5988, 100},
		},
	}
	b, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	sr, err := c.SearchByName(context.Background(), "aspirin", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sr.CIDs) != 2 {
		t.Errorf("expected 2 CIDs (limit), got %d", len(sr.CIDs))
	}
	if sr.Total != 3 {
		t.Errorf("expected total=3, got %d", sr.Total)
	}
	if sr.Query != "aspirin" {
		t.Errorf("expected query=aspirin, got %q", sr.Query)
	}
}

func TestSearchByName_EmptyName(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused"}
	_, err := c.SearchByName(context.Background(), "", 5)
	if err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestSearchByName_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"Fault":{"Message":"No CID found"}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.SearchByName(context.Background(), "xyznonexistentcompound", 5)
	if err == nil {
		t.Fatal("expected not_found error")
	}
}

func TestGetByCID_OK(t *testing.T) {
	resp := map[string]interface{}{
		"PropertyTable": map[string]interface{}{
			"Properties": []map[string]interface{}{
				{
					"CID":                    float64(2519),
					"IUPACName":              "2-acetyloxybenzoic acid",
					"MolecularFormula":       "C9H8O4",
					"MolecularWeight":        float64(180.16),
					"CanonicalSMILES":        "CC(=O)OC1=CC=CC=C1C(=O)O",
					"InChIKey":               "BSYNRYMUTXBXSQ-UHFFFAOYSA-N",
					"XLogP":                  float64(1.2),
					"HBondDonorCount":        float64(1),
					"HBondAcceptorCount":     float64(4),
					"RotatableBondCount":     float64(3),
					"TPSA":                   float64(63.6),
					"Charge":                 float64(0),
				},
			},
		},
	}
	b, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	compound, err := c.GetByCID(context.Background(), 2519)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if compound.CID != 2519 {
		t.Errorf("expected CID 2519, got %d", compound.CID)
	}
	if compound.Name != "2-acetyloxybenzoic acid" {
		t.Errorf("unexpected name: %q", compound.Name)
	}
	if compound.MolFormula != "C9H8O4" {
		t.Errorf("unexpected formula: %q", compound.MolFormula)
	}
	if compound.MolWeight != 180.16 {
		t.Errorf("unexpected weight: %f", compound.MolWeight)
	}
	if compound.HBondDonors != 1 {
		t.Errorf("unexpected HBondDonors: %d", compound.HBondDonors)
	}
}

func TestGetByCID_InvalidCID(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused"}
	_, err := c.GetByCID(context.Background(), -1)
	if err == nil {
		t.Fatal("expected error for negative CID")
	}
}

func TestGetByCID_EmptyProperties(t *testing.T) {
	resp := map[string]interface{}{
		"PropertyTable": map[string]interface{}{
			"Properties": []map[string]interface{}{},
		},
	}
	b, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.GetByCID(context.Background(), 99999999)
	if err == nil {
		t.Fatal("expected not_found error for empty properties")
	}
}

func TestGetSynonyms_OK(t *testing.T) {
	resp := map[string]interface{}{
		"InformationList": map[string]interface{}{
			"Information": []map[string]interface{}{
				{
					"Synonym": []string{"aspirin", "Bayer Aspirin", "acetylsalicylic acid", "ASA"},
				},
			},
		},
	}
	b, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	synonyms, err := c.GetSynonyms(context.Background(), 2519, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(synonyms) != 3 {
		t.Errorf("expected 3 synonyms (limit), got %d", len(synonyms))
	}
}

func TestGetSynonyms_InvalidCID(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused"}
	_, err := c.GetSynonyms(context.Background(), 0, 10)
	if err == nil {
		t.Fatal("expected error for CID=0")
	}
}

func TestGetDescription_OK(t *testing.T) {
	resp := map[string]interface{}{
		"InformationList": map[string]interface{}{
			"Information": []map[string]interface{}{
				{
					"Title":       "Aspirin",
					"Description": "Aspirin is a salicylate drug, often used as an analgesic.",
				},
			},
		},
	}
	b, _ := json.Marshal(resp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	desc, err := c.GetDescription(context.Background(), 2519)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if desc == "" {
		t.Error("expected non-empty description")
	}
	if desc != "Aspirin is a salicylate drug, often used as an analgesic." {
		t.Errorf("unexpected description: %q", desc)
	}
}

func TestGetDescription_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"Fault":{"Message":"No description"}}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	// Should return empty string, not error
	desc, err := c.GetDescription(context.Background(), 99999)
	if err != nil {
		t.Fatalf("expected no error for missing description, got: %v", err)
	}
	if desc != "" {
		t.Errorf("expected empty description, got %q", desc)
	}
}

func TestStrVal(t *testing.T) {
	m := map[string]interface{}{"key": "value", "num": float64(42)}
	if strVal(m, "key") != "value" {
		t.Error("strVal failed for string")
	}
	if strVal(m, "num") != "" {
		t.Error("strVal should return empty for non-string")
	}
	if strVal(m, "missing") != "" {
		t.Error("strVal should return empty for missing key")
	}
}

func TestFloatVal(t *testing.T) {
	m := map[string]interface{}{"f": float64(3.14), "i": int(5)}
	if floatVal(m, "f") != 3.14 {
		t.Errorf("floatVal wrong: %f", floatVal(m, "f"))
	}
	if floatVal(m, "i") != 5.0 {
		t.Errorf("floatVal from int wrong: %f", floatVal(m, "i"))
	}
}

func TestIntVal(t *testing.T) {
	m := map[string]interface{}{"n": float64(7)}
	if intVal(m, "n") != 7 {
		t.Errorf("intVal wrong: %d", intVal(m, "n"))
	}
}
