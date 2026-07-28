<#
.SYNOPSIS
    Aranea-Agents 一键打包并发布到 GitHub Release。

.DESCRIPTION
    流程：
      1. 校验版本号与 git 工作区状态
      2. 调用 build-package.ps1 完整构建安装包
      3. 创建/推送 git tag（可选）
      4. 通过 gh CLI 创建 GitHub Release 并上传安装包

    前置条件：
      - 已安装 GitHub CLI (gh) 并完成 gh auth login
      - 本机具备完整构建链（Go / pnpm / Rust / NSIS）

.PARAMETER Version
    版本号，如 v0.2.0（可省略 v 前缀）。必填。

.PARAMETER SkipBuild
    跳过构建，直接发布 build/release/ 下已存在的安装包。

.PARAMETER SkipTag
    不创建/推送 git tag（仅创建 GitHub Release）。

.PARAMETER Draft
    创建为草稿 Release。

.PARAMETER Prerelease
    标记为预发布。

.EXAMPLE
    .\build\release-github.ps1 -Version v0.2.0

.EXAMPLE
    .\build\release-github.ps1 -Version 0.2.0-rc1 -Prerelease
#>
param(
    [Parameter(Mandatory = $true)]
    [string]$Version,
    [switch]$SkipBuild,
    [switch]$SkipTag,
    [switch]$Draft,
    [switch]$Prerelease
)

$ErrorActionPreference = "Stop"
$RepoRoot = Resolve-Path "$PSScriptRoot/.."
Set-Location $RepoRoot

# 规范化版本号：tag 统一带 v 前缀
$tag = if ($Version -match '^v') { $Version } else { "v$Version" }
$ver = $tag -replace '^v', ''

Write-Host ""
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "  Aranea-Agents GitHub Release" -ForegroundColor Cyan
Write-Host "  Tag: $tag" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# ─── 前置检查 ─────────────────────────────────────────────────
if (-not (Get-Command gh -ErrorAction SilentlyContinue)) {
    Write-Error "未找到 gh CLI。请安装: https://cli.github.com/ 并执行 gh auth login"
}
gh auth status 2>&1 | Out-Null
if ($LASTEXITCODE -ne 0) {
    Write-Error "gh 未登录。请执行: gh auth login"
}

$dirty = git status --porcelain
if ($dirty -and -not $SkipBuild) {
    Write-Host "[WARN] git 工作区有未提交改动：" -ForegroundColor Yellow
    git status --short | Select-Object -First 10 | ForEach-Object { Write-Host "  $_" }
    $ans = Read-Host "继续发布？(y/N)"
    if ($ans -ne 'y' -and $ans -ne 'Y') { Write-Error "已取消" }
}

# ─── Step 1: 构建安装包 ───────────────────────────────────────
$installerName = "AraneaAgents-$ver-win-x64.exe"
$installerPath = Join-Path $RepoRoot "build\release\$installerName"

if (-not $SkipBuild) {
    Write-Host "[1/3] 构建安装包..." -ForegroundColor Cyan
    & (Join-Path $RepoRoot "build\build-package.ps1") -Version $ver
    if ($LASTEXITCODE -ne 0) { Write-Error "构建失败" }
} else {
    Write-Host "[1/3] 跳过构建（使用已有安装包）" -ForegroundColor Yellow
}

if (-not (Test-Path $installerPath)) {
    Write-Error "安装包不存在: $installerPath"
}
$sizeMB = [math]::Round((Get-Item $installerPath).Length / 1MB, 1)
Write-Host "  [OK] $installerName ($sizeMB MB)" -ForegroundColor Green

# ─── Step 2: git tag ──────────────────────────────────────────
if (-not $SkipTag) {
    Write-Host "[2/3] 创建并推送 git tag $tag..." -ForegroundColor Cyan
    $existing = git tag -l $tag
    if ($existing) {
        Write-Host "  tag 已存在，跳过创建" -ForegroundColor Yellow
    } else {
        git tag -a $tag -m "Release $tag"
        if ($LASTEXITCODE -ne 0) { Write-Error "git tag 创建失败" }
    }
    git push origin $tag
    if ($LASTEXITCODE -ne 0) { Write-Error "git push tag 失败" }
} else {
    Write-Host "[2/3] 跳过 git tag" -ForegroundColor Yellow
}

# ─── Step 3: GitHub Release ───────────────────────────────────
Write-Host "[3/3] 创建 GitHub Release 并上传安装包..." -ForegroundColor Cyan

$ghArgs = @("release", "create", $tag, $installerPath,
    "--title", "Aranea-Agents $ver",
    "--generate-notes")
if ($Draft) { $ghArgs += "--draft" }
if ($Prerelease) { $ghArgs += "--prerelease" }

# Release 已存在则改为上传覆盖
$null = gh release view $tag 2>&1
if ($LASTEXITCODE -eq 0) {
    Write-Host "  Release $tag 已存在，上传资源（--clobber）..." -ForegroundColor Yellow
    gh release upload $tag $installerPath --clobber
    if ($LASTEXITCODE -ne 0) { Write-Error "gh release upload 失败" }
} else {
    & gh @ghArgs
    if ($LASTEXITCODE -ne 0) { Write-Error "gh release create 失败" }
}

$repoUrl = (gh repo view --json url -q .url)
Write-Host ""
Write-Host "========================================" -ForegroundColor Green
Write-Host "  Release Published!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Green
Write-Host "  Tag:       $tag" -ForegroundColor Green
Write-Host "  Installer: $installerPath" -ForegroundColor Green
Write-Host "  URL:       $repoUrl/releases/tag/$tag" -ForegroundColor Green
Write-Host ""
