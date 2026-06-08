// MCP (Model Context Protocol) server for the EU Sanctions API.
// Exposes sanctions screening as an MCP tool for AI assistants.
// POST /mcp — JSON-RPC 2.0
package handler

import (
	"context"
	"encoding/json"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"github.com/sanctions-api/internal/sanctions"
)

// MCPHandler implements MCP tools/list and tools/call.
type MCPHandler struct {
	index *sanctions.Index
	log   zerolog.Logger
}

func NewMCPHandler(idx *sanctions.Index, log zerolog.Logger) *MCPHandler {
	return &MCPHandler{index: idx, log: log}
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

// Handle handles POST /mcp
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
				"name":    "eu-sanctions-api",
				"version": "1.0.0",
			},
		})

	case "tools/list":
		result = mustMarshal(map[string]any{
			"tools": []any{
				map[string]any{
					"name":        "check_sanctions",
					"description": "Check if a person or company appears on the EU consolidated sanctions list. Returns a boolean match result plus matching entity details. Use for AML/KYC compliance screening.",
					"inputSchema": map[string]any{
						"type":     "object",
						"required": []string{"name"},
						"properties": map[string]any{
							"name": map[string]any{
								"type":        "string",
								"description": "Full name of the person or company to screen (e.g. 'Vladimir Putin' or 'Sberbank')",
							},
						},
					},
				},
				map[string]any{
					"name":        "search_sanctions",
					"description": "Search the EU sanctions list for matching entities. Returns up to 20 matching records with details including aliases, birthdates, nationalities, and sanction grounds.",
					"inputSchema": map[string]any{
						"type":     "object",
						"required": []string{"query"},
						"properties": map[string]any{
							"query": map[string]any{
								"type":        "string",
								"description": "Name, alias, or company to search for",
							},
							"type": map[string]any{
								"type":        "string",
								"enum":        []string{"person", "entity", "ship"},
								"description": "Optional: filter by subject type",
							},
							"maxResults": map[string]any{
								"type":        "integer",
								"description": "Max results (1-50, default 20)",
								"default":     20,
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
	_ = ctx
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &mcpError{Code: -32602, Message: "invalid params"}
	}

	switch p.Name {
	case "check_sanctions":
		var args struct {
			Name string `json:"name"`
		}
		json.Unmarshal(p.Arguments, &args)
		if args.Name == "" {
			return nil, &mcpError{Code: -32602, Message: "name is required"}
		}
		entities := h.index.Check(args.Name)
		matched := len(entities) > 0
		return toolResult(map[string]any{
			"query":   args.Name,
			"matched": matched,
			"count":   len(entities),
			"matches": entities,
		})

	case "search_sanctions":
		var args struct {
			Query      string `json:"query"`
			Type       string `json:"type"`
			MaxResults int    `json:"maxResults"`
		}
		json.Unmarshal(p.Arguments, &args)
		if args.Query == "" {
			return nil, &mcpError{Code: -32602, Message: "query is required"}
		}
		if args.MaxResults <= 0 || args.MaxResults > 50 {
			args.MaxResults = 20
		}
		entities := h.index.Search(args.Query, args.MaxResults)
		// Filter by type if specified
		if args.Type != "" {
			filtered := entities[:0]
			for _, e := range entities {
				if e.SubjectType == args.Type {
					filtered = append(filtered, e)
				}
			}
			entities = filtered
		}
		return toolResult(map[string]any{
			"query":   args.Query,
			"count":   len(entities),
			"results": entities,
		})

	default:
		return nil, &mcpError{Code: -32602, Message: "unknown tool: " + p.Name}
	}
}

func toolResult(v any) (json.RawMessage, *mcpError) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, &mcpError{Code: -32000, Message: "marshal error"}
	}
	out, _ := json.Marshal(map[string]any{
		"content": []any{
			map[string]any{"type": "text", "text": string(data)},
		},
	})
	return out, nil
}

func mustMarshal(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
