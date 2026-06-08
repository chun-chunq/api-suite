package handler

import (
	"strconv"
	"time"

	"aviation-api/internal/client"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

type AviationHandler struct {
	client *client.Client
	log    zerolog.Logger
}

func NewAviationHandler(c *client.Client, log zerolog.Logger) *AviationHandler {
	return &AviationHandler{client: c, log: log}
}

// GetStates handles GET /v1/aviation/states
//
//	?minLat=&maxLat=&minLon=&maxLon=  bounding box (optional)
func (h *AviationHandler) GetStates(c *fiber.Ctx) error {
	var box *client.BoundingBox
	if c.Query("minLat") != "" {
		minLat, e1 := strconv.ParseFloat(c.Query("minLat"), 64)
		maxLat, e2 := strconv.ParseFloat(c.Query("maxLat"), 64)
		minLon, e3 := strconv.ParseFloat(c.Query("minLon"), 64)
		maxLon, e4 := strconv.ParseFloat(c.Query("maxLon"), 64)
		if e1 == nil && e2 == nil && e3 == nil && e4 == nil {
			box = &client.BoundingBox{MinLat: minLat, MaxLat: maxLat, MinLon: minLon, MaxLon: maxLon}
		}
	}

	aircraft, ts, err := h.client.GetAllStates(c.Context(), box)
	if err != nil {
		h.log.Error().Err(err).Msg("OpenSky states failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "OpenSky data fetch failed: " + err.Error()})
	}

	// Limit response to 500 aircraft to avoid huge payloads
	if len(aircraft) > 500 {
		aircraft = aircraft[:500]
	}

	return c.JSON(fiber.Map{
		"timestamp":  ts,
		"count":      len(aircraft),
		"aircraft":   aircraft,
		"dataSource": "OpenSky Network (opensky-network.org)",
	})
}

// GetAircraft handles GET /v1/aviation/aircraft/:icao24
func (h *AviationHandler) GetAircraft(c *fiber.Ctx) error {
	icao24 := c.Params("icao24")
	a, err := h.client.GetAircraftByICAO(c.Context(), icao24)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": err.Error()})
	}
	if a == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "aircraft not found or not currently tracked"})
	}
	return c.JSON(a)
}

// GetFlights handles GET /v1/aviation/flights/:icao24?days=7
func (h *AviationHandler) GetFlights(c *fiber.Ctx) error {
	icao24 := c.Params("icao24")
	days := 1
	if d := c.Query("days"); d != "" {
		if n, err := strconv.Atoi(d); err == nil && n > 0 && n <= 30 {
			days = n
		}
	}

	now := time.Now().Unix()
	begin := now - int64(days)*86400

	flights, err := h.client.GetFlightsByAircraft(c.Context(), icao24, begin, now)
	if err != nil {
		h.log.Error().Err(err).Str("icao24", icao24).Msg("flight history failed")
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{"error": "flight history fetch failed"})
	}

	return c.JSON(fiber.Map{
		"icao24":     icao24,
		"daysBack":   days,
		"count":      len(flights),
		"flights":    flights,
		"dataSource": "OpenSky Network",
	})
}
