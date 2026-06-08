// Package handler — MCP (Model Context Protocol) endpoint for VAT API.
package handler

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
	"vat-api/internal/client"
)

// ── MCP JSON-RPC 2.0 types ────────────────────────────────────────────────────

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

type toolsListResult struct {
	Tools []mcpTool `json:"tools"`
}

type mcpTool struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	InputSchema interface{} `json:"inputSchema"`
}

// MCP handles POST /mcp — JSON-RPC 2.0 dispatcher for AI tool use.
func MCP(c *fiber.Ctx, cl *client.Client) error {
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
				"serverInfo":      fiber.Map{"name": "vat-api-mcp", "version": "1.0.0"},
			},
		})

	case "tools/list":
		return c.JSON(mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: toolsListResult{Tools: []mcpTool{
				{
					Name:        "validate_vat",
					Description: "Validate a single EU VAT number via the official EU VIES system. Returns validity, company name, and address.",
					InputSchema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"vat":     map[string]string{"type": "string", "description": "Full VAT number with country prefix, e.g. DE123456789 or LU26375245"},
							"country": map[string]string{"type": "string", "description": "2-letter EU country code (optional if prefix included in vat)"},
						},
						"required": []string{"vat"},
					},
				},
				{
					Name:        "validate_vat_batch",
					Description: "Validate up to 10 EU VAT numbers in a single request. Pass full VAT numbers with country prefix.",
					InputSchema: map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"vat_numbers": map[string]interface{}{
								"type":        "array",
								"items":       map[string]string{"type": "string"},
								"description": "List of full VAT numbers, e.g. [\"DE123456789\", \"FR12345678901\"]",
								"maxItems":    10,
							},
						},
						"required": []string{"vat_numbers"},
					},
				},
				{
					Name:        "list_vat_countries",
					Description: "List all EU member states supported by the VIES VAT validation system.",
					InputSchema: map[string]interface{}{
						"type":       "object",
						"properties": map[string]interface{}{},
					},
				},
			}},
		})

	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(mcpResponse{
				JSONRPC: "2.0", ID: req.ID,
				Error: &mcpError{Code: -32602, Message: "invalid params"},
			})
		}
		return dispatchTool(c, req.ID, params.Name, params.Arguments, cl)

	default:
		return c.JSON(mcpResponse{
			JSONRPC: "2.0", ID: req.ID,
			Error: &mcpError{Code: -32601, Message: "method not found: " + req.Method},
		})
	}
}

func dispatchTool(c *fiber.Ctx, id interface{}, name string, args map[string]interface{}, cl *client.Client) error {
	ok := func(data interface{}) error {
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: id, Result: fiber.Map{
			"content": []fiber.Map{{"type": "text", "text": toJSON(data)}},
		}})
	}
	fail := func(msg string) error {
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: id,
			Error: &mcpError{Code: -32000, Message: msg},
		})
	}

	switch name {
	case "validate_vat":
		vat := strings.TrimSpace(strArg(args, "vat"))
		if vat == "" {
			return fail("vat is required")
		}
		country := strings.TrimSpace(strArg(args, "country"))
		if country == "" && len(vat) >= 2 {
			country = vat[:2]
		}
		res, err := cl.ValidateVAT(c.Context(), country, vat)
		if err != nil {
			return fail(err.Error())
		}
		return ok(res)

	case "validate_vat_batch":
		raw, ok2 := args["vat_numbers"]
		if !ok2 {
			return fail("vat_numbers is required")
		}
		var vatNums []string
		switch v := raw.(type) {
		case []interface{}:
			for _, item := range v {
				if s, ok := item.(string); ok {
					vatNums = append(vatNums, s)
				}
			}
		default:
			return fail("vat_numbers must be an array of strings")
		}
		res, err := cl.ValidateBatch(c.Context(), vatNums)
		if err != nil {
			return fail(err.Error())
		}
		return ok(res)

	case "list_vat_countries":
		codes := client.ValidCountryCodes()
		return ok(fiber.Map{"countries": codes, "count": len(codes)})

	default:
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: id,
			Error: &mcpError{Code: -32601, Message: "unknown tool: " + name},
		})
	}
}

func strArg(args map[string]interface{}, key string) string {
	if v, ok := args[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func toJSON(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}
