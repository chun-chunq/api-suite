package handler

import (
	"strconv"

	"gdpr-api/internal/client"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

type GDPRHandler struct {
	client *client.Client
	log    zerolog.Logger
}

func NewGDPRHandler(c *client.Client, log zerolog.Logger) *GDPRHandler {
	return &GDPRHandler{client: c, log: log}
}

// Search handles GET /v1/gdpr/fines
func (h *GDPRHandler) Search(c *fiber.Ctx) error {
	q := client.SearchQuery{
		Country:   c.Query("country"),
		Authority: c.Query("authority"),
		Entity:    c.Query("entity"),
		Article:   c.Query("article"),
		Sector:    c.Query("sector"),
	}

	if m := c.Query("minAmount"); m != "" {
		if n, err := strconv.ParseFloat(m, 64); err == nil {
			q.MinAmount = n
		}
	}
	if m := c.Query("maxAmount"); m != "" {
		if n, err := strconv.ParseFloat(m, 64); err == nil {
			q.MaxAmount = n
		}
	}
	if y := c.Query("yearFrom"); y != "" {
		if n, err := strconv.Atoi(y); err == nil {
			q.YearFrom = n
		}
	}
	if y := c.Query("yearTo"); y != "" {
		if n, err := strconv.Atoi(y); err == nil {
			q.YearTo = n
		}
	}
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 500 {
			q.MaxResults = n
		}
	}
	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			q.Offset = n
		}
	}

	result, err := h.client.Search(c.Context(), q)
	if err != nil {
		h.log.Error().Err(err).Msg("GDPR search failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "GDPR data fetch failed: " + err.Error()})
	}

	return c.JSON(fiber.Map{
		"total":      result.Total,
		"offset":     result.Offset,
		"count":      len(result.Results),
		"stats":      result.Stats,
		"results":    result.Results,
		"dataSource": "GDPR Enforcement Tracker (enforcementtracker.com)",
	})
}

// TopFines handles GET /v1/gdpr/top?country=DE&n=10
func (h *GDPRHandler) TopFines(c *fiber.Ctx) error {
	country := c.Query("country")
	n := 10
	if nStr := c.Query("n"); nStr != "" {
		if v, err := strconv.Atoi(nStr); err == nil && v > 0 && v <= 100 {
			n = v
		}
	}

	fines, err := h.client.GetTopFines(c.Context(), country, n)
	if err != nil {
		h.log.Error().Err(err).Msg("top fines failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "data fetch failed"})
	}

	return c.JSON(fiber.Map{
		"count":      len(fines),
		"country":    country,
		"results":    fines,
		"dataSource": "GDPR Enforcement Tracker",
	})
}

// CacheStatus handles GET /v1/gdpr/cache-status (public endpoint to check data freshness)
func (h *GDPRHandler) CacheStatus(c *fiber.Ctx) error {
	count, cachedAt := h.client.CacheInfo()
	return c.JSON(fiber.Map{
		"cachedRecords": count,
		"cachedAt":      cachedAt,
		"dataSource":    "GDPR Enforcement Tracker (enforcementtracker.com)",
	})
}
