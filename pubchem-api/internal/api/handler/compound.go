// Package handler provides HTTP handlers for the PubChem API.
package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"pubchem-api/internal/client"
)

// Search handles GET /v1/chem/search?name=aspirin&limit=5
func Search(c *fiber.Ctx, cl *client.Client) error {
	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "name query parameter is required",
		})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "10"))
	if limit <= 0 || limit > 20 {
		limit = 10
	}

	sr, err := cl.SearchByName(c.Context(), name, limit)
	if err != nil {
		if err.Error() == "not_found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "no compounds found for: " + name,
				"cids":  []int64{},
				"total": 0,
			})
		}
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "upstream error: " + err.Error(),
		})
	}

	return c.JSON(sr)
}

// GetByCID handles GET /v1/chem/cid/:cid
func GetByCID(c *fiber.Ctx, cl *client.Client) error {
	cidStr := c.Params("cid")
	cid, err := strconv.ParseInt(cidStr, 10, 64)
	if err != nil || cid <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid CID: must be a positive integer",
		})
	}

	compound, err := cl.GetByCID(c.Context(), cid)
	if err != nil {
		if err.Error() == "not_found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "compound not found: CID " + cidStr,
			})
		}
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(compound)
}

// GetByName handles GET /v1/chem/name/:name — fetches best matching compound with full details.
func GetByName(c *fiber.Ctx, cl *client.Client) error {
	name := strings.TrimSpace(c.Params("name"))
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "name is required",
		})
	}

	compound, err := cl.GetByName(c.Context(), name)
	if err != nil {
		if err.Error() == "not_found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "compound not found: " + name,
			})
		}
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(compound)
}

// GetSynonyms handles GET /v1/chem/cid/:cid/synonyms?limit=20
func GetSynonyms(c *fiber.Ctx, cl *client.Client) error {
	cidStr := c.Params("cid")
	cid, err := strconv.ParseInt(cidStr, 10, 64)
	if err != nil || cid <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid CID",
		})
	}

	limit, _ := strconv.Atoi(c.Query("limit", "20"))
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	synonyms, err := cl.GetSynonyms(c.Context(), cid, limit)
	if err != nil {
		if err.Error() == "not_found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "compound not found: CID " + cidStr,
			})
		}
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"cid":      cid,
		"count":    len(synonyms),
		"synonyms": synonyms,
	})
}

// GetDescription handles GET /v1/chem/cid/:cid/description
func GetDescription(c *fiber.Ctx, cl *client.Client) error {
	cidStr := c.Params("cid")
	cid, err := strconv.ParseInt(cidStr, 10, 64)
	if err != nil || cid <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid CID",
		})
	}

	desc, err := cl.GetDescription(c.Context(), cid)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"cid":         cid,
		"description": desc,
	})
}
