# RapidAPI & Marketplace Listing Guide

## Listing Priority Order

List these immediately (most tested, highest demand):
1. **sanctions-api** — EU Sanctions / AML (best market fit, highest price)
2. **gleif-api** — GLEIF LEI Lookup (zero competition, regulatory demand)
3. **insolvency-api** — German Insolvency Register
4. **zvg-api** — German Foreclosure Auctions
5. **ted-api** — EU TED Procurement
6. **dpma-api** — German Trademark Register
7. **safety-api** — EU Safety Gate / RAPEX
8. **zefix-api** — Swiss Company Register
9. **bafin-api** — BaFin Licensed Institutions
10. **cordis-api** — EU Research Grants

---

## RapidAPI Listing — Step by Step

### 1. Create Account
- Go to https://rapidapi.com/provider
- Sign up as a provider (free)
- Verify email

### 2. Create New API (repeat for each)

**Dashboard:** https://rapidapi.com/provider/apis → "Add New API"

Fill in:
- **API Name:** (see table below)
- **Category:** (see table below)
- **Short description:** (copy from table below)
- **Base URL:** `https://api.yourdomain.com`

### API Names, Categories, Descriptions

| API | Name on RapidAPI | Category | Short Description |
|-----|-----------------|----------|-------------------|
| sanctions-api | EU Sanctions & AML Screening API | Finance > Compliance | Instantly screen individuals and companies against the EU consolidated sanctions list. Required for AML/KYC compliance. 1,500+ sanctioned entities updated daily. |
| gleif-api | GLEIF LEI Lookup & Validation API | Finance > Data | Search and validate Legal Entity Identifiers (LEI) from the GLEIF Global LEI Index. Required for MiFID II/EMIR trade reporting. 2.3M+ entities, corporate ownership chains. |
| insolvency-api | German Insolvency Register API | Finance > Data | Search German insolvency proceedings in real time. Screen debtors, monitor court cases, retrieve case details. Wraps the official Insolvenzbekanntmachungen.de portal. |
| zvg-api | German Foreclosure Auctions API | Real Estate > Data | Search German ZVG foreclosure auctions by location and property type. Access official auction dates, property descriptions, and estimated values. |
| ted-api | EU TED Procurement Tenders API | Business > Data | Search 1M+ EU public procurement notices from TED (Tenders Electronic Daily). Filter by country, CPV code, buyer, value. Wraps the official TED REST API. |
| dpma-api | German Trademark Register API (DPMA) | Legal > Data | Search German trademark registrations at the DPMA (Deutsches Patent- und Markenamt). Check trademark availability, retrieve owner details, status, and classes. |
| safety-api | EU Safety Gate Product Recalls API | E-Commerce > Data | Search dangerous product alerts and recalls from the EU Safety Gate / RAPEX system. Filter by product type, category, country, risk type. Updated weekly. |
| zefix-api | Swiss Company Register API (Zefix) | Business > Data | Search Swiss companies in the official Zefix commercial register. Get UID, legal form, address, purpose, and SHAB publications. |
| bafin-api | BaFin Licensed Institutions API | Finance > Compliance | Search BaFin-licensed banks, payment institutions, crypto asset firms, investment firms, and insurers. Verify regulatory authorization in Germany. |
| cordis-api | EU CORDIS Research Grants API | Science > Data | Search 50,000+ EU-funded research projects (Horizon Europe, H2020, FP7). Filter by keyword, country, programme, status. Find partners, track competitors. |

### 3. Add Endpoints in RapidAPI

For each API, add endpoints matching the OpenAPI spec.
Example for sanctions-api:

| Method | Path | Name |
|--------|------|------|
| GET | /v1/sanctions/search | Search Sanctioned Entities |
| GET | /v1/sanctions/check | Compliance Check (yes/no) |
| GET | /health/sanctions | Health Check |

### 4. Set Pricing Tiers

In RapidAPI dashboard → "Pricing Plans":

**Copy the tiers from PRICING.md for each API.**

For the Free tier, always:
- Enable "Allow unauthenticated requests": NO
- Request limit: as per PRICING.md
- Rate limit: 1 request/minute

### 5. Upload OpenAPI Spec

Each API has `openapi.yaml` in its folder.
In RapidAPI dashboard → "API Definition" → Import OpenAPI → upload the file.

Locations:
- `C:\gleif-api\openapi.yaml`
- `C:\cordis-api\openapi.yaml`
- (write specs for remaining APIs — same structure)

### 6. Set Base URL + Test

- Base URL: `https://api.yourdomain.com`
- Test each endpoint in the RapidAPI console before publishing
- Make sure your server is running: `docker compose up -d`

### 7. Add Descriptions (for SEO + conversion)

Each API needs:
- **Long description** (Markdown) — explain use cases, data source, update frequency
- **Code examples** in curl, Python, JavaScript, Go
- **Screenshots** if possible

### 8. Publish

Click "Publish API" — it goes live immediately.

---

## APILayer Listing

1. Go to https://apilayer.com/marketplace
2. "Submit API" → fill same info as RapidAPI
3. APILayer focuses on: Finance, Business Data, Compliance
4. Best fit: sanctions-api, gleif-api, bafin-api

---

## MCPize Registration (MCP Registry)

MCP (Model Context Protocol) lets AI assistants like Claude call your APIs.

### Which APIs have MCP endpoints:
- dpma-api: `/mcp` (already implemented)
- **Add MCP to:** sanctions-api, gleif-api (highest value for AI assistants)

### Register at MCP registries:
1. https://mcpize.com — submit your API
2. https://glama.ai/mcp/servers — open registry
3. https://smithery.ai — MCP marketplace

### MCP listing info needed:
- API name
- Base URL: `https://api.yourdomain.com`
- MCP endpoint: `https://api.yourdomain.com/mcp` (for each API)
- Description of tools provided
- Auth: X-API-Key header

---

## Apify Actor (Optional)

Wrap each scraper API as an Apify Actor for the Apify marketplace.
Best candidates: insolvency-api, bafin-api, dpma-api (browser-based).

Actor input schema maps to API query params.
Not urgent — focus on RapidAPI first.

---

## Checklist Before Going Live

- [ ] Domain configured: `api.yourdomain.com` → Hetzner server IP
- [ ] SSL cert issued: `./setup-ssl.sh api.yourdomain.com your@email.com`
- [ ] All services running: `docker compose up -d` in `C:\deploy\`
- [ ] `.env` filled in from `.env.template`
- [ ] Impressum placeholders filled: `C:\deploy\static\impressum.html`
- [ ] Privacy page reviewed: `C:\deploy\static\privacy.html`
- [ ] Test each `/health/` endpoint returns 200
- [ ] Test each API with curl using your API key
- [ ] RapidAPI account created and verified
- [ ] First 3 APIs listed on RapidAPI
