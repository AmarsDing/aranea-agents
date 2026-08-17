# C 域前置探测：环境探活 + embedder 配置 + 既有 collection 列表
. f:\myproject\aranea-agents\docs\testing\agent-eval-20260818\_lib.ps1

$hc = Api-Get -Path "/v1/knowledge/collections" -TimeoutSec 15
Write-Host ("collections http=" + $hc.Code + " ms=" + $hc.Ms)

$emb = Api-Get -Path "/v1/knowledge/embedder-config" -TimeoutSec 15
Write-Host ("embedder-config http=" + $emb.Code)
if ($emb.Body) { $emb.Raw }

$tok = Get-Token
Write-Host ("token_len=" + $tok.Length)
