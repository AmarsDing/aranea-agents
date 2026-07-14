@echo off
title Aranea-Agents Stopper
echo ============================================
echo   Stopping Aranea-Agents...
echo ============================================

REM ---- 1. Stop frontend (Electron) ----
echo [1/4] Closing desktop app...
taskkill /im AraneaAgents.exe /f /t >nul 2>&1
if %errorlevel%==0 (echo   Desktop app closed.) else (echo   Desktop app not running.)

REM ---- 2. Stop backend ----
echo [2/4] Stopping backend service...
taskkill /im aranea-server.exe /f /t >nul 2>&1
taskkill /fi "windowtitle eq Aranea Server*" /f /t >nul 2>&1
if %errorlevel%==0 (echo   Backend service stopped.) else (echo   Backend service not running.)

REM ---- 3. Stop Redis ----
echo [3/4] Stopping Redis...
taskkill /im redis-server.exe /f /t >nul 2>&1
taskkill /fi "windowtitle eq Aranea Redis*" /f /t >nul 2>&1
if %errorlevel%==0 (echo   Redis stopped.) else (echo   Redis not running.)

REM ---- 4. Stop PostgreSQL ----
echo [4/4] Stopping PostgreSQL...
"%~dp0postgres\bin\pg_ctl.exe" stop -D "%~dp0postgres\data" -m fast >nul 2>&1
if %errorlevel%==0 (echo   PostgreSQL stopped.) else (echo   PostgreSQL not running or already stopped.)

REM ---- Wait for ports to be released ----
echo   Waiting for ports to be released...
timeout /t 2 /nobreak >nul

echo.
echo ============================================
echo   Aranea-Agents fully stopped.
echo ============================================
echo.
pause
