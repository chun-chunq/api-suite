package handler

import (
	"encoding/json"
	"fmt"

	"gdpr-api/internal/client"

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
		"name": "search_gdpr_fines",
		"description": "Search GDPR enforcement actions and fines from all EU/EEA data protection authorities. Filter by country, entity, fine amount, GDPR article violated, or sector. Returns amount, authority, violation summary, and source links.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"country":    fiber.Map{"type": "string", "description": "ISO country code: DE, FR, IT, ES, IE, NL, PL, ..."},
				"entity":     fiber.Map{"type": "string", "description": "Company or person name (partial match)"},
				"article":    fiber.Map{"type": "string", "description": "GDPR article number e.g. '5', '6', '17', '83'"},
				"minAmount":  fiber.Map{"type": "number", "description": "Minimum fine amount in EUR"},
				"yearFrom":   fiber.Map{"type": "integer", "description": "Minimum year e.g. 2020"},
				"yearTo":     fiber.Map{"type": "integer", "description": "Maximum year e.g. 2024"},
				"sector":     fiber.Map{"type": "string", "description": "Industry sector e.g. 'health', 'telecom', 'finance'"},
				"limit":      fiber.Map{"type": "integer", "default": 10, "maximum": 100},
			},
		},
	},
	{
		"name": "get_top_gdpr_fines",
		"description": "Get the largest GDPR fines of all time, optionally filtered by country.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"country": fiber.Map{"type": "string", "description": "Optional: ISO country code to filter by"},
				"n":       fiber.Map{"type": "integer", "default": 10, "maximum": 50, "description": "Number of top fines to return"},
			},
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
			"serverInfo":      fiber.Map{"name": "gdpr-mcp", "version": "1.0.0"},
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
	flt := func(k string) float64 {
		if v, ok := args[k]; ok {
			if n, ok := v.(float64); ok { return n }
		}
		return 0
	}
	intArg := func(k string, def int) int {
		if v, ok := args[k]; ok {
			if n, ok := v.(float64); ok { return int(n) }
		}
		return def
	}

	switch params.Name {
	case "search_gdpr_fines":
		q := client.SearchQuery{
			Country:    str("country"),
			Entity:     str("entity"),
			Article:    str("article"),
			Sector:     str("sector"),
			MinAmount:  flt("minAmount"),
			YearFrom:   intArg("yearFrom", 0),
			YearTo:     intArg("yearTo", 0),
			MaxResults: intArg("limit", 10),
		}
		result, err := h.client.Search(c.Context(), q)
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: err.Error()}})
		}
		data, _ := json.Marshal(result)
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	case "get_top_gdpr_fines":
		fines, err := h.client.GetTopFines(c.Context(), str("country"), intArg("n", 10))
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: err.Error()}})
		}
		data, _ := json.Marshal(fines)
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	default:
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32601, Message: "unknown tool: " + params.Name}})
	}
}

func toolResult(text string) fiber.Map {
	return fiber.Map{"content": []fiber.Map{{"type": "text", "text": text}}}
}
