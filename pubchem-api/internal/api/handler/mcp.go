// Package handler — MCP endpoint for PubChem API.
package handler

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"pubchem-api/internal/client"
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

// MCP handles POST /mcp
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
			JSONRPC: "2.0", ID: req.ID,
			Result: fiber.Map{
				"protocolVersion": "2024-11-05",
				"capabilities":    fiber.Map{"tools": fiber.Map{}},
				"serverInfo":      fiber.Map{"name": "pubchem-api-mcp", "version": "1.0.0"},
			},
		})

	case "tools/list":
		return c.JSON(mcpResponse{
			JSONRPC: "2.0", ID: req.ID,
			Result: fiber.Map{"tools": []fiber.Map{
				{
					"name":        "search_compound",
					"description": "Search PubChem for chemical compounds by name. Returns CIDs (Compound IDs).",
					"inputSchema": fiber.Map{
						"type": "object", "required": []string{"name"},
						"properties": fiber.Map{
							"name":  fiber.Map{"type": "string", "description": "Compound name, e.g. aspirin, caffeine, glucose"},
							"limit": fiber.Map{"type": "integer", "description": "Max results (1-20, default 10)"},
						},
					},
				},
				{
					"name":        "get_compound_by_cid",
					"description": "Get detailed molecular properties of a compound by its PubChem CID.",
					"inputSchema": fiber.Map{
						"type": "object", "required": []string{"cid"},
						"properties": fiber.Map{
							"cid": fiber.Map{"type": "integer", "description": "PubChem Compound ID (CID)"},
						},
					},
				},
				{
					"name":        "get_compound_by_name",
					"description": "Get full compound details for the best matching compound name. Combines search + property lookup.",
					"inputSchema": fiber.Map{
						"type": "object", "required": []string{"name"},
						"properties": fiber.Map{
							"name": fiber.Map{"type": "string", "description": "Compound name, e.g. aspirin, ibuprofen, ethanol"},
						},
					},
				},
				{
					"name":        "get_compound_synonyms",
					"description": "Get synonym names (trade names, common names, IUPAC name) for a compound by CID.",
					"inputSchema": fiber.Map{
						"type": "object", "required": []string{"cid"},
						"properties": fiber.Map{
							"cid":   fiber.Map{"type": "integer", "description": "PubChem Compound ID"},
							"limit": fiber.Map{"type": "integer", "description": "Max synonyms (1-50, default 20)"},
						},
					},
				},
				{
					"name":        "get_compound_description",
					"description": "Get a textual description/summary of a compound from PubChem.",
					"inputSchema": fiber.Map{
						"type": "object", "required": []string{"cid"},
						"properties": fiber.Map{
							"cid": fiber.Map{"type": "integer", "description": "PubChem Compound ID"},
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

	getCID := func() (int64, error) {
		if v, ok := args["cid"]; ok {
			switch n := v.(type) {
			case float64:
				return int64(n), nil
			case int64:
				return n, nil
			case string:
				return strconv.ParseInt(strings.TrimSpace(n), 10, 64)
			}
		}
		return 0, fmt.Errorf("cid is required")
	}

	switch name {
	case "search_compound":
		n, _ := args["name"].(string)
		if n = strings.TrimSpace(n); n == "" {
			return fail("name is required")
		}
		limit := 10
		if v, ok := args["limit"]; ok {
			if f, ok := v.(float64); ok {
				limit = int(f)
			}
		}
		sr, err := cl.SearchByName(c.Context(), n, limit)
		if err != nil {
			return fail(err.Error())
		}
		return ok(sr)

	case "get_compound_by_cid":
		cid, err := getCID()
		if err != nil {
			return fail("cid is required")
		}
		compound, err := cl.GetByCID(c.Context(), cid)
		if err != nil {
			return fail(err.Error())
		}
		return ok(compound)

	case "get_compound_by_name":
		n, _ := args["name"].(string)
		if n = strings.TrimSpace(n); n == "" {
			return fail("name is required")
		}
		compound, err := cl.GetByName(c.Context(), n)
		if err != nil {
			return fail(err.Error())
		}
		return ok(compound)

	case "get_compound_synonyms":
		cid, err := getCID()
		if err != nil {
			return fail("cid is required")
		}
		limit := 20
		if v, ok := args["limit"]; ok {
			if f, ok := v.(float64); ok {
				limit = int(f)
			}
		}
		synonyms, err := cl.GetSynonyms(c.Context(), cid, limit)
		if err != nil {
			return fail(err.Error())
		}
		return ok(fiber.Map{"cid": cid, "count": len(synonyms), "synonyms": synonyms})

	case "get_compound_description":
		cid, err := getCID()
		if err != nil {
			return fail("cid is required")
		}
		desc, err := cl.GetDescription(c.Context(), cid)
		if err != nil {
			return fail(err.Error())
		}
		return ok(fiber.Map{"cid": cid, "description": desc})

	default:
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: id,
			Error: &mcpError{Code: -32601, Message: "unknown tool: " + name}})
	}
}
