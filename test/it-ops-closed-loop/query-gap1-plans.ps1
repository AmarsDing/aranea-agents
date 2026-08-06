$token = (Get-Content (Join-Path $PSScriptRoot ".token") -Raw).Trim()
$headers = @{ "Authorization" = "Bearer $token" }
$sid = "ac8cc235-95ee-4646-8ae0-fdbc8dfd089c"
try {
    $plans = Invoke-RestMethod -Uri "http://localhost:8000/v1/chat/plans?session_id=$sid" -Headers $headers -TimeoutSec 30
    Write-Output ("count=" + @($plans.items).Count)
    $plans | ConvertTo-Json -Depth 5 | Set-Content (Join-Path $PSScriptRoot "ts9v2-gap1-plans.json") -Encoding UTF8
    foreach ($p in $plans.items) {
        Write-Output ("plan=" + $p.id + " status=" + $p.status + " subs=" + @($p.sub_tasks).Count)
    }
} catch {
    Write-Output ("API FAIL: " + $_.Exception.Message)
    if ($_.ErrorDetails) { Write-Output $_.ErrorDetails.Message }
}
