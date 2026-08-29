#Requires -Version 5.1
<#
.SYNOPSIS
  Aranea 通用目标机离线包导出：docker save 镜像 + 部署 compose + config + install.sh。
.DESCRIPTION
  产物目录：build\release\aranea-release-<yyyyMMdd-HHmm>\（build/ 为安装介质目录约定，不入库）。
  包内布局镜像 compose 相对路径（docker-compose.yaml 在根，config 在 docker/ 下），
  目标机解压后 bash install.sh --server-ip <IP> 即可上线，无需路径改写。
  前置：Docker daemon 就绪；aranea-runtime:local 已构建（docker/build-runtime.ps1）；
        aranea-web:local 已构建（docker build -f web/Dockerfile -t aranea-web:local web）。
  幂等：可重跑，每次生成新时间戳目录。
.EXAMPLE
  powershell -ExecutionPolicy Bypass -File docker\export-release.ps1
#>
param(
  # 跳过 aranea-sandbox-base:local 导出（沙箱 codeexec profile 将不可用，引擎降级运行）
  [switch]$SkipSandboxBase
)
$ErrorActionPreference = 'Stop'
$repo      = Split-Path $PSScriptRoot -Parent
$stamp     = Get-Date -Format 'yyyyMMdd-HHmm'
$outDir    = Join-Path $repo "build\release\aranea-release-$stamp"
$null = New-Item -ItemType Directory -Force -Path $outDir

function Write-Step([string]$msg) { Write-Host "`n== $msg ==" -ForegroundColor Cyan }
# PS5.1 EAP=Stop 下原生命令 stderr 重定向会抛 NativeCommandError，统一临时降级
function Invoke-Native([scriptblock]$block) {
  $prev = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
  & $block | Out-Null
  $code = $LASTEXITCODE
  $ErrorActionPreference = $prev
  return $code
}

# ---------- 0. Docker 就绪自检 ----------
Write-Step '0. Docker daemon 自检'
$code = Invoke-Native { docker info 2>&1 }
if ($code -ne 0) { throw 'Docker daemon 未就绪，请先启动 Docker Desktop' }

# ---------- 1. 镜像清单 ----------
Write-Step '1. 导出镜像（docker save → images.tar）'
$selfImages = @('aranea-runtime:local', 'aranea-web:local')
if (-not $SkipSandboxBase) { $selfImages += 'aranea-sandbox-base:local' }
$baseImages = @(
  'pgvector/pgvector:pg18',
  'redis:7-alpine',
  'ubuntu/squid:latest',
  'python:3.11-slim'   # codeexecutor 沙箱镜像（CODE_EXECUTOR_DOCKER_IMAGE）
)
$images = @($selfImages) + @($baseImages)
foreach ($img in $selfImages) {
  $code = Invoke-Native { docker image inspect $img 2>&1 }
  if ($code -ne 0) {
    if ($img -eq 'aranea-sandbox-base:local') {
      Write-Host "  缺少 $img，跳过（沙箱 codeexec 将降级）" -ForegroundColor Yellow
      $images = $images | Where-Object { $_ -ne $img }
      continue
    }
    throw "镜像缺失: $img（runtime 用 docker/build-runtime.ps1；web 用 docker build -f web/Dockerfile -t aranea-web:local web）"
  }
}
foreach ($img in $baseImages) {
  $code = Invoke-Native { docker image inspect $img 2>&1 }
  if ($code -ne 0) {
    Write-Host "  本地缺失 $img，尝试拉取" -ForegroundColor Yellow
    $code = Invoke-Native { docker pull $img }
    if ($code -ne 0) { throw "镜像拉取失败: $img（离线环境请提前载入，或走 docs/打包 附录的 102 中转路径）" }
  }
}
$imagesTar = Join-Path $outDir 'images.tar'
Write-Host ("  共 {0} 个镜像，保存中（体积大，耗时较长）..." -f $images.Count)
$code = Invoke-Native { docker save -o $imagesTar $images }
if ($code -ne 0) { throw 'docker save 失败' }
Write-Host ("  images.tar: {0:N1} GB" -f ((Get-Item $imagesTar).Length / 1GB)) -ForegroundColor Green

# ---------- 2. 部署 compose（仓库版 + web 服务 + 参数化 CORS） ----------
Write-Step '2. 生成部署 compose（含 web 服务，CORS/地址由 install.sh --server-ip 注入）'
$pkgCompose = Join-Path $outDir 'docker-compose.yaml'
Copy-Item (Join-Path $repo 'docker-compose.yaml') $pkgCompose -Force
# k6 是压测 profile 且引用 test/ 目录，通用包剔除，避免目标机报路径缺失
$composeText = [IO.File]::ReadAllText($pkgCompose)
$composeText = $composeText -replace '(?ms)^  # G3 压测执行器.*?^networks:', 'networks:'
# CORS 注入目标机 IP（compose env 插值，install.sh 写 .env）
$composeText = $composeText -replace '(KRATOS_HTTP_EXTRA_CORS_ORIGINS:\s*")[^"]*"',
  '$1http://0.0.0.0:9301,http://localhost:9301,http://127.0.0.1:9301,http://${ARANEA_SERVER_IP:-127.0.0.1}:9301,http://${ARANEA_SERVER_IP:-127.0.0.1}:8810"'
# 追加 web 服务（nginx 静态托管 dist，runtime-config.json 由 install.sh 生成）
$webService = @'

  # Web 前端（nginx 静态托管 dist，9301→80；runtime-config.json 指向本机后端）
  web:
    image: aranea-web:local
    container_name: aranea-web
    restart: unless-stopped
    ports:
      - "9301:80"
    volumes:
      - ./web-config/runtime-config.json:/usr/share/nginx/html/assets/config/runtime-config.json:ro
    networks:
      - araneanet
'@
$composeText = $composeText -replace '(?m)^networks:', ($webService + "`n`nnetworks:")
[IO.File]::WriteAllText($pkgCompose, $composeText, (New-Object System.Text.UTF8Encoding $false))

# ---------- 3. 配置 / web-config 模板 / 卷目录占位 ----------
Write-Step '3. 配置拷贝与目录占位'
$pkgDocker = Join-Path $outDir 'docker'
Copy-Item (Join-Path $PSScriptRoot 'config') (Join-Path $pkgDocker 'config') -Recurse -Force
foreach ($d in 'logs','data','skills') { $null = New-Item -ItemType Directory -Force -Path (Join-Path $pkgDocker "volumes\$d") }
$null = New-Item -ItemType Directory -Force -Path (Join-Path $outDir 'web-config')
# install.sh 按 --server-ip 重写；此处给占位防 compose 挂载缺失
'{}' | Set-Content (Join-Path $outDir 'web-config\runtime-config.json') -Encoding utf8
Copy-Item (Join-Path $PSScriptRoot 'install.sh') $outDir -Force

# ---------- 4. 清单 ----------
Write-Step '4. 生成 MANIFEST'
$manifest = [ordered]@{
  exported_at = $stamp
  images      = $images
  notes       = '目标机执行 bash install.sh --server-ip <本机IP>；数据为零初始化（PG 空库，启动时自动迁移+seed）'
}
$manifest | ConvertTo-Json | Set-Content (Join-Path $outDir 'MANIFEST.json') -Encoding UTF8

$sizeMB = [math]::Round(((Get-ChildItem $outDir -Recurse -File | Measure-Object Length -Sum).Sum / 1MB), 0)
Write-Host ("`n导出完成: {0}（{1:N0} MB）" -f $outDir, $sizeMB) -ForegroundColor Green
Write-Host '目标机：解压后执行 bash install.sh --server-ip <目标机IP>' -ForegroundColor Green
