// Package handler provides HTTP handlers for the Air Quality API.
package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"airquality-api/internal/client"
)

// parseLatLon validates and parses latitude/longitude from query params.
func parseLatLon(c *fiber.Ctx) (float64, float64, error) {
	latStr := strings.TrimSpace(c.Query("lat"))
	lonStr := strings.TrimSpace(c.Query("lon"))
	if latStr == "" || lonStr == "" {
		return 0, 0, fiber.NewError(fiber.StatusBadRequest, "lat and lon query parameters are required")
	}
	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil || lat < -90 || lat > 90 {
		return 0, 0, fiber.NewError(fiber.StatusBadRequest, "lat must be between -90 and 90")
	}
	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil || lon < -180 || lon > 180 {
		return 0, 0, fiber.NewError(fiber.StatusBadRequest, "lon must be between -180 and 180")
	}
	return lat, lon, nil
}

// GetCurrent handles GET /v1/air/current?lat=52.52&lon=13.41&timezone=Europe/Berlin
func GetCurrent(c *fiber.Ctx, cl *client.Client) error {
	lat, lon, err := parseLatLon(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	tz := c.Query("timezone", "auto")

	result, err := cl.GetCurrent(c.Context(), lat, lon, tz)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}

// GetForecast handles GET /v1/air/forecast?lat=52.52&lon=13.41&hours=24
func GetForecast(c *fiber.Ctx, cl *client.Client) error {
	lat, lon, err := parseLatLon(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	tz := c.Query("timezone", "auto")
	hours, _ := strconv.Atoi(c.Query("hours", "24"))

	result, err := cl.GetForecast(c.Context(), lat, lon, tz, hours)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}
