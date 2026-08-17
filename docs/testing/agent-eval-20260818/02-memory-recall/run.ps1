﻿# 域 B 记忆召回评测（指令驱动，真实 LLM）
# 用法：powershell -ExecutionPolicy Bypass -File run.ps1 [-Pilot] [-SkipPlant]
param([switch]$Pilot, [switch]$SkipPlant)
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "02"
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null
Renew-Token | Out-Null

$agentKey = "eval_memory_probe"
$qa = Get-Content (Join-Path $PSScriptRoot "sample-memory-qa.json") -Raw -Encoding UTF8 | ConvertFrom-Json
$cases = @($qa.cases)
if ($Pilot) { $cases = $cases | Select-Object -First 3 }

# 拒答判定的植入词表（全部 case 的 expected_keywords 汇总，另补关键实体）
$plantedTerms = @("张伟","李明","王芳","小赵","刘洋","CE6857","SW-Core-01","FW-Edge-02","SRV-DB-03","NAS-Backup-01","UPS-Main-01","SW-Core-Spare","10.20.0.2","0571-8899-1234","0571-6655-0001","PostgreSQL","MySQL","Jira","禅道","ELK","elk-01","CN2","2Gbps","800Mbps","行政部")
$abstainWords = @("没有记录","不知道","没有告诉","无法确认","不清楚","不了解","没有提到","未曾","不记得","没有相关","无法得知","没有收到")

# ---- B-00 探针 agent ----
# 注意：GET /v1/agents/{id} 只认 UUID，按 key 查需走列表端点过滤；列表分页参数是 limit/offset（page_size 会被静默忽略，默认仅 24 条）。
$rList = Api-Get "/v1/agents?limit=500"
$existing = @($rList.Body.items) | Where-Object { $_.agentKey -eq $agentKey } | Select-Object -First 1
if (-not $existing) {
    # 注意：开关必须走 settings 子对象（CreateAgentRequest.settings），顶层字段会被静默忽略。
    # clarification_enabled 不在 proto 中，需建后 DB 补丁（见 README 或评测记录）。
    $create = @{ agent_key = $agentKey; display_name = "评测-记忆探针"; provider = "deepseek"; model = "deepseek-v4-flash"; agent_description = "agent-eval-20260818 域B专用，测后清理"; settings = @{ intent_pass_enabled = $false } }
    $r = Api-Post "/v1/agents" $create -OutFile (Join-Path $ev "b00-create-agent.json")
    Record $M "B-00" "创建 eval agent" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
    $agentId = $r.Body.id
} else {
    Record $M "B-00" "eval agent 已存在复用" "PASS" "exists" 0
    $agentId = $existing.id
}

# ---- B07 植入前 prompt 基线（preview 端点只认 agent UUID，不认 agent_key）----
$rPre = Api-Get "/v1/agents/$agentId/system-prompt/preview" -OutFile (Join-Path $ev "b07-prompt-before.json")
Record $M "B07-pre" "prompt 基线" ($(if ($rPre.Code -eq "200") { "PASS" } else { "FAIL" })) "len=$($rPre.Raw.Length)" $rPre.Ms

# ---- 植入阶段 ----
$plantSid = $null
$plantMs = @()
if (-not $SkipPlant) {
    $r = Api-Post "/v1/sessions" @{ agent_id = $agentId; title = "eval-记忆植入-20260818"; owner_type = "agent" }
    $plantSid = $r.Body.id
    [IO.File]::WriteAllText((Join-Path $ev "plant-session.txt"), "$plantSid", [Text.UTF8Encoding]::new($false))
    $seq = 0
    foreach ($c in $cases) {
        foreach ($pm in @($c.plant_messages)) {
            if (-not $pm) { continue }
            $seq++
            $pr = Api-Post "/v1/chat/messages" @{ session_id = $plantSid; agent_key = $agentKey; content = $pm } -OutFile (Join-Path $ev "$($c.id)-plant-$seq.json") -TimeoutSec 180
            $plantMs += $pr.Ms
            $hasReply = ($null -ne $pr.Body.agentMessage)
            Record $M "PLANT-$seq" "植入 $($c.id)" ($(if ($pr.Code -eq "200" -and $hasReply) { "PASS" } else { "FAIL" })) "code=$($pr.Code)" $pr.Ms
        }
    }
    Write-Host "植入完成 $seq 条，等待 90s 异步提取落库..."
    Start-Sleep -Seconds 90
} else {
    $plantSid = (Get-Content (Join-Path $ev "plant-session.txt") -Raw).Trim()
}

# ---- A01/A02 佐证：facts 落库抽查 ----
$rFacts = Api-Get "/v1/memory/l3/facts?page_size=200" -OutFile (Join-Path $ev "b-facts-after-plant.json")
$factHit = 0
if ($rFacts.Body.items) {
    $factText = ($rFacts.Raw)
    foreach ($t in $plantedTerms) { if ($factText.Contains($t)) { $factHit++ } }
}
Record $M "A01-facts" "植入事实落库抽查" ($(if ($rFacts.Code -eq "200" -and $factHit -gt 0) { "PASS" } else { "REVIEW" })) "命中植入词 $factHit/$($plantedTerms.Count)" $rFacts.Ms

# ---- B02 召回段延迟（纯召回路径，不经 LLM）----
$recallMs = @()
foreach ($c in ($cases | Select-Object -First 10)) {
    $rd = Api-Post "/v1/memory/recall/debug" @{ query = $c.question; session_id = $plantSid; agent_id = $agentId } -OutFile (Join-Path $ev "$($c.id)-recall-debug.json")
    $recallMs += $rd.Ms
    Record $M "B02-$($c.id)" "召回段" ($(if ($rd.Code -eq "200") { "PASS" } else { "FAIL" })) "recall_debug" $rd.Ms
}

# ---- 提问阶段（跨会话：每 case 独立新会话）----
$stat = @{}
foreach ($c in $cases) {
    $rs = Api-Post "/v1/sessions" @{ agent_id = $agentId; title = "eval-提问-$($c.id)"; owner_type = "agent" }
    $qsid = $rs.Body.id
    $ra = Api-Post "/v1/chat/messages" @{ session_id = $qsid; agent_key = $agentKey; content = $c.question } -OutFile (Join-Path $ev "$($c.id)-ask.json") -TimeoutSec 180
    $answerText = $ra.Raw
    $result = "REVIEW"; $note = ""
    if ($ra.Code -ne "200") { $result = "FAIL"; $note = "http $($ra.Code)" }
    elseif ($c.grading -eq "abstain") {
        $hasRefuse = $false; foreach ($w in $abstainWords) { if ($answerText.Contains($w)) { $hasRefuse = $true; break } }
        $hasPlanted = $false; foreach ($t in $plantedTerms) { if ($answerText.Contains($t)) { $hasPlanted = $true; $note = "编造词:$t"; break } }
        if ($hasPlanted) { $result = "FAIL" } elseif ($hasRefuse) { $result = "PASS" } else { $result = "REVIEW" }
    } else {
        $kws = @($c.expected_keywords)
        $hit = 0; foreach ($k in $kws) { if ($answerText.Contains($k)) { $hit++ } }
        if ($c.grading -eq "keywords_any") { $result = $(if ($hit -ge 1) { "PASS" } else { "FAIL" }) }
        else { $result = $(if ($hit -eq $kws.Count) { "PASS" } else { "FAIL" }) }
        $note = "kw $hit/$($kws.Count)"
    }
    if (-not $stat.ContainsKey($c.category)) { $stat[$c.category] = @{ pass = 0; fail = 0; review = 0 } }
    if ($result -eq "PASS") { $stat[$c.category].pass++ } elseif ($result -eq "FAIL") { $stat[$c.category].fail++ } else { $stat[$c.category].review++ }
    Record $M "ASK-$($c.id)" "提问($($c.category))" $result $note $ra.Ms
}

# ---- B07 植入后 prompt ----
$rPost = Api-Get "/v1/agents/$agentId/system-prompt/preview" -OutFile (Join-Path $ev "b07-prompt-after.json")
Record $M "B07-post" "prompt 植入后" ($(if ($rPost.Code -eq "200") { "PASS" } else { "FAIL" })) "len=$($rPost.Raw.Length) before=$($rPre.Raw.Length)" $rPost.Ms

# ---- 汇总 result.md ----
$recallSorted = $recallMs | Sort-Object
$p95Idx = [Math]::Min([Math]::Ceiling($recallSorted.Count * 0.95) - 1, $recallSorted.Count - 1)
$recallP95 = 0; if ($recallSorted.Count -gt 0) { $recallP95 = $recallSorted[$p95Idx] }
$lines = @()
$lines += "# 域 B 记忆召回评测结果（$(Get-Date -Format 'yyyy-MM-dd HH:mm')）"
$lines += ""
$lines += "Pilot=$Pilot SkipPlant=$SkipPlant；植入消息数=$($plantMs.Count)；植入会话=$plantSid"
$lines += ""
$lines += "## 准确率（按类别）"
$lines += "| 类别 | PASS | FAIL | REVIEW | 准确率(不含REVIEW) |"
$lines += "|------|------|------|--------|------|"
foreach ($cat in $stat.Keys) {
    $s = $stat[$cat]; $den = $s.pass + $s.fail
    $acc = "-"; if ($den -gt 0) { $acc = "{0:P0}" -f ($s.pass / $den) }
    $lines += "| $cat | $($s.pass) | $($s.fail) | $($s.review) | $acc |"
}
$lines += ""
$lines += "## 召回段延迟（recall/debug，不经 LLM，n=$($recallMs.Count)）"
if ($recallMs.Count -gt 0) {
    $lines += "- min=$($recallSorted[0])ms max=$($recallSorted[-1])ms P95=${recallP95}ms"
    $lines += "- 目标 <500ms；业界参考 Mem0 549ms / Zep ~200ms"
}
$lines += ""
$lines += "## B07 prompt 体积"
$lines += "- 植入前 $($rPre.Raw.Length) 字符 → 植入后 $($rPost.Raw.Length) 字符"
[IO.File]::WriteAllText((Join-Path $PSScriptRoot "result.md"), ($lines -join "`n"), [Text.UTF8Encoding]::new($false))
Write-Host "=== 完成，结果见 result.md ==="
