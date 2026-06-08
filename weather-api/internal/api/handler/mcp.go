package handler

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"weather-api/internal/client"
)

type MCPHandler struct {
	client *client.Client
	log    zerolog.Logger
}

func NewMCPHandler(c *client.Client, log zerolog.Logger) *MCPHandler {
	return &MCPHandler{client: c, log: log}
}

var mcpTools = []fiber.Map{
	{
		"name":        "get_current_weather",
		"description": "Get current weather conditions at a geographic coordinate (temperature, humidity, wind, precipitation, cloud cover, pressure).",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"latitude":  fiber.Map{"type": "number", "description": "Latitude (-90 to 90)"},
				"longitude": fiber.Map{"type": "number", "description": "Longitude (-180 to 180)"},
				"timezone":  fiber.Map{"type": "string", "description": "IANA timezone (e.g. Europe/Berlin). Defaults to 'auto'."},
			},
			"required": []string{"latitude", "longitude"},
		},
	},
	{
		"name":        "get_weather_forecast",
		"description": "Get weather forecast for up to 16 days including daily highs/lows, precipitation probability, UV index and hourly breakdown.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"latitude":     fiber.Map{"type": "number", "description": "Latitude (-90 to 90)"},
				"longitude":    fiber.Map{"type": "number", "description": "Longitude (-180 to 180)"},
				"timezone":     fiber.Map{"type": "string", "description": "IANA timezone. Defaults to 'auto'."},
				"days":         fiber.Map{"type": "integer", "description": "Forecast days 1-16. Default 7."},
				"hourly_hours": fiber.Map{"type": "integer", "description": "Number of hourly points to include (0-168). Default 24. Set 0 to skip hourly."},
			},
			"required": []string{"latitude", "longitude"},
		},
	},
	{
		"name":        "geocode_location",
		"description": "Convert a city/place name to latitude, longitude, and timezone. Use results to call the weather endpoints.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"name":        fiber.Map{"type": "string", "description": "City or place name (e.g. 'Berlin', 'New York', 'Tokyo')"},
				"max_results": fiber.Map{"type": "integer", "description": "Max results to return (1-20). Default 5."},
			},
			"required": []string{"name"},
		},
	},
}

func (h *MCPHandler) Handle(c *fiber.Ctx) error {
	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      interface{}     `json:"id"`
		Method  string          `json:"method"`
		Params  fiber.Map       `json:"params"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(mcpError(nil, -32700, "Parse error", err.Error()))
	}

	switch req.Method {
	case "initialize":
		return c.JSON(fiber.Map{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": fiber.Map{
				"protocolVersion": "2024-11-05",
				"serverInfo":      fiber.Map{"name": "weather-api", "version": "1.0.0"},
				"capabilities":    fiber.Map{"tools": fiber.Map{}},
			},
		})

	case "tools/list":
		return c.JSON(fiber.Map{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  fiber.Map{"tools": mcpTools},
		})

	case "tools/call":
		return h.callTool(c, req.ID, req.Params)

	default:
		return c.Status(400).JSON(mcpError(req.ID, -32601, "Method not found", req.Method))
	}
}

func (h *MCPHandler) callTool(c *fiber.Ctx, id interface{}, params fiber.Map) error {
	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]interface{})
	if args == nil {
		args = fiber.Map{}
	}

	switch name {
	case "get_current_weather":
		lat, lon, tz, err := parseArgs(args)
		if err != nil {
			return c.JSON(mcpError(id, -32602, "Invalid params", err.Error()))
		}
		result, err := h.client.GetCurrent(c.Context(), lat, lon, tz)
		if err != nil {
			return c.JSON(mcpToolError(id, err.Error()))
		}
		return c.JSON(mcpResult(id, fmt.Sprintf(
			"Current weather at (%.4f, %.4f) [%s]\nTime: %s\nTemperature: %.1f°C (feels like %.1f°C)\nHumidity: %d%%\nWind: %.1f km/h @ %d°\nConditions: %s\nPrecipitation: %.1f mm\nCloud cover: %d%%\nPressure: %.1f hPa",
			result.Latitude, result.Longitude, result.Timezone,
			result.Time.Format("2006-01-02 15:04"),
			result.Temperature, result.FeelsLike,
			result.Humidity,
			result.WindSpeed, result.WindDirection,
			result.WeatherDesc,
			result.Precipitation,
			result.CloudCover,
			result.Pressure,
		)))

	case "get_weather_forecast":
		lat, lon, tz, err := parseArgs(args)
		if err != nil {
			return c.JSON(mcpError(id, -32602, "Invalid params", err.Error()))
		}
		days := intArg(args, "days", 7)
		hourlyHours := intArg(args, "hourly_hours", 24)
		result, err := h.client.GetForecast(c.Context(), lat, lon, tz, days, hourlyHours)
		if err != nil {
			return c.JSON(mcpToolError(id, err.Error()))
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "Weather forecast for (%.4f, %.4f) [%s]\n\n", result.Latitude, result.Longitude, result.Timezone)
		if result.Current != nil {
			fmt.Fprintf(&sb, "Now: %.1f°C, %s\n\n", result.Current.Temperature, result.Current.WeatherDesc)
		}
		fmt.Fprintf(&sb, "Daily Forecast:\n")
		for _, d := range result.Daily {
			fmt.Fprintf(&sb, "  %s: %.1f/%.1f°C, %s, precip %.1f mm (%d%% prob), UV max %.1f\n",
				d.Date, d.TempMax, d.TempMin, d.WeatherDesc, d.PrecipSum, d.PrecipProbMax, d.UVIndexMax)
		}
		if len(result.Hourly) > 0 {
			fmt.Fprintf(&sb, "\nNext %d hours:\n", len(result.Hourly))
			for _, h := range result.Hourly {
				fmt.Fprintf(&sb, "  %s: %.1f°C, %s, precip %d%%\n",
					h.Time.Format("01-02 15:04"), h.Temperature, h.WeatherDesc, h.PrecipProb)
			}
		}
		return c.JSON(mcpResult(id, sb.String()))

	case "geocode_location":
		nameArg, _ := args["name"].(string)
		if nameArg == "" {
			return c.JSON(mcpError(id, -32602, "Invalid params", "name is required"))
		}
		maxResults := intArg(args, "max_results", 5)
		locs, err := h.client.SearchLocations(c.Context(), nameArg, maxResults)
		if err != nil {
			return c.JSON(mcpToolError(id, err.Error()))
		}
		if len(locs) == 0 {
			return c.JSON(mcpResult(id, fmt.Sprintf("No locations found for %q", nameArg)))
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Locations matching %q:\n", nameArg)
		for i, l := range locs {
			admin := ""
			if l.Admin1 != "" {
				admin = ", " + l.Admin1
			}
			fmt.Fprintf(&sb, "  %d. %s%s, %s (%s) — lat: %.4f, lon: %.4f, tz: %s\n",
				i+1, l.Name, admin, l.Country, l.CountryCode, l.Latitude, l.Longitude, l.Timezone)
		}
		return c.JSON(mcpResult(id, sb.String()))

	default:
		return c.JSON(mcpError(id, -32601, "Unknown tool", name))
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func parseArgs(args map[string]interface{}) (lat, lon float64, tz string, err error) {
	latV, ok1 := args["latitude"]
	lonV, ok2 := args["longitude"]
	if !ok1 || !ok2 {
		return 0, 0, "", fmt.Errorf("latitude and longitude are required")
	}
	lat, err = toFloat(latV)
	if err != nil {
		return 0, 0, "", fmt.Errorf("invalid latitude: %w", err)
	}
	lon, err = toFloat(lonV)
	if err != nil {
		return 0, 0, "", fmt.Errorf("invalid longitude: %w", err)
	}
	if tzV, ok := args["timezone"].(string); ok {
		tz = tzV
	}
	if tz == "" {
		tz = "auto"
	}
	return lat, lon, tz, nil
}

func toFloat(v interface{}) (float64, error) {
	switch x := v.(type) {
	case float64:
		return x, nil
	case float32:
		return float64(x), nil
	case int:
		return float64(x), nil
	case string:
		return strconv.ParseFloat(x, 64)
	}
	return 0, fmt.Errorf("cannot convert %T to float64", v)
}

func intArg(args map[string]interface{}, key string, def int) int {
	v, ok := args[key]
	if !ok {
		return def
	}
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	}
	return def
}

func mcpResult(id interface{}, text string) fiber.Map {
	return fiber.Map{
		"jsonrpc": "2.0",
		"id":      id,
		"result": fiber.Map{
			"content": []fiber.Map{{"type": "text", "text": text}},
		},
	}
}

func mcpError(id interface{}, code int, msg, data string) fiber.Map {
	return fiber.Map{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   fiber.Map{"code": code, "message": msg, "data": data},
	}
}

func mcpToolError(id interface{}, msg string) fiber.Map {
	return fiber.Map{
		"jsonrpc": "2.0",
		"id":      id,
		"result": fiber.Map{
			"content":  []fiber.Map{{"type": "text", "text": msg}},
			"isError":  true,
		},
	}
}
