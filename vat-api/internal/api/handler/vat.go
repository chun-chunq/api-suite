// Package handler provides HTTP handlers for the VAT validation API.
package handler

import (
	"sort"
	"strings"

	"github.com/gofiber/fiber/v2"
	"vat-api/internal/client"
)

// Validate handles GET /v1/vat/validate?country=DE&vat=123456789
// or with full VAT: GET /v1/vat/validate?vat=DE123456789
func Validate(c *fiber.Ctx, cl *client.Client) error {
	vatParam := strings.TrimSpace(c.Query("vat"))
	country := strings.TrimSpace(c.Query("country"))

	if vatParam == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "vat query parameter is required",
		})
	}

	// If no country provided, try to extract from the VAT number prefix
	if country == "" {
		if len(vatParam) >= 2 {
			country = vatParam[:2]
		} else {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "country parameter required when vat number has no country prefix",
			})
		}
	}

	result, err := cl.ValidateVAT(c.Context(), country, vatParam)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(result)
}

// BatchValidate handles POST /v1/vat/batch
// Body: {"vat_numbers": ["DE123456789", "FR12345678901"]}
func BatchValidate(c *fiber.Ctx, cl *client.Client) error {
	var body struct {
		VATNumbers []string `json:"vat_numbers"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "invalid JSON body",
		})
	}
	if len(body.VATNumbers) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "vat_numbers array is required and must not be empty",
		})
	}

	result, err := cl.ValidateBatch(c.Context(), body.VATNumbers)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(result)
}

// Countries handles GET /v1/vat/countries — returns all supported EU member state codes.
func Countries(c *fiber.Ctx) error {
	codes := client.ValidCountryCodes()
	sort.Strings(codes)

	countryNames := map[string]string{
		"AT": "Austria", "BE": "Belgium", "BG": "Bulgaria", "CY": "Cyprus",
		"CZ": "Czech Republic", "DE": "Germany", "DK": "Denmark", "EE": "Estonia",
		"EL": "Greece", "GR": "Greece", "ES": "Spain", "FI": "Finland",
		"FR": "France", "HR": "Croatia", "HU": "Hungary", "IE": "Ireland",
		"IT": "Italy", "LT": "Lithuania", "LU": "Luxembourg", "LV": "Latvia",
		"MT": "Malta", "NL": "Netherlands", "PL": "Poland", "PT": "Portugal",
		"RO": "Romania", "SE": "Sweden", "SI": "Slovenia", "SK": "Slovakia",
	}

	type entry struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	entries := make([]entry, 0, len(codes))
	for _, code := range codes {
		entries = append(entries, entry{Code: code, Name: countryNames[code]})
	}

	return c.JSON(fiber.Map{
		"countries": entries,
		"count":     len(entries),
		"note":      "EL is the VIES code for Greece (GR is also accepted as input)",
	})
}
