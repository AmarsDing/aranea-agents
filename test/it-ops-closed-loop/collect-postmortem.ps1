# Stage 3: postmortem_writer 单 Agent 冒烟（补齐「复盘」环节证据）
# 用法: powershell -File collect-postmortem.ps1

$ErrorActionPreference = "Stop"
$base = "http://localhost:8000"
$outDir = $PSScriptRoot
$token = (Get-Content (Join-Path $outDir ".token") -Raw).Trim()
$headers = @{ "Authorization" = "Bearer $token"; "Content-Type" = "application/json" }

function Invoke-Api($method, $path, $body = $null) {
    $params = @{ Uri = "$base$path"; Method = $method; Headers = $headers; TimeoutSec = 180 }
    if ($body) { $params.Body = [Text.Encoding]::UTF8.GetBytes(($body | ConvertTo-Json -Depth 10 -Compress)) }
    return Invoke-RestMethod @params
}
function Save-Evidence($name, $data) {
    $path = Join-Path $outDir "$name.json"
    $data | ConvertTo-Json -Depth 30 | Set-Content $path -Encoding UTF8
    Write-Output "[saved] $name.json"
}

# ---------- 1. 找 postmortem_writer agent ----------
$agents = Invoke-Api Get "/v1/agents?limit=400"
$items = @($agents.items); if (-not $items) { $items = @($agents.agents) }
$pm = $items | Where-Object { $_.agentKey -eq 'postmortem_writer__general' } | Select-Object -First 1
if (-not $pm) { throw "postmortem_writer__general not found" }
Write-Output "agent: $($pm.id) $($pm.name)"

# ---------- 2. 创建会话并提交复盘输入 ----------
$session = Invoke-Api Post "/v1/sessions" @{ agent_id = $pm.id; title = "TS9-冒烟-事故复盘报告" }
Save-Evidence "ts9-postmortem-session" $session
$sid = $session.id

$input = @'
请基于以下已完成的故障处置过程，输出标准《事故复盘报告》（时间线/根因/处置评估/改进项）。

## 故障信息
- 故障编号: 2026-07-29-ORDER-001
- 时间: 2026-07-29 18:00 CST 告警触发，18:25 完全恢复（历时 30 分钟）
- 业务: 电商平台订单服务（大促期间）
- 定级: P2（502 错误率 23%、慢查询 47s、IO util 98%、HikariCP 连接池 200/200 耗尽）

## 根因（根因定位团队结论）
- 直接根因(95%): 17:55 未评审上线订单查询功能，SQL 缺 (user_id, created_at) 复合索引 → 全表扫描慢查询(47s)
- 间接根因(90%): 变更未经评审直接上线，SQL Review 环节缺失
- 系统根因(85%): HikariCP maximumPoolSize=200 硬编码、无重试/熔断/降级，慢查询影响被放大至全局
- 故障链: 未评审变更 → 缺索引全表扫描 → 慢查询积压 → 连接池耗尽 → 请求超时 → 502

## 处置过程（恢复执行团队记录）
- Step1 回滚: order-api v1.2.9→v1.2.8，3min20s，健康检查通过
- Step2 索引: 表 874 万行，pt-online-schema-change 在线创建 idx_user_created(user_id, created_at DESC)，128s 完成无锁表；执行计划 type=ref rows=12
- Step3 验证: 502 错误率 23%→0.03%（阈值<0.1%）、慢查询 187→2/min、HikariCP active 198→85、订单查询 QPS 45→320，全部达标
- 残留风险: v1.2.9 未评审上线原因待查、SQL Review 门禁缺失、pt-osc 额外占用 280MB 磁盘

## 执行模式
全自动化多 Agent 团队（告警分诊→根因定位→修复方案→恢复执行 4 团队 DAG 顺序执行），人工仅做 2 次审批（澄清确认 + 高危索引操作审批）
'@
$reply = Invoke-Api Post "/v1/chat/messages" @{ session_id = $sid; content = $input }
Save-Evidence "ts9-postmortem-reply" $reply
$len = $reply.agentMessage.content_markdown.Length
Write-Output "postmortem reply length: $len"
Write-Output $reply.agentMessage.content_markdown.Substring(0, [Math]::Min(500, $len))
Write-Output "== DONE stage 3 =="
