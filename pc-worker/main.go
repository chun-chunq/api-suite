// pc-worker — Windows Heimnetzwerk-Scrape-Worker
// Verbindet sich zum Hetzner-Server und übernimmt Scrape-Jobs
// wenn die Server-IP geblockt ist.
//
// Konfiguration: config.yaml (im gleichen Verzeichnis wie die .exe)
// Starten: pc-worker.exe
//
// Bauen: go build -o pc-worker.exe .
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/yaml.v3"

	"github.com/pc-worker/scraper"
)

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

type Config struct {
	ServerURL          string   `yaml:"server_url"`
	WorkerSecret       string   `yaml:"worker_secret"`
	ChromeBin          string   `yaml:"chrome_bin"`
	Scrapers           []string `yaml:"scrapers"`
	PollTimeoutSeconds int      `yaml:"poll_timeout_seconds"`
}

func loadConfig() (*Config, error) {
	exe, err := os.Executable()
	if err != nil {
		exe = "."
	}
	cfgPath := filepath.Join(filepath.Dir(exe), "config.yaml")
	// also check working dir
	if _, err2 := os.Stat(cfgPath); os.IsNotExist(err2) {
		cfgPath = "config.yaml"
	}
	f, err := os.Open(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("config.yaml nicht gefunden: %w", err)
	}
	defer f.Close()
	var cfg Config
	if err := yaml.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("config.yaml ungültig: %w", err)
	}
	if cfg.PollTimeoutSeconds <= 0 {
		cfg.PollTimeoutSeconds = 30
	}
	return &cfg, nil
}

// ---------------------------------------------------------------------------
// Job protocol (must match server-side jobqueue)
// ---------------------------------------------------------------------------

type Job struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"` // "insolvency" | "zvg"
	Payload json.RawMessage `json:"payload"`
}

type JobResult struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data,omitempty"`
	Error string          `json:"error,omitempty"`
}

// ---------------------------------------------------------------------------
// Worker loop
// ---------------------------------------------------------------------------

func main() {
	log := zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "15:04:05"}).
		With().Timestamp().Logger()

	cfg, err := loadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Konfiguration konnte nicht geladen werden")
	}

	// Validate config
	if cfg.ServerURL == "" || cfg.ServerURL == "https://DEINE-DOMAIN.DE" {
		log.Fatal().Msg("server_url in config.yaml eintragen!")
	}
	if cfg.WorkerSecret == "" || cfg.WorkerSecret == "aendere-mich-geheim-123" {
		log.Fatal().Msg("worker_secret in config.yaml eintragen!")
	}
	if cfg.ChromeBin == "" {
		cfg.ChromeBin = defaultChromePath()
	}
	if len(cfg.Scrapers) == 0 {
		cfg.Scrapers = []string{"insolvency", "zvg"}
	}

	log.Info().
		Str("server", cfg.ServerURL).
		Strs("scrapers", cfg.Scrapers).
		Str("chrome", cfg.ChromeBin).
		Msgf("PC-Worker gestartet (%s/%s)", runtime.GOOS, runtime.GOARCH)

	printLocalIPs(log)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	client := &http.Client{Timeout: time.Duration(cfg.PollTimeoutSeconds+10) * time.Second}
	scraperTypes := joinTypes(cfg.Scrapers)

	log.Info().Msg("Warte auf Jobs vom Server... (Ctrl+C zum Beenden)")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Worker beendet.")
			return
		default:
		}

		job, err := pollJob(ctx, client, cfg, scraperTypes)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.Warn().Err(err).Msg("Poll-Fehler, warte 5s")
			sleep(ctx, 5*time.Second)
			continue
		}
		if job == nil {
			// 204 No Content — no job available, poll again immediately
			continue
		}

		log.Info().Str("id", job.ID).Str("type", job.Type).Msg("Job erhalten, führe Scrape aus")

		result := executeJob(ctx, job, cfg, log)
		if err := submitResult(ctx, client, cfg, result); err != nil {
			log.Error().Err(err).Str("id", job.ID).Msg("Ergebnis konnte nicht gesendet werden")
		} else {
			if result.OK {
				log.Info().Str("id", job.ID).Msg("Job erfolgreich abgeschlossen")
			} else {
				log.Warn().Str("id", job.ID).Str("error", result.Error).Msg("Job fehlgeschlagen")
			}
		}
	}
}

// pollJob calls GET /internal/worker/poll on the server (long-poll).
// Returns nil job and nil error when the server returns 204 (no job ready).
func pollJob(ctx context.Context, client *http.Client, cfg *Config, scraperTypes string) (*Job, error) {
	url := fmt.Sprintf("%s/internal/worker/poll?types=%s", cfg.ServerURL, scraperTypes)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Worker-Secret", cfg.WorkerSecret)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server antwortete %d: %s", resp.StatusCode, body)
	}

	var job Job
	if err := json.NewDecoder(resp.Body).Decode(&job); err != nil {
		return nil, fmt.Errorf("job JSON ungültig: %w", err)
	}
	return &job, nil
}

// executeJob runs the appropriate scraper and returns a JobResult.
func executeJob(ctx context.Context, job *Job, cfg *Config, log zerolog.Logger) JobResult {
	scrapeCtx, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()

	switch job.Type {
	case "insolvency":
		data, err := scraper.RunInsolvency(scrapeCtx, cfg.ChromeBin, job.Payload, log)
		if err != nil {
			return JobResult{ID: job.ID, OK: false, Error: err.Error()}
		}
		return JobResult{ID: job.ID, OK: true, Data: data}

	case "zvg":
		data, err := scraper.RunZVG(scrapeCtx, cfg.ChromeBin, job.Payload, log)
		if err != nil {
			return JobResult{ID: job.ID, OK: false, Error: err.Error()}
		}
		return JobResult{ID: job.ID, OK: true, Data: data}

	default:
		return JobResult{ID: job.ID, OK: false, Error: "unbekannter job type: " + job.Type}
	}
}

// submitResult POSTs the result to the server.
func submitResult(ctx context.Context, client *http.Client, cfg *Config, result JobResult) error {
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/internal/worker/result/%s", cfg.ServerURL, result.ID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Worker-Secret", cfg.WorkerSecret)

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("server akzeptierte Ergebnis nicht: %d", resp.StatusCode)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func defaultChromePath() string {
	candidates := []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
		os.Getenv("LOCALAPPDATA") + `\Google\Chrome\Application\chrome.exe`,
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return candidates[0]
}

func joinTypes(scrapers []string) string {
	out := ""
	for i, s := range scrapers {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func printLocalIPs(log zerolog.Logger) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return
	}
	log.Info().Msg("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Info().Msg("Diese IP-Adressen auf dem Server in WORKER_SECRET eintragen:")
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				log.Info().Msgf("  → %s (lokal/LAN)", ip4)
			}
		}
	}
	log.Info().Msg("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Info().Msg("HINWEIS: Dieser Worker verbindet sich zum Server (outbound).")
	log.Info().Msg("Kein Port-Forwarding nötig!")
}
