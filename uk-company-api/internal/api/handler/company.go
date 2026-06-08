package handler

import (
	"strconv"

	"uk-company-api/internal/client"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

type CompanyHandler struct {
	client *client.Client
	log    zerolog.Logger
}

func NewCompanyHandler(c *client.Client, log zerolog.Logger) *CompanyHandler {
	return &CompanyHandler{client: c, log: log}
}

// Search handles GET /v1/uk/company/search?q=&limit=&offset=
func (h *CompanyHandler) Search(c *fiber.Ctx) error {
	q := c.Query("q")
	if q == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "q parameter is required"})
	}

	limit := 20
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	offset := 0
	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	result, err := h.client.Search(c.Context(), q, limit, offset)
	if err != nil {
		h.log.Error().Err(err).Str("q", q).Msg("Companies House search failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "upstream Companies House search failed"})
	}

	return c.JSON(fiber.Map{
		"total":        result.Total,
		"startIndex":   result.StartIndex,
		"itemsPerPage": result.ItemsPerPage,
		"count":        len(result.Results),
		"results":      result.Results,
		"dataSource":   "Companies House (UK)",
	})
}

// GetByNumber handles GET /v1/uk/company/:number
func (h *CompanyHandler) GetByNumber(c *fiber.Ctx) error {
	number := c.Params("number")
	co, err := h.client.GetByNumber(c.Context(), number)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if co == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "company not found"})
	}
	return c.JSON(co)
}

// GetOfficers handles GET /v1/uk/company/:number/officers?activeOnly=true
func (h *CompanyHandler) GetOfficers(c *fiber.Ctx) error {
	number := c.Params("number")
	activeOnly := c.Query("activeOnly") == "true"

	officers, err := h.client.GetOfficers(c.Context(), number, activeOnly)
	if err != nil {
		h.log.Error().Err(err).Str("number", number).Msg("officers lookup failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "upstream officers lookup failed"})
	}
	if officers == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "company not found"})
	}
	return c.JSON(fiber.Map{
		"companyNumber": number,
		"count":         len(officers),
		"officers":      officers,
	})
}
