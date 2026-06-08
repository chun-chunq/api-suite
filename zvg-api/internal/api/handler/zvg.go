package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"encoding/json"
	"strconv"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"github.com/zvg-api/internal/analytics"
	"github.com/zvg-api/internal/cache"
	"github.com/zvg-api/internal/jobqueue"
	"github.com/zvg-api/internal/pool"
	"github.com/zvg-api/internal/scraper"
	"github.com/zvg-api/internal/scrapequeue"
)

// ZVGHandler serves the public ZVG foreclosure endpoints.
type ZVGHandler struct {
	cache     *cache.Cache
	log       zerolog.Logger
	cacheTTL  time.Duration
	chromeBin string
	pool      *pool.Pool
	jq        *jobqueue.Queue
	queue     *scrapequeue.Queue
	analytics *analytics.Analytics
}

func NewZVGHandler(c *cache.Cache, log zerolog.Logger, cacheTTL time.Duration, chromeBin string, p *pool.Pool, jq *jobqueue.Queue, q *scrapequeue.Queue, an *analytics.Analytics) *ZVGHandler {
	return &ZVGHandler{cache: c, log: log, cacheTTL: cacheTTL, chromeBin: chromeBin, pool: p, jq: jq, queue: q, analytics: an}
}

// Search handles GET /v1/zvg/search
//
// Query params:
//
//	state       — Bundesland abbreviation (by, nw, be, …)
//	courtId     — Amtsgericht ID (D2601 = München)
//	procedureType — Verfahrensart: "", 0..8, -1
//	objectType  — comma-separated Objektart IDs (1=Reihenhaus, 3=Einfamilienhaus, 5=ETW …)
//	postalCode  — PLZ
//	city        — Ort
//	street      — Straße
//	caseNumber  — Aktenzeichen
//	objectText  — free-text object search
//	sortBy      — 2=Termin (default), 1=Aktualisierung, 3=Aktenzeichen
func (h *ZVGHandler) Search(c *fiber.Ctx) error {
	q := scraper.SearchQuery{
		State:         strings.TrimSpace(c.Query("state")),
		CourtID:       strings.TrimSpace(c.Query("courtId")),
		ProcedureType: strings.TrimSpace(c.Query("procedureType")),
		PostalCode:    strings.TrimSpace(c.Query("postalCode")),
		City:          strings.TrimSpace(c.Query("city")),
		Street:        strings.TrimSpace(c.Query("street")),
		CaseNumber:    strings.TrimSpace(c.Query("caseNumber")),
		ObjectText:    strings.TrimSpace(c.Query("objectText")),
		SortBy:        strings.TrimSpace(c.Query("sortBy")),
	}
	if ot := strings.TrimSpace(c.Query("objectType")); ot != "" {
		for _, v := range strings.Split(ot, ",") {
			if v = strings.TrimSpace(v); v != "" {
				q.ObjectTypes = append(q.ObjectTypes, v)
			}
		}
	}

	if q.State == "" && q.PostalCode == "" && q.City == "" && q.CaseNumber == "" && q.CourtID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "at least one filter required: state, courtId, postalCode, city, or caseNumber",
		})
	}

	cacheKey := fmt.Sprintf("zvg:search:%s:%s:%s:%s:%s:%s:%s:%s:%s",
		q.State, q.CourtID, q.ProcedureType,
		strings.Join(q.ObjectTypes, ","),
		q.PostalCode, q.City, q.Street, q.CaseNumber, q.SortBy)

	var result scraper.SearchResult
	apiKey := c.Get("X-API-Key")
	if apiKey == "" {
		apiKey = c.Get("X-RapidAPI-Proxy-Secret")
	}

	if h.cacheGet(c.Context(), cacheKey, &result) {
		h.analytics.Record(analytics.Record{
			Time: time.Now().UnixMilli(), Endpoint: "/v1/zvg/search",
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
			Time: start.UnixMilli(), Endpoint: "/v1/zvg/search",
			StatusCode: 502, LatencyMs: latMs, Scrape: true, Error: true, APIKey: apiKey,
		})
		h.log.Error().Err(err).Msg("zvg search failed")
		if errors.Is(err, scrapequeue.ErrQueueFull) {
			h.analytics.RecordQueueRejected()
			retry := h.queue.RetryAfterSeconds(60)
			c.Set("Retry-After", strconv.Itoa(retry))
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":             "scrape queue full — too many concurrent requests",
				"retryAfterSeconds": retry,
			})
		}
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "ZVG portal error", "details": err.Error(),
		})
	}

	h.analytics.Record(analytics.Record{
		Time: start.UnixMilli(), Endpoint: "/v1/zvg/search",
		StatusCode: 200, LatencyMs: latMs, QueueWaitMs: waitDur.Milliseconds(), Scrape: true, APIKey: apiKey,
	})
	h.cacheSet(c.Context(), cacheKey, res, h.cacheTTL)
	c.Set("X-Cache", "MISS")
	c.Set("X-Queue-Wait-Ms", strconv.FormatInt(waitDur.Milliseconds(), 10))
	return c.JSON(res)
}

// Courts handles GET /v1/zvg/courts?state=by — returns Amtsgericht list for a Bundesland.
func (h *ZVGHandler) Courts(c *fiber.Ctx) error {
	state := strings.ToLower(strings.TrimSpace(c.Query("state")))
	if state == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "state parameter required"})
	}

	cacheKey := "zvg:courts:" + state
	var courts any
	if h.cacheGet(c.Context(), cacheKey, &courts) {
		c.Set("X-Cache", "HIT")
		return c.JSON(courts)
	}

	var list []map[string]string
	if h.pool != nil && h.pool.HasWorkers() {
		payload := map[string]string{"state": state}
		if err := h.pool.Dispatch(c.Context(), "/scrape/zvg/courts", payload, &list); err != nil {
			h.log.Warn().Err(err).Msg("all workers failed for courts; using local scrape")
			list = nil
		}
	}
	if list == nil {
		sc, err := scraper.New(scraper.Options{Logger: h.log, BrowserBin: h.chromeBin})
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
		}
		defer sc.Close()
		var err2 error
		list, err2 = sc.GetCourts(c.Context(), state)
		if err2 != nil {
			return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err2.Error()})
		}
	}

	h.cacheSet(c.Context(), cacheKey, list, 24*time.Hour)
	c.Set("X-Cache", "MISS")
	return c.JSON(list)
}

func (h *ZVGHandler) runSearch(ctx context.Context, q scraper.SearchQuery) (*scraper.SearchResult, time.Duration, error) {
	// 1. Push-workers (IP rotation pool).
	if h.pool != nil && h.pool.HasWorkers() {
		var result scraper.SearchResult
		if err := h.pool.Dispatch(ctx, "/scrape/zvg", q, &result); err == nil {
			return &result, 0, nil
		}
		h.log.Warn().Msg("all push-workers failed; trying PC-worker bridge")
	}

	// 2. Home-PC pull-worker bridge.
	if h.jq != nil && h.jq.HasWorkers() {
		raw, err := h.jq.Dispatch(ctx, "zvg", q)
		if err == nil {
			var result scraper.SearchResult
			if jsonErr := json.Unmarshal(raw, &result); jsonErr == nil {
				return &result, 0, nil
			}
		}
		h.log.Warn().Err(err).Msg("PC-worker bridge failed; falling back to local scrape")
	}

	// 3. Local scrape — go through bounded queue.
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

func (h *ZVGHandler) cacheGet(ctx context.Context, key string, dst any) bool {
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

func (h *ZVGHandler) cacheSet(ctx context.Context, key string, val any, ttl time.Duration) {
	if h.cache == nil {
		return
	}
	if err := h.cache.SetJSON(ctx, key, val, ttl); err != nil {
		h.log.Warn().Err(err).Str("key", key).Msg("cache set failed")
	}
}
