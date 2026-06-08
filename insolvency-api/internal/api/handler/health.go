package handler

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/insolvency-api/internal/cache"
	"github.com/insolvency-api/internal/pool"
)

type HealthHandler struct {
	cache *cache.Cache
	pool  *pool.Pool
}

func NewHealthHandler(c *cache.Cache, p *pool.Pool) *HealthHandler {
	return &HealthHandler{cache: c, pool: p}
}

func (h *HealthHandler) Health(c *fiber.Ctx) error {
	ctx, cancel := context.WithTimeout(c.Context(), 2*time.Second)
	defer cancel()

	redisStatus := "ok"
	if h.cache != nil {
		if err := h.cache.Ping(ctx); err != nil {
			redisStatus = "unavailable"
		}
	} else {
		redisStatus = "disabled"
	}

	overall := "healthy"
	if redisStatus == "unavailable" {
		overall = "degraded"
	}

	resp := fiber.Map{
		"status":  overall,
		"service": "insolvency-api",
		"time":    time.Now().UTC().Format(time.RFC3339),
		"redis":   redisStatus,
	}
	if h.pool != nil {
		resp["workers"] = h.pool.Status()
	}
	return c.JSON(resp)
}
