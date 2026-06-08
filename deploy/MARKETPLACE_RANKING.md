# API Marketplace Ranking — Revenue & Competition Analysis

## TL;DR — Recommended Priority

| Rank | Marketplace | Monthly Revenue Potential | Competition | Action |
|------|-------------|--------------------------|-------------|--------|
| 🥇 1 | **RapidAPI** | $500–$5,000/mo (realistic) | High on VAT/FX, low on ClinicalTrials/GBIF/PubChem | List NOW, focus niche APIs |
| 🥈 2 | **API.market** | $50–$500/mo | Very low across ALL categories | Early mover advantage — list everything |
| 🥉 3 | **APILayer** | $200–$2,000/mo | Avoid VAT+FX (they own those), perfect for everything else | Apply, pitch niche APIs |
| 4 | **Zyla API Hub** | $10–$150/mo | Thin but tiny user base | Low effort to list, low return |
| ❌ | Postman | $0 (no billing) | — | Use for discovery/marketing only |
| ❌ | Algorithmia | Dead | — | Skip |
| ❌ | Eden AI | Wrong niche | — | Skip |

---

## Detailed Analysis

### 🥇 1. RapidAPI — Best Overall

**Revenue potential**: High  
**Competition**: Varies dramatically by category  
**Commission**: 20% (you keep 80%)  
**Self-hosted**: YES — you run your API, they proxy

**Where you WIN on RapidAPI (low competition):**
| Your API | Competing APIs on RapidAPI | Verdict |
|----------|---------------------------|---------|
| ClinicalTrials.gov | ~2 (thin, poorly maintained) | ✅ Strong opportunity |
| GBIF Biodiversity | 0–1 | ✅ Almost no competition |
| PubChem Chemistry | 1–2 | ✅ Strong opportunity |
| World Bank Data | 2–3 (low quality) | ✅ Good opportunity |
| Name Prediction | 3–4 | 🟡 Moderate competition |
| NASA Data | 3–5 | 🟡 Moderate, but you can undercut |
| Air Quality | 5+ (Weatherbit, Ambee) | 🔴 Tough |
| VAT Validation | 6+ competing | 🔴 Tough |
| FX Rates | 8+ competing | 🔴 Very tough |
| Trivia/Jokes/Numbers | Dozens | 🔴 Saturated |

**Strategy**: On RapidAPI, push **ClinicalTrials, GBIF, PubChem, World Bank** hard.
Price them lower than competitors. Bundle similar APIs.

**Revenue reality**: Median API earns ~$100/month. Top 5% earn $1K–$10K/month.
With ~40 APIs and some niche ones, realistically $300–$1,500/month total at launch.

---

### 🥈 2. API.market — Best for Low Competition

**Revenue potential**: Lower volume, but first-mover advantage  
**Competition**: Very thin across ALL your categories  
**Commission**: 20% (negotiable to 15% above $2,500 MRR)  
**Self-hosted**: YES — proxy model  
**Listing**: Open, free, fast

**Why it matters**: API.market is growing and actively recruiting quality providers.
Your APIs would be among the first in most categories. Early listings = better discovery ranking.

---

### 🥉 3. APILayer — Best Commission, Niche Only

**Revenue potential**: Medium — smaller user base but higher quality B2B customers  
**Competition**: Almost none in your niche categories  
**Commission**: 15% (best rate of any major marketplace)  
**Self-hosted**: YES  
**Listing**: Application-based (curated) — takes 1–2 weeks

**⚠️ AVOID listing these on APILayer** (they own competing products):
- VAT Validation → they own Vatlayer.com ($29–$99/mo plans)
- FX Rates → they own Fixer.io + Currencylayer

**List THESE on APILayer** (no competition):
- ClinicalTrials, PubChem, GBIF, World Bank, NASA, Air Quality, Name Prediction

---

### 4. Zyla API Hub — Easy but Small

**Revenue potential**: Low ($10–$150/month realistic)  
**Competition**: Thin  
**Commission**: 20%  
**Why list**: Minimal effort, extra passive income  
**Why not prioritize**: Very small developer audience

---

---

## MCP Registries — Claude / AI Agent Distribution

Your APIs already expose MCP JSON-RPC 2.0 at `/mcp/<service>`. This makes them
directly usable in **Claude Desktop, Cursor, ChatGPT plugins, and any MCP-compatible client**.
MCP registries are discovery channels — no revenue share, but they drive traffic
to your API key sales (RapidAPI or direct).

| Registry | Servers | Revenue | Transport | Submission |
|----------|---------|---------|-----------|------------|
| **modelcontextprotocol.io/registry** | Early-stage | Discovery only | Streamable HTTP ✅ | CLI + `server.json` per API |
| **Smithery.ai** | Thousands | Discovery only | Streamable HTTP ✅ | Open, CLI |
| **Glama.ai** | 30,000+ | Discovery only | HTTP/SSE ✅ | Open web form (or auto from registry) |
| **PulseMCP** | Large | Discovery only | N/A (aggregator) | Auto-feeds from official registry |

**MCP Flow:**
```
User finds your API on Smithery
  → wants to use it in Claude Desktop
  → Claude asks for X-Api-Key
  → user buys subscription on your site (or RapidAPI)
  → enters key → Claude calls your /mcp endpoint
```

**Your setup:**
- All MCP endpoints: `https://DOMAIN/mcp/<service-name>`
- Gateway returns **401** on missing/invalid key ✅ (required by Smithery)
- `deploy/generate-mcp-servers.sh` generates all `server.json` files
- `deploy/publish-smithery.sh` bulk-publishes to Smithery
- `deploy/well-known/` folder → nginx serves `/.well-known/mcp-registry-auth`

**Best niche MCP servers (low competition in AI agent ecosystem):**
- Handelsregister, Insolvency, DPMA, GLEIF — no competition at all
- ClinicalTrials, PubChem, GBIF — rare specialized science data
- Sanctions, BaFin, ZVG — compliance/legal data unique to this suite

---

## Action Plan

### Week 1: RapidAPI
1. Create provider account at rapidapi.com/provider
2. Copy `RAPIDAPI_PROXY_SECRET` from settings → paste in `.env`
3. List your top 5 niche APIs: ClinicalTrials, GBIF, PubChem, World Bank, Name Prediction
4. Set pricing (start free tier + $9/mo basic to attract early users)

### Week 2: API.market  
1. Register at api.market/seller
2. List all APIs (open submission, fast)
3. Price slightly lower than RapidAPI to capture early market

### Week 3: APILayer
1. Apply at apilayer.com/marketplace/submit
2. Pitch: ClinicalTrials, GBIF, PubChem, World Bank, NASA, Air Quality
3. Wait for review (1–2 weeks)

### Week 4: MCP Registries
1. Run `./generate-mcp-servers.sh yourdomain.com yourorg` → 40 `server.json` files
2. `npm install -g @modelcontextprotocol/publisher` then publish all to official registry
3. Run `./publish-smithery.sh yourdomain.com yourorg` → Smithery bulk publish
4. Visit `glama.ai/mcp/servers` → Add Server for your top 10 niche APIs
5. PulseMCP auto-picks up from the official registry within ~1 week

### Ongoing: Postman
- Export OpenAPI spec, publish to Postman API Network for discovery
- No revenue but brings customers who find you and buy on RapidAPI/APILayer
