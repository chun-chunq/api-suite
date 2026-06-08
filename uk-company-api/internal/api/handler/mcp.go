package handler

import (
	"encoding/json"
	"fmt"

	"uk-company-api/internal/client"

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
		"name":        "search_uk_company",
		"description": "Search UK companies in the official Companies House register. Returns company name, number, status, type, incorporation date, and registered address.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"q":      fiber.Map{"type": "string", "description": "Company name or company number"},
				"limit":  fiber.Map{"type": "integer", "default": 10, "maximum": 100},
				"offset": fiber.Map{"type": "integer", "default": 0},
			},
			"required": []string{"q"},
		},
	},
	{
		"name":        "lookup_uk_company",
		"description": "Get full company profile by UK Companies House company number (e.g. '00102498'). Returns SIC codes, jurisdiction, and registered address.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"companyNumber": fiber.Map{"type": "string", "description": "Companies House number e.g. '00102498'"},
			},
			"required": []string{"companyNumber"},
		},
	},
	{
		"name":        "get_uk_company_officers",
		"description": "Get directors and officers for a UK company. Optionally filter to active (non-resigned) officers only.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"companyNumber": fiber.Map{"type": "string", "description": "Companies House number"},
				"activeOnly":    fiber.Map{"type": "boolean", "default": true, "description": "Return only active (non-resigned) officers"},
			},
			"required": []string{"companyNumber"},
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
			"serverInfo":      fiber.Map{"name": "uk-company-mcp", "version": "1.0.0"},
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
	case "search_uk_company":
		result, err := h.client.Search(c.Context(), str("q"), intArg("limit", 10), intArg("offset", 0))
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: err.Error()}})
		}
		data, _ := json.Marshal(result)
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	case "lookup_uk_company":
		co, err := h.client.GetByNumber(c.Context(), str("companyNumber"))
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32602, Message: err.Error()}})
		}
		if co == nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: "company not found"}})
		}
		data, _ := json.Marshal(co)
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	case "get_uk_company_officers":
		activeOnly := str("activeOnly") != "false"
		officers, err := h.client.GetOfficers(c.Context(), str("companyNumber"), activeOnly)
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: err.Error()}})
		}
		data, _ := json.Marshal(officers)
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	default:
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32601, Message: "unknown tool: " + params.Name}})
	}
}

func toolResult(text string) fiber.Map {
	return fiber.Map{"content": []fiber.Map{{"type": "text", "text": text}}}
}
