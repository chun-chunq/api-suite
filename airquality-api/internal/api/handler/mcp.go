// Package handler — MCP endpoint for Air Quality API.
package handler

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"airquality-api/internal/client"
)

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type mcpResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpError   `json:"error,omitempty"`
}
type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func MCP(c *fiber.Ctx, cl *client.Client) error {
	var req mcpRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32700, Message: "parse error"}})
	}

	switch req.Method {
	case "initialize":
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: fiber.Map{
			"protocolVersion": "2024-11-05",
			"capabilities":    fiber.Map{"tools": fiber.Map{}},
			"serverInfo":      fiber.Map{"name": "airquality-api-mcp", "version": "1.0.0"},
		}})

	case "tools/list":
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: fiber.Map{"tools": []fiber.Map{
			{
				"name":        "get_current_air_quality",
				"description": "Get current air quality conditions for a location: PM2.5, PM10, ozone, NO2, CO, UV index, and European/US AQI with health categories.",
				"inputSchema": fiber.Map{"type": "object", "required": []string{"latitude", "longitude"}, "properties": fiber.Map{
					"latitude":  fiber.Map{"type": "number", "description": "Latitude (-90 to 90)"},
					"longitude": fiber.Map{"type": "number", "description": "Longitude (-180 to 180)"},
					"timezone":  fiber.Map{"type": "string", "description": "Timezone (e.g. Europe/Berlin, auto)"},
				}},
			},
			{
				"name":        "get_air_quality_forecast",
				"description": "Get hourly air quality forecast (PM2.5, PM10, ozone, NO2, UV) for up to 48 hours.",
				"inputSchema": fiber.Map{"type": "object", "required": []string{"latitude", "longitude"}, "properties": fiber.Map{
					"latitude":  fiber.Map{"type": "number", "description": "Latitude (-90 to 90)"},
					"longitude": fiber.Map{"type": "number", "description": "Longitude (-180 to 180)"},
					"timezone":  fiber.Map{"type": "string", "description": "Timezone (default: auto)"},
					"hours":     fiber.Map{"type": "integer", "description": "Forecast hours (1-48, default 24)"},
				}},
			},
		}}})

	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpError{Code: -32602, Message: "invalid params"}})
		}
		return callTool(c, req.ID, params.Name, params.Arguments, cl)

	default:
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpError{Code: -32601, Message: "method not found: " + req.Method}})
	}
}

func callTool(c *fiber.Ctx, id interface{}, name string, args map[string]interface{}, cl *client.Client) error {
	ok := func(data interface{}) error {
		b, _ := json.MarshalIndent(data, "", "  ")
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: id, Result: fiber.Map{
			"content": []fiber.Map{{"type": "text", "text": string(b)}},
		}})
	}
	fail := func(msg string) error {
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: id, Error: &mcpError{Code: -32000, Message: msg}})
	}

	getLatLon := func() (float64, float64, error) {
		lat, ok1 := args["latitude"].(float64)
		lon, ok2 := args["longitude"].(float64)
		if !ok1 || !ok2 {
			return 0, 0, fmt.Errorf("latitude and longitude are required as numbers")
		}
		return lat, lon, nil
	}
	tz := func() string {
		if v, ok := args["timezone"].(string); ok {
			return strings.TrimSpace(v)
		}
		return "auto"
	}

	switch name {
	case "get_current_air_quality":
		lat, lon, err := getLatLon()
		if err != nil {
			return fail(err.Error())
		}
		result, err := cl.GetCurrent(c.Context(), lat, lon, tz())
		if err != nil {
			return fail(err.Error())
		}
		return ok(result)

	case "get_air_quality_forecast":
		lat, lon, err := getLatLon()
		if err != nil {
			return fail(err.Error())
		}
		hours := 24
		if v, ok := args["hours"].(float64); ok {
			hours = int(v)
		}
		result, err := cl.GetForecast(c.Context(), lat, lon, tz(), hours)
		if err != nil {
			return fail(err.Error())
		}
		return ok(result)

	default:
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: id, Error: &mcpError{Code: -32601, Message: "unknown tool: " + name}})
	}
}
