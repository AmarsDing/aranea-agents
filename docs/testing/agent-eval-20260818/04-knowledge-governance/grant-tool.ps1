# 一次性 + 可重入：授予 eval_memory_probe knowledge_write/knowledge_search 工具策略
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
Renew-Token | Out-Null
$aid = "a21265a2d8f24072fb638b50"   # eval_memory_probe UUID
$r = Api-Call -Method PUT -Path "/v1/agents/$aid/tools/policy" -Body @{
    tools_enabled = $true
    profile       = "coding"
    allow         = @("knowledge_write", "knowledge_search")
    deny          = @()
}
Write-Host "PUT code=$($r.Code) raw=$($r.Raw.Substring(0, [Math]::Min(200, $r.Raw.Length)))"
$e = Api-Get -Path "/v1/agents/$aid/tools/effective"
foreach ($it in @($e.Body.items)) {
    $k = if ($it.toolKey) { $it.toolKey } else { $it.tool_key }
    if ($k -like "*knowledge*") {
        $st = if ($it.effectiveState) { $it.effectiveState } else { $it.effective_state }
        Write-Host ("{0} enabled={1} state={2} reason={3}" -f $k, $it.enabled, $st, $it.reason)
    }
}
