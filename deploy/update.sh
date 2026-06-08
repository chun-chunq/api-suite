#!/usr/bin/env bash
# update.sh — pull latest from GitHub + rebuild and restart services
# Usage:
#   ./update.sh              → pull + rebuild + restart all
#   ./update.sh dpma-api     → rebuild and restart only dpma-api (no git pull)
set -euo pipefail

DEPLOY_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(dirname "$DEPLOY_DIR")"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'
info() { echo -e "${GREEN}[INFO]${NC}  $*"; }
warn() { echo -e "${YELLOW}[WARN]${NC}  $*"; }

SERVICE="${1:-}"

if [[ -n "$SERVICE" ]]; then
  # Single service restart (no git pull)
  cd "$DEPLOY_DIR"
  info "Rebuilding and restarting $SERVICE..."
  docker compose build "$SERVICE"
  docker compose up -d --no-deps "$SERVICE"
  info "Done. Logs: docker compose logs -f $SERVICE"
  exit 0
fi

# ── Pull latest code ─────────────────────────────────────────────────────────
cd "$REPO_DIR"
if [[ -d ".git" ]]; then
  info "Pulling latest code from GitHub..."
  BEFORE=$(git rev-parse HEAD 2>/dev/null || echo "none")
  git fetch origin 2>/dev/null || warn "git fetch failed — proceeding with local code"
  git reset --hard origin/$(git branch --show-current 2>/dev/null || echo "main") 2>/dev/null || true
  AFTER=$(git rev-parse HEAD 2>/dev/null || echo "none")
  if [[ "$BEFORE" != "$AFTER" ]]; then
    info "Updated to: $(git log --oneline -1)"
  else
    info "Already at latest commit."
  fi
else
  warn "Not a git repo — skipping git pull"
fi

# ── Rebuild and restart ──────────────────────────────────────────────────────
cd "$DEPLOY_DIR"
info "Rebuilding changed images in parallel..."
docker compose build --parallel

info "Restarting services (rolling update)..."
docker compose up -d --remove-orphans

info "Waiting for services to start..."
sleep 10

# ── Quick health check ────────────────────────────────────────────────────────
FAIL=0
for port in 8000 8080 8088 8091; do
  name=""
  case $port in
    8000) name="gateway" ;;
    8080) name="insolvency-api" ;;
    8088) name="dashboard" ;;
    8091) name="monitor" ;;
  esac
  if curl -sf --max-time 5 "http://localhost:$port/health" >/dev/null 2>&1; then
    info "$name (port $port): ✅"
  else
    warn "$name (port $port): ❌  →  docker compose logs --tail=50 $name"
    ((FAIL++)) || true
  fi
done

if [[ $FAIL -gt 0 ]]; then
  warn "$FAIL service(s) not healthy after update."
else
  info "All services healthy. Update complete."
fi
