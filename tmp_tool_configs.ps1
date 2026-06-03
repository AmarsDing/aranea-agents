$baseUrl = "http://localhost:8000"
$headers = @{"X-User-ID"="dev"}

$tools = (Invoke-RestMethod -Uri "$baseUrl/v1/tools?limit=100" -Headers $headers).items
foreach($t in $tools) {
    Write-Host "=== $($t.id) (key=$($t.key)) ==="
    Write-Host "  source: $($t.source)"
    Write-Host "  configJson: $($t.configJson)"
    Write-Host "  defaultConfigJson: $($t.defaultConfigJson)"
    Write-Host "  metadataJson: $($t.metadataJson)"
    Write-Host ""
}
