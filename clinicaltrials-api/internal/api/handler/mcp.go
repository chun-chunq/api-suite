package handler

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gofiber/fiber/v2"
	"clinicaltrials-api/internal/client"
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
				"serverInfo":      fiber.Map{"name": "clinicaltrials-api", "version": "1.0.0"},
			},
		})

	case "tools/list":
		return c.JSON(mcpResponse{
			Jsonrpc: "2.0", ID: req.ID,
			Result: fiber.Map{
				"tools": []fiber.Map{
					{
						"name":        "search_clinical_trials",
						"description": "Search ClinicalTrials.gov for clinical studies. Filter by condition, drug, status, or phase. Returns trial details including conditions, interventions, sponsor, dates, and locations.",
						"inputSchema": fiber.Map{
							"type": "object",
							"properties": fiber.Map{
								"query":      fiber.Map{"type": "string", "description": "Search term (condition, drug name, sponsor, etc.)"},
								"status":     fiber.Map{"type": "string", "description": "Filter by status: RECRUITING, COMPLETED, NOT_YET_RECRUITING, ACTIVE_NOT_RECRUITING (leave empty for all)"},
								"phase":      fiber.Map{"type": "string", "description": "Filter by phase: PHASE1, PHASE2, PHASE3, PHASE4, EARLY_PHASE1 (leave empty for all)"},
								"limit":      fiber.Map{"type": "integer", "description": "Number of results (1-100, default 10)"},
								"page_token": fiber.Map{"type": "string", "description": "Pagination token from previous response"},
							},
						},
					},
					{
						"name":        "get_clinical_trial",
						"description": "Get detailed information about a specific clinical trial by its NCT ID.",
						"inputSchema": fiber.Map{
							"type":     "object",
							"required": []string{"nct_id"},
							"properties": fiber.Map{
								"nct_id": fiber.Map{"type": "string", "description": "NCT ID of the clinical trial (e.g. NCT04280705)"},
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

		switch params.Name {
		case "search_clinical_trials":
			query, _ := params.Arguments["query"].(string)
			status, _ := params.Arguments["status"].(string)
			phase, _ := params.Arguments["phase"].(string)
			pageToken, _ := params.Arguments["page_token"].(string)
			limit := 10
			if v, ok := params.Arguments["limit"]; ok {
				switch n := v.(type) {
				case float64:
					limit = int(n)
				case string:
					limit, _ = strconv.Atoi(n)
				}
			}
			result, err := cl.Search(c.Context(), query, status, phase, limit, pageToken)
			if err != nil {
				return c.JSON(mcpErr(req.ID, -32603, err.Error()))
			}
			return c.JSON(mcpResponse{Jsonrpc: "2.0", ID: req.ID,
				Result: fiber.Map{"content": []fiber.Map{{"type": "text", "text": toJSON(result)}}}})

		case "get_clinical_trial":
			nctID, _ := params.Arguments["nct_id"].(string)
			if strings.TrimSpace(nctID) == "" {
				return c.JSON(mcpErr(req.ID, -32602, "nct_id is required"))
			}
			study, err := cl.GetStudy(c.Context(), nctID)
			if err != nil {
				return c.JSON(mcpErr(req.ID, -32603, err.Error()))
			}
			return c.JSON(mcpResponse{Jsonrpc: "2.0", ID: req.ID,
				Result: fiber.Map{"content": []fiber.Map{{"type": "text", "text": toJSON(study)}}}})

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
