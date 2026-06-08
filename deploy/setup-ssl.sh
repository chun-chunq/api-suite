#!/usr/bin/env bash
# setup-ssl.sh — install Nginx + get Let's Encrypt SSL cert
# Run ONCE on a fresh server after pointing your domain DNS to this IP.
# Usage: ./setup-ssl.sh api.yourdomain.com your@email.com
set -euo pipefail

DOMAIN="${1:-}"
EMAIL="${2:-}"

if [[ -z "$DOMAIN" || -z "$EMAIL" ]]; then
  echo "Usage: ./setup-ssl.sh api.yourdomain.com your@email.com"
  exit 1
fi

echo "Setting up Nginx + SSL for $DOMAIN"

# Install Nginx + Certbot
apt-get update -qq
apt-get install -y nginx certbot python3-certbot-nginx

# Copy static legal pages (Impressum + Datenschutz)
mkdir -p /var/www/static
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cp "$SCRIPT_DIR/static/impressum.html" /var/www/static/impressum.html
cp "$SCRIPT_DIR/static/privacy.html"   /var/www/static/privacy.html
echo "Static pages copied to /var/www/static/"

# Write Nginx config (substitute domain)
sed "s/api.yourdomain.com/$DOMAIN/g" "$SCRIPT_DIR/nginx.conf" > /etc/nginx/sites-available/apis
ln -sf /etc/nginx/sites-available/apis /etc/nginx/sites-enabled/apis
rm -f /etc/nginx/sites-enabled/default

# Test Nginx config
nginx -t

# Get SSL certificate
certbot --nginx -d "$DOMAIN" --email "$EMAIL" --agree-tos --non-interactive

# Enable auto-renewal
systemctl enable certbot.timer

# Reload Nginx
systemctl reload nginx

echo ""
echo "✅ SSL configured for $DOMAIN"
echo "   Test: curl https://$DOMAIN/health/insolvency"
echo ""
echo "Next: update .env and run ./start.sh"
