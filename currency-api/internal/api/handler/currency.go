package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"currency-api/internal/client"
)

type CurrencyHandler struct {
	client *client.Client
	log    zerolog.Logger
}

func NewCurrencyHandler(c *client.Client, log zerolog.Logger) *CurrencyHandler {
	return &CurrencyHandler{client: c, log: log}
}

// GET /v1/currency/latest?base=EUR
func (h *CurrencyHandler) GetLatest(c *fiber.Ctx) error {
	rates, err := h.client.GetLatestRates(c.Context())
	if err != nil {
		h.log.Error().Err(err).Msg("GetLatestRates failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}

	base := strings.ToUpper(strings.TrimSpace(c.Query("base", "EUR")))
	if base == "EUR" {
		return c.JSON(fiber.Map{
			"base":  "EUR",
			"date":  rates.Date,
			"rates": rates.Rates,
		})
	}

	// Rebase: convert all rates so `base` = 1
	baseRate, ok := rates.Rates[base]
	if !ok {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "unsupported base currency: " + base})
	}
	rebased := make(map[string]float64, len(rates.Rates)+1)
	rebased["EUR"] = 1.0 / baseRate
	for code, rate := range rates.Rates {
		rebased[code] = rate / baseRate
	}
	delete(rebased, base)
	return c.JSON(fiber.Map{
		"base":  base,
		"date":  rates.Date,
		"rates": rebased,
	})
}

// GET /v1/currency/convert?from=USD&to=EUR&amount=100
func (h *CurrencyHandler) Convert(c *fiber.Ctx) error {
	from := strings.TrimSpace(c.Query("from"))
	to := strings.TrimSpace(c.Query("to"))
	if from == "" || to == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "from and to are required"})
	}

	amount := 1.0
	if a := c.Query("amount"); a != "" {
		v, err := strconv.ParseFloat(a, 64)
		if err != nil || v < 0 {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "invalid amount"})
		}
		amount = v
	}

	result, err := h.client.Convert(c.Context(), from, to, amount)
	if err != nil {
		h.log.Error().Err(err).Str("from", from).Str("to", to).Msg("Convert failed")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}

// GET /v1/currency/currencies
func (h *CurrencyHandler) GetCurrencies(c *fiber.Ctx) error {
	codes, err := h.client.GetCurrencies(c.Context())
	if err != nil {
		h.log.Error().Err(err).Msg("GetCurrencies failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"currencies": codes,
		"total":      len(codes),
		"source":     "European Central Bank (ECB) Euro FX Reference Rates",
	})
}

// GET /v1/currency/history?from=USD&to=EUR&days=30
func (h *CurrencyHandler) GetHistory(c *fiber.Ctx) error {
	from := strings.TrimSpace(c.Query("from"))
	to := strings.TrimSpace(c.Query("to"))
	if from == "" || to == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "from and to are required"})
	}

	days := 30
	if d := c.Query("days"); d != "" {
		if v, e := strconv.Atoi(d); e == nil && v > 0 && v <= 90 {
			days = v
		}
	}

	points, err := h.client.GetHistory(c.Context(), from, to, days)
	if err != nil {
		h.log.Error().Err(err).Str("from", from).Str("to", to).Msg("GetHistory failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"from":    strings.ToUpper(from),
		"to":      strings.ToUpper(to),
		"history": points,
		"count":   len(points),
	})
}
