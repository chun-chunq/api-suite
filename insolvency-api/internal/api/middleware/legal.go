package middleware

import "github.com/gofiber/fiber/v2"

// LegalHeaders attaches data-usage notice headers to every response.
// Required for GDPR Art. 13/14 transparency and marketplace compliance.
func LegalHeaders() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Data source: publicly mandated under § 9 InsO (Insolvenzordnung).
		c.Set("X-Data-Source", "insolvenzbekanntmachungen.de (§9 InsO)")
		// Personal data (natural persons) processed under Art. 6(1)(f) DSGVO —
		// legitimate interest for credit risk assessment and due diligence.
		c.Set("X-Data-Legal-Basis", "DSGVO Art. 6(1)(f) — berechtigtes Interesse")
		c.Set("X-Data-Usage-Restriction", "B2B due-diligence and credit-risk only; no profiling of natural persons")
		return c.Next()
	}
}
