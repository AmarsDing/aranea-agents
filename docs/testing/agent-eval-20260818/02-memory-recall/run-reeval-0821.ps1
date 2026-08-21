# Domain-B full re-eval 2026-08-21 (SkipPlant). This file is ASCII-only.
# Chinese strings live in sample-memory-qa.json and reeval-grade.json (UTF8).
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null
$logFile = Join-Path $ev "run-reeval-0821.log"
if (Test-Path $logFile) { Remove-Item $logFile -Force }
function Log([string]$s) {
    Add-Content -Path $logFile -Value $s -Encoding UTF8
    Write-Host $s
}
Renew-Token | Out-Null

$agentKey = "eval_memory_probe"
$qa = Get-Content (Join-Path $PSScriptRoot "sample-memory-qa.json") -Raw -Encoding UTF8 | ConvertFrom-Json
$grade = Get-Content (Join-Path $PSScriptRoot "reeval-grade.json") -Raw -Encoding UTF8 | ConvertFrom-Json
$cases = @($qa.cases)
$plantedTerms = @($grade.plantedTerms)
$abstainWords = @($grade.abstainWords)
$plantSid = (Get-Content (Join-Path $ev "plant-session.txt") -Raw).Trim()
Log "plant session=$plantSid cases=$($cases.Count)"

$rList = Api-Get "/v1/agents?limit=500"
$agentId = (@($rList.Body.items) | Where-Object { $_.agentKey -eq $agentKey } | Select-Object -First 1).id
if (-not $agentId) { Log "FAIL: eval agent not found code=$($rList.Code)"; exit 1 }
Log "agentId=$agentId"

$recallMs = @()
foreach ($c in ($cases | Select-Object -First 10)) {
    $rd = Api-Post "/v1/memory/recall/debug" @{ query = $c.question; session_id = $plantSid; agent_id = $agentId } -OutFile (Join-Path $ev "$($c.id)-reeval-recall.json")
    $recallMs += $rd.Ms
    Log ("[RECALL] {0} code={1} {2}ms" -f $c.id, $rd.Code, $rd.Ms)
}

$stat = @{}
foreach ($c in $cases) {
    $rs = Api-Post "/v1/sessions" @{ agent_id = $agentId; title = ("eval-reeval-{0}-20260821" -f $c.id); owner_type = "agent" }
    $qsid = $rs.Body.id
    if (-not $qsid) {
        Log ("[FAIL] {0} session create code={1}" -f $c.id, $rs.Code)
        continue
    }
    $ra = Api-Post "/v1/chat/messages" @{ session_id = $qsid; agent_key = $agentKey; content = $c.question } -OutFile (Join-Path $ev ("{0}-reeval-ask.json" -f $c.id)) -TimeoutSec 180
    $answerText = $ra.Raw
    $result = "REVIEW"
    $note = ""
    if ($ra.Code -ne "200") {
        $result = "FAIL"
        $note = ("http {0}" -f $ra.Code)
    } elseif ($c.grading -eq "abstain") {
        $hasRefuse = $false
        foreach ($w in $abstainWords) { if ($answerText.Contains([string]$w)) { $hasRefuse = $true; break } }
        $hasPlanted = $false
        foreach ($t in $plantedTerms) {
            if ($answerText.Contains([string]$t)) { $hasPlanted = $true; $note = ("hallucinate:{0}" -f $t); break }
        }
        if ($hasPlanted) { $result = "FAIL" } elseif ($hasRefuse) { $result = "PASS" } else { $result = "REVIEW" }
    } else {
        $kws = @($c.expected_keywords)
        $hit = 0
        foreach ($k in $kws) { if ($answerText.Contains([string]$k)) { $hit++ } }
        if ($c.grading -eq "keywords_any") {
            $result = $(if ($hit -ge 1) { "PASS" } else { "FAIL" })
        } else {
            $result = $(if ($hit -eq $kws.Count) { "PASS" } else { "FAIL" })
        }
        $note = ("kw {0}/{1}" -f $hit, $kws.Count)
    }
    if (-not $stat.ContainsKey($c.category)) { $stat[$c.category] = @{ pass = 0; fail = 0; review = 0 } }
    if ($result -eq "PASS") { $stat[$c.category].pass++ } elseif ($result -eq "FAIL") { $stat[$c.category].fail++ } else { $stat[$c.category].review++ }
    Log ("[{0}] {1} {2} {3} ({4}ms)" -f $result, $c.id, $c.category, $note, $ra.Ms)
}

Log "=== accuracy ==="
foreach ($cat in @("single_hop","multi_hop","temporal","update","abstention")) {
    if (-not $stat.ContainsKey($cat)) { continue }
    $s = $stat[$cat]
    $den = $s.pass + $s.fail
    $acc = "-"
    if ($den -gt 0) { $acc = "{0:P0}" -f ($s.pass / $den) }
    Log ("{0}: PASS={1} FAIL={2} REVIEW={3} acc={4}" -f $cat, $s.pass, $s.fail, $s.review, $acc)
}
$recallSorted = @($recallMs | Sort-Object)
if ($recallSorted.Count -gt 0) {
    Log ("recall latency min={0} max={1} n={2}" -f $recallSorted[0], $recallSorted[-1], $recallSorted.Count)
}
Log "=== done ==="
