# Stage 2: Spirit 运维闭环实跑（告警→根因定位→修复→验证→复盘）
# 用法: powershell -File collect-spirit-run.ps1

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

# ---------- 1. 创建 Spirit 会话 ----------
Write-Output "== create spirit session =="
$session = Invoke-Api Post "/v1/sessions" @{ agent_id = "agent___spirit__"; title = "TS9-零人工运维闭环-订单服务502" }
Save-Evidence "ts9-spirit-session" $session
$sid = $session.id
Write-Output "spirit session: $sid"

# ---------- 2. 提交事故场景 ----------
$incident = @'
[生产事故告警 - P2]
时间: 2026-07-29 18:00 CST
业务: 电商平台订单服务（大促期间）
现象:
1. 18:00:12 监控触发: order-service 网关 502 错误率从 0.1% 飙升至 23%（持续 5 分钟）
2. 18:00:45 数据库告警: orders-db-01 慢查询数量突增，order_items 表出现大量全表扫描，最长查询 47s
3. 18:01:03 主机告警: orders-db-01 磁盘 IO util 98%，订单库连接池耗尽（active=200/200）
4. 18:01:20 应用日志: order-service 大量 "connection wait timeout after 30000ms"
变更背景: 17:55 运维曾上线一个订单查询功能（按用户ID+时间范围查询订单），未走完整评审
当前状态: 502 仍在持续，大促流量峰值 19:00 到来前必须恢复
请组建运维团队完成零人工运维闭环:
(1) 告警分诊与事件定级
(2) 根因定位（日志/指标/变更关联分析）
(3) 修复方案制定与风险评估
(4) 修复执行与恢复验证
(5) 事故复盘报告（时间线/根因/改进项）
'@
Write-Output "== submit incident =="
$ack = Invoke-Api Post "/v1/chat/messages/submit" @{ session_id = $sid; content = $incident }
Save-Evidence "ts9-spirit-submit-ack" $ack
Write-Output "submitted: accepted=$($ack.accepted) status=$($ack.status)"

# ---------- 3. 轮询 run status ----------
$deadline = (Get-Date).AddMinutes(30)
$lastStatus = ""
$pollCount = 0
$finalStatus = $null
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
        Write-Output "!! tool confirm required: $($rs.awaitToolKey) call=$($rs.awaitToolCallId)"
        Save-Evidence "ts9-spirit-await-confirm-$pollCount" $rs
        # 审批通过（运维闭环场景：审批高危命令执行）
        try {
            $confirm = Invoke-Api Post "/v1/chat/activities/$($rs.awaitToolCallId)/confirm" @{ approved = $true }
            Save-Evidence "ts9-spirit-confirm-$pollCount" $confirm
            Write-Output "   confirmed."
        } catch {
            Write-Output "   confirm failed: $($_.Exception.Message)"
        }
    }
    if ($rs.status -in @('completed','failed','cancelled','idle') -and $pollCount -gt 2) {
        $finalStatus = $rs
        break
    }
}
if ($finalStatus) { Save-Evidence "ts9-spirit-run-status-final" $finalStatus; Write-Output "final status: $($finalStatus.status)" }
else { Write-Output "TIMEOUT after $pollCount polls; last=$lastStatus" }

# ---------- 4. 采集证据 ----------
Write-Output "== collect evidence =="
try { $tree = Invoke-Api Get "/v1/sessions/$sid/tree"; Save-Evidence "ts9-spirit-session-tree" $tree } catch { Write-Output "tree: $($_.Exception.Message)" }
try { $acts = Invoke-Api Get "/v1/sessions/$sid/activities"; Save-Evidence "ts9-spirit-activities" $acts; Write-Output "activities: $(@($acts.items).Count)" } catch { Write-Output "activities: $($_.Exception.Message)" }
try { $sess = Invoke-Api Get "/v1/sessions/$sid"; Save-Evidence "ts9-spirit-session-final" $sess } catch { Write-Output "session: $($_.Exception.Message)" }

# 子会话 activities（团队成员执行明细）
if ($tree -and $tree.root -and $tree.root.children) {
    $childIdx = 0
    foreach ($child in $tree.root.children) {
        $childIdx++
        $csid = $child.session.id
        if (-not $csid) { $csid = $child.sessionId }
        if (-not $csid) { continue }
        try {
            $cacts = Invoke-Api Get "/v1/sessions/$csid/activities"
            Save-Evidence "ts9-spirit-child$childIdx-activities" @{ session_id = $csid; title = $child.session.title; items = $cacts.items }
            Write-Output "child[$childIdx] $csid : $(@($cacts.items).Count) activities"
        } catch { Write-Output "child[$childIdx] ${csid}: $($_.Exception.Message)" }
    }
}
Write-Output "== DONE stage 2 =="
