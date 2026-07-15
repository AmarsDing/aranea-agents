@echo off
cd /d "%~dp0"

if exist "%~dp0AraneaLauncher.exe" (
    start "" "%~dp0AraneaLauncher.exe" -stop
    exit /b 0
)

title Aranea-Agents Stopper
echo Stopping Aranea-Agents...
taskkill /im AraneaAgents.exe /f /t >nul 2>&1
taskkill /im aranea-server.exe /f /t >nul 2>&1
taskkill /im redis-server.exe /f /t >nul 2>&1
"%~dp0postgres\bin\pg_ctl.exe" stop -D "%~dp0postgres\data" -m fast >nul 2>&1
echo Done.
timeout /t 2 /nobreak >nul
exit /b 0
