. 'f:\myproject\aranea-agents\docs\testing\agent-eval-20260818\_lib.ps1'
$r = Api-Post '/v1/sessions' @{ agent_id='a21265a2d8f24072fb638b50'; title='eval-s1-s2-s4-s5-verify'; owner_type='agent' }
$r.Body | ConvertTo-Json -Depth 5
Write-Host "session_id=$($r.Body.id)"
