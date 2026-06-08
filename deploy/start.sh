#!/usr/bin/env bash
# ══════════════════════════════════════════════════════════════════════════════
#  start.sh — deploy all APIs on a fresh Hetzner VPS (Ubuntu 22.04/24.04)
#  Usage:
#    1. Upload this whole folder to the server
#    2. cp .env.template .env && nano .env  (fill in secrets)
#    3. chmod +x start.sh && ./start.sh
# ══════════════════════════════════════════════════════════════════════════════
set -euo pipefail

DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DEPLOY_DIR"

# ── Colours ────────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; NC='\033[0m'
info()  { echo -e "${GREEN}[INFO]${NC}  $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error() { echo -e "${RED}[ERR]${NC}   $*"; }

# ── Check .env ─────────────────────────────────────────────────────────────────
if [[ ! -f .env ]]; then
  error ".env not found!"
  echo "  Run: cp .env.template .env && nano .env"
  exit 1
fi

# Quick sanity check for placeholder values
if grep -qE '(change-me|my-secret-key-1)' .env; then
  warn ".env still contains placeholder values — edit .env before going live!"
fi

# ── Install Docker if missing ──────────────────────────────────────────────────
if ! command -v docker &>/dev/null; then
  info "Installing Docker..."
  curl -fsSL https://get.docker.com | sh
  systemctl enable --now docker
  info "Docker installed."
fi

if ! docker compose version &>/dev/null; then
  info "Installing Docker Compose plugin..."
  apt-get install -y docker-compose-plugin
fi

# ── Check API source folders exist ────────────────────────────────────────────
MISSING=()
for dir in insolvency-api zvg-api ted-api dpma-api sanctions-api safety-api zefix-api bafin-api gleif-api cordis-api monitor dashboard handelsregister-api; do
  if [[ ! -d "../$dir" ]]; then
    MISSING+=("../$dir")
  fi
done
if [[ ${#MISSING[@]} -gt 0 ]]; then
  error "Missing API source folders: ${MISSING[*]}"
  echo "  Expected layout:"
  echo "    /srv/apis/"
  echo "      deploy/           ← this folder"
  echo "      insolvency-api/"
  echo "      zvg-api/"
  echo "      ted-api/"
  echo "      dpma-api/"
  echo "      sanctions-api/"
  exit 1
fi

# ── Pull / build ───────────────────────────────────────────────────────────────
info "Building Docker images (this takes ~3 min on first run)..."
docker compose build --parallel

# ── Start ─────────────────────────────────────────────────────────────────────
info "Starting all APIs..."
docker compose up -d

# Wait for health checks
info "Waiting for services to be healthy..."
sleep 10

# ── Health check ──────────────────────────────────────────────────────────────
PORTS=(8080 8081 8082 8083 8084 8085 8086 8087 8088 8089 8090 8091 8092)
NAMES=(insolvency-api zvg-api ted-api dpma-api sanctions-api safety-api zefix-api bafin-api dashboard gleif-api cordis-api monitor handelsregister-api)
ALL_OK=true

for i in "${!PORTS[@]}"; do
  PORT="${PORTS[$i]}"
  NAME="${NAMES[$i]}"
  if curl -sf "http://localhost:$PORT/health" >/dev/null 2>&1; then
    info "${NAME} (port $PORT) ✅"
  else
    warn "${NAME} (port $PORT) ❌ — check: docker compose logs $NAME"
    ALL_OK=false
  fi
done

echo ""
if $ALL_OK; then
  info "All APIs running!"
else
  warn "Some APIs failed. Check logs above."
fi

echo ""
echo "  Useful commands:"
echo "    docker compose logs -f               # stream all logs"
echo "    docker compose logs -f sanctions-api  # one service"
echo "    docker compose ps                    # status"
echo "    docker compose restart dpma-api      # restart one"
echo "    ./stop.sh                            # stop all"
echo ""
echo "  Admin endpoints (set X-Admin-Secret: <your-ADMIN_SECRET>):"
echo "    GET http://localhost:8080/admin/analytics"
echo "    GET http://localhost:8083/admin/stats"
echo ""
echo "  PC-Worker (run on Windows home PC):"
echo "    cd C:\\pc-worker && pc-worker.exe"
echo "    (edit config.yaml: server_url, worker_secret)"
