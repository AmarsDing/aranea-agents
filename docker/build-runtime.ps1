﻿#Requires -Version 5.1
<#
.SYNOPSIS
  本地交叉编译 Aranea admin（linux/amd64）并重建 aranea-runtime:local 薄镜像。
.DESCRIPTION
  流程：Docker daemon 预检 → GOOS=linux GOARCH=amd64 CGO_ENABLED=0 交叉编译
  （走本地 gocache，秒级）→ docker build 薄镜像（仅二进制层变化）。
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File build-runtime.ps1
#>
$ErrorActionPreference = 'Stop'
$repo = Split-Path $PSScriptRoot -Parent

# PS5.1 + EAP=Stop 下原生命令写 stderr 会抛 NativeCommandError；
# 统一经本函数包装：stderr 合并为文本输出，仅按 LASTEXITCODE 判定成败。
# 注意必须是简单函数（无 [Parameter()] 高级参数），否则 go build -o 等实参
# 会被绑定到公共参数（-OutVariable/-OutBuffer）而报 AmbiguousParameter。
function Invoke-Native {
  $exe = $args[0]
  $rest = @()
  if ($args.Count -gt 1) { $rest = $args[1..($args.Count - 1)] }
  # EAP=Stop 会让任何 stderr 行（go 的 '# pkg' 头、buildkit 进度）抛
  # NativeCommandError；函数作用域内降为 Continue，仅按退出码判成败。
  # 输出必须走 Write-Host：直接 "$_" 会把输出行混进返回值（调用方 $rc 收到
  # Object[] 导致 -ne 0 恒真误抛，2026-08-13 实证）。
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

# ---- Docker daemon 预检：未启动则拉起 Docker Desktop 并等待就绪 ----
if (-not (Test-DockerDaemon)) {
  Write-Host 'Docker daemon 未运行，启动 Docker Desktop...' -ForegroundColor Yellow
  # WMI 派生而非 Start-Process：彻底脱离调用方进程树，防止终端/脚本退出时
  # Docker Desktop 作为子进程被一并回收（2026-08-13 实证：Start-Process 拉起后
  # 终端回收导致引擎反复 flap）。
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

# ---- 交叉编译 ----
Push-Location $repo
try {
  $version = (git describe --tags --always 2>$null); if (-not $version) { $version = 'docker' }
  $commit  = (git rev-parse HEAD 2>$null);           if (-not $commit)  { $commit  = 'unknown' }
  $buildDate = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
  $ldflags = "-X main.Version=$version -X main.Commit=$commit -X main.BuildDate=$buildDate"

  New-Item -ItemType Directory -Force (Join-Path $repo 'bin\linux') | Out-Null
  $env:GOOS = 'linux'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
  Write-Host '交叉编译 admin (linux/amd64)...' -ForegroundColor Cyan
  # -tags pgvector：启用 PG 向量召回（2026-08-17 评测实证缺失导致记忆全量降级
  # ErrMemoryUnavailable；根 Dockerfile 注释本就声明 pgvector tag 为预期能力）
  $rc = Invoke-Native go build -tags pgvector -ldflags $ldflags -o .\bin\linux\ .\cmd\admin
  if ($rc -ne 0) { throw "go build 失败（$rc）" }
} finally {
  Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
  Pop-Location
}

# ---- 重建镜像 ----
Write-Host '构建镜像 aranea-runtime:local...' -ForegroundColor Cyan
$rc = Invoke-Native docker build -f (Join-Path $PSScriptRoot 'Dockerfile.runtime') -t aranea-runtime:local $repo
if ($rc -ne 0) { throw "docker build 失败（$rc）" }
Write-Host '完成：aranea-runtime:local' -ForegroundColor Green
