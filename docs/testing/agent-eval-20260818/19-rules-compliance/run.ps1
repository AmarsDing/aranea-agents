# G1 规则一致性评测（Leaderboard G 维，指令驱动，真实 LLM）
# 用法：powershell -ExecutionPolicy Bypass -File run.ps1 [-Pilot] [-SkipPlant]
param([switch]$Pilot, [switch]$SkipPlant)
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "19"
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null
Renew-Token | Out-Null

$agentKey = "eval_rules_probe"
$qa = Get-Content (Join-Path $PSScriptRoot "sample-rules-compliance.json") -Raw -Encoding UTF8 | ConvertFrom-Json
$probes = @($qa.probes)
if ($Pilot) { $probes = $probes | Select-Object -First 3 }
$bannedWords = @("毫无疑问")
$tailMarker = "以上由网络运维助手提供"
$ruleTerms = @("表格", "毫无疑问", "尾注", "影响面")

# ---- G-00 探针 agent（记忆必须开启：钉住注入门控 = memoryEnabled && l3Enabled && l0InjectL3）----
$memSettings = @{ memory_enabled = $true; l3_enabled = $true; l0_inject_l3 = $true }
$rList = Api-Get "/v1/agents?limit=500"
$existing = @($rList.Body.items) | Where-Object { $_.agentKey -eq $agentKey } | Select-Object -First 1
if (-not $existing) {
    # 注意：开关必须走 settings 子对象（CreateAgentRequest.settings），顶层字段会被静默忽略。
    $create = @{ agent_key = $agentKey; display_name = "评测-规则探针"; provider = "deepseek"; model = "deepseek-v4-flash"; agent_description = "agent-eval-20260818 G1规则一致性专用，测后清理"; settings = (@{ intent_pass_enabled = $false } + $memSettings) }
    $r = Api-Post "/v1/agents" $create -OutFile (Join-Path $ev "g00-create-agent.json")
    Record $M "G-00" "创建 eval agent" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
    $agentId = $r.Body.id
} else {
    $agentId = $existing.id
    # 复用路径：幂等确保记忆开关（2026-08-18 踩坑：新建 agent 默认 memoryEnabled=false，
    # 钉住注入被门控 → G1 全灭；UpdateAgent body:"agent" 要求 body 即 Agent 本体，不可再包 {"agent":{}}）。
    $cur = (Api-Get "/v1/agents/$agentId").Body.settings
    if (-not ($cur.memoryEnabled -and $cur.l3Enabled -and $cur.l0InjectL3)) {
        $cur.memoryEnabled = $true; $cur.l3Enabled = $true; $cur.l0InjectL3 = $true
        $patchBody = @{ settings = $cur } | ConvertTo-Json -Depth 20 -Compress
        $rp = Api-Call -Method PATCH -Path "/v1/agents/$agentId" -Body $patchBody
        Record $M "G-00" "eval agent 记忆开关补齐" ($(if ($rp.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($rp.Code)" $rp.Ms
    } else {
        Record $M "G-00" "eval agent 已存在复用（记忆已开启）" "PASS" "exists" 0
    }
}

# ---- 植入阶段（4 条 standing rule，同一会话按序发送）----
$plantSid = $null
$plantMs = @()
if (-not $SkipPlant) {
    $r = Api-Post "/v1/sessions" @{ agent_id = $agentId; title = "eval-规则植入-20260818"; owner_type = "agent" }
    $plantSid = $r.Body.id
    [IO.File]::WriteAllText((Join-Path $ev "plant-session.txt"), "$plantSid", [Text.UTF8Encoding]::new($false))
    $seq = 0
    foreach ($pm in @($qa.plant_messages)) {
        $seq++
        $pr = Api-Post "/v1/chat/messages" @{ session_id = $plantSid; agent_key = $agentKey; content = $pm } -OutFile (Join-Path $ev "g1-plant-$seq.json") -TimeoutSec 180
        $plantMs += $pr.Ms
        $hasReply = ($null -ne $pr.Body.agentMessage)
        Record $M "G1-PLANT-$seq" "植入规则 R$seq" ($(if ($pr.Code -eq "200" -and $hasReply) { "PASS" } else { "FAIL" })) "code=$($pr.Code)" $pr.Ms
    }
    Write-Host "植入完成 $seq 条，等待 90s 异步提取落库..."
    Start-Sleep -Seconds 90
} else {
    $plantSid = (Get-Content (Join-Path $ev "plant-session.txt") -Raw).Trim()
}

# ---- G-FACTS 规则事实落库抽查（按 agent_id 过滤，避免全量分页把规则事实挤出首页的假象）----
# 注意：ListMemoryFacts 分页契约是 limit/offset（memory.go ListMemoryFacts），
# page_size 不生效会静默回退默认 20 条（2026-08-18 run4 G-PIN 误判根因）。
$rFacts = Api-Get "/v1/memory/l3/facts?limit=200&agent_id=$agentId" -OutFile (Join-Path $ev "g-facts-after-plant.json")
$factHit = 0
if ($rFacts.Body.items) {
    foreach ($t in $ruleTerms) { if ($rFacts.Raw.Contains($t)) { $factHit++ } }
}
Record $M "G-FACTS" "规则事实落库抽查" ($(if ($rFacts.Code -eq "200" -and $factHit -ge 2) { "PASS" } else { "REVIEW" })) "命中规则词 $factHit/$($ruleTerms.Count)" $rFacts.Ms

# ---- 探针阶段（跨会话：每题独立新会话）----
$ruleStat = @{
    table   = @{ app = 0; pass = 0 }
    banned  = @{ app = 0; pass = 0 }
    tailnote = @{ app = 0; pass = 0 }
    confirm = @{ app = 0; pass = 0 }
}
$probePass = 0; $probeFail = 0; $probeReview = 0
foreach ($c in $probes) {
    $rs = Api-Post "/v1/sessions" @{ agent_id = $agentId; title = "eval-规则探针-$($c.id)"; owner_type = "agent" }
    $qsid = $rs.Body.id
    $ra = Api-Post "/v1/chat/messages" @{ session_id = $qsid; agent_key = $agentKey; content = $c.question } -OutFile (Join-Path $ev "$($c.id)-ask.json") -TimeoutSec 180
    # 判分只看最终答复正文（content_markdown）。Raw 含思考链（reasoning），模型在思考中
    # 引用规则原文（如禁令词「毫无疑问」）会造成 R2 误判（2026-08-18 run2 全灭根因）。
    $ans = $ra.Raw
    if ($ra.Body -and $ra.Body.agentMessage -and $ra.Body.agentMessage.content_markdown) {
        $ans = [string]$ra.Body.agentMessage.content_markdown
    }
    $result = "REVIEW"; $fmtFail = @()
    if ($ra.Code -ne "200") {
        $result = "FAIL"; $fmtFail += "http $($ra.Code)"
    } else {
        # R1 表格
        if ($c.expect.table) {
            $ruleStat.table.app++
            if ($ans.Contains("|") -and $ans.Contains("项目") -and $ans.Contains("结果")) { $ruleStat.table.pass++ } else { $fmtFail += "table" }
        }
        # R2 禁令
        if ($c.expect.no_banned) {
            $ruleStat.banned.app++
            $violated = $false
            foreach ($w in $bannedWords) { if ($ans.Contains($w)) { $violated = $true; $fmtFail += "banned:$w"; break } }
            if (-not $violated) { $ruleStat.banned.pass++ }
        }
        # R3 尾注
        if ($c.expect.tailnote) {
            $ruleStat.tailnote.app++
            if ($ans.Contains($tailMarker)) { $ruleStat.tailnote.pass++ } else { $fmtFail += "tailnote" }
        }
        # R4 流程确认
        if ($c.expect.confirm_flow) {
            $ruleStat.confirm.app++
            if ($ans.Contains("影响") -and $ans.Contains("确认")) { $ruleStat.confirm.pass++ } else { $fmtFail += "confirm" }
        }
        # 关键词轨
        $kwHit = 0
        foreach ($k in @($c.expected_keywords)) { if ($ans.Contains($k)) { $kwHit++ } }
        if ($fmtFail.Count -eq 0 -and $kwHit -ge 1) { $result = "PASS" }
        elseif ($fmtFail.Count -eq 0) { $result = "REVIEW" }
        else { $result = "FAIL" }
    }
    $note = "kw $kwHit/$(@($c.expected_keywords).Count)"
    if ($fmtFail.Count -gt 0) { $note += " fmt_fail:" + ($fmtFail -join ",") }
    if ($result -eq "PASS") { $probePass++ } elseif ($result -eq "FAIL") { $probeFail++ } else { $probeReview++ }
    Record $M "ASK-$($c.id)" "探针" $result $note $ra.Ms
}

# ---- G-PIN 钉住注入验证（P1 核心证据：规则事实 injectedCount>0）----
# 注意：/system-prompt/preview 只渲染静态 prompt（BuildPreviewReport），不含运行时记忆块，
# 不能用于钉住验证。钉住块经 before-model hook 注入并对命中事实递增 injected_count
# （memory_inject.go FR-12.6），故以探针轮后 injectedCount>0 为钉住生效的直接证据。
$rPin = Api-Get "/v1/memory/l3/facts?limit=200&agent_id=$agentId" -OutFile (Join-Path $ev "g-pin-injected-count.json")
$pinPass = 0; $pinDetail = @()
foreach ($t in $ruleTerms) {
    # 同一规则词可能命中多条事实（多次植入产生的 0.6 重复提取 + 0.8 显式规则）。
    # 钉住语义 = 任一承载该规则的事实被注入，故取命中集 injectedCount 最大值，
    # 而非 First 1（2026-08-18 run3 误判：首条命中是未入钉住 top10 的重复提取行）。
    $pinMax = 0
    foreach ($h in @($rPin.Body.items) | Where-Object { $_.agentId -eq $agentId -and $_.statement.Contains($t) }) {
        if ($h.injectedCount -gt $pinMax) { $pinMax = $h.injectedCount }
    }
    if ($pinMax -gt 0) { $pinPass++; $pinDetail += "$t=$pinMax" } else { $pinDetail += "$t=0" }
}
Record $M "G-PIN" "钉住注入验证(injectedCount)" ($(if ($pinPass -eq $ruleTerms.Count) { "PASS" } else { "FAIL" })) ($pinDetail -join " ") $rPin.Ms

# ---- 汇总 result.md ----
$lines = @()
$lines += "# G1 规则一致性评测结果（$(Get-Date -Format 'yyyy-MM-dd HH:mm')）"
$lines += ""
$lines += "Pilot=$Pilot SkipPlant=$SkipPlant；植入会话=$plantSid；探针数=$($probes.Count)"
$lines += ""
$lines += "## 分规则合规率"
$lines += "| 规则 | 适用题数 | 通过题数 | 合规率 |"
$lines += "|------|---------|---------|--------|"
$ruleNames = @{ table = "R1 表格格式"; banned = "R2 禁令词"; tailnote = "R3 固定尾注"; confirm = "R4 变更确认流程" }
foreach ($k in @("table", "banned", "tailnote", "confirm")) {
    $s = $ruleStat[$k]
    $rate = "-"; if ($s.app -gt 0) { $rate = "{0:P0}" -f ($s.pass / $s.app) }
    $lines += "| $($ruleNames[$k]) | 适用 $($s.app) | $($s.pass) | $rate |"
}
$lines += ""
$lines += "## 探针判定"
$total = $probePass + $probeFail + $probeReview
$acc = "-"; $den = $probePass + $probeFail
if ($den -gt 0) { $acc = "{0:P0}" -f ($probePass / $den) }
$lines += "- PASS=$probePass FAIL=$probeFail REVIEW=$probeReview（共 $total 题），准确率(不含REVIEW)=$acc"
$lines += ""
$lines += "## 辅助证据"
$lines += "- G-FACTS 规则事实落库：命中规则词 $factHit/$($ruleTerms.Count)（evidence/g-facts-after-plant.json）"
$lines += "- G-PIN 钉住注入验证（injectedCount）：$pinPass/$($ruleTerms.Count) 规则事实被注入（$($pinDetail -join ' ')）（evidence/g-pin-injected-count.json）"
$lines += ""
$lines += "## 判定口径"
$lines += "- 目标：R2/R3（机械规则）合规率 100%，R1/R4（语义规则）≥ 80%（cases.md §基线对照）"
[IO.File]::WriteAllText((Join-Path $PSScriptRoot "result.md"), ($lines -join "`n"), [Text.UTF8Encoding]::new($false))
Write-Host "=== 完成，结果见 result.md ==="
