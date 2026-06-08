package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

// MultiAlerter fans out to multiple alerters — all are tried even if one fails.
type MultiAlerter struct {
	alerters []Alerter
}

func NewMultiAlerter(a ...Alerter) *MultiAlerter {
	return &MultiAlerter{alerters: a}
}

func (m *MultiAlerter) Alert(msg string) error {
	var errs []string
	for _, a := range m.alerters {
		if err := a.Alert(msg); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("alert errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

// NoopAlerter does nothing (used when no alert channels are configured).
type NoopAlerter struct{}

func (n *NoopAlerter) Alert(_ string) error { return nil }

// ─── Telegram ────────────────────────────────────────────────────────────────

// TelegramAlerter sends messages to a Telegram chat via Bot API.
// Setup:
//  1. Message @BotFather → /newbot → get token
//  2. Message your bot once, then get chatID:
//     curl https://api.telegram.org/bot<TOKEN>/getUpdates
type TelegramAlerter struct {
	token  string
	chatID string
	client *http.Client
}

func NewTelegramAlerter(botToken, chatID string) *TelegramAlerter {
	return &TelegramAlerter{
		token:  botToken,
		chatID: chatID,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (t *TelegramAlerter) Alert(msg string) error {
	if t.token == "" || t.chatID == "" {
		return nil // not configured
	}
	payload, _ := json.Marshal(map[string]any{
		"chat_id":    t.chatID,
		"text":       msg,
		"parse_mode": "Markdown",
	})
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
	resp, err := t.client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("telegram: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("telegram: HTTP %d", resp.StatusCode)
	}
	return nil
}

// ─── Discord ─────────────────────────────────────────────────────────────────

// DiscordAlerter sends messages to a Discord channel via webhook.
// Setup: Discord server → channel Settings → Integrations → New Webhook → copy URL
type DiscordAlerter struct {
	webhookURL string
	client     *http.Client
}

func NewDiscordAlerter(webhookURL string) *DiscordAlerter {
	return &DiscordAlerter{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 10 * time.Second},
	}
}

func (d *DiscordAlerter) Alert(msg string) error {
	if d.webhookURL == "" {
		return nil
	}
	// Convert Markdown to Discord-friendly format (strip * for bold since Discord uses **)
	discordMsg := strings.ReplaceAll(msg, "*", "**")
	payload, _ := json.Marshal(map[string]string{"content": discordMsg})
	resp, err := d.client.Post(d.webhookURL, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("discord: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("discord: HTTP %d", resp.StatusCode)
	}
	return nil
}

// ─── Email (SMTP) ─────────────────────────────────────────────────────────────

// EmailAlerter sends alerts via SMTP.
// Works with Gmail (use App Password), any SMTP server, Mailgun SMTP, etc.
type EmailAlerter struct {
	host     string // e.g. "smtp.gmail.com"
	port     string // e.g. "587"
	user     string // sender email
	password string // SMTP password / app password
	to       string // recipient email
}

func NewEmailAlerter(host, port, user, password, to string) *EmailAlerter {
	return &EmailAlerter{host: host, port: port, user: user, password: password, to: to}
}

func (e *EmailAlerter) Alert(msg string) error {
	if e.host == "" || e.to == "" {
		return nil
	}
	auth := smtp.PlainAuth("", e.user, e.password, e.host)
	subject := "API Monitor Alert"
	if strings.Contains(msg, "DOWN") {
		subject = "🔴 API DOWN — Action Required"
	} else if strings.Contains(msg, "RECOVERED") {
		subject = "✅ API Recovered"
	}
	body := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s",
		e.user, e.to, subject, msg)
	addr := fmt.Sprintf("%s:%s", e.host, e.port)
	if err := smtp.SendMail(addr, auth, e.user, []string{e.to}, []byte(body)); err != nil {
		return fmt.Errorf("email: %w", err)
	}
	return nil
}
