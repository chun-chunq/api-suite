// MCP (Model Context Protocol) server for the GLEIF LEI API.
// Exposes LEI search and lookup as MCP tools for AI assistants.
// POST /mcp — JSON-RPC 2.0
package handler

import (
	"context"
	"encoding/json"

	"github.com/gleif-api/internal/client"
	"github.com/gofiber/fiber/v2"
)

// MCPHandler exposes LEI tools via the Model Context Protocol.
type MCPHandler struct {
	client *client.Client
}

func NewMCPHandler(c *client.Client) *MCPHandler {
	return &MCPHandler{client: c}
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
		result = mustMarshalM(map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo": map[string]any{
				"name":    "gleif-lei-api",
				"version": "1.0.0",
			},
		})

	case "tools/list":
		result = mustMarshalM(map[string]any{
			"tools": []any{
				map[string]any{
					"name":        "search_lei",
					"description": "Search for a company in the GLEIF Global LEI Index by name. Returns the company's LEI code, legal address, jurisdiction, and registration status. Use to validate counterparties or find the LEI code required for MiFID II trade reporting.",
					"inputSchema": map[string]any{
						"type":     "object",
						"required": []string{"name"},
						"properties": map[string]any{
							"name": map[string]any{
								"type":        "string",
								"description": "Company or legal entity name to search (e.g. 'Deutsche Bank' or 'Apple Inc')",
							},
							"country": map[string]any{
								"type":        "string",
								"description": "Optional ISO-2 country code to filter results (e.g. 'DE', 'US', 'CH')",
							},
							"activeOnly": map[string]any{
								"type":        "boolean",
								"description": "If true, only return entities with ACTIVE LEI status",
								"default":     true,
							},
						},
					},
				},
				map[string]any{
					"name":        "lookup_lei",
					"description": "Get the full legal entity record for a known 20-character LEI code. Returns name, address, jurisdiction, BIC codes, and registration details.",
					"inputSchema": map[string]any{
						"type":     "object",
						"required": []string{"lei"},
						"properties": map[string]any{
							"lei": map[string]any{
								"type":        "string",
								"description": "20-character LEI code (ISO 17442), e.g. '5299000J2N45DDNE4Y28'",
								"minLength":   20,
								"maxLength":   20,
							},
						},
					},
				},
				map[string]any{
					"name":        "get_lei_relationships",
					"description": "Get the corporate ownership structure for a legal entity: direct parent, ultimate parent (top of ownership chain), and direct subsidiaries.",
					"inputSchema": map[string]any{
						"type":     "object",
						"required": []string{"lei"},
						"properties": map[string]any{
							"lei": map[string]any{
								"type":        "string",
								"description": "20-character LEI code",
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
	case "search_lei":
		var args struct {
			Name       string `json:"name"`
			Country    string `json:"country"`
			ActiveOnly bool   `json:"activeOnly"`
		}
		args.ActiveOnly = true // default
		json.Unmarshal(p.Arguments, &args)
		if args.Name == "" {
			return nil, &mcpError{Code: -32602, Message: "name is required"}
		}
		entities, total, err := h.client.SearchByName(ctx, args.Name, args.Country, args.ActiveOnly, 10)
		if err != nil {
			return nil, &mcpError{Code: -32000, Message: err.Error()}
		}
		return toolResultM(map[string]any{
			"query":   args.Name,
			"total":   total,
			"count":   len(entities),
			"results": entities,
		})

	case "lookup_lei":
		var args struct {
			LEI string `json:"lei"`
		}
		json.Unmarshal(p.Arguments, &args)
		if len(args.LEI) != 20 {
			return nil, &mcpError{Code: -32602, Message: "lei must be exactly 20 characters"}
		}
		entity, err := h.client.GetByLEI(ctx, args.LEI)
		if err != nil {
			return nil, &mcpError{Code: -32000, Message: err.Error()}
		}
		if entity == nil {
			return toolResultM(map[string]any{"found": false, "lei": args.LEI})
		}
		return toolResultM(map[string]any{"found": true, "entity": entity})

	case "get_lei_relationships":
		var args struct {
			LEI string `json:"lei"`
		}
		json.Unmarshal(p.Arguments, &args)
		if len(args.LEI) != 20 {
			return nil, &mcpError{Code: -32602, Message: "lei must be exactly 20 characters"}
		}
		summary, err := h.client.GetRelationships(ctx, args.LEI)
		if err != nil {
			return nil, &mcpError{Code: -32000, Message: err.Error()}
		}
		return toolResultM(map[string]any{"lei": args.LEI, "relationships": summary})

	default:
		return nil, &mcpError{Code: -32602, Message: "unknown tool: " + p.Name}
	}
}

func toolResultM(v any) (json.RawMessage, *mcpError) {
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

func mustMarshalM(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
