// Package handler provides HTTP handlers for the GBIF Biodiversity API.
package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gbif-api/internal/client"
)

// SearchSpecies handles GET /v1/bio/species?q=lion&rank=SPECIES&limit=20
func SearchSpecies(c *fiber.Ctx, cl *client.Client) error {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "q query parameter is required",
		})
	}
	rank := c.Query("rank")
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	result, err := cl.SearchSpecies(c.Context(), q, rank, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}

// GetSpecies handles GET /v1/bio/species/:key
func GetSpecies(c *fiber.Ctx, cl *client.Client) error {
	keyStr := c.Params("key")
	key, err := strconv.Atoi(keyStr)
	if err != nil || key <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid species key"})
	}

	species, err := cl.GetSpecies(c.Context(), key)
	if err != nil {
		if err.Error() == "not_found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "species not found: " + keyStr})
		}
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(species)
}

// GetVernacularNames handles GET /v1/bio/species/:key/names?lang=eng
func GetVernacularNames(c *fiber.Ctx, cl *client.Client) error {
	keyStr := c.Params("key")
	key, err := strconv.Atoi(keyStr)
	if err != nil || key <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid species key"})
	}
	lang := c.Query("lang", "eng")

	names, err := cl.GetVernacularNames(c.Context(), key, lang)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"key":      key,
		"language": lang,
		"count":    len(names),
		"names":    names,
	})
}

// SearchOccurrences handles GET /v1/bio/occurrences?speciesKey=5219404&country=KE&year=2022
func SearchOccurrences(c *fiber.Ctx, cl *client.Client) error {
	speciesKey, _ := strconv.Atoi(c.Query("speciesKey"))
	country := strings.TrimSpace(c.Query("country"))
	year, _ := strconv.Atoi(c.Query("year"))
	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	offset, _ := strconv.Atoi(c.Query("offset", "0"))

	if speciesKey <= 0 && country == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "at least speciesKey or country is required",
		})
	}

	result, err := cl.SearchOccurrences(c.Context(), speciesKey, country, year, limit, offset)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}
