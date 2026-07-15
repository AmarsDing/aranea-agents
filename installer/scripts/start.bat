@echo off
setlocal enabledelayedexpansion
cd /d "%~dp0"

REM Debug / fallback console launcher.
REM Normal users should use AraneaLauncher.exe (no black console window).
REM This script is kept for troubleshooting and CI smoke tests.

title Aranea-Agents (Debug Console)
echo ============================================
echo   Aranea-Agents Debug Launcher
echo   Prefer desktop shortcut / AraneaLauncher.exe
echo ============================================
echo.

if exist "%~dp0AraneaLauncher.exe" (
    echo Delegating to AraneaLauncher.exe ...
    start "" "%~dp0AraneaLauncher.exe"
    exit /b 0
)

echo [WARN] AraneaLauncher.exe missing — using legacy console path.
echo.

set KRATOS_AUTH_SECRET=aranea-portable-dev-secret-32chars!!
set DEPLOY_ENV=dev
set DAO_VECTOR_PGVECTOR=1
set PGDATA=%~dp0postgres\data
set PGBIN=%~dp0postgres\bin
set LOGDIR=%~dp0logs

if not exist "%LOGDIR%" mkdir "%LOGDIR%"

echo [1/5] Checking PostgreSQL...
if exist "%PGDATA%\PG_VERSION" goto pg_start

echo   First run: initializing database...
"%PGBIN%\initdb.exe" -D "%PGDATA%" -U postgres --auth=trust --encoding=UTF8 > "%LOGDIR%\initdb.log" 2>&1
if errorlevel 1 (
    echo   ERROR: initdb failed! See %LOGDIR%\initdb.log
    pause
    exit /b 1
)

:pg_start
findstr /C:"lc_messages = 'C'" "%PGDATA%\postgresql.conf" >nul 2>&1
if errorlevel 1 echo lc_messages = 'C' >> "%PGDATA%\postgresql.conf"

"%PGBIN%\pg_ctl.exe" start -D "%PGDATA%" -l "%LOGDIR%\postgres.log" -o "-p 5433" -w > "%LOGDIR%\pgctl.log" 2>&1
timeout /t 2 /nobreak >nul

"%PGBIN%\psql.exe" -U postgres -h 127.0.0.1 -p 5433 -tc "SELECT 1 FROM pg_database WHERE datname = 'aranea'" > "%LOGDIR%\dbcheck.tmp" 2>&1
find "1" "%LOGDIR%\dbcheck.tmp" >nul 2>&1
if not errorlevel 1 goto db_ready
"%PGBIN%\psql.exe" -U postgres -h 127.0.0.1 -p 5433 -c "CREATE DATABASE aranea" > "%LOGDIR%\dbsetup.log" 2>&1
"%PGBIN%\psql.exe" -U postgres -h 127.0.0.1 -p 5433 -d aranea -c "CREATE EXTENSION IF NOT EXISTS vector" >> "%LOGDIR%\dbsetup.log" 2>&1
:db_ready
if exist "%LOGDIR%\dbcheck.tmp" del "%LOGDIR%\dbcheck.tmp" >nul 2>&1

echo [2/5] Checking Redis...
tasklist /fi "imagename eq redis-server.exe" | find "redis-server.exe" >nul 2>&1
if not errorlevel 1 goto redis_done
start "" /b "%~dp0redis\redis-server.exe" --port 6379 --bind 127.0.0.1
timeout /t 1 /nobreak >nul
:redis_done

echo [3/5] Starting backend...
powershell -NoProfile -Command "try { $r = Invoke-WebRequest -Uri 'http://127.0.0.1:8000/healthz' -UseBasicParsing -TimeoutSec 2; if ($r.StatusCode -eq 200) { exit 0 } else { exit 1 } } catch { exit 1 }" >nul 2>&1
if not errorlevel 1 goto backend_ready
taskkill /im aranea-server.exe /f >nul 2>&1
start "" /b cmd /c "cd /d %~dp0 && aranea-server.exe -conf configs >> logs\server.log 2>&1"

echo [4/5] Waiting for backend...
set /a wait_count=0
:wait_backend
timeout /t 1 /nobreak >nul
set /a wait_count+=1
powershell -NoProfile -Command "try { $r = Invoke-WebRequest -Uri 'http://127.0.0.1:8000/healthz' -UseBasicParsing -TimeoutSec 2; if ($r.StatusCode -eq 200) { exit 0 } else { exit 1 } } catch { exit 1 }" >nul 2>&1
if not errorlevel 1 goto backend_ready
if %wait_count% geq 30 (
    echo   WARNING: backend not ready. See logs\server.log
    goto start_frontend
)
goto wait_backend

:backend_ready
:start_frontend
echo [5/5] Starting desktop app...
tasklist /fi "imagename eq AraneaAgents.exe" | find "AraneaAgents.exe" >nul 2>&1
if not errorlevel 1 goto app_done
start "" "%~dp0frontend\AraneaAgents.exe"
:app_done

echo.
echo Started. Login: admin / changeme
echo Close this window anytime — services keep running.
echo To stop: AraneaLauncher.exe -stop  or  stop.bat
echo.
exit /b 0
