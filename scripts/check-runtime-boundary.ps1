param(
  [switch]$Strict
)

$ErrorActionPreference = "Stop"

$root = (Resolve-Path (Join-Path $PSScriptRoot "..")).ProviderPath.TrimEnd('\', '/')
$scanRoots = @("internal", "cmd", "api")

$runtimeImportPatterns = @(
  '"trpc.group/trpc-go/trpc-agent-go',
  '"google.golang.org/adk'
)

# Transitional ADK zones are the current migration surface. New ADK imports
# outside these paths should be treated as runtime-boundary regressions.
$allowedAdkPrefixes = @(
  "internal/agent/",
  "internal/provider/",
  "internal/team/",
  "internal/tools/"
)
$allowedAdkFiles = @(
  "internal/service/adk_turn.go"
)

function Relative-GoPath([string]$fullName) {
  $full = (Resolve-Path $fullName).ProviderPath
  if ($full.StartsWith($root)) {
    $rel = $full.Substring($root.Length).TrimStart('\', '/')
  } else {
    $rel = $full
  }
  return $rel.Replace('\', '/')
}

function Has-AllowedAdkPath([string]$rel) {
  foreach ($file in $allowedAdkFiles) {
    if ($rel -eq $file) {
      return $true
    }
  }
  foreach ($prefix in $allowedAdkPrefixes) {
    if ($rel.StartsWith($prefix)) {
      return $true
    }
  }
  return $false
}

$files = foreach ($sr in $scanRoots) {
  $path = Join-Path $root $sr
  if (Test-Path $path) {
    Get-ChildItem -Path $path -Recurse -File -Filter "*.go"
  }
}

$violations = New-Object System.Collections.Generic.List[string]
$adkImports = New-Object System.Collections.Generic.List[string]
$trpcImports = New-Object System.Collections.Generic.List[string]

foreach ($f in $files) {
  $rel = Relative-GoPath $f.FullName
  $text = Get-Content -Raw -Encoding UTF8 $f.FullName

  $hasRuntimeImport = $false
  foreach ($pat in $runtimeImportPatterns) {
    if ($text.Contains($pat)) {
      $hasRuntimeImport = $true
      break
    }
  }

  if (-not $hasRuntimeImport) {
    continue
  }

  if ($text.Contains('"google.golang.org/adk')) {
    $adkImports.Add($rel) | Out-Null
    if (-not (Has-AllowedAdkPath $rel)) {
      $violations.Add("ADK runtime import outside transitional zones: $rel") | Out-Null
    }
  }

  if ($text.Contains('"trpc.group/trpc-go/trpc-agent-go')) {
    $trpcImports.Add($rel) | Out-Null
  }

  if ($rel.StartsWith("internal/server/")) {
    $violations.Add("runtime import is forbidden in internal/server: $rel") | Out-Null
  }
  if ($rel.StartsWith("internal/biz/")) {
    $violations.Add("runtime import is forbidden in internal/biz: $rel") | Out-Null
  }
}

if ($Strict -and $adkImports.Count -gt 0) {
  foreach ($rel in $adkImports) {
    $violations.Add("strict mode forbids ADK runtime import: $rel") | Out-Null
  }
}

Write-Host "Runtime boundary scan"
Write-Host "  ADK imports:  $($adkImports.Count)"
Write-Host "  TRPC imports: $($trpcImports.Count)"

if ($violations.Count -gt 0) {
  Write-Host ""
  Write-Host "Violations:"
  foreach ($v in $violations) {
    Write-Host "  - $v"
  }
  exit 1
}

Write-Host "  OK"
