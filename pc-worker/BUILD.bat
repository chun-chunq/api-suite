@echo off
echo Baue pc-worker.exe...
where go >nul 2>&1
if %ERRORLEVEL% neq 0 (
    echo FEHLER: Go ist nicht installiert oder nicht im PATH!
    echo Download: https://go.dev/dl/
    pause
    exit /b 1
)
go build -ldflags="-s -w" -o pc-worker.exe .
if %ERRORLEVEL% equ 0 (
    echo Erfolgreich gebaut: pc-worker.exe
) else (
    echo Build fehlgeschlagen!
)
pause
