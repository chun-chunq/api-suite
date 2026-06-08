# Marketplace Configuration Files

One `.env` file per marketplace. Each file contains **all settings** needed to connect
your server to that marketplace — no hunting through docs.

## Files

| File | Marketplace | Commission | Competition | Listing |
|------|-------------|------------|-------------|---------|
| `rapidapi.env` | RapidAPI | 20% | Mixed (see ranking) | Open, instant |
| `apilayer.env` | APILayer | **15%** (best) | Low for niche APIs | Application, ~1-2 weeks |
| `apimarket.env` | API.market | 20% (drops to 15% at $2,500 MRR) | **Very low** | Open, instant |
| `zyla.env` | Zyla API Hub | 20% | Low | Open, instant |

## How to Use

```bash
# To deploy for a specific marketplace, copy its env file:
cp deploy/marketplace/rapidapi.env deploy/.env

# Fill in your actual secrets:
nano deploy/.env

# Start the stack:
cd deploy && docker compose up -d
```

## Key Differences Per Marketplace

### RapidAPI
- Inject: `X-RapidAPI-Proxy-Secret` header (validate in gateway)
- Also sends: `X-RapidAPI-Key`, `X-RapidAPI-Subscription`, `X-RapidAPI-User`
- Your gateway validates `RAPIDAPI_PROXY_SECRET` to block direct traffic

### APILayer
- Inject: `apikey` header (already handled by gateway)
- 15% commission — best rate available
- Don't list VAT or FX APIs (they own those categories)

### API.market
- Inject: `X-Api-Key` header (already handled by gateway)
- Early stage — list everything for first-mover advantage

### Zyla
- Inject: `Authorization: Bearer <token>` (already handled by gateway)
- Low volume, easy to list, minimal maintenance

## MCP Registries (separate from API marketplaces)

Your APIs also expose MCP at `/mcp/<service>` — usable directly in Claude Desktop, Cursor, etc.

| Registry | File | Notes |
|----------|------|-------|
| Official MCP Registry | `mcp.env` | Use `generate-mcp-servers.sh` to generate server.json files |
| Smithery.ai | `mcp.env` | Use `publish-smithery.sh` to bulk publish |
| Glama.ai | `mcp.env` | Manual web form or auto-feeds from official registry |
| PulseMCP | — | Auto-feeds from official registry (no config needed) |

MCP registries don't pay you — they drive traffic to your API key sales.

## Detailed Analysis

See `../MARKETPLACE_RANKING.md` for the full revenue/competition breakdown and action plan.
