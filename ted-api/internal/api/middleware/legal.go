package middleware

import "github.com/gofiber/fiber/v2"

func LegalHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Data source: Official EU TED API, public procurement data under EU Open Data Policy
		c.Set("X-Data-Source", "api.ted.europa.eu — Official EU TED Procurement API")
		c.Set("X-Data-Legal-Basis", "EU Open Data Policy; no personal data; commercial use permitted")
		c.Set("X-Data-Attribution", "© European Union, TED — Tenders Electronic Daily")
		return c.Next()
	}
}
