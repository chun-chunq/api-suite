# API Suite — Deployment Guide

## Overview

| Port | API | Type | Chrome needed? |
|------|-----|------|----------------|
| 8080 | **insolvency-api** | German insolvency register | ✅ Yes |
| 8081 | **zvg-api** | German foreclosure auctions | ✅ Yes |
| 8082 | **ted-api** | EU procurement (TED) | ❌ No (REST wrapper) |
| 8083 | **dpma-api** | German trademark register | ✅ Yes |
| 8084 | **sanctions-api** | EU consolidated sanctions list | ❌ No (XML download) |

## Quick Start (Hetzner VPS, Ubuntu 22.04+)

> **Layout note:** the whole suite now lives in a single git repo
> (`C:\api-suite`). There is no `pack.bat` step any more — just commit and push
> normally (`git add -A && git commit -m "…" && git push`), then pull on the
> server. All API folders are siblings of `deploy/` inside this one repo.

### Step 0 — Push your changes

```bash
# On your Windows PC, from C:\api-suite
git add -A
git commit -m "your change"
git push
```

### Step 1 — Get it onto the server

```bash
# First time: clone the repo
ssh root@YOUR_SERVER
git clone https://github.com/chun-chunq/api-suite.git /srv/apis
cd /srv/apis/deploy

# Later updates: just pull
cd /srv/apis && git pull

# Configure secrets (first time only; an existing .env is preserved on pull)
cp .env.template .env
nano .env         # fill in API_KEYS, ADMIN_SECRET, WORKER_SECRET

# Start everything
chmod +x start.sh stop.sh update.sh
./start.sh
```

*(Prefer not to use git on the server? You can still `rsync -av /c/api-suite/ root@YOUR_SERVER:/srv/apis/` — but exclude `.env`.)*

## Expected folder layout on server

```
/srv/apis/
├── deploy/             ← this folder (docker-compose.yml, start.sh, .env)
├── insolvency-api/     ← Go source + Dockerfile
├── zvg-api/
├── ted-api/
├── dpma-api/
├── sanctions-api/
├── safety-api/
├── zefix-api/
├── bafin-api/
├── gleif-api/
├── cordis-api/
├── monitor/
├── dashboard/
├── handelsregister-api/
└── pc-worker/          ← Windows binary source (build on Windows, deploy .exe to home PC)
```

## PC-Worker (Home PC as scrape worker)

The home PC worker connects outbound to the server (no port forwarding needed):

```
# On Windows home PC:
cd C:\pc-worker
# Edit config.yaml:
#   server_url: "https://YOUR-DOMAIN.de"
#   worker_secret: "same as WORKER_SECRET in .env"
#   scrapers: [insolvency, zvg, dpma]
.\pc-worker.exe
```

The worker registers automatically. Check via:
```bash
curl -H "X-Admin-Secret: $ADMIN_SECRET" http://localhost:8080/admin/stats
```

## Adding a new API key (new customer)

```bash
# On server, edit .env:
nano .env
# Add the new key to API_KEYS (comma-separated)

# Restart all services to pick up new key:
./update.sh
```

## Analytics — who uses which API how often

```bash
# Per-key usage (who calls how often, sorted by total calls):
curl -H "X-Admin-Secret: $ADMIN_SECRET" http://localhost:8080/admin/analytics | jq .perAPIKey

# Example output:
# [
#   {"key": "…abc123", "totalCalls": 847, "lastSeen": "2025-01-15T10:23:00Z",
#    "perEndpoint": {"/v1/insolvency/search": 700, "/v1/insolvency/company": 147}},
#   ...
# ]
```

## Dynamic worker management

```bash
# Add a new IP (e.g. second Hetzner IP or home PC via Tailscale):
curl -X POST -H "X-Admin-Secret: $ADMIN_SECRET" \
  -H "Content-Type: application/json" \
  -d '{"url":"http://NEW_IP:9090"}' \
  http://localhost:8080/admin/workers

# Check all workers:
curl -H "X-Admin-Secret: $ADMIN_SECRET" http://localhost:8080/admin/stats
```

## Monitoring

```bash
docker compose logs -f          # all logs
docker compose logs -f dpma-api  # one service
docker compose ps               # health status
```

## Update a single API after code change

```bash
./update.sh dpma-api    # rebuild and hot-swap dpma-api only
./update.sh             # rebuild all
```

## Bandwidth

Each Chrome-based API tracks bandwidth independently.
At 80% of `BANDWIDTH_LIMIT_GB`, a warning is logged.
At 100%, requests return 503.

Total budget: set `BANDWIDTH_LIMIT_GB=15` to leave headroom on a 20 TB Hetzner plan
(5 × 3 TB headroom across the 4 Chrome APIs).

sanctions-api and ted-api use minimal bandwidth (REST/XML only).
