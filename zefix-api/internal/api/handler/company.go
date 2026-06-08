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
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"github.com/zefix-api/internal/zefix"
)

// CompanyHandler serves Swiss company endpoints.
type CompanyHandler struct {
	client   *zefix.Client
	rdb      *redis.Client
	cacheTTL time.Duration
	log      zerolog.Logger
}

func NewCompanyHandler(client *zefix.Client, rdb *redis.Client, cacheTTL time.Duration, log zerolog.Logger) *CompanyHandler {
	return &CompanyHandler{client: client, rdb: rdb, cacheTTL: cacheTTL, log: log}
}

// Search handles GET /v1/ch/company/search
//
// Query params:
//   name       — company name (required)
//   lang       — de | fr | it | en (default: de)
//   activeOnly — true | false (default: false)
//   maxResults — 1–200 (default 50)
func (h *CompanyHandler) Search(c *fiber.Ctx) error {
	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "parameter 'name' is required",
		})
	}
	lang := strings.ToLower(strings.TrimSpace(c.Query("lang")))
	if lang == "" {
		lang = "de"
	}
	activeOnly := c.Query("activeOnly") == "true"
	maxResults := 50
	if mr := c.Query("maxResults"); mr != "" {
		n, err := strconv.Atoi(mr)
		if err != nil || n < 1 || n > 200 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "maxResults must be 1–200",
			})
		}
		maxResults = n
	}

	cacheKey := fmt.Sprintf("zefix:search:%s:%s:%v:%d", name, lang, activeOnly, maxResults)
	if cached := h.cacheGet(c.Context(), cacheKey); cached != nil {
		c.Set("X-Cache", "HIT")
		return c.JSON(cached)
	}

	companies, err := h.client.Search(c.Context(), name, lang, activeOnly, maxResults)
	if err != nil {
		h.log.Error().Err(err).Str("name", name).Msg("Zefix search failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "Zefix API error",
			"details": err.Error(),
		})
	}

	result := fiber.Map{
		"total":      len(companies),
		"results":    companies,
		"query":      name,
		"dataSource": "Zefix (Swiss Federal Office of Justice) — official Swiss commercial register",
	}
	h.cacheSet(c.Context(), cacheKey, result)
	c.Set("X-Cache", "MISS")
	return c.JSON(result)
}

// GetByUID handles GET /v1/ch/company/:uid
func (h *CompanyHandler) GetByUID(c *fiber.Ctx) error {
	uid := strings.TrimSpace(c.Params("uid"))
	if uid == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "UID required"})
	}

	cacheKey := "zefix:uid:" + uid
	if cached := h.cacheGet(c.Context(), cacheKey); cached != nil {
		c.Set("X-Cache", "HIT")
		return c.JSON(cached)
	}

	company, err := h.client.GetByUID(c.Context(), uid)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	if company == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "company not found",
			"uid":   uid,
		})
	}

	h.cacheSet(c.Context(), cacheKey, company)
	c.Set("X-Cache", "MISS")
	return c.JSON(company)
}

// GetPublications handles GET /v1/ch/company/:uid/publications
func (h *CompanyHandler) GetPublications(c *fiber.Ctx) error {
	uid := strings.TrimSpace(c.Params("uid"))
	if uid == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "UID required"})
	}

	cacheKey := "zefix:shab:" + uid
	if cached := h.cacheGet(c.Context(), cacheKey); cached != nil {
		c.Set("X-Cache", "HIT")
		return c.JSON(cached)
	}

	pubs, err := h.client.GetPublications(c.Context(), uid)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}

	result := fiber.Map{
		"uid":          uid,
		"total":        len(pubs),
		"publications": pubs,
		"dataSource":   "SHAB (Swiss Federal Gazette)",
	}
	h.cacheSet(c.Context(), cacheKey, result)
	c.Set("X-Cache", "MISS")
	return c.JSON(result)
}

func (h *CompanyHandler) cacheGet(ctx context.Context, key string) any {
	if h.rdb == nil {
		return nil
	}
	val, err := h.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			h.log.Warn().Err(err).Msg("cache get failed")
		}
		return nil
	}
	var result any
	if err := json.Unmarshal(val, &result); err != nil {
		return nil
	}
	return result
}

func (h *CompanyHandler) cacheSet(ctx context.Context, key string, val any) {
	if h.rdb == nil {
		return
	}
	data, err := json.Marshal(val)
	if err != nil {
		return
	}
	if err := h.rdb.Set(ctx, key, data, h.cacheTTL).Err(); err != nil {
		h.log.Warn().Err(err).Msg("cache set failed")
	}
}
