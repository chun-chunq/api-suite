# API Pricing Strategy

> Competitive analysis based on RapidAPI market research (June 2026).
> All prices in USD. Adjust for APILayer (EUR) accordingly.

---

## Pricing Model per API

### Tiers used across all APIs
| Tier | Price/month | Requests/month | Notes |
|------|------------|----------------|-------|
| **Free** | $0 | 100 | Good for testing/discovery. No credit card. |
| **Basic** | $9.99 | 1,000 | Freelancers, small tools |
| **Pro** | $49.99 | 10,000 | SMEs, SaaS startups |
| **Business** | $199/mo | 100,000 | Agencies, compliance teams |
| **Enterprise** | Custom | Unlimited | Banks, large integrations (min $500/mo) |

> **Rate limits:** Free = 1 req/min, Basic = 5/min, Pro = 20/min, Business = 60/min

---

## Per-API Recommendations

### 1. 🏚 Insolvency API (Port 8080)
**Target:** Debt collection, banks, credit insurers, landlords screening tenants
**Recommended pricing:** Pro model
- Free: 50 req/mo
- Basic: $14.99 — 1,000 req
- Pro: $79.99 — 15,000 req
- Business: $299/mo — 100,000 req
- **Rationale:** Credit/risk data commands higher prices. Creditreform charges €0.50+ per query. Even at $0.005/req we're 100x cheaper with self-serve UX.

---

### 2. 🏠 ZVG Foreclosure API (Port 8081)
**Target:** Real estate investors, property funds, Immoscout data enrichment, Makler
**Recommended pricing:** Standard model
- Free: 50 req/mo
- Basic: $9.99 — 500 req
- Pro: $39.99 — 5,000 req
- Business: $149/mo — 50,000 req
- **Rationale:** Only API for this data. Niche audience, but willing to pay for unique data. Real estate investors process deals daily.

---

### 3. 📋 TED EU Procurement API (Port 8082)
**Target:** Consulting firms, bid writers, procurement software, CRM enrichment
**Recommended pricing:** Volume model (high usage)
- Free: 100 req/mo
- Basic: $9.99 — 2,000 req
- Pro: $49.99 — 30,000 req
- Business: $199/mo — 500,000 req (TED data is public, margin comes from scale)
- **Rationale:** Companies run daily monitoring. High request volume expected. Keep price low to compete with official TED API complexity.

---

### 4. ™️ DPMA Trademark API (Port 8083)
**Target:** Brand agencies, patent attorneys, trademark monitoring SaaS, entrepreneurs checking name availability
**Recommended pricing:** Premium model
- Free: 30 req/mo (trademark searches are expensive to run — Chrome)
- Basic: $19.99 — 500 req
- Pro: $79.99 — 5,000 req
- Business: $349/mo — 50,000 req
- Enterprise: $999+/mo (white-label for law firms)
- **Rationale:** No competitor for DPMA-specific data. IP attorneys bill €300/h. Even $0.04/query is a bargain.

---

### 5. 🚫 EU Sanctions API (Port 8084)
**Target:** Fintech KYC/AML, banks, crypto exchanges, e-commerce fraud prevention
**Recommended pricing:** High-value compliance model
- Free: 100 req/mo (compliance demo)
- Basic: $19.99 — 5,000 req
- Pro: $99.99 — 100,000 req
- Business: $399/mo — 1,000,000 req (bulk screening)
- Enterprise: Custom (for banks running nightly batch checks)
- **Rationale:** Compliance is non-negotiable. Companies run this check on EVERY customer. AML solutions like ComplyAdvantage charge $1000+/mo. We can position at the affordable end.

---

### 6. ⚠️ EU Safety Gate / RAPEX API (Port 8085)
**Target:** Amazon FBA sellers, importers, e-commerce compliance, retailers, product liability lawyers
**Recommended pricing:** Standard model
- Free: 100 req/mo
- Basic: $9.99 — 2,000 req
- Pro: $49.99 — 25,000 req
- Business: $199/mo — 500,000 req
- **Rationale:** Amazon sellers NEED this to avoid ASIN suspensions. Large importers run bulk checks on product catalogs. Pure Go = very low operating cost → high margin.

---

### 7. 🇨🇭 Swiss Zefix Company API (Port 8086)
**Target:** DACH compliance tools, Swiss KYB, M&A due diligence, Swiss business directories
**Recommended pricing:** Standard model
- Free: 100 req/mo
- Basic: $9.99 — 1,000 req
- Pro: $39.99 — 10,000 req
- Business: $149/mo — 100,000 req
- **Rationale:** Niche but steady demand. Swiss companies do lots of cross-border business. Operating cost near-zero (REST wrapper).

---

### 8. 🏦 BaFin Licensed Institutions API (Port 8087)
**Target:** Fintech compliance, crypto exchanges verifying partners, M&A in financial services, due diligence
**Recommended pricing:** Premium compliance model
- Free: 30 req/mo
- Basic: $19.99 — 300 req (Chrome-heavy, high cost)
- Pro: $99.99 — 5,000 req
- Business: $399/mo — 50,000 req (cache-heavy, manageable)
- **Rationale:** Extremely niche + critical. Fintech companies are well-funded. No competition. A bank checking if their payment partner is really BaFin-licensed is worth $1+ per check.

---

### 9. 🏛 GLEIF LEI Lookup API (Port 8089)
**Target:** Banks, fintech, compliance teams, MiFID II / EMIR reporting, KYB platforms, Bloomberg/Refinitiv alternative
**Recommended pricing:** High-value compliance model
- Free: 200 req/mo
- Basic: $14.99 — 5,000 req
- Pro: $79.99 — 100,000 req
- Business: $299/mo — 2,000,000 req (regulatory reporting runs millions of checks)
- Enterprise: Custom (for banks with LEI validation in trading systems)
- **Rationale:** LEI is legally required for every MiFID II trade report. Banks must validate LEIs in real-time. Refinitiv charges $2000+/mo for entity data. Zero competition on RapidAPI. Near-zero operating cost (wraps free GLEIF API).

---

### 10. 🔬 CORDIS EU Research Grants API (Port 8090)
**Target:** Research institutions, universities, grant consultants, EU funding advisors, innovation hubs, VCs tracking EU-funded startups
**Recommended pricing:** Standard/volume model
- Free: 100 req/mo
- Basic: $9.99 — 2,000 req
- Pro: $39.99 — 25,000 req
- Business: $149/mo — 200,000 req
- **Rationale:** €100B+ in EU grants are largely invisible to private organizations without a data layer. Grant consultants bill €150/h and need to search by country, topic, budget. Zero competition on RapidAPI. Pure REST wrapper = very low cost.

---

## Revenue Projections (Conservative)

Assuming 50 paying customers across all 8 APIs after 3 months:

| Mix | Monthly Revenue |
|-----|----------------|
| 30× Basic (~$15 avg) | $450 |
| 15× Pro (~$65 avg) | $975 |
| 5× Business (~$250 avg) | $1,250 |
| **Total** | **~$2,675/mo** |

After 12 months with growth and Enterprise deals: **$8,000–15,000/mo** is realistic.

---

## Platform Fees
- **RapidAPI:** Takes 20% of revenue (net you get 80%)
- **APILayer:** Takes 20% similar
- **Direct (Stripe):** No platform fee but more work to acquire customers

**Recommendation:** List on RapidAPI first (free exposure), then add direct billing for Enterprise.

---

## Marketplace Listing Priority

Build these first for RapidAPI listing (already tested):
1. insolvency-api ← list this week
2. zvg-api ← list this week
3. ted-api ← list this week

Then list after more testing:
4. dpma-api
5. sanctions-api
6. safety-api
7. zefix-api
8. bafin-api
