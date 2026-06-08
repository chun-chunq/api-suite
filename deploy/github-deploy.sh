#!/usr/bin/env bash
# ══════════════════════════════════════════════════════════════════════════════
#  github-deploy.sh  —  ONE-COMMAND deployment from GitHub
#
#  Run this on a fresh Ubuntu 22.04/24.04 server:
#
#    curl -fsSL https://raw.githubusercontent.com/YOUR_USER/YOUR_REPO/main/deploy/github-deploy.sh | bash
#
#  Or clone first and then run:
#    git clone https://github.com/YOUR_USER/YOUR_REPO /srv/apis
#    cd /srv/apis/deploy
#    cp .env.template .env && nano .env
#    chmod +x github-deploy.sh && ./github-deploy.sh
#
# ══════════════════════════════════════════════════════════════════════════════
set -euo pipefail

REPO_URL="${REPO_URL:-https://github.com/YOUR_USER/YOUR_REPO.git}"
INSTALL_DIR="${INSTALL_DIR:-/srv/apis}"
BRANCH="${BRANCH:-main}"

GREEN='\033[0;32m'; YELLOW='\033[1;33m'; RED='\033[0;31m'; CYAN='\033[0;36m'; NC='\033[0m'
info()    { echo -e "${GREEN}[INFO]${NC}  $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC}  $*"; }
error()   { echo -e "${RED}[ERR]${NC}   $*"; exit 1; }
section() { echo -e "\n${CYAN}══ $* ══${NC}"; }

# ── Must run as root or with sudo ─────────────────────────────────────────────
if [[ $EUID -ne 0 ]]; then
  error "Please run as root: sudo bash github-deploy.sh"
fi

section "1/7 System dependencies"
apt-get update -qq
apt-get install -y -qq git curl ca-certificates gnupg

# ── Docker ────────────────────────────────────────────────────────────────────
if ! command -v docker &>/dev/null; then
  info "Installing Docker..."
  curl -fsSL https://get.docker.com | sh
  systemctl enable --now docker
  info "Docker installed: $(docker --version)"
else
  info "Docker already installed: $(docker --version)"
fi

if ! docker compose version &>/dev/null; then
  info "Installing Docker Compose plugin..."
  apt-get install -y docker-compose-plugin
fi
info "Docker Compose: $(docker compose version)"

# ── Clone or update repo ──────────────────────────────────────────────────────
section "2/7 Repository"
if [[ -d "$INSTALL_DIR/.git" ]]; then
  info "Repository already exists at $INSTALL_DIR — pulling latest..."
  cd "$INSTALL_DIR"
  git fetch origin
  git reset --hard "origin/$BRANCH"
  git clean -fd
  info "Updated to $(git log --oneline -1)"
else
  info "Cloning $REPO_URL → $INSTALL_DIR ..."
  git clone --depth=1 --branch "$BRANCH" "$REPO_URL" "$INSTALL_DIR"
  cd "$INSTALL_DIR"
  info "Cloned at $(git log --oneline -1)"
fi

DEPLOY_DIR="$INSTALL_DIR/deploy"
cd "$DEPLOY_DIR"

# ── Environment file ─────────────────────────────────────────────────────────
section "3/7 Environment"
if [[ ! -f .env ]]; then
  cp .env.template .env
  warn ".env created from template. IMPORTANT: Edit it now before continuing!"
  warn "  nano $DEPLOY_DIR/.env"
  echo ""
  read -rp "Press ENTER when you've finished editing .env (Ctrl+C to abort)..."
else
  info ".env already exists — using existing configuration"
  # Show summary (without secrets)
  info "Active settings:"
  grep -v "^#" .env | grep -v "^$" | sed 's/=.*/=***/' | head -20 || true
fi

# Sanity check
if grep -qE '(change-me|my-secret-key-1|YOUR_)' .env 2>/dev/null; then
  warn ".env still contains placeholder values — update before going live!"
fi

# ── SSL Certificate ───────────────────────────────────────────────────────────
section "4/7 SSL"
DOMAIN=$(grep "^DOMAIN=" .env 2>/dev/null | cut -d= -f2 || echo "")
EMAIL=$(grep "^SSL_EMAIL=" .env 2>/dev/null | cut -d= -f2 || echo "")

if [[ -n "$DOMAIN" && -n "$EMAIL" && "$DOMAIN" != "yourdomain.com" ]]; then
  if [[ ! -f "/etc/letsencrypt/live/$DOMAIN/fullchain.pem" ]]; then
    info "Setting up SSL for $DOMAIN ..."
    chmod +x setup-ssl.sh
    ./setup-ssl.sh "$DOMAIN" "$EMAIL"
  else
    info "SSL certificate already exists for $DOMAIN"
  fi
else
  warn "No domain/email in .env — skipping SSL setup (HTTP only)"
  warn "  Set DOMAIN=yourdomain.com and SSL_EMAIL=you@example.com in .env"
fi

# ── Build & Start ─────────────────────────────────────────────────────────────
section "5/7 Build Docker images"
info "Building all images in parallel (first run ~5-10 min)..."
docker compose build --parallel 2>&1 | tail -20

section "6/7 Start services"
docker compose up -d --remove-orphans
info "Waiting for services to initialize..."
sleep 15

# ── Health verification ───────────────────────────────────────────────────────
section "7/7 Health check"

# All service ports and names
declare -A SERVICES=(
  [8080]="insolvency-api"  [8081]="zvg-api"          [8082]="ted-api"
  [8083]="dpma-api"        [8084]="sanctions-api"     [8085]="safety-api"
  [8086]="zefix-api"       [8087]="bafin-api"         [8088]="dashboard"
  [8089]="gleif-api"       [8090]="cordis-api"        [8091]="monitor"
  [8092]="handelsregister" [8093]="euipo-api"         [8094]="french-company-api"
  [8095]="uk-company-api"  [8096]="research-api"      [8097]="gdpr-api"
  [8098]="sec-api"         [8099]="food-api"          [8100]="aviation-api"
  [8101]="weather-api"     [8102]="currency-api"      [8103]="openfda-api"
  [8104]="wikidata-api"    [8105]="crypto-api"        [8106]="books-api"
  [8107]="ipgeo-api"       [8108]="vat-api"           [8109]="countries-api"
  [8110]="pubchem-api"     [8111]="nasa-api"          [8112]="pokeapi"
  [8113]="airquality-api"  [8114]="exchangerate-api"  [8115]="gbif-api"
  [8116]="trivia-api"      [8117]="numbers-api"       [8118]="joke-api"
  [8119]="namepredict-api" [8120]="worldbank-api"     [8121]="clinicaltrials-api"
)

OK=0; FAIL=0
for PORT in $(echo "${!SERVICES[@]}" | tr ' ' '\n' | sort -n); do
  NAME="${SERVICES[$PORT]}"
  if curl -sf --max-time 5 "http://localhost:$PORT/health" >/dev/null 2>&1; then
    echo -e "  ${GREEN}✅${NC}  $NAME (port $PORT)"
    ((OK++))
  else
    echo -e "  ${RED}❌${NC}  $NAME (port $PORT)"
    ((FAIL++))
  fi
done

echo ""
info "Health summary: $OK OK, $FAIL failed"

if [[ $FAIL -gt 0 ]]; then
  warn "Failed services — check logs:"
  warn "  docker compose logs --tail=50 <service-name>"
fi

# ── Summary ───────────────────────────────────────────────────────────────────
section "Done!"
DOMAIN_OR_IP=$(grep "^DOMAIN=" .env 2>/dev/null | cut -d= -f2 || hostname -I | awk '{print $1}')
echo ""
echo "  🚀 API Suite is running!"
echo ""
echo "  Dashboard:  http://$DOMAIN_OR_IP/dashboard  (or https:// if SSL configured)"
echo "  Monitor:    http://$DOMAIN_OR_IP/monitor"
echo "  Health:     http://$DOMAIN_OR_IP/health"
echo ""
echo "  Update later:  cd $INSTALL_DIR && git pull && cd deploy && docker compose up -d --build"
echo "  View logs:     docker compose logs -f"
echo "  Stop all:      docker compose down"
echo ""
echo "  Edit config:   nano $DEPLOY_DIR/.env"
echo ""
