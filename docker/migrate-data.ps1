#Requires -Version 5.1
<#
.SYNOPSIS
  将本地原生 PG 的 aranea 库迁移到 Aranea compose 的 postgres 容器。
.DESCRIPTION
  前置：本地 PG（app\env\postgresql，127.0.0.1:5432）在运行（数据源），Docker daemon 可用。
  步骤（任一步失败即中止，本地环境未动可直接回退，脚本幂等可重跑）：
    1) 本地 pg_dump -Fc aranea 库 → docker\volumes\migrate\aranea.dump
    2) compose up -d postgres（等待 healthy）→ 容器内 pg_restore（--clean --if-exists）
    3) 校验：public 全表行数本地 vs 容器逐表对比
  Redis 不迁移（缓存/队列类数据，重建即可，与 TwinMonitor 设计 §4.5 口径一致）。
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File migrate-data.ps1
#>
$ErrorActionPreference = 'Stop'
$repo = Split-Path $PSScriptRoot -Parent
$pgBin = 'f:\myproject\app\env\postgresql\bin'
$migDir = Join-Path $PSScriptRoot 'volumes\migrate'
$dumpFile = Join-Path $migDir 'aranea.dump'
$env:PGPASSWORD = '123456'

# PS5.1 + EAP=Stop 下原生命令写 stderr 会抛 NativeCommandError；
# 统一经本函数包装：stderr 合并为文本输出，仅按 LASTEXITCODE 判定成败。
# 注意必须是简单函数（无 [Parameter()] 高级参数），否则实参中的短横线开关
# 会被绑定到公共参数（-OutVariable/-OutBuffer）而报 AmbiguousParameter。
function Invoke-Native {
  $exe = $args[0]
  $rest = @()
  if ($args.Count -gt 1) { $rest = $args[1..($args.Count - 1)] }
  # EAP=Stop 下任何 stderr 行都会抛 NativeCommandError；降为 Continue，按退出码判成败。
  # 输出走 Write-Host：直接 "$_" 会把输出行混进返回值（$rc 变 Object[] 误抛，2026-08-13 实证）。
  $prev = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  try {
    & $exe @rest 2>&1 | ForEach-Object { Write-Host "$_" }
    return $LASTEXITCODE
  } finally {
    $ErrorActionPreference = $prev
  }
}
# 捕获原生命令 stdout 用于取值比较（与 Invoke-Native 分离，避免输出行污染退出码）；
# 失败（非零退出）返回 $null。
function Get-NativeOutput {
  $exe = $args[0]
  $rest = @()
  if ($args.Count -gt 1) { $rest = $args[1..($args.Count - 1)] }
  $prev = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  try {
    $out = & $exe @rest 2>$null
    if ($LASTEXITCODE -ne 0) { return $null }
    return $out
  } finally {
    $ErrorActionPreference = $prev
  }
}
function Test-DockerDaemon { cmd /c 'docker info >nul 2>&1'; return ($LASTEXITCODE -eq 0) }
function Find-DockerDesktop {
  $dd = "$env:ProgramFiles\Docker\Docker\Docker Desktop.exe"
  if (Test-Path $dd) { return $dd }
  $reg = Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*' -ErrorAction SilentlyContinue |
         Where-Object { $_.DisplayName -eq 'Docker Desktop' -and $_.InstallLocation } | Select-Object -First 1
  if ($reg) { $dd = Join-Path $reg.InstallLocation 'Docker Desktop.exe'; if (Test-Path $dd) { return $dd } }
  throw '未找到 Docker Desktop（已查默认路径与注册表 InstallLocation）'
}

# ---- 0. Docker daemon 预检 ----
if (-not (Test-DockerDaemon)) {
  Write-Host 'Docker daemon 未运行，启动 Docker Desktop...' -ForegroundColor Yellow
  # WMI 派生而非 Start-Process：彻底脱离调用方进程树，防止终端/脚本退出时
  # Docker Desktop 作为子进程被一并回收（2026-08-13 实证）。
  $null = ([wmiclass]'Win32_Process').Create('"' + (Find-DockerDesktop) + '"')
  $ready = $false
  $deadline = (Get-Date).AddSeconds(180)
  do { Start-Sleep -Seconds 3; if (Test-DockerDaemon) { $ready = $true; break } } while ((Get-Date) -lt $deadline)
  if (-not $ready) { throw 'Docker daemon 启动超时（180s）' }
}

# ---- 1. 数据源预检 + pg_dump ----
$rc = Invoke-Native "$pgBin\psql.exe" -h 127.0.0.1 -U postgres -d aranea -tAc 'SELECT 1'
if ($rc -ne 0) { throw '本地 PG（127.0.0.1:5432 aranea 库）不可达，请先启动数据源' }
New-Item -ItemType Directory -Force $migDir | Out-Null
Write-Host '1/3 本地 pg_dump aranea 库...' -ForegroundColor Cyan
$rc = Invoke-Native "$pgBin\pg_dump.exe" -h 127.0.0.1 -U postgres -d aranea -Fc -f $dumpFile
if ($rc -ne 0) { throw "pg_dump 失败（$rc）" }

# ---- 2. 目标容器就绪 + restore ----
Write-Host '2/3 启动 compose postgres 并恢复...' -ForegroundColor Cyan
$rc = Invoke-Native docker compose -f (Join-Path $repo 'docker-compose.yaml') up -d postgres
if ($rc -ne 0) { throw "compose up postgres 失败（$rc）" }
$st = ''
$deadline = (Get-Date).AddSeconds(120)
do {
  Start-Sleep -Seconds 3
  $st = (Get-NativeOutput docker inspect -f '{{.State.Health.Status}}' aranea-postgres | Select-Object -First 1)
} until ($st -eq 'healthy' -or (Get-Date) -gt $deadline)
if ($st -ne 'healthy') { throw "aranea-postgres 未就绪（状态: $st）" }

$rc = Invoke-Native docker cp $dumpFile aranea-postgres:/tmp/aranea.dump
if ($rc -ne 0) { throw "docker cp 失败（$rc）" }
# pg_restore 对无害告警（如 public schema 注释差异）也可能返回非零，成败以第 3 步校验为准
Invoke-Native docker exec aranea-postgres pg_restore -U postgres -d aranea --clean --if-exists --no-owner --no-privileges /tmp/aranea.dump | Out-Null

# ---- 3. 校验：public 全表行数对比 ----
Write-Host '3/3 校验：逐表行数对比...' -ForegroundColor Cyan
$tables = & "$pgBin\psql.exe" -h 127.0.0.1 -U postgres -d aranea -tAc "SELECT tablename FROM pg_tables WHERE schemaname='public' ORDER BY 1" 2>$null
if ($LASTEXITCODE -ne 0 -or -not $tables) { throw '读取源表清单失败' }
if ($tables -is [string]) { $tables = @($tables) }
$mismatch = 0
foreach ($tb in $tables) {
  $src = (Get-NativeOutput "$pgBin\psql.exe" -h 127.0.0.1 -U postgres -d aranea -tAc "SELECT count(*) FROM $tb" | Select-Object -First 1).Trim()
  $dst = (Get-NativeOutput docker exec aranea-postgres psql -U postgres -d aranea -tAc "SELECT count(*) FROM $tb" | Select-Object -First 1).Trim()
  if ($src -ne $dst) { Write-Host ("  [MISMATCH] {0}: local={1} container={2}" -f $tb, $src, $dst) -ForegroundColor Red; $mismatch++ }
}
if ($mismatch -gt 0) { throw "$mismatch 张表行数不一致，请检查 pg_restore 输出" }
Write-Host ("校验通过：{0} 张表行数全部一致" -f $tables.Count) -ForegroundColor Green

# pgvector 扩展确认
$ext = (Get-NativeOutput docker exec aranea-postgres psql -U postgres -d aranea -tAc "SELECT extname FROM pg_extension WHERE extname='vector'" | Select-Object -First 1).Trim()
if ($ext -ne 'vector') { Write-Host '警告: vector 扩展未建，admin 首次启动迁移时应自动创建；若未创建请手动 CREATE EXTENSION vector' -ForegroundColor Yellow }

Write-Host ''
Write-Host '迁移完成。尾注（切换后必做）：DB 系统设置中的 localhost 出站地址' -ForegroundColor Cyan
Write-Host '（Ollama embedding base_url、MCP/channel 回调等）需改指 host.docker.internal，'
Write-Host 'TwinMonitor 侧注册到 Aranea 的 webhook 回调地址同理。'
