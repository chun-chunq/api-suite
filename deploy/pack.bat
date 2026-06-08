@echo off
REM ============================================================
REM  pack.bat  —  assembles everything into C:\api-suite\
REM              and pushes to GitHub automatically.
REM
REM  Usage:
REM    pack.bat                    <- commit message = timestamp
REM    pack.bat "my commit msg"    <- custom commit message
REM ============================================================

SET DEST=C:\api-suite
SET REPO=https://github.com/chun-chunq/api-suite.git

REM ── Optional commit message from first argument ───────────────────────────
SET MSG=%~1

REM ── Step 0: save .git so we don't lose history ────────────────────────────
echo [0/4] Preserving git history...
if exist "%DEST%\.git" (
    move "%DEST%\.git" "C:\.api-suite-git-tmp" >nul 2>&1
)

REM ── Step 1: clean and recreate folder ────────────────────────────────────
echo [1/4] Cleaning old pack...
if exist "%DEST%" rmdir /s /q "%DEST%"
mkdir "%DEST%"

REM restore .git right away so git works from here on
if exist "C:\.api-suite-git-tmp" (
    move "C:\.api-suite-git-tmp" "%DEST%\.git" >nul 2>&1
)

REM ── Step 2: copy all source folders ──────────────────────────────────────
echo [2/4] Copying folders...

xcopy /E /I /Q "C:\deploy"             "%DEST%\deploy"

xcopy /E /I /Q "C:\insolvency-api"      "%DEST%\insolvency-api"
xcopy /E /I /Q "C:\zvg-api"             "%DEST%\zvg-api"
xcopy /E /I /Q "C:\ted-api"             "%DEST%\ted-api"
xcopy /E /I /Q "C:\dpma-api"            "%DEST%\dpma-api"
xcopy /E /I /Q "C:\sanctions-api"       "%DEST%\sanctions-api"
xcopy /E /I /Q "C:\safety-api"          "%DEST%\safety-api"
xcopy /E /I /Q "C:\zefix-api"           "%DEST%\zefix-api"
xcopy /E /I /Q "C:\bafin-api"           "%DEST%\bafin-api"
xcopy /E /I /Q "C:\gleif-api"           "%DEST%\gleif-api"
xcopy /E /I /Q "C:\cordis-api"          "%DEST%\cordis-api"
xcopy /E /I /Q "C:\monitor"             "%DEST%\monitor"
xcopy /E /I /Q "C:\dashboard"           "%DEST%\dashboard"
xcopy /E /I /Q "C:\handelsregister-api" "%DEST%\handelsregister-api"
xcopy /E /I /Q "C:\euipo-api"           "%DEST%\euipo-api"
xcopy /E /I /Q "C:\french-company-api"  "%DEST%\french-company-api"
xcopy /E /I /Q "C:\uk-company-api"      "%DEST%\uk-company-api"
xcopy /E /I /Q "C:\research-api"        "%DEST%\research-api"
xcopy /E /I /Q "C:\gdpr-api"            "%DEST%\gdpr-api"
xcopy /E /I /Q "C:\sec-api"             "%DEST%\sec-api"
xcopy /E /I /Q "C:\food-api"            "%DEST%\food-api"
xcopy /E /I /Q "C:\aviation-api"        "%DEST%\aviation-api"
xcopy /E /I /Q "C:\weather-api"         "%DEST%\weather-api"
xcopy /E /I /Q "C:\currency-api"        "%DEST%\currency-api"
xcopy /E /I /Q "C:\openfda-api"         "%DEST%\openfda-api"
xcopy /E /I /Q "C:\wikidata-api"        "%DEST%\wikidata-api"
xcopy /E /I /Q "C:\crypto-api"          "%DEST%\crypto-api"
xcopy /E /I /Q "C:\ipgeo-api"           "%DEST%\ipgeo-api"
xcopy /E /I /Q "C:\vat-api"             "%DEST%\vat-api"
xcopy /E /I /Q "C:\countries-api"       "%DEST%\countries-api"
xcopy /E /I /Q "C:\pubchem-api"         "%DEST%\pubchem-api"
xcopy /E /I /Q "C:\nasa-api"            "%DEST%\nasa-api"
xcopy /E /I /Q "C:\airquality-api"      "%DEST%\airquality-api"
xcopy /E /I /Q "C:\exchangerate-api"    "%DEST%\exchangerate-api"
xcopy /E /I /Q "C:\gbif-api"            "%DEST%\gbif-api"
xcopy /E /I /Q "C:\namepredict-api"     "%DEST%\namepredict-api"
xcopy /E /I /Q "C:\worldbank-api"       "%DEST%\worldbank-api"
xcopy /E /I /Q "C:\clinicaltrials-api"  "%DEST%\clinicaltrials-api"
xcopy /E /I /Q "C:\gateway"             "%DEST%\gateway"
xcopy /E /I /Q "C:\resilience"          "%DEST%\resilience"

xcopy /E /I /Q "C:\apify-insolvency"    "%DEST%\apify-insolvency"
xcopy /E /I /Q "C:\apify-sanctions"     "%DEST%\apify-sanctions"
xcopy /E /I /Q "C:\apify-gleif"         "%DEST%\apify-gleif"
xcopy /E /I /Q "C:\apify-dpma"          "%DEST%\apify-dpma"
xcopy /E /I /Q "C:\apify-bafin"         "%DEST%\apify-bafin"

REM Copy GitHub Actions workflow
if not exist "%DEST%\.github\workflows" mkdir "%DEST%\.github\workflows"
copy /Y "C:\deploy\ci.yml" "%DEST%\.github\workflows\ci.yml" >nul

REM ── Step 3: git init if first time, then commit + push ───────────────────
echo [3/4] Committing and pushing to GitHub...
cd /d "%DEST%"

REM First-time setup (no remote yet)
git rev-parse --git-dir >nul 2>&1
if errorlevel 1 (
    git init
    git remote add origin %REPO%
    git branch -M main
) else (
    REM Make sure remote exists (handles re-init edge case)
    git remote get-url origin >nul 2>&1
    if errorlevel 1 git remote add origin %REPO%
)

git config user.email "louismaximilianmeyer@gmail.com"
git config user.name "Louis Meyer"

REM Build commit message: use argument or fall back to timestamp
if "%MSG%"=="" (
    for /f "tokens=2 delims==" %%a in ('wmic OS Get localdatetime /value 2^>nul') do set _dt=%%a
    set MSG=Update %_dt:~0,4%-%_dt:~4,2%-%_dt:~6,2% %_dt:~8,2%:%_dt:~10,2%
)

git add -A

REM Only commit if there are actual changes
git diff --cached --quiet
if errorlevel 1 (
    git commit -m "%MSG%"
    git push --force --set-upstream origin main
    echo.
    echo  Pushed to GitHub: %MSG%
) else (
    echo.
    echo  Nothing changed — no commit needed.
)

echo [4/4] Done!
echo.
pause
