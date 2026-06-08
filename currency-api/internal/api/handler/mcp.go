package handler

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"currency-api/internal/client"
)

type MCPHandler struct {
	client *client.Client
	log    zerolog.Logger
}

func NewMCPHandler(c *client.Client, log zerolog.Logger) *MCPHandler {
	return &MCPHandler{client: c, log: log}
}

var mcpTools = []fiber.Map{
	{
		"name":        "get_latest_exchange_rates",
		"description": "Get the latest ECB Euro FX reference rates. Returns all ~30 major currencies vs EUR (updated daily ~16:00 CET). Optionally rebase to any supported currency.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"base": fiber.Map{"type": "string", "description": "Base currency (3-letter ISO code). Default: EUR"},
			},
		},
	},
	{
		"name":        "convert_currency",
		"description": "Convert an amount from one currency to another using today's ECB reference rates.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"from":   fiber.Map{"type": "string", "description": "Source currency code (e.g. USD, GBP, JPY, EUR)"},
				"to":     fiber.Map{"type": "string", "description": "Target currency code"},
				"amount": fiber.Map{"type": "number", "description": "Amount to convert. Default: 1"},
			},
			"required": []string{"from", "to"},
		},
	},
	{
		"name":        "get_rate_history",
		"description": "Get historical daily exchange rates for a currency pair over the last 1-90 days (ECB data).",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"from": fiber.Map{"type": "string", "description": "Source currency code"},
				"to":   fiber.Map{"type": "string", "description": "Target currency code"},
				"days": fiber.Map{"type": "integer", "description": "Number of days of history (1-90). Default 30."},
			},
			"required": []string{"from", "to"},
		},
	},
}

func (h *MCPHandler) Handle(c *fiber.Ctx) error {
	var req struct {
		JSONRPC string      `json:"jsonrpc"`
		ID      interface{} `json:"id"`
		Method  string      `json:"method"`
		Params  fiber.Map   `json:"params"`
	}
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(mcpError(nil, -32700, "Parse error", err.Error()))
	}

	switch req.Method {
	case "initialize":
		return c.JSON(fiber.Map{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": fiber.Map{
				"protocolVersion": "2024-11-05",
				"serverInfo":      fiber.Map{"name": "currency-api", "version": "1.0.0"},
				"capabilities":    fiber.Map{"tools": fiber.Map{}},
			},
		})
	case "tools/list":
		return c.JSON(fiber.Map{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  fiber.Map{"tools": mcpTools},
		})
	case "tools/call":
		return h.callTool(c, req.ID, req.Params)
	default:
		return c.Status(400).JSON(mcpError(req.ID, -32601, "Method not found", req.Method))
	}
}

func (h *MCPHandler) callTool(c *fiber.Ctx, id interface{}, params fiber.Map) error {
	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]interface{})
	if args == nil {
		args = fiber.Map{}
	}

	switch name {
	case "get_latest_exchange_rates":
		base, _ := args["base"].(string)
		if base == "" {
			base = "EUR"
		}
		base = strings.ToUpper(base)

		rates, err := h.client.GetLatestRates(c.Context())
		if err != nil {
			return c.JSON(mcpToolError(id, err.Error()))
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "ECB Euro FX Reference Rates — %s (base: %s)\n\n", rates.Date, base)
		if base == "EUR" {
			for _, code := range sortedKeys(rates.Rates) {
				fmt.Fprintf(&sb, "  1 EUR = %.4f %s\n", rates.Rates[code], code)
			}
		} else {
			baseRate, ok := rates.Rates[base]
			if !ok {
				return c.JSON(mcpToolError(id, "unsupported base: "+base))
			}
			fmt.Fprintf(&sb, "  1 %s = %.6f EUR\n", base, 1.0/baseRate)
			for _, code := range sortedKeys(rates.Rates) {
				if code == base {
					continue
				}
				fmt.Fprintf(&sb, "  1 %s = %.6f %s\n", base, rates.Rates[code]/baseRate, code)
			}
		}
		return c.JSON(mcpResult(id, sb.String()))

	case "convert_currency":
		from, _ := args["from"].(string)
		to, _ := args["to"].(string)
		amount := 1.0
		if v, ok := args["amount"].(float64); ok {
			amount = v
		}
		res, err := h.client.Convert(c.Context(), from, to, amount)
		if err != nil {
			return c.JSON(mcpToolError(id, err.Error()))
		}
		text := fmt.Sprintf(
			"Currency Conversion (%s)\n%.6f %s = %.6f %s\nRate: 1 %s = %.6f %s  (inverse: 1 %s = %.6f %s)",
			res.Date,
			res.Amount, res.From, res.Result, res.To,
			res.From, res.Rate, res.To,
			res.To, res.Inverted, res.From,
		)
		return c.JSON(mcpResult(id, text))

	case "get_rate_history":
		from, _ := args["from"].(string)
		to, _ := args["to"].(string)
		days := 30
		if v, ok := args["days"].(float64); ok {
			days = int(v)
		}
		points, err := h.client.GetHistory(c.Context(), from, to, days)
		if err != nil {
			return c.JSON(mcpToolError(id, err.Error()))
		}
		from = strings.ToUpper(from)
		to = strings.ToUpper(to)
		var sb strings.Builder
		fmt.Fprintf(&sb, "Rate history %s/%s (last %d trading days):\n", from, to, len(points))
		for _, p := range points {
			fmt.Fprintf(&sb, "  %s: %.6f\n", p.Date, p.Rate)
		}
		return c.JSON(mcpResult(id, sb.String()))

	default:
		return c.JSON(mcpError(id, -32601, "Unknown tool", name))
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func sortedKeys(m map[string]float64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// sort inline
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

func mcpResult(id interface{}, text string) fiber.Map {
	return fiber.Map{
		"jsonrpc": "2.0", "id": id,
		"result": fiber.Map{"content": []fiber.Map{{"type": "text", "text": text}}},
	}
}

func mcpError(id interface{}, code int, msg, data string) fiber.Map {
	return fiber.Map{
		"jsonrpc": "2.0", "id": id,
		"error": fiber.Map{"code": code, "message": msg, "data": data},
	}
}

func mcpToolError(id interface{}, msg string) fiber.Map {
	return fiber.Map{
		"jsonrpc": "2.0", "id": id,
		"result": fiber.Map{"content": []fiber.Map{{"type": "text", "text": msg}}, "isError": true},
	}
}
