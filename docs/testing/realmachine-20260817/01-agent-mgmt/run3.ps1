# 01-agent-mgmt 真机测试（v3：动态按 agentKey 解析 id）
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "01"
$ev = Join-Path $PSScriptRoot "evidence"

function Resolve-AgentId([string]$key) {
    $r = Api-Get "/v1/agents?limit=500&offset=0"
    $a = $r.Body.items | Where-Object { $_.agentKey -eq $key } | Select-Object -First 1
    if ($a) { return $a.id } else { return $null }
}

$opsId = Resolve-AgentId "ops_fault_diagnosis"
Record $M "AGT-00" "按 agentKey 解析 id" ($(if ($opsId) { "PASS" } else { "FAIL" })) "ops_fault_diagnosis -> $opsId"

# AGT-02 详情
$r = Api-Get "/v1/agents/$opsId" -OutFile (Join-Path $ev "agt02-detail.json")
Record $M "AGT-02" "Agent 详情" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) provider=$($r.Body.provider) model=$($r.Body.model)" $r.Ms

# AGT-04 更新（用新建的 test_rm_agent2）
$newId = Resolve-AgentId "test_rm_agent2"
if (-not $newId) {
    $c = @{ agent_key = "test_rm_agent2"; display_name = "真机测试Agent2"; provider = "deepseek"; model = "deepseek-v4-flash"; position_key = "test_position_rm"; agent_variant = "general" }
    Api-Post "/v1/agents" $c | Out-Null
    $newId = Resolve-AgentId "test_rm_agent2"
}
$r = Api-Call -Method PATCH -Path "/v1/agents/$newId" -Body @{ agent = @{ display_name = "真机测试Agent2-改" } }
Record $M "AGT-04" "更新 Agent 名称" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) id=$newId" $r.Ms

# AGT-05 收藏切换
$r = Api-Call -Method PATCH -Path "/v1/agents/$newId/favorite" -Body @{}
Record $M "AGT-05" "收藏切换" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# AGT-06 prompt 预览
$r = Api-Get "/v1/agents/$opsId/system-prompt/preview" -OutFile (Join-Path $ev "agt06-prompt.json")
Record $M "AGT-06" "System Prompt 预览" ($(if ($r.Code -eq "200" -and $r.Raw.Length -gt 200) { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# AGT-07 effective tools
$r = Api-Get "/v1/agents/$opsId/tools/effective" -OutFile (Join-Path $ev "agt07-tools.json")
Record $M "AGT-07" "effective tools" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# AGT-09 删除测试 Agent
$r = Api-Delete "/v1/agents/$newId"
Record $M "AGT-09" "删除 Agent" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# AGT-10 删除后查询应为错误
$r = Api-Get "/v1/agents/$newId"
Record $M "AGT-10" "删除后查询应为错误" ($(if ($r.Code -ne "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
