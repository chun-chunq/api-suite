# Marketplace Setup Guide
**Server:** api.novaroute.fyi (live)  
**Repo:** github.com/chun-chunq/api-suite  
**Reihenfolge:** RapidAPI → API.market → APILayer → Zyla → MCP

---

## Vorbereitung (einmalig, gilt für alle Marketplaces)

Du brauchst einen **API-Key** für dich selbst zum Testen.  
Generiere einen auf dem Server:
```bash
openssl rand -hex 20
```
Trag ihn in `/srv/apis/deploy/.env` ein:
```
API_KEYS=dein-test-key:ultra
```
Dann: `docker compose restart gateway`

Test ob er funktioniert:
```bash
curl -H "X-Api-Key: dein-test-key" https://api.novaroute.fyi/v1/sanctions/search?q=test
```

---

## 1. RapidAPI (höchste Priorität)

**URL:** https://rapidapi.com/provider  
**Kommission:** 20% nimmt RapidAPI, 80% bekommst du  
**Auszahlung:** Stripe, ab $10  

### Schritt 1 — Account erstellen
1. Geh auf https://rapidapi.com/provider
2. "Sign Up" → mit GitHub oder Email
3. Provider Hub öffnen → "My APIs" → "Add New API"

### Schritt 2 — Proxy Secret einrichten
1. In RapidAPI: API erstellen → "Settings" → kopiere den **Proxy Secret**
2. Auf dem Server:
```bash
nano /srv/apis/deploy/.env
# Zeile eintragen: RAPIDAPI_PROXY_SECRET=dein-proxy-secret-hier
docker compose restart gateway
```
Das Gateway prüft dann automatisch alle RapidAPI-Anfragen.

### Schritt 3 — Welche APIs zuerst listen (Top 10 für den Start)

| API | Endpoint auf deinem Server | Warum |
|-----|---------------------------|-------|
| Insolvency (DE) | `/v1/insolvency/` | Einzigartig, B2B, keine Konkurrenz |
| ZVG Foreclosures | `/v1/zvg/` | Einzigartig, DE-only |
| DPMA Trademarks | `/v1/trademark/` | Solide B2B-Nachfrage |
| EU Sanctions | `/v1/sanctions/` | Compliance, viele Käufer |
| GLEIF LEI Lookup | `/v1/lei/` | Finanzbranche braucht das |
| VAT Validation | `/v1/vat/` | Sehr gefragt, viele Shops/SaaS |
| UK Companies House | `/v1/uk/` | Englischsprachig → globale Reichweite |
| SEC EDGAR | `/v1/sec/` | US-Markt, viel Traffic |
| EU TED Procurement | `/v1/ted/` | GovTech-Nische, kaum Konkurrenz |
| GDPR Fines Tracker | `/v1/gdpr/` | Compliance, Alleinstellung |

### Schritt 4 — Eine API anlegen (Beispiel: EU Sanctions)

1. "Add New API" → Name: `EU Sanctions List API`
2. **Base URL:** `https://api.novaroute.fyi`
3. **Category:** Business → Finance / Legal
4. Endpoints manuell eintragen:

```
GET /v1/sanctions/search     ?q=name&country=DE
GET /v1/sanctions/{id}
GET /v1/sanctions/recent
```

5. **Authentication:**
   - Type: `Header`
   - Header Name: `X-Api-Key`
   - RapidAPI injiziert automatisch den Key über ihren Proxy — du musst nichts weiter tun

6. **Pricing Plans anlegen:**

| Plan | Preis | Limit | Für wen |
|------|-------|-------|---------|
| Free | $0 | 50 req/Tag | Zum Testen |
| Basic | $9/Mo | 1.000 req/Tag | Freelancer |
| Pro | $29/Mo | 10.000 req/Tag | Startups |
| Ultra | $99/Mo | Unlimited | Enterprise |

7. Publish → "Make Public"

### Musterbeschreibung (anpassen pro API)
```
Real-time search across the official EU Consolidated Sanctions List.
Covers individuals, entities, and vessels sanctioned by the 
European Union. Updated daily from the official EU data source.

Use cases: KYC/AML compliance, onboarding screening, 
risk management, RegTech automation.

Data source: European Commission (CC BY 4.0)
```

### Tipp: Alle 10 APIs nacheinander anlegen
Ca. 30 Min pro API. Mach eine pro Tag — nach 2 Wochen sind alle Top-APIs draußen.

---

## 2. API.market (zweite Priorität)

**URL:** https://api.market  
**Kommission:** ~15%  
**Vorteil:** Sehr wenig Konkurrenz, einfache Submission, extra Reichweite  

### Schritte
1. https://api.market → "Submit API" → Account erstellen
2. Für jede API: Name, Base URL (`https://api.novaroute.fyi`), Beschreibung, Endpoints
3. **Gleiche APIs listen wie auf RapidAPI**
4. Preis: 10% günstiger als RapidAPI (zieht Käufer an)

Kein Proxy Secret nötig — API.market leitet `X-Api-Key` Header direkt durch.  
Dein Gateway akzeptiert das out of the box.

---

## 3. APILayer (dritte Priorität)

**URL:** https://apilayer.com/marketplace  
**Kommission:** 15%  

**⚠️ NICHT listen:**
- VAT-APIs → APILayer besitzt Vatlayer.com (direkte Konkurrenz, werden abgelehnt)
- FX/Currency → APILayer besitzt Fixer.io (gleiche Begründung)

### Welche APIs für APILayer
Alles außer VAT und FX:
Sanctions, GLEIF, GDPR, SEC EDGAR, UK Companies, Handelsregister, TED, ZVG, Insolvency, DPMA

### Schritte
1. https://apilayer.com → "List Your API" → Formular
2. Base URL, Endpoints, Beschreibung (copy-paste aus RapidAPI)
3. Gleicher Preis wie RapidAPI
4. Review-Prozess: 1–3 Tage

---

## 4. Zyla API Hub (vierte Priorität)

**URL:** https://zylalabs.com  
**Kommission:** ~20%  
**Strategie:** Nur Top 5, Rest lohnt Aufwand nicht  

### Nur diese 5 listen
1. Insolvency Register
2. EU Sanctions
3. GLEIF LEI
4. VAT Validation
5. GDPR Fines

### Schritte
1. https://zylalabs.com/api-creator → Account
2. "Create API" → gleiche Daten wie RapidAPI
3. Preis: 10% unter RapidAPI

---

## 5. MCP Registry + Smithery (für KI-Tool-Nutzer)

**Was das ist:** Claude, Cursor, GPT etc. können deine APIs direkt als KI-Tools nutzen.  
**Geld verdienen:** Gleiche API-Keys wie überall — Registries sind nur Discovery.

### Smithery.ai
1. https://smithery.ai → Account → Org: `novaroute`
2. Für jede API einen MCP-Server anlegen:
   - Transport: `streamable-http`
   - URL: `https://api.novaroute.fyi/mcp/sanctions` (Beispiel)
   - Auth: Header `X-Api-Key`

Das Skript `deploy/publish-smithery.sh` macht das für alle APIs auf einmal  
(auf dem Server: `npm install -g @smithery/cli` dann Skript ausführen).

### Offizielles MCP Registry
1. `npm install -g mcp-publisher`
2. `mcp-publisher login http --domain api.novaroute.fyi`
3. Domain-Verification-Datei ablegen: `/srv/apis/deploy/well-known/mcp-registry-auth`
4. `deploy/generate-mcp-servers.sh` ausführen → erstellt `server.json` pro API
5. Pro API: `mcp-publisher publish server.json`

---

## Realistischer Zeitplan

| Woche | Aufgabe | Zeit |
|-------|---------|------|
| 1 | RapidAPI Account + Proxy Secret + erste 5 APIs | ~3h |
| 2 | RapidAPI restliche 5 APIs | ~2h |
| 3 | API.market: alle 10 listen (copy-paste) | ~2h |
| 4 | APILayer: Top 8 ohne VAT/FX | ~2h |
| Monat 2 | Zyla Top 5 + Smithery MCP | ~2h |

---

## Checkliste vor dem ersten Listing

- [ ] Server läuft: `curl https://api.novaroute.fyi/` gibt JSON zurück
- [ ] RAPIDAPI_PROXY_SECRET in `.env` eingetragen und `gateway` neugestartet
- [ ] Eigener Test-Key funktioniert (curl-Test oben)
- [ ] RapidAPI Account erstellt + Stripe für Auszahlungen verbunden
- [ ] Mindestens eine API öffentlich erreichbar getestet

---

## Troubleshooting

**RapidAPI gibt 403 zurück:**
```bash
grep RAPIDAPI_PROXY_SECRET /srv/apis/deploy/.env   # prüfen ob gesetzt
docker compose restart gateway
```

**API antwortet nicht:**
```bash
docker compose ps    # läuft der Container?
docker compose logs sanctions-api --tail=20
```

**Nach .env-Änderungen immer:**
```bash
cd /srv/apis/deploy && docker compose restart gateway
```
