package handler

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/dpma-api/internal/jobqueue"
)

type WorkerBridgeHandler struct {
	queue        *jobqueue.Queue
	workerSecret string
}

func NewWorkerBridgeHandler(q *jobqueue.Queue, secret string) *WorkerBridgeHandler {
	return &WorkerBridgeHandler{queue: q, workerSecret: secret}
}

// Poll handles GET /internal/worker/poll — long-poll for home-PC worker.
func (h *WorkerBridgeHandler) Poll(c *fiber.Ctx) error {
	if !h.authorized(c) {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"error": "unauthorized"})
	}
	typesParam := c.Query("types", "trademark")
	types := map[string]bool{}
	for _, t := range strings.Split(typesParam, ",") {
		if t = strings.TrimSpace(t); t != "" {
			types[t] = true
		}
	}
	pollCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	job := h.queue.Poll(pollCtx, types)
	if job == nil {
		return c.SendStatus(fiber.StatusNoContent)
	}
	return c.JSON(job)
}

// Result handles POST /internal/worker/result/:id
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
		return c.Status(fiber.StatusGone).JSON(fiber.Map{"error": "job not found or expired"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *WorkerBridgeHandler) authorized(c *fiber.Ctx) bool {
	return h.workerSecret != "" && c.Get("X-Worker-Secret") == h.workerSecret
}
