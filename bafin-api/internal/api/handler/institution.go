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

	"github.com/bafin-api/internal/cache"
	"github.com/bafin-api/internal/scraper"
	"github.com/bafin-api/internal/scrapequeue"
)

type InstitutionHandler struct {
	cache     *cache.Cache
	log       zerolog.Logger
	cacheTTL  time.Duration
	chromeBin string
	queue     *scrapequeue.Queue
}

func NewInstitutionHandler(c *cache.Cache, log zerolog.Logger, ttl time.Duration, chromeBin string, q *scrapequeue.Queue) *InstitutionHandler {
	return &InstitutionHandler{cache: c, log: log, cacheTTL: ttl, chromeBin: chromeBin, queue: q}
}

// Search handles GET /v1/bafin/search
//
// Query params:
//   name        — institution name (partial match)
//   licenseType — bank | investmentfirm | paymentinstitution | emoneyinstitution |
//                 cryptoassets | insurance | fundmanager | broker
//   status      — active | revoked | withdrawn
//   maxResults  — 1–200 (default 50)
func (h *InstitutionHandler) Search(c *fiber.Ctx) error {
	q := scraper.SearchQuery{
		Name:         strings.TrimSpace(c.Query("name")),
		LicenseType:  strings.ToLower(strings.TrimSpace(c.Query("licenseType"))),
		StatusFilter: strings.ToLower(strings.TrimSpace(c.Query("status"))),
	}
	if mr := c.Query("maxResults"); mr != "" {
		n, err := strconv.Atoi(mr)
		if err != nil || n < 1 || n > 200 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "maxResults must be 1–200",
			})
		}
		q.MaxResults = n
	}
	if q.MaxResults == 0 {
		q.MaxResults = 50
	}

	if q.Name == "" && q.LicenseType == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "at least one parameter required: name or licenseType",
			"validLicenseTypes": []string{
				"bank", "investmentfirm", "paymentinstitution", "emoneyinstitution",
				"cryptoassets", "insurance", "fundmanager", "broker",
			},
		})
	}

	cacheKey := fmt.Sprintf("bafin:search:%s:%s:%s:%d", q.Name, q.LicenseType, q.StatusFilter, q.MaxResults)
	var cached scraper.SearchResult
	if h.cache != nil {
		if err := h.cache.GetJSON(c.Context(), cacheKey, &cached); err == nil {
			c.Set("X-Cache", "HIT")
			return c.JSON(cached)
		}
	}

	result, waitDur, err := h.runSearch(c.Context(), q)
	if err != nil {
		if errors.Is(err, scrapequeue.ErrQueueFull) {
			retry := h.queue.RetryAfterSeconds(60)
			c.Set("Retry-After", strconv.Itoa(retry))
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":             "scrape queue full — too many concurrent requests",
				"retryAfterSeconds": retry,
			})
		}
		h.log.Error().Err(err).Msg("BaFin search failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":   "BaFin portal error",
			"details": err.Error(),
		})
	}

	if h.cache != nil {
		_ = h.cache.SetJSON(c.Context(), cacheKey, result, h.cacheTTL)
	}
	c.Set("X-Cache", "MISS")
	c.Set("X-Queue-Wait-Ms", strconv.FormatInt(waitDur.Milliseconds(), 10))
	return c.JSON(result)
}

// LicenseTypes handles GET /v1/bafin/license-types — list valid license type values.
func (h *InstitutionHandler) LicenseTypes(c *fiber.Ctx) error {
	types := []map[string]string{
		{"key": "bank", "label": "CRR-Kreditinstitut (Bank)", "description": "Licensed credit institutions under KWG §32"},
		{"key": "investmentfirm", "label": "Wertpapierinstitut (Investment Firm)", "description": "Licensed under WpIG §15"},
		{"key": "paymentinstitution", "label": "Zahlungsinstitut (Payment Institution)", "description": "Licensed under ZAG §10 — includes PayPal, Klarna-type firms"},
		{"key": "emoneyinstitution", "label": "E-Geld-Institut (E-Money Institution)", "description": "Licensed electronic money issuers"},
		{"key": "cryptoassets", "label": "Kryptowertedienstleister (Crypto Asset Service Provider)", "description": "Licensed under MiCA / KWG — exchanges, custodians"},
		{"key": "insurance", "label": "Versicherungsunternehmen (Insurance Company)", "description": "BaFin-supervised insurance providers"},
		{"key": "fundmanager", "label": "Kapitalverwaltungsgesellschaft (Fund Manager / AIFM)", "description": "Licensed fund managers under KAGB"},
		{"key": "broker", "label": "Finanzdienstleistungsinstitut (Financial Services Institution)", "description": "Brokers, advisors, intermediaries under KWG"},
	}
	return c.JSON(fiber.Map{
		"licenseTypes": types,
		"hint":         "Use the 'key' value as the licenseType parameter in /v1/bafin/search",
	})
}

func (h *InstitutionHandler) runSearch(ctx context.Context, q scraper.SearchQuery) (*scraper.SearchResult, time.Duration, error) {
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
	result, err := sc.Search(cctx, q)
	return result, waitDur, err
}
