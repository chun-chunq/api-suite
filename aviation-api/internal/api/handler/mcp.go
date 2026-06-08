package handler

import (
	"encoding/json"
	"fmt"
	"time"

	"aviation-api/internal/client"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

type MCPHandler struct {
	client *client.Client
	log    zerolog.Logger
}

func NewMCPHandler(c *client.Client, log zerolog.Logger) *MCPHandler {
	return &MCPHandler{client: c, log: log}
}

type mcpReq  struct { JSONRPC string `json:"jsonrpc"`; ID interface{} `json:"id"`; Method string `json:"method"`; Params json.RawMessage `json:"params"` }
type mcpResp struct { JSONRPC string `json:"jsonrpc"`; ID interface{} `json:"id"`; Result interface{} `json:"result,omitempty"`; Error *mcpErr `json:"error,omitempty"` }
type mcpErr  struct { Code int `json:"code"`; Message string `json:"message"` }

var mcpTools = []fiber.Map{
	{
		"name": "get_live_flights",
		"description": "Get live aircraft positions currently in the air. Optionally filter by geographic bounding box. Returns ICAO24 code, callsign, origin country, position, altitude, speed, and heading.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"minLat": fiber.Map{"type": "number", "description": "Bounding box min latitude (optional)"},
				"maxLat": fiber.Map{"type": "number", "description": "Bounding box max latitude (optional)"},
				"minLon": fiber.Map{"type": "number", "description": "Bounding box min longitude (optional)"},
				"maxLon": fiber.Map{"type": "number", "description": "Bounding box max longitude (optional)"},
			},
		},
	},
	{
		"name": "track_aircraft",
		"description": "Get current live position of a specific aircraft by its ICAO 24-bit hex address.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"icao24": fiber.Map{"type": "string", "description": "ICAO 24-bit hex transponder address e.g. '3c675a' for a Lufthansa aircraft"},
			},
			"required": []string{"icao24"},
		},
	},
	{
		"name": "get_flight_history",
		"description": "Get recent flight history for an aircraft over the last N days.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"icao24": fiber.Map{"type": "string", "description": "ICAO 24-bit hex address"},
				"days":   fiber.Map{"type": "integer", "default": 7, "maximum": 30, "description": "How many days of history to return (max 30)"},
			},
			"required": []string{"icao24"},
		},
	},
}

func (h *MCPHandler) Handle(c *fiber.Ctx) error {
	var req mcpReq
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(mcpResp{JSONRPC: "2.0", Error: &mcpErr{Code: -32700, Message: "parse error"}})
	}
	switch req.Method {
	case "initialize":
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: fiber.Map{
			"protocolVersion": "2024-11-05",
			"capabilities":    fiber.Map{"tools": fiber.Map{}},
			"serverInfo":      fiber.Map{"name": "aviation-mcp", "version": "1.0.0"},
		}})
	case "tools/list":
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: fiber.Map{"tools": mcpTools}})
	case "tools/call":
		return h.handleToolCall(c, req)
	default:
		return c.Status(400).JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32601, Message: "method not found"}})
	}
}

func (h *MCPHandler) handleToolCall(c *fiber.Ctx, req mcpReq) error {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32602, Message: "invalid params"}})
	}
	var args map[string]interface{}
	json.Unmarshal(params.Arguments, &args)

	str := func(k string) string {
		if v, ok := args[k]; ok { return fmt.Sprintf("%v", v) }
		return ""
	}
	flt := func(k string) (float64, bool) {
		if v, ok := args[k]; ok {
			if n, ok := v.(float64); ok { return n, true }
		}
		return 0, false
	}
	intArg := func(k string, def int) int {
		if v, ok := args[k]; ok {
			if n, ok := v.(float64); ok { return int(n) }
		}
		return def
	}

	switch params.Name {
	case "get_live_flights":
		var box *client.BoundingBox
		minLat, ok1 := flt("minLat")
		maxLat, ok2 := flt("maxLat")
		minLon, ok3 := flt("minLon")
		maxLon, ok4 := flt("maxLon")
		if ok1 && ok2 && ok3 && ok4 {
			box = &client.BoundingBox{MinLat: minLat, MaxLat: maxLat, MinLon: minLon, MaxLon: maxLon}
		}
		aircraft, ts, err := h.client.GetAllStates(c.Context(), box)
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: err.Error()}})
		}
		if len(aircraft) > 200 {
			aircraft = aircraft[:200]
		}
		data, _ := json.Marshal(fiber.Map{"timestamp": ts, "count": len(aircraft), "aircraft": aircraft})
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	case "track_aircraft":
		a, err := h.client.GetAircraftByICAO(c.Context(), str("icao24"))
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: err.Error()}})
		}
		if a == nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: "aircraft not found or not currently tracked"}})
		}
		data, _ := json.Marshal(a)
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	case "get_flight_history":
		days := intArg("days", 7)
		now := time.Now().Unix()
		begin := now - int64(days)*86400
		flights, err := h.client.GetFlightsByAircraft(c.Context(), str("icao24"), begin, now)
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: err.Error()}})
		}
		data, _ := json.Marshal(flights)
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	default:
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32601, Message: "unknown tool: " + params.Name}})
	}
}

func toolResult(text string) fiber.Map {
	return fiber.Map{"content": []fiber.Map{{"type": "text", "text": text}}}
}
