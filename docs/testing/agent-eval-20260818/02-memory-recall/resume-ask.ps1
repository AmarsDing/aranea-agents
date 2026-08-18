# 域 B 记忆召回评测 — ASK 阶段续跑（Run 2 于 sh-03 后中断，本脚本补跑剩余 case）
# 复用 evidence/plant-session.txt 中的植入会话，不重新植入；逐 case try/catch 防单点中断。
# 用法：powershell -ExecutionPolicy Bypass -File resume-ask.ps1
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "02"
$ev = Join-Path $PSScriptRoot "evidence"
Renew-Token | Out-Null

$agentKey = "eval_memory_probe"
$qa = Get-Content (Join-Path $PSScriptRoot "sample-memory-qa.json") -Raw -Encoding UTF8 | ConvertFrom-Json
$done = @{}
$resFile = Join-Path $ev "results.md"
if (Test-Path $resFile) {
    # 仅以「本轮」为口径判定已完成：最后一轮 B02 召回段/B07-post 边界之后的 ASK 行才算数，
    # 避免把上一轮（Run 1）的 ASK 记录误判为已完成。
    $lines = Get-Content $resFile -Encoding UTF8
    $boundary = -1
    for ($i = 0; $i -lt $lines.Count; $i++) {
        if ($lines[$i] -match '^\| (B02-mem-sh-10|B07-post) ') { $boundary = $i }
    }
    for ($i = $boundary + 1; $i -lt $lines.Count; $i++) {
        $m = [regex]::Match($lines[$i], '^\| ASK-(mem-[^| ]+)')
        if ($m.Success) { $done[$m.Groups[1].Value] = $true }
    }
}
$cases = @($qa.cases) | Where-Object { -not $done.ContainsKey($_.id) }
Write-Host "待补跑 case 数: $($cases.Count)（已完成 $($done.Count) 题自动跳过）"
if ($cases.Count -eq 0) { Write-Host "无待补跑 case，退出"; exit 0 }

$plantedTerms = @("张伟","李明","王芳","小赵","刘洋","CE6857","SW-Core-01","FW-Edge-02","SRV-DB-03","NAS-Backup-01","UPS-Main-01","SW-Core-Spare","10.20.0.2","0571-8899-1234","0571-6655-0001","PostgreSQL","MySQL","Jira","禅道","ELK","elk-01","CN2","2Gbps","800Mbps","行政部")
$abstainWords = @("没有记录","不知道","没有告诉","无法确认","不清楚","不了解","没有提到","未曾","不记得","没有相关","无法得知","没有收到")

$rList = Api-Get "/v1/agents?limit=500"
$agentId = (@($rList.Body.items) | Where-Object { $_.agentKey -eq $agentKey } | Select-Object -First 1).id
if (-not $agentId) { Write-Host "ERROR: 找不到 agent $agentKey"; exit 1 }

foreach ($c in $cases) {
    try {
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
        Record $M "ASK-$($c.id)" "提问($($c.category))" $result $note $ra.Ms
    } catch {
        Record $M "ASK-$($c.id)" "提问($($c.category))" "FAIL" "exception: $($_.Exception.Message)" 0
    }
}
Write-Host "=== 补跑完成 ==="
