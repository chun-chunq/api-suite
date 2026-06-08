package handler

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"wikidata-api/internal/client"
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
		"name":        "search_wikidata",
		"description": "Search Wikidata for entities (people, places, companies, concepts) by name. Returns entity IDs, labels, and descriptions.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"query":    fiber.Map{"type": "string", "description": "Search term, e.g. 'Berlin', 'Albert Einstein', 'Apple Inc'"},
				"language": fiber.Map{"type": "string", "description": "Language code (e.g. 'en', 'de', 'fr'). Default: en"},
				"type":     fiber.Map{"type": "string", "description": "'item' (default) or 'property'"},
				"limit":    fiber.Map{"type": "integer", "description": "Max results (1-50). Default 10."},
			},
			"required": []string{"query"},
		},
	},
	{
		"name":        "get_wikidata_entity",
		"description": "Get detailed information about a Wikidata entity by ID (e.g. Q42 = Douglas Adams, Q64 = Berlin). Returns label, description, aliases, instance-of, country, website, coordinates.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"entity_id": fiber.Map{"type": "string", "description": "Wikidata entity ID (e.g. Q42, Q64, Q312)"},
				"language":  fiber.Map{"type": "string", "description": "Language code. Default: en"},
			},
			"required": []string{"entity_id"},
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
				"serverInfo":      fiber.Map{"name": "wikidata-api", "version": "1.0.0"},
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

	switch name {
	case "search_wikidata":
		query, _ := args["query"].(string)
		if query == "" {
			return c.JSON(mcpError(id, -32602, "Invalid params", "query is required"))
		}
		lang, _ := args["language"].(string)
		if lang == "" {
			lang = "en"
		}
		entityType, _ := args["type"].(string)
		limit := 10
		if v, ok := args["limit"].(float64); ok {
			limit = int(v)
		}

		results, _, err := h.client.Search(c.Context(), query, lang, entityType, limit)
		if err != nil {
			return c.JSON(mcpToolError(id, err.Error()))
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Wikidata search results for %q (%d found):\n\n", query, len(results))
		for i, r := range results {
			fmt.Fprintf(&sb, "%d. [%s] %s", i+1, r.ID, r.Label)
			if r.Description != "" {
				fmt.Fprintf(&sb, " — %s", r.Description)
			}
			fmt.Fprintln(&sb)
			if len(r.Aliases) > 0 {
				fmt.Fprintf(&sb, "   Also known as: %s\n", strings.Join(r.Aliases, ", "))
			}
		}
		return c.JSON(mcpResult(id, sb.String()))

	case "get_wikidata_entity":
		entityID, _ := args["entity_id"].(string)
		if entityID == "" {
			return c.JSON(mcpError(id, -32602, "Invalid params", "entity_id is required"))
		}
		lang, _ := args["language"].(string)
		if lang == "" {
			lang = "en"
		}

		ent, err := h.client.GetEntity(c.Context(), entityID, lang)
		if err != nil {
			return c.JSON(mcpToolError(id, err.Error()))
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Wikidata Entity: %s (%s)\n", ent.Label, ent.ID)
		if ent.Description != "" {
			fmt.Fprintf(&sb, "Description: %s\n", ent.Description)
		}
		if len(ent.Aliases) > 0 {
			fmt.Fprintf(&sb, "Aliases: %s\n", strings.Join(ent.Aliases, ", "))
		}
		if len(ent.InstanceOf) > 0 {
			fmt.Fprintf(&sb, "Instance of: %s\n", strings.Join(ent.InstanceOf, ", "))
		}
		if ent.Country != "" {
			fmt.Fprintf(&sb, "Country: %s\n", ent.Country)
		}
		if ent.OfficialSite != "" {
			fmt.Fprintf(&sb, "Website: %s\n", ent.OfficialSite)
		}
		if ent.Coordinates != nil {
			fmt.Fprintf(&sb, "Coordinates: %.4f, %.4f\n", ent.Coordinates.Lat, ent.Coordinates.Lon)
		}
		fmt.Fprintf(&sb, "Wikidata URL: %s\n", ent.URL)
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
