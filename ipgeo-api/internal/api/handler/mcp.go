package handler

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"ipgeo-api/internal/client"
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
		"name":        "lookup_ip",
		"description": "Geolocate an IP address: country, city, coordinates, ISP, ASN. Also detects VPN/proxy, Tor exit nodes, and hosting/datacenter IPs.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"ip": fiber.Map{"type": "string", "description": "IPv4 or IPv6 address. Use 'self' to look up your own IP."},
			},
			"required": []string{"ip"},
		},
	},
	{
		"name":        "lookup_ips_batch",
		"description": "Geolocate up to 100 IP addresses in a single request. Returns country, city, ISP, and proxy/VPN detection for each.",
		"inputSchema": fiber.Map{
			"type": "object",
			"properties": fiber.Map{
				"ips": fiber.Map{
					"type":  "array",
					"items": fiber.Map{"type": "string"},
					"description": "List of IP addresses (max 100)",
				},
			},
			"required": []string{"ips"},
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
				"serverInfo":      fiber.Map{"name": "ipgeo-api", "version": "1.0.0"},
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
	case "lookup_ip":
		ip, _ := args["ip"].(string)
		if ip == "" {
			return c.JSON(mcpError(id, -32602, "Invalid params", "ip is required"))
		}
		result, err := h.client.Lookup(c.Context(), ip)
		if err != nil {
			return c.JSON(mcpToolError(id, err.Error()))
		}
		text := formatGeoResult(result)
		return c.JSON(mcpResult(id, text))

	case "lookup_ips_batch":
		arr, _ := args["ips"].([]interface{})
		if len(arr) == 0 {
			return c.JSON(mcpError(id, -32602, "Invalid params", "ips is required"))
		}
		ips := make([]string, 0, len(arr))
		for _, v := range arr {
			if s, ok := v.(string); ok {
				ips = append(ips, s)
			}
		}
		results, err := h.client.LookupBatch(c.Context(), ips)
		if err != nil {
			return c.JSON(mcpToolError(id, err.Error()))
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "Batch IP Geolocation Results (%d IPs)\n\n", len(results))
		for i, r := range results {
			fmt.Fprintf(&sb, "%d. %s\n%s\n", i+1, r.IP, formatGeoResult(&r))
		}
		return c.JSON(mcpResult(id, sb.String()))

	default:
		return c.JSON(mcpError(id, -32601, "Unknown tool", name))
	}
}

func formatGeoResult(r *client.GeoResult) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "IP: %s\n", r.IP)
	if r.Country != "" {
		fmt.Fprintf(&sb, "Location: %s, %s, %s (%s)\n", r.City, r.RegionName, r.Country, r.CountryCode)
	}
	if r.ZIP != "" {
		fmt.Fprintf(&sb, "ZIP: %s\n", r.ZIP)
	}
	if r.Lat != 0 || r.Lon != 0 {
		fmt.Fprintf(&sb, "Coordinates: %.4f, %.4f\n", r.Lat, r.Lon)
	}
	if r.Timezone != "" {
		fmt.Fprintf(&sb, "Timezone: %s\n", r.Timezone)
	}
	if r.ISP != "" {
		fmt.Fprintf(&sb, "ISP: %s\n", r.ISP)
	}
	if r.AS != "" {
		fmt.Fprintf(&sb, "ASN: %s (%s)\n", r.AS, r.ASName)
	}
	flags := []string{}
	if r.Proxy {
		flags = append(flags, "⚠️ VPN/Proxy/Tor detected")
	}
	if r.Hosting {
		flags = append(flags, "🏢 Datacenter/Hosting IP")
	}
	if r.Mobile {
		flags = append(flags, "📱 Mobile network")
	}
	if len(flags) > 0 {
		fmt.Fprintf(&sb, "Flags: %s\n", strings.Join(flags, " | "))
	}
	return sb.String()
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
