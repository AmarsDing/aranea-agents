﻿﻿﻿# Domain B ask-phase only rerun (facts already planted in DB)
# Usage: powershell -ExecutionPolicy Bypass -File ask-only.ps1 [-StartIndex N] [-Count N]
param([int]$StartIndex = 0, [int]$Count = 50)
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "02"
$ev = Join-Path $PSScriptRoot "evidence"
Renew-Token | Out-Null

$agentKey = "eval_memory_probe"
$rList = Api-Get "/v1/agents?limit=500"
$agentId = ($rList.Body.items | Where-Object { $_.agentKey -eq $agentKey } | Select-Object -First 1).id
$qa = Get-Content (Join-Path $PSScriptRoot "sample-memory-qa.json") -Raw -Encoding UTF8 | ConvertFrom-Json
$cases = @($qa.cases) | Select-Object -Skip $StartIndex -First $Count

$plantedTerms = @("张伟","李明","王芳","小赵","刘洋","CE6857","SW-Core-01","FW-Edge-02","SRV-DB-03","NAS-Backup-01","UPS-Main-01","SW-Core-Spare","10.20.0.2","0571-8899-1234","0571-6655-0001","PostgreSQL","MySQL","Jira","禅道","ELK","elk-01","CN2","2Gbps","800Mbps","行政部")
$abstainWords = @("没有记录","不知道","没有告诉","无法确认","不清楚","不了解","没有提到","未曾","不记得","没有相关","无法得知","没有收到")

foreach ($c in $cases) {
    $rs = Api-Post "/v1/sessions" @{ agent_id = $agentId; title = "eval-ask-rerun-$($c.id)"; owner_type = "agent" }
    $qsid = $rs.Body.id
    $ra = Api-Post "/v1/chat/messages" @{ session_id = $qsid; agent_key = $agentKey; content = $c.question } -OutFile (Join-Path $ev "$($c.id)-ask.json") -TimeoutSec 180
    $answerText = $ra.Raw
    $result = "REVIEW"; $note = ""
    if ($ra.Code -ne "200") { $result = "FAIL"; $note = "http $($ra.Code)" }
    elseif ($c.grading -eq "abstain") {
        $hasRefuse = $false; foreach ($w in $abstainWords) { if ($answerText.Contains($w)) { $hasRefuse = $true; break } }
        $hasPlanted = $false; foreach ($t in $plantedTerms) { if ($answerText.Contains($t)) { $hasPlanted = $true; $note = "fabricated:$t"; break } }
        if ($hasPlanted) { $result = "FAIL" } elseif ($hasRefuse) { $result = "PASS" } else { $result = "REVIEW" }
    } else {
        $kws = @($c.expected_keywords)
        $hit = 0; foreach ($k in $kws) { if ($answerText.Contains($k)) { $hit++ } }
        if ($c.grading -eq "keywords_any") { $result = $(if ($hit -ge 1) { "PASS" } else { "FAIL" }) }
        else { $result = $(if ($hit -eq $kws.Count) { "PASS" } else { "FAIL" }) }
        $note = "kw $hit/$($kws.Count)"
    }
    Record $M "ASK-$($c.id)" "ask($($c.category))" $result $note $ra.Ms
}
Write-Host ("ask-only done: {0} .. {1}" -f $StartIndex, ($StartIndex + $cases.Count - 1))
