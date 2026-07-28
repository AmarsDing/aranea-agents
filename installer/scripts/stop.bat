@echo off
REM Prefer silent launcher; avoid long-lived console.
cd /d "%~dp0"
if exist "%~dp0AraneaLauncher.exe" (
  start "" "%~dp0AraneaLauncher.exe" -stop
  exit /b 0
)
taskkill /im AraneaAgents.exe /f /t >nul 2>&1
taskkill /im aranea-server.exe /f /t >nul 2>&1
taskkill /im redis-server.exe /f /t >nul 2>&1
if exist "%~dp0postgres\bin\pg_ctl.exe" (
  "%~dp0postgres\bin\pg_ctl.exe" stop -D "%~dp0postgres\data" -m fast >nul 2>&1
)
exit /b 0
