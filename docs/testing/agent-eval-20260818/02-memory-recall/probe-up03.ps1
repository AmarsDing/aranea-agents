# up-03 定向探针：用原始问题打 recall/debug，确认 24℃ 事实是否进入候选集及得分分量
. "f:\myproject\aranea-agents\docs\testing\agent-eval-20260818\_lib.ps1"
$ev = Join-Path $PSScriptRoot "evidence"
Renew-Token | Out-Null
$rList = Api-Get "/v1/agents?limit=500"
$agentId = ($rList.Body.items | Where-Object { $_.agentKey -eq "eval_memory_probe" } | Select-Object -First 1).id
$plantSid = (Get-Content (Join-Path $ev "plant-session.txt") -Raw).Trim()
Write-Host "agentId=$agentId plantSid=$plantSid"

$r = Api-Post "/v1/memory/recall/debug" @{ agent_id = $agentId; session_id = $plantSid; query = "机房空调现在设定多少度？"; l3_limit = 12 } -OutFile (Join-Path $ev "up03-recall-now.json")
Write-Host "code=$($r.Code) l3hits=$(@($r.Body.l3Hits).Count) l2hits=$(@($r.Body.l2Hits).Count)"
foreach ($h in @($r.Body.l3Hits)) {
    $s = $h.statement; if ($s.Length -gt 50) { $s = $s.Substring(0,50) }
    Write-Host ("L3 {0} kw={1} vec={2} imp={3} rec={4} q={5} total={6} | {7}" -f $h.id.Substring(0,8), $h.scores.keyword, $h.scores.vector, $h.scores.importance, $h.scores.recency, $h.scores.qualityScore, $h.scores.total, $s)
}
foreach ($h in @($r.Body.l2Hits)) {
    $s = "$($h.title)"; if ($s.Length -gt 40) { $s = $s.Substring(0,40) }
    Write-Host ("L2 {0} score={1} | {2}" -f $h.id.Substring(0,8), $h.score, $s)
}
