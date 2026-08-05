$targets = @(
    'F:\aranea-agents\test\gocache-review',
    'F:\aranea-agents\test\gocache-w2b',
    'F:\aranea-agents\test\.gocache-fixA',
    'F:\aranea-agents\test\gocache-canary',
    'F:\aranea-agents\test\.gocache-v3',
    'F:\aranea-agents\test\gocache-si-apply',
    'F:\aranea-agents\.gocache-t43',
    'F:\aranea-agents\.gocache-wire',
    'F:\aranea-agents\.gocache-t42'
)
foreach ($t in $targets) {
    if (Test-Path $t) {
        Remove-Item $t -Recurse -Force -ErrorAction SilentlyContinue
        Write-Output "removed: $t"
    }
}
Write-Output ("free now: {0:N2} GB" -f ((Get-PSDrive F).Free/1GB))
