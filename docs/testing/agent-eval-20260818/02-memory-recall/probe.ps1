# 域 B 排障探针：召回链路实证（非破坏性）
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$ev = Join-Path $PSScriptRoot "evidence"
Renew-Token | Out-Null

$agentKey = "eval_memory_probe"
$ag = Api-Get "/v1/agents/$agentKey"
$agentId = $ag.Body.id
$plantSid = (Get-Content (Join-Path $ev "plant-session.txt") -Raw).Trim()
Write-Host "agentId=$agentId plantSid=$plantSid"
Write-Host "intentPassEnabled=$($ag.Body.settings.intentPassEnabled) dialogMode=$($ag.Body.settings.dialogMode) memoryEnabled=$($ag.Body.settings.memoryEnabled) l3Enabled=$($ag.Body.settings.l3Enabled)"

# 探针 1：身份问题召回（对应 mem-sh-01）
$r1 = Api-Post "/v1/memory/recall/debug" @{ agent_id = $agentId; session_id = $plantSid; query = "用户叫什么名字？担任什么职务？"; l3_limit = 8 } -OutFile (Join-Path $ev "probe-recall-identity.json")
Write-Host "probe1 code=$($r1.Code) l3hits=$(@($r1.Body.l3Hits).Count) l2hits=$(@($r1.Body.l2Hits).Count)"

# 探针 2：空调温度召回（对应 mem-sh-03）
$r2 = Api-Post "/v1/memory/recall/debug" @{ agent_id = $agentId; session_id = $plantSid; query = "机房空调温度设定为多少？"; l3_limit = 8 } -OutFile (Join-Path $ev "probe-recall-ac.json")
Write-Host "probe2 code=$($r2.Code) l3hits=$(@($r2.Body.l3Hits).Count) l2hits=$(@($r2.Body.l2Hits).Count)"

# 探针 3：不带 query 的广谱召回（看 scope 内是否有任何 fact 可达）
$r3 = Api-Post "/v1/memory/recall/debug" @{ agent_id = $agentId; session_id = $plantSid; query = "张伟 网络运维 资产台账"; l3_limit = 8 } -OutFile (Join-Path $ev "probe-recall-broad.json")
Write-Host "probe3 code=$($r3.Code) l3hits=$(@($r3.Body.l3Hits).Count) l2hits=$(@($r3.Body.l2Hits).Count)"
