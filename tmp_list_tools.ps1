$tools = (Invoke-RestMethod -Uri 'http://localhost:8000/v1/tools?limit=100' -Headers @{'X-User-ID'='dev'}).items
Write-Host "Total tools: $($tools.Count)"
Write-Host ""
foreach($t in $tools) {
    Write-Host "$($t.id) | key=$($t.key) | source=$($t.source) | risk=$($t.riskLevel) | enabled=$($t.enabled)"
}
