# Health Monitoring & Alerting Setup

The `monitor` service (port 8091) checks every API's `/health` endpoint every 5 minutes.
If an API fails **2 checks in a row** (10 min), you get an alert.
When it recovers, you get a recovery notification.

---

## Option 1: Telegram (Recommended — 2 minutes)

1. Open Telegram → search **@BotFather** → `/newbot`
2. Choose a name (e.g. "My API Monitor") and username
3. Copy the **bot token** (looks like `7123456789:AAFxxx...`)
4. Start a chat with your new bot (click the link BotFather gives you → Start)
5. Get your chat ID:
   ```bash
   curl "https://api.telegram.org/bot<YOUR_TOKEN>/getUpdates"
   # Look for: "chat":{"id":123456789,...}
   ```
6. Add to `.env`:
   ```
   TELEGRAM_BOT_TOKEN=7123456789:AAFxxx...
   TELEGRAM_CHAT_ID=123456789
   ```
7. Test it:
   ```bash
   curl -X POST https://api.yourdomain.com/monitor/alert/test
   ```

You'll receive messages like:
```
🔴 API DOWN: bafin-api
Route: /v1/bafin/
Error: connection refused
Checked: 14:32:05 UTC

✅ API RECOVERED: bafin-api
Route: /v1/bafin/
Latency: 234ms
Time: 14:47:05 UTC
```

---

## Option 2: Discord Webhook

1. Open Discord → your server → channel ⚙ Settings
2. Integrations → Webhooks → New Webhook → copy URL
3. Add to `.env`:
   ```
   DISCORD_WEBHOOK_URL=https://discord.com/api/webhooks/123.../abc...
   ```

---

## Option 3: Email (Gmail)

1. Google account → Security → 2-Step Verification → App Passwords
2. Generate app password for "Mail"
3. Add to `.env`:
   ```
   SMTP_HOST=smtp.gmail.com
   SMTP_PORT=587
   SMTP_USER=you@gmail.com
   SMTP_PASSWORD=abcd efgh ijkl mnop   (16-char app password, spaces ok)
   ALERT_EMAIL_TO=you@gmail.com
   ```

> You can combine all three — all configured channels receive every alert.

---

## Monitor API endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /monitor/status` | JSON status of all APIs |
| `GET /monitor/status/:name` | Single API status |
| `POST /monitor/alert/test` | Send test alert to all channels |
| `GET /health` | Monitor service health |

Example:
```bash
curl https://api.yourdomain.com/monitor/status | jq '.'
```
Returns:
```json
{
  "allOK": false,
  "downAPIs": ["bafin-api"],
  "checkEvery": "5m0s",
  "apis": [
    {"name": "bafin-api", "up": false, "consecFails": 3, "error": "..."},
    {"name": "sanctions-api", "up": true, "latencyMs": 45},
    ...
  ]
}
```

---

## Alert Logic

- Check interval: every 5 minutes (configurable via `CHECK_INTERVAL_SEC`)
- Alert threshold: **2 consecutive failures** (avoids false alarms from brief hiccups)
- Separate alert for each channel — even if Telegram fails, email is still tried
- Recovery alert sent when API comes back up
