package handler

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"openfda-api/internal/client"
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
		"name":        "search_drug_labels",
		"description": "Search FDA drug labels by brand name, generic name, or active substance. Returns indications, warnings, dosage, and adverse reactions from official FDA label.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"query":  fiber.Map{"type": "string", "description": "Drug name (brand or generic), e.g. 'aspirin', 'ibuprofen', 'Lipitor'"},
				"limit":  fiber.Map{"type": "integer", "description": "Max results (1-100). Default 5."},
				"skip":   fiber.Map{"type": "integer", "description": "Pagination offset. Default 0."},
			},
			"required": []string{"query"},
		},
	},
	{
		"name":        "search_adverse_events",
		"description": "Search the FDA Adverse Event Reporting System (FAERS) for reports involving a specific drug. Returns serious adverse events, reactions, and patient data.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"drug":  fiber.Map{"type": "string", "description": "Drug name as reported (e.g. 'IBUPROFEN', 'ASPIRIN', 'HUMIRA')"},
				"limit": fiber.Map{"type": "integer", "description": "Max results (1-100). Default 5."},
				"skip":  fiber.Map{"type": "integer", "description": "Pagination offset. Default 0."},
			},
			"required": []string{"drug"},
		},
	},
	{
		"name":        "search_drug_recalls",
		"description": "Search FDA drug enforcement/recall records by company name, product, or recall class (I=most serious, II, III).",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"query":          fiber.Map{"type": "string", "description": "Company name or product to search"},
				"classification": fiber.Map{"type": "string", "description": "Recall class: I, II, or III (optional)"},
				"limit":          fiber.Map{"type": "integer", "description": "Max results (1-100). Default 5."},
				"skip":           fiber.Map{"type": "integer", "description": "Pagination offset. Default 0."},
			},
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
				"serverInfo":      fiber.Map{"name": "openfda-api", "version": "1.0.0"},
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

	intArg := func(key string, def int) int {
		if v, ok := args[key].(float64); ok {
			return int(v)
		}
		return def
	}

	switch name {
	case "search_drug_labels":
		query, _ := args["query"].(string)
		if query == "" {
			return c.JSON(mcpError(id, -32602, "Invalid params", "query is required"))
		}
		limit := intArg("limit", 5)
		skip := intArg("skip", 0)
		res, err := h.client.SearchDrugLabels(c.Context(), query, limit, skip)
		if err != nil {
			return c.JSON(mcpToolError(id, err.Error()))
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "FDA Drug Labels for %q (%d total)\n\n", query, res.Total)
		for i, dl := range res.Items {
			fmt.Fprintf(&sb, "%d. %s", i+1, dl.BrandName)
			if dl.GenericName != "" && dl.GenericName != dl.BrandName {
				fmt.Fprintf(&sb, " (%s)", dl.GenericName)
			}
			if dl.LabelerName != "" {
				fmt.Fprintf(&sb, " — %s", dl.LabelerName)
			}
			fmt.Fprintln(&sb)
			if dl.ProductType != "" {
				fmt.Fprintf(&sb, "   Type: %s\n", dl.ProductType)
			}
			if len(dl.Route) > 0 {
				fmt.Fprintf(&sb, "   Route: %s\n", strings.Join(dl.Route, ", "))
			}
			if dl.Indications != "" {
				fmt.Fprintf(&sb, "   Indications: %s\n", dl.Indications)
			}
			if dl.Warnings != "" {
				fmt.Fprintf(&sb, "   Warnings: %s\n", dl.Warnings)
			}
			fmt.Fprintln(&sb)
		}
		return c.JSON(mcpResult(id, sb.String()))

	case "search_adverse_events":
		drug, _ := args["drug"].(string)
		if drug == "" {
			return c.JSON(mcpError(id, -32602, "Invalid params", "drug is required"))
		}
		limit := intArg("limit", 5)
		skip := intArg("skip", 0)
		res, err := h.client.SearchAdverseEvents(c.Context(), drug, limit, skip)
		if err != nil {
			return c.JSON(mcpToolError(id, err.Error()))
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "FDA Adverse Events for %q (%d total reports)\n\n", drug, res.Total)
		for i, ae := range res.Items {
			fmt.Fprintf(&sb, "%d. Report %s (%s)", i+1, ae.ReportID, ae.ReceiveDate)
			if ae.Serious {
				fmt.Fprintf(&sb, " [SERIOUS")
				if len(ae.SeriousReasons) > 0 {
					fmt.Fprintf(&sb, ": %s", strings.Join(ae.SeriousReasons, ", "))
				}
				fmt.Fprint(&sb, "]")
			}
			fmt.Fprintln(&sb)
			if ae.Sex != "" || ae.Age != nil {
				patient := "Patient:"
				if ae.Age != nil {
					patient += fmt.Sprintf(" age %.0f", *ae.Age)
				}
				if ae.Sex != "" {
					patient += " " + ae.Sex
				}
				fmt.Fprintf(&sb, "   %s\n", patient)
			}
			if len(ae.Reactions) > 0 {
				fmt.Fprintf(&sb, "   Reactions: %s\n", strings.Join(ae.Reactions, "; "))
			}
			if len(ae.Drugs) > 0 {
				fmt.Fprintf(&sb, "   Drugs involved: %s\n", strings.Join(ae.Drugs, ", "))
			}
			fmt.Fprintln(&sb)
		}
		return c.JSON(mcpResult(id, sb.String()))

	case "search_drug_recalls":
		query, _ := args["query"].(string)
		class, _ := args["classification"].(string)
		limit := intArg("limit", 5)
		skip := intArg("skip", 0)
		res, err := h.client.SearchRecalls(c.Context(), query, class, limit, skip)
		if err != nil {
			return c.JSON(mcpToolError(id, err.Error()))
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "FDA Drug Recalls (%d total)\n\n", res.Total)
		for i, r := range res.Items {
			fmt.Fprintf(&sb, "%d. [%s] %s — %s\n", i+1, r.Classification, r.RecallingFirm, r.Status)
			fmt.Fprintf(&sb, "   Product: %s\n", r.ProductDesc)
			fmt.Fprintf(&sb, "   Reason: %s\n", r.Reason)
			fmt.Fprintf(&sb, "   Initiated: %s\n", r.InitiationDate)
			fmt.Fprintln(&sb)
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
