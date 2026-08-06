$token = (Get-Content (Join-Path $PSScriptRoot ".token") -Raw).Trim()
$headers = @{ "Authorization" = "Bearer $token" }
$planId = "tp_622f0975-e4f9-4868-b7df-85e8cb6f8c59"
$detail = Invoke-RestMethod -Uri "http://localhost:8000/v1/chat/plans/$planId`?session_id=ac8cc235-95ee-4646-8ae0-fdbc8dfd089c" -Headers $headers -TimeoutSec 30
$detail | ConvertTo-Json -Depth 20 | Set-Content (Join-Path $PSScriptRoot "ts9v2-gap1-plan-detail.json") -Encoding UTF8
$plan = $detail.plan
if (-not $plan) { $plan = $detail }
$subs = $plan.subTasks
if (-not $subs) { $subs = $plan.sub_tasks }
Write-Output ("subtasks=" + @($subs).Count)
foreach ($s in $subs) {
    $deps = $s.dependsOn
    if (-not $deps) { $deps = $s.depends_on }
    Write-Output (" - " + $s.name + " | depends_on=" + ($deps -join ","))
}
$pm = $subs | Where-Object { $_.id -eq "st_8b27fbc3-0333-4115-baec-bf13ca5befa2" -or ($_.name -match "postmortem") }
if ($pm) { Write-Output "PASS: postmortem node present in plan detail" } else { Write-Output "FAIL: no postmortem node"; exit 1 }
