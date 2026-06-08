package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/dpma-api/internal/analytics"
	"github.com/dpma-api/internal/api/middleware"
	"github.com/dpma-api/internal/pool"
	"github.com/dpma-api/internal/scrapequeue"
)

// AdminHandler exposes runtime management endpoints for the worker pool.
// All endpoints require X-Admin-Secret header.
type AdminHandler struct {
	pool      *pool.Pool
	bandwidth *middleware.BandwidthTracker
	analytics *analytics.Analytics
	queue     *scrapequeue.Queue
}

func NewAdminHandler(p *pool.Pool, bw *middleware.BandwidthTracker, an *analytics.Analytics, q *scrapequeue.Queue) *AdminHandler {
	return &AdminHandler{pool: p, bandwidth: bw, analytics: an, queue: q}
}

// Workers handles GET/POST/DELETE /admin/workers
func (h *AdminHandler) Workers(c *fiber.Ctx) error {
	switch c.Method() {
	case fiber.MethodGet:
		return c.JSON(fiber.Map{
			"workers": h.pool.Status(),
		})

	case fiber.MethodPost:
		var body struct {
			URL string `json:"url"`
		}
		if err := c.BodyParser(&body); err != nil || strings.TrimSpace(body.URL) == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "body must be {\"url\": \"http://...\"}"})
		}
		url := strings.TrimSpace(body.URL)
		if !strings.HasPrefix(url, "http") {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "url must start with http:// or https://"})
		}
		h.pool.AddWorker(url)
		return c.JSON(fiber.Map{"added": url, "workers": h.pool.Status()})

	case fiber.MethodDelete:
		var body struct {
			URL string `json:"url"`
		}
		if err := c.BodyParser(&body); err != nil || body.URL == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "body must be {\"url\": \"http://...\"}"})
		}
		removed := h.pool.RemoveWorker(body.URL)
		return c.JSON(fiber.Map{"removed": removed, "workers": h.pool.Status()})
	}
	return c.Status(fiber.StatusMethodNotAllowed).JSON(fiber.Map{"error": "use GET, POST, or DELETE"})
}

// Stats handles GET /admin/stats — resource usage + queue status
func (h *AdminHandler) Stats(c *fiber.Ctx) error {
	m := fiber.Map{
		"workers": h.pool.Status(),
	}
	if h.bandwidth != nil {
		m["bandwidth"] = h.bandwidth.Stats()
	}
	if h.queue != nil {
		m["queue"] = h.queue.Stats()
	}
	return c.JSON(m)
}

// Analytics handles GET /admin/analytics — request metrics + recent log
func (h *AdminHandler) Analytics(c *fiber.Ctx) error {
	if h.analytics == nil {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"error": "analytics not enabled"})
	}
	return c.JSON(h.analytics.Snapshot())
}
