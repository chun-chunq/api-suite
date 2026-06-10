@echo off
echo ============================================================
echo  PC-Worker - Insolvenz ^& ZVG Scrape-Worker
echo ============================================================
echo.

REM Prüfe ob config.yaml vorhanden
if not exist "%~dp0config.yaml" (
    echo FEHLER: config.yaml nicht gefunden!
    echo Bitte config.yaml im gleichen Ordner wie diese .bat-Datei erstellen.
    pause
    exit /b 1
)

echo Starte Worker... (Fenster offen lassen!)
echo Zum Beenden: Ctrl+C druecken
echo.

"%~dp0pc-worker.exe"

echo.
echo Worker beendet.
pause
