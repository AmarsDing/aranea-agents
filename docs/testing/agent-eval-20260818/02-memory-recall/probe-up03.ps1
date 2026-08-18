# up-03 定向探针：确认「机房空调现在设定多少度？」的 L3 召回得分
. "f:\myproject\aranea-agents\docs\testing\agent-eval-20260818\_lib.ps1"
$ev = Join-Path $PSScriptRoot "evidence"
Renew-Token | Out-Null
$rList = Api-Get "/v1/agents?limit=500"
$agentId = ($rList.Body.items | Where-Object { $_.agentKey -eq "eval_memory_probe" } | Select-Object -First 1).id

$r = Api-Post "/v1/memory/recall/debug" @{ agent_id = $agentId; query = "机房空调现在设定多少度？"; l3_limit = 10 } -OutFile (Join-Path $ev "up03-recall-now.json")
Write-Host "code=$($r.Code) l3hits=$(@($r.Body.l3Hits).Count)"
foreach ($h in @($r.Body.l3Hits)) {
    $s = $h.statement; if ($s.Length -gt 44) { $s = $s.Substring(0,44) }
    Write-Host ("L3 {0} kw={1} vec={2} imp={3} total={4} | {5}" -f $h.id.Substring(0,8), $h.scores.keyword, $h.scores.vector, $h.scores.importance, $h.scores.total, $s)
}
