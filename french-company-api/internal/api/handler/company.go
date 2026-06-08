package handler

import (
	"strconv"

	"french-company-api/internal/client"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

// CompanyHandler handles company search/lookup endpoints.
type CompanyHandler struct {
	client *client.Client
	log    zerolog.Logger
}

// NewCompanyHandler creates a new CompanyHandler.
func NewCompanyHandler(c *client.Client, log zerolog.Logger) *CompanyHandler {
	return &CompanyHandler{client: c, log: log}
}

// Search handles GET /v1/fr/company/search
//
//	?q=           company name, trade name, or SIREN
//	&postalCode=  5-digit French postal code
//	&department=  2-digit department code e.g. "75"
//	&activity=    NAF/APE code e.g. "62.01Z"
//	&legalForm=   INSEE nature juridique code e.g. "5710"
//	&activeOnly=  true|false (default: false)
//	&limit=       1–25 (default: 25)
//	&page=        page number (default: 1)
func (h *CompanyHandler) Search(c *fiber.Ctx) error {
	q := client.SearchQuery{
		Query:        c.Query("q"),
		PostalCode:   c.Query("postalCode"),
		Department:   c.Query("department"),
		ActivityCode: c.Query("activity"),
		LegalForm:    c.Query("legalForm"),
		ActiveOnly:   c.Query("activeOnly") == "true",
	}

	if q.Query == "" && q.PostalCode == "" && q.Department == "" && q.ActivityCode == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "provide at least one of: q, postalCode, department, activity",
		})
	}

	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 25 {
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
		h.log.Error().Err(err).Str("q", q.Query).Msg("SIRENE search failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "upstream SIRENE search failed",
		})
	}

	return c.JSON(fiber.Map{
		"total":      result.Total,
		"page":       result.Page,
		"perPage":    result.PerPage,
		"totalPages": result.TotalPages,
		"count":      len(result.Results),
		"results":    result.Results,
		"dataSource": "SIRENE / data.gouv.fr",
	})
}

// GetBySIREN handles GET /v1/fr/company/:siren
func (h *CompanyHandler) GetBySIREN(c *fiber.Ctx) error {
	siren := c.Params("siren")

	co, err := h.client.GetBySIREN(c.Context(), siren)
	if err != nil {
		h.log.Error().Err(err).Str("siren", siren).Msg("SIRENE lookup failed")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	if co == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "company not found",
		})
	}

	return c.JSON(co)
}
