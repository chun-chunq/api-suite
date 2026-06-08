package handler

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"namepredict-api/internal/client"
)

// PredictAll handles GET /v1/name/predict?name=Emma&country=DE
// Returns age, gender, and nationality predictions concurrently.
func PredictAll(c *fiber.Ctx, cl *client.Client) error {
	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "query parameter 'name' is required",
		})
	}
	countryID := strings.ToUpper(strings.TrimSpace(c.Query("country", "")))

	result, err := cl.GetAll(c.Context(), name, countryID)
	if err != nil {
		if strings.Contains(err.Error(), "rate_limit") {
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "upstream error: " + err.Error(),
		})
	}
	return c.JSON(result)
}

// PredictAge handles GET /v1/name/age?name=Michael&country=DE
func PredictAge(c *fiber.Ctx, cl *client.Client) error {
	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "query parameter 'name' is required",
		})
	}
	countryID := strings.ToUpper(strings.TrimSpace(c.Query("country", "")))

	result, err := cl.GetAge(c.Context(), name, countryID)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(result)
}

// PredictGender handles GET /v1/name/gender?name=Emma&country=DE
func PredictGender(c *fiber.Ctx, cl *client.Client) error {
	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "query parameter 'name' is required",
		})
	}
	countryID := strings.ToUpper(strings.TrimSpace(c.Query("country", "")))

	result, err := cl.GetGender(c.Context(), name, countryID)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(result)
}

// PredictNationality handles GET /v1/name/nationality?name=Zhang
func PredictNationality(c *fiber.Ctx, cl *client.Client) error {
	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "query parameter 'name' is required",
		})
	}

	result, err := cl.GetNationality(c.Context(), name)
	if err != nil {
		return handleErr(c, err)
	}
	return c.JSON(result)
}

func handleErr(c *fiber.Ctx, err error) error {
	msg := err.Error()
	if strings.Contains(msg, "rate_limit") {
		return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{"error": msg})
	}
	if strings.Contains(msg, "required") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": msg})
	}
	return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "upstream error: " + msg})
}
