// Package handler provides HTTP handlers for the Countries API.
package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"countries-api/internal/client"
)

// All handles GET /v1/countries — returns all ~250 countries.
func All(c *fiber.Ctx, cl *client.Client) error {
	countries, err := cl.GetAll(c.Context())
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "upstream error: " + err.Error(),
		})
	}

	// Optional region filter via query param
	region := strings.TrimSpace(c.Query("region"))
	if region != "" {
		query := strings.ToLower(region)
		var filtered []client.Country
		for _, ct := range countries {
			if strings.ToLower(ct.Region) == query || strings.ToLower(ct.Subregion) == query {
				filtered = append(filtered, ct)
			}
		}
		countries = filtered
	}

	return c.JSON(fiber.Map{
		"count":     len(countries),
		"countries": countries,
	})
}

// ByCode handles GET /v1/countries/:code — lookup by CCA2/CCA3 (e.g. "DE" or "DEU").
func ByCode(c *fiber.Ctx, cl *client.Client) error {
	code := strings.TrimSpace(c.Params("code"))
	if code == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "country code is required",
		})
	}

	ct, err := cl.GetByCode(c.Context(), code)
	if err != nil {
		if err.Error() == "not_found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "country not found: " + code,
			})
		}
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.JSON(ct)
}

// Search handles GET /v1/countries/search?name=germany&full=false
func Search(c *fiber.Ctx, cl *client.Client) error {
	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "name query parameter is required",
		})
	}

	fullText := c.Query("full") == "true"

	results, err := cl.SearchByName(c.Context(), name, fullText)
	if err != nil {
		if err.Error() == "not_found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error":  "no countries found matching: " + name,
				"count":  0,
				"results": []interface{}{},
			})
		}
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"count":   len(results),
		"results": results,
	})
}

// ByLanguage handles GET /v1/countries/language/:lang — e.g. "German" or "deu"
func ByLanguage(c *fiber.Ctx, cl *client.Client) error {
	lang := strings.TrimSpace(c.Params("lang"))
	if lang == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "language is required",
		})
	}

	results, err := cl.GetByLanguage(c.Context(), lang)
	if err != nil {
		if err.Error() == "not_found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "no countries found with language: " + lang,
			})
		}
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"language": lang,
		"count":    len(results),
		"countries": results,
	})
}

// ByCurrency handles GET /v1/countries/currency/:code — e.g. "EUR"
func ByCurrency(c *fiber.Ctx, cl *client.Client) error {
	currency := strings.TrimSpace(c.Params("code"))
	if currency == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "currency code is required",
		})
	}

	results, err := cl.GetByCurrency(c.Context(), currency)
	if err != nil {
		if err.Error() == "not_found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "no countries found with currency: " + currency,
			})
		}
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"currency":  strings.ToUpper(currency),
		"count":     len(results),
		"countries": results,
	})
}
