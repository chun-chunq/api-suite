// Package handler — MCP (Model Context Protocol) endpoint for Countries API.
package handler

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
	"countries-api/internal/client"
)

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

// MCP handles POST /mcp — JSON-RPC 2.0 dispatcher.
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
				"serverInfo":      fiber.Map{"name": "countries-api-mcp", "version": "1.0.0"},
			},
		})

	case "tools/list":
		return c.JSON(mcpResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: fiber.Map{"tools": []fiber.Map{
				{
					"name":        "get_country",
					"description": "Get detailed information about a country by its ISO code (2-letter or 3-letter).",
					"inputSchema": fiber.Map{
						"type":     "object",
						"required": []string{"code"},
						"properties": fiber.Map{
							"code": fiber.Map{"type": "string", "description": "ISO 3166-1 alpha-2 or alpha-3 code, e.g. DE, DEU, FR, USA"},
						},
					},
				},
				{
					"name":        "search_country",
					"description": "Search for countries by name. Returns partial matches.",
					"inputSchema": fiber.Map{
						"type":     "object",
						"required": []string{"name"},
						"properties": fiber.Map{
							"name":      fiber.Map{"type": "string", "description": "Country name to search for"},
							"full_text": fiber.Map{"type": "boolean", "description": "If true, require exact full name match"},
						},
					},
				},
				{
					"name":        "get_countries_by_region",
					"description": "Get all countries in a geographic region (Europe, Asia, Americas, Africa, Oceania, Antarctic).",
					"inputSchema": fiber.Map{
						"type":     "object",
						"required": []string{"region"},
						"properties": fiber.Map{
							"region": fiber.Map{"type": "string", "description": "Region name, e.g. Europe, Asia, Americas"},
						},
					},
				},
				{
					"name":        "get_countries_by_currency",
					"description": "Get all countries that use a given currency (e.g. EUR, USD, GBP).",
					"inputSchema": fiber.Map{
						"type":     "object",
						"required": []string{"currency"},
						"properties": fiber.Map{
							"currency": fiber.Map{"type": "string", "description": "ISO 4217 currency code, e.g. EUR"},
						},
					},
				},
				{
					"name":        "get_countries_by_language",
					"description": "Get all countries where a given language is spoken.",
					"inputSchema": fiber.Map{
						"type":     "object",
						"required": []string{"language"},
						"properties": fiber.Map{
							"language": fiber.Map{"type": "string", "description": "Language name or ISO 639-3 code, e.g. German or deu"},
						},
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
			return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID,
				Error: &mcpError{Code: -32602, Message: "invalid params"}})
		}
		return callTool(c, req.ID, params.Name, params.Arguments, cl)

	default:
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID,
			Error: &mcpError{Code: -32601, Message: "method not found: " + req.Method}})
	}
}

func callTool(c *fiber.Ctx, id interface{}, name string, args map[string]interface{}, cl *client.Client) error {
	ok := func(data interface{}) error {
		b, _ := json.MarshalIndent(data, "", "  ")
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: id, Result: fiber.Map{
			"content": []fiber.Map{{"type": "text", "text": string(b)}},
		}})
	}
	fail := func(msg string) error {
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: id,
			Error: &mcpError{Code: -32000, Message: msg}})
	}
	strArg := func(key string) string {
		if v, ok := args[key]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
		return ""
	}

	switch name {
	case "get_country":
		code := strArg("code")
		if code == "" {
			return fail("code is required")
		}
		ct, err := cl.GetByCode(c.Context(), code)
		if err != nil {
			return fail(err.Error())
		}
		return ok(ct)

	case "search_country":
		n := strArg("name")
		if n == "" {
			return fail("name is required")
		}
		fullText := false
		if v, ok := args["full_text"]; ok {
			if b, ok := v.(bool); ok {
				fullText = b
			}
		}
		results, err := cl.SearchByName(c.Context(), n, fullText)
		if err != nil {
			return fail(err.Error())
		}
		return ok(fiber.Map{"count": len(results), "results": results})

	case "get_countries_by_region":
		region := strArg("region")
		if region == "" {
			return fail("region is required")
		}
		results, err := cl.GetByRegion(c.Context(), region)
		if err != nil {
			return fail(err.Error())
		}
		return ok(fiber.Map{"region": region, "count": len(results), "countries": results})

	case "get_countries_by_currency":
		currency := strArg("currency")
		if currency == "" {
			return fail("currency is required")
		}
		results, err := cl.GetByCurrency(c.Context(), currency)
		if err != nil {
			return fail(err.Error())
		}
		return ok(fiber.Map{"currency": currency, "count": len(results), "countries": results})

	case "get_countries_by_language":
		lang := strArg("language")
		if lang == "" {
			return fail("language is required")
		}
		results, err := cl.GetByLanguage(c.Context(), lang)
		if err != nil {
			return fail(err.Error())
		}
		return ok(fiber.Map{"language": lang, "count": len(results), "countries": results})

	default:
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: id,
			Error: &mcpError{Code: -32601, Message: "unknown tool: " + name}})
	}
}
