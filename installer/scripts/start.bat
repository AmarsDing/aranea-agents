@echo off
setlocal enabledelayedexpansion

REM ---- Auto-elevate to admin (required for PostgreSQL service in Program Files) ----
REM PostgreSQL initdb/pg_ctl need write access to install dir; when installed under
REM Program Files, NSIS admin-installed files are owned by Administrator, so a normal
REM user launch cannot create postgres/data. Force admin via UAC.
net session >nul 2>&1
if %errorlevel% neq 0 (
    echo Requesting administrator privileges...
    powershell -NoProfile -Command "Start-Process -FilePath '%~f0' -Verb RunAs -WorkingDirectory '%~dp0'"
    exit /b
)
cd /d "%~dp0"

title Aranea-Agents Launcher
echo ============================================
echo   Aranea-Agents One-Click Launcher
echo ============================================
echo.

REM ---- 0. Environment variables ----
set KRATOS_AUTH_SECRET=aranea-portable-dev-secret-32chars!!
set DEPLOY_ENV=dev
set DAO_VECTOR_PGVECTOR=1
set PGDATA=%~dp0postgres\data
set PGBIN=%~dp0postgres\bin
set LOGDIR=%~dp0logs

if not exist "%LOGDIR%" mkdir "%LOGDIR%"

REM ---- 1. Initialize PostgreSQL on first run ----
echo [1/5] Checking PostgreSQL...
if exist "%PGDATA%\PG_VERSION" goto pg_start

echo   First run: initializing database...
"%PGBIN%\initdb.exe" -D "%PGDATA%" -U postgres --auth=trust --encoding=UTF8 > "%LOGDIR%\initdb.log" 2>&1
if errorlevel 1 (
    echo   ERROR: initdb failed! Exit code: %errorlevel%
    echo   See %LOGDIR%\initdb.log for details.
    pause
    exit /b 1
)
echo   Database initialized.

:pg_start
echo   Starting PostgreSQL...
"%PGBIN%\pg_ctl.exe" start -D "%PGDATA%" -l "%LOGDIR%\postgres.log" -o "-p 5433" -w > "%LOGDIR%\pgctl.log" 2>&1
if errorlevel 1 (
    echo   PostgreSQL may already be running or failed to start, continuing...
) else (
    echo   PostgreSQL started.
)

echo   Waiting for PostgreSQL to be ready...
set /a ready_count=0
:pg_wait
"%PGBIN%\pg_isready.exe" -h 127.0.0.1 -p 5433 >nul 2>&1
if errorlevel 1 (
    set /a ready_count+=1
    if !ready_count! geq 10 (
        echo   WARNING: PostgreSQL not ready after 10s, continuing anyway...
        goto check_db
    )
    timeout /t 1 /nobreak >nul
    goto pg_wait
)
echo   PostgreSQL is ready.

:check_db
echo   Checking aranea database...
"%PGBIN%\psql.exe" -U postgres -h 127.0.0.1 -p 5433 -tc "SELECT 1 FROM pg_database WHERE datname = 'aranea'" > "%LOGDIR%\dbcheck.tmp" 2>&1
find "1" "%LOGDIR%\dbcheck.tmp" >nul 2>&1
if not errorlevel 1 goto db_ready

echo   Creating aranea database + pgvector extension...
"%PGBIN%\psql.exe" -U postgres -h 127.0.0.1 -p 5433 -c "CREATE DATABASE aranea" > "%LOGDIR%\dbsetup.log" 2>&1
"%PGBIN%\psql.exe" -U postgres -h 127.0.0.1 -p 5433 -d aranea -c "CREATE EXTENSION IF NOT EXISTS vector" >> "%LOGDIR%\dbsetup.log" 2>&1
echo   Database ready.
goto db_done

:db_ready
echo   aranea database already exists.

:db_done
if exist "%LOGDIR%\dbcheck.tmp" del "%LOGDIR%\dbcheck.tmp" >nul 2>&1

REM ---- 2. Start Redis ----
echo [2/5] Checking Redis...
tasklist /fi "imagename eq redis-server.exe" | find "redis-server.exe" >nul 2>&1
if not errorlevel 1 goto redis_ok

echo   Starting Redis...
start "Aranea Redis" /min "%~dp0redis\redis-server.exe" --port 6379 --bind 127.0.0.1
timeout /t 1 /nobreak >nul
echo   Redis started.
goto redis_done

:redis_ok
echo   Redis already running.

:redis_done

REM ---- 3. Start backend service ----
echo [3/5] Starting backend service...

REM Check if port 8000 is already responding (more reliable than process check)
powershell -Command "try { $r = Invoke-WebRequest -Uri 'http://127.0.0.1:8000/healthz' -UseBasicParsing -TimeoutSec 2; if ($r.StatusCode -eq 200) { exit 0 } else { exit 1 } } catch { exit 1 }" >nul 2>&1
if not errorlevel 1 (
    echo   Backend already running and healthy.
    goto backend_ready
)

REM Kill any stale backend process that might hold the port
taskkill /im aranea-server.exe /f >nul 2>&1
taskkill /fi "windowtitle eq Aranea Server*" /f >nul 2>&1
timeout /t 1 /nobreak >nul

echo   Launching backend...
start "Aranea Server" /min cmd /c "cd /d %~dp0 && aranea-server.exe -conf configs > logs\server.log 2>&1"
echo   Backend service starting...

REM ---- 4. Wait for backend readiness ----
echo [4/5] Waiting for backend to be ready...
set /a wait_count=0
:wait_backend
timeout /t 1 /nobreak >nul
set /a wait_count+=1
powershell -Command "try { $r = Invoke-WebRequest -Uri 'http://127.0.0.1:8000/healthz' -UseBasicParsing -TimeoutSec 2; if ($r.StatusCode -eq 200) { exit 0 } else { exit 1 } } catch { exit 1 }" >nul 2>&1
if not errorlevel 1 goto backend_ready
if %wait_count% geq 30 (
    echo   WARNING: backend not ready after 30s.
    echo   Check logs\server.log for details.
    echo   Attempting to start frontend anyway...
    goto start_frontend
)
echo   Waiting... (%wait_count%/30)
goto wait_backend

:backend_ready
echo   Backend is ready!

:start_frontend
REM ---- 5. Start frontend app ----
echo [5/5] Starting Aranea desktop app...
tasklist /fi "imagename eq AraneaAgents.exe" | find "AraneaAgents.exe" >nul 2>&1
if not errorlevel 1 goto app_running

start "" "%~dp0frontend\AraneaAgents.exe"
goto app_done

:app_running
echo   Desktop app already running.

:app_done
echo.
echo ============================================
echo   Aranea-Agents started successfully!
echo ============================================
echo   Backend API: http://127.0.0.1:8000
echo   Desktop app launched.
echo.
echo   Login: admin / changeme
echo   To stop: run stop.bat
echo.
pause
