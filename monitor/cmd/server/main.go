package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/monitor/internal/monitor"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	zerolog.TimeFieldFormat = time.RFC3339
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	port := envOr("PORT", "8091")
	intervalSec, _ := strconv.Atoi(envOr("CHECK_INTERVAL_SEC", "300")) // 5 minutes default
	interval := time.Duration(intervalSec) * time.Second

	// ── API targets ───────────────────────────────────────────────────────────
	// All services run on localhost (dashboard uses network_mode: host)
	targets := []monitor.APITarget{
		{Name: "insolvency-api", HealthURL: "http://localhost:8080/health", Route: "/v1/insolvency/"},
		{Name: "zvg-api", HealthURL: "http://localhost:8081/health", Route: "/v1/zvg/"},
		{Name: "ted-api", HealthURL: "http://localhost:8082/health", Route: "/v1/ted/"},
		{Name: "dpma-api", HealthURL: "http://localhost:8083/health", Route: "/v1/trademark/"},
		{Name: "sanctions-api", HealthURL: "http://localhost:8084/health", Route: "/v1/sanctions/"},
		{Name: "safety-api", HealthURL: "http://localhost:8085/health", Route: "/v1/recalls/"},
		{Name: "zefix-api", HealthURL: "http://localhost:8086/health", Route: "/v1/ch/"},
		{Name: "bafin-api", HealthURL: "http://localhost:8087/health", Route: "/v1/bafin/"},
		{Name: "gleif-api", HealthURL: "http://localhost:8089/health", Route: "/v1/lei/"},
		{Name: "cordis-api", HealthURL: "http://localhost:8090/health", Route: "/v1/grants/"},
		{Name: "euipo-api", HealthURL: "http://localhost:8093/health", Route: "/v1/eu-trademark/"},
		{Name: "french-company-api", HealthURL: "http://localhost:8094/health", Route: "/v1/fr/"},
		{Name: "uk-company-api", HealthURL: "http://localhost:8095/health", Route: "/v1/uk/"},
		{Name: "research-api", HealthURL: "http://localhost:8096/health", Route: "/v1/research/"},
		{Name: "gdpr-api", HealthURL: "http://localhost:8097/health", Route: "/v1/gdpr/"},
		{Name: "sec-api", HealthURL: "http://localhost:8098/health", Route: "/v1/sec/"},
		{Name: "food-api", HealthURL: "http://localhost:8099/health", Route: "/v1/food/"},
		{Name: "aviation-api", HealthURL: "http://localhost:8100/health", Route: "/v1/aviation/"},
		{Name: "weather-api", HealthURL: "http://localhost:8101/health", Route: "/v1/weather/"},
		{Name: "currency-api", HealthURL: "http://localhost:8102/health", Route: "/v1/currency/"},
		{Name: "openfda-api", HealthURL: "http://localhost:8103/health", Route: "/v1/drug/"},
		{Name: "wikidata-api", HealthURL: "http://localhost:8104/health", Route: "/v1/wikidata/"},
		{Name: "crypto-api", HealthURL: "http://localhost:8105/health", Route: "/v1/crypto/"},
		{Name: "ipgeo-api", HealthURL: "http://localhost:8107/health", Route: "/v1/ipgeo/"},
		{Name: "vat-api", HealthURL: "http://localhost:8108/health", Route: "/v1/vat/"},
		{Name: "countries-api", HealthURL: "http://localhost:8109/health", Route: "/v1/countries/"},
		{Name: "pubchem-api", HealthURL: "http://localhost:8110/health", Route: "/v1/chem/"},
		{Name: "nasa-api", HealthURL: "http://localhost:8111/health", Route: "/v1/nasa/"},
		{Name: "airquality-api", HealthURL: "http://localhost:8113/health", Route: "/v1/air/"},
		{Name: "exchangerate-api", HealthURL: "http://localhost:8114/health", Route: "/v1/fx/"},
		{Name: "gbif-api", HealthURL: "http://localhost:8115/health", Route: "/v1/bio/"},
		{Name: "namepredict-api", HealthURL: "http://localhost:8119/health", Route: "/v1/name/"},
		{Name: "worldbank-api", HealthURL: "http://localhost:8120/health", Route: "/v1/worldbank/"},
		{Name: "clinicaltrials-api", HealthURL: "http://localhost:8121/health", Route: "/v1/trials/"},
		{Name: "gateway", HealthURL: "http://localhost:8000/health", Route: "/gateway/status"},
	}

	// ── Alert channels ────────────────────────────────────────────────────────
	var alerters []monitor.Alerter

	// Telegram (primary — easiest to set up)
	if token := os.Getenv("TELEGRAM_BOT_TOKEN"); token != "" {
		chatID := os.Getenv("TELEGRAM_CHAT_ID")
		alerters = append(alerters, monitor.NewTelegramAlerter(token, chatID))
		log.Info().Msg("Telegram alerts: enabled")
	} else {
		log.Warn().Msg("Telegram alerts: disabled (set TELEGRAM_BOT_TOKEN + TELEGRAM_CHAT_ID)")
	}

	// Discord webhook
	if webhookURL := os.Getenv("DISCORD_WEBHOOK_URL"); webhookURL != "" {
		alerters = append(alerters, monitor.NewDiscordAlerter(webhookURL))
		log.Info().Msg("Discord alerts: enabled")
	}

	// Email (SMTP)
	if smtpHost := os.Getenv("SMTP_HOST"); smtpHost != "" {
		emailAlerter := monitor.NewEmailAlerter(
			smtpHost,
			envOr("SMTP_PORT", "587"),
			os.Getenv("SMTP_USER"),
			os.Getenv("SMTP_PASSWORD"),
			os.Getenv("ALERT_EMAIL_TO"),
		)
		alerters = append(alerters, emailAlerter)
		log.Info().Msg("Email alerts: enabled")
	}

	if len(alerters) == 0 {
		log.Warn().Msg("No alert channels configured — running in monitor-only mode")
		alerters = append(alerters, &monitor.NoopAlerter{})
	}

	alerter := monitor.NewMultiAlerter(alerters...)
	mon := monitor.New(targets, alerter, interval, log.Logger)

	// Send a startup test message so you know it works
	go func() {
		time.Sleep(3 * time.Second) // let services start first
		mon.Start()
		log.Info().Dur("interval", interval).Msg("monitoring started")
	}()

	// ── HTTP status API ───────────────────────────────────────────────────────
	app := fiber.New(fiber.Config{
		AppName:               "api-monitor",
		DisableStartupMessage: true,
	})
	app.Use(recover.New())

	// GET /status — JSON overview of all APIs
	app.Get("/status", func(c *fiber.Ctx) error {
		statuses := mon.Snapshot()
		sort.Slice(statuses, func(i, j int) bool {
			return statuses[i].Name < statuses[j].Name
		})
		allOK := true
		down := []string{}
		for _, s := range statuses {
			if !s.Up {
				allOK = false
				down = append(down, s.Name)
			}
		}
		httpStatus := fiber.StatusOK
		if !allOK {
			httpStatus = fiber.StatusServiceUnavailable
		}
		return c.Status(httpStatus).JSON(fiber.Map{
			"allOK":       allOK,
			"downAPIs":    down,
			"checkEvery":  interval.String(),
			"apis":        statuses,
			"lastChecked": time.Now().UTC().Format(time.RFC3339),
		})
	})

	// GET /status/:name — single API status
	app.Get("/status/:name", func(c *fiber.Ctx) error {
		name := c.Params("name")
		for _, s := range mon.Snapshot() {
			if s.Name == name {
				httpStatus := fiber.StatusOK
				if !s.Up {
					httpStatus = fiber.StatusServiceUnavailable
				}
				return c.Status(httpStatus).JSON(s)
			}
		}
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "API not found: " + name})
	})

	// POST /alert/test — send a test alert to all configured channels
	app.Post("/alert/test", func(c *fiber.Ctx) error {
		msg := "🔔 *Test alert from API Monitor*\n\nAll alert channels are working correctly.\nTime: " + time.Now().Format("15:04:05 UTC")
		if err := alerter.Alert(msg); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"sent": true, "message": msg})
	})

	// GET /health
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok", "service": "api-monitor"})
	})

	log.Info().Str("port", port).Msg("monitor HTTP server starting")
	if err := app.Listen(fmt.Sprintf(":%s", port)); err != nil {
		log.Fatal().Err(err).Msg("server failed")
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
