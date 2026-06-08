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

// mockVIESResponse returns a viesResponse as JSON.
func mockVIESResponse(isValid bool, name, address string) []byte {
	r := viesResponse{
		IsValid:     isValid,
		RequestDate: "2024-01-15+01:00",
		UserError:   "VALID",
		TraderName:  name,
		TraderAddr:  address,
		VATNumber:   "123456789",
	}
	if !isValid {
		r.UserError = "INVALID"
	}
	b, _ := json.Marshal(r)
	return b
}

func TestValidateVAT_Valid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockVIESResponse(true, "AMAZON EU SARL", "38 AVENUE JOHN F. KENNEDY\nL-1855 LUXEMBOURG"))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	res, err := c.ValidateVAT(context.Background(), "LU", "26375245")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Valid {
		t.Error("expected valid=true")
	}
	if res.Name != "AMAZON EU SARL" {
		t.Errorf("unexpected name: %q", res.Name)
	}
	if res.CountryCode != "LU" {
		t.Errorf("unexpected country: %q", res.CountryCode)
	}
	if res.FullVAT != "LU26375245" {
		t.Errorf("unexpected full_vat: %q", res.FullVAT)
	}
}

func TestValidateVAT_Invalid(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockVIESResponse(false, "", ""))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	res, err := c.ValidateVAT(context.Background(), "DE", "000000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Valid {
		t.Error("expected valid=false")
	}
}

func TestValidateVAT_StripPrefix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify that the URL contains just the number without prefix
		if r.URL.Path != "/ms/DE/vat/123456789" {
			http.Error(w, "wrong path: "+r.URL.Path, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockVIESResponse(true, "Test GmbH", "Berlin"))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	// Pass "DE123456789" — prefix should be stripped
	res, err := c.ValidateVAT(context.Background(), "DE", "DE123456789")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.VATNumber != "123456789" {
		t.Errorf("expected stripped vat_number, got %q", res.VATNumber)
	}
}

func TestValidateVAT_InvalidCountry(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused"}
	_, err := c.ValidateVAT(context.Background(), "US", "123456789")
	if err == nil {
		t.Fatal("expected error for unsupported country code")
	}
}

func TestValidateVAT_EmptyVAT(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused"}
	_, err := c.ValidateVAT(context.Background(), "DE", "")
	if err == nil {
		t.Fatal("expected error for empty VAT number")
	}
}

func TestValidateVAT_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"actionSucceed":false,"errorWrappers":[{"error":"INVALID_INPUT"}]}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.ValidateVAT(context.Background(), "DE", "bad!")
	if err == nil {
		t.Fatal("expected error from server 400")
	}
}

func TestValidateBatch_OK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(mockVIESResponse(true, "Test Corp", "Test Street"))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	br, err := c.ValidateBatch(context.Background(), []string{"DE123456789", "FR12345678901"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if br.Total != 2 {
		t.Errorf("expected total=2, got %d", br.Total)
	}
}

func TestValidateBatch_TooMany(t *testing.T) {
	c := &Client{http: http.DefaultClient, baseURL: "http://unused"}
	vats := make([]string, 11)
	for i := range vats {
		vats[i] = "DE123456789"
	}
	_, err := c.ValidateBatch(context.Background(), vats)
	if err == nil {
		t.Fatal("expected error for >10 VAT numbers")
	}
}

func TestSplitVAT_OK(t *testing.T) {
	cc, num, err := splitVAT("LU26375245")
	if err != nil {
		t.Fatal(err)
	}
	if cc != "LU" || num != "26375245" {
		t.Errorf("got cc=%q num=%q", cc, num)
	}
}

func TestSplitVAT_Unknown(t *testing.T) {
	_, _, err := splitVAT("XX123")
	if err == nil {
		t.Fatal("expected error for unknown country code")
	}
}

func TestAddressClean(t *testing.T) {
	// Address with newlines should be collapsed
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		resp := viesResponse{
			IsValid:    true,
			TraderName: "Test",
			TraderAddr: "Line 1\nLine 2\n   Line 3   ",
		}
		b, _ := json.Marshal(resp)
		w.Write(b)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	res, err := c.ValidateVAT(context.Background(), "DE", "123")
	if err != nil {
		t.Fatal(err)
	}
	// strings.Fields collapses all whitespace including \n
	if res.Address == "" {
		t.Error("address should not be empty")
	}
	for _, ch := range res.Address {
		if ch == '\n' {
			t.Error("address should not contain newlines after cleaning")
		}
	}
}
