package handler

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v2"
	"namepredict-api/internal/client"
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
				"serverInfo":      fiber.Map{"name": "namepredict-api", "version": "1.0.0"},
			},
		})

	case "tools/list":
		return c.JSON(mcpResponse{
			Jsonrpc: "2.0", ID: req.ID,
			Result: fiber.Map{
				"tools": []fiber.Map{
					{
						"name":        "predict_name_all",
						"description": "Predict age, gender, and nationality from a first name using Agify, Genderize, and Nationalize APIs concurrently.",
						"inputSchema": fiber.Map{
							"type":     "object",
							"required": []string{"name"},
							"properties": fiber.Map{
								"name":       fiber.Map{"type": "string", "description": "First name to analyze"},
								"country_id": fiber.Map{"type": "string", "description": "Optional ISO 3166-1 alpha-2 country code for localized age/gender predictions (e.g. US, DE, FR)"},
							},
						},
					},
					{
						"name":        "predict_age",
						"description": "Predict the likely age of a person based on their first name using Agify.io.",
						"inputSchema": fiber.Map{
							"type":     "object",
							"required": []string{"name"},
							"properties": fiber.Map{
								"name":       fiber.Map{"type": "string", "description": "First name"},
								"country_id": fiber.Map{"type": "string", "description": "Optional country code (e.g. US, DE)"},
							},
						},
					},
					{
						"name":        "predict_gender",
						"description": "Predict the likely gender of a person based on their first name using Genderize.io.",
						"inputSchema": fiber.Map{
							"type":     "object",
							"required": []string{"name"},
							"properties": fiber.Map{
								"name":       fiber.Map{"type": "string", "description": "First name"},
								"country_id": fiber.Map{"type": "string", "description": "Optional country code (e.g. US, DE)"},
							},
						},
					},
					{
						"name":        "predict_nationality",
						"description": "Predict the likely nationality/country of origin based on a first name using Nationalize.io.",
						"inputSchema": fiber.Map{
							"type":     "object",
							"required": []string{"name"},
							"properties": fiber.Map{
								"name": fiber.Map{"type": "string", "description": "First name"},
							},
						},
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

		getName := func() string {
			if v, ok := params.Arguments["name"]; ok {
				return strings.TrimSpace(v.(string))
			}
			return ""
		}
		getCountry := func() string {
			if v, ok := params.Arguments["country_id"]; ok {
				return strings.ToUpper(strings.TrimSpace(v.(string)))
			}
			return ""
		}

		switch params.Name {
		case "predict_name_all":
			n := getName()
			if n == "" {
				return c.JSON(mcpErr(req.ID, -32602, "name is required"))
			}
			result, err := cl.GetAll(c.Context(), n, getCountry())
			if err != nil {
				return c.JSON(mcpErr(req.ID, -32603, err.Error()))
			}
			return c.JSON(mcpResponse{Jsonrpc: "2.0", ID: req.ID,
				Result: fiber.Map{"content": []fiber.Map{{"type": "text", "text": toJSON(result)}}}})

		case "predict_age":
			n := getName()
			if n == "" {
				return c.JSON(mcpErr(req.ID, -32602, "name is required"))
			}
			result, err := cl.GetAge(c.Context(), n, getCountry())
			if err != nil {
				return c.JSON(mcpErr(req.ID, -32603, err.Error()))
			}
			return c.JSON(mcpResponse{Jsonrpc: "2.0", ID: req.ID,
				Result: fiber.Map{"content": []fiber.Map{{"type": "text", "text": toJSON(result)}}}})

		case "predict_gender":
			n := getName()
			if n == "" {
				return c.JSON(mcpErr(req.ID, -32602, "name is required"))
			}
			result, err := cl.GetGender(c.Context(), n, getCountry())
			if err != nil {
				return c.JSON(mcpErr(req.ID, -32603, err.Error()))
			}
			return c.JSON(mcpResponse{Jsonrpc: "2.0", ID: req.ID,
				Result: fiber.Map{"content": []fiber.Map{{"type": "text", "text": toJSON(result)}}}})

		case "predict_nationality":
			n := getName()
			if n == "" {
				return c.JSON(mcpErr(req.ID, -32602, "name is required"))
			}
			result, err := cl.GetNationality(c.Context(), n)
			if err != nil {
				return c.JSON(mcpErr(req.ID, -32603, err.Error()))
			}
			return c.JSON(mcpResponse{Jsonrpc: "2.0", ID: req.ID,
				Result: fiber.Map{"content": []fiber.Map{{"type": "text", "text": toJSON(result)}}}})

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
