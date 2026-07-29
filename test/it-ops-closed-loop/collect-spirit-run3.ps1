# Stage 2c: 计划确认后，监控 DAG 团队执行并采集证据
# 用法: powershell -File collect-spirit-run3.ps1

$ErrorActionPreference = "Stop"
$base = "http://localhost:8000"
$outDir = $PSScriptRoot
$token = (Get-Content (Join-Path $outDir ".token") -Raw).Trim()
$headers = @{ "Authorization" = "Bearer $token"; "Content-Type" = "application/json" }

function Invoke-Api($method, $path, $body = $null) {
    $params = @{ Uri = "$base$path"; Method = $method; Headers = $headers; TimeoutSec = 120 }
    if ($body) { $params.Body = [Text.Encoding]::UTF8.GetBytes(($body | ConvertTo-Json -Depth 10 -Compress)) }
    return Invoke-RestMethod @params
}
function Save-Evidence($name, $data) {
    $path = Join-Path $outDir "$name.json"
    $data | ConvertTo-Json -Depth 30 | Set-Content $path -Encoding UTF8
    Write-Output "[saved] $name.json"
}

$sid = "ec86e351-88fc-4ffd-88d8-0ffce1e8af53"
$deadline = (Get-Date).AddMinutes(45)
$confirmedCalls = @{}
$pollCount = 0
$lastSig = ""

while ((Get-Date) -lt $deadline) {
    Start-Sleep -Seconds 20
    $pollCount++

    # 1. spirit run status + 自动工具审批
    try {
        $rs = Invoke-Api Get "/v1/chat/run-status?session_id=$sid"
        if ($rs.status -eq 'awaiting_user' -and $rs.awaitKind -eq 'tool_confirm') {
            $callId = $rs.awaitToolCallId
            if (-not $confirmedCalls.ContainsKey($callId)) {
                $confirmedCalls[$callId] = $true
                Write-Output "!! spirit tool confirm: $($rs.awaitToolKey) call=$callId"
                Save-Evidence "ts9-await-confirm-$pollCount" $rs
                try {
                    $c = Invoke-Api Post "/v1/chat/activities/$callId/confirm" @{ session_id = $sid; activity_id = $callId; approved = $true }
                    Save-Evidence "ts9-confirm-$pollCount" $c
                    Write-Output "   confirmed: $($c.status)"
                } catch { Write-Output "   confirm failed: $($_.Exception.Message)" }
            }
        }
    } catch { Write-Output "[$pollCount] spirit status err: $($_.Exception.Message)" }

    # 2. 子会话状态签名
    $sig = ""
    $children = $null
    try {
        $children = Invoke-Api Get "/v1/sessions/$sid/children"
        $items = @($children.sessions)
        if (-not $items -or $items.Count -eq 0) { $items = @($children.items) }
        foreach ($c in $items) {
            $sig += "  [$($c.sessionType)] $($c.id.Substring(0,8)) $($c.status) $($c.executionStage) steps=$($c.completedSteps)/$($c.totalSteps) agent=$($c.memberAgentKey)`n"
            # 子会话 run status + 工具审批
            try {
                $crs = Invoke-Api Get "/v1/chat/run-status?session_id=$($c.id)"
                if ($crs.status -eq 'awaiting_user' -and $crs.awaitKind -eq 'tool_confirm') {
                    $cid2 = $crs.awaitToolCallId
                    if (-not $confirmedCalls.ContainsKey($cid2)) {
                        $confirmedCalls[$cid2] = $true
                        Write-Output "!! child $($c.id.Substring(0,8)) tool confirm: $($crs.awaitToolKey)"
                        Save-Evidence "ts9-child-await-$($c.id.Substring(0,8))-$pollCount" $crs
                        try {
                            $c2 = Invoke-Api Post "/v1/chat/activities/$cid2/confirm" @{ session_id = $c.id; activity_id = $cid2; approved = $true }
                            Write-Output "   confirmed: $($c2.status)"
                        } catch { Write-Output "   child confirm failed: $($_.Exception.Message)" }
                    }
                }
            } catch {}
        }
    } catch { $sig = "children err: $($_.Exception.Message)" }

    if ($sig -ne $lastSig) {
        Write-Output "[$pollCount] spirit=$($rs.status) tree:"
        Write-Output $sig
        $lastSig = $sig
    } elseif ($pollCount % 3 -eq 0) {
        Write-Output "[$pollCount] spirit=$($rs.status) (unchanged)"
    }

    # 3. 结束条件：plan 完成
    try {
        $plan = Invoke-Api Get "/v1/chat/plans/tp_d3de45d3-bbd7-4757-ae75-14b477756eef?session_id=$sid"
        $pstatus = $plan.plan.status
        if ($pstatus -in @('completed','failed','abandoned')) {
            Write-Output "PLAN TERMINAL: $pstatus"
            Save-Evidence "ts9-spirit-plan-final" $plan
            break
        }
    } catch {}
}

# ---------- 终态证据 ----------
Write-Output "== collect final evidence =="
try { $rs = Invoke-Api Get "/v1/chat/run-status?session_id=$sid"; Save-Evidence "ts9-final-run-status" $rs; Write-Output "spirit run: $($rs.status)" } catch {}
try { $msgs = Invoke-Api Get "/v1/sessions/$sid/messages?limit=100"; Save-Evidence "ts9-final-messages" $msgs; Write-Output "messages: $(@($msgs.items).Count)" } catch {}
try { $acts = Invoke-Api Get "/v1/sessions/$sid/activities"; Save-Evidence "ts9-final-activities" $acts; Write-Output "activities: $(@($acts.items).Count)" } catch {}
try { $plan = Invoke-Api Get "/v1/chat/plans/tp_d3de45d3-bbd7-4757-ae75-14b477756eef?session_id=$sid"; Save-Evidence "ts9-spirit-plan-final" $plan; Write-Output "plan: $($plan.plan.status)" } catch {}
try {
    $children = Invoke-Api Get "/v1/sessions/$sid/children"
    Save-Evidence "ts9-final-children" $children
    $items = @($children.sessions); if (-not $items) { $items = @($children.items) }
    foreach ($c in $items) {
        $csid = $c.id
        try {
            $cmsgs = Invoke-Api Get "/v1/sessions/$csid/messages?limit=100"
            Save-Evidence "ts9-child-$($csid.Substring(0,8))-messages" @{ session_id = $csid; agent_key = $c.memberAgentKey; session_type = $c.sessionType; status = $c.status; items = $cmsgs.items }
            Write-Output "child $($csid.Substring(0,8)) [$($c.sessionType)/$($c.memberAgentKey)] $($c.status): $(@($cmsgs.items).Count) msgs"
        } catch { Write-Output "child $csid msgs err: $($_.Exception.Message)" }
        try {
            $cacts = Invoke-Api Get "/v1/sessions/$csid/activities"
            Save-Evidence "ts9-child-$($csid.Substring(0,8))-activities" @{ session_id = $csid; agent_key = $c.memberAgentKey; items = $cacts.items }
        } catch {}
    }
} catch { Write-Output "children err: $($_.Exception.Message)" }
Write-Output "== DONE stage 2c =="
