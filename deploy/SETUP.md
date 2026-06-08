# Setup Guide — API Suite

Vollständige Schritt-für-Schritt Anleitung: Windows → GitHub → Server → Marketplaces → MCP.

---

## Inhaltsverzeichnis

1. [Voraussetzungen](#1-voraussetzungen)
2. [Windows: Repo zusammenbauen und pushen](#2-windows-repo-zusammenbauen-und-pushen)
3. [Server: Einmalig deployen](#3-server-einmalig-deployen)
4. [Server: .env konfigurieren](#4-server-env-konfigurieren)
5. [SSL einrichten](#5-ssl-einrichten)
6. [Updates deployen](#6-updates-deployen)
7. [RapidAPI einrichten](#7-rapidapi-einrichten)
8. [APILayer einrichten](#8-apilayer-einrichten)
9. [API.market einrichten](#9-apimarket-einrichten)
10. [MCP Registries einrichten](#10-mcp-registries-einrichten)
11. [Monitoring & Dashboard](#11-monitoring--dashboard)
12. [Troubleshooting](#12-troubleshooting)

---

## 1. Voraussetzungen

**Windows (Entwicklungsmaschine):**
- Git installiert
- PowerShell oder CMD

**Server:**
- Ubuntu 22.04 oder 24.04 (Hetzner CX21 reicht: 2 vCPU, 4 GB RAM, ~5€/Monat)
- Root-Zugang per SSH
- Eine Domain die auf die Server-IP zeigt (A-Record)

**Accounts die du brauchst:**
- GitHub Account
- RapidAPI Provider Account (rapidapi.com/provider)
- Deine Domain bei einem Registrar

---

## 2. Windows: Repo zusammenbauen und pushen

### Schritt 1 — Pack alles in einen Ordner

```bat
C:\deploy\pack.bat
```

Das erstellt `C:\api-suite\` mit allen APIs, Configs, Scripts. Das ist der Ordner der auf GitHub kommt — **nicht** dein gesamtes C:\.

### Schritt 2 — Git Repo initialisieren

```bat
C:\deploy\git-setup.bat
```

Das macht:
- Erstellt `.gitignore` (`.env` wird niemals committet)
- Kopiert `.github/workflows/ci.yml` für GitHub Actions
- Führt `git init` und ersten Commit durch
- Zeigt dir die nächsten Schritte

### Schritt 3 — GitHub Repo erstellen und pushen

1. Geh auf **github.com/new**
2. Name z.B. `api-suite`
3. **Leer lassen** — kein README, kein .gitignore (haben wir schon)
4. "Create repository" klicken
5. Dann auf deiner Maschine:

```bat
cd C:\api-suite
git remote add origin https://github.com/DEIN_USERNAME/api-suite.git
git branch -M main
git push -u origin main
```

GitHub Actions CI startet automatisch und testet alle Services.

---

## 3. Server: Einmalig deployen

### Option A — One-Liner (empfohlen)

Auf dem Server als root:

```bash
curl -fsSL https://raw.githubusercontent.com/DEIN_USERNAME/api-suite/main/deploy/github-deploy.sh | bash
```

Das Script macht automatisch:
- Docker installieren (falls nicht vorhanden)
- Repo nach `/srv/apis` klonen
- `.env` aus Template anlegen und dich zum Editieren auffordern
- SSL mit Let's Encrypt einrichten
- Alle Docker Images bauen (~5-10 min beim ersten Mal)
- Alle Services starten
- Health Check auf allen 40+ Ports

### Option B — Manuell (mehr Kontrolle)

```bash
# Repo klonen
git clone https://github.com/DEIN_USERNAME/api-suite.git /srv/apis
cd /srv/apis/deploy

# .env anlegen
cp .env.template .env
nano .env     # Secrets eintragen (siehe Abschnitt 4)

# SSL
chmod +x setup-ssl.sh
./setup-ssl.sh deine-domain.com deine@email.com

# Alles starten
chmod +x start.sh
./start.sh
```

---

## 4. Server: .env konfigurieren

Die `.env` liegt auf dem Server unter `/srv/apis/deploy/.env`. **Niemals committen.**

```bash
nano /srv/apis/deploy/.env
```

### Pflichtfelder

```env
# Deine Domain
DOMAIN=api.deine-domain.com
SSL_EMAIL=deine@email.com

# Admin-Zugang (Gateway-Stats, Reset-Endpoints)
ADMIN_SECRET=   # openssl rand -hex 24

# Dashboard-Passwort (https://DOMAIN/dashboard)
DASHBOARD_PASSWORD=   # openssl rand -base64 16
```

### API Keys für deine Kunden

```env
# Format: KEY:TIER  (Tiers: free=20/min, basic=100/min, pro=300/min, ultra=1000/min)
# Generieren: openssl rand -hex 20
API_KEYS=abc123def456:pro,xyz789:basic
```

### RapidAPI (Marketplace)

```env
# Aus dem RapidAPI Provider Dashboard unter API Settings → Security
RAPIDAPI_PROXY_SECRET=dein-proxy-secret-von-rapidapi

# Auf false setzen wenn du auf RapidAPI bist (die injizieren immer Keys)
ALLOW_ANONYMOUS=false
```

### Optionale API Keys (für höhere Rate Limits bei Upstream-APIs)

```env
NASA_API_KEY=         # api.nasa.gov — kostenlos, instant
TED_API_KEY=          # api.ted.europa.eu — kostenlos
OPENFDA_API_KEY=      # open.fda.gov — kostenlos
COINGECKO_API_KEY=    # coingecko.com/api — kostenlos Demo Key
CONTACT_EMAIL=        # für OpenAlex + SEC EDGAR (bessere Rate Limits)
```

### Alerting (optional aber empfohlen)

```env
# Telegram (einfachste Option):
# 1. @BotFather auf Telegram → /newbot → Token kopieren
# 2. Bot anschreiben, dann: curl https://api.telegram.org/bot<TOKEN>/getUpdates
# 3. "id" aus dem Response = deine Chat-ID
TELEGRAM_BOT_TOKEN=
TELEGRAM_CHAT_ID=
```

Nach dem Bearbeiten:

```bash
cd /srv/apis/deploy
docker compose up -d   # neue .env laden
```

---

## 5. SSL einrichten

SSL wird normalerweise automatisch von `github-deploy.sh` gemacht. Falls manuell:

```bash
# Voraussetzung: DOMAIN zeigt bereits per A-Record auf deinen Server
cd /srv/apis/deploy
./setup-ssl.sh api.deine-domain.com deine@email.com
```

Das installiert Certbot, holt ein Let's Encrypt Zertifikat und konfiguriert nginx für HTTPS + Auto-Renewal.

**DNS prüfen vorher:**
```bash
dig +short api.deine-domain.com   # muss deine Server-IP zurückgeben
```

---

## 6. Updates deployen

### Normaler Update-Flow

```bat
REM Windows: Änderungen machen, dann:
cd C:\api-suite
git add -A
git commit -m "update xyz"
git push
```

```bash
# Server: Update pullen und neu starten
cd /srv/apis/deploy
./update.sh
```

`update.sh` macht: `git pull` → `docker compose build --parallel` → rolling restart → Health Check.

### Nur einen Service neu starten

```bash
./update.sh dpma-api    # nur diesen Service rebuilden + restart, kein git pull
```

### Schnell ohne Build

```bash
docker compose restart dpma-api   # restart ohne rebuild (für .env-Änderungen)
```

---

## 7. RapidAPI einrichten

RapidAPI ist der wichtigste Marketplace. Billing läuft komplett über die.

### Schritt 1 — Provider Account

1. Geh zu **rapidapi.com/provider** → Sign Up
2. "Add New API" klicken

### Schritt 2 — API anlegen (für jede API wiederholen)

1. **Name**: z.B. "ClinicalTrials.gov API"
2. **Base URL**: `https://api.deine-domain.com` (dein Gateway)
3. **Category**: passend wählen
4. **Endpoints** definieren (oder OpenAPI spec importieren):
   ```bash
   # Spec generieren:
   cd /srv/apis/deploy
   ./generate-openapi.sh api.deine-domain.com > openapi.json
   ```

### Schritt 3 — Proxy Secret holen

In der API-Übersicht: **Settings → Security → Proxy Secret** → kopieren

```env
# In /srv/apis/deploy/.env eintragen:
RAPIDAPI_PROXY_SECRET=das-secret-von-rapidapi
ALLOW_ANONYMOUS=false
```

Dann `docker compose restart gateway`.

### Schritt 4 — Pläne erstellen

In RapidAPI Dashboard → **Pricing**:

| Plan | Preis | Requests/Monat |
|------|-------|----------------|
| Free | $0 | 100 |
| Basic | $9 | 10.000 |
| Pro | $29 | 100.000 |
| Ultra | $99 | 1.000.000 |

### Schritt 5 — Welche APIs zuerst listen

Starte mit diesen (wenig Konkurrenz auf RapidAPI):
- ClinicalTrials.gov API
- GBIF Biodiversity API
- PubChem Chemistry API
- World Bank Data API
- Handelsregister API
- GLEIF LEI API

**Nicht** sofort: VAT, Exchange Rates, Trivia — da ist die Konkurrenz groß.

### Wie RapidAPI mit deinem Gateway kommuniziert

RapidAPI injiziert bei jeder Anfrage automatisch:
```
X-RapidAPI-Proxy-Secret: <RAPIDAPI_PROXY_SECRET>   ← Gateway validiert das
X-RapidAPI-Key:          <subscriber-key>
X-RapidAPI-Subscription: BASIC / PRO / ULTRA
X-RapidAPI-User:         <subscriber-username>
```

Das Gateway mappt `X-RapidAPI-Subscription` → internes Rate-Limit-Tier.
Kein weiterer Code nötig — alles bereits eingebaut.

---

## 8. APILayer einrichten

**15% Commission** — beste Rate aller Marketplaces. Application-basiert (1-2 Wochen Review).

### Nicht listen (Interessenkonflikt):
- VAT API → APILayer besitzt Vatlayer
- Exchange Rates API → APILayer besitzt Fixer.io

### Diese APIs pitchen:
ClinicalTrials, PubChem, GBIF, World Bank, NASA, Air Quality, Name Prediction, Sanctions, GLEIF

### Bewerbung:
1. Geh zu **apilayer.com** → "Submit Your API"
2. Base URL: `https://api.deine-domain.com`
3. Beschreibe daten-Quelle (z.B. "clinicaltrials.gov government data") und Use Cases
4. APILayer injiziert `apikey` Header — das Gateway unterstützt das bereits

Config für APILayer:
```bash
cp /srv/apis/deploy/marketplace/apilayer.env /srv/apis/deploy/.env
nano /srv/apis/deploy/.env   # secrets eintragen
docker compose restart gateway
```

---

## 9. API.market einrichten

**Offene Submissions, sehr wenig Konkurrenz** — alle APIs listen.

1. **api.market** → "Become a Seller"
2. Für jede API: Base URL `https://api.deine-domain.com/v1/<endpoint>`
3. API.market injiziert `X-Api-Key` Header — Gateway unterstützt das bereits

Config:
```bash
cp /srv/apis/deploy/marketplace/apimarket.env /srv/apis/deploy/.env
nano /srv/apis/deploy/.env
docker compose restart gateway
```

---

## 10. MCP Registries einrichten

Deine APIs sind bereits MCP-Server (jede hat `/mcp` Endpoint). Das macht sie nutzbar in **Claude Desktop, Cursor, ChatGPT** etc.

MCP Registries = kostenlose Discovery. User finden dich dort → brauchen API-Key → kaufen bei dir oder auf RapidAPI.

### Schritt 1 — server.json Files generieren

```bash
cd /srv/apis/deploy
chmod +x generate-mcp-servers.sh
./generate-mcp-servers.sh api.deine-domain.com dein-namespace
# Erstellt: mcp-servers/*.json (40+ Dateien)
```

### Schritt 2 — Offizielles MCP Registry (feeds Glama + PulseMCP)

```bash
npm install -g @modelcontextprotocol/publisher

# Domain verifizieren (einmalig):
mcp-publisher login http --domain api.deine-domain.com
# → Das CLI zeigt dir was du in die well-known Datei schreiben musst
# → Inhalt in /srv/apis/deploy/well-known/mcp-registry-auth eintragen
# → Dann: docker compose restart nginx

# Alle Server publishen:
for f in mcp-servers/*.json; do
  mcp-publisher publish "$f"
done
```

PulseMCP und Glama holen sich die Daten automatisch innerhalb ~1 Woche.

### Schritt 3 — Smithery.ai (größtes aktives Directory)

```bash
npm install -g @smithery/cli
smithery login   # Browser öffnet sich

chmod +x publish-smithery.sh
./publish-smithery.sh api.deine-domain.com dein-org-name
```

### Schritt 4 — Glama.ai

Geh zu **glama.ai/mcp/servers** → "Add Server" → deine URLs eintragen.
Oder warte 1 Woche — Glama aggregiert vom offiziellen Registry automatisch.

### MCP in Claude Desktop testen

```json
// ~/.claude/claude_desktop_config.json
{
  "mcpServers": {
    "handelsregister": {
      "transport": "http",
      "url": "https://api.deine-domain.com/mcp/handelsregister",
      "headers": {
        "X-Api-Key": "dein-test-key"
      }
    }
  }
}
```

---

## 11. Monitoring & Dashboard

### Dashboard (live Statistiken aller APIs)

```
https://api.deine-domain.com/dashboard
```

Passwort: `DASHBOARD_PASSWORD` aus `.env`.

Zeigt für jede API: Requests/min, Fehlerrate, Uptime, Response Times.

### Monitor (Health Checks)

```
https://api.deine-domain.com/monitor
```

Prüft alle 40+ Services alle 5 Minuten. Sendet Telegram/Discord/Email Alert wenn ein Service ausfällt.

### Gateway Status (Circuit Breaker)

```bash
curl https://api.deine-domain.com/gateway/status
# Zeigt Circuit-Breaker-State für jeden Upstream: closed/open/half-open
```

### Logs

```bash
# Alle Services
docker compose logs -f

# Einzelner Service
docker compose logs -f clinicaltrials-api

# Nur Fehler
docker compose logs --tail=100 | grep -i error

# Gateway Requests
docker compose logs -f gateway | grep '"status"'
```

### Service neu starten wenn er hängt

```bash
docker compose restart clinicaltrials-api

# Oder rebuild wenn es ein Code-Problem ist:
cd /srv/apis/deploy
./update.sh clinicaltrials-api
```

---

## 12. Troubleshooting

### Service startet nicht

```bash
docker compose logs --tail=50 <service-name>
# Häufige Ursachen:
# - Port schon belegt: ss -tlnp | grep 808x
# - .env Wert fehlt: grep SERVICE_NAME /srv/apis/deploy/.env
```

### nginx 502 Bad Gateway

```bash
# Ist der Service überhaupt up?
curl http://localhost:8080/health   # Port des Services prüfen

# nginx Fehlerlog:
docker compose logs --tail=30 nginx
```

### SSL Zertifikat läuft ab / Renewal schlägt fehl

```bash
certbot renew --dry-run   # Test
certbot renew             # Wirklich erneuern
docker compose restart nginx
```

### RapidAPI gibt 403 zurück

```bash
# Proxy Secret stimmt nicht überein:
grep RAPIDAPI_PROXY_SECRET /srv/apis/deploy/.env
# Mit dem Wert in RapidAPI Dashboard vergleichen (Settings → Security)
docker compose restart gateway
```

### Circuit Breaker ist offen (Gateway blockiert Requests)

```bash
# Status checken:
curl -H "X-Admin-Secret: $ADMIN_SECRET" https://api.deine-domain.com/gateway/status

# Manuell resetten:
curl -X POST -H "X-Admin-Secret: $ADMIN_SECRET" \
  https://api.deine-domain.com/gateway/reset/clinicaltrials-api
```

### Disk voll

```bash
df -h
docker system prune -f          # unbenutzte Images löschen
docker volume prune -f          # unbenutzte Volumes
```

### Alles neu bauen (nuclear option)

```bash
cd /srv/apis/deploy
docker compose down
docker system prune -af
git pull
docker compose build --parallel
docker compose up -d
```

---

## Schnellreferenz — wichtige Pfade

| Was | Wo |
|-----|----|
| Alle Configs | `/srv/apis/deploy/` |
| Secrets | `/srv/apis/deploy/.env` |
| nginx Config | `/srv/apis/deploy/nginx.conf` |
| docker-compose | `/srv/apis/deploy/docker-compose.yml` |
| Logs | `docker compose logs -f` |
| Dashboard | `https://DOMAIN/dashboard` |
| Gateway Status | `https://DOMAIN/gateway/status` |
| Health Check | `https://DOMAIN/health` |
| MCP Endpoints | `https://DOMAIN/mcp/<service-name>` |
| API Endpoints | `https://DOMAIN/v1/<endpoint>` |
| Marketplace Configs | `/srv/apis/deploy/marketplace/*.env` |

## Schnellreferenz — wichtige Befehle

```bash
# Alles starten
cd /srv/apis/deploy && docker compose up -d

# Alles stoppen
docker compose down

# Update (nach git push)
./update.sh

# Einen Service neu starten
./update.sh <service-name>

# Logs live
docker compose logs -f

# Health aller Services
for p in $(seq 8080 8121); do
  r=$(curl -sf --max-time 2 http://localhost:$p/health && echo OK || echo FAIL)
  echo "$p: $r"
done

# Neuen API-Key generieren
openssl rand -hex 20

# Key zur .env hinzufügen
echo "API_KEYS=$(grep ^API_KEYS .env | cut -d= -f2),NEUERKEY:pro" >> .env
docker compose restart gateway
```
