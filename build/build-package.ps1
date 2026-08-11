<#
.SYNOPSIS
    Aranea-Agents Windows 安装包自动构建脚本。

.DESCRIPTION
    完整构建流程：
      1. 编译后端 Go 二进制（admin → aranea-server.exe）
      2. 编译前端 Vue 应用（quasar build）
      3. 打包 Tauri 桌面应用（cargo build --release）
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
    跳过前端编译和 Tauri 打包

.EXAMPLE
    # 完整构建（自动下载依赖）
    .\build\build-package.ps1

.EXAMPLE
    # 从本地依赖目录构建
    .\build\build-package.ps1 -DepsDir .\build\deps

.EXAMPLE
    # 只构建指定版本
    .\build\build-package.ps1 -Version v1.0.0 -DepsDir .\build\deps
#>
param(
    [string]$Version,
    [string]$OutDir = "build\release",
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
$BinDir = Join-Path $RepoRoot "bin"
New-Item -ItemType Directory -Force -Path $BinDir | Out-Null
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

# ─── Step 1: 编译后端 + 静默启动器 ────────────────────────────
if (-not $SkipBackend) {
    Write-Host "[1/6] Building backend (admin → aranea-server.exe)..." -ForegroundColor Cyan

    $ldflags = "-s -w -X main.Version=$Version -X main.Name=aranea-agents"
    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"

    & go build -tags pgvector -ldflags $ldflags -o (Join-Path $BinDir "aranea-server.exe") ./cmd/admin
    if ($LASTEXITCODE -ne 0) {
        Write-Error "后端编译失败"
    }
    Write-Host "  [OK] bin/aranea-server.exe built" -ForegroundColor Green

    # -H windowsgui：双击无黑框控制台
    $launcherLdflags = "-s -w -H windowsgui"
    & go build -ldflags $launcherLdflags -o (Join-Path $BinDir "AraneaLauncher.exe") ./cmd/launcher
    if ($LASTEXITCODE -ne 0) {
        Write-Error "启动器编译失败"
    }
    Write-Host "  [OK] bin/AraneaLauncher.exe built (windowsgui)" -ForegroundColor Green
} else {
    Write-Host "[1/6] Skipping backend build" -ForegroundColor Yellow
    if (-not (Test-Path (Join-Path $BinDir "AraneaLauncher.exe"))) {
        Write-Host "  Building AraneaLauncher.exe (required for packaging)..." -ForegroundColor Yellow
        $env:CGO_ENABLED = "0"
        $env:GOOS = "windows"
        $env:GOARCH = "amd64"
        & go build -ldflags "-s -w -H windowsgui" -o (Join-Path $BinDir "AraneaLauncher.exe") ./cmd/launcher
        if ($LASTEXITCODE -ne 0) {
            Write-Error "启动器编译失败"
        }
    }
}

# ─── Step 2: 编译前端 ──────────────────────────────────────────
if (-not $SkipFrontend) {
    Write-Host "[2/6] Building frontend (quasar build)..." -ForegroundColor Cyan

    Push-Location (Join-Path $RepoRoot "web")
    # PS 5.1: native cmd stderr + EAP=Stop => terminating NativeCommandError (Vite/Rollup warnings).
    # Relax EAP during install/build; rely on exit codes for real failures.
    $prevEap = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    & pnpm install --frozen-lockfile 2>$null
    if ($LASTEXITCODE -ne 0) {
        Write-Host "  pnpm install failed, retrying without frozen-lockfile..." -ForegroundColor Yellow
        & pnpm install
    }
    & pnpm build
    $frontendExit = $LASTEXITCODE
    $ErrorActionPreference = $prevEap
    if ($frontendExit -ne 0) {
        Pop-Location
        Write-Error "前端编译失败"
    }
    Pop-Location
    Write-Host "  [OK] frontend built" -ForegroundColor Green
} else {
    Write-Host "[2/6] Skipping frontend build" -ForegroundColor Yellow
}

# ─── Step 3: 打包 Tauri 应用 ──────────────────────────────────
if (-not $SkipFrontend) {
    Write-Host "[3/6] Packaging Tauri app (cargo build --release)..." -ForegroundColor Cyan

    Push-Location (Join-Path $RepoRoot "web")
    $prevEap = $ErrorActionPreference
    $ErrorActionPreference = 'Continue'
    & node (Join-Path $RepoRoot "web\scripts\build-tauri.mjs")
    $tauriExit = $LASTEXITCODE
    $ErrorActionPreference = $prevEap
    if ($tauriExit -ne 0) {
        Pop-Location
        Write-Error "Tauri 打包失败（需要本机已安装 Rust stable 工具链 + MSVC）"
    }
    Pop-Location
    Write-Host "  [OK] Tauri app packaged" -ForegroundColor Green
} else {
    Write-Host "[3/6] Skipping Tauri packaging" -ForegroundColor Yellow
}

# ─── Step 4: 准备依赖 ──────────────────────────────────────────
Write-Host "[4/6] Preparing dependencies (PostgreSQL + Redis)..." -ForegroundColor Cyan

if ($DepsDir) {
    # 从指定目录复制
    $depsSrc = if ([System.IO.Path]::IsPathRooted($DepsDir)) { $DepsDir } else { Join-Path $RepoRoot $DepsDir }
    Write-Host "  从本地目录复制依赖：$depsSrc" -ForegroundColor Yellow
    & (Join-Path $RepoRoot "build\download-deps.ps1") -LocalDepsDir $depsSrc -OutDir $DepsTargetDir
    if ($LASTEXITCODE -ne 0) {
        Write-Error "依赖准备失败"
    }
} elseif (-not $SkipDepsDownload) {
    # 自动下载
    if (Test-Path (Join-Path $DepsTargetDir "postgres\bin\postgres.exe")) {
        Write-Host "  依赖已存在，跳过下载（使用 -SkipDepsDownload=false 强制重新下载）" -ForegroundColor Yellow
    } else {
        Write-Host "  自动下载依赖..." -ForegroundColor Yellow
        & (Join-Path $RepoRoot "build\download-deps.ps1") -OutDir $DepsTargetDir
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

# 复制后端二进制 + 静默启动器
Copy-Item (Join-Path $BinDir "aranea-server.exe") (Join-Path $StagingDir "aranea-server.exe") -Force
Write-Host "  [OK] aranea-server.exe" -ForegroundColor Green
$launcherSrc = Join-Path $BinDir "AraneaLauncher.exe"
if (-not (Test-Path $launcherSrc)) {
    Write-Error "缺少 AraneaLauncher.exe，请先编译 ./cmd/launcher"
}
Copy-Item $launcherSrc (Join-Path $StagingDir "AraneaLauncher.exe") -Force
Write-Host "  [OK] AraneaLauncher.exe" -ForegroundColor Green

# 复制 Tauri 应用（单 exe：AraneaAgents.exe，内嵌前端静态资源）
$tauriAppDir = Join-Path $RepoRoot "web\build\tauri\AraneaAgents-win32-x64"
if (-not (Test-Path (Join-Path $tauriAppDir "AraneaAgents.exe"))) {
    Write-Error "Tauri 应用目录不存在或缺少 AraneaAgents.exe：$tauriAppDir"
}
Copy-Item $tauriAppDir (Join-Path $StagingDir "frontend") -Recurse -Force
Write-Host "  [OK] frontend/" -ForegroundColor Green

# 复制配置文件
$configDir = Join-Path $StagingDir "configs"
New-Item -ItemType Directory -Force -Path $configDir | Out-Null
# 使用 deploy 专用配置（PostgreSQL 端口 5433，无密码）
$deployConfig = @"
server:
  http:
    addr: 0.0.0.0:8800
    timeout: 0s
  grpc:
    addr: 0.0.0.0:9900
    timeout: 120s
  ws:
    enable: true
    network: tcp
    addr: 0.0.0.0:8802
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

# 复制运行时种子资源（相对安装目录 CWD，与 biz.ScenarioDir / modelregistry.Store 一致）
# 1) internal/scenario：agency-pack / builtin-templates / system prompts → Agent + 公司架构
# 2) data/model-catalog：厂商目录 + logos → Channel/Provider 资源与图标
function Copy-RequiredRuntimeAssets {
    param([string]$SrcRel, [string]$DestRel, [string]$Label)

    $src = Join-Path $RepoRoot $SrcRel
    $dest = Join-Path $StagingDir $DestRel
    if (-not (Test-Path $src)) {
        Write-Error "缺少运行时资源：$SrcRel（$Label）"
    }
    $destParent = Split-Path $dest -Parent
    New-Item -ItemType Directory -Force -Path $destParent | Out-Null
    if (Test-Path $dest) {
        Remove-Item $dest -Recurse -Force
    }
    Copy-Item $src $dest -Recurse -Force
    $fileCount = (Get-ChildItem $dest -Recurse -File -ErrorAction SilentlyContinue | Measure-Object).Count
    if ($fileCount -lt 1) {
        Write-Error "运行时资源为空：$DestRel（$Label）"
    }
    Write-Host "  [OK] $DestRel/ ($fileCount files) — $Label" -ForegroundColor Green
}

Copy-RequiredRuntimeAssets -SrcRel "internal\scenario" -DestRel "internal\scenario" -Label "packs + prompts (agents/org)"
Copy-RequiredRuntimeAssets -SrcRel "data\model-catalog" -DestRel "data\model-catalog" -Label "providers + logos"

# 校验关键种子路径（失败则禁止打包）
$requiredSeedPaths = @(
    "internal\scenario\packs\agency-pack",
    "internal\scenario\packs\builtin-templates",
    "internal\scenario\system",
    "internal\scenario\system\prompts\IDENTITY.md",
    "internal\scenario\system\prompts\orchestrator.md",
    "data\model-catalog\current.json",
    "data\model-catalog\logos"
)
foreach ($rel in $requiredSeedPaths) {
    $p = Join-Path $StagingDir $rel
    if (-not (Test-Path $p)) {
        Write-Error "Staging 缺少必需资源：$rel — 禁止打包"
    }
}
Write-Host "  [OK] seed asset paths verified" -ForegroundColor Green

# 复制启停脚本
$scriptsDir = Join-Path $RepoRoot "build\installer\scripts"
$startBat = Join-Path $scriptsDir "start.bat"
$stopBat = Join-Path $scriptsDir "stop.bat"
if (-not (Test-Path $startBat)) { Write-Error "缺少 $startBat" }
if (-not (Test-Path $stopBat)) { Write-Error "缺少 $stopBat" }
Copy-Item $startBat (Join-Path $StagingDir "start.bat") -Force
Copy-Item $stopBat (Join-Path $StagingDir "stop.bat") -Force
$startVbs = Join-Path $scriptsDir "start-silent.vbs"
if (Test-Path $startVbs) {
    Copy-Item $startVbs (Join-Path $StagingDir "start-silent.vbs") -Force
}
Write-Host "  [OK] start.bat + stop.bat + start-silent.vbs" -ForegroundColor Green
if (-not (Test-Path (Join-Path $StagingDir "AraneaLauncher.exe"))) {
    Write-Error "Staging 缺少 AraneaLauncher.exe — 禁止打包"
}

# 创建 README
$readmeContent = @"
# Aranea-Agents v$Version

## 快速开始

1. 双击桌面「Aranea-Agents」快捷方式（或 AraneaLauncher.exe）
2. 首次运行自动弹出初始化配置向导（openclaw 风格控制台）：环境检查 → 选择 PostgreSQL/Redis（内置便携版或系统实例）→ 可选注册开机自启
3. 之后启动会先做环境检查：优先使用已保存配置；否则自动探测本机 PostgreSQL / Redis，未检出则启用内置实例
4. 桌面应用自动打开；登录：admin / changeme

## 环境检查与配置

- 开始菜单「环境检查」或：``AraneaLauncher.exe -check``
- 开始菜单「Configure (Setup Wizard)」或：``AraneaLauncher.exe -setup``（重新运行配置向导）

若系统 PostgreSQL 需要密码，任选其一：
- 用户环境变量 ``ARANEA_PG_PASSWORD=你的密码``
- 或在安装目录创建 ``configs\pg.password``（单行密码）
- 或在配置向导中选择系统 PostgreSQL 并输入密码（自动写入 pg.password）

## 开机自启

- 注册：``AraneaLauncher.exe -install-autostart``
  - 系统 PostgreSQL + 系统 Redis → 注册为 Windows 服务（延迟自启）
  - 含内置组件 → 注册为用户登录计划任务（无需管理员）
- 移除：``AraneaLauncher.exe -uninstall-autostart``
- 无界面后台启动（自启动项内部使用）：``AraneaLauncher.exe -headless``

## 停止

- 开始菜单「停止 Aranea-Agents」，或
- ``AraneaLauncher.exe -stop``，或
- ``stop.bat``

（不会停止系统自带的 PostgreSQL/Redis）

## 调试

若需查看控制台日志，使用开始菜单「启动（调试控制台）」或 ``start.bat``。
故障日志：``logs\launcher.log``、``logs\server.log``、``logs\preflight.txt``、``logs\postgres.log``

## 端口

| 端口 | 服务 |
|------|------|
| 8800 | 后端 HTTP / WebSocket (``/v1/ws``) |
| 9900 | gRPC |
| 8802 | WebSocket 独立端口 |
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

    $nsiScript = Join-Path $RepoRoot "build\installer\aranea.nsi"
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
