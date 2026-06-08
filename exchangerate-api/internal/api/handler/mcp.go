// Package handler — MCP endpoint for Exchange Rate API.
package handler

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
	"exchangerate-api/internal/client"
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

func MCP(c *fiber.Ctx, cl *client.Client) error {
	var req mcpRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32700, Message: "parse error"}})
	}

	switch req.Method {
	case "initialize":
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: fiber.Map{
			"protocolVersion": "2024-11-05",
			"capabilities":    fiber.Map{"tools": fiber.Map{}},
			"serverInfo":      fiber.Map{"name": "exchangerate-api-mcp", "version": "1.0.0"},
		}})

	case "tools/list":
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: fiber.Map{"tools": []fiber.Map{
			{
				"name":        "get_latest_rates",
				"description": "Get the latest exchange rates for a base currency (ECB data via Frankfurter).",
				"inputSchema": fiber.Map{"type": "object", "properties": fiber.Map{
					"base":    fiber.Map{"type": "string", "description": "Base currency code (e.g. EUR, USD, GBP). Default: EUR"},
					"symbols": fiber.Map{"type": "string", "description": "Comma-separated target currencies (e.g. USD,GBP,JPY). Omit for all."},
				}},
			},
			{
				"name":        "get_historical_rates",
				"description": "Get exchange rates for a specific past date (back to 1999-01-04).",
				"inputSchema": fiber.Map{"type": "object", "required": []string{"date"}, "properties": fiber.Map{
					"date":    fiber.Map{"type": "string", "description": "Date in YYYY-MM-DD format"},
					"base":    fiber.Map{"type": "string", "description": "Base currency (default EUR)"},
					"symbols": fiber.Map{"type": "string", "description": "Target currencies (comma-separated)"},
				}},
			},
			{
				"name":        "get_rate_time_series",
				"description": "Get exchange rate time series for a date range (max 365 days). Returns daily rates.",
				"inputSchema": fiber.Map{"type": "object", "required": []string{"start", "end"}, "properties": fiber.Map{
					"start":   fiber.Map{"type": "string", "description": "Start date YYYY-MM-DD"},
					"end":     fiber.Map{"type": "string", "description": "End date YYYY-MM-DD"},
					"base":    fiber.Map{"type": "string", "description": "Base currency (default EUR)"},
					"symbols": fiber.Map{"type": "string", "description": "Target currencies (comma-separated)"},
				}},
			},
			{
				"name":        "convert_currency",
				"description": "Convert an amount from one currency to another using live ECB rates.",
				"inputSchema": fiber.Map{"type": "object", "required": []string{"from", "to", "amount"}, "properties": fiber.Map{
					"from":   fiber.Map{"type": "string", "description": "Source currency code (e.g. USD)"},
					"to":     fiber.Map{"type": "string", "description": "Target currency code (e.g. EUR)"},
					"amount": fiber.Map{"type": "number", "description": "Amount to convert"},
				}},
			},
			{
				"name":        "list_currencies",
				"description": "List all supported currency codes and their names.",
				"inputSchema": fiber.Map{"type": "object", "properties": fiber.Map{}},
			},
		}}})

	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpError{Code: -32602, Message: "invalid params"}})
		}
		return callTool(c, req.ID, params.Name, params.Arguments, cl)

	default:
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Error: &mcpError{Code: -32601, Message: "method not found: " + req.Method}})
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
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: id, Error: &mcpError{Code: -32000, Message: msg}})
	}
	str := func(key, def string) string {
		if v, ok := args[key]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
		return def
	}
	syms := func() []string { return parseSymbols(str("symbols", "")) }

	switch name {
	case "get_latest_rates":
		result, err := cl.GetLatest(c.Context(), str("base", "EUR"), syms())
		if err != nil {
			return fail(err.Error())
		}
		return ok(result)

	case "get_historical_rates":
		date := str("date", "")
		if date == "" {
			return fail("date is required")
		}
		result, err := cl.GetHistorical(c.Context(), date, str("base", "EUR"), syms())
		if err != nil {
			return fail(err.Error())
		}
		return ok(result)

	case "get_rate_time_series":
		start, end := str("start", ""), str("end", "")
		if start == "" || end == "" {
			return fail("start and end are required")
		}
		result, err := cl.GetTimeSeries(c.Context(), start, end, str("base", "EUR"), syms())
		if err != nil {
			return fail(err.Error())
		}
		return ok(result)

	case "convert_currency":
		from, to := str("from", ""), str("to", "")
		if from == "" || to == "" {
			return fail("from and to are required")
		}
		amount := 1.0
		if v, ok := args["amount"].(float64); ok {
			amount = v
		}
		result, err := cl.Convert(c.Context(), from, to, amount)
		if err != nil {
			return fail(err.Error())
		}
		return ok(result)

	case "list_currencies":
		currencies, err := cl.GetCurrencies(c.Context())
		if err != nil {
			return fail(err.Error())
		}
		return ok(fiber.Map{"count": len(currencies), "currencies": currencies})

	default:
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: id, Error: &mcpError{Code: -32601, Message: "unknown tool: " + name}})
	}
}
