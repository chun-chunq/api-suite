package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"ipgeo-api/internal/client"
)

type GeoHandler struct {
	client *client.Client
	log    zerolog.Logger
}

func NewGeoHandler(c *client.Client, log zerolog.Logger) *GeoHandler {
	return &GeoHandler{client: c, log: log}
}

// GET /v1/ipgeo/lookup/:ip    — single IP (or "self" for caller's IP)
// GET /v1/ipgeo/lookup        — caller's IP (no param)
func (h *GeoHandler) Lookup(c *fiber.Ctx) error {
	ip := strings.TrimSpace(c.Params("ip", ""))

	result, err := h.client.Lookup(c.Context(), ip)
	if err != nil {
		if strings.Contains(err.Error(), "invalid IP") {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
		}
		if strings.Contains(err.Error(), "ip-api error") {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{"error": err.Error()})
		}
		h.log.Error().Err(err).Str("ip", ip).Msg("Lookup failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}

// POST /v1/ipgeo/batch
// Body: {"ips":["1.1.1.1","8.8.8.8"]}
func (h *GeoHandler) Batch(c *fiber.Ctx) error {
	var body struct {
		IPs []string `json:"ips"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid JSON body"})
	}
	if len(body.IPs) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "ips array is required"})
	}
	if len(body.IPs) > 100 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "max 100 IPs per batch"})
	}

	results, err := h.client.LookupBatch(c.Context(), body.IPs)
	if err != nil {
		h.log.Error().Err(err).Int("count", len(body.IPs)).Msg("Batch failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"results": results, "count": len(results)})
}
