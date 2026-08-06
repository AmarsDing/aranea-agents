# TS9-GAP-1 回归验证：闭环事故消息 → plan DAG 必须包含引擎自动追加的「事故复盘」节点
# 用法: powershell -File verify-postmortem-node.ps1
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
$session = Invoke-Api Post "/v1/sessions" @{ agent_id = "agent___spirit__"; title = "TS9-GAP1-复盘节点自动追加验证" }
$sid = $session.id
Write-Output "spirit session: $sid"

# ---------- 2. 提交事故场景（与 TS-9 相同输入） ----------
$incident = @'
[生产事故告警 - P2]
时间: 2026-08-06 18:30 CST
业务: 电商平台订单服务（大促期间）
现象:
1. 18:30:12 监控触发: order-service 网关 502 错误率从 0.1% 飙升至 23%（持续 5 分钟）
2. 18:30:45 数据库告警: orders-db-01 慢查询数量突增，order_items 表出现大量全表扫描，最长查询 47s
3. 18:31:03 主机告警: orders-db-01 磁盘 IO util 98%，订单库连接池耗尽（active=200/200）
变更背景: 17:55 运维曾上线一个订单查询功能，未走完整评审
当前状态: 502 仍在持续，大促流量峰值 19:00 到来前必须恢复
请组建运维团队完成零人工运维闭环:
(1) 告警分诊与事件定级
(2) 根因定位
(3) 修复方案制定与风险评估
(4) 修复执行与恢复验证
'@
$ack = Invoke-Api Post "/v1/chat/messages/submit" @{ session_id = $sid; content = $incident }
Write-Output "submitted: accepted=$($ack.accepted) status=$($ack.status)"

# ---------- 3. 轮询至 plan draft（澄清则自动回答） ----------
$deadline = (Get-Date).AddMinutes(8)
$lastStatus = ""
$planFound = $false
while ((Get-Date) -lt $deadline) {
    Start-Sleep -Seconds 10
    try { $rs = Invoke-Api Get "/v1/chat/run-status?session_id=$sid" } catch { Write-Output "poll err: $($_.Exception.Message)"; continue }
    if ($rs.status -ne $lastStatus -or "$($rs.awaitKind)" -ne "") {
        Write-Output "status=$($rs.status) await=$($rs.awaitKind)/$($rs.awaitToolKey) call=$($rs.awaitToolCallId)"
        $lastStatus = $rs.status
    }
    if ($rs.status -eq 'awaiting_user' -and $rs.awaitKind -eq 'clarification') {
        $stepId = $rs.awaitToolCallId
        Write-Output "-> answering clarification step $stepId"
        $answers = @(
            @{ selected = @("SQL已记录，索引不存在"); other = "无 (user_id, created_at) 复合索引" },
            @{ selected = @("硬限制200，无动态调整"); other = "HikariCP maximumPoolSize=200 硬编码" },
            @{ selected = @("优先回滚变更并快速恢复"); other = "同意回滚 17:55 上线的功能" }
        )
        try {
            $resp = Invoke-Api Post "/v1/chat/clarifications/$stepId" @{ session_id = $sid; step_id = $stepId; answers = $answers }
            Write-Output "clarification accepted=$($resp.accepted)"
        } catch { Write-Output "clarification failed: $($_.Exception.Message)" }
    }
    if ($rs.status -eq 'awaiting_user' -and ($rs.awaitKind -match 'plan' -or $rs.awaitKind -eq 'tool_confirm')) {
        Write-Output "-> plan gate reached (awaitKind=$($rs.awaitKind))"
        $planFound = $true
        break
    }
    # 兜底：直接查 plans 看是否已产出 draft
    try {
        $plans = Invoke-Api Get "/v1/chat/plans?session_id=$sid"
        if ($plans.items -and @($plans.items).Count -gt 0) { $planFound = $true; break }
    } catch {}
}
if (-not $planFound) { Write-Output "TIMEOUT: no plan produced"; exit 1 }

# ---------- 4. 校验 plan DAG 含复盘节点 ----------
Start-Sleep -Seconds 3
$plans = Invoke-Api Get "/v1/chat/plans?session_id=$sid"
Save-Evidence "ts9v2-gap1-plans" @{ session_id = $sid; plans = $plans }
$plan = @($plans.items)[0]
$subs = $plan.sub_tasks
if (-not $subs) { $subs = $plan.subTasks }
Write-Output ("subtasks: " + ($subs | ForEach-Object { $_.name }) -join " | ")
$pm = $subs | Where-Object { $_.name -match "复盘|postmortem" }
if (-not $pm) { Write-Output "FAIL: no postmortem node in plan DAG"; exit 1 }
Write-Output "PASS: postmortem node found: $($pm.name) depends_on=$($pm.depends_on -join ',')"
Write-Output "== DONE =="
