# Apify Marketplace Listing Guide

Apify is the leading web scraping / data automation marketplace.
Your scraper and data APIs are a perfect fit — Apify users love ready-made data actors.

## Actors to publish

| Actor folder | Apify name | Apify category |
|---|---|---|
| `C:\apify-insolvency\` | german-insolvency-register | Business > Finance |
| `C:\apify-sanctions\` | eu-sanctions-screening | Business > Compliance |
| `C:\apify-gleif\` | gleif-lei-lookup | Business > Finance |
| `C:\apify-dpma\` | dpma-trademark-register | Legal > Business |
| `C:\apify-bafin\` | bafin-licensed-institutions | Business > Compliance |

---

## Step-by-step: Publishing an Actor

### 1. Create Apify account
- https://console.apify.com/sign-up (free)
- Verify email

### 2. Install Apify CLI
```bash
npm install -g apify-cli
apify login   # uses your Apify API token from console.apify.com/account/integrations
```

### 3. Push each actor

For each actor folder (e.g. `C:\apify-insolvency\`):
```bash
cd C:\apify-insolvency
apify push
```

This uploads the Dockerfile, source, and schema to your Apify account.

### 4. Set the actor name and description
After push, go to https://console.apify.com/actors → your actor:
- **Title**: as in actor.json
- **Description**: long Markdown description (see template below)
- **Categories**: as listed in table above
- **SEO keywords**: add 5-10 relevant terms

### 5. Set pricing

Apify actors use their own credit system.
Recommended pricing per actor:

| Actor | Monthly credits equiv. | Price |
|---|---|---|
| insolvency | 10 credits/run | Free up to 50 runs/mo, then $9.99/mo |
| sanctions | 2 credits/run | Free up to 200 runs/mo, then $9.99/mo |
| gleif | 2 credits/run | Free up to 200 runs/mo, then $9.99/mo |
| dpma | 10 credits/run | Free up to 20 runs/mo, then $19.99/mo |
| bafin | 10 credits/run | Free up to 20 runs/mo, then $19.99/mo |

> Note: You earn ~70% of Apify revenue (they take 30%).
> Alternatively, offer actors free and monetize through RapidAPI API keys (actors call your API).

### 6. Test before publishing
In Apify console → actor → Run with test input:
```json
{
  "apiKey": "your-test-key",
  "name": "Test Company GmbH"
}
```

### 7. Publish to Apify Store
Actor settings → "Publish to Apify Store" → fills SEO metadata → Submit.

Review takes 1-3 business days.

---

## Actor Long Description Template

Use this template for each actor (adapt per actor):

```markdown
## German Insolvency Register Actor

Search the official German insolvency register (Insolvenzbekanntmachungen.de) programmatically.

### What you get
- Debtor name and type (person / company / partnership)
- Court name, case number, proceedings type
- Opening date, status
- City and state

### Use cases
- **Credit checks** — screen customers or counterparties before extending credit
- **Debt collection** — find insolvency cases for your debtors
- **Due diligence** — M&A or investment screening
- **Risk monitoring** — watch-list alerts for existing customers
- **Legal research** — find all proceedings at a specific court

### Data source
Official German court announcements portal: insolvenzbekanntmachungen.de
Updated in real time as courts publish new proceedings.

### Input
| Parameter | Type | Description |
|---|---|---|
| apiKey | string | Your API key (required) |
| name | string | Debtor name to search |
| city | string | City filter |
| maxResults | integer | Max results (default 50) |

### Output
Array of insolvency proceedings with all available details.

### Pricing
Free for up to 50 results/run. See actor pricing for higher volumes.
```

---

## Other Marketplaces to List On

Beyond RapidAPI and Apify:

| Marketplace | Focus | Action |
|---|---|---|
| **APILayer** | Business/Finance APIs | https://apilayer.com/marketplace → Submit API |
| **Zyla API Hub** | All categories | https://zylalabs.com → Submit → fill form |
| **APIs.guru** | Open API directory | https://apis.guru/add-api/ → submit OpenAPI YAML |
| **RapidAPI** | Already covered | See RAPIDAPI_LISTING.md |
| **MCPize** | MCP/AI tools | https://mcpize.com → Submit tool |
| **Glama.ai** | MCP registry | https://glama.ai/mcp/servers → add JSON config |
| **Smithery** | MCP marketplace | https://smithery.ai → submit |

### For Glama.ai / MCPize (MCP registries)
Submit this JSON for each MCP-enabled API:
```json
{
  "name": "EU Sanctions Screening",
  "description": "Screen names against EU consolidated sanctions list",
  "url": "https://api.yourdomain.com/mcp",
  "transport": "http",
  "auth": {
    "type": "header",
    "header": "X-API-Key"
  }
}
```

APIs with MCP:
- `https://api.yourdomain.com/mcp` → dpma-api (trademark search)
- `https://api.yourdomain.com/mcp` → sanctions-api (sanctions check)  
- `https://api.yourdomain.com/mcp` → gleif-api (LEI lookup)

> ⚠️ MCP endpoints are currently on the same domain — each needs a unique sub-path.
> Add nginx routing: `/mcp/sanctions` → port 8084, `/mcp/lei` → port 8089, `/mcp/trademark` → port 8083
