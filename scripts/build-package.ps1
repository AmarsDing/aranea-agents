<#
.SYNOPSIS
    Aranea-Agents Windows 安装包自动构建脚本。

.DESCRIPTION
    完整构建流程：
      1. 编译后端 Go 二进制（admin → aranea-server.exe）
      2. 编译前端 Vue 应用（quasar build）
      3. 打包 Electron 桌面应用（electron-packager）
      4. 下载/准备依赖（PostgreSQL + Redis）
      5. 组装部署目录（AraneaAgents-deploy 结构）
      6. 调用 NSIS 生成 .exe 安装包

.PARAMETER Version
    版本号（默认从 git tag 获取）

.PARAMETER OutDir
    输出目录（默认：release/）

.PARAMETER DepsDir
    依赖目录（包含 postgres/ 和 redis/）。不指定则自动下载。

.PARAMETER SkipDepsDownload
    跳过依赖下载（用于本地开发，需配合 -DepsDir 使用）

.PARAMETER SkipNSIS
    跳过 NSIS 打包（只生成部署目录，不生成 .exe 安装包）

.PARAMETER SkipBackend
    跳过后端编译（使用已有的 aranea-server.exe）

.PARAMETER SkipFrontend
    跳过前端编译和 Electron 打包

.EXAMPLE
    # 完整构建（自动下载依赖）
    .\scripts\build-package.ps1

.EXAMPLE
    # 从本地依赖目录构建
    .\scripts\build-package.ps1 -DepsDir .\AraneaAgents-deploy

.EXAMPLE
    # 只构建指定版本
    .\scripts\build-package.ps1 -Version v1.0.0 -DepsDir .\build\deps
#>
param(
    [string]$Version,
    [string]$OutDir = "release",
    [string]$DepsDir,
    [switch]$SkipDepsDownload,
    [switch]$SkipNSIS,
    [switch]$SkipBackend,
    [switch]$SkipFrontend
)

$ErrorActionPreference = "Stop"
$RepoRoot = Resolve-Path "$PSScriptRoot/.."
Set-Location $RepoRoot

# ─── 版本号 ────────────────────────────────────────────────────
if (-not $Version) {
    $gitTag = git describe --tags --always 2>$null
    $Version = if ($gitTag) { $gitTag } else { "dev-$(Get-Date -Format 'yyyyMMdd')" }
}
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Aranea-Agents Package Builder" -ForegroundColor Cyan
Write-Host "  Version: $Version" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# ─── 路径定义 ──────────────────────────────────────────────────
$BuildDir = Join-Path $RepoRoot "build"
# 检查 staging 是否被锁定（default_app.asar 等遗留文件常被 IDE 索引器持有）
$stagingDefault = Join-Path $BuildDir "staging"
$stagingLocked = $false
if (Test-Path $stagingDefault) {
    $emptyTmp = Join-Path $BuildDir "empty_tmp_clear"
    New-Item -ItemType Directory -Force -Path $emptyTmp | Out-Null
    & robocopy $emptyTmp $stagingDefault /MIR /NFL /NDL /NJH /NJS 2>&1 | Out-Null
    Remove-Item $emptyTmp -Force -Recurse -ErrorAction SilentlyContinue
    try {
        Remove-Item $stagingDefault -Recurse -Force -ErrorAction Stop
    } catch {
        $stagingLocked = $true
    }
}
# 若 staging 被锁定无法删除，回退到 staging-v2 目录
$StagingDir = if ($stagingLocked) {
    Write-Host "[INFO] staging locked, using staging-v2" -ForegroundColor Yellow
    Join-Path $BuildDir "staging-v2"
} else {
    $stagingDefault
}
$DepsTargetDir = Join-Path $BuildDir "deps"
$OutDirFull = if ([System.IO.Path]::IsPathRooted($OutDir)) { $OutDir } else { Join-Path $RepoRoot $OutDir }

# ─── Step 1: 编译后端 ─────────────────────────────────────────
if (-not $SkipBackend) {
    Write-Host "[1/6] Building backend (admin → aranea-server.exe)..." -ForegroundColor Cyan

    $ldflags = "-s -w -X main.Version=$Version -X main.Name=aranea-agents"
    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"

    & go build -tags pgvector -ldflags $ldflags -o (Join-Path $BuildDir "aranea-server.exe") ./cmd/admin
    if ($LASTEXITCODE -ne 0) {
        Write-Error "后端编译失败"
    }
    Write-Host "  [OK] aranea-server.exe built" -ForegroundColor Green
} else {
    Write-Host "[1/6] Skipping backend build" -ForegroundColor Yellow
}

# ─── Step 2: 编译前端 ──────────────────────────────────────────
if (-not $SkipFrontend) {
    Write-Host "[2/6] Building frontend (quasar build)..." -ForegroundColor Cyan

    Push-Location (Join-Path $RepoRoot "web")
    & pnpm install --frozen-lockfile 2>$null
    if ($LASTEXITCODE -ne 0) {
        Write-Host "  pnpm install failed, retrying without frozen-lockfile..." -ForegroundColor Yellow
        & pnpm install
    }
    & pnpm build
    if ($LASTEXITCODE -ne 0) {
        Pop-Location
        Write-Error "前端编译失败"
    }

    # 确保 esbuild 可用
    $esbuildPath = Join-Path $RepoRoot "web\node_modules\.bin\esbuild.CMD"
    if (-not (Test-Path $esbuildPath)) {
        Write-Host "  安装 esbuild..." -ForegroundColor Yellow
        & pnpm add -D esbuild
    }
    Pop-Location
    Write-Host "  [OK] frontend built" -ForegroundColor Green
} else {
    Write-Host "[2/6] Skipping frontend build" -ForegroundColor Yellow
}

# ─── Step 3: 打包 Electron 应用 ────────────────────────────────
if (-not $SkipFrontend) {
    Write-Host "[3/6] Packaging Electron app..." -ForegroundColor Cyan

    Push-Location (Join-Path $RepoRoot "web")
    $env:QUASAR_ELECTRON_PRELOAD_FOLDER = "preload"
    $env:QUASAR_ELECTRON_PRELOAD_EXTENSION = ".cjs"
    & node (Join-Path $RepoRoot "web\scripts\build-electron.mjs") --platform=win32 --arch=x64
    if ($LASTEXITCODE -ne 0) {
        Pop-Location
        Write-Error "Electron 打包失败"
    }
    Pop-Location
    Write-Host "  [OK] Electron app packaged" -ForegroundColor Green
} else {
    Write-Host "[3/6] Skipping Electron packaging" -ForegroundColor Yellow
}

# ─── Step 4: 准备依赖 ──────────────────────────────────────────
Write-Host "[4/6] Preparing dependencies (PostgreSQL + Redis)..." -ForegroundColor Cyan

if ($DepsDir) {
    # 从指定目录复制
    $depsSrc = if ([System.IO.Path]::IsPathRooted($DepsDir)) { $DepsDir } else { Join-Path $RepoRoot $DepsDir }
    Write-Host "  从本地目录复制依赖：$depsSrc" -ForegroundColor Yellow
    & (Join-Path $RepoRoot "scripts\download-deps.ps1") -LocalDepsDir $depsSrc -OutDir $DepsTargetDir
    if ($LASTEXITCODE -ne 0) {
        Write-Error "依赖准备失败"
    }
} elseif (-not $SkipDepsDownload) {
    # 自动下载
    if (Test-Path (Join-Path $DepsTargetDir "postgres\bin\postgres.exe")) {
        Write-Host "  依赖已存在，跳过下载（使用 -SkipDepsDownload=false 强制重新下载）" -ForegroundColor Yellow
    } else {
        Write-Host "  自动下载依赖..." -ForegroundColor Yellow
        & (Join-Path $RepoRoot "scripts\download-deps.ps1") -OutDir $DepsTargetDir
        if ($LASTEXITCODE -ne 0) {
            Write-Error "依赖下载失败"
        }
    }
} else {
    Write-Host "  跳过依赖下载" -ForegroundColor Yellow
    if (-not (Test-Path (Join-Path $DepsTargetDir "postgres\bin\postgres.exe"))) {
        Write-Error "依赖目录不存在且跳过了下载。请提供 -DepsDir 或去掉 -SkipDepsDownload"
    }
}
Write-Host "  [OK] dependencies ready" -ForegroundColor Green

# ─── Step 5: 组装部署目录 ──────────────────────────────────────
Write-Host "[5/6] Assembling deploy directory..." -ForegroundColor Cyan

# 清理 staging 目录（带文件锁 fallback：start-stderr.log 常被遗留 cmd.exe 子进程持有）
if (Test-Path $StagingDir) {
    try {
        Remove-Item $StagingDir -Recurse -Force -ErrorAction Stop
    } catch {
        Write-Host "  [WARN] Remove-Item failed, trying Move-Item fallback..." -ForegroundColor Yellow
        $ts = Get-Date -Format 'yyyyMMdd-HHmmss'
        $backup = "$StagingDir-old-$ts"
        Move-Item $StagingDir $backup -Force
        Write-Host "  [INFO] Old staging moved to: $backup" -ForegroundColor Yellow
    }
}
New-Item -ItemType Directory -Force -Path $StagingDir | Out-Null

# 复制后端二进制
Copy-Item (Join-Path $BuildDir "aranea-server.exe") (Join-Path $StagingDir "aranea-server.exe") -Force
Write-Host "  [OK] aranea-server.exe" -ForegroundColor Green

# 复制 Electron 应用
$electronAppDir = Join-Path $RepoRoot "web\build\electron\AraneaAgents-win32-x64"
if (-not (Test-Path $electronAppDir)) {
    # 兼容 fallback 目录（带时间戳，位于 web/build/electron-<ts>/）
    $fallbackBase = Join-Path $RepoRoot "web\build"
    if (Test-Path $fallbackBase) {
        $fallback = Get-ChildItem -Path $fallbackBase -Directory -Filter "electron-*" | Sort-Object LastWriteTime -Descending | Select-Object -First 1
        if ($fallback) {
            $electronAppDir = Join-Path $fallback.FullName "AraneaAgents-win32-x64"
        }
    }
}
if (-not (Test-Path $electronAppDir)) {
    Write-Error "Electron 应用目录不存在：$electronAppDir"
}
Copy-Item $electronAppDir (Join-Path $StagingDir "frontend") -Recurse -Force
Write-Host "  [OK] frontend/" -ForegroundColor Green

# 复制配置文件
$configDir = Join-Path $StagingDir "configs"
New-Item -ItemType Directory -Force -Path $configDir | Out-Null
# 使用 deploy 专用配置（PostgreSQL 端口 5433，无密码）
$deployConfig = @"
server:
  http:
    addr: 0.0.0.0:8000
    timeout: 0s
  grpc:
    addr: 0.0.0.0:9000
    timeout: 120s
  ws:
    enable: true
    network: tcp
    addr: 0.0.0.0:8002
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
    source: "postgres://postgres@127.0.0.1:5433/aranea?sslmode=disable"
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
Set-Content -Path (Join-Path $configDir "config.yaml") -Value $deployConfig -Encoding UTF8
Write-Host "  [OK] configs/config.yaml" -ForegroundColor Green

# 复制 PostgreSQL 便携版（排除 data/ 目录）
$srcPg = Join-Path $DepsTargetDir "postgres"
$destPg = Join-Path $StagingDir "postgres"
if (Test-Path $srcPg) {
    New-Item -ItemType Directory -Force -Path $destPg | Out-Null
    # 只复制 bin/ 和 lib/ 目录（不复制 data/，首次启动自动初始化）
    $pgBin = Join-Path $srcPg "bin"
    $pgLib = Join-Path $srcPg "lib"
    $pgShare = Join-Path $srcPg "share"
    if (Test-Path $pgBin) { Copy-Item $pgBin (Join-Path $destPg "bin") -Recurse -Force }
    if (Test-Path $pgLib) { Copy-Item $pgLib (Join-Path $destPg "lib") -Recurse -Force }
    if (Test-Path $pgShare) { Copy-Item $pgShare (Join-Path $destPg "share") -Recurse -Force }
    Write-Host "  [OK] postgres/ (bin + lib + share)" -ForegroundColor Green
} else {
    Write-Host "  [WARN] postgres/ not found in deps" -ForegroundColor Yellow
}

# 复制 Redis
$srcRedis = Join-Path $DepsTargetDir "redis"
$destRedis = Join-Path $StagingDir "redis"
if (Test-Path $srcRedis) {
    Copy-Item $srcRedis $destRedis -Recurse -Force
    # 删除可能存在的 dump.rdb（持久化数据）
    $rdbFile = Join-Path $destRedis "dump.rdb"
    if (Test-Path $rdbFile) { Remove-Item $rdbFile -Force }
    Write-Host "  [OK] redis/" -ForegroundColor Green
} else {
    Write-Host "  [WARN] redis/ not found in deps" -ForegroundColor Yellow
}

# 创建 logs 目录
New-Item -ItemType Directory -Force -Path (Join-Path $StagingDir "logs") | Out-Null

# 复制启停脚本
$scriptsDir = Join-Path $RepoRoot "installer\scripts"
$startBat = Join-Path $scriptsDir "start.bat"
$stopBat = Join-Path $scriptsDir "stop.bat"
if (Test-Path $startBat) {
    Copy-Item $startBat (Join-Path $StagingDir "start.bat") -Force
} else {
    # 使用现有 AraneaAgents-deploy 的脚本
    Copy-Item (Join-Path $RepoRoot "AraneaAgents-deploy\start.bat") (Join-Path $StagingDir "start.bat") -Force
}
if (Test-Path $stopBat) {
    Copy-Item $stopBat (Join-Path $StagingDir "stop.bat") -Force
} else {
    Copy-Item (Join-Path $RepoRoot "AraneaAgents-deploy\stop.bat") (Join-Path $StagingDir "stop.bat") -Force
}
Write-Host "  [OK] start.bat + stop.bat" -ForegroundColor Green

# 创建 README
$readmeContent = @"
# Aranea-Agents v$Version

## 快速开始

1. 双击 start.bat 启动
2. 首次启动自动初始化数据库（约 10 秒）
3. 桌面应用自动打开
4. 登录：admin / changeme

## 停止

双击 stop.bat

## 端口

| 端口 | 服务 |
|------|------|
| 8000 | 后端 HTTP API |
| 9000 | gRPC |
| 8002 | WebSocket |
| 5433 | PostgreSQL（便携版） |
| 6379 | Redis |

版本: $Version
构建时间: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')
"@
Set-Content -Path (Join-Path $StagingDir "README.md") -Value $readmeContent -Encoding UTF8

Write-Host "  [OK] Staging directory ready: $StagingDir" -ForegroundColor Green

# ─── Step 6: NSIS 打包 ────────────────────────────────────────
if (-not $SkipNSIS) {
    Write-Host "[6/6] Building NSIS installer..." -ForegroundColor Cyan

    $nsiScript = Join-Path $RepoRoot "installer\aranea.nsi"
    if (-not (Test-Path $nsiScript)) {
        Write-Error "NSIS 脚本不存在：$nsiScript"
    }

    # 查找 makensis
    $makensis = Get-Command "makensis" -ErrorAction SilentlyContinue
    if (-not $makensis) {
        # 尝试常见安装路径
        $nsisPaths = @(
            "C:\Program Files\NSIS\makensis.exe",
            "C:\Program Files (x86)\NSIS\makensis.exe"
        )
        foreach ($p in $nsisPaths) {
            if (Test-Path $p) {
                $makensis = @{ Source = $p }
                break
            }
        }
    }

    if (-not $makensis) {
        Write-Host "  [WARN] makensis 未找到！跳过 NSIS 打包。" -ForegroundColor Yellow
        Write-Host "         请安装 NSIS: https://nsis.sourceforge.io/Download" -ForegroundColor Yellow
        Write-Host "         或使用 choco: choco install nsis" -ForegroundColor Yellow
        Write-Host ""
        Write-Host "  生成的部署目录: $StagingDir" -ForegroundColor Green
        Write-Host "  可以手动压缩为 zip 分发" -ForegroundColor Yellow
    } else {
        $nsisExe = $makensis.Source
        New-Item -ItemType Directory -Force -Path $OutDirFull | Out-Null

        Write-Host "  运行 NSIS: $nsisExe" -ForegroundColor Yellow
        & $nsisExe /V2 /DVERSION=$Version /DSTAGING_DIR=$StagingDir /DOUT_DIR=$OutDirFull $nsiScript
        if ($LASTEXITCODE -ne 0) {
            Write-Error "NSIS 打包失败"
        }

        $installerName = "AraneaAgents-${Version}-win-x64.exe"
        $installerPath = Join-Path $OutDirFull $installerName
        if (Test-Path $installerPath) {
            $size = (Get-Item $installerPath).Length / 1MB
            Write-Host "  [OK] Installer created: $installerPath ($([math]::Round($size, 1)) MB)" -ForegroundColor Green
        }
    }
} else {
    Write-Host "[6/6] Skipping NSIS packaging" -ForegroundColor Yellow
}

# ─── 完成 ─────────────────────────────────────────────────────
Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "  Build Complete!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host "  Version:  $Version" -ForegroundColor Green
Write-Host "  Staging:  $StagingDir" -ForegroundColor Green
if (-not $SkipNSIS -and (Test-Path variable:installerPath) -and (Test-Path $installerPath)) {
    Write-Host "  Installer: $installerPath" -ForegroundColor Green
}
Write-Host ""
