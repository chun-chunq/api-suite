package handler

import (
	"encoding/json"
	"fmt"

	"food-api/internal/client"

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
		"name": "lookup_food_product",
		"description": "Look up a food product by barcode (EAN-13, UPC, etc.). Returns product name, brand, nutritional values per 100g, NutriScore (A-E), NOVA processing group (1-4), eco-score, allergens, and ingredients.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"barcode": fiber.Map{"type": "string", "description": "Product barcode e.g. '3017620422003' for Nutella"},
			},
			"required": []string{"barcode"},
		},
	},
	{
		"name": "search_food_products",
		"description": "Search food products by name, brand, or category. Returns nutritional info, NutriScore, and ingredients.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"q":          fiber.Map{"type": "string", "description": "Product name search"},
				"brands":     fiber.Map{"type": "string", "description": "Brand name filter e.g. 'Nestlé', 'Ferrero'"},
				"categories": fiber.Map{"type": "string", "description": "Category e.g. 'chocolates', 'yogurts', 'breads'"},
				"nutriScore": fiber.Map{"type": "string", "enum": []string{"A","B","C","D","E"}, "description": "Filter by NutriScore grade"},
				"limit":      fiber.Map{"type": "integer", "default": 10, "maximum": 50},
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
			"serverInfo":      fiber.Map{"name": "food-mcp", "version": "1.0.0"},
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
	intArg := func(k string, def int) int {
		if v, ok := args[k]; ok {
			if n, ok := v.(float64); ok { return int(n) }
		}
		return def
	}

	switch params.Name {
	case "lookup_food_product":
		p, err := h.client.GetByBarcode(c.Context(), str("barcode"))
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: err.Error()}})
		}
		if p == nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: "product not found"}})
		}
		data, _ := json.Marshal(p)
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	case "search_food_products":
		q := client.SearchQuery{
			Query:      str("q"),
			Brands:     str("brands"),
			Categories: str("categories"),
			NutriScore: str("nutriScore"),
			MaxResults: intArg("limit", 10),
		}
		result, err := h.client.Search(c.Context(), q)
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: err.Error()}})
		}
		data, _ := json.Marshal(result)
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	default:
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32601, Message: "unknown tool: " + params.Name}})
	}
}

func toolResult(text string) fiber.Map {
	return fiber.Map{"content": []fiber.Map{{"type": "text", "text": text}}}
}
