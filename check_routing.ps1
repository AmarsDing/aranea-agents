# Check feishu channel routing config
$ch = Invoke-RestMethod -Uri 'http://localhost:8000/v1/channels/754e972d-60ed-42cf-9bc9-6fde40437530' -Method Get
Write-Host "Channel configJson:"
$cfg = $ch.configJson | ConvertFrom-Json
Write-Host ($cfg.routing | ConvertTo-Json -Depth 3)
Write-Host ""
Write-Host "Channel enabled: $($ch.enabled)"
Write-Host "Channel status: $($ch.status)"
