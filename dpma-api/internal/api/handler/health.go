package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/dpma-api/internal/cache"
	"github.com/dpma-api/internal/pool"
)

type HealthHandler struct {
	cache *cache.Cache
	pool  *pool.Pool
}

func NewHealthHandler(c *cache.Cache, p *pool.Pool) *HealthHandler {
	return &HealthHandler{cache: c, pool: p}
}

func (h *HealthHandler) Health(c *fiber.Ctx) error {
	status := "ok"
	services := fiber.Map{
		"api": "ok",
	}

	if h.cache != nil {
		if err := h.cache.Ping(c.Context()); err != nil {
			services["redis"] = "unavailable"
			status = "degraded"
		} else {
			services["redis"] = "ok"
		}
	}

	if h.pool != nil {
		services["workers"] = h.pool.Status()
	}

	code := fiber.StatusOK
	if status != "ok" {
		code = fiber.StatusServiceUnavailable
	}
	return c.Status(code).JSON(fiber.Map{"status": status, "services": services})
}
