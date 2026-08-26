#Requires -Version 5.1
<#
.SYNOPSIS
  构建 aranea-sandbox-base 会话沙箱基座镜像（M82 P1-3）。
.DESCRIPTION
  流程：Docker daemon 预检 → docker build（docker/sandbox-base/Dockerfile）。
  沙箱 codeexec profile 默认镜像（config sandbox.profiles.codeexec.image）
  指向 aranea-sandbox-base:local；镜像缺失时热池补水失败并走 P0-8 降级告警链。
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File build-sandbox-base.ps1
  powershell -ExecutionPolicy Bypass -File build-sandbox-base.ps1 -Tag v1
#>
param(
  [string]$Tag = 'local',
  # CN 网络默认走阿里云镜像（deb.debian.org / pypi.org 在本机 502/403 实测）；
  # 传 '' 显式回退上游默认源。
  [string]$AptMirror = 'https://mirrors.aliyun.com',
  [string]$PipIndexUrl = 'https://mirrors.aliyun.com/pypi/simple/'
)
$ErrorActionPreference = 'Stop'

# PS5.1 + EAP=Stop 下原生命令写 stderr 会抛 NativeCommandError；
# 统一经本函数包装：stderr 合并为文本输出，仅按 LASTEXITCODE 判定成败。
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

$image = "aranea-sandbox-base:$Tag"
Write-Host "构建镜像 $image (APT_MIRROR=$AptMirror, PIP_INDEX_URL=$PipIndexUrl) ..." -ForegroundColor Cyan
$rc = Invoke-Native docker build -t $image --build-arg "APT_MIRROR=$AptMirror" --build-arg "PIP_INDEX_URL=$PipIndexUrl" $PSScriptRoot
if ($rc -ne 0) { throw "docker build 失败（$rc）" }
Write-Host "完成：$image" -ForegroundColor Green
