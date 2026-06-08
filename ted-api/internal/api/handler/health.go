package handler

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/ted-api/internal/cache"
)

type HealthHandler struct{ cache *cache.Cache }

func NewHealthHandler(c *cache.Cache) *HealthHandler { return &HealthHandler{cache: c} }

func (h *HealthHandler) Health(c *fiber.Ctx) error {
	redis := "ok"
	ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
	defer cancel()
	if err := h.cache.Ping(ctx); err != nil {
		redis = "unreachable"
	}
	return c.JSON(fiber.Map{"status": "ok", "redis": redis, "upstream": "api.ted.europa.eu"})
}
