// Package handler — MCP endpoint for GBIF Biodiversity API.
package handler

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
	"gbif-api/internal/client"
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
			"serverInfo":      fiber.Map{"name": "gbif-api-mcp", "version": "1.0.0"},
		}})

	case "tools/list":
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: req.ID, Result: fiber.Map{"tools": []fiber.Map{
			{
				"name":        "search_species",
				"description": "Search the GBIF backbone taxonomy for species, genera, families, etc. Returns scientific names, classification, and keys.",
				"inputSchema": fiber.Map{"type": "object", "required": []string{"query"}, "properties": fiber.Map{
					"query": fiber.Map{"type": "string", "description": "Species name or keyword (e.g. lion, Panthera, Felidae)"},
					"rank":  fiber.Map{"type": "string", "description": "Taxonomic rank filter: SPECIES, GENUS, FAMILY, ORDER, CLASS, PHYLUM, KINGDOM"},
					"limit": fiber.Map{"type": "integer", "description": "Max results (1-100, default 20)"},
				}},
			},
			{
				"name":        "get_species",
				"description": "Get full taxonomic details for a species by GBIF taxon key.",
				"inputSchema": fiber.Map{"type": "object", "required": []string{"key"}, "properties": fiber.Map{
					"key": fiber.Map{"type": "integer", "description": "GBIF taxon key (e.g. 5219404 for Panthera leo)"},
				}},
			},
			{
				"name":        "get_vernacular_names",
				"description": "Get common (vernacular) names for a species in a given language.",
				"inputSchema": fiber.Map{"type": "object", "required": []string{"key"}, "properties": fiber.Map{
					"key":  fiber.Map{"type": "integer", "description": "GBIF taxon key"},
					"lang": fiber.Map{"type": "string", "description": "Language code: eng, fra, deu, spa, etc. (default eng)"},
				}},
			},
			{
				"name":        "search_occurrences",
				"description": "Search wildlife observation records. Returns geo-located sightings with date, country, and observer data.",
				"inputSchema": fiber.Map{"type": "object", "properties": fiber.Map{
					"species_key":  fiber.Map{"type": "integer", "description": "GBIF species key (from search_species)"},
					"country_code": fiber.Map{"type": "string", "description": "ISO 2-letter country code (e.g. DE, KE, US)"},
					"year":         fiber.Map{"type": "integer", "description": "Filter by observation year (e.g. 2022)"},
					"limit":        fiber.Map{"type": "integer", "description": "Max results (1-50, default 20)"},
				}},
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
	str := func(key string) string {
		if v, ok := args[key]; ok {
			if s, ok := v.(string); ok {
				return strings.TrimSpace(s)
			}
		}
		return ""
	}
	intArg := func(key string) int {
		if v, ok := args[key]; ok {
			if f, ok := v.(float64); ok {
				return int(f)
			}
		}
		return 0
	}

	switch name {
	case "search_species":
		q := str("query")
		if q == "" {
			return fail("query is required")
		}
		result, err := cl.SearchSpecies(c.Context(), q, str("rank"), intArg("limit"), 0)
		if err != nil {
			return fail(err.Error())
		}
		return ok(result)

	case "get_species":
		key := intArg("key")
		if key <= 0 {
			return fail("key must be a positive integer")
		}
		species, err := cl.GetSpecies(c.Context(), key)
		if err != nil {
			return fail(err.Error())
		}
		return ok(species)

	case "get_vernacular_names":
		key := intArg("key")
		if key <= 0 {
			return fail("key must be a positive integer")
		}
		lang := str("lang")
		if lang == "" {
			lang = "eng"
		}
		names, err := cl.GetVernacularNames(c.Context(), key, lang)
		if err != nil {
			return fail(err.Error())
		}
		return ok(fiber.Map{"key": key, "language": lang, "count": len(names), "names": names})

	case "search_occurrences":
		speciesKey := intArg("species_key")
		country := str("country_code")
		year := intArg("year")
		limit := intArg("limit")
		if limit <= 0 {
			limit = 20
		}
		if speciesKey <= 0 && country == "" {
			return fail("at least species_key or country_code is required")
		}
		result, err := cl.SearchOccurrences(c.Context(), speciesKey, country, year, limit, 0)
		if err != nil {
			return fail(err.Error())
		}
		return ok(result)

	default:
		return c.JSON(mcpResponse{JSONRPC: "2.0", ID: id, Error: &mcpError{Code: -32601, Message: "unknown tool: " + name}})
	}
}
