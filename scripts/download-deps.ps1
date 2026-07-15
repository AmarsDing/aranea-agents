<#
.SYNOPSIS
    下载 Windows 打包依赖（PostgreSQL 便携版 + Redis + pgvector）。

.DESCRIPTION
    支持三种模式（按优先级）：
      1. -LocalDepsDir：从本地目录复制（开发用，最快）
      2. -FromRelease：从 GitHub Release asset 下载（CI 推荐）
      3. 默认：从官方 URL 下载 PostgreSQL + Redis

.PARAMETER OutDir
    依赖输出目录（默认：build/deps/）

.PARAMETER LocalDepsDir
    本地依赖目录（应包含 postgres/ 和 redis/ 子目录）

.PARAMETER FromRelease
    从 GitHub Release 下载预打包的依赖 zip

.PARAMETER ReleaseTag
    下载依赖的 Release tag（默认：deps-v1）

.EXAMPLE
    # 从本地已有 AraneaAgents-deploy 复制
    .\scripts\download-deps.ps1 -LocalDepsDir .\AraneaAgents-deploy

.EXAMPLE
    # 从官方 URL 下载（不含 pgvector，需手动添加）
    .\scripts\download-deps.ps1

.EXAMPLE
    # 从 GitHub Release 下载预打包依赖（含 pgvector）
    .\scripts\download-deps.ps1 -FromRelease -ReleaseTag deps-v1
#>
param(
    [string]$OutDir = "build/deps",
    [string]$LocalDepsDir,
    [switch]$FromRelease,
    [string]$ReleaseTag = "deps-v1"
)

$ErrorActionPreference = "Stop"
$RepoRoot = Resolve-Path "$PSScriptRoot/.."
$OutDirFull = if ([System.IO.Path]::IsPathRooted($OutDir)) { $OutDir } else { Join-Path $RepoRoot $OutDir }

# ─── 模式 1：从本地目录复制 ───────────────────────────────────
if ($LocalDepsDir) {
    $srcPostgres = Join-Path $LocalDepsDir "postgres"
    $srcRedis = Join-Path $LocalDepsDir "redis"

    if (-not (Test-Path $srcPostgres)) {
        Write-Error "本地依赖目录缺少 postgres/ 子目录：$srcPostgres"
    }

    Write-Host "[download-deps] 从本地复制依赖：$LocalDepsDir → $OutDirFull" -ForegroundColor Cyan
    New-Item -ItemType Directory -Force -Path $OutDirFull | Out-Null

    # 复制 postgres（排除 data/ 目录，避免复制数据库数据）
    $destPostgres = Join-Path $OutDirFull "postgres"
    if (Test-Path $destPostgres) { Remove-Item $destPostgres -Recurse -Force }
    Copy-Item $srcPostgres $destPostgres -Recurse
    # 删除 data/ 目录（用户首次启动时自动初始化）
    $dataDir = Join-Path $destPostgres "data"
    if (Test-Path $dataDir) { Remove-Item $dataDir -Recurse -Force }
    Write-Host "  [OK] postgres/ copied" -ForegroundColor Green

    # 复制 redis
    if (Test-Path $srcRedis) {
        $destRedis = Join-Path $OutDirFull "redis"
        if (Test-Path $destRedis) { Remove-Item $destRedis -Recurse -Force }
        Copy-Item $srcRedis $destRedis -Recurse
        Write-Host "  [OK] redis/ copied" -ForegroundColor Green
    } else {
        Write-Host "  [WARN] redis/ not found in source, will download separately" -ForegroundColor Yellow
    }

    Write-Host "[download-deps] 本地依赖复制完成" -ForegroundColor Green
    return
}

# ─── 模式 2：从 GitHub Release 下载 ────────────────────────────
if ($FromRelease) {
    $repo = "AmarsDing/aranea-agents"
    $assetName = "windows-deps.zip"
    $downloadUrl = "https://github.com/$repo/releases/download/$ReleaseTag/$assetName"

    Write-Host "[download-deps] 从 GitHub Release 下载依赖：$downloadUrl" -ForegroundColor Cyan
    New-Item -ItemType Directory -Force -Path $OutDirFull | Out-Null

    $zipPath = Join-Path $OutDirFull $assetName
    Write-Host "  下载中..."
    try {
        Invoke-WebRequest -Uri $downloadUrl -OutFile $zipPath -UseBasicParsing
    } catch {
        Write-Error "下载失败：$($_.Exception.Message)。请确保 Release $ReleaseTag 中有 asset：$assetName"
    }

    Write-Host "  解压中..."
    Expand-Archive -Path $zipPath -DestinationPath $OutDirFull -Force
    Remove-Item $zipPath -Force

    Write-Host "[download-deps] GitHub Release 依赖下载完成" -ForegroundColor Green
    return
}

# ─── 模式 3：从官方 URL 下载 ──────────────────────────────────
# 注意：此模式不包含 pgvector，需要手动添加 vector.dll
$PgVersion = "17.5-1"
$PgUrl = "https://get.enterprisedb.com/postgresql/postgresql-$PgVersion-windows-x64-binaries.zip"
$RedisUrl = "https://github.com/tporadowski/redis/releases/download/v5.0.14.1/Redis-x64-5.0.14.1.zip"

New-Item -ItemType Directory -Force -Path $OutDirFull | Out-Null

# 带超时和重试的下载函数
function Invoke-DownloadWithRetry {
    param([string]$Url, [string]$OutFile, [string]$Label, [int]$MaxRetries = 3, [int]$TimeoutSec = 300)
    for ($i = 1; $i -le $MaxRetries; $i++) {
        try {
            Write-Host "  [$Label] 尝试 $i/$MaxRetries..." -ForegroundColor Cyan
            Invoke-WebRequest -Uri $Url -OutFile $OutFile -UseBasicParsing -TimeoutSec $TimeoutSec
            return $true
        } catch {
            Write-Host "  [$Label] 尝试 $i 失败：$($_.Exception.Message)" -ForegroundColor Yellow
            if (Test-Path $OutFile) { Remove-Item $OutFile -Force }
            if ($i -lt $MaxRetries) { Start-Sleep -Seconds 10 }
        }
    }
    return $false
}

# 下载 PostgreSQL
$pgDir = Join-Path $OutDirFull "postgres"
if (-not (Test-Path (Join-Path $pgDir "bin\postgres.exe"))) {
    Write-Host "[download-deps] 下载 PostgreSQL $PgVersion..." -ForegroundColor Cyan
    $pgZip = Join-Path $OutDirFull "pg-binaries.zip"
    $ok = Invoke-DownloadWithRetry -Url $PgUrl -OutFile $pgZip -Label "PostgreSQL"
    if (-not $ok) {
        Write-Error "PostgreSQL 下载失败（重试 3 次）。请手动下载并放到 $pgDir"
    }

    Write-Host "  解压中..."
    $pgExtract = Join-Path $OutDirFull "pg-extract"
    Expand-Archive -Path $pgZip -DestinationPath $pgExtract -Force
    Remove-Item $pgZip -Force

    # EnterpriseDB zip 解压后是 pgsql/ 目录，重命名为 postgres/
    $pgsqlDir = Join-Path $pgExtract "pgsql"
    if (Test-Path $pgDir) { Remove-Item $pgDir -Recurse -Force }
    Move-Item $pgsqlDir $pgDir -Force
    Remove-Item $pgExtract -Recurse -Force
    Write-Host "  [OK] PostgreSQL downloaded to postgres/" -ForegroundColor Green
} else {
    Write-Host "  [SKIP] PostgreSQL already exists" -ForegroundColor Yellow
}

# 检查 pgvector
$vectorDll = Join-Path $pgDir "lib\vector.dll"
if (-not (Test-Path $vectorDll)) {
    Write-Host ""
    Write-Host "  [WARN] pgvector (vector.dll) 未找到！" -ForegroundColor Yellow
    Write-Host "         PostgreSQL 可用但向量搜索功能不可用。" -ForegroundColor Yellow
    Write-Host "         请手动编译 pgvector 并将 vector.dll 放到:" -ForegroundColor Yellow
    Write-Host "         $vectorDll" -ForegroundColor Yellow
    Write-Host "         或使用 -FromRelease 模式下载预打包依赖。" -ForegroundColor Yellow
    Write-Host ""
}

# 下载 Redis
$redisDir = Join-Path $OutDirFull "redis"
if (-not (Test-Path (Join-Path $redisDir "redis-server.exe"))) {
    Write-Host "[download-deps] 下载 Redis for Windows..." -ForegroundColor Cyan
    $redisZip = Join-Path $OutDirFull "redis.zip"
    $ok = Invoke-DownloadWithRetry -Url $RedisUrl -OutFile $redisZip -Label "Redis"
    if (-not $ok) {
        Write-Error "Redis 下载失败（重试 3 次）"
    }

    Write-Host "  解压中..."
    New-Item -ItemType Directory -Force -Path $redisDir | Out-Null
    Expand-Archive -Path $redisZip -DestinationPath $redisDir -Force
    Remove-Item $redisZip -Force

    # 创建 redis.conf（精简版，仅必要配置）
    $redisConf = @"
bind 127.0.0.1
protected-mode yes
port 6379
timeout 0
tcp-keepalive 300
loglevel notice
databases 16
save 900 1
save 300 10
save 60 10000
stop-writes-on-bgsave-error yes
rdbcompression yes
rdbchecksum yes
dbfilename dump.rdb
dir ./
appendonly no
appendfsync everysec
"@
    Set-Content -Path (Join-Path $redisDir "redis.conf") -Value $redisConf -Encoding UTF8
    Write-Host "  [OK] Redis downloaded to redis/" -ForegroundColor Green
} else {
    Write-Host "  [SKIP] Redis already exists" -ForegroundColor Yellow
}

Write-Host "[download-deps] 依赖下载完成" -ForegroundColor Green
