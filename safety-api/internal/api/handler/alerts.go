package handler

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"github.com/safety-api/internal/safety"
)

type AlertsHandler struct {
	index *safety.Index
	log   zerolog.Logger
}

func NewAlertsHandler(idx *safety.Index, log zerolog.Logger) *AlertsHandler {
	return &AlertsHandler{index: idx, log: log}
}

// Search handles GET /v1/recalls/search
//
// Query params:
//   product    — product name or keyword (e.g. "fidget spinner", "charger")
//   brand      — brand name (e.g. "Samsung", "unknown")
//   category   — product category (e.g. "Toys", "Electrical appliances")
//   country    — 2-letter notifying country code (DE, FR, IT…)
//   origin     — country of manufacture (CN, TR…)
//   risk       — risk type keyword (Chemical, Electrical, Injury…)
//   from       — date filter YYYY-MM-DD
//   to         — date filter YYYY-MM-DD
//   maxResults — 1–500 (default 50)
func (h *AlertsHandler) Search(c *fiber.Ctx) error {
	maxResults := 50
	if mr := c.Query("maxResults"); mr != "" {
		n, err := strconv.Atoi(mr)
		if err != nil || n < 1 || n > 500 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "maxResults must be 1–500",
			})
		}
		maxResults = n
	}

	q := safety.SearchQuery{
		Product:    strings.TrimSpace(c.Query("product")),
		Brand:      strings.TrimSpace(c.Query("brand")),
		Category:   strings.TrimSpace(c.Query("category")),
		Country:    strings.ToUpper(strings.TrimSpace(c.Query("country"))),
		Origin:     strings.TrimSpace(c.Query("origin")),
		Risk:       strings.TrimSpace(c.Query("risk")),
		From:       strings.TrimSpace(c.Query("from")),
		To:         strings.TrimSpace(c.Query("to")),
		MaxResults: maxResults,
	}

	// Require at least one filter
	if q.Product == "" && q.Brand == "" && q.Category == "" && q.Country == "" && q.Origin == "" && q.Risk == "" && q.From == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "at least one filter required: product, brand, category, country, origin, risk, or from",
			"hint":  "example: /v1/recalls/search?product=toy&country=DE",
		})
	}

	results := h.index.Search(q)
	return c.JSON(fiber.Map{
		"total":    len(results),
		"results":  results,
		"query":    q,
		"dataDate": h.index.DataDate(),
	})
}

// Get handles GET /v1/recalls/:reference — fetch a single alert by reference number.
func (h *AlertsHandler) Get(c *fiber.Ctx) error {
	ref := strings.TrimSpace(c.Params("reference"))
	if ref == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "reference required"})
	}
	alert, ok := h.index.Get(ref)
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "alert not found",
			"reference": ref,
		})
	}
	return c.JSON(alert)
}

// Categories handles GET /v1/recalls/categories — list all known product categories.
func (h *AlertsHandler) Categories(c *fiber.Ctx) error {
	cats := h.index.Categories()
	return c.JSON(fiber.Map{
		"categories": cats,
		"total":      len(cats),
	})
}

// Status handles GET /v1/recalls/status — data health check.
func (h *AlertsHandler) Status(c *fiber.Ctx) error {
	s := h.index.Status()
	return c.JSON(fiber.Map{
		"loaded":      s.Loaded,
		"alertCount":  s.AlertCount,
		"dataDate":    s.DataDate,
		"nextRefresh": s.NextRefresh,
		"checkedAt":   time.Now().UTC().Format(time.RFC3339),
	})
}
