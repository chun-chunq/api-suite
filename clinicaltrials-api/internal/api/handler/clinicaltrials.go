package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"clinicaltrials-api/internal/client"
)

// SearchStudies handles GET /v1/trials/search?q=cancer&status=RECRUITING&phase=PHASE3&limit=10&page_token=...
func SearchStudies(c *fiber.Ctx, cl *client.Client) error {
	query := strings.TrimSpace(c.Query("q", ""))
	status := strings.TrimSpace(c.Query("status", ""))
	phase := strings.TrimSpace(c.Query("phase", ""))
	limitStr := c.Query("limit", "10")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}
	nextToken := strings.TrimSpace(c.Query("page_token", ""))

	result, err := cl.Search(c.Context(), query, status, phase, limit, nextToken)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "upstream error: " + err.Error(),
		})
	}
	return c.JSON(result)
}

// GetStudy handles GET /v1/trials/:nct_id
func GetStudy(c *fiber.Ctx, cl *client.Client) error {
	nctID := strings.TrimSpace(c.Params("nct_id"))
	if nctID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "NCT ID is required in path (/v1/trials/{NCT_ID})",
		})
	}

	study, err := cl.GetStudy(c.Context(), nctID)
	if err != nil {
		if strings.Contains(err.Error(), "not_found") {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "upstream error: " + err.Error(),
		})
	}
	return c.JSON(study)
}
