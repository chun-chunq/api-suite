// Package handler — MCP endpoint for NASA API.
package handler

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
	"nasa-api/internal/client"
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
			"serverInfo":      fiber.Map{"name": "nasa-api-mcp", "version": "1.0.0"},
		}})

	case "tools/list":
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: fiber.Map{"tools": []fiber.Map{
			{
				"name":        "get_apod",
				"description": "Get NASA's Astronomy Picture of the Day. Returns image URL, title, and scientific explanation.",
				"inputSchema": fiber.Map{"type": "object", "properties": fiber.Map{
					"date": fiber.Map{"type": "string", "description": "Date in YYYY-MM-DD format. Omit for today's APOD."},
				}},
			},
			{
				"name":        "get_apod_range",
				"description": "Get NASA APOD entries for a date range (max 7 days).",
				"inputSchema": fiber.Map{"type": "object", "required": []string{"start", "end"}, "properties": fiber.Map{
					"start": fiber.Map{"type": "string", "description": "Start date YYYY-MM-DD"},
					"end":   fiber.Map{"type": "string", "description": "End date YYYY-MM-DD (max 7 days from start)"},
				}},
			},
			{
				"name":        "get_mars_photos",
				"description": "Get photos from NASA Mars rovers (Curiosity, Perseverance, Opportunity, Spirit).",
				"inputSchema": fiber.Map{"type": "object", "properties": fiber.Map{
					"rover":      fiber.Map{"type": "string", "description": "Rover name: curiosity, perseverance, opportunity, spirit (default: curiosity)"},
					"sol":        fiber.Map{"type": "integer", "description": "Martian sol (day number, e.g. 1000)"},
					"earth_date": fiber.Map{"type": "string", "description": "Earth date YYYY-MM-DD (alternative to sol)"},
					"camera":     fiber.Map{"type": "string", "description": "Camera abbreviation: FHAZ, RHAZ, MAST, NAVCAM, CHEMCAM"},
					"limit":      fiber.Map{"type": "integer", "description": "Max photos (1-25, default 10)"},
				}},
			},
			{
				"name":        "get_neo_feed",
				"description": "Get Near Earth Objects (asteroids) passing close to Earth in a date range (max 7 days).",
				"inputSchema": fiber.Map{"type": "object", "properties": fiber.Map{
					"start": fiber.Map{"type": "string", "description": "Start date YYYY-MM-DD (default: today)"},
					"end":   fiber.Map{"type": "string", "description": "End date YYYY-MM-DD (default: today+6)"},
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
	str := func(key string) string {
		if v, ok := args[key]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
		return ""
	}

	switch name {
	case "get_apod":
		entry, err := cl.GetAPOD(c.Context(), str("date"))
		if err != nil {
			return fail(err.Error())
		}
		return ok(entry)

	case "get_apod_range":
		start, end := str("start"), str("end")
		if start == "" || end == "" {
			return fail("start and end are required")
		}
		entries, err := cl.GetAPODRange(c.Context(), start, end)
		if err != nil {
			return fail(err.Error())
		}
		return ok(fiber.Map{"count": len(entries), "entries": entries})

	case "get_mars_photos":
		rover := str("rover")
		camera := str("camera")
		earthDate := str("earth_date")
		sol := 0
		limit := 10
		if v, ok := args["sol"]; ok {
			if f, ok := v.(float64); ok {
				sol = int(f)
			}
		}
		if v, ok := args["limit"]; ok {
			if f, ok := v.(float64); ok {
				limit = int(f)
			}
		}
		result, err := cl.GetMarsPhotos(c.Context(), rover, camera, sol, earthDate, limit)
		if err != nil {
			return fail(err.Error())
		}
		return ok(result)

	case "get_neo_feed":
		result, err := cl.GetNEOFeed(c.Context(), str("start"), str("end"))
		if err != nil {
			return fail(err.Error())
		}
		return ok(result)

	default:
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: id, Error: &mcpError{Code: -32601, Message: "unknown tool: " + name}})
	}
}
