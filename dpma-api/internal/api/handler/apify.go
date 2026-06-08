// Apify Actor compatibility endpoint.
// Apify Actors read JSON input and write to a key-value store.
// This endpoint makes the API compatible with the Apify platform.
//
// Usage on Apify: POST /apify/run with Apify input JSON.
// The response mimics Apify Actor output format.
package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"github.com/dpma-api/internal/scraper"
)

// ApifyHandler wraps the trademark search in Apify Actor format.
type ApifyHandler struct {
	trademark *TrademarkHandler
	log       zerolog.Logger
}

func NewApifyHandler(tm *TrademarkHandler, log zerolog.Logger) *ApifyHandler {
	return &ApifyHandler{trademark: tm, log: log}
}

// Run handles POST /apify/run
// Input schema matches the Apify Actor input format.
//
// Input JSON:
//
//	{
//	  "name": "Apple",
//	  "class": "9,35",
//	  "owner": "",
//	  "status": "registered",
//	  "maxResults": 50
//	}
func (h *ApifyHandler) Run(c *fiber.Ctx) error {
	var input map[string]any
	if err := c.BodyParser(&input); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"status":  "FAILED",
			"message": "invalid input JSON",
		})
	}

	q := scraper.SearchQuery{MaxResults: 50}

	if v, ok := input["name"].(string); ok {
		q.Name = v
	}
	if v, ok := input["owner"].(string); ok {
		q.Owner = v
	}
	if v, ok := input["registrationNumber"].(string); ok {
		q.RegistrationNumber = v
	}
	if v, ok := input["status"].(string); ok {
		q.Status = v
	}
	if v, ok := input["markType"].(string); ok {
		q.MarkType = v
	}
	if v, ok := input["maxResults"].(float64); ok {
		q.MaxResults = int(v)
	}
	if v, ok := input["class"].(string); ok {
		for _, part := range strings.Split(v, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && n >= 1 && n <= 45 {
				q.Classes = append(q.Classes, n)
			}
		}
	}

	result, _, err := h.trademark.runSearch(c.Context(), q)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"status":  "FAILED",
			"message": err.Error(),
		})
	}

	// Apify-compatible output format
	return c.JSON(fiber.Map{
		"status":  "SUCCEEDED",
		"output":  result,
		"itemCount": len(result.Results),
	})
}
