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

func TestGetByBarcode_Empty(t *testing.T) {
	c := New()
	_, err := c.GetByBarcode(nil, "")
	if err == nil {
		t.Error("expected error for empty barcode")
	}
}

func TestSearch_NoParams(t *testing.T) {
	c := New()
	_, err := c.Search(nil, SearchQuery{})
	if err == nil {
		t.Error("expected error for empty search query")
	}
}

func TestMapProduct_Basic(t *testing.T) {
	r := offProduct{
		Code:            "3017620422003",
		ProductName:     "Nutella",
		Brands:          "Ferrero",
		NutriscoreGrade: "e",
		NovaGroup:       4,
		EcoscoreGrade:   "c",
	}
	r.Nutriments.EnergyKcal100g = 539
	r.Nutriments.Proteins100g = 6.3
	r.Nutriments.Fat100g = 31.6

	p := mapProduct(r)
	if p.Barcode != "3017620422003" {
		t.Errorf("Barcode want 3017620422003, got %s", p.Barcode)
	}
	if p.Name != "Nutella" {
		t.Errorf("Name want Nutella, got %s", p.Name)
	}
	if p.NutriScore != "E" {
		t.Errorf("NutriScore want E (uppercase), got %s", p.NutriScore)
	}
	if p.NovaGroup != 4 {
		t.Errorf("NovaGroup want 4, got %d", p.NovaGroup)
	}
	if p.Nutrition == nil {
		t.Fatal("Nutrition should not be nil")
	}
	if p.Nutrition.Energy != 539 {
		t.Errorf("Energy want 539, got %f", p.Nutrition.Energy)
	}
	if !strings.Contains(p.URL, "3017620422003") {
		t.Errorf("URL should contain barcode, got %s", p.URL)
	}
}

func TestMapProduct_NoNutrition(t *testing.T) {
	r := offProduct{Code: "123", ProductName: "Test"}
	p := mapProduct(r)
	if p.Nutrition != nil {
		t.Error("Nutrition should be nil when no values")
	}
}

func TestMapProduct_TagCleaning(t *testing.T) {
	r := offProduct{
		Code:          "123",
		AllergensTags: []string{"en:gluten", "en:nuts", "fr:lait"},
		LabelsTags:    []string{"en:organic", "en:fair-trade"},
	}
	p := mapProduct(r)
	for _, a := range p.Allergens {
		if strings.Contains(a, "en:") || strings.Contains(a, "fr:") {
			t.Errorf("Allergen tag should have prefix stripped, got %s", a)
		}
	}
	for _, l := range p.Labels {
		if strings.Contains(l, "en:") {
			t.Errorf("Label tag should have prefix stripped, got %s", l)
		}
	}
}

func TestMapProduct_IngredientsLong(t *testing.T) {
	r := offProduct{
		Code:            "123",
		IngredientsText: strings.Repeat("sugar, ", 200),
	}
	r.Nutriments.EnergyKcal100g = 1 // ensure nutrition is set
	p := mapProduct(r)
	if len(p.Ingredients) > 500 {
		t.Errorf("Ingredients should be truncated to <=500 chars, got %d", len(p.Ingredients))
	}
	if !strings.HasSuffix(p.Ingredients, "...") {
		t.Error("truncated Ingredients should end with ...")
	}
}

func TestMapProduct_CategoriesLimit(t *testing.T) {
	tags := []string{"en:foods", "en:beverages", "en:dairy", "en:cheese", "en:soft-cheese", "en:aged-cheese", "en:extra"}
	r := offProduct{Code: "123", CategoriesTags: tags}
	p := mapProduct(r)
	if len(p.Categories) > 5 {
		t.Errorf("Categories should be limited to 5, got %d", len(p.Categories))
	}
}

func TestSearch_DefaultPagination(t *testing.T) {
	q := SearchQuery{Query: "test", MaxResults: 0, Page: 0}
	if q.MaxResults <= 0 || q.MaxResults > 100 {
		q.MaxResults = 24
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	if q.MaxResults != 24 {
		t.Errorf("default MaxResults want 24, got %d", q.MaxResults)
	}
	if q.Page != 1 {
		t.Errorf("default Page want 1, got %d", q.Page)
	}
}
