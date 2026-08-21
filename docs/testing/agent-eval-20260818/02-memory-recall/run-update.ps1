# Domain-B update regression (P0-1 tiebreak acceptance, real LLM).
# Reuses facts planted on 2026-08-18 (no replant). All Chinese text comes from
# sample-memory-qa.json read as UTF8 -- keep this file pure ASCII.
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null
$logFile = Join-Path $ev "run-update-0821.log"
function Log([string]$s) { Add-Content -Path $logFile -Value $s -Encoding UTF8 }
Renew-Token | Out-Null

$agentKey = "eval_memory_probe"
$qa = Get-Content (Join-Path $PSScriptRoot "sample-memory-qa.json") -Raw -Encoding UTF8 | ConvertFrom-Json
$cases = @($qa.cases | Where-Object { $_.category -eq "update" })
$plantSid = (Get-Content (Join-Path $ev "plant-session.txt") -Raw).Trim()
Log "plant session=$plantSid cases=$($cases.Count)"

$rList = Api-Get "/v1/agents?limit=500"
$agentId = (@($rList.Body.items) | Where-Object { $_.agentKey -eq $agentKey } | Select-Object -First 1).id
if (-not $agentId) { Log "FAIL: eval agent not found code=$($rList.Code)"; exit 1 }
Log "agentId=$agentId"

$pass = 0; $fail = 0; $rank1OK = 0
foreach ($c in $cases) {
    # (1) recall/debug: rank-1 must be the newest fact (direct P0-1 evidence)
    $rd = Api-Post "/v1/memory/recall/debug" @{ query = $c.question; session_id = $plantSid; agent_id = $agentId } -OutFile (Join-Path $ev "$($c.id)-upd-recall.json")
    $top1 = ""
    if ($rd.Body -and $rd.Body.results) { $top1 = ($rd.Body.results | Select-Object -First 1 | ConvertTo-Json -Compress -Depth 6) }
    elseif ($rd.Body -and $rd.Body.hits) { $top1 = ($rd.Body.hits | Select-Object -First 1 | ConvertTo-Json -Compress -Depth 6) }
    $kw0 = [string]@($c.expected_keywords)[0]
    $rank1Hit = ($top1.Contains($kw0))
    if ($rank1Hit) { $rank1OK++ }

    # (2) fresh session ask; answer must contain the new-value keyword(s)
    $rs = Api-Post "/v1/sessions" @{ agent_id = $agentId; title = "eval-upd-$($c.id)-20260821"; owner_type = "agent" }
    $qsid = $rs.Body.id
    if (-not $qsid) { Log "[FAIL] $($c.id) session create failed code=$($rs.Code)"; $fail++; continue }
    $ra = Api-Post "/v1/chat/messages" @{ session_id = $qsid; agent_key = $agentKey; content = $c.question } -OutFile (Join-Path $ev "$($c.id)-upd-ask.json") -TimeoutSec 180
    $hit = 0; $kws = @($c.expected_keywords)
    foreach ($k in $kws) { if ($ra.Raw.Contains([string]$k)) { $hit++ } }
    if ($ra.Code -eq "200" -and $hit -eq $kws.Count) { $pass++; $result = "PASS" } else { $fail++; $result = "FAIL" }
    Log "[$result] $($c.id) kw=$hit/$($kws.Count) rank1=$rank1Hit ($($ra.Ms)ms)"
}
Log "=== domain-B update: ask PASS=$pass FAIL=$fail /10; recall rank1-newvalue $rank1OK/10 ==="
