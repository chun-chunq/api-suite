package handler

import (
	"encoding/json"
	"fmt"

	"french-company-api/internal/client"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

// MCPHandler serves MCP JSON-RPC 2.0 over POST /mcp
type MCPHandler struct {
	client *client.Client
	log    zerolog.Logger
}

func NewMCPHandler(c *client.Client, log zerolog.Logger) *MCPHandler {
	return &MCPHandler{client: c, log: log}
}

type mcpReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpResp struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpErr     `json:"error,omitempty"`
}

type mcpErr struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

var mcpTools = []fiber.Map{
	{
		"name":        "search_french_company",
		"description": "Search French companies in the official SIRENE registry (all 10M+ French companies and establishments). Search by name, SIREN number, postal code, department, or NAF activity code.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"q":           fiber.Map{"type": "string", "description": "Company name, trade name, or SIREN number"},
				"postalCode":  fiber.Map{"type": "string", "description": "5-digit French postal code e.g. '75001'"},
				"department":  fiber.Map{"type": "string", "description": "2-digit department code e.g. '75' for Paris, '69' for Lyon"},
				"activity":    fiber.Map{"type": "string", "description": "NAF/APE activity code e.g. '62.01Z' for software development"},
				"activeOnly":  fiber.Map{"type": "boolean", "default": false, "description": "Return only active companies"},
				"limit":       fiber.Map{"type": "integer", "default": 10, "maximum": 25},
			},
		},
	},
	{
		"name":        "lookup_french_company",
		"description": "Look up a French company by its 9-digit SIREN number. Returns full details including legal form, address, activity, and employee range.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"siren": fiber.Map{"type": "string", "description": "9-digit SIREN number e.g. '542051180'"},
			},
			"required": []string{"siren"},
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
			"serverInfo":      fiber.Map{"name": "french-company-mcp", "version": "1.0.0"},
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
		if v, ok := args[k]; ok {
			return fmt.Sprintf("%v", v)
		}
		return ""
	}
	intArg := func(k string, def int) int {
		if v, ok := args[k]; ok {
			if n, ok := v.(float64); ok {
				return int(n)
			}
		}
		return def
	}

	switch params.Name {
	case "search_french_company":
		q := client.SearchQuery{
			Query:        str("q"),
			PostalCode:   str("postalCode"),
			Department:   str("department"),
			ActivityCode: str("activity"),
			ActiveOnly:   str("activeOnly") == "true",
			MaxResults:   intArg("limit", 10),
		}
		result, err := h.client.Search(c.Context(), q)
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: err.Error()}})
		}
		data, _ := json.Marshal(result)
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	case "lookup_french_company":
		siren := str("siren")
		co, err := h.client.GetBySIREN(c.Context(), siren)
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32602, Message: err.Error()}})
		}
		if co == nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: "company not found"}})
		}
		data, _ := json.Marshal(co)
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	default:
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32601, Message: "unknown tool: " + params.Name}})
	}
}

func toolResult(text string) fiber.Map {
	return fiber.Map{"content": []fiber.Map{{"type": "text", "text": text}}}
}
