package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"github.com/dpma-api/internal/analytics"
	"github.com/dpma-api/internal/cache"
	"github.com/dpma-api/internal/jobqueue"
	"github.com/dpma-api/internal/pool"
	"github.com/dpma-api/internal/scraper"
	"github.com/dpma-api/internal/scrapequeue"
)

type TrademarkHandler struct {
	cache     *cache.Cache
	log       zerolog.Logger
	cacheTTL  time.Duration
	chromeBin string
	pool      *pool.Pool
	jq        *jobqueue.Queue
	queue     *scrapequeue.Queue
	analytics *analytics.Analytics
}

func NewTrademarkHandler(
	c *cache.Cache,
	log zerolog.Logger,
	ttl time.Duration,
	chromeBin string,
	p *pool.Pool,
	jq *jobqueue.Queue,
	q *scrapequeue.Queue,
	an *analytics.Analytics,
) *TrademarkHandler {
	return &TrademarkHandler{
		cache:     c,
		log:       log,
		cacheTTL:  ttl,
		chromeBin: chromeBin,
		pool:      p,
		jq:        jq,
		queue:     q,
		analytics: an,
	}
}

// Search handles GET /v1/trademark/search
//
// Query params:
//
//	name            — trademark name / keyword
//	registrationNumber — exact DPMA registration number
//	owner           — owner/applicant name
//	class           — comma-separated Nice classes (1–45), e.g. "9,35,42"
//	status          — registered | applied | expired | deleted
//	markType        — word | figurative | combined | 3d | sound | color
//	dateFrom        — filing date from (YYYY-MM-DD)
//	dateTo          — filing date to (YYYY-MM-DD)
//	maxResults      — max results to return (default 50, max 200)
func (h *TrademarkHandler) Search(c *fiber.Ctx) error {
	start := time.Now()
	apiKey := c.Get("X-API-Key")
	if apiKey == "" {
		apiKey = c.Get("X-RapidAPI-Proxy-Secret")
	}
	rec := analytics.Record{
		Time:     start.UnixMilli(),
		Endpoint: "/v1/trademark/search",
		Method:   c.Method(),
		APIKey:   apiKey,
	}
	defer func() {
		rec.LatencyMs = time.Since(start).Milliseconds()
		rec.StatusCode = int32(c.Response().StatusCode())
		rec.Error = rec.StatusCode >= 400
		h.analytics.Record(rec)
	}()

	q, err := parseQuery(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if q.Name == "" && q.RegistrationNumber == "" && q.Owner == "" && len(q.Classes) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "at least one parameter required: name, registrationNumber, owner, or class",
		})
	}

	cacheKey := fmt.Sprintf("dpma:search:%s:%s:%s:%v:%s:%s:%s:%s:%d",
		q.Name, q.RegistrationNumber, q.Owner, q.Classes,
		q.Status, q.MarkType, q.DateFrom, q.DateTo, q.MaxResults)

	var result scraper.SearchResult
	if h.cacheGet(c.Context(), cacheKey, &result) {
		rec.Cached = true
		c.Set("X-Cache", "HIT")
		c.Set("X-Queue-Depth", "0")
		return c.JSON(result)
	}

	rec.Scrape = true
	res, queueWait, err := h.runSearch(c.Context(), q)
	rec.QueueWaitMs = queueWait.Milliseconds()
	if err != nil {
		h.log.Error().Err(err).Msg("trademark search failed")
		if errors.Is(err, scrapequeue.ErrQueueFull) {
			h.analytics.RecordQueueRejected()
			retry := h.queue.RetryAfterSeconds(45)
			c.Set("Retry-After", strconv.Itoa(retry))
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":            "scrape queue full — too many concurrent requests",
				"queueDepth":       h.queue.Depth(),
				"retryAfterSeconds": retry,
			})
		}
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "DPMA portal error",
			"details": err.Error(),
		})
	}

	h.cacheSet(c.Context(), cacheKey, res, h.cacheTTL)
	c.Set("X-Cache", "MISS")
	c.Set("X-Queue-Wait-Ms", strconv.FormatInt(queueWait.Milliseconds(), 10))
	return c.JSON(res)
}

// Detail handles GET /v1/trademark/:number
func (h *TrademarkHandler) Detail(c *fiber.Ctx) error {
	start := time.Now()
	apiKey2 := c.Get("X-API-Key")
	if apiKey2 == "" {
		apiKey2 = c.Get("X-RapidAPI-Proxy-Secret")
	}
	rec := analytics.Record{
		Time:     start.UnixMilli(),
		Endpoint: "/v1/trademark/:number",
		Method:   c.Method(),
		APIKey:   apiKey2,
	}
	defer func() {
		rec.LatencyMs = time.Since(start).Milliseconds()
		rec.StatusCode = int32(c.Response().StatusCode())
		rec.Error = rec.StatusCode >= 400
		h.analytics.Record(rec)
	}()

	num := strings.TrimSpace(c.Params("number"))
	if num == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "registration number required"})
	}

	cacheKey := "dpma:detail:" + num
	var tm scraper.Trademark
	if h.cacheGet(c.Context(), cacheKey, &tm) {
		rec.Cached = true
		c.Set("X-Cache", "HIT")
		return c.JSON(tm)
	}

	rec.Scrape = true
	q := scraper.SearchQuery{RegistrationNumber: num, MaxResults: 1}
	res, queueWait, err := h.runSearch(c.Context(), q)
	rec.QueueWaitMs = queueWait.Milliseconds()
	if err != nil {
		if errors.Is(err, scrapequeue.ErrQueueFull) {
			h.analytics.RecordQueueRejected()
			c.Set("Retry-After", strconv.Itoa(h.queue.RetryAfterSeconds(45)))
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": "scrape queue full — too many concurrent requests",
			})
		}
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	if len(res.Results) == 0 {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "trademark not found"})
	}

	h.cacheSet(c.Context(), cacheKey, res.Results[0], 24*time.Hour)
	c.Set("X-Cache", "MISS")
	return c.JSON(res.Results[0])
}

// runSearch tries workers in order, falls back to local scrape with queue.
// Returns the result, how long the request waited in queue, and any error.
func (h *TrademarkHandler) runSearch(ctx context.Context, q scraper.SearchQuery) (*scraper.SearchResult, time.Duration, error) {
	// 1. Push-workers (remote IP pool) — no queue needed, they're remote
	if h.pool != nil && h.pool.HasWorkers() {
		var result scraper.SearchResult
		if err := h.pool.Dispatch(ctx, "/scrape/trademark", q, &result); err == nil {
			return &result, 0, nil
		}
		h.log.Warn().Msg("push-workers failed; trying PC-worker bridge")
	}

	// 2. Home-PC pull-worker bridge
	if h.jq != nil && h.jq.HasWorkers() {
		raw, err := h.jq.Dispatch(ctx, "trademark", q)
		if err == nil {
			var result scraper.SearchResult
			if jsonErr := json.Unmarshal(raw, &result); jsonErr == nil {
				return &result, 0, nil
			}
		}
		h.log.Warn().Msg("PC-worker bridge failed; falling back to local scrape")
	}

	// 3. Local scrape — go through the bounded queue
	release, waitDur, err := h.queue.Acquire(ctx)
	if err != nil {
		return nil, waitDur, err // includes ErrQueueFull
	}
	defer func() {
		release()
		h.queue.Complete()
	}()

	if waitDur > 0 {
		h.log.Debug().Dur("waited", waitDur).Int("depth", h.queue.Depth()).Msg("queue wait done")
	}

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

// parseQuery reads and validates query parameters from the request.
func parseQuery(c *fiber.Ctx) (scraper.SearchQuery, error) {
	q := scraper.SearchQuery{
		Name:               strings.TrimSpace(c.Query("name")),
		RegistrationNumber: strings.TrimSpace(c.Query("registrationNumber")),
		Owner:              strings.TrimSpace(c.Query("owner")),
		Status:             strings.ToLower(strings.TrimSpace(c.Query("status"))),
		MarkType:           strings.ToLower(strings.TrimSpace(c.Query("markType"))),
		DateFrom:           strings.TrimSpace(c.Query("dateFrom")),
		DateTo:             strings.TrimSpace(c.Query("dateTo")),
	}

	if mr := c.Query("maxResults"); mr != "" {
		n, err := strconv.Atoi(mr)
		if err != nil || n < 1 || n > 200 {
			return q, errors.New("maxResults must be 1–200")
		}
		q.MaxResults = n
	}

	if cls := strings.TrimSpace(c.Query("class")); cls != "" {
		for _, part := range strings.Split(cls, ",") {
			n, err := strconv.Atoi(strings.TrimSpace(part))
			if err != nil || n < 1 || n > 45 {
				return q, fmt.Errorf("invalid class %q: must be integer 1–45", part)
			}
			q.Classes = append(q.Classes, n)
		}
	}

	// Validate date formats
	for _, d := range []string{q.DateFrom, q.DateTo} {
		if d != "" {
			if _, err := time.Parse("2006-01-02", d); err != nil {
				return q, fmt.Errorf("date %q must be YYYY-MM-DD", d)
			}
		}
	}

	return q, nil
}

func (h *TrademarkHandler) cacheGet(ctx context.Context, key string, dst any) bool {
	if h.cache == nil {
		return false
	}
	if err := h.cache.GetJSON(ctx, key, dst); err == nil {
		return true
	} else if !errors.Is(err, cache.ErrMiss) {
		h.log.Warn().Err(err).Str("key", key).Msg("cache get failed")
	}
	return false
}

func (h *TrademarkHandler) cacheSet(ctx context.Context, key string, val any, ttl time.Duration) {
	if h.cache == nil {
		return
	}
	if err := h.cache.SetJSON(ctx, key, val, ttl); err != nil {
		h.log.Warn().Err(err).Str("key", key).Msg("cache set failed")
	}
}
