package handler

import (
	"encoding/json"
	"fmt"
	"strings"

	"euipo-api/internal/client"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
)

// MCPHandler serves Model Context Protocol (MCP) JSON-RPC 2.0 over HTTP POST /mcp
type MCPHandler struct {
	client *client.Client
	log    zerolog.Logger
	secret string
}

// NewMCPHandler creates a new MCPHandler.
func NewMCPHandler(c *client.Client, log zerolog.Logger, secret string) *MCPHandler {
	return &MCPHandler{client: c, log: log, secret: secret}
}

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

var mcpTools = []fiber.Map{
	{
		"name":        "search_eu_trademark",
		"description": "Search EU and international trademarks via TMview. Covers EUIPO (EU), DPMA (Germany), INPI (France), IPO (UK), and 60+ national offices.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"query":     fiber.Map{"type": "string", "description": "Trademark name to search"},
				"holder":    fiber.Map{"type": "string", "description": "Applicant or holder name"},
				"territory": fiber.Map{"type": "string", "description": "Comma-separated office codes: EM (EUIPO), DE, FR, GB, US — default all"},
				"class":     fiber.Map{"type": "string", "description": "Comma-separated Nice classes (1-45)"},
				"status":    fiber.Map{"type": "string", "enum": []string{"REGISTERED", "PENDING", "EXPIRED"}, "description": "Filter by status"},
				"limit":     fiber.Map{"type": "integer", "default": 10, "maximum": 50},
			},
		},
	},
	{
		"name":        "lookup_eu_trademark",
		"description": "Fetch detailed trademark record by office code and application number.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"office": fiber.Map{"type": "string", "description": "Office code: EM, DE, FR, GB, ..."},
				"appNum": fiber.Map{"type": "string", "description": "Application number"},
			},
			"required": []string{"office", "appNum"},
		},
	},
}

// Handle processes MCP JSON-RPC 2.0 requests.
func (h *MCPHandler) Handle(c *fiber.Ctx) error {
	var req mcpRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(mcpResponse{
			JSONRPC: "2.0",
			Error:   &mcpError{Code: -32700, Message: "parse error"},
		})
	}

	switch req.Method {
	case "initialize":
		return c.JSON(mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: fiber.Map{
				"protocolVersion": "2024-11-05",
				"capabilities":    fiber.Map{"tools": fiber.Map{}},
				"serverInfo":      fiber.Map{"name": "eu-trademark-mcp", "version": "1.0.0"},
			},
		})

	case "tools/list":
		return c.JSON(mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  fiber.Map{"tools": mcpTools},
		})

	case "tools/call":
		return h.handleToolCall(c, req)

	default:
		return c.Status(400).JSON(mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &mcpError{Code: -32601, Message: "method not found"},
		})
	}
}

func (h *MCPHandler) handleToolCall(c *fiber.Ctx, req mcpRequest) error {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpError{Code: -32602, Message: "invalid params"}})
	}

	var args map[string]interface{}
	if err := json.Unmarshal(params.Arguments, &args); err != nil {
		args = map[string]interface{}{}
	}

	str := func(key string) string {
		if v, ok := args[key]; ok {
			return fmt.Sprintf("%v", v)
		}
		return ""
	}
	intArg := func(key string, def int) int {
		if v, ok := args[key]; ok {
			switch n := v.(type) {
			case float64:
				return int(n)
			}
		}
		return def
	}

	switch params.Name {
	case "search_eu_trademark":
		q := client.SearchQuery{
			Query:      str("query"),
			Holder:     str("holder"),
			Status:     strings.ToUpper(str("status")),
			MaxResults: intArg("limit", 10),
		}
		if t := str("territory"); t != "" {
			for _, tc := range strings.Split(t, ",") {
				q.Territories = append(q.Territories, strings.TrimSpace(strings.ToUpper(tc)))
			}
		}
		result, err := h.client.Search(c.Context(), q)
		if err != nil {
			return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpError{Code: -32000, Message: err.Error()}})
		}
		data, _ := json.Marshal(result)
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	case "lookup_eu_trademark":
		office := str("office")
		appNum := str("appNum")
		if office == "" || appNum == "" {
			return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpError{Code: -32602, Message: "office and appNum required"}})
		}
		tm, err := h.client.GetByID(c.Context(), office, appNum)
		if err != nil {
			return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpError{Code: -32000, Message: err.Error()}})
		}
		if tm == nil {
			return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpError{Code: -32000, Message: "trademark not found"}})
		}
		data, _ := json.Marshal(tm)
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	default:
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpError{Code: -32601, Message: "unknown tool: " + params.Name}})
	}
}

func toolResult(text string) fiber.Map {
	return fiber.Map{
		"content": []fiber.Map{
			{"type": "text", "text": text},
		},
	}
}
