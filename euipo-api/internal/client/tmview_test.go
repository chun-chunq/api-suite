package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	c := New()
	if c == nil {
		t.Fatal("New() returned nil")
	}
	if c.http == nil {
		t.Fatal("http client is nil")
	}
	if c.baseURL == "" {
		t.Fatal("baseURL is empty")
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	c := New()
	_, err := c.Search(context.Background(), SearchQuery{})
	if err == nil {
		t.Error("expected error for empty query and holder, got nil")
	}
}

func TestSearch_DefaultsMaxResults(t *testing.T) {
	q := SearchQuery{Query: "Nike", MaxResults: 0}
	if q.MaxResults <= 0 {
		q.MaxResults = 25
	}
	if q.MaxResults != 25 {
		t.Errorf("want default MaxResults=25, got %d", q.MaxResults)
	}
}

func TestMapTrademark_Basic(t *testing.T) {
	r := tmviewRaw{
		ST13:              "EM012345678",
		TrademarkName:     "TESTMARK",
		ApplicationNumber: "012345678",
		TrademarkStatus:   "REGISTERED",
		Office:            "EM",
		OfficeName:        "European Union Intellectual Property Office",
		MarkFeature:       "WORD",
		FilingDate:        "2020-01-15",
	}
	tm := mapTrademark(r)
	if tm.ID != "EM012345678" {
		t.Errorf("ID want EM012345678, got %s", tm.ID)
	}
	if tm.Name != "TESTMARK" {
		t.Errorf("Name want TESTMARK, got %s", tm.Name)
	}
	if tm.Status != "REGISTERED" {
		t.Errorf("Status want REGISTERED, got %s", tm.Status)
	}
	if tm.Office != "EM" {
		t.Errorf("Office want EM, got %s", tm.Office)
	}
	if tm.Type != "word" {
		t.Errorf("Type want word, got %s", tm.Type)
	}
	if !strings.Contains(tm.URL, "EM012345678") {
		t.Errorf("URL should contain ID, got %s", tm.URL)
	}
}

func TestMapTrademark_NiceClasses(t *testing.T) {
	r := tmviewRaw{
		ST13: "EM999",
		NiceClasses: []struct {
			NiceClass        int    `json:"niceClass"`
			GoodsAndServices string `json:"goodsAndServices"`
		}{
			{NiceClass: 25, GoodsAndServices: "Clothing, footwear"},
			{NiceClass: 35, GoodsAndServices: "Retail services"},
		},
	}
	tm := mapTrademark(r)
	if len(tm.NiceClasses) != 2 {
		t.Errorf("want 2 classes, got %d", len(tm.NiceClasses))
	}
	if tm.NiceClasses[0] != 25 {
		t.Errorf("want class 25, got %d", tm.NiceClasses[0])
	}
	if !strings.Contains(tm.Goods, "[25]") {
		t.Errorf("Goods should contain [25], got %s", tm.Goods)
	}
}

func TestMapTrademark_GoodsTruncation(t *testing.T) {
	longGoods := strings.Repeat("x", 600)
	r := tmviewRaw{
		ST13: "EM1",
		NiceClasses: []struct {
			NiceClass        int    `json:"niceClass"`
			GoodsAndServices string `json:"goodsAndServices"`
		}{
			{NiceClass: 1, GoodsAndServices: longGoods},
		},
	}
	tm := mapTrademark(r)
	if len(tm.Goods) > 503 {
		t.Errorf("Goods should be truncated to <=503 chars, got %d", len(tm.Goods))
	}
	if !strings.HasSuffix(tm.Goods, "...") {
		t.Error("truncated Goods should end with ...")
	}
}

func TestMapTrademark_ImageURL(t *testing.T) {
	r := tmviewRaw{
		ST13:     "EM123",
		ImageURI: "/tmview/images/logo.png",
	}
	tm := mapTrademark(r)
	if !strings.HasPrefix(tm.ImageURL, "https://www.tmdn.org") {
		t.Errorf("ImageURL should have tmdn.org prefix, got %s", tm.ImageURL)
	}
}

func TestSearch_ServerResponse(t *testing.T) {
	raw := tmviewSearchResponse{
		TotalResults: 1,
		Results: []tmviewRaw{
			{
				ST13:              "EM018123456",
				TrademarkName:     "OPENAI",
				ApplicationNumber: "018123456",
				TrademarkStatus:   "REGISTERED",
				Office:            "EM",
				OfficeName:        "EUIPO",
				MarkFeature:       "WORD",
			},
		},
	}
	body, _ := json.Marshal(raw)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	defer srv.Close()

	c := &Client{http: &http.Client{}, baseURL: srv.URL}
	result, err := c.Search(context.Background(), SearchQuery{Query: "OPENAI", MaxResults: 10})
	if err != nil {
		t.Fatalf("Search error: %v", err)
	}
	if result.Total != 1 {
		t.Errorf("total want 1, got %d", result.Total)
	}
	if len(result.Results) != 1 {
		t.Fatalf("want 1 result, got %d", len(result.Results))
	}
	if result.Results[0].Name != "OPENAI" {
		t.Errorf("name want OPENAI, got %s", result.Results[0].Name)
	}
}

func TestGetByID_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := &Client{http: &http.Client{}, baseURL: srv.URL}
	result, err := c.GetByID(context.Background(), "EM", "000000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Error("expected nil result for 404")
	}
}

func TestGetByID_EmptyInputs(t *testing.T) {
	c := New()
	_, err := c.GetByID(context.Background(), "", "123")
	if err == nil {
		t.Error("expected error for empty officeCode")
	}
	_, err = c.GetByID(context.Background(), "EM", "")
	if err == nil {
		t.Error("expected error for empty appNum")
	}
}
