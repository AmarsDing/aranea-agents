$token = (Get-Content (Join-Path $PSScriptRoot ".token") -Raw).Trim()
try {
    $r = Invoke-RestMethod -Uri "http://localhost:8000/v1/admins/current" -Headers @{ Authorization = "Bearer $token" } -TimeoutSec 10
    Write-Output ("OK user=" + $r.name)
} catch {
    Write-Output ("FAIL: " + $_.Exception.Message)
    exit 1
}
