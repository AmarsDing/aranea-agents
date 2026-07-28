$ErrorActionPreference = "Stop"
$pgDir = "f:\aranea-agents\AraneaAgents-deploy\postgres"
$pgBin = Join-Path $pgDir "bin"
$pgData = Join-Path $pgDir "data"
$logDir = "f:\aranea-agents\logs"
if (-not (Test-Path $logDir)) { New-Item -ItemType Directory -Force -Path $logDir | Out-Null }

Write-Host "=== Step 1: Check if PostgreSQL is running on 5433 ==="
& "$pgBin\pg_isready.exe" -U postgres -h 127.0.0.1 -p 5433 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "=== Step 2: Start PostgreSQL ==="
    & "$pgBin\pg_ctl.exe" start -D $pgData -l "$logDir\verify-pg.log" -o "-p 5433" -w 2>&1
    if ($LASTEXITCODE -ne 0) { Write-Host "pg_ctl start failed"; exit 1 }
}

Write-Host "=== Step 3: Drop and recreate test database (no vector extension) ==="
& "$pgBin\psql.exe" -U postgres -h 127.0.0.1 -p 5433 -c "DROP DATABASE IF EXISTS aranea_novector_test" 2>&1
& "$pgBin\psql.exe" -U postgres -h 127.0.0.1 -p 5433 -c "CREATE DATABASE aranea_novector_test" 2>&1

Write-Host "=== Step 4: Verify vector extension is NOT installed in test db ==="
& "$pgBin\psql.exe" -U postgres -h 127.0.0.1 -p 5433 -d aranea_novector_test -c "SELECT extname FROM pg_extension WHERE extname='vector'" 2>&1

Write-Host "=== Step 5: Create temp config ==="
$tempConfDir = "$env:TEMP\aranea-verify-conf"
if (Test-Path $tempConfDir) { Remove-Item $tempConfDir -Recurse -Force }
New-Item -ItemType Directory -Force -Path $tempConfDir | Out-Null
$config = @"
server:
  http:
    addr: 127.0.0.1:8001
    timeout: 0s
  grpc:
    addr: 127.0.0.1:9001
    timeout: 120s
  ws:
    enable: true
    network: tcp
    addr: 127.0.0.1:8003
  monitor:
    process_log_enabled: true
data:
  driver: postgres
  sqlite:
    enable: false
    source: file:./data/arenea.sqlite?cache=shared&_fk=1
  initial_admin:
    name: admin
    email: admin@local.invalid
    password: changeme
    access: admin
  postgres:
    source: "postgres://postgres@127.0.0.1:5433/aranea_novector_test?sslmode=disable"
    vector_dim: 1536
  redis:
    addr: 127.0.0.1:6379
    read_timeout: 0.2s
    write_timeout: 0.2s
logging:
  level: info
  output_dir: "./logs"
  max_size_mb: 100
  max_backups: 10
  max_age_days: 30
  compress: true
  stdout_enabled: true
  hook_level: info
"@
Set-Content -Path "$tempConfDir\config.yaml" -Value $config -Encoding UTF8

Write-Host "=== Step 6: Build aranea-server with the fix ==="
Push-Location f:\aranea-agents
& go build -tags pgvector -o "$env:TEMP\aranea-verify.exe" ./cmd/admin 2>&1
if ($LASTEXITCODE -ne 0) { Pop-Location; Write-Host "Build failed"; exit 1 }
Pop-Location

Write-Host "=== Step 7: Start aranea-server (should NOT panic) ==="
$env:KRATOS_AUTH_SECRET = "aranea-portable-dev-secret-32chars!!"
$env:DEPLOY_ENV = "dev"
$env:DAO_VECTOR_PGVECTOR = "1"
$proc = Start-Process -FilePath "$env:TEMP\aranea-verify.exe" `
    -ArgumentList "-conf", $tempConfDir `
    -RedirectStandardOutput "$logDir\verify-server-stdout.log" `
    -RedirectStandardError "$logDir\verify-server-stderr.log" `
    -PassThru -NoNewWindow

Write-Host "Server PID: $($proc.Id), waiting up to 15s for startup..."
$startOk = $false
for ($i = 1; $i -le 15; $i++) {
    Start-Sleep -Seconds 1
    if ($proc.HasExited) {
        Write-Host "Server exited prematurely at $i s with code $($proc.ExitCode)"
        break
    }
    try {
        $r = Invoke-WebRequest -Uri "http://127.0.0.1:8001/healthz" -UseBasicParsing -TimeoutSec 2
        if ($r.StatusCode -eq 200) {
            Write-Host "Server is HEALTHY at $i s (HTTP 200)"
            $startOk = $true
            break
        }
    } catch {
        Write-Host "  $i s: not ready yet"
    }
}

Write-Host "=== Step 8: Stop server ==="
if (-not $proc.HasExited) {
    Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue
    Start-Sleep -Seconds 1
}

Write-Host "=== Step 9: Check logs for panic / pgvector warning ==="
$stderr = Get-Content "$logDir\verify-server-stderr.log" -Raw -ErrorAction SilentlyContinue
$stdout = Get-Content "$logDir\verify-server-stdout.log" -Raw -ErrorAction SilentlyContinue
$combined = "$stdout`n$stderr"

if ($combined -match "panic:") {
    Write-Host "FAIL: panic detected in logs" -ForegroundColor Red
    $combined -split "`n" | Where-Object { $_ -match "panic:" -or $_ -match "pgvector" } | Select-Object -First 10
    $result = "FAIL"
} elseif ($combined -match "pgvector unavailable") {
    Write-Host "PASS: pgvector unavailable warning detected (degradation path triggered)" -ForegroundColor Green
    $result = "PASS"
} else {
    Write-Host "WARNING: no panic and no pgvector warning — check logs manually" -ForegroundColor Yellow
    $result = "UNKNOWN"
}

Write-Host ""
Write-Host "=== Result: $result ==="

Write-Host "=== Step 10: Cleanup ==="
& "$pgBin\psql.exe" -U postgres -h 127.0.0.1 -p 5433 -c "DROP DATABASE IF EXISTS aranea_novector_test" 2>&1
Remove-Item "$env:TEMP\aranea-verify.exe" -Force -ErrorAction SilentlyContinue
Remove-Item $tempConfDir -Recurse -Force -ErrorAction SilentlyContinue

Write-Host "=== Done ==="
