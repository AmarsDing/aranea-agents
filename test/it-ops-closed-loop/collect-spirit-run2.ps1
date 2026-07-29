# Stage 2b: 提交澄清答案并续跑 Spirit 运维闭环
# 用法: powershell -File collect-spirit-run2.ps1

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
$stepId = "5a48e091-12ca-4a5f-a04e-712149be1430-clarify"

# ---------- 1. 提交澄清答案 ----------
Write-Output "== submit clarification =="
$answers = @(
    @{ selected = @("SQL已记录，索引不存在"); other = "SQL: SELECT * FROM order_items WHERE user_id = ? AND created_at BETWEEN ? AND ? ORDER BY created_at DESC LIMIT 50; 表上仅 PRIMARY KEY 与 order_id 单列索引，无 (user_id, created_at) 复合索引" },
    @{ selected = @("硬限制200，无动态调整", "应用无重试机制"); other = "HikariCP maximumPoolSize=200 硬编码，调整需重启；应用侧无重试/熔断/降级配置" },
    @{ selected = @("优先回滚变更并快速恢复"); other = "大促 19:00 流量峰值前必须恢复，同意回滚 17:55 上线的订单查询功能" }
)
try {
    $resp = Invoke-Api Post "/v1/chat/clarifications/$stepId" @{ session_id = $sid; step_id = $stepId; answers = $answers }
    Save-Evidence "ts9-spirit-clarification" $resp
    Write-Output "clarification accepted=$($resp.accepted) status=$($resp.status)"
} catch {
    Write-Output "clarification failed: $($_.Exception.Message)"
    if ($_.ErrorDetails) { Write-Output $_.ErrorDetails.Message }
    exit 1
}

# ---------- 2. 轮询 run status（处理工具确认） ----------
$deadline = (Get-Date).AddMinutes(40)
$lastStatus = ""
$pollCount = 0
$finalStatus = $null
$confirmedCalls = @{}
while ((Get-Date) -lt $deadline) {
    Start-Sleep -Seconds 15
    $pollCount++
    try {
        $rs = Invoke-Api Get "/v1/chat/run-status?session_id=$sid"
    } catch {
        Write-Output "[$pollCount] run-status poll error: $($_.Exception.Message)"
        continue
    }
    if ($rs.status -ne $lastStatus) {
        Write-Output "[$pollCount] status: $lastStatus -> $($rs.status) (events=$($rs.eventCount) agent=$($rs.agentName) await=$($rs.awaitKind)/$($rs.awaitToolKey))"
        $lastStatus = $rs.status
    } elseif ($pollCount % 4 -eq 0) {
        Write-Output "[$pollCount] still $lastStatus (events=$($rs.eventCount))"
    }
    if ($rs.status -eq 'awaiting_user' -and $rs.awaitKind -eq 'tool_confirm') {
        $callId = $rs.awaitToolCallId
        if (-not $confirmedCalls.ContainsKey($callId)) {
            $confirmedCalls[$callId] = $true
            Write-Output "!! tool confirm required: $($rs.awaitToolKey) call=$callId"
            Save-Evidence "ts9-spirit-await-confirm-$pollCount" $rs
            try {
                $confirm = Invoke-Api Post "/v1/chat/activities/$callId/confirm" @{ session_id = $sid; activity_id = $callId; approved = $true }
                Save-Evidence "ts9-spirit-confirm-$pollCount" $confirm
                Write-Output "   confirmed: $($confirm.status)"
            } catch {
                Write-Output "   confirm failed: $($_.Exception.Message)"
            }
        }
    }
    if ($rs.status -in @('completed','failed','cancelled','idle') -and $pollCount -gt 2) {
        $finalStatus = $rs
        break
    }
}
if ($finalStatus) { Save-Evidence "ts9-spirit-run-status-final" $finalStatus; Write-Output "final status: $($finalStatus.status)" }
else { Write-Output "TIMEOUT after $pollCount polls; last=$lastStatus" }

# ---------- 3. 采集证据 ----------
Write-Output "== collect evidence =="
$tree = $null
try { $tree = Invoke-Api Get "/v1/sessions/$sid/tree"; Save-Evidence "ts9-spirit-session-tree" $tree } catch { Write-Output "tree: $($_.Exception.Message)" }
try { $acts = Invoke-Api Get "/v1/sessions/$sid/activities"; Save-Evidence "ts9-spirit-activities" $acts; Write-Output "activities: $(@($acts.items).Count)" } catch { Write-Output "activities: $($_.Exception.Message)" }
try { $sess = Invoke-Api Get "/v1/sessions/$sid"; Save-Evidence "ts9-spirit-session-final" $sess } catch { Write-Output "session: $($_.Exception.Message)" }
try { $plans = Invoke-Api Get "/v1/chat/plans?session_id=$sid"; Save-Evidence "ts9-spirit-plans" $plans; Write-Output "plans: $(@($plans.items).Count)" } catch { Write-Output "plans: $($_.Exception.Message)" }

# 子会话 activities（团队成员执行明细，递归两层）
function Save-ChildActivities($node, $prefix) {
    $idx = 0
    foreach ($child in $node.children) {
        $idx++
        $csid = $child.session.id
        if (-not $csid) { continue }
        $label = "$prefix$idx"
        try {
            $cacts = Invoke-Api Get "/v1/sessions/$csid/activities"
            Save-Evidence "ts9-spirit-child$label-activities" @{ session_id = $csid; title = $child.session.title; agent_key = $child.session.member_agent_key; items = $cacts.items }
            Write-Output "child[$label] $($child.session.title) ($($child.session.member_agent_key)): $(@($cacts.items).Count) activities"
        } catch { Write-Output "child[$label] ${csid}: $($_.Exception.Message)" }
        if ($child.children) { Save-ChildActivities $child "$label." }
    }
}
if ($tree -and $tree.root) {
    Save-ChildActivities $tree.root ""
}
Write-Output "== DONE stage 2b =="
