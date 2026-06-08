package handler

import (
	"encoding/json"
	"fmt"

	"sec-api/internal/client"

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

type mcpReq  struct { JSONRPC string `json:"jsonrpc"`; ID interface{} `json:"id"`; Method string `json:"method"`; Params json.RawMessage `json:"params"` }
type mcpResp struct { JSONRPC string `json:"jsonrpc"`; ID interface{} `json:"id"`; Result interface{} `json:"result,omitempty"`; Error *mcpErr `json:"error,omitempty"` }
type mcpErr  struct { Code int `json:"code"`; Message string `json:"message"` }

var mcpTools = []fiber.Map{
	{
		"name": "search_sec_company",
		"description": "Search US public companies in SEC EDGAR. Returns company name, CIK, ticker, exchange, and SIC code.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"q":     fiber.Map{"type": "string", "description": "Company name or ticker symbol"},
				"limit": fiber.Map{"type": "integer", "default": 10},
			},
			"required": []string{"q"},
		},
	},
	{
		"name": "get_sec_company_filings",
		"description": "Get SEC filings for a US public company by CIK number. Optionally filter by form type (10-K annual report, 10-Q quarterly, 8-K current report, DEF 14A proxy).",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"cik":   fiber.Map{"type": "string", "description": "SEC CIK number e.g. '789019' for Microsoft"},
				"form":  fiber.Map{"type": "string", "description": "Form type filter: 10-K, 10-Q, 8-K, DEF 14A, S-1"},
				"limit": fiber.Map{"type": "integer", "default": 10},
			},
			"required": []string{"cik"},
		},
	},
	{
		"name": "get_sec_financials",
		"description": "Get structured financial data from SEC XBRL filings. Returns historical values for a specific financial concept (revenue, net income, assets, etc.).",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"cik":     fiber.Map{"type": "string", "description": "SEC CIK number"},
				"concept": fiber.Map{"type": "string", "description": "XBRL concept in taxonomy/ConceptName format. Examples: us-gaap/Revenues, us-gaap/NetIncomeLoss, us-gaap/Assets, us-gaap/EarningsPerShareBasic"},
				"limit":   fiber.Map{"type": "integer", "default": 10},
			},
			"required": []string{"cik", "concept"},
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
			"serverInfo":      fiber.Map{"name": "sec-mcp", "version": "1.0.0"},
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
		if v, ok := args[k]; ok { return fmt.Sprintf("%v", v) }
		return ""
	}
	intArg := func(k string, def int) int {
		if v, ok := args[k]; ok {
			if n, ok := v.(float64); ok { return int(n) }
		}
		return def
	}

	switch params.Name {
	case "search_sec_company":
		companies, total, err := h.client.SearchCompanies(c.Context(), str("q"), intArg("limit", 10))
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: err.Error()}})
		}
		data, _ := json.Marshal(fiber.Map{"total": total, "results": companies})
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	case "get_sec_company_filings":
		filings, err := h.client.GetFilings(c.Context(), str("cik"), str("form"), intArg("limit", 10))
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: err.Error()}})
		}
		data, _ := json.Marshal(filings)
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	case "get_sec_financials":
		facts, err := h.client.GetFinancialFacts(c.Context(), str("cik"), str("concept"), intArg("limit", 10))
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32602, Message: err.Error()}})
		}
		data, _ := json.Marshal(facts)
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	default:
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32601, Message: "unknown tool: " + params.Name}})
	}
}

func toolResult(text string) fiber.Map {
	return fiber.Map{"content": []fiber.Map{{"type": "text", "text": text}}}
}
