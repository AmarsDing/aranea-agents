#Requires -Version 5.1
<#
.SYNOPSIS
  Aranea 102 远程发布：本机构建镜像 → docker save → SMB 传输 → 102 docker load
  → compose up -d → 健康验证。
.DESCRIPTION
  运行环境唯一 = 192.168.0.102（WSL Ubuntu-22.04 Docker CE，/mnt/f/deploy-102/aranea）。
  全量镜像 save/load 单一路径（2026-08-28 裁定，无二进制热修补丁路径）；
  staging 双端即用即清；不覆盖 102 的 compose/config（102 侧为唯一真理）。
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File docker/release-102.ps1 -Target admin
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File docker/release-102.ps1            # admin+web 全量
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File docker/release-102.ps1 -SkipBuild # 复用本机已有镜像
#>
[CmdletBinding()]
param(
  [ValidateSet('all','admin','web')]
  [string]$Target = 'all',
  [switch]$SkipBuild,
  [string]$RemoteHost = '192.168.0.102',
  [string]$RemoteUser = 'ding',
  [string]$RemotePass = '123'
)
$ErrorActionPreference = 'Stop'
$repo = Split-Path $PSScriptRoot -Parent

# PS5.1 + EAP=Stop 下原生命令写 stderr 会抛 NativeCommandError；
# 统一包装：stderr 合并为文本输出，仅按 LASTEXITCODE 判定成败。
function Invoke-Native {
  $exe = $args[0]
  $rest = @()
  if ($args.Count -gt 1) { $rest = $args[1..($args.Count - 1)] }
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

# ---- 远程执行：WinRM → 102 Windows → WSL Ubuntu-22.04 ----
$secPass = ConvertTo-SecureString $RemotePass -AsPlainText -Force
$remoteCred = New-Object PSCredential($RemoteUser, $secPass)
function Invoke-102([string]$bashCmd) {
  return Invoke-Command -ComputerName $RemoteHost -Credential $remoteCred `
    -ScriptBlock { param($c) wsl -d Ubuntu-22.04 -- bash -c $c 2>&1 } -ArgumentList $bashCmd
}

function Mount-102Share {
  $rc = Invoke-Native net use "\\$RemoteHost\deploy102" "/user:$RemoteUser" $RemotePass
  if ($rc -ne 0) { throw "net use \\\\$RemoteHost\deploy102 失败（$rc）" }
}
function Dismount-102Share {
  # 未连接时 /delete 会报 2250 噪音，直接吞掉（幂等）
  $prev = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
  try { net use "\\$RemoteHost\deploy102" /delete 2>&1 | Out-Null } finally { $ErrorActionPreference = $prev }
}

# ---- 预检：本机 daemon ----
if (-not (Test-DockerDaemon)) {
  Write-Host 'Docker daemon 未运行，启动 Docker Desktop...' -ForegroundColor Yellow
  # WMI 派生而非 Start-Process：脱离调用方进程树，防终端退出连杀
  $null = ([wmiclass]'Win32_Process').Create('"' + (Find-DockerDesktop) + '"')
  $ready = $false
  $deadline = (Get-Date).AddSeconds(180)
  do {
    Start-Sleep -Seconds 3
    if (Test-DockerDaemon) { $ready = $true; break }
  } while ((Get-Date) -lt $deadline)
  if (-not $ready) { throw 'Docker daemon 启动超时（180s）' }
  Write-Host 'Docker daemon 就绪' -ForegroundColor Green
}

# ---- 预检：102 WinRM + WSL + docker + 部署目录 ----
Write-Host "预检 102（$RemoteHost）WinRM/WSL/docker..." -ForegroundColor Cyan
$precheck = Invoke-102 "docker info --format '{{.ServerVersion}}' && test -d /mnt/f/deploy-102/aranea && echo ARANEA_DIR_OK"
if (-not ($precheck | Select-String 'ARANEA_DIR_OK')) { throw "102 预检失败：$precheck" }
Write-Host "102 docker 就绪（$(($precheck | Select-Object -First 1) -replace '\s',''))" -ForegroundColor Green

# ---- 构建 ----
$images = @()
if ($Target -in 'all','admin') { $images += 'aranea-runtime:local' }
if ($Target -in 'all','web')   { $images += 'aranea-web:local' }

if (-not $SkipBuild) {
  if ($Target -in 'all','admin') {
    Write-Host '构建 admin（交叉编译 + 薄镜像）...' -ForegroundColor Cyan
    & (Join-Path $PSScriptRoot 'build-runtime.ps1')
    if ($LASTEXITCODE -ne 0) { throw "build-runtime.ps1 失败（$LASTEXITCODE）" }
  }
  if ($Target -in 'all','web') {
    Write-Host '构建 web（aranea-web:local）...' -ForegroundColor Cyan
    $rc = Invoke-Native docker build -f (Join-Path $repo 'web\Dockerfile') -t aranea-web:local (Join-Path $repo 'web')
    if ($rc -ne 0) { throw "web docker build 失败（$rc）" }
  }
} else {
  Write-Host 'SkipBuild：复用本机已有镜像' -ForegroundColor Yellow
}

# ---- 导出镜像 ----
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$tarName = "aranea-release-$stamp.tar"
$localStage = Join-Path $env:TEMP "aranea-release-102\$stamp"
New-Item -ItemType Directory -Force $localStage | Out-Null
$localTar = Join-Path $localStage $tarName
$remoteStageUnc = "\\$RemoteHost\deploy102\aranea\staging"
$remoteStageWsl = '/mnt/f/deploy-102/aranea/staging'

try {
  Write-Host "导出镜像（$($images -join ' ')）..." -ForegroundColor Cyan
  $rc = Invoke-Native docker save -o $localTar @images
  if ($rc -ne 0) { throw "docker save 失败（$rc）" }
  $sizeMB = [math]::Round((Get-Item $localTar).Length / 1MB)
  Write-Host "导出完成（${sizeMB}MB）" -ForegroundColor Green

  # ---- SMB 传输到 102 staging ----
  Write-Host "传输 → $remoteStageUnc ..." -ForegroundColor Cyan
  Mount-102Share
  try {
    New-Item -ItemType Directory -Force $remoteStageUnc | Out-Null
    Copy-Item $localTar (Join-Path $remoteStageUnc $tarName) -Force
  } finally {
    Dismount-102Share
  }
  Write-Host '传输完成' -ForegroundColor Green

  # ---- 远端加载 + 重建容器 ----
  # compose up -d 仅重建镜像 ID 变化的容器；postgres/redis/egress-proxy 不动，数据零影响
  Write-Host '102 加载镜像并 compose up -d...' -ForegroundColor Cyan
  $bash = @(
    'set -e',
    "docker load -i '$remoteStageWsl/$tarName'",
    'cd /mnt/f/deploy-102/aranea',
    'docker compose up -d',
    "rm -f '$remoteStageWsl/$tarName'",
    "echo '--- docker ps ---'",
    "docker ps --filter name=aranea- --format '{{.Names}}  {{.Status}}'"
  ) -join ' && '
  $remote = Invoke-102 $bash
  $remote | ForEach-Object { Write-Host "$_" }
  if (-not ($remote | Select-String 'aranea-admin')) { throw "远端执行异常：$remote" }

  # ---- 验证 ----
  if ($Target -in 'all','admin') {
    Write-Host '验证 admin healthz...' -ForegroundColor Cyan
    $ok = $false
    for ($i = 0; $i -lt 10; $i++) {
      try {
        $resp = Invoke-WebRequest -UseBasicParsing -TimeoutSec 5 "http://${RemoteHost}:8810/healthz"
        if ($resp.StatusCode -eq 200) { $ok = $true; break }
      } catch { Start-Sleep -Seconds 3 }
    }
    if (-not $ok) { throw "admin healthz 验证失败：http://${RemoteHost}:8810/healthz" }
    Write-Host 'admin healthz OK' -ForegroundColor Green
  }
  if ($Target -in 'all','web') {
    Write-Host '验证 web 9301...' -ForegroundColor Cyan
    $ok = $false
    for ($i = 0; $i -lt 6; $i++) {
      try {
        $resp = Invoke-WebRequest -UseBasicParsing -TimeoutSec 5 "http://${RemoteHost}:9301/"
        if ($resp.StatusCode -eq 200) { $ok = $true; break }
      } catch { Start-Sleep -Seconds 3 }
    }
    if (-not $ok) { throw "web 验证失败：http://${RemoteHost}:9301/" }
    Write-Host 'web 9301 OK' -ForegroundColor Green
  }

  Write-Host "`n发布完成：$($images -join ' ') → $RemoteHost" -ForegroundColor Green
} finally {
  # 本机 staging 即用即清（102 侧 staging 由远端 bash 自删）
  Remove-Item $localStage -Recurse -Force -ErrorAction SilentlyContinue
  Dismount-102Share
}
