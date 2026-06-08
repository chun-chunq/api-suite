package handler

import (
	"encoding/json"
	"fmt"

	"research-api/internal/client"

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

type mcpReq struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type mcpResp struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpErr     `json:"error,omitempty"`
}
type mcpErr struct{ Code int `json:"code"`; Message string `json:"message"` }

var mcpTools = []fiber.Map{
	{
		"name":        "search_research_papers",
		"description": "Search 250M+ scholarly publications via OpenAlex. Filter by topic, author, year, open access status, or publication type. Returns title, abstract, authors, journal, citations, and open access PDF links.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"q":          fiber.Map{"type": "string", "description": "Research topic or keyword search"},
				"author":     fiber.Map{"type": "string", "description": "Author name filter"},
				"yearFrom":   fiber.Map{"type": "integer", "description": "Minimum publication year"},
				"yearTo":     fiber.Map{"type": "integer", "description": "Maximum publication year"},
				"openAccess": fiber.Map{"type": "boolean", "description": "Only return open access papers with free PDF"},
				"type":       fiber.Map{"type": "string", "description": "Work type: journal-article, book, dataset, preprint"},
				"sort":       fiber.Map{"type": "string", "enum": []string{"cited_by_count", "publication_date"}, "description": "Sort order"},
				"limit":      fiber.Map{"type": "integer", "default": 10, "maximum": 50},
			},
		},
	},
	{
		"name":        "lookup_paper_by_doi",
		"description": "Get full paper details by DOI (Digital Object Identifier). Returns title, abstract, authors, citations, open access URL.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"doi": fiber.Map{"type": "string", "description": "DOI string e.g. '10.1038/s41586-021-03819-2' or full URL"},
			},
			"required": []string{"doi"},
		},
	},
	{
		"name":        "search_research_institutions",
		"description": "Search universities and research institutions. Returns country, type, publication count, and citation count.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"q":     fiber.Map{"type": "string", "description": "Institution name e.g. 'MIT', 'ETH Zurich'"},
				"limit": fiber.Map{"type": "integer", "default": 10},
			},
			"required": []string{"q"},
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
			"serverInfo":      fiber.Map{"name": "research-mcp", "version": "1.0.0"},
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
	case "search_research_papers":
		q := client.WorkSearchQuery{
			Query:          str("q"),
			Author:         str("author"),
			Type:           str("type"),
			SortBy:         str("sort"),
			OpenAccessOnly: str("openAccess") == "true",
			YearFrom:       intArg("yearFrom", 0),
			YearTo:         intArg("yearTo", 0),
			MaxResults:     intArg("limit", 10),
		}
		result, err := h.client.SearchWorks(c.Context(), q)
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: err.Error()}})
		}
		data, _ := json.Marshal(result)
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	case "lookup_paper_by_doi":
		work, err := h.client.GetWorkByDOI(c.Context(), str("doi"))
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: err.Error()}})
		}
		if work == nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: "not found"}})
		}
		data, _ := json.Marshal(work)
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	case "search_research_institutions":
		institutions, total, err := h.client.SearchInstitutions(c.Context(), str("q"), intArg("limit", 10))
		if err != nil {
			return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32000, Message: err.Error()}})
		}
		data, _ := json.Marshal(fiber.Map{"total": total, "results": institutions})
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Result: toolResult(string(data))})

	default:
		return c.JSON(mcpResp{JSONRPC: "2.0", ID: req.ID, Error: &mcpErr{Code: -32601, Message: "unknown tool: " + params.Name}})
	}
}

func toolResult(text string) fiber.Map {
	return fiber.Map{"content": []fiber.Map{{"type": "text", "text": text}}}
}
