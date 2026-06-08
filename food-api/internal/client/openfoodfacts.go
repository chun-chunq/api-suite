// Package client wraps the Open Food Facts REST API.
// Open Food Facts is a free, open database of food products worldwide.
// Docs: https://wiki.openfoodfacts.org/API
// License: Open Database License (ODbL) — freely reusable
// No auth required.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://world.openfoodfacts.org"

// Product represents a food product from Open Food Facts.
type Product struct {
	Barcode       string            `json:"barcode"`        // EAN-13, UPC, etc.
	Name          string            `json:"name"`
	Brands        string            `json:"brands,omitempty"`
	Categories    []string          `json:"categories,omitempty"`
	Countries     []string          `json:"countries,omitempty"`
	Quantity      string            `json:"quantity,omitempty"` // e.g. "500g", "1L"
	Ingredients   string            `json:"ingredients,omitempty"`
	Allergens     []string          `json:"allergens,omitempty"`
	Labels        []string          `json:"labels,omitempty"` // organic, fair-trade, etc.
	NutriScore    string            `json:"nutriScore,omitempty"` // A-E
	NovaGroup     int               `json:"novaGroup,omitempty"`  // 1-4 processing level
	EcoScore      string            `json:"ecoScore,omitempty"`   // A-E environmental impact
	Nutrition     *Nutrition        `json:"nutrition,omitempty"`
	ImageURL      string            `json:"imageUrl,omitempty"`
	URL           string            `json:"url"`
}

// Nutrition holds per-100g nutritional values.
type Nutrition struct {
	Energy          float64 `json:"energyKcal,omitempty"`
	Fat             float64 `json:"fatG,omitempty"`
	SaturatedFat    float64 `json:"saturatedFatG,omitempty"`
	Carbohydrates   float64 `json:"carbohydratesG,omitempty"`
	Sugars          float64 `json:"sugarsG,omitempty"`
	Fiber           float64 `json:"fiberG,omitempty"`
	Protein         float64 `json:"proteinG,omitempty"`
	Salt            float64 `json:"saltG,omitempty"`
	Sodium          float64 `json:"sodiumG,omitempty"`
}

// SearchResult is a paginated search response.
type SearchResult struct {
	Total      int       `json:"total"`
	Page       int       `json:"page"`
	PageSize   int       `json:"pageSize"`
	Results    []Product `json:"results"`
}

// SearchQuery holds search parameters.
type SearchQuery struct {
	Query      string   // product name search
	Brands     string   // brand name filter
	Categories string   // category filter
	Countries  string   // country code e.g. "france", "united-states"
	NutriScore string   // A, B, C, D, or E
	Labels     string   // e.g. "en:organic"
	MaxResults int      // 1–100
	Page       int
}

// Client wraps the Open Food Facts API.
type Client struct {
	http    *http.Client
	baseURL string
}

// New creates a new Open Food Facts client.
func New() *Client {
	return &Client{
		http:    &http.Client{Timeout: 20 * time.Second},
		baseURL: defaultBaseURL,
	}
}

// GetByBarcode fetches a product by its barcode (EAN-13, UPC, etc.)
func (c *Client) GetByBarcode(ctx context.Context, barcode string) (*Product, error) {
	barcode = strings.TrimSpace(barcode)
	if barcode == "" {
		return nil, fmt.Errorf("barcode is required")
	}

	u := fmt.Sprintf("%s/api/v2/product/%s.json", c.baseURL, url.PathEscape(barcode))
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}

	var raw offProductResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode product: %w", err)
	}
	if raw.Status == 0 {
		return nil, nil // product not found
	}

	p := mapProduct(raw.Product)
	return &p, nil
}

// Search searches for products by name, brand, or category.
func (c *Client) Search(ctx context.Context, q SearchQuery) (*SearchResult, error) {
	if q.Query == "" && q.Brands == "" && q.Categories == "" {
		return nil, fmt.Errorf("provide at least one of: query, brands, or categories")
	}
	if q.MaxResults <= 0 || q.MaxResults > 100 {
		q.MaxResults = 24
	}
	if q.Page <= 0 {
		q.Page = 1
	}

	params := url.Values{}
	if q.Query != "" {
		params.Set("search_terms", q.Query)
	}
	if q.Brands != "" {
		params.Set("tagtype_0", "brands")
		params.Set("tag_contains_0", "contains")
		params.Set("tag_0", q.Brands)
	}
	if q.Categories != "" {
		params.Set("tagtype_0", "categories")
		params.Set("tag_contains_0", "contains")
		params.Set("tag_0", q.Categories)
	}
	if q.Countries != "" {
		params.Set("tagtype_1", "countries")
		params.Set("tag_contains_1", "contains")
		params.Set("tag_1", q.Countries)
	}
	if q.NutriScore != "" {
		params.Set("tagtype_1", "nutrition_grades")
		params.Set("tag_contains_1", "contains")
		params.Set("tag_1", strings.ToLower(q.NutriScore))
	}
	params.Set("page_size", fmt.Sprintf("%d", q.MaxResults))
	params.Set("page", fmt.Sprintf("%d", q.Page))
	params.Set("json", "1")
	params.Set("fields", "code,product_name,brands,categories_tags,countries_tags,quantity,ingredients_text,allergens_tags,labels_tags,nutriscore_grade,nova_group,ecoscore_grade,nutriments,image_front_url")

	u := fmt.Sprintf("%s/cgi/search.pl?%s", c.baseURL, params.Encode())
	body, err := c.get(ctx, u)
	if err != nil {
		return nil, err
	}

	var raw offSearchResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode search: %w", err)
	}

	products := make([]Product, 0, len(raw.Products))
	for _, r := range raw.Products {
		products = append(products, mapProduct(r))
	}

	return &SearchResult{
		Total:    raw.Count,
		Page:     q.Page,
		PageSize: q.MaxResults,
		Results:  products,
	}, nil
}

// ── Raw OFF JSON structures ────────────────────────────────────────────────────

type offProductResponse struct {
	Status  int        `json:"status"`
	Product offProduct `json:"product"`
}

type offSearchResponse struct {
	Count    int          `json:"count"`
	Page     int          `json:"page"`
	PageSize int          `json:"page_size"`
	Products []offProduct `json:"products"`
}

type offProduct struct {
	Code            string  `json:"code"`
	ProductName     string  `json:"product_name"`
	Brands          string  `json:"brands"`
	CategoriesTags  []string `json:"categories_tags"`
	CountriesTags   []string `json:"countries_tags"`
	Quantity        string  `json:"quantity"`
	IngredientsText string  `json:"ingredients_text"`
	AllergensTags   []string `json:"allergens_tags"`
	LabelsTags      []string `json:"labels_tags"`
	NutriscoreGrade string  `json:"nutriscore_grade"`
	NovaGroup       int     `json:"nova_group"`
	EcoscoreGrade   string  `json:"ecoscore_grade"`
	ImageFrontURL   string  `json:"image_front_url"`
	Nutriments      struct {
		EnergyKcal100g     float64 `json:"energy-kcal_100g"`
		Fat100g            float64 `json:"fat_100g"`
		SaturatedFat100g   float64 `json:"saturated-fat_100g"`
		Carbohydrates100g  float64 `json:"carbohydrates_100g"`
		Sugars100g         float64 `json:"sugars_100g"`
		Fiber100g          float64 `json:"fiber_100g"`
		Proteins100g       float64 `json:"proteins_100g"`
		Salt100g           float64 `json:"salt_100g"`
		Sodium100g         float64 `json:"sodium_100g"`
	} `json:"nutriments"`
}

func mapProduct(r offProduct) Product {
	// Clean tag lists (remove language prefix "en:", "fr:", etc.)
	cleanTags := func(tags []string) []string {
		out := make([]string, 0, len(tags))
		for _, t := range tags {
			if idx := strings.Index(t, ":"); idx >= 0 && idx < 4 {
				t = t[idx+1:]
			}
			t = strings.ReplaceAll(t, "-", " ")
			if t != "" {
				out = append(out, t)
			}
		}
		return out
	}

	p := Product{
		Barcode:     r.Code,
		Name:        r.ProductName,
		Brands:      r.Brands,
		Categories:  cleanTags(r.CategoriesTags),
		Countries:   cleanTags(r.CountriesTags),
		Quantity:    r.Quantity,
		Ingredients: r.IngredientsText,
		Allergens:   cleanTags(r.AllergensTags),
		Labels:      cleanTags(r.LabelsTags),
		NutriScore:  strings.ToUpper(r.NutriscoreGrade),
		NovaGroup:   r.NovaGroup,
		EcoScore:    strings.ToUpper(r.EcoscoreGrade),
		ImageURL:    r.ImageFrontURL,
		URL:         fmt.Sprintf("https://world.openfoodfacts.org/product/%s", r.Code),
	}

	// Nutrition
	n := r.Nutriments
	if n.EnergyKcal100g > 0 || n.Proteins100g > 0 || n.Carbohydrates100g > 0 {
		p.Nutrition = &Nutrition{
			Energy:        n.EnergyKcal100g,
			Fat:           n.Fat100g,
			SaturatedFat:  n.SaturatedFat100g,
			Carbohydrates: n.Carbohydrates100g,
			Sugars:        n.Sugars100g,
			Fiber:         n.Fiber100g,
			Protein:       n.Proteins100g,
			Salt:          n.Salt100g,
			Sodium:        n.Sodium100g,
		}
	}

	// Trim ingredients
	if len(p.Ingredients) > 500 {
		p.Ingredients = p.Ingredients[:497] + "..."
	}

	// Trim categories to top 5
	if len(p.Categories) > 5 {
		p.Categories = p.Categories[:5]
	}

	return p
}

func (c *Client) get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "FoodAPI/1.0")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Open Food Facts request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Open Food Facts HTTP %d", resp.StatusCode)
	}
	return body, nil
}
