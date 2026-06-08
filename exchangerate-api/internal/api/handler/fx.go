// Package handler provides HTTP handlers for the Exchange Rate API.
package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"exchangerate-api/internal/client"
)

// parseSymbols splits a comma-separated currency list.
func parseSymbols(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// Latest handles GET /v1/fx/latest?base=EUR&symbols=USD,GBP,JPY
func Latest(c *fiber.Ctx, cl *client.Client) error {
	base := c.Query("base", "EUR")
	symbols := parseSymbols(c.Query("symbols"))

	result, err := cl.GetLatest(c.Context(), base, symbols)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}

// Historical handles GET /v1/fx/historical/:date?base=EUR&symbols=USD
func Historical(c *fiber.Ctx, cl *client.Client) error {
	date := c.Params("date")
	base := c.Query("base", "EUR")
	symbols := parseSymbols(c.Query("symbols"))

	result, err := cl.GetHistorical(c.Context(), date, base, symbols)
	if err != nil {
		if err.Error() == "not_found" {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "no data for date: " + date + " (data starts from 1999-01-04)",
			})
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}

// TimeSeries handles GET /v1/fx/series?start=2024-01-01&end=2024-03-31&base=EUR&symbols=USD
func TimeSeries(c *fiber.Ctx, cl *client.Client) error {
	start := strings.TrimSpace(c.Query("start"))
	end := strings.TrimSpace(c.Query("end"))

	if start == "" || end == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "start and end query parameters are required (YYYY-MM-DD)",
		})
	}

	base := c.Query("base", "EUR")
	symbols := parseSymbols(c.Query("symbols"))

	result, err := cl.GetTimeSeries(c.Context(), start, end, base, symbols)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}

// Convert handles GET /v1/fx/convert?from=USD&to=EUR&amount=100
func Convert(c *fiber.Ctx, cl *client.Client) error {
	from := strings.TrimSpace(c.Query("from"))
	to := strings.TrimSpace(c.Query("to"))
	amountStr := c.Query("amount", "1")

	if from == "" || to == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "from and to query parameters are required",
		})
	}

	amount, err := strconv.ParseFloat(amountStr, 64)
	if err != nil || amount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "amount must be a positive number",
		})
	}

	result, err := cl.Convert(c.Context(), from, to, amount)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}

// Currencies handles GET /v1/fx/currencies
func Currencies(c *fiber.Ctx, cl *client.Client) error {
	currencies, err := cl.GetCurrencies(c.Context())
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"count":      len(currencies),
		"currencies": currencies,
	})
}
