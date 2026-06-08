package handler

import (
	"strconv"

	"research-api/internal/client"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

type ResearchHandler struct {
	client *client.Client
	log    zerolog.Logger
}

func NewResearchHandler(c *client.Client, log zerolog.Logger) *ResearchHandler {
	return &ResearchHandler{client: c, log: log}
}

// SearchWorks handles GET /v1/research/works/search
//
//	?q=            free-text search
//	&author=       author name filter
//	&year=         exact publication year
//	&yearFrom=     min year
//	&yearTo=       max year
//	&openAccess=   true|false
//	&type=         journal-article | book | dataset | ...
//	&sort=         cited_by_count | publication_date
//	&limit=        1–200 (default 25)
//	&page=         page number (default 1)
func (h *ResearchHandler) SearchWorks(c *fiber.Ctx) error {
	q := client.WorkSearchQuery{
		Query:          c.Query("q"),
		Author:         c.Query("author"),
		ConceptID:      c.Query("conceptId"),
		Type:           c.Query("type"),
		SortBy:         c.Query("sort"),
		OpenAccessOnly: c.Query("openAccess") == "true",
	}

	if q.Query == "" && q.Author == "" && q.ConceptID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "provide at least one of: q (search text), author, or conceptId",
		})
	}

	if y := c.Query("year"); y != "" {
		if n, err := strconv.Atoi(y); err == nil {
			q.Year = n
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
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			q.MaxResults = n
		}
	}
	if p := c.Query("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			q.Page = n
		}
	}

	result, err := h.client.SearchWorks(c.Context(), q)
	if err != nil {
		h.log.Error().Err(err).Str("q", q.Query).Msg("OpenAlex search failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "upstream OpenAlex search failed"})
	}

	return c.JSON(fiber.Map{
		"total":      result.Total,
		"page":       result.Page,
		"perPage":    result.PerPage,
		"count":      len(result.Results),
		"results":    result.Results,
		"dataSource": "OpenAlex (open.alex.org)",
	})
}

// GetWorkByDOI handles GET /v1/research/works/doi?doi=10.1234/example
func (h *ResearchHandler) GetWorkByDOI(c *fiber.Ctx) error {
	doi := c.Query("doi")
	if doi == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "doi query parameter is required"})
	}

	work, err := h.client.GetWorkByDOI(c.Context(), doi)
	if err != nil {
		h.log.Error().Err(err).Str("doi", doi).Msg("DOI lookup failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "DOI lookup failed"})
	}
	if work == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "work not found for this DOI"})
	}
	return c.JSON(work)
}

// SearchInstitutions handles GET /v1/research/institutions/search?q=MIT&limit=10
func (h *ResearchHandler) SearchInstitutions(c *fiber.Ctx) error {
	q := c.Query("q")
	if q == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "q parameter is required"})
	}
	limit := 10
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	institutions, total, err := h.client.SearchInstitutions(c.Context(), q, limit)
	if err != nil {
		h.log.Error().Err(err).Str("q", q).Msg("institution search failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "upstream institution search failed"})
	}

	return c.JSON(fiber.Map{
		"total":      total,
		"count":      len(institutions),
		"results":    institutions,
		"dataSource": "OpenAlex (open.alex.org)",
	})
}
