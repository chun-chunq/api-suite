package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"worldbank-api/internal/client"
)

// GetCountry handles GET /v1/worldbank/country/:code
func GetCountry(c *fiber.Ctx, cl *client.Client) error {
	code := strings.TrimSpace(c.Params("code"))
	if code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "country code is required in path (/v1/worldbank/country/{code})",
		})
	}

	country, err := cl.GetCountry(c.Context(), code)
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
	return c.JSON(country)
}

// GetIndicator handles GET /v1/worldbank/indicator?country=DE&indicator=NY.GDP.MKTP.CD&start=2010&end=2022
func GetIndicator(c *fiber.Ctx, cl *client.Client) error {
	country := strings.TrimSpace(c.Query("country"))
	indicator := strings.TrimSpace(c.Query("indicator"))
	if country == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "query parameter 'country' (ISO2 code or 'all') is required",
		})
	}
	if indicator == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "query parameter 'indicator' (e.g. NY.GDP.MKTP.CD) is required",
		})
	}

	startYear, _ := strconv.Atoi(c.Query("start", "0"))
	endYear, _ := strconv.Atoi(c.Query("end", "0"))

	result, err := cl.GetIndicator(c.Context(), country, indicator, startYear, endYear)
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
	return c.JSON(result)
}

// ListIndicators handles GET /v1/worldbank/indicators — returns common indicators
func ListIndicators(c *fiber.Ctx, cl *client.Client) error {
	indicators := cl.CommonIndicators()
	return c.JSON(fiber.Map{
		"count":      len(indicators),
		"indicators": indicators,
	})
}
