package handler

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/insolvency-api/internal/jobqueue"
)

// WorkerBridgeHandler exposes the endpoints used by the home-PC worker.
type WorkerBridgeHandler struct {
	queue        *jobqueue.Queue
	workerSecret string
}

func NewWorkerBridgeHandler(q *jobqueue.Queue, secret string) *WorkerBridgeHandler {
	return &WorkerBridgeHandler{queue: q, workerSecret: secret}
}

// Poll handles GET /internal/worker/poll
// The PC-worker calls this in a loop (long-poll, 30s timeout).
// Returns 200 + Job JSON when a job is ready, or 204 when nothing arrived in time.
func (h *WorkerBridgeHandler) Poll(c *fiber.Ctx) error {
	if !h.authorized(c) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	// Parse accepted types: ?types=insolvency,zvg
	typesParam := c.Query("types", "insolvency")
	types := map[string]bool{}
	for _, t := range strings.Split(typesParam, ",") {
		if t = strings.TrimSpace(t); t != "" {
			types[t] = true
		}
	}

	// Long-poll — waits up to 30 s for a job
	pollCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	job := h.queue.Poll(pollCtx, types)
	if job == nil {
		return c.SendStatus(fiber.StatusNoContent)
	}
	return c.JSON(job)
}

// Result handles POST /internal/worker/result/:id
// The PC-worker POSTs the scrape result here.
func (h *WorkerBridgeHandler) Result(c *fiber.Ctx) error {
	if !h.authorized(c) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}

	id := c.Params("id")
	var body struct {
		ID    string          `json:"id"`
		OK    bool            `json:"ok"`
		Data  json.RawMessage `json:"data"`
		Error string          `json:"error"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid body"})
	}
	if body.ID != "" {
		id = body.ID
	}

	if !h.queue.Complete(id, body.Data, body.Error) {
		// Job already timed out — not an error from worker's perspective
		return c.Status(fiber.StatusGone).JSON(fiber.Map{"error": "job not found or already expired"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *WorkerBridgeHandler) authorized(c *fiber.Ctx) bool {
	if h.workerSecret == "" {
		return false
	}
	return c.Get("X-Worker-Secret") == h.workerSecret
}
