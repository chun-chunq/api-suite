package handler

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/hibiken/asynq"
	"github.com/rs/zerolog"

	"github.com/handelsregister-api/internal/cache"
	"github.com/handelsregister-api/internal/scraper"
	"github.com/handelsregister-api/internal/worker"
)

// CompanyHandler serves company lookup and search endpoints.
//
// Strategy: serve from cache when warm. On a miss, scrape synchronously within
// a bounded deadline so the customer gets an immediate answer, and also enqueue
// an async refresh task so subsequent requests are fast and the browser load is
// amortized. If the synchronous scrape exceeds the deadline, we return 202 and
// the result lands in cache for the retry.
type CompanyHandler struct {
	cache      *cache.Cache
	scraper    *scraper.Scraper
	queue      *asynq.Client
	log        zerolog.Logger
	syncBudget time.Duration
}

// NewCompanyHandler constructs a CompanyHandler.
func NewCompanyHandler(c *cache.Cache, s *scraper.Scraper, q *asynq.Client, log zerolog.Logger) *CompanyHandler {
	return &CompanyHandler{
		cache:      c,
		scraper:    s,
		queue:      q,
		log:        log,
		syncBudget: 30 * time.Second,
	}
}

// GetByHRB handles GET /v1/company/:hrb?state=Bayern
func (h *CompanyHandler) GetByHRB(ctx *fiber.Ctx) error {
	hrb := ctx.Params("hrb")
	state := ctx.Query("state")

	if hrb == "" {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "hrb path parameter is required",
		})
	}

	key := cache.CompanyKey(hrb, state)

	// 1. Cache hit -> return immediately.
	var cached scraper.CompanyData
	if err := h.cache.GetJSON(ctx.UserContext(), key, &cached); err == nil {
		ctx.Set("X-Cache", "HIT")
		return ctx.JSON(fiber.Map{"data": cached, "cached": true})
	} else if !errors.Is(err, cache.ErrMiss) {
		h.log.Error().Err(err).Str("key", key).Msg("cache read error")
	}

	// 2. Synchronous scrape within budget.
	sctx, cancel := context.WithTimeout(ctx.UserContext(), h.syncBudget)
	defer cancel()

	data, err := h.scraper.GetByHRB(sctx, hrb, state)
	if err != nil {
		if errors.Is(err, scraper.ErrNotFound) {
			return ctx.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "company not found",
				"hrb":   hrb,
				"state": state,
			})
		}
		if errors.Is(err, context.DeadlineExceeded) {
			h.enqueueCompany(ctx, hrb, state)
			return ctx.Status(fiber.StatusAccepted).JSON(fiber.Map{
				"status":  "processing",
				"message": "scrape in progress, retry shortly",
			})
		}
		h.log.Error().Err(err).Str("hrb", hrb).Msg("synchronous scrape failed")
		return ctx.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "upstream registry unavailable",
		})
	}

	// 3. Warm cache (best-effort) and respond.
	if err := h.cache.SetJSON(ctx.UserContext(), key, data); err != nil {
		h.log.Warn().Err(err).Msg("failed to warm cache")
	}

	ctx.Set("X-Cache", "MISS")
	return ctx.JSON(fiber.Map{"data": data, "cached": false})
}

// Search handles GET /v1/company/search?name=BMW
func (h *CompanyHandler) Search(ctx *fiber.Ctx) error {
	name := ctx.Query("name")
	if name == "" {
		return ctx.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "name query parameter is required",
		})
	}

	key := cache.SearchKey(name)

	var cached []scraper.SearchResult
	if err := h.cache.GetJSON(ctx.UserContext(), key, &cached); err == nil {
		ctx.Set("X-Cache", "HIT")
		return ctx.JSON(fiber.Map{"data": cached, "count": len(cached), "cached": true})
	} else if !errors.Is(err, cache.ErrMiss) {
		h.log.Error().Err(err).Str("key", key).Msg("cache read error")
	}

	sctx, cancel := context.WithTimeout(ctx.UserContext(), h.syncBudget)
	defer cancel()

	results, err := h.scraper.Search(sctx, name)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			h.enqueueSearch(ctx, name)
			return ctx.Status(fiber.StatusAccepted).JSON(fiber.Map{
				"status":  "processing",
				"message": "search in progress, retry shortly",
			})
		}
		h.log.Error().Err(err).Str("name", name).Msg("search failed")
		return ctx.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "upstream registry unavailable",
		})
	}

	if err := h.cache.SetJSONTTL(ctx.UserContext(), key, results, time.Hour); err != nil {
		h.log.Warn().Err(err).Msg("failed to warm search cache")
	}

	ctx.Set("X-Cache", "MISS")
	return ctx.JSON(fiber.Map{"data": results, "count": len(results), "cached": false})
}

func (h *CompanyHandler) enqueueCompany(ctx *fiber.Ctx, hrb, state string) {
	task, err := worker.NewScrapeCompanyTask(hrb, state)
	if err != nil {
		h.log.Error().Err(err).Msg("build company task")
		return
	}
	if _, err := h.queue.Enqueue(task, asynq.MaxRetry(3), asynq.Timeout(60*time.Second)); err != nil {
		h.log.Error().Err(err).Msg("enqueue company task")
	}
}

func (h *CompanyHandler) enqueueSearch(ctx *fiber.Ctx, name string) {
	task, err := worker.NewScrapeSearchTask(name)
	if err != nil {
		h.log.Error().Err(err).Msg("build search task")
		return
	}
	if _, err := h.queue.Enqueue(task, asynq.MaxRetry(3), asynq.Timeout(60*time.Second)); err != nil {
		h.log.Error().Err(err).Msg("enqueue search task")
	}
}
