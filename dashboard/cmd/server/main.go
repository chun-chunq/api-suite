package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type APITarget struct {
	Name        string
	Port        string
	AdminSecret string
	Color       string
	Icon        string
}

var apis []APITarget

func main() {
	password := envOr("DASHBOARD_PASSWORD", "admin123")
	adminSecret := envOr("ADMIN_SECRET", "admin-secret-aendere-mich")
	port := envOr("PORT", "8088")

	level, _ := zerolog.ParseLevel(envOr("LOG_LEVEL", "info"))
	logger := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).
		With().Timestamp().Logger().Level(level)
	log.Logger = logger

	// All APIs to aggregate — add/remove here as APIs come online
	apis = []APITarget{
		{Name: "Insolvency", Port: "8080", AdminSecret: adminSecret, Color: "#e74c3c", Icon: "⚖️"},
		{Name: "ZVG (Foreclosures)", Port: "8081", AdminSecret: adminSecret, Color: "#e67e22", Icon: "🏠"},
		{Name: "TED (EU Procurement)", Port: "8082", AdminSecret: adminSecret, Color: "#f39c12", Icon: "📋"},
		{Name: "DPMA (Trademarks)", Port: "8083", AdminSecret: adminSecret, Color: "#27ae60", Icon: "™️"},
		{Name: "Sanctions", Port: "8084", AdminSecret: adminSecret, Color: "#8e44ad", Icon: "🚫"},
		{Name: "Safety Gate (Recalls)", Port: "8085", AdminSecret: adminSecret, Color: "#2980b9", Icon: "⚠️"},
		{Name: "Zefix (Swiss Cos)", Port: "8086", AdminSecret: adminSecret, Color: "#16a085", Icon: "🇨🇭"},
		{Name: "BaFin (Finance)", Port: "8087", AdminSecret: adminSecret, Color: "#c0392b", Icon: "🏦"},
		{Name: "GLEIF (LEI)", Port: "8089", AdminSecret: adminSecret, Color: "#1a6fa8", Icon: "🏛"},
		{Name: "CORDIS (EU Grants)", Port: "8090", AdminSecret: adminSecret, Color: "#2e7d32", Icon: "🔬"},
		{Name: "Handelsregister", Port: "8092", AdminSecret: adminSecret, Color: "#7b1fa2", Icon: "📜"},
		{Name: "EUIPO (EU Trademark)", Port: "8093", AdminSecret: adminSecret, Color: "#1565c0", Icon: "🇪🇺"},
		{Name: "French Companies (SIRENE)", Port: "8094", AdminSecret: adminSecret, Color: "#0d47a1", Icon: "🇫🇷"},
		{Name: "UK Companies House", Port: "8095", AdminSecret: adminSecret, Color: "#b71c1c", Icon: "🇬🇧"},
		{Name: "Research (OpenAlex)", Port: "8096", AdminSecret: adminSecret, Color: "#00695c", Icon: "🔬"},
		{Name: "GDPR Fines Tracker", Port: "8097", AdminSecret: adminSecret, Color: "#ad1457", Icon: "🔏"},
		{Name: "SEC EDGAR (US Filings)", Port: "8098", AdminSecret: adminSecret, Color: "#1a237e", Icon: "🏛"},
		{Name: "Food & Nutrition (OFF)", Port: "8099", AdminSecret: adminSecret, Color: "#2e7d32", Icon: "🥗"},
		{Name: "Aviation (OpenSky)", Port: "8100", AdminSecret: adminSecret, Color: "#0277bd", Icon: "✈️"},
		{Name: "Weather (Open-Meteo)", Port: "8101", AdminSecret: adminSecret, Color: "#00838f", Icon: "🌤️"},
		{Name: "Currency (ECB FX)", Port: "8102", AdminSecret: adminSecret, Color: "#558b2f", Icon: "💱"},
		{Name: "OpenFDA (Drug Data)", Port: "8103", AdminSecret: adminSecret, Color: "#e53935", Icon: "💊"},
		{Name: "Wikidata (Knowledge Graph)", Port: "8104", AdminSecret: adminSecret, Color: "#006699", Icon: "🌐"},
		{Name: "Crypto (CoinGecko)", Port: "8105", AdminSecret: adminSecret, Color: "#f57f17", Icon: "₿"},
		{Name: "IP Geolocation", Port: "8107", AdminSecret: adminSecret, Color: "#37474f", Icon: "🌍"},
		{Name: "VAT Validation (VIES)", Port: "8108", AdminSecret: adminSecret, Color: "#1b5e20", Icon: "🧾"},
		{Name: "REST Countries", Port: "8109", AdminSecret: adminSecret, Color: "#4527a0", Icon: "🌎"},
		{Name: "PubChem (Chemistry)", Port: "8110", AdminSecret: adminSecret, Color: "#00695c", Icon: "⚗️"},
		{Name: "NASA Open Data", Port: "8111", AdminSecret: adminSecret, Color: "#0d47a1", Icon: "🚀"},
		{Name: "Air Quality (Open-Meteo)", Port: "8113", AdminSecret: adminSecret, Color: "#33691e", Icon: "🌬️"},
		{Name: "FX History (Frankfurter)", Port: "8114", AdminSecret: adminSecret, Color: "#bf360c", Icon: "📈"},
		{Name: "Biodiversity (GBIF)", Port: "8115", AdminSecret: adminSecret, Color: "#2e7d32", Icon: "🦁"},
		{Name: "Name Prediction", Port: "8119", AdminSecret: adminSecret, Color: "#880e4f", Icon: "👤"},
		{Name: "World Bank Data", Port: "8120", AdminSecret: adminSecret, Color: "#1a237e", Icon: "🌍"},
		{Name: "ClinicalTrials.gov", Port: "8121", AdminSecret: adminSecret, Color: "#880e4f", Icon: "🧬"},
		{Name: "API Gateway", Port: "8000", AdminSecret: adminSecret, Color: "#212121", Icon: "🔀"},
	}

	app := fiber.New(fiber.Config{
		AppName:               "dashboard",
		DisableStartupMessage: true,
	})
	app.Use(recover.New())

	// Auth middleware
	app.Use(func(c *fiber.Ctx) error {
		if c.Path() == "/login" || c.Path() == "/logout" {
			return c.Next()
		}
		cookie := c.Cookies("dash_auth")
		if cookie != password {
			return c.Redirect("/login")
		}
		return c.Next()
	})

	app.Get("/login", handleLogin)
	app.Post("/login", func(c *fiber.Ctx) error {
		pwd := c.FormValue("password")
		if pwd != password {
			return c.Status(401).SendString(loginPage("❌ Wrong password"))
		}
		c.Cookie(&fiber.Cookie{
			Name:     "dash_auth",
			Value:    password,
			Expires:  time.Now().Add(24 * time.Hour),
			HTTPOnly: true,
			SameSite: "Lax",
		})
		return c.Redirect("/")
	})
	app.Get("/logout", func(c *fiber.Ctx) error {
		c.ClearCookie("dash_auth")
		return c.Redirect("/login")
	})

	app.Get("/", handleDashboard)
	app.Get("/api/stats", handleStats)

	logger.Info().Str("port", port).Msg("dashboard started — http://localhost:" + port)
	if err := app.Listen(":" + port); err != nil {
		logger.Fatal().Err(err).Msg("server error")
	}
}

func handleLogin(c *fiber.Ctx) error {
	return c.Type("html").SendString(loginPage(""))
}

func loginPage(msg string) string {
	return `<!DOCTYPE html><html><head>
<title>API Dashboard — Login</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
body{font-family:system-ui,sans-serif;background:#1a1a2e;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0}
.box{background:#16213e;padding:2rem;border-radius:12px;width:320px;box-shadow:0 4px 20px rgba(0,0,0,.4)}
h2{color:#e2e8f0;margin:0 0 1.5rem;text-align:center}
input{width:100%;padding:.75rem;border:1px solid #2d3748;border-radius:8px;background:#1a1a2e;color:#e2e8f0;font-size:1rem;box-sizing:border-box;margin-bottom:1rem}
button{width:100%;padding:.75rem;background:#667eea;color:#fff;border:none;border-radius:8px;font-size:1rem;cursor:pointer}
button:hover{background:#5a67d8}
.msg{color:#fc8181;text-align:center;margin-bottom:1rem;font-size:.9rem}
</style></head><body>
<div class="box">
<h2>🔐 API Dashboard</h2>
` + func() string {
		if msg != "" {
			return `<div class="msg">` + msg + `</div>`
		}
		return ""
	}() + `
<form method="POST" action="/login">
<input type="password" name="password" placeholder="Password" autofocus autocomplete="current-password">
<button type="submit">Login</button>
</form></div></body></html>`
}

func handleDashboard(c *fiber.Ctx) error {
	return c.Type("html").SendString(dashboardHTML())
}

func handleStats(c *fiber.Ctx) error {
	results := aggregateStats()
	return c.JSON(results)
}

type APIStats struct {
	Name        string         `json:"name"`
	Port        string         `json:"port"`
	Color       string         `json:"color"`
	Icon        string         `json:"icon"`
	Online      bool           `json:"online"`
	Analytics   map[string]any `json:"analytics,omitempty"`
	Health      map[string]any `json:"health,omitempty"`
	Error       string         `json:"error,omitempty"`
}

func aggregateStats() []APIStats {
	results := make([]APIStats, len(apis))
	var wg sync.WaitGroup
	for i, api := range apis {
		wg.Add(1)
		go func(i int, api APITarget) {
			defer wg.Done()
			s := APIStats{Name: api.Name, Port: api.Port, Color: api.Color, Icon: api.Icon}

			// Health check
			health, err := fetchJSON(fmt.Sprintf("http://localhost:%s/health", api.Port), "")
			if err != nil {
				s.Online = false
				s.Error = err.Error()
				results[i] = s
				return
			}
			s.Online = true
			s.Health = health

			// Analytics (admin endpoint)
			analytics, err := fetchJSON(
				fmt.Sprintf("http://localhost:%s/admin/analytics", api.Port),
				api.AdminSecret,
			)
			if err == nil {
				s.Analytics = analytics
			}
			results[i] = s
		}(i, api)
	}
	wg.Wait()
	return results
}

func fetchJSON(url, adminSecret string) (map[string]any, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if adminSecret != "" {
		req.Header.Set("X-Admin-Secret", adminSecret)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func dashboardHTML() string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>API Analytics Dashboard</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:system-ui,-apple-system,sans-serif;background:#0f172a;color:#e2e8f0;min-height:100vh}
header{background:#1e293b;padding:1rem 2rem;display:flex;justify-content:space-between;align-items:center;border-bottom:1px solid #334155}
header h1{font-size:1.25rem;font-weight:700}
header a{color:#94a3b8;text-decoration:none;font-size:.875rem}
header a:hover{color:#e2e8f0}
.grid{display:grid;grid-template-columns:repeat(auto-fill,minmax(380px,1fr));gap:1.5rem;padding:1.5rem}
.card{background:#1e293b;border-radius:12px;overflow:hidden;border:1px solid #334155;transition:transform .2s}
.card:hover{transform:translateY(-2px)}
.card-header{padding:1rem 1.25rem;display:flex;align-items:center;gap:.75rem}
.card-header .icon{font-size:1.5rem}
.card-header h2{font-size:1rem;font-weight:600}
.badge{margin-left:auto;padding:.25rem .75rem;border-radius:999px;font-size:.75rem;font-weight:600}
.badge.online{background:#065f46;color:#6ee7b7}
.badge.offline{background:#7f1d1d;color:#fca5a5}
.card-body{padding:1.25rem}
.stat-row{display:flex;justify-content:space-between;padding:.5rem 0;border-bottom:1px solid #1e293b;font-size:.875rem}
.stat-row:last-child{border-bottom:none}
.stat-label{color:#94a3b8}
.stat-value{font-weight:600;color:#f1f5f9}
.key-table{width:100%;border-collapse:collapse;font-size:.8rem;margin-top:.75rem}
.key-table th{color:#64748b;text-align:left;padding:.35rem .5rem;border-bottom:1px solid #334155;font-weight:500}
.key-table td{padding:.35rem .5rem;border-bottom:1px solid #1e293b;color:#cbd5e1}
.key-table tr:last-child td{border-bottom:none}
.bar{height:4px;border-radius:2px;margin-top:.5rem}
.section-title{color:#64748b;font-size:.75rem;font-weight:600;text-transform:uppercase;letter-spacing:.05em;margin:.75rem 0 .5rem}
.refresh{color:#64748b;font-size:.8rem}
.error-msg{color:#f87171;font-size:.875rem;padding:.5rem 0}
.total-bar{background:#1e293b;border-bottom:1px solid #334155;padding:.75rem 2rem;display:flex;gap:2rem;flex-wrap:wrap}
.total-stat{display:flex;flex-direction:column}
.total-stat span:first-child{font-size:.75rem;color:#64748b;text-transform:uppercase;letter-spacing:.05em}
.total-stat span:last-child{font-size:1.5rem;font-weight:700;color:#f1f5f9}
</style>
</head>
<body>
<header>
  <h1>📊 API Analytics Dashboard</h1>
  <div style="display:flex;gap:1rem;align-items:center">
    <span class="refresh" id="lastUpdate">Loading…</span>
    <a href="/logout">Logout</a>
  </div>
</header>
<div class="total-bar" id="totals"></div>
<div class="grid" id="grid"></div>

<script>
async function load() {
  try {
    const res = await fetch('/api/stats');
    const apis = await res.json();
    renderTotals(apis);
    renderCards(apis);
    document.getElementById('lastUpdate').textContent = 'Updated: ' + new Date().toLocaleTimeString();
  } catch(e) {
    console.error(e);
  }
}

function renderTotals(apis) {
  let totalReqs = 0, totalErrors = 0, onlineCount = 0;
  apis.forEach(api => {
    if (api.online) onlineCount++;
    const a = api.analytics;
    if (a?.requests) {
      totalReqs += (a.requests.total || 0);
      totalErrors += (a.requests.errors || 0);
    }
  });
  document.getElementById('totals').innerHTML = ` + "`" + `
    <div class="total-stat"><span>APIs Online</span><span>${onlineCount}/${apis.length}</span></div>
    <div class="total-stat"><span>Total Requests</span><span>${totalReqs.toLocaleString()}</span></div>
    <div class="total-stat"><span>Total Errors</span><span>${totalErrors.toLocaleString()}</span></div>
    <div class="total-stat"><span>Unique API Keys</span><span>${apis.reduce((s,a)=>s+(a.analytics?.perAPIKey?.length||0),0)}</span></div>
  ` + "`" + `;
}

function renderCards(apis) {
  document.getElementById('grid').innerHTML = apis.map(api => {
    const a = api.analytics || {};
    const req = a.requests || {};
    const cache = a.cache || {};
    const scraper = a.scraper || {};
    const keys = a.perAPIKey || [];
    const eps = a.perEndpoint || {};

    const isOnline = api.online;

    let keysHtml = '';
    if (keys.length > 0) {
      keysHtml = ` + "`" + `<div class="section-title">Top API Keys</div>
      <table class="key-table">
        <thead><tr><th>Key</th><th>Total Calls</th><th>Last Seen</th></tr></thead>
        <tbody>` + "`" + ` +
        keys.slice(0,5).map(k => ` + "`" + `<tr>
          <td><code>${k.key}</code></td>
          <td>${(k.totalCalls||0).toLocaleString()}</td>
          <td>${k.lastSeen ? new Date(k.lastSeen).toLocaleDateString() : '-'}</td>
        </tr>` + "`" + `).join('') +
        ` + "`" + `</tbody></table>` + "`" + `;
    }

    const epEntries = Object.entries(eps);
    let epsHtml = '';
    if (epEntries.length > 0) {
      epsHtml = ` + "`" + `<div class="section-title">Endpoints</div>
      <table class="key-table">
        <thead><tr><th>Endpoint</th><th>Reqs</th><th>Errors</th><th>Avg ms</th></tr></thead>
        <tbody>` + "`" + ` +
        epEntries.map(([ep, s]) => ` + "`" + `<tr>
          <td>${ep}</td>
          <td>${(s.requests||0).toLocaleString()}</td>
          <td>${s.errors||0}</td>
          <td>${Math.round(s.avgLatencyMs||0)}</td>
        </tr>` + "`" + `).join('') +
        ` + "`" + `</tbody></table>` + "`" + `;
    }

    return ` + "`" + `<div class="card">
      <div class="card-header" style="border-left:4px solid ${api.color}">
        <span class="icon">${api.icon}</span>
        <h2>${api.name}</h2>
        <span class="badge ${isOnline?'online':'offline'}">${isOnline?'● Online':'● Offline'}</span>
      </div>
      <div class="card-body">
        ${!isOnline ? '<div class="error-msg">⚠️ ' + (api.error||'Unreachable') + '</div>' : ''}
        ${isOnline ? ` + "`" + `
        <div class="stat-row"><span class="stat-label">Total Requests</span><span class="stat-value">${(req.total||0).toLocaleString()}</span></div>
        <div class="stat-row"><span class="stat-label">Errors</span><span class="stat-value">${req.errors||0} (${req.errorRate||'0.00%'})</span></div>
        <div class="stat-row"><span class="stat-label">Avg Latency</span><span class="stat-value">${Math.round(req.avgLatencyMs||0)} ms</span></div>
        <div class="stat-row"><span class="stat-label">Cache Hit Rate</span><span class="stat-value">${cache.hitRate||'—'}</span></div>
        <div class="stat-row"><span class="stat-label">Scrape OK / Fail</span><span class="stat-value">${scraper.success||0} / ${scraper.failure||0}</span></div>
        <div class="stat-row"><span class="stat-label">Queue Rejected</span><span class="stat-value">${scraper.queueRejected||0}</span></div>
        <div class="stat-row"><span class="stat-label">Unique API Keys</span><span class="stat-value">${keys.length}</span></div>
        ` + "`" + ` + keysHtml + epsHtml : ''}
      </div>
    </div>` + "`" + `;
  }).join('');
}

load();
setInterval(load, 30000); // refresh every 30s
</script>
</body>
</html>`
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Keep unused import happy
var _ = strings.TrimSpace
