# IT 运维闭环实跑证据采集脚本
# 用法: powershell -File collect-evidence.ps1
# 前置: admin 服务运行于 localhost:8000，.token 文件已写入登录 token

$ErrorActionPreference = "Stop"
$base = "http://localhost:8000"
$outDir = $PSScriptRoot
$token = (Get-Content (Join-Path $outDir ".token") -Raw).Trim()
$headers = @{ "Authorization" = "Bearer $token"; "Content-Type" = "application/json" }

function Invoke-Api($method, $path, $body = $null) {
    $params = @{ Uri = "$base$path"; Method = $method; Headers = $headers; TimeoutSec = 600 }
    if ($body) { $params.Body = [Text.Encoding]::UTF8.GetBytes(($body | ConvertTo-Json -Depth 10 -Compress)) }
    return Invoke-RestMethod @params
}

function Save-Evidence($name, $data) {
    $path = Join-Path $outDir "$name.json"
    $data | ConvertTo-Json -Depth 20 | Set-Content $path -Encoding UTF8
    Write-Output "[saved] $name.json"
}

# ---------- 阶段 0: it-ops 岗位 Agent 清单证据 ----------
Write-Output "== Stage 0: list it-ops agents =="
$agents = Invoke-Api Get "/v1/agents?limit=400"
$itopsKeys = @('alert_handler__general','incident_commander__general','fault_diagnostician__general','log_analyst__general','metric_analyst__general','change_executor__general','runbook_engineer__general','system_inspector__general','network_inspector__general','db_operator__general','compliance_checker__general','postmortem_writer__general')
$agentItems = @($agents.items)
if (-not $agentItems -and $agents.agents) { $agentItems = @($agents.agents) }
$itops = $agentItems | Where-Object { $itopsKeys -contains $_.agentKey }
Save-Evidence "ts9-itops-agents" @{ total_agents = $agentItems.Count; itops_count = @($itops).Count; items = $itops }
Write-Output "it-ops agents: $(@($itops).Count)/12"

# ---------- 阶段 1: 单 Agent 冒烟（告警处理专家） ----------
Write-Output "== Stage 1: smoke test alert_handler =="
$alertAgent = $itops | Where-Object { $_.agentKey -eq 'alert_handler__general' } | Select-Object -First 1
if (-not $alertAgent) { throw "alert_handler__general not found" }

$smokeSession = Invoke-Api Post "/v1/sessions" @{ agent_id = $alertAgent.id; title = "TS9-冒烟-告警分诊" }
Save-Evidence "ts9-smoke-session" $smokeSession

$smokeAlert = "[监控系统告警 - P3]`n时间: 2026-07-29 17:45:00 CST`n主机: web-frontend-03.prod.internal (10.20.3.15)`n告警项: disk.usage.percent /var/log = 91.2% (阈值 85%)`n关联告警: 同主机 nginx error.log 写入速率异常升高 (过去 15 分钟 +340%)`n历史: 该主机过去 7 天已触发 2 次同类告警，每次清理临时文件后 24 小时内复发`n请完成告警分诊: 评估严重度、判断是否告警风暴前兆、给出处置建议。"
$smokeReply = Invoke-Api Post "/v1/chat/messages" @{ session_id = $smokeSession.id; content = $smokeAlert }
Save-Evidence "ts9-smoke-reply" $smokeReply
$replyText = $smokeReply.agentMessage.content_markdown
Write-Output "smoke reply length: $($replyText.Length)"

Write-Output "== DONE stage 0+1 =="
