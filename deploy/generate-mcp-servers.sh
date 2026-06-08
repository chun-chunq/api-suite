#!/usr/bin/env bash
# generate-mcp-servers.sh
# Generates server.json files for the official MCP Registry
# (modelcontextprotocol.io) for all 40+ APIs.
#
# Usage:
#   cd /srv/apis/deploy
#   ./generate-mcp-servers.sh yourdomain.com yourorg
#
# Output: ./mcp-servers/<api-name>.json  (one per API)
#
# After generating, publish with:
#   npm install -g @modelcontextprotocol/publisher
#   mcp-publisher login http --domain "yourdomain.com"
#   for f in mcp-servers/*.json; do mcp-publisher publish "$f"; done

set -euo pipefail

DOMAIN="${1:-your-server-domain.com}"
NAMESPACE="${2:-com.${DOMAIN//./-}}"  # e.g. com.example-com

SCHEMA="https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json"
MCP_BASE="https://${DOMAIN}/mcp"
OUT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/mcp-servers"

mkdir -p "$OUT_DIR"

# ─── Helper function ──────────────────────────────────────────────────────────
generate_server() {
  local slug="$1"         # e.g. handelsregister
  local title="$2"        # e.g. "Handelsregister API"
  local description="$3"  # short description
  local version="${4:-1.0.0}"
  local out="${OUT_DIR}/${slug}.json"

  cat > "$out" <<EOF
{
  "\$schema": "${SCHEMA}",
  "name": "${NAMESPACE}/${slug}",
  "title": "${title}",
  "description": "${description}",
  "version": "${version}",
  "license": "Proprietary",
  "homepage": "https://${DOMAIN}",
  "repository": null,
  "remotes": [
    {
      "type": "streamable-http",
      "url": "${MCP_BASE}/${slug}",
      "headers": [
        {
          "name": "X-Api-Key",
          "description": "Your API key. Get one at https://${DOMAIN} or via RapidAPI.",
          "isRequired": true,
          "isSecret": true
        }
      ]
    }
  ]
}
EOF
  echo "  ✅ $out"
}

echo "Generating MCP server.json files for domain: ${DOMAIN}"
echo "Namespace: ${NAMESPACE}"
echo "Output: ${OUT_DIR}"
echo ""

# ── Business / Legal / Compliance ────────────────────────────────────────────
generate_server "handelsregister"   "Handelsregister API"        "German commercial register — company data, directors, filings"
generate_server "insolvency"        "Insolvency API"             "German insolvency announcements from the official Insolvenzbekanntmachungen portal"
generate_server "zvg"               "ZVG Foreclosure API"        "German foreclosure auction data from ZVG-Portal (Zwangsversteigerungen)"
generate_server "dpma"              "DPMA Patent & Trademark API" "German Patent and Trademark Office — patent and trademark search"
generate_server "sanctions"         "Sanctions API"              "Global sanctions lists: EU, OFAC, UN — entity and vessel screening"
generate_server "gleif"             "GLEIF LEI API"              "Global Legal Entity Identifier Foundation — LEI lookup and validation"
generate_server "bafin"             "BaFin API"                  "German Federal Financial Supervisory Authority — licensed entities, warnings"
generate_server "euipo"             "EUIPO Trademark API"        "European Union Intellectual Property Office — EU trademark search"
generate_server "french-company"    "French Company API"         "French company register — SIRENE/Pappers company data"
generate_server "uk-company"        "UK Companies House API"     "UK Companies House — company details, officers, filings"

# ── EU / Research / Science ───────────────────────────────────────────────────
generate_server "ted"               "TED EU Tenders API"         "EU Tenders Electronic Daily — public procurement tenders above EU thresholds"
generate_server "cordis"            "CORDIS EU Research API"     "EU-funded research projects from the CORDIS database"
generate_server "clinicaltrials"    "ClinicalTrials.gov API"     "Search and retrieve clinical trial data from ClinicalTrials.gov v2"
generate_server "pubchem"           "PubChem Chemistry API"      "Molecular and chemical compound data from NCBI PubChem"
generate_server "gbif"              "GBIF Biodiversity API"      "Species occurrences and biodiversity data from the Global Biodiversity Information Facility"
generate_server "nasa"              "NASA Data API"              "NASA APOD, near-Earth objects, DONKI solar events"
generate_server "sec"               "SEC EDGAR API"              "US Securities and Exchange Commission — company filings and financial data"
generate_server "research"          "Research API"               "Academic papers and citations — OpenAlex, CrossRef, Semantic Scholar"
generate_server "gdpr"              "GDPR Decisions API"         "GDPR enforcement decisions from European data protection authorities"
generate_server "openfda"           "OpenFDA Drug API"           "FDA drug adverse events, recalls, and labeling via openFDA"

# ── Financial / Economic ──────────────────────────────────────────────────────
generate_server "vat"               "VAT Validation API"         "EU VAT number validation via VIES — real-time company and VAT data"
generate_server "exchangerate"      "Exchange Rate API"          "Live and historical foreign exchange rates — 170+ currencies"
generate_server "worldbank"         "World Bank Data API"        "World Bank economic and development indicators — GDP, inflation, population"
generate_server "crypto"            "Crypto Price API"           "Cryptocurrency prices and market data via CoinGecko"
generate_server "currency"          "Currency Conversion API"    "Currency conversion with live rates"

# ── General / Consumer ────────────────────────────────────────────────────────
generate_server "airquality"        "Air Quality API"            "Real-time and historical air quality measurements via OpenAQ"
generate_server "countries"         "Countries API"              "Country information — capitals, currencies, languages, flags"
generate_server "weather"           "Weather API"                "Current and forecast weather via Open-Meteo (no key required upstream)"
generate_server "wikidata"          "Wikidata API"               "Structured entity data from Wikidata / Wikipedia knowledge graph"
generate_server "ipgeo"             "IP Geolocation API"         "IP address geolocation — country, city, ASN, timezone"
generate_server "books"             "Books API"                  "Book search and metadata via Open Library"
generate_server "food"              "Food & Nutrition API"       "Nutritional data for foods via USDA FoodData Central"
generate_server "aviation"          "Aviation API"               "Flight data and airport information via AviationStack"
generate_server "safety"            "Product Safety API"         "EU product safety recalls from RAPEX/Safety Gate"
generate_server "zefix"             "ZEFIX Swiss Company API"    "Swiss commercial register — company data from ZEFIX"
generate_server "pokeapi"           "PokéAPI"                    "Pokémon data — species, moves, types, abilities via PokéAPI"
generate_server "namepredict"       "Name Prediction API"        "Predict age, gender, and nationality from a first name (Agify/Genderize/Nationalize)"
generate_server "trivia"            "Trivia API"                 "Random trivia questions with categories and difficulty levels"
generate_server "numbers"           "Numbers API"                "Number facts — math, trivia, date facts via NumbersAPI"
generate_server "jokes"             "Jokes API"                  "Random jokes with category and safety filters"

echo ""
echo "Generated $(ls "$OUT_DIR"/*.json | wc -l) server.json files in ${OUT_DIR}/"
echo ""
echo "════════════════════════════════════════════════════════════"
echo " NEXT STEPS"
echo "════════════════════════════════════════════════════════════"
echo ""
echo " 1. Install the MCP publisher CLI:"
echo "    npm install -g @modelcontextprotocol/publisher"
echo ""
echo " 2. Authenticate with your domain:"
echo "    mcp-publisher login http --domain '${DOMAIN}'"
echo "    (follow prompts to add /.well-known/mcp-registry-auth to your server)"
echo ""
echo " 3. Publish all servers:"
echo "    for f in ${OUT_DIR}/*.json; do mcp-publisher publish \"\$f\"; done"
echo ""
echo " 4. Publish to Smithery separately:"
echo "    ./publish-smithery.sh '${DOMAIN}' 'yourorg'"
echo ""
echo " 5. Glama.ai: visit glama.ai/mcp/servers → Add Server"
echo "    (or it auto-feeds from the official registry within ~1 week)"
