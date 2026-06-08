// Package handler provides HTTP handlers for the NASA API.
package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"nasa-api/internal/client"
)

// APOD handles GET /v1/nasa/apod?date=YYYY-MM-DD
func APOD(c *fiber.Ctx, cl *client.Client) error {
	date := strings.TrimSpace(c.Query("date"))
	entry, err := cl.GetAPOD(c.Context(), date)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.JSON(entry)
}

// APODRange handles GET /v1/nasa/apod/range?start=YYYY-MM-DD&end=YYYY-MM-DD
func APODRange(c *fiber.Ctx, cl *client.Client) error {
	start := strings.TrimSpace(c.Query("start"))
	end := strings.TrimSpace(c.Query("end"))

	if start == "" || end == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "start and end query parameters are required (YYYY-MM-DD)",
		})
	}

	entries, err := cl.GetAPODRange(c.Context(), start, end)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"start_date": start,
		"end_date":   end,
		"count":      len(entries),
		"entries":    entries,
	})
}

// MarsPhotos handles GET /v1/nasa/mars/:rover/photos?sol=1000&camera=FHAZ&limit=10
func MarsPhotos(c *fiber.Ctx, cl *client.Client) error {
	rover := c.Params("rover", "curiosity")

	sol, _ := strconv.Atoi(c.Query("sol", "0"))
	earthDate := strings.TrimSpace(c.Query("earth_date"))
	camera := strings.TrimSpace(c.Query("camera"))
	limit, _ := strconv.Atoi(c.Query("limit", "10"))

	result, err := cl.GetMarsPhotos(c.Context(), rover, camera, sol, earthDate, limit)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(result)
}

// NEOFeed handles GET /v1/nasa/neo?start=YYYY-MM-DD&end=YYYY-MM-DD
func NEOFeed(c *fiber.Ctx, cl *client.Client) error {
	start := strings.TrimSpace(c.Query("start"))
	end := strings.TrimSpace(c.Query("end"))

	result, err := cl.GetNEOFeed(c.Context(), start, end)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(result)
}
