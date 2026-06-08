package handler

import (
	"strconv"
	"strings"

	"euipo-api/internal/client"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

// TrademarkHandler handles trademark search/lookup endpoints.
type TrademarkHandler struct {
	client *client.Client
	log    zerolog.Logger
}

// NewTrademarkHandler creates a new TrademarkHandler.
func NewTrademarkHandler(c *client.Client, log zerolog.Logger) *TrademarkHandler {
	return &TrademarkHandler{client: c, log: log}
}

// Search handles GET /v1/trademark/search
//
//	?q=         trademark name query (required unless holder is provided)
//	&holder=    applicant/holder name
//	&territory= comma-separated office codes: EM,DE,FR,GB,US ... (default: all)
//	&class=     comma-separated Nice classes: 9,25,35
//	&status=    REGISTERED | PENDING | EXPIRED (default: all)
//	&limit=     1–100 (default: 25)
//	&offset=    pagination offset (default: 0)
func (h *TrademarkHandler) Search(c *fiber.Ctx) error {
	q := c.Query("q")
	holder := c.Query("holder")
	if q == "" && holder == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "provide at least q (trademark name) or holder parameter",
		})
	}

	// territories
	var territories []string
	if t := c.Query("territory"); t != "" {
		for _, tc := range strings.Split(t, ",") {
			tc = strings.TrimSpace(strings.ToUpper(tc))
			if tc != "" {
				territories = append(territories, tc)
			}
		}
	}

	// Nice classes
	var classes []int
	if cl := c.Query("class"); cl != "" {
		for _, s := range strings.Split(cl, ",") {
			s = strings.TrimSpace(s)
			if n, err := strconv.Atoi(s); err == nil && n >= 1 && n <= 45 {
				classes = append(classes, n)
			}
		}
	}

	limit := 25
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	offset := 0
	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	query := client.SearchQuery{
		Query:       q,
		Holder:      holder,
		Territories: territories,
		Classes:     classes,
		Status:      strings.ToUpper(c.Query("status")),
		MaxResults:  limit,
		Offset:      offset,
	}

	result, err := h.client.Search(c.Context(), query)
	if err != nil {
		h.log.Error().Err(err).Str("q", q).Msg("trademark search failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "upstream TMview search failed",
		})
	}

	return c.JSON(fiber.Map{
		"total":      result.Total,
		"offset":     result.Offset,
		"count":      len(result.Results),
		"results":    result.Results,
		"dataSource": "TMview / EUIPO",
	})
}

// GetByID handles GET /v1/trademark/:office/:appNum
// office: EM, DE, FR, GB, ...
// appNum: application number
func (h *TrademarkHandler) GetByID(c *fiber.Ctx) error {
	office := strings.ToUpper(strings.TrimSpace(c.Params("office")))
	appNum := strings.TrimSpace(c.Params("appNum"))

	if office == "" || appNum == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "office and appNum path parameters are required",
		})
	}

	tm, err := h.client.GetByID(c.Context(), office, appNum)
	if err != nil {
		h.log.Error().Err(err).Str("office", office).Str("appNum", appNum).Msg("trademark lookup failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "upstream TMview lookup failed",
		})
	}
	if tm == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "trademark not found",
		})
	}

	return c.JSON(tm)
}
