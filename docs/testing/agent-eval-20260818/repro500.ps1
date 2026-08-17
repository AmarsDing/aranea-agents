$ErrorActionPreference = "Continue"
. (Join-Path $PSScriptRoot "_lib.ps1")
$agentId = "a21265a2d8f24072fb638b50"
$agentKey = "eval_memory_probe"
$rs = Api-Post "/v1/sessions" @{ agent_id = $agentId; title = "repro-500"; owner_type = "agent" }
"session Code=$($rs.Code) sid=$($rs.Body.id)"
$r = Api-Post "/v1/chat/messages" @{ session_id = $rs.Body.id; agent_key = $agentKey; content = "hello, reply briefly" } -TimeoutSec 60
"chat Code=$($r.Code) ms=$($r.Ms)"
"raw head: " + $r.Raw.Substring(0, [Math]::Min(300, $r.Raw.Length))
