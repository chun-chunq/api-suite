package handler

import (
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"weather-api/internal/client"
)

type WeatherHandler struct {
	client *client.Client
	log    zerolog.Logger
}

func NewWeatherHandler(c *client.Client, log zerolog.Logger) *WeatherHandler {
	return &WeatherHandler{client: c, log: log}
}

// GET /v1/weather/current?lat=52.52&lon=13.405&timezone=Europe/Berlin
func (h *WeatherHandler) GetCurrent(c *fiber.Ctx) error {
	lat, lon, err := parseLatLon(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	timezone := strings.TrimSpace(c.Query("timezone", "auto"))

	result, err := h.client.GetCurrent(c.Context(), lat, lon, timezone)
	if err != nil {
		h.log.Error().Err(err).Float64("lat", lat).Float64("lon", lon).Msg("GetCurrent failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}

// GET /v1/weather/forecast?lat=52.52&lon=13.405&days=7&hourly=24&timezone=auto
func (h *WeatherHandler) GetForecast(c *fiber.Ctx) error {
	lat, lon, err := parseLatLon(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	timezone := strings.TrimSpace(c.Query("timezone", "auto"))

	days := 7
	if d := c.Query("days"); d != "" {
		if v, e := strconv.Atoi(d); e == nil && v > 0 && v <= 16 {
			days = v
		}
	}

	hourlyHours := 24
	if h := c.Query("hourly"); h != "" {
		if v, e := strconv.Atoi(h); e == nil {
			if v < 0 {
				v = 0
			}
			if v > 168 {
				v = 168
			}
			hourlyHours = v
		}
	}

	result, err := h.client.GetForecast(c.Context(), lat, lon, timezone, days, hourlyHours)
	if err != nil {
		h.log.Error().Err(err).Float64("lat", lat).Float64("lon", lon).Msg("GetForecast failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(result)
}

// GET /v1/weather/geocode?name=Berlin&max=5
func (h *WeatherHandler) Geocode(c *fiber.Ctx) error {
	name := strings.TrimSpace(c.Query("name"))
	if name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "name is required"})
	}
	maxResults := 5
	if m := c.Query("max"); m != "" {
		if v, e := strconv.Atoi(m); e == nil && v > 0 && v <= 20 {
			maxResults = v
		}
	}

	locs, err := h.client.SearchLocations(c.Context(), name, maxResults)
	if err != nil {
		h.log.Error().Err(err).Str("name", name).Msg("Geocode failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"results": locs,
		"total":   len(locs),
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func parseLatLon(c *fiber.Ctx) (float64, float64, error) {
	latStr := strings.TrimSpace(c.Query("lat"))
	lonStr := strings.TrimSpace(c.Query("lon"))
	if latStr == "" || lonStr == "" {
		return 0, 0, fiber.NewError(fiber.StatusBadRequest, "lat and lon are required")
	}
	lat, err := strconv.ParseFloat(latStr, 64)
	if err != nil || lat < -90 || lat > 90 {
		return 0, 0, fiber.NewError(fiber.StatusBadRequest, "lat must be a number between -90 and 90")
	}
	lon, err := strconv.ParseFloat(lonStr, 64)
	if err != nil || lon < -180 || lon > 180 {
		return 0, 0, fiber.NewError(fiber.StatusBadRequest, "lon must be a number between -180 and 180")
	}
	return lat, lon, nil
}
