package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"encoding/json"
	"strconv"

	"github.com/insolvency-api/internal/analytics"
	"github.com/insolvency-api/internal/cache"
	"github.com/insolvency-api/internal/jobqueue"
	"github.com/insolvency-api/internal/pool"
	"github.com/insolvency-api/internal/scraper"
	"github.com/insolvency-api/internal/scrapequeue"
)

// InsolvencyHandler serves the public insolvency endpoints.
type InsolvencyHandler struct {
	cache          *cache.Cache
	log            zerolog.Logger
	searchCacheTTL time.Duration
	recordCacheTTL time.Duration
	pool           *pool.Pool
	jq             *jobqueue.Queue
	queue          *scrapequeue.Queue
	analytics      *analytics.Analytics
	chromeBin      string
}

// NewInsolvencyHandler builds the handler.
func NewInsolvencyHandler(c *cache.Cache, log zerolog.Logger, searchTTL, recordTTL time.Duration, p *pool.Pool, jq *jobqueue.Queue, q *scrapequeue.Queue, an *analytics.Analytics, chromeBin string) *InsolvencyHandler {
	return &InsolvencyHandler{
		cache:          c,
		log:            log,
		searchCacheTTL: searchTTL,
		recordCacheTTL: recordTTL,
		pool:           p,
		jq:             jq,
		queue:          q,
		analytics:      an,
		chromeBin:      chromeBin,
	}
}

// Search handles GET /v1/insolvency/search.
func (h *InsolvencyHandler) Search(c *fiber.Ctx) error {
	q := scraper.SearchQuery{
		Name:           strings.TrimSpace(c.Query("name")),
		State:          strings.ToUpper(strings.TrimSpace(c.Query("state"))),
		RegisterType:   strings.ToUpper(strings.TrimSpace(c.Query("registerType"))),
		RegisterNumber: strings.TrimSpace(c.Query("registerNumber")),
		Subject:        strings.TrimSpace(c.Query("subject")),
		FirstName:      strings.TrimSpace(c.Query("firstName")),
		City:           strings.TrimSpace(c.Query("city")),
		MatchMode:      strings.TrimSpace(c.Query("matchMode")),
		CaseNumber:     strings.TrimSpace(c.Query("caseNumber")),
	}

	if df := c.Query("dateFrom"); df != "" {
		t, err := time.Parse("2006-01-02", df)
		if err != nil {
			return badRequest(c, "dateFrom must be YYYY-MM-DD")
		}
		q.DateFrom = t
	}
	if dt := c.Query("dateTo"); dt != "" {
		t, err := time.Parse("2006-01-02", dt)
		if err != nil {
			return badRequest(c, "dateTo must be YYYY-MM-DD")
		}
		q.DateTo = t
	}

	if q.Name == "" && q.RegisterNumber == "" && q.FirstName == "" && q.City == "" && q.CaseNumber == "" {
		return badRequest(c, "at least one of 'name', 'firstName', 'city', 'registerNumber' or 'caseNumber' is required")
	}

	cacheKey := fmt.Sprintf("search:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s:%s",
		q.Name, q.FirstName, q.City, q.State, q.RegisterType, q.RegisterNumber,
		q.Subject, q.MatchMode, q.CaseNumber,
		q.Court, dateStr(q.DateFrom), dateStr(q.DateTo))

	apiKey := c.Get("X-API-Key")
	if apiKey == "" {
		apiKey = c.Get("X-RapidAPI-Proxy-Secret")
	}

	var result scraper.SearchResult
	if h.cacheGet(c.Context(), cacheKey, &result) {
		h.analytics.Record(analytics.Record{
			Time: time.Now().UnixMilli(), Endpoint: "/v1/insolvency/search",
			StatusCode: 200, Cached: true, APIKey: apiKey,
		})
		c.Set("X-Cache", "HIT")
		return c.JSON(result)
	}

	start := time.Now()
	res, waitDur, err := h.runSearch(c.Context(), q)
	latMs := time.Since(start).Milliseconds()
	if err != nil {
		h.analytics.Record(analytics.Record{
			Time: start.UnixMilli(), Endpoint: "/v1/insolvency/search",
			StatusCode: 502, LatencyMs: latMs, Scrape: true, Error: true, APIKey: apiKey,
		})
		if errors.Is(err, scrapequeue.ErrQueueFull) {
			h.analytics.RecordQueueRejected()
			retry := h.queue.RetryAfterSeconds(60)
			c.Set("Retry-After", strconv.Itoa(retry))
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":             "scrape queue full — too many concurrent requests",
				"retryAfterSeconds": retry,
			})
		}
		return upstreamError(c, err)
	}

	h.analytics.Record(analytics.Record{
		Time: start.UnixMilli(), Endpoint: "/v1/insolvency/search",
		StatusCode: 200, LatencyMs: latMs, QueueWaitMs: waitDur.Milliseconds(),
		Scrape: true, APIKey: apiKey,
	})
	h.cacheSet(c.Context(), cacheKey, res, h.searchCacheTTL)
	c.Set("X-Cache", "MISS")
	c.Set("X-Queue-Wait-Ms", strconv.FormatInt(waitDur.Milliseconds(), 10))
	return c.JSON(res)
}

// Company handles GET /v1/insolvency/company/{hrb}.
func (h *InsolvencyHandler) Company(c *fiber.Ctx) error {
	hrb := strings.TrimSpace(c.Params("hrb"))
	if hrb == "" {
		return badRequest(c, "hrb path parameter is required")
	}
	state := strings.ToUpper(strings.TrimSpace(c.Query("state")))

	cacheKey := fmt.Sprintf("company:%s:%s", hrb, state)
	var result scraper.SearchResult
	if h.cacheGet(c.Context(), cacheKey, &result) {
		c.Set("X-Cache", "HIT")
		return c.JSON(result)
	}

	sc, err := scraper.New(scraper.Options{Logger: h.log, BrowserBin: h.chromeBin})
	if err != nil {
		return upstreamError(c, err)
	}
	defer sc.Close()
	ctx, cancel := context.WithTimeout(c.Context(), 60*time.Second)
	defer cancel()

	res, err := sc.SearchByHRB(ctx, hrb, state)
	if err != nil {
		return upstreamError(c, err)
	}

	h.cacheSet(c.Context(), cacheKey, res, h.recordCacheTTL)
	c.Set("X-Cache", "MISS")
	return c.JSON(res)
}

// Monitor handles GET /v1/insolvency/monitor. Webhook subscription management is
// stubbed; it returns the currently monitored companies (empty in this build).
func (h *InsolvencyHandler) Monitor(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"monitored":   []any{},
		"description": "Add companies via the (forthcoming) POST /v1/insolvency/monitor endpoint to receive webhook alerts when new insolvency announcements are published.",
		"webhookSupported": true,
	})
}

func (h *InsolvencyHandler) runSearch(ctx context.Context, q scraper.SearchQuery) (*scraper.SearchResult, time.Duration, error) {
	// 1. Push-workers (remote IP pool)
	if h.pool != nil && h.pool.HasWorkers() {
		var result scraper.SearchResult
		if err := h.pool.Dispatch(ctx, "/scrape/insolvency", q, &result); err == nil {
			return &result, 0, nil
		}
		h.log.Warn().Msg("all push-workers failed; trying PC-worker bridge")
	}

	// 2. Home-PC pull-worker bridge
	if h.jq != nil && h.jq.HasWorkers() {
		raw, err := h.jq.Dispatch(ctx, "insolvency", q)
		if err == nil {
			var result scraper.SearchResult
			if jsonErr := json.Unmarshal(raw, &result); jsonErr == nil {
				return &result, 0, nil
			}
		}
		h.log.Warn().Err(err).Msg("PC-worker bridge failed; falling back to local scrape")
	}

	// 3. Local scrape — go through the bounded queue
	release, waitDur, err := h.queue.Acquire(ctx)
	if err != nil {
		return nil, waitDur, err
	}
	defer func() { release(); h.queue.Complete() }()

	sc, err := scraper.New(scraper.Options{Logger: h.log, BrowserBin: h.chromeBin})
	if err != nil {
		return nil, waitDur, err
	}
	defer sc.Close()
	cctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	result, scrapeErr := sc.Search(cctx, q)
	return result, waitDur, scrapeErr
}

// ---- cache helpers ----

func (h *InsolvencyHandler) cacheGet(ctx context.Context, key string, dst any) bool {
	if h.cache == nil {
		return false
	}
	err := h.cache.GetJSON(ctx, key, dst)
	if err == nil {
		return true
	}
	if !errors.Is(err, cache.ErrMiss) {
		h.log.Warn().Err(err).Str("key", key).Msg("cache get failed")
	}
	return false
}

func (h *InsolvencyHandler) cacheSet(ctx context.Context, key string, val any, ttl time.Duration) {
	if h.cache == nil {
		return
	}
	if err := h.cache.SetJSON(ctx, key, val, ttl); err != nil {
		h.log.Warn().Err(err).Str("key", key).Msg("cache set failed")
	}
}

// ---- response helpers ----

func badRequest(c *fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": msg})
}

func upstreamError(c *fiber.Ctx, err error) error {
	return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
		"error":   "upstream insolvency portal error",
		"details": err.Error(),
	})
}

func dateStr(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

