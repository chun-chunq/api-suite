# API Suite — Deployment Guide

Step-by-step guide for deploying the full API suite to a Hetzner VPS (or any Ubuntu/Debian server).  
Estimated time: **~30 minutes** on a fresh VPS.

---

## Overview

```
Your Windows PC  →  pack.bat  →  C:\api-suite\  →  scp upload  →  VPS /srv/apis/
                                                                      └─ docker compose up -d
```

The full suite runs as **Docker containers** behind **nginx** (HTTPS via Certbot).  
A **dashboard** and **monitor** service run in host network mode so they can reach all containers.

---

## Prerequisites

| What | Where to get |
|---|---|
| Hetzner VPS (CX21 or bigger) | cloud.hetzner.com → "Add Server" → Ubuntu 22.04 |
| Domain pointing to VPS IP | Your DNS provider → A record → VPS IP |
| Companies House API key | developer.company-information.service.gov.uk (free, ~2 min) |
| Telegram bot token + chat ID | @BotFather on Telegram → /newbot |

---

## Step 0 — Pack everything (on your Windows PC)

```bat
C:\deploy\pack.bat
```

This creates `C:\api-suite\` with all API folders + deploy config in one place.

---

## Step 1 — First-time VPS setup

SSH into your fresh server and install Docker + nginx:

```bash
ssh root@YOUR_SERVER_IP

# Update system
apt update && apt upgrade -y

# Install Docker
curl -fsSL https://get.docker.com | sh
systemctl enable docker

# Install nginx + certbot
apt install -y nginx certbot python3-certbot-nginx

# Install docker compose plugin (v2)
apt install -y docker-compose-plugin

# Create deployment directory
mkdir -p /srv/apis
```

---

## Step 2 — Upload the packed suite

From your Windows PC (PowerShell or Git Bash):

```bash
# Option A: scp (simplest)
scp -r C:\api-suite root@YOUR_SERVER_IP:/srv/apis

# Option B: rsync (faster on re-uploads, skips unchanged files)
rsync -avz --progress /c/api-suite/ root@YOUR_SERVER_IP:/srv/apis/
```

---

## Step 3 — Configure secrets

On the server:

```bash
cd /srv/apis/deploy

# Copy template
cp .env.template .env

# Edit — fill in your values:
nano .env
```

Minimum required values in `.env`:

```env
API_KEYS=your-secret-api-key-here          # customers use this
ADMIN_SECRET=a-long-random-admin-secret    # for /admin/* endpoints
WORKER_SECRET=another-random-secret       # PC worker ↔ server

# For UK Companies House API:
COMPANIES_HOUSE_API_KEY=your-key-here

# For alerts:
TELEGRAM_BOT_TOKEN=123456789:AABBCCxxx
TELEGRAM_CHAT_ID=987654321

# For analytics dashboard:
DASHBOARD_PASSWORD=choose-a-password

# Your email (for OpenAlex + SEC EDGAR polite pools):
CONTACT_EMAIL=your@email.com
```

Generate random secrets:
```bash
openssl rand -hex 24   # use output as ADMIN_SECRET, WORKER_SECRET, etc.
```

---

## Step 4 — Configure nginx + HTTPS

```bash
cd /srv/apis/deploy

# Replace placeholder domain in nginx config:
sed -i 's/api.yourdomain.com/YOUR_ACTUAL_DOMAIN/g' nginx.conf

# Copy nginx config
cp nginx.conf /etc/nginx/sites-available/api-suite
ln -sf /etc/nginx/sites-available/api-suite /etc/nginx/sites-enabled/api-suite

# Remove default nginx site
rm -f /etc/nginx/sites-enabled/default

# Test config
nginx -t

# Get HTTPS certificate (Let's Encrypt — free)
certbot --nginx -d YOUR_ACTUAL_DOMAIN -m YOUR_EMAIL --agree-tos -n

# Restart nginx
systemctl restart nginx
systemctl enable nginx
```

---

## Step 5 — Start all services

```bash
cd /srv/apis/deploy

# Build all Docker images and start containers (first run takes ~5-10 min)
docker compose up -d --build

# Watch startup logs
docker compose logs -f

# Check that everything is healthy
docker compose ps
```

All services should show **"Up"** status. Expected ports:

| Port | Service |
|---|---|
| 8080 | insolvency-api |
| 8081 | zvg-api |
| 8082 | ted-api |
| 8083 | dpma-api |
| 8084 | sanctions-api |
| 8085 | safety-api |
| 8086 | zefix-api |
| 8087 | bafin-api |
| 8089 | gleif-api |
| 8090 | cordis-api |
| 8091 | monitor |
| 8092 | handelsregister-api |
| 8093 | euipo-api |
| 8094 | french-company-api |
| 8095 | uk-company-api |
| 8096 | research-api |
| 8097 | gdpr-api |
| 8098 | sec-api |
| 8099 | food-api |
| 8100 | aviation-api |
| 8101 | weather-api |
| 8102 | currency-api |
| 8103 | openfda-api |
| 8104 | wikidata-api |
| 8105 | crypto-api |
| 8088 | dashboard (host network) |

---

## Step 6 — Verify everything works

```bash
# Health check all APIs (replace with your domain):
curl https://YOUR_DOMAIN/health/sanctions
curl https://YOUR_DOMAIN/health/gleif
curl https://YOUR_DOMAIN/health/weather
curl https://YOUR_DOMAIN/health/currency
curl https://YOUR_DOMAIN/health/eu-trademark

# Quick smoke test:
curl "https://YOUR_DOMAIN/v1/sanctions/search?query=Putin"
curl "https://YOUR_DOMAIN/v1/weather/current?lat=52.52&lon=13.405"
curl "https://YOUR_DOMAIN/v1/currency/latest"
curl "https://YOUR_DOMAIN/v1/crypto/top?limit=3"

# Dashboard:
open https://YOUR_DOMAIN/dashboard/
# Login with DASHBOARD_PASSWORD from .env
```

---

## Step 7 — Connect PC Worker (for scraping APIs)

The insolvency, zvg, dpma, bafin, and handelsregister APIs use your home PC for Chrome scraping.

On your **Windows PC**, configure `C:\pc-worker\config.yaml`:

```yaml
server_url: https://YOUR_DOMAIN
worker_secret: YOUR_WORKER_SECRET_FROM_ENV
apis:
  - insolvency
  - zvg
  - dpma
  - bafin
  - handelsregister
```

Then start the worker:
```bat
cd C:\pc-worker
go run . 
```

The worker registers itself with the server and starts accepting scraping jobs.

---

## Step 8 — Register on RapidAPI (start earning)

1. Go to **rapidapi.com/provider** → Add API
2. For each API, set:
   - **Base URL**: `https://YOUR_DOMAIN/v1/{route}/`
   - **Authentication**: Header `X-Api-Key` → maps to your `API_KEYS` value
3. Set a pricing plan (e.g. Free: 100 req/day, Basic: €9/mo, Pro: €29/mo)
4. Add endpoint docs (copy from health response / test curls above)

---

## Ongoing Operations

### Deploy an update

```bat
# 1. On Windows PC — pack new code
C:\deploy\pack.bat

# 2. Upload changes
scp -r C:\api-suite root@YOUR_SERVER_IP:/srv/apis

# 3. On server — rebuild only changed services
cd /srv/apis/deploy
docker compose up -d --build weather-api currency-api   # only rebuild what changed
# OR rebuild everything:
docker compose up -d --build
```

### View logs

```bash
# All services:
docker compose logs -f

# One service:
docker compose logs -f weather-api

# Last 100 lines:
docker compose logs --tail=100 currency-api
```

### Restart a single service

```bash
docker compose restart weather-api
```

### Check memory usage (idle)

```bash
docker stats --no-stream
```

Expected idle usage per pure REST-wrapper API: **~8–15 MB RSS** (GOGC=20 + GOMAXPROCS=1).  
Scraping APIs with Chrome: **~200–400 MB** when active, less when idle.

### Backup .env

```bash
# On server — keep a local copy somewhere safe
cat /srv/apis/deploy/.env
```

### Certificate renewal

Certbot auto-renews. Force renew manually if needed:
```bash
certbot renew --force-renewal
systemctl restart nginx
```

---

## Memory Budget (Hetzner CX21 = 4 GB RAM)

| Category | Services | ~RAM each | Total |
|---|---|---|---|
| Chrome scrapers (active) | 5 | ~350 MB | ~1.75 GB |
| In-memory dataset APIs | 2 (gdpr, safety) | ~200 MB | ~400 MB |
| Pure REST wrappers | 18 | ~12 MB | ~216 MB |
| Redis instances | 8 | ~70 MB | ~560 MB |
| nginx + OS | — | — | ~300 MB |
| **Total** | | | **~3.2 GB** |

Plenty of headroom on CX21. Chrome scrapers only use full RAM when actively scraping — idle usage is much lower.

---

## Troubleshooting

| Problem | Fix |
|---|---|
| Container exits immediately | `docker compose logs service-name` — usually a missing env var |
| nginx 502 Bad Gateway | Container not started: `docker compose ps` + `docker compose restart service-name` |
| Certificate not found | Run `certbot --nginx -d YOUR_DOMAIN` again |
| API returns 401 Unauthorized | `X-Api-Key` header missing or wrong — check your `API_KEYS` in `.env` |
| Scraping APIs timeout | PC Worker not connected — check `C:\pc-worker` is running |
| Out of memory | `docker stats` → increase `mem_limit` in docker-compose.yml for the offending service |
| 429 Too Many Requests (upstream) | Rate-limited by upstream API — add caching or reduce your call rate |
