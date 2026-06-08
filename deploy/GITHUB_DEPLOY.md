# GitHub → Server One-Command Deployment

## Repo Structure

Your entire API suite lives in **one GitHub repository**:

```
your-repo/
├── deploy/                   ← docker-compose, nginx, scripts, .env.template
│   ├── docker-compose.yml
│   ├── nginx.conf
│   ├── .env.template
│   ├── github-deploy.sh      ← ONE-COMMAND deploy script
│   ├── start.sh
│   ├── stop.sh
│   └── update.sh
├── gateway/                  ← API Gateway (circuit breaker + auth)
├── insolvency-api/
├── zvg-api/
├── ... (all other APIs)
├── monitor/
├── dashboard/
└── .gitignore
```

## Step 1: Push to GitHub

On your Windows machine:

```bash
# If you haven't already set up git:
git config --global user.email "louismaximilianmeyer@gmail.com"
git config --global user.name "Your Name"

# Navigate to the root of your project (one level above deploy/)
# e.g. if deploy/ is at C:\deploy, go to C:\
cd C:\

# Initialize repo (or this might already be done)
git init
git add .
git commit -m "Initial API suite commit"

# Create repo on GitHub (via web or CLI):
#   gh repo create api-suite --private
#   OR go to github.com → New Repository → name: api-suite

# Push
git remote add origin https://github.com/YOUR_USER/api-suite.git
git push -u origin main
```

> **Important**: The `.gitignore` excludes `deploy/.env` — your secrets never go to GitHub.

## Step 2: Deploy on Server (ONE command)

SSH into your Hetzner/VPS server and run:

```bash
# Option A: curl directly from GitHub (no prior setup needed)
curl -fsSL https://raw.githubusercontent.com/YOUR_USER/api-suite/main/deploy/github-deploy.sh | sudo bash

# Option B: Clone manually, then run deploy script
git clone https://github.com/YOUR_USER/api-suite.git /srv/apis
cd /srv/apis/deploy
cp .env.template .env
nano .env          # Fill in your secrets
chmod +x github-deploy.sh
sudo ./github-deploy.sh
```

The script will:
1. Install Docker (if not installed)
2. Clone/update the repo
3. Prompt you to fill in `.env` if not already done
4. Set up SSL with Let's Encrypt (if DOMAIN is set)
5. Build all Docker images in parallel
6. Start all services
7. Run health checks on all ports
8. Show you the result

## Step 3: Updates

When you push new code to GitHub, update the server with:

```bash
# On the server:
cd /srv/apis
git pull
cd deploy
docker compose up -d --build
```

Or use the included `update.sh` script:
```bash
cd /srv/apis/deploy
./update.sh
```

## Environment Variables (`.env`)

| Variable | Description | Required |
|---|---|---|
| `ADMIN_SECRET` | Admin access key for management endpoints | Yes |
| `DOMAIN` | Your domain (e.g. `api.example.com`) | For HTTPS |
| `SSL_EMAIL` | Email for Let's Encrypt | For HTTPS |
| `DASHBOARD_PASSWORD` | Analytics dashboard password | Yes |
| `API_KEYS` | Customer API keys (KEY:TIER format) | Optional |
| `RAPIDAPI_PROXY_SECRET` | For RapidAPI marketplace | Marketplace |
| `ALLOW_ANONYMOUS` | Allow keyless access (default: true) | Optional |
| `TELEGRAM_BOT_TOKEN` | For health alerts | Optional |
| `NASA_API_KEY` | Higher NASA API rate limits | Optional |

## Private Repo Access on Server

If your repo is **private**, the server needs SSH key access:

```bash
# On server: generate a deploy key
ssh-keygen -t ed25519 -f ~/.ssh/github_deploy -N ""
cat ~/.ssh/github_deploy.pub
# Copy the output → GitHub repo Settings → Deploy keys → Add deploy key

# Then clone with SSH instead:
git clone git@github.com:YOUR_USER/api-suite.git /srv/apis
```

## Multiple Servers

To run on multiple servers, just repeat Step 2 on each server with the same repo URL. Each server gets its own `.env` file.
