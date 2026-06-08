package handler

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"worldbank-api/internal/client"
)

type mcpRequest struct {
	Jsonrpc string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	Jsonrpc string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpError   `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func mcpErr(id interface{}, code int, msg string) mcpResponse {
	return mcpResponse{Jsonrpc: "2.0", ID: id, Error: &mcpError{Code: code, Message: msg}}
}

// MCP handles POST /mcp
func MCP(c *fiber.Ctx, cl *client.Client) error {
	var req mcpRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(mcpErr(nil, -32700, "parse error"))
	}

	switch req.Method {
	case "initialize":
		return c.JSON(mcpResponse{
			Jsonrpc: "2.0", ID: req.ID,
			Result: fiber.Map{
				"protocolVersion": "2024-11-05",
				"capabilities":    fiber.Map{"tools": fiber.Map{}},
				"serverInfo":      fiber.Map{"name": "worldbank-api", "version": "1.0.0"},
			},
		})

	case "tools/list":
		return c.JSON(mcpResponse{
			Jsonrpc: "2.0", ID: req.ID,
			Result: fiber.Map{
				"tools": []fiber.Map{
					{
						"name":        "get_country_info",
						"description": "Get country metadata from the World Bank including region, income level, capital city, and coordinates.",
						"inputSchema": fiber.Map{
							"type":     "object",
							"required": []string{"country_code"},
							"properties": fiber.Map{
								"country_code": fiber.Map{"type": "string", "description": "ISO 3166-1 alpha-2 country code (e.g. DE, US, CN)"},
							},
						},
					},
					{
						"name":        "get_indicator_data",
						"description": "Fetch World Bank economic/development indicator data for a country. Returns historical time series (e.g. GDP, population, inflation, CO2 emissions). Use list_indicators to find available indicator IDs.",
						"inputSchema": fiber.Map{
							"type":     "object",
							"required": []string{"country", "indicator"},
							"properties": fiber.Map{
								"country":    fiber.Map{"type": "string", "description": "ISO2 country code (e.g. DE) or 'all' for global data"},
								"indicator":  fiber.Map{"type": "string", "description": "World Bank indicator ID (e.g. NY.GDP.MKTP.CD for GDP). Use list_indicators for options."},
								"start_year": fiber.Map{"type": "integer", "description": "Start year for time range (optional)"},
								"end_year":   fiber.Map{"type": "integer", "description": "End year for time range (optional)"},
							},
						},
					},
					{
						"name":        "list_indicators",
						"description": "List commonly used World Bank indicators with their IDs. Use these IDs in get_indicator_data.",
						"inputSchema": fiber.Map{"type": "object", "properties": fiber.Map{}},
					},
				},
			},
		})

	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return c.JSON(mcpErr(req.ID, -32602, "invalid params"))
		}

		switch params.Name {
		case "get_country_info":
			code, _ := params.Arguments["country_code"].(string)
			if code == "" {
				return c.JSON(mcpErr(req.ID, -32602, "country_code is required"))
			}
			result, err := cl.GetCountry(c.Context(), code)
			if err != nil {
				return c.JSON(mcpErr(req.ID, -32603, err.Error()))
			}
			return c.JSON(mcpResponse{Jsonrpc: "2.0", ID: req.ID,
				Result: fiber.Map{"content": []fiber.Map{{"type": "text", "text": toJSON(result)}}}})

		case "get_indicator_data":
			country, _ := params.Arguments["country"].(string)
			indicator, _ := params.Arguments["indicator"].(string)
			if country == "" || indicator == "" {
				return c.JSON(mcpErr(req.ID, -32602, "country and indicator are required"))
			}
			startYear, endYear := 0, 0
			if v, ok := params.Arguments["start_year"]; ok {
				switch n := v.(type) {
				case float64:
					startYear = int(n)
				case string:
					startYear, _ = strconv.Atoi(n)
				}
			}
			if v, ok := params.Arguments["end_year"]; ok {
				switch n := v.(type) {
				case float64:
					endYear = int(n)
				case string:
					endYear, _ = strconv.Atoi(n)
				}
			}
			result, err := cl.GetIndicator(c.Context(), country, indicator, startYear, endYear)
			if err != nil {
				return c.JSON(mcpErr(req.ID, -32603, err.Error()))
			}
			return c.JSON(mcpResponse{Jsonrpc: "2.0", ID: req.ID,
				Result: fiber.Map{"content": []fiber.Map{{"type": "text", "text": toJSON(result)}}}})

		case "list_indicators":
			indicators := cl.CommonIndicators()
			out := fiber.Map{"count": len(indicators), "indicators": indicators}
			return c.JSON(mcpResponse{Jsonrpc: "2.0", ID: req.ID,
				Result: fiber.Map{"content": []fiber.Map{{"type": "text", "text": toJSON(out)}}}})

		default:
			return c.JSON(mcpErr(req.ID, -32601, "unknown tool: "+params.Name))
		}

	default:
		return c.JSON(mcpErr(req.ID, -32601, "method not found"))
	}
}

func toJSON(v interface{}) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return ""
}
