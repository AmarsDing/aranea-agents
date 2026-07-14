<#
.SYNOPSIS
    将 aranea.png 转换为 256x256 icon.png + 多尺寸 icon.ico。

.DESCRIPTION
    使用 ImageMagick (magick.exe) 将源 PNG 转换为：
      - icon.png: 256x256 PNG，白色背景填充保持纵横比
      - icon.ico: 多尺寸 ICO（256/128/64/48/32/16），Windows 任务栏/桌面快捷方式友好

    优先使用 ImageMagick；若未安装则报错（fallback 见文末注释）。

.PARAMETER Source
    源 PNG 文件路径（默认：docs/image/aranea.png）

.PARAMETER OutDir
    输出目录（默认：web/src-electron/icons）

.EXAMPLE
    .\scripts\convert-icon.ps1

.NOTES
    ImageMagick 安装：winget install --id ImageMagick.ImageMagick
    若无 ImageMagick，可临时用 System.Drawing + 手写 ICO 文件格式，
    但生成质量较低（详见 git 历史中早期版本）。
#>
param(
    [string]$Source = "$PSScriptRoot\..\docs\image\aranea.png",
    [string]$OutDir = "$PSScriptRoot\..\web\src-electron\icons"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $Source)) {
    throw "Source PNG not found: $Source"
}
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

# ─── 定位 magick.exe ─────────────────────────────────────────────
$magick = (Get-Command magick -ErrorAction SilentlyContinue).Source
if (-not $magick) {
    $candidate = Get-ChildItem "C:\Program Files\ImageMagick*" -Filter "magick.exe" -Recurse -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($candidate) { $magick = $candidate.FullName }
}
if (-not $magick) {
    throw "ImageMagick not found. Install: winget install --id ImageMagick.ImageMagick"
}

$png = Join-Path $OutDir "icon.png"
$ico = Join-Path $OutDir "icon.ico"

Write-Host "[convert-icon] Source: $Source" -ForegroundColor Cyan
Write-Host "[convert-icon] OutDir: $OutDir" -ForegroundColor Cyan
Write-Host "[convert-icon] Magick:  $magick" -ForegroundColor Cyan

# ─── 1. 生成 256x256 icon.png（保持纵横比，白色背景填充）───────────
& $magick $Source -resize 256x256 -background white -gravity center -extent 256x256 $png
if ($LASTEXITCODE -ne 0) { throw "magick PNG conversion failed (exit $LASTEXITCODE)" }
$pngSize = (Get-Item $png).Length
Write-Host "  [OK] icon.png: 256x256 ($pngSize bytes)" -ForegroundColor Green

# ─── 2. 生成多尺寸 icon.ico ──────────────────────────────────────
& $magick $Source -resize 256x256 -background white -gravity center -extent 256x256 `
    -define icon:auto-resize=256,128,64,48,32,16 $ico
if ($LASTEXITCODE -ne 0) { throw "magick ICO conversion failed (exit $LASTEXITCODE)" }
$icoSize = (Get-Item $ico).Length
Write-Host "  [OK] icon.ico: 256/128/64/48/32/16 sizes ($icoSize bytes)" -ForegroundColor Green

Write-Host "[convert-icon] Done" -ForegroundColor Cyan
