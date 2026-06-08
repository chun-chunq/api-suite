package handler

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"crypto-api/internal/client"
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
		"name":        "get_top_cryptocurrencies",
		"description": "Get the top N cryptocurrencies ranked by market cap with price, volume, and 24h change.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"currency": fiber.Map{"type": "string", "description": "Quote currency (usd, eur, gbp, etc). Default: usd"},
				"limit":    fiber.Map{"type": "integer", "description": "Number of coins (1-250). Default: 10"},
			},
		},
	},
	{
		"name":        "get_crypto_prices",
		"description": "Get current prices for specific cryptocurrencies by CoinGecko ID (e.g. bitcoin, ethereum, solana).",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"coin_ids": fiber.Map{"type": "array", "items": fiber.Map{"type": "string"}, "description": "List of CoinGecko coin IDs"},
				"currency": fiber.Map{"type": "string", "description": "Quote currency. Default: usd"},
			},
			"required": []string{"coin_ids"},
		},
	},
	{
		"name":        "get_trending_crypto",
		"description": "Get the top 7 trending cryptocurrencies on CoinGecko in the last 24 hours.",
		"inputSchema": fiber.Map{"type": "object", "properties": fiber.Map{}},
	},
	{
		"name":        "get_coin_detail",
		"description": "Get detailed profile for a single cryptocurrency: description, categories, website, market data.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"coin_id":  fiber.Map{"type": "string", "description": "CoinGecko coin ID (e.g. bitcoin, ethereum, solana)"},
				"currency": fiber.Map{"type": "string", "description": "Quote currency. Default: usd"},
			},
			"required": []string{"coin_id"},
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
			"jsonrpc": "2.0", "id": req.ID,
			"result": fiber.Map{
				"protocolVersion": "2024-11-05",
				"serverInfo":      fiber.Map{"name": "crypto-api", "version": "1.0.0"},
				"capabilities":    fiber.Map{"tools": fiber.Map{}},
			},
		})
	case "tools/list":
		return c.JSON(fiber.Map{"jsonrpc": "2.0", "id": req.ID, "result": fiber.Map{"tools": mcpTools}})
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

	currency := "usd"
	if cur, ok := args["currency"].(string); ok && cur != "" {
		currency = strings.ToLower(cur)
	}

	switch name {
	case "get_top_cryptocurrencies":
		limit := 10
		if v, ok := args["limit"].(float64); ok {
			limit = int(v)
		}
		coins, err := h.client.GetTopCoins(c.Context(), currency, limit)
		if err != nil {
			return c.JSON(mcpToolError(id, err.Error()))
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Top %d Cryptocurrencies by Market Cap (%s)\n\n", len(coins), strings.ToUpper(currency))
		for i, coin := range coins {
			sign := "+"
			if coin.PriceChangePct24h < 0 {
				sign = ""
			}
			fmt.Fprintf(&sb, "%d. %s (%s) — %.2f %s | MCap: %.2fB | 24h: %s%.2f%%\n",
				i+1, coin.Name, strings.ToUpper(coin.Symbol),
				coin.CurrentPrice, strings.ToUpper(currency),
				coin.MarketCap/1e9,
				sign, coin.PriceChangePct24h,
			)
		}
		return c.JSON(mcpResult(id, sb.String()))

	case "get_crypto_prices":
		var coinIDs []string
		if arr, ok := args["coin_ids"].([]interface{}); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok {
					coinIDs = append(coinIDs, s)
				}
			}
		}
		if len(coinIDs) == 0 {
			return c.JSON(mcpError(id, -32602, "Invalid params", "coin_ids is required"))
		}
		coins, err := h.client.GetPrices(c.Context(), coinIDs, currency)
		if err != nil {
			return c.JSON(mcpToolError(id, err.Error()))
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Cryptocurrency Prices (%s)\n\n", strings.ToUpper(currency))
		for _, coin := range coins {
			sign := "+"
			if coin.PriceChangePct24h < 0 {
				sign = ""
			}
			fmt.Fprintf(&sb, "%s (%s): %.6f %s | 24h: %s%.2f%% | Vol: %.2fM\n",
				coin.Name, strings.ToUpper(coin.Symbol),
				coin.CurrentPrice, strings.ToUpper(currency),
				sign, coin.PriceChangePct24h,
				coin.TotalVolume/1e6,
			)
		}
		return c.JSON(mcpResult(id, sb.String()))

	case "get_trending_crypto":
		coins, err := h.client.GetTrending(c.Context())
		if err != nil {
			return c.JSON(mcpToolError(id, err.Error()))
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Trending Cryptocurrencies (last 24h)\n\n")
		for i, coin := range coins {
			fmt.Fprintf(&sb, "%d. %s (%s) — Rank #%d | Score: %d\n",
				i+1, coin.Name, strings.ToUpper(coin.Symbol), coin.MarketCapRank, coin.Score+1)
		}
		return c.JSON(mcpResult(id, sb.String()))

	case "get_coin_detail":
		coinID, _ := args["coin_id"].(string)
		if coinID == "" {
			return c.JSON(mcpError(id, -32602, "Invalid params", "coin_id is required"))
		}
		detail, err := h.client.GetCoinDetail(c.Context(), coinID, currency)
		if err != nil {
			return c.JSON(mcpToolError(id, err.Error()))
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "%s (%s) — Rank #%d\n", detail.Name, strings.ToUpper(detail.Symbol), detail.Rank)
		if detail.Description != "" {
			fmt.Fprintf(&sb, "\n%s\n", detail.Description)
		}
		if len(detail.Categories) > 0 {
			fmt.Fprintf(&sb, "\nCategories: %s\n", strings.Join(detail.Categories, ", "))
		}
		if detail.HomePage != "" {
			fmt.Fprintf(&sb, "Website: %s\n", detail.HomePage)
		}
		if detail.GenesisDate != "" {
			fmt.Fprintf(&sb, "Genesis: %s\n", detail.GenesisDate)
		}
		if detail.SentimentUp > 0 {
			fmt.Fprintf(&sb, "Bullish sentiment: %.1f%%\n", detail.SentimentUp)
		}
		if detail.Price != nil {
			p := detail.Price
			fmt.Fprintf(&sb, "\nPrice: %.6f %s | 24h: %.2f%% | MCap: %.2fB | ATH: %.2f\n",
				p.CurrentPrice, strings.ToUpper(currency),
				p.PriceChangePct24h, p.MarketCap/1e9, p.ATH)
		}
		return c.JSON(mcpResult(id, sb.String()))

	default:
		return c.JSON(mcpError(id, -32601, "Unknown tool", name))
	}
}

func mcpResult(id interface{}, text string) fiber.Map {
	return fiber.Map{"jsonrpc": "2.0", "id": id,
		"result": fiber.Map{"content": []fiber.Map{{"type": "text", "text": text}}}}
}

func mcpError(id interface{}, code int, msg, data string) fiber.Map {
	return fiber.Map{"jsonrpc": "2.0", "id": id,
		"error": fiber.Map{"code": code, "message": msg, "data": data}}
}

func mcpToolError(id interface{}, msg string) fiber.Map {
	return fiber.Map{"jsonrpc": "2.0", "id": id,
		"result": fiber.Map{"content": []fiber.Map{{"type": "text", "text": msg}}, "isError": true}}
}
