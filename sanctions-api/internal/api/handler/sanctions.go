package handler

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"github.com/sanctions-api/internal/sanctions"
)

// SanctionsHandler serves the public sanctions endpoints.
type SanctionsHandler struct {
	index *sanctions.Index
	log   zerolog.Logger
}

func NewSanctionsHandler(idx *sanctions.Index, log zerolog.Logger) *SanctionsHandler {
	return &SanctionsHandler{index: idx, log: log}
}

// Search handles GET /v1/sanctions/search
//
// Query params:
//
//	q          — search term (name, alias, company)
//	type       — filter by subject type: person | entity | ship
//	maxResults — max results (default 50, max 200)
func (h *SanctionsHandler) Search(c *fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "parameter 'q' is required (name to search for)",
		})
	}

	maxResults := 50
	if mr := c.Query("maxResults"); mr != "" {
		n, err := strconv.Atoi(mr)
		if err != nil || n < 1 || n > 200 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "maxResults must be an integer between 1 and 200",
			})
		}
		maxResults = n
	}

	subjectTypeFilter := strings.ToLower(strings.TrimSpace(c.Query("type")))

	results := h.index.Search(q, maxResults*3) // fetch extra, then filter

	if subjectTypeFilter != "" {
		filtered := results[:0]
		for _, e := range results {
			if e.SubjectType == subjectTypeFilter {
				filtered = append(filtered, e)
			}
		}
		results = filtered
	}
	if len(results) > maxResults {
		results = results[:maxResults]
	}

	return c.JSON(fiber.Map{
		"total":    len(results),
		"results":  results,
		"query":    q,
		"listDate": h.index.ListDate(),
	})
}

// Check handles GET /v1/sanctions/check — quick yes/no for compliance automation.
//
// Query params:
//
//	name — name to check (person or company)
//
// Returns {"sanctioned": true/false, "matches": [...], "query": "...", "checkedAt": "...", "listDate": "..."}
func (h *SanctionsHandler) Check(c *fiber.Ctx) error {
	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "parameter 'name' is required",
		})
	}

	matches := h.index.Check(name)
	return c.JSON(fiber.Map{
		"sanctioned": len(matches) > 0,
		"matches":    matches,
		"query":      name,
		"checkedAt":  time.Now().UTC().Format(time.RFC3339),
		"listDate":   h.index.ListDate(),
	})
}

// Status handles GET /v1/sanctions/status — list health info.
func (h *SanctionsHandler) Status(c *fiber.Ctx) error {
	s := h.index.Status()
	return c.JSON(s)
}
