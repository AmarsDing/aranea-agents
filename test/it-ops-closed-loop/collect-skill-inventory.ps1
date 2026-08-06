# D3-a: collect skill inventory evidence (count + alibaba skills presence)
$ErrorActionPreference = "Stop"
$outDir = $PSScriptRoot
$token = (Get-Content (Join-Path $outDir ".token") -Raw).Trim()
$headers = @{ "Authorization" = "Bearer $token" }

$all = @()
$page = 1
while ($true) {
    $resp = Invoke-RestMethod -Uri "http://localhost:8000/v1/skills?page=$page&page_size=200" -Headers $headers -TimeoutSec 60
    $items = @($resp.items)
    $all += $items
    if ($items.Count -lt 200) { break }
    $page++
}
Write-Output ("total skills: " + $all.Count)
$ali = $all | Where-Object { $_.name -match "alibaba|aliyun" -or $_.display_name -match "阿里" }
Write-Output ("alibaba skills: " + @($ali).Count)
foreach ($s in $ali) { Write-Output (" - " + $s.name + " | " + $s.display_name) }
@{
    collected_at = (Get-Date).ToUniversalTime().ToString("o")
    total = $all.Count
    alibaba_count = @($ali).Count
    alibaba_skills = @($ali | ForEach-Object { @{ name = $_.name; display_name = $_.display_name; source = $_.source } })
    names = @($all | ForEach-Object { $_.name })
} | ConvertTo-Json -Depth 6 | Set-Content (Join-Path $outDir "d3-skill-inventory.json") -Encoding UTF8
Write-Output "[saved] d3-skill-inventory.json"
