#Requires -Version 5.1
<#
.SYNOPSIS
  Aranea Docker 日常开发验证一键脚本：编译 → 镜像 → 替换容器 → 健康检查 → 登录冒烟。
.DESCRIPTION
  流程：Docker daemon 预检 → 端口冲突预检（8810/9910/8812）→ gen-config 重生成 overlay
  → build-runtime（可 -SkipBuild）→ compose up -d → 等待 /healthz ready → 登录冒烟。
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File dev-up.ps1
  powershell -ExecutionPolicy Bypass -File dev-up.ps1 -SkipBuild   # 仅换配置/重启
#>
param(
  [switch]$SkipBuild,
  [string]$SmokeUser = 'admin',
  [string]$SmokePassword = ''   # 空 = 跳过登录冒烟（仅健康检查）
)
$ErrorActionPreference = 'Stop'
$repo = Split-Path $PSScriptRoot -Parent

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
function Test-DockerDaemon { cmd /c 'docker info >nul 2>&1'; return ($LASTEXITCODE -eq 0) }
function Find-DockerDesktop {
  $dd = "$env:ProgramFiles\Docker\Docker\Docker Desktop.exe"
  if (Test-Path $dd) { return $dd }
  $reg = Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\*' -ErrorAction SilentlyContinue |
         Where-Object { $_.DisplayName -eq 'Docker Desktop' -and $_.InstallLocation } | Select-Object -First 1
  if ($reg) { $dd = Join-Path $reg.InstallLocation 'Docker Desktop.exe'; if (Test-Path $dd) { return $dd } }
  throw '未找到 Docker Desktop（已查默认路径与注册表 InstallLocation）'
}

# ---- Docker daemon 预检 ----
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

# ---- 端口冲突预检（报具体 PID；Docker 端口映射代理占用属重跑场景，放行）----
foreach ($p in 8810, 9910, 8812) {
  $c = Get-NetTCPConnection -LocalPort $p -State Listen -ErrorAction SilentlyContinue |
       Where-Object { $_.OwningProcess -ne 0 } | Select-Object -First 1
  if ($c) {
    $proc = Get-Process -Id $c.OwningProcess -ErrorAction SilentlyContinue
    if ($proc.ProcessName -match 'com\.docker|vpnkit|wslrelay') { continue }   # Docker 端口映射代理（wslrelay = WSL2 后端的 localhost 转发）
    throw "端口 $p 被占用：$($proc.ProcessName)(PID $($c.OwningProcess))。若是本地 aranea-server.exe，请先停止（切换）"
  }
}

# ---- overlay 配置（幂等重生成）----
& (Join-Path $PSScriptRoot 'gen-config.ps1')
if ($LASTEXITCODE -ne 0) { throw 'gen-config 失败' }

# ---- 构建 ----
if (-not $SkipBuild) {
  & (Join-Path $PSScriptRoot 'build-runtime.ps1')
  if ($LASTEXITCODE -ne 0) { throw 'build-runtime 失败' }
}

# ---- compose up ----
$rc = Invoke-Native docker compose -f (Join-Path $repo 'docker-compose.yaml') up -d
if ($rc -ne 0) { throw "compose up 失败（$rc）" }

# ---- 等待 readiness（启动迁移在后台跑，/healthz 未 ready 返回 503）----
Write-Host '等待 /healthz ready（含启动迁移，最长 180s）...' -ForegroundColor Cyan
$deadline = (Get-Date).AddSeconds(180)
$ready = $false
do {
  Start-Sleep -Seconds 3
  try {
    $resp = Invoke-WebRequest -Uri 'http://127.0.0.1:8810/healthz' -UseBasicParsing -TimeoutSec 5
    if ($resp.StatusCode -eq 200) { $ready = $true }
  } catch { }
} until ($ready -or (Get-Date) -gt $deadline)
if (-not $ready) {
  Invoke-Native docker logs aranea-admin --tail 50
  throw '/healthz 180s 未 ready，上方为容器日志尾部'
}
Write-Host '/healthz ready' -ForegroundColor Green

# ---- 登录冒烟 ----
if ($SmokePassword) {
  $body = @{ username = $SmokeUser; password = $SmokePassword } | ConvertTo-Json
  try {
    $login = Invoke-RestMethod -Uri 'http://127.0.0.1:8810/v1/admins/login' -Method Post -Body $body -ContentType 'application/json' -TimeoutSec 10
    Write-Host '登录冒烟 OK' -ForegroundColor Green
  } catch {
    throw "登录冒烟失败：$($_.Exception.Message)"
  }
} else {
  Write-Host '（未提供 -SmokePassword，跳过登录冒烟）' -ForegroundColor Yellow
}
Write-Host 'dev-up 完成。容器：aranea-postgres / aranea-redis / aranea-admin' -ForegroundColor Green
