param(
    [Parameter(Mandatory = $true, Position = 0)]
    [ValidateSet('api', 'internal')]
    [string]$Subdir
)

# Called from Makefile via: powershell -File scripts/list-proto-files.ps1 api|internal
# Resolves repo root from script location — avoids Makefile / sh escaping issues (Git Bash, MSYS).
$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$scan = Join-Path $repo $Subdir

if (-not (Test-Path -LiteralPath $scan)) {
    [Console]::Write('')
    exit 0
}

$slash = [char]92
$a = @(Get-ChildItem -LiteralPath $scan -Filter '*.proto' -Recurse -File -ErrorAction SilentlyContinue)
$rels = foreach ($f in $a) {
    $full = $f.FullName
    if (-not $full.StartsWith($repo, [System.StringComparison]::OrdinalIgnoreCase)) { continue }
    $rel = $full.Substring($repo.Length).TrimStart($slash).Replace($slash, '/')
    if ($rel.Length -gt 0) { $rel }
}

[Console]::Write(($rels -join ' '))
