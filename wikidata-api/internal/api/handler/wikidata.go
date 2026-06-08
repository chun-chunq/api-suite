package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"wikidata-api/internal/client"
)

type WikidataHandler struct {
	client *client.Client
	log    zerolog.Logger
}

func NewWikidataHandler(c *client.Client, log zerolog.Logger) *WikidataHandler {
	return &WikidataHandler{client: c, log: log}
}

// GET /v1/wikidata/search?query=Douglas+Adams&lang=en&type=item&limit=10
func (h *WikidataHandler) Search(c *fiber.Ctx) error {
	query := strings.TrimSpace(c.Query("query"))
	if query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "query is required"})
	}
	lang := c.Query("lang", "en")
	entityType := c.Query("type", "item")
	limit := 10
	if l := c.Query("limit"); l != "" {
		if v, e := strconv.Atoi(l); e == nil && v > 0 && v <= 50 {
			limit = v
		}
	}

	results, cont, err := h.client.Search(c.Context(), query, lang, entityType, limit)
	if err != nil {
		h.log.Error().Err(err).Str("query", query).Msg("Search failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"query":        query,
		"results":      results,
		"total":        len(results),
		"searchContinue": cont,
	})
}

// GET /v1/wikidata/entity/:id?lang=en
func (h *WikidataHandler) GetEntity(c *fiber.Ctx) error {
	id := strings.TrimSpace(c.Params("id"))
	if id == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "entity ID is required"})
	}
	lang := c.Query("lang", "en")

	entity, err := h.client.GetEntity(c.Context(), id, lang)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": err.Error()})
		}
		h.log.Error().Err(err).Str("id", id).Msg("GetEntity failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(entity)
}
