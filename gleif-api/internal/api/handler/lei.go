package handler

import (
	"strings"

	"github.com/gleif-api/internal/client"
	"github.com/gofiber/fiber/v2"
)

// LEIHandler handles all LEI-related requests.
type LEIHandler struct {
	client *client.Client
}

// NewLEIHandler creates a new LEI handler.
func NewLEIHandler(c *client.Client) *LEIHandler {
	return &LEIHandler{client: c}
}

// Search handles GET /v1/lei/search
// Query params:
//
//	name     (required) — company name to search
//	country  (optional) — ISO-2 country code filter (e.g. DE, US, CH)
//	active   (optional) — "true" to return only active LEIs
//	limit    (optional) — max results, 1-200, default 50
func (h *LEIHandler) Search(c *fiber.Ctx) error {
	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "query parameter 'name' is required",
		})
	}

	country := strings.ToUpper(strings.TrimSpace(c.Query("country")))
	active := c.QueryBool("active", false)
	limit := c.QueryInt("limit", 50)
	if limit < 1 {
		limit = 1
	}
	if limit > 200 {
		limit = 200
	}

	entities, total, err := h.client.SearchByName(c.Context(), name, country, active, limit)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "GLEIF API error",
			"details": err.Error(),
		})
	}
	if entities == nil {
		entities = []client.Entity{}
	}

	return c.JSON(fiber.Map{
		"total":      total,
		"count":      len(entities),
		"results":    entities,
		"query":      name,
		"country":    country,
		"activeOnly": active,
		"dataSource": "GLEIF Global LEI Index",
	})
}

// GetByLEI handles GET /v1/lei/:lei
// Returns full entity record for a given 20-character LEI code.
func (h *LEIHandler) GetByLEI(c *fiber.Ctx) error {
	lei := strings.ToUpper(strings.TrimSpace(c.Params("lei")))
	if len(lei) != 20 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "LEI must be exactly 20 characters",
			"hint":  "Example: 5299000J2N45DDNE4Y28",
		})
	}

	entity, err := h.client.GetByLEI(c.Context(), lei)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "GLEIF API error",
			"details": err.Error(),
		})
	}
	if entity == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "LEI not found",
			"lei":   lei,
		})
	}

	return c.JSON(fiber.Map{
		"lei":        entity,
		"dataSource": "GLEIF Global LEI Index",
	})
}

// GetRelationships handles GET /v1/lei/:lei/relationships
// Returns direct parent, ultimate parent, and direct children.
func (h *LEIHandler) GetRelationships(c *fiber.Ctx) error {
	lei := strings.ToUpper(strings.TrimSpace(c.Params("lei")))
	if len(lei) != 20 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "LEI must be exactly 20 characters",
		})
	}

	summary, err := h.client.GetRelationships(c.Context(), lei)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "GLEIF API error",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"lei":           lei,
		"relationships": summary,
		"dataSource":    "GLEIF Global LEI Index",
	})
}
