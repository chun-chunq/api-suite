package handler

import (
	"strconv"

	"sec-api/internal/client"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

type EdgarHandler struct {
	client *client.Client
	log    zerolog.Logger
}

func NewEdgarHandler(c *client.Client, log zerolog.Logger) *EdgarHandler {
	return &EdgarHandler{client: c, log: log}
}

// Search handles GET /v1/sec/company/search?q=Microsoft&limit=20
func (h *EdgarHandler) Search(c *fiber.Ctx) error {
	q := c.Query("q")
	if q == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "q parameter is required"})
	}
	limit := 20
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	companies, total, err := h.client.SearchCompanies(c.Context(), q, limit)
	if err != nil {
		h.log.Error().Err(err).Str("q", q).Msg("EDGAR search failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "EDGAR search failed"})
	}
	return c.JSON(fiber.Map{
		"total":      total,
		"count":      len(companies),
		"results":    companies,
		"dataSource": "SEC EDGAR",
	})
}

// GetProfile handles GET /v1/sec/company/:cik
func (h *EdgarHandler) GetProfile(c *fiber.Ctx) error {
	cik := c.Params("cik")
	co, err := h.client.GetCompanyProfile(c.Context(), cik)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if co == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "company not found"})
	}
	return c.JSON(co)
}

// GetFilings handles GET /v1/sec/company/:cik/filings?form=10-K&limit=10
func (h *EdgarHandler) GetFilings(c *fiber.Ctx) error {
	cik := c.Params("cik")
	formType := c.Query("form")
	limit := 20
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	filings, err := h.client.GetFilings(c.Context(), cik, formType, limit)
	if err != nil {
		h.log.Error().Err(err).Str("cik", cik).Msg("EDGAR filings failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "filings fetch failed"})
	}
	if filings == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "company not found"})
	}
	return c.JSON(fiber.Map{
		"cik":        cik,
		"formFilter": formType,
		"count":      len(filings),
		"filings":    filings,
		"dataSource": "SEC EDGAR",
	})
}

// GetFinancials handles GET /v1/sec/company/:cik/financials?concept=us-gaap/NetIncomeLoss&limit=10
func (h *EdgarHandler) GetFinancials(c *fiber.Ctx) error {
	cik := c.Params("cik")
	concept := c.Query("concept")
	if concept == "" {
		concept = "us-gaap/Revenues"
	}
	limit := 20
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	facts, err := h.client.GetFinancialFacts(c.Context(), cik, concept, limit)
	if err != nil {
		h.log.Error().Err(err).Str("cik", cik).Str("concept", concept).Msg("EDGAR financials failed")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if facts == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "no data found"})
	}
	return c.JSON(fiber.Map{
		"cik":     cik,
		"concept": concept,
		"count":   len(facts),
		"facts":   facts,
		"note":    "Common concepts: us-gaap/Revenues, us-gaap/NetIncomeLoss, us-gaap/Assets, us-gaap/StockholdersEquity, us-gaap/EarningsPerShareBasic",
		"dataSource": "SEC EDGAR XBRL",
	})
}
