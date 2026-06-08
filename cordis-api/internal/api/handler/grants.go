package handler

import (
	"strings"

	"github.com/cordis-api/internal/client"
	"github.com/gofiber/fiber/v2"
)

// GrantsHandler handles CORDIS project/grant requests.
type GrantsHandler struct {
	client *client.Client
}

// NewGrantsHandler creates a new grants handler.
func NewGrantsHandler(c *client.Client) *GrantsHandler {
	return &GrantsHandler{client: c}
}

// Search handles GET /v1/grants/search
// Query params:
//
//	q         — keywords (title, objective, acronym)
//	country   — ISO-2 country of coordinator (DE, FR, IT…)
//	programme — HORIZON | H2020 | FP7
//	from      — start year >= (e.g. 2020)
//	to        — start year <= (e.g. 2024)
//	status    — ACTIVE | CLOSED
//	limit     — 1-100, default 25
//	page      — page number, default 1
func (h *GrantsHandler) Search(c *fiber.Ctx) error {
	q := client.SearchQuery{
		Keywords:   strings.TrimSpace(c.Query("q")),
		Country:    strings.ToUpper(strings.TrimSpace(c.Query("country"))),
		Programme:  strings.ToUpper(strings.TrimSpace(c.Query("programme"))),
		Status:     strings.ToUpper(strings.TrimSpace(c.Query("status"))),
		FromYear:   c.QueryInt("from", 0),
		ToYear:     c.QueryInt("to", 0),
		MaxResults: c.QueryInt("limit", 25),
		Page:       c.QueryInt("page", 1),
	}

	if q.Keywords == "" && q.Country == "" && q.Programme == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "at least one filter required: q, country, or programme",
		})
	}

	result, err := h.client.Search(c.Context(), q)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "CORDIS API error",
			"details": err.Error(),
		})
	}
	if result == nil || result.Results == nil {
		result = &client.SearchResult{Results: []client.Project{}}
	}

	return c.JSON(fiber.Map{
		"total":      result.Total,
		"page":       result.Page,
		"perPage":    result.PerPage,
		"count":      len(result.Results),
		"results":    result.Results,
		"dataSource": "EU CORDIS — Horizon Europe / H2020 / FP7",
	})
}

// GetProject handles GET /v1/grants/:id
func (h *GrantsHandler) GetProject(c *fiber.Ctx) error {
	id := strings.TrimSpace(c.Params("id"))
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "project ID is required",
		})
	}

	project, err := h.client.GetProject(c.Context(), id)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "CORDIS API error",
			"details": err.Error(),
		})
	}
	if project == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "project not found",
			"id":    id,
		})
	}

	return c.JSON(fiber.Map{
		"project":    project,
		"dataSource": "EU CORDIS",
	})
}
