@echo off
REM ============================================================
REM  git-setup.bat
REM  Initializes a proper, isolated git repo at C:\api-suite\
REM  (the pack.bat output folder — NOT your entire C:\ drive)
REM
REM  HOW TO USE:
REM    1. Run pack.bat first  →  creates C:\api-suite\
REM    2. Run git-setup.bat   →  initializes git repo there
REM    3. Push to GitHub      →  follow the printed instructions
REM ============================================================

SET REPO=C:\api-suite

echo [1/5] Checking that pack.bat has been run...
if not exist "%REPO%\deploy\docker-compose.yml" (
    echo ERROR: %REPO% not found or incomplete.
    echo Please run pack.bat first.
    pause
    exit /b 1
)
echo OK - api-suite folder found.

echo [2/5] Writing .gitignore...
(
echo # Secrets — never commit these
echo deploy/.env
echo *.env
echo .env.local
echo .env.*.local
echo
echo # Build artifacts
echo **/*.exe
echo **/bin/
echo **/vendor/
echo
echo # OS files
echo .DS_Store
echo Thumbs.db
echo desktop.ini
echo
echo # Editor
echo .vscode/
echo .idea/
echo *.swp
echo *.swo
echo
echo # Logs
echo *.log
echo logs/
echo
echo # Docker volumes (if mapped locally)
echo data/
echo volumes/
) > "%REPO%\.gitignore"
echo OK

echo [3/5] Creating GitHub Actions workflow...
mkdir "%REPO%\.github\workflows" 2>nul
copy /Y "%REPO%\deploy\ci.yml" "%REPO%\.github\workflows\ci.yml" >nul
echo OK

echo [4/5] Initializing git repo...
cd /d "%REPO%"
git init
git add .
git commit -m "Initial commit: API suite with 40+ commercial Go APIs

Includes:
- 40+ REST APIs (Go/Fiber), each with client, handler, MCP, tests
- API Gateway with circuit breaker, auth, rate limiting
- nginx reverse proxy with health checks and failover
- Docker Compose stack
- Monitor + Dashboard
- Marketplace configs (RapidAPI, APILayer, API.market, Zyla)
- One-command GitHub deploy script"

echo OK

echo.
echo ============================================================
echo  GIT REPO READY AT: %REPO%
echo.
echo  READ FIRST: %REPO%\deploy\SETUP.md
echo  (vollständige Schritt-für-Schritt Anleitung)
echo ============================================================
echo.
echo  NEXT STEPS — push to GitHub:
echo.
echo  1. Go to github.com/new and create a NEW empty repo
echo     (no README, no .gitignore — we already have those)
echo     Name it e.g. "api-suite"
echo.
echo  2. Back here, run:
echo     cd C:\api-suite
echo     git remote add origin https://github.com/YOUR_USERNAME/api-suite.git
echo     git branch -M main
echo     git push -u origin main
echo.
echo  3. On your server, deploy with ONE command:
echo     curl -fsSL https://raw.githubusercontent.com/YOUR_USERNAME/api-suite/main/deploy/github-deploy.sh ^| bash
echo.
echo  To update later (after git push):
echo     ssh user@server "cd /srv/apis/deploy && ./update.sh"
echo.
pause
