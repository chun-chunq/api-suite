package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/handelsregister-api/internal/cache"
)

// HealthHandler reports service liveness and dependency status.
type HealthHandler struct {
	cache   *cache.Cache
	version string
	started time.Time
}

// NewHealthHandler constructs a HealthHandler.
func NewHealthHandler(c *cache.Cache, version string) *HealthHandler {
	return &HealthHandler{cache: c, version: version, started: time.Now()}
}

// Health is the GET /health endpoint.
func (h *HealthHandler) Health(ctx *fiber.Ctx) error {
	status := "ok"
	redisOK := true

	if err := h.cache.Health(ctx.UserContext()); err != nil {
		status = "degraded"
		redisOK = false
	}

	code := fiber.StatusOK
	if status != "ok" {
		code = fiber.StatusServiceUnavailable
	}

	return ctx.Status(code).JSON(fiber.Map{
		"status":  status,
		"version": h.version,
		"uptime":  time.Since(h.started).String(),
		"deps": fiber.Map{
			"redis": redisOK,
		},
	})
}
