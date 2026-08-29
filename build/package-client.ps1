#Requires -Version 5.1
<#
.SYNOPSIS
    Aranea 桌面客户端一键打包（服务+客户端形态：客户端与服务器分离部署）。

.DESCRIPTION
    流程：
      1. 前置检查（node / pnpm / cargo 默认 toolchain）
      2. pnpm install --frozen-lockfile
      3. pnpm build（quasar build → web/dist/spa）
      4. node web/scripts/build-tauri.mjs（cargo build --release，rust-embed 内嵌 SPA → 单 exe）
      5. 附客户端 README（首启配置服务器地址说明）
      6. 打 zip → build/release/AraneaAgents-<Version>-win-x64.zip

    产物为绿色包：解压即用，首启在设置页填服务端地址（http://<服务器IP>:8810），
    地址持久化于 backend-config.json，修改免重启。

.PARAMETER Version
    版本号（默认 git describe --tags --always，兜底 dev-yyyyMMdd）

.PARAMETER OutDir
    输出目录（默认：build/release/）

.PARAMETER ServerUrl
    构建期注入的默认后端地址（如 http://192.168.0.102:8810）。
    注入后首启免填服务器地址；用户仍可在设置页修改（持久化覆盖默认值）。
    实现：临时改写 web/public/assets/config/runtime-config.json，打包结束恢复原内容。

.EXAMPLE
    powershell -ExecutionPolicy Bypass -File build\package-client.ps1
.EXAMPLE
    powershell -ExecutionPolicy Bypass -File build\package-client.ps1 -Version v1.2.0 -ServerUrl http://192.168.0.102:8810
#>
param(
    [string]$Version,
    [string]$OutDir = "build\release",
    [string]$ServerUrl
)

$ErrorActionPreference = "Stop"
$RepoRoot = Resolve-Path "$PSScriptRoot/.."
$WebDir   = Join-Path $RepoRoot "web"
Set-Location $RepoRoot

# ─── 版本号 ────────────────────────────────────────────────────
if (-not $Version) {
    $gitTag = git describe --tags --always 2>$null
    $Version = if ($gitTag) { $gitTag } else { "dev-$(Get-Date -Format 'yyyyMMdd')" }
}
Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Aranea Client Package Builder" -ForegroundColor Cyan
Write-Host "  Version: $Version" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# ─── Step 1: 前置检查 ─────────────────────────────────────────
Write-Host "[1/6] 前置检查（node / pnpm / cargo）..." -ForegroundColor Cyan

foreach ($tool in @('node', 'pnpm')) {
    if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
        Write-Error "未找到 $tool，请先安装并加入 PATH"
    }
}

# cargo 需已配置默认 toolchain（rustup 未设 default 时 cargo --version 非零退出）
$prevEap = $ErrorActionPreference
$ErrorActionPreference = 'Continue'
$null = & cargo --version 2>&1
$cargoCode = $LASTEXITCODE
$ErrorActionPreference = $prevEap
if ($cargoCode -ne 0) {
    Write-Error "cargo 不可用或未配置默认 toolchain。请先执行: rustup default stable"
}
Write-Host "  [OK] node / pnpm / cargo 就绪" -ForegroundColor Green

# ─── Step 2: 安装依赖 ─────────────────────────────────────────
Write-Host "[2/6] pnpm install --frozen-lockfile..." -ForegroundColor Cyan
Push-Location $WebDir
$prevEap = $ErrorActionPreference
$ErrorActionPreference = 'Continue'
# 非交互：node_modules 与 lockfile 不吻合时 pnpm 默认询问是否清空重装，CI=true + confirmModulesPurge=false 免询问
$env:CI = 'true'
& pnpm install --frozen-lockfile --config.confirmModulesPurge=false
if ($LASTEXITCODE -ne 0) {
    # 回退普通 install 会同步改写 pnpm-lock.yaml；若 lockfile 因此与仓库不一致，
    # 产物将基于未提交依赖状态（不可复现），必须拦截让人工提交后再打包
    Write-Host "  frozen-lockfile 失败，回退 pnpm install 同步依赖..." -ForegroundColor Yellow
    & pnpm install --config.confirmModulesPurge=false
    if ($LASTEXITCODE -ne 0) {
        $ErrorActionPreference = $prevEap
        Pop-Location
        Write-Error "pnpm install 失败"
    }
    $lockDirty = git -C $RepoRoot status --short -- web/pnpm-lock.yaml
    if ($lockDirty) {
        $ErrorActionPreference = $prevEap
        Pop-Location
        Write-Error "pnpm-lock.yaml 已被回退 install 改写且未提交（$lockDirty）。请先核对并提交 lockfile，再重新打包。"
    }
}
$ErrorActionPreference = $prevEap
Pop-Location
Write-Host "  [OK] dependencies installed" -ForegroundColor Green

# ─── Step 3: 前端构建 ─────────────────────────────────────────
# 构建期注入默认后端地址：改写 public/assets/config/runtime-config.json（quasar 拷入 dist/spa，
# rust-embed 内嵌进 exe），打包结束恢复原内容（finally 保证失败也恢复，不污染工作区）
$runtimeConfigPath = Join-Path $WebDir "public\assets\config\runtime-config.json"
$runtimeConfigBak = $null
if ($ServerUrl) {
    $ServerUrl = $ServerUrl.TrimEnd('/')
    $runtimeConfigBak = [IO.File]::ReadAllText($runtimeConfigPath)
    $injected = @{ backendUrl = $ServerUrl; wsOrigin = $ServerUrl } | ConvertTo-Json -Compress
    [IO.File]::WriteAllText($runtimeConfigPath, $injected, (New-Object System.Text.UTF8Encoding $false))
    Write-Host "  注入默认后端地址: $ServerUrl" -ForegroundColor Yellow
}
try {
Write-Host "[3/6] pnpm build（quasar build → dist/spa）..." -ForegroundColor Cyan
Push-Location $WebDir
$prevEap = $ErrorActionPreference
$ErrorActionPreference = 'Continue'
& pnpm build
$buildCode = $LASTEXITCODE
$ErrorActionPreference = $prevEap
Pop-Location
if ($buildCode -ne 0) { Write-Error "前端构建失败" }
if (-not (Test-Path (Join-Path $WebDir "dist\spa\index.html"))) {
    Write-Error "dist/spa/index.html 未生成"
}
Write-Host "  [OK] dist/spa built" -ForegroundColor Green

# ─── Step 4: Tauri 打包（rust-embed 内嵌 SPA → 单 exe）─────────
Write-Host "[4/6] Tauri 打包（cargo build --release）..." -ForegroundColor Cyan
$prevEap = $ErrorActionPreference
$ErrorActionPreference = 'Continue'
& node (Join-Path $WebDir "scripts\build-tauri.mjs")
$tauriCode = $LASTEXITCODE
$ErrorActionPreference = $prevEap
if ($tauriCode -ne 0) {
    Write-Error "Tauri 打包失败（需要 Rust stable 工具链 + MSVC；rustup default stable）"
}
$appDir = Join-Path $WebDir "build\tauri\AraneaAgents-win32-x64"
$exePath = Join-Path $appDir "AraneaAgents.exe"
if (-not (Test-Path $exePath)) { Write-Error "产物缺失：$exePath" }
Write-Host "  [OK] AraneaAgents.exe" -ForegroundColor Green

# ─── Step 5: 附客户端 README ──────────────────────────────────
Write-Host "[5/6] 写入客户端 README..." -ForegroundColor Cyan
$readme = @"
# AraneaAgents 桌面客户端 v$Version

## 使用

1. 解压本目录到任意位置（绿色软件，无需安装）
2. 双击 AraneaAgents.exe
3. 首次启动在设置页填写服务端地址，例如：

       http://192.168.0.102:8810

   地址保存后持久生效（backend-config.json），更换服务器免重启。

## 依赖

- Windows 10/11 x64
- 系统已装 WebView2 运行时（Win11 自带；Win10 若没有会自动提示安装）

版本: $Version
构建时间: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')
"@
Set-Content -Path (Join-Path $appDir "README.txt") -Value $readme -Encoding UTF8
Write-Host "  [OK] README.txt" -ForegroundColor Green

# ─── Step 6: 打 zip 分发包 ────────────────────────────────────
Write-Host "[6/6] 打 zip 分发包..." -ForegroundColor Cyan
$OutDirFull = if ([System.IO.Path]::IsPathRooted($OutDir)) { $OutDir } else { Join-Path $RepoRoot $OutDir }
New-Item -ItemType Directory -Force -Path $OutDirFull | Out-Null
$zipName = "AraneaAgents-$Version-win-x64.zip"
$zipPath = Join-Path $OutDirFull $zipName
if (Test-Path $zipPath) { Remove-Item $zipPath -Force }
Compress-Archive -Path $appDir -DestinationPath $zipPath -CompressionLevel Optimal

$zipMB = [math]::Round((Get-Item $zipPath).Length / 1MB, 1)
$exeMB = [math]::Round((Get-Item $exePath).Length / 1MB, 1)

Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "  Client Package Complete!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host "  Version:  $Version" -ForegroundColor Green
Write-Host "  Exe:      $exePath ($exeMB MB)" -ForegroundColor Green
Write-Host "  Zip:      $zipPath ($zipMB MB)" -ForegroundColor Green
Write-Host ""
} finally {
    # 恢复 runtime-config.json 原内容（注入了 -ServerUrl 时）
    if ($null -ne $runtimeConfigBak) {
        [IO.File]::WriteAllText($runtimeConfigPath, $runtimeConfigBak, (New-Object System.Text.UTF8Encoding $false))
        Write-Host "runtime-config.json 已恢复原内容" -ForegroundColor DarkGray
    }
}
