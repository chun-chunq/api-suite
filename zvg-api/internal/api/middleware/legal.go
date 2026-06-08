package middleware

import "github.com/gofiber/fiber/v2"

// LegalHeaders attaches data-usage transparency headers per DSGVO Art. 13/14.
func LegalHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("X-Data-Source", "zvg-portal.de — öffentliche Behördendaten (ZVG)")
		c.Set("X-Data-Legal-Basis", "Daten gesetzlich veröffentlicht; DSGVO Art. 6(1)(f)")
		c.Set("X-Data-Usage-Restriction", "B2B use only; no consumer profiling")
		return c.Next()
	}
}
