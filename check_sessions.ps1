# Check spirit agent sessions
$r = Invoke-RestMethod -Uri 'http://localhost:8000/v1/sessions?agent_id=agent___spirit__&limit=10' -Method Get
Write-Host ("Total: " + $r.total)
$r.items | Sort-Object createdAt -Descending | Select-Object -First 5 | ForEach-Object {
    Write-Host ($_.id + " | " + $_.title + " | created: " + $_.createdAt + " | status: " + $_.status)
}
