# 单次钉住验证：发一条消息，检查 injected_count 是否递增（临时脚本，验证后可删）
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
Renew-Token | Out-Null
$agentId = '083a1fe3a9b3d7f9a8397b48'
$rs = Api-Post '/v1/sessions' @{ agent_id = $agentId; title = 'eval-钉住单次验证'; owner_type = 'agent' }
$sid = $rs.Body.id
Write-Host "sid=$sid"
$ra = Api-Post '/v1/chat/messages' @{ session_id = $sid; agent_key = 'eval_rules_probe'; content = '运维最重要的是什么？' } -TimeoutSec 120
Write-Host "code=$($ra.Code) ms=$($ra.Ms)"
Start-Sleep -Seconds 5
