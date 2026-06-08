package handler

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"github.com/ted-api/internal/cache"
	"github.com/ted-api/internal/ted"
)

// NoticesHandler serves the TED procurement notice endpoints.
type NoticesHandler struct {
	cache    *cache.Cache
	tedClient *ted.Client
	log      zerolog.Logger
	cacheTTL time.Duration
}

func NewNoticesHandler(c *cache.Cache, log zerolog.Logger, cacheTTL time.Duration) *NoticesHandler {
	return &NoticesHandler{
		cache:     c,
		tedClient: ted.NewClient(),
		log:       log,
		cacheTTL:  cacheTTL,
	}
}

// Search handles GET /v1/ted/search
//
// Query params:
//
//	country     — ISO 3166-1 alpha-3, default DEU (Germany)
//	keyword     — full-text keyword search in notice title
//	dateFrom    — YYYY-MM-DD or YYYYMMDD
//	dateTo      — YYYY-MM-DD or YYYYMMDD
//	noticeType  — cn-standard|can-standard|pin-only|qu-sy|cn-social|can-social
//	page        — page number (default 1)
//	limit       — results per page, max 100 (default 10)
func (h *NoticesHandler) Search(c *fiber.Ctx) error {
	country    := strings.ToUpper(strings.TrimSpace(c.Query("country", "DEU")))
	keyword    := strings.TrimSpace(c.Query("keyword"))
	dateFrom   := strings.TrimSpace(c.Query("dateFrom"))
	dateTo     := strings.TrimSpace(c.Query("dateTo"))
	noticeType := strings.TrimSpace(c.Query("noticeType"))
	limit      := clampInt(c.Query("limit", "10"), 1, 100)
	page       := clampInt(c.Query("page", "1"), 1, 1000)

	query := ted.BuildQuery(country, keyword, dateFrom, dateTo, noticeType)

	cacheKey := fmt.Sprintf("ted:search:%s:%s:%s:%s:%s:%d:%d",
		country, keyword, dateFrom, dateTo, noticeType, page, limit)

	var result ted.SearchResult
	if h.cacheGet(c.Context(), cacheKey, &result) {
		c.Set("X-Cache", "HIT")
		return c.JSON(result)
	}

	ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
	defer cancel()

	raw, err := h.tedClient.Search(ctx, ted.SearchRequest{
		Query:          query,
		Limit:          limit,
		Page:           page,
		Scope:          "ACTIVE",
		PaginationMode: "PAGE_NUMBER",
	})
	if err != nil {
		h.log.Error().Err(err).Str("query", query).Msg("TED API search failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "TED API error",
			"details": err.Error(),
		})
	}

	res := ted.NormalizeResponse(raw, page, limit)
	h.cacheSet(c.Context(), cacheKey, res, h.cacheTTL)
	c.Set("X-Cache", "MISS")
	return c.JSON(res)
}

// Recent handles GET /v1/ted/recent — shortcut for last 7 days in Germany
func (h *NoticesHandler) Recent(c *fiber.Ctx) error {
	country    := strings.ToUpper(strings.TrimSpace(c.Query("country", "DEU")))
	noticeType := strings.TrimSpace(c.Query("noticeType", "cn-standard"))
	limit      := clampInt(c.Query("limit", "20"), 1, 100)
	page       := clampInt(c.Query("page", "1"), 1, 1000)

	dateFrom := time.Now().AddDate(0, 0, -7).Format("20060102")
	query := ted.BuildQuery(country, "", dateFrom, "", noticeType)

	cacheKey := fmt.Sprintf("ted:recent:%s:%s:%d:%d", country, noticeType, page, limit)

	var result ted.SearchResult
	if h.cacheGet(c.Context(), cacheKey, &result) {
		c.Set("X-Cache", "HIT")
		return c.JSON(result)
	}

	ctx, cancel := context.WithTimeout(c.Context(), 30*time.Second)
	defer cancel()

	raw, err := h.tedClient.Search(ctx, ted.SearchRequest{
		Query:          query,
		Limit:          limit,
		Page:           page,
		Scope:          "ACTIVE",
		PaginationMode: "PAGE_NUMBER",
	})
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}

	res := ted.NormalizeResponse(raw, page, limit)
	// Shorter cache for "recent" — data changes daily
	h.cacheSet(c.Context(), cacheKey, res, time.Hour)
	c.Set("X-Cache", "MISS")
	return c.JSON(res)
}

// NoticeTypes handles GET /v1/ted/notice-types — returns supported notice type codes.
func (h *NoticesHandler) NoticeTypes(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"noticeTypes": []fiber.Map{
			{"code": "cn-standard",  "label": "Auftragsbekanntmachung (Contract Notice)"},
			{"code": "can-standard", "label": "Bekanntmachung vergebener Aufträge (Contract Award Notice)"},
			{"code": "pin-only",     "label": "Vorinformation (Prior Information Notice)"},
			{"code": "qu-sy",        "label": "Qualifikationssystem (Qualification System)"},
			{"code": "cn-social",    "label": "Soziale Leistungen (Social Services Notice)"},
			{"code": "can-social",   "label": "Vergabe Soziale Leistungen (Social Services Award)"},
			{"code": "cn-desg",      "label": "Wettbewerbsbekanntmachung (Design Contest)"},
			{"code": "veat",         "label": "Freiwillige Vorabbekanntmachung (VEAT)"},
		},
	})
}

// ---- helpers ----

func (h *NoticesHandler) cacheGet(ctx context.Context, key string, dst any) bool {
	if h.cache == nil {
		return false
	}
	if err := h.cache.GetJSON(ctx, key, dst); err == nil {
		return true
	} else if !errors.Is(err, cache.ErrMiss) {
		h.log.Warn().Err(err).Str("key", key).Msg("cache get")
	}
	return false
}

func (h *NoticesHandler) cacheSet(ctx context.Context, key string, val any, ttl time.Duration) {
	if h.cache == nil {
		return
	}
	if err := h.cache.SetJSON(ctx, key, val, ttl); err != nil {
		h.log.Warn().Err(err).Str("key", key).Msg("cache set")
	}
}

func clampInt(s string, min, max int) int {
	n, err := strconv.Atoi(s)
	if err != nil || n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
