package handler

import (
	"strconv"

	"food-api/internal/client"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

type FoodHandler struct {
	client *client.Client
	log    zerolog.Logger
}

func NewFoodHandler(c *client.Client, log zerolog.Logger) *FoodHandler {
	return &FoodHandler{client: c, log: log}
}

// GetByBarcode handles GET /v1/food/product/:barcode
func (h *FoodHandler) GetByBarcode(c *fiber.Ctx) error {
	barcode := c.Params("barcode")
	p, err := h.client.GetByBarcode(c.Context(), barcode)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if p == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "product not found for barcode " + barcode})
	}
	return c.JSON(p)
}

// Search handles GET /v1/food/search
//
//	?q=          product name search
//	&brands=     brand name filter
//	&categories= category filter
//	&countries=  country e.g. "france"
//	&nutriScore= A|B|C|D|E
//	&limit=      1–100 (default 24)
//	&page=       page number
func (h *FoodHandler) Search(c *fiber.Ctx) error {
	q := client.SearchQuery{
		Query:      c.Query("q"),
		Brands:     c.Query("brands"),
		Categories: c.Query("categories"),
		Countries:  c.Query("countries"),
		NutriScore: c.Query("nutriScore"),
	}

	if q.Query == "" && q.Brands == "" && q.Categories == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "provide at least one of: q (product name), brands, or categories",
		})
	}

	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			q.MaxResults = n
		}
	}
	if p := c.Query("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			q.Page = n
		}
	}

	result, err := h.client.Search(c.Context(), q)
	if err != nil {
		h.log.Error().Err(err).Str("q", q.Query).Msg("food search failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "upstream Open Food Facts search failed"})
	}

	return c.JSON(fiber.Map{
		"total":      result.Total,
		"page":       result.Page,
		"pageSize":   result.PageSize,
		"count":      len(result.Results),
		"results":    result.Results,
		"dataSource": "Open Food Facts (openfoodfacts.org)",
	})
}
