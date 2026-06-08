// Package handler — MCP (Model Context Protocol) server implementation.
// Exposes trademark search as an MCP tool so AI assistants (Claude Desktop,
// Cursor, etc.) can call it directly.
//
// Protocol: JSON-RPC 2.0 over HTTP POST /mcp
// Spec: https://modelcontextprotocol.io
package handler

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"

	"github.com/dpma-api/internal/scraper"
)

// MCPHandler implements the MCP tools/list and tools/call methods.
type MCPHandler struct {
	trademark *TrademarkHandler
	log       zerolog.Logger
}

func NewMCPHandler(tm *TrademarkHandler, log zerolog.Logger) *MCPHandler {
	return &MCPHandler{trademark: tm, log: log}
}

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Handle handles POST /mcp — JSON-RPC 2.0
func (h *MCPHandler) Handle(c *fiber.Ctx) error {
	var req mcpRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(mcpResponse{
			JSONRPC: "2.0",
			Error:   &mcpError{Code: -32700, Message: "parse error"},
		})
	}

	var result json.RawMessage
	var rpcErr *mcpError

	switch req.Method {
	case "initialize":
		result = mustMarshal(map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{
				"name":    "dpma-trademark-api",
				"version": "1.0.0",
			},
		})

	case "tools/list":
		result = mustMarshal(map[string]any{
			"tools": []any{
				map[string]any{
					"name":        "search_trademark",
					"description": "Search the German DPMA trademark register for trademarks by name, owner, or Nice class. Returns registration status, filing dates, and owner information.",
					"inputSchema": map[string]any{
						"type":     "object",
						"required": []string{},
						"properties": map[string]any{
							"name": map[string]any{
								"type":        "string",
								"description": "Trademark name or keyword to search for",
							},
							"owner": map[string]any{
								"type":        "string",
								"description": "Owner or applicant company/person name",
							},
							"registrationNumber": map[string]any{
								"type":        "string",
								"description": "Exact DPMA registration number (e.g. '30010285')",
							},
							"class": map[string]any{
								"type":        "string",
								"description": "Nice Classification class numbers 1-45, comma-separated (e.g. '9,35,42')",
							},
							"status": map[string]any{
								"type":        "string",
								"enum":        []string{"registered", "applied", "expired", "deleted"},
								"description": "Filter by trademark status",
							},
							"maxResults": map[string]any{
								"type":        "integer",
								"description": "Maximum results to return (1-200, default 20)",
								"default":     20,
							},
						},
					},
				},
				map[string]any{
					"name":        "get_trademark",
					"description": "Get full details for a specific trademark by its DPMA registration number.",
					"inputSchema": map[string]any{
						"type":     "object",
						"required": []string{"registrationNumber"},
						"properties": map[string]any{
							"registrationNumber": map[string]any{
								"type":        "string",
								"description": "DPMA registration number",
							},
						},
					},
				},
			},
		})

	case "tools/call":
		result, rpcErr = h.callTool(c.Context(), req.Params)

	default:
		rpcErr = &mcpError{Code: -32601, Message: "method not found: " + req.Method}
	}

	return c.JSON(mcpResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
		Result:  result,
		Error:   rpcErr,
	})
}

func (h *MCPHandler) callTool(ctx context.Context, params json.RawMessage) (json.RawMessage, *mcpError) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &mcpError{Code: -32602, Message: "invalid params"}
	}

	switch p.Name {
	case "search_trademark":
		var args map[string]any
		json.Unmarshal(p.Arguments, &args)

		q := scraper.SearchQuery{MaxResults: 20}
		if v, ok := args["name"].(string); ok {
			q.Name = v
		}
		if v, ok := args["owner"].(string); ok {
			q.Owner = v
		}
		if v, ok := args["registrationNumber"].(string); ok {
			q.RegistrationNumber = v
		}
		if v, ok := args["status"].(string); ok {
			q.Status = v
		}
		if v, ok := args["maxResults"].(float64); ok {
			q.MaxResults = int(v)
		}
		if v, ok := args["class"].(string); ok {
			for _, part := range strings.Split(v, ",") {
				if n, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && n >= 1 && n <= 45 {
					q.Classes = append(q.Classes, n)
				}
			}
		}

		result, _, err := h.trademark.runSearch(ctx, q)
		if err != nil {
			return nil, &mcpError{Code: -32000, Message: err.Error()}
		}
		return toolResult(result)

	case "get_trademark":
		var args map[string]string
		json.Unmarshal(p.Arguments, &args)
		num := args["registrationNumber"]
		if num == "" {
			return nil, &mcpError{Code: -32602, Message: "registrationNumber required"}
		}
		q := scraper.SearchQuery{RegistrationNumber: num, MaxResults: 1}
		result, _, err := h.trademark.runSearch(ctx, q)
		if err != nil {
			return nil, &mcpError{Code: -32000, Message: err.Error()}
		}
		if len(result.Results) == 0 {
			return toolResult(map[string]any{"found": false, "registrationNumber": num})
		}
		return toolResult(result.Results[0])

	default:
		return nil, &mcpError{Code: -32602, Message: "unknown tool: " + p.Name}
	}
}

// toolResult wraps a value in MCP tool result format.
func toolResult(v any) (json.RawMessage, *mcpError) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, &mcpError{Code: -32000, Message: "marshal error"}
	}
	out, _ := json.Marshal(map[string]any{
		"content": []any{
			map[string]any{
				"type": "text",
				"text": string(data),
			},
		},
	})
	return out, nil
}

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
