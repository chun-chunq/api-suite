package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"openfda-api/internal/client"
)

type DrugHandler struct {
	client *client.Client
	log    zerolog.Logger
}

func NewDrugHandler(c *client.Client, log zerolog.Logger) *DrugHandler {
	return &DrugHandler{client: c, log: log}
}

// GET /v1/drug/labels?query=aspirin&limit=10&skip=0
func (h *DrugHandler) SearchLabels(c *fiber.Ctx) error {
	query := strings.TrimSpace(c.Query("query"))
	if query == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "query is required"})
	}
	limit, skip := parsePagination(c)

	result, err := h.client.SearchDrugLabels(c.Context(), query, limit, skip)
	if err != nil {
		if strings.Contains(err.Error(), "no results") {
			return c.JSON(fiber.Map{"items": []interface{}{}, "total": 0, "skip": skip, "limit": limit})
		}
		h.log.Error().Err(err).Str("query", query).Msg("SearchDrugLabels failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}

// GET /v1/drug/events?drug=ibuprofen&limit=10&skip=0
func (h *DrugHandler) SearchEvents(c *fiber.Ctx) error {
	drug := strings.TrimSpace(c.Query("drug"))
	if drug == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "drug is required"})
	}
	limit, skip := parsePagination(c)

	result, err := h.client.SearchAdverseEvents(c.Context(), drug, limit, skip)
	if err != nil {
		if strings.Contains(err.Error(), "no results") {
			return c.JSON(fiber.Map{"items": []interface{}{}, "total": 0, "skip": skip, "limit": limit})
		}
		h.log.Error().Err(err).Str("drug", drug).Msg("SearchAdverseEvents failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}

// GET /v1/drug/recalls?query=bayer&class=I&limit=10&skip=0
func (h *DrugHandler) SearchRecalls(c *fiber.Ctx) error {
	query := strings.TrimSpace(c.Query("query"))
	class := strings.TrimSpace(c.Query("class"))
	limit, skip := parsePagination(c)

	result, err := h.client.SearchRecalls(c.Context(), query, class, limit, skip)
	if err != nil {
		if strings.Contains(err.Error(), "no results") {
			return c.JSON(fiber.Map{"items": []interface{}{}, "total": 0, "skip": skip, "limit": limit})
		}
		h.log.Error().Err(err).Str("query", query).Msg("SearchRecalls failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}

func parsePagination(c *fiber.Ctx) (limit, skip int) {
	limit = 10
	if l := c.Query("limit"); l != "" {
		if v, e := strconv.Atoi(l); e == nil && v > 0 && v <= 100 {
			limit = v
		}
	}
	skip = 0
	if s := c.Query("skip"); s != "" {
		if v, e := strconv.Atoi(s); e == nil && v >= 0 {
			skip = v
		}
	}
	return
}
