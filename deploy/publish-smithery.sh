#!/usr/bin/env bash
# publish-smithery.sh
# Bulk-publishes all APIs to Smithery.ai (smithery.ai)
#
# Usage:
#   ./publish-smithery.sh yourdomain.com yourorg
#
# Prerequisites:
#   npm install -g @smithery/cli
#   smithery login   (one-time, browser OAuth)

set -euo pipefail

DOMAIN="${1:-your-server-domain.com}"
ORG="${2:-yourorg}"
MCP_BASE="https://${DOMAIN}/mcp"

# Map of slug → display name
declare -A APIS=(
  [handelsregister]="Handelsregister API"
  [insolvency]="Insolvency API"
  [zvg]="ZVG Foreclosure API"
  [dpma]="DPMA Patent & Trademark API"
  [sanctions]="Sanctions API"
  [gleif]="GLEIF LEI API"
  [bafin]="BaFin API"
  [euipo]="EUIPO Trademark API"
  [french-company]="French Company API"
  [uk-company]="UK Companies House API"
  [ted]="TED EU Tenders API"
  [cordis]="CORDIS EU Research API"
  [clinicaltrials]="ClinicalTrials.gov API"
  [pubchem]="PubChem Chemistry API"
  [gbif]="GBIF Biodiversity API"
  [nasa]="NASA Data API"
  [sec]="SEC EDGAR API"
  [research]="Research API"
  [gdpr]="GDPR Decisions API"
  [openfda]="OpenFDA Drug API"
  [vat]="VAT Validation API"
  [exchangerate]="Exchange Rate API"
  [worldbank]="World Bank Data API"
  [crypto]="Crypto Price API"
  [currency]="Currency Conversion API"
  [airquality]="Air Quality API"
  [countries]="Countries API"
  [weather]="Weather API"
  [wikidata]="Wikidata API"
  [ipgeo]="IP Geolocation API"
  [books]="Books API"
  [food]="Food & Nutrition API"
  [aviation]="Aviation API"
  [safety]="Product Safety API"
  [zefix]="ZEFIX Swiss Company API"
  [pokeapi]="PokéAPI"
  [namepredict]="Name Prediction API"
  [trivia]="Trivia API"
  [numbers]="Numbers API"
  [jokes]="Jokes API"
)

SUCCESS=0
FAIL=0

for slug in "${!APIS[@]}"; do
  name="${APIS[$slug]}"
  url="${MCP_BASE}/${slug}"
  smithery_name="@${ORG}/${slug}-api"

  echo "Publishing: ${smithery_name} → ${url}"
  if smithery mcp publish "$url" \
      -n "$smithery_name" \
      --config-schema '{
        "type": "object",
        "properties": {
          "apiKey": {
            "type": "string",
            "title": "API Key",
            "description": "Your API key. Get one at https://'"${DOMAIN}"' or via RapidAPI.",
            "secret": true
          }
        },
        "required": ["apiKey"]
      }' 2>/dev/null; then
    echo "  ✅ Published"
    ((SUCCESS++)) || true
  else
    echo "  ⚠️  Failed (may already exist or need manual review)"
    ((FAIL++)) || true
  fi
done

echo ""
echo "════════════════════════════════════════════════════════════"
echo " Smithery publish complete: ${SUCCESS} ok, ${FAIL} failed"
echo " View your servers: https://smithery.ai/server/@${ORG}"
echo "════════════════════════════════════════════════════════════"
