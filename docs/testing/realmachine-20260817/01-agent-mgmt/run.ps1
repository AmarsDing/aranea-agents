# 01-agent-mgmt 真机测试
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "01"
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null

# AGT-01 列表分页
$r = Api-Get "/v1/agents?page=1&page_size=10" -OutFile (Join-Path $ev "agt01-list.json")
Record $M "AGT-01" "Agent 列表分页" ($(if ($r.Code -eq "200" -and $r.Body.total -gt 0) { "PASS" } else { "FAIL" })) "code=$($r.Code) total=$($r.Body.total) page_items=$(@($r.Body.items).Count)" $r.Ms

# AGT-02 详情（ops_fault_diagnosis）
$r = Api-Get "/v1/agents/ops_fault_diagnosis" -OutFile (Join-Path $ev "agt02-detail.json")
$ok = ($r.Code -eq "200" -and $r.Body.agentKey -eq "ops_fault_diagnosis")
Record $M "AGT-02" "Agent 详情" ($(if ($ok) { "PASS" } else { "FAIL" })) "code=$($r.Code) provider=$($r.Body.provider) model=$($r.Body.model)" $r.Ms

# AGT-03 创建测试 Agent
$create = @{ agent_key = "test_realmachine_agent"; display_name = "真机测试Agent"; provider = "deepseek"; model = "deepseek-v4-flash"; agent_description = "真机功能测试专用，测完即删" }
$r = Api-Post "/v1/agents" $create -OutFile (Join-Path $ev "agt03-create.json")
Record $M "AGT-03" "创建 Agent" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# AGT-04 更新
$r = Api-Call -Method PATCH -Path "/v1/agents/test_realmachine_agent" -Body @{ agent = @{ display_name = "真机测试Agent-改" } }
Record $M "AGT-04" "更新 Agent 名称" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# AGT-05 收藏切换
$r = Api-Call -Method PATCH -Path "/v1/agents/test_realmachine_agent/favorite" -Body @{}
Record $M "AGT-05" "收藏切换" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# AGT-06 prompt 预览
$r = Api-Get "/v1/agents/ops_fault_diagnosis/system-prompt/preview" -OutFile (Join-Path $ev "agt06-prompt.json")
Record $M "AGT-06" "System Prompt 预览" ($(if ($r.Code -eq "200" -and $r.Raw.Length -gt 200) { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# AGT-07 effective tools
$r = Api-Get "/v1/agents/ops_fault_diagnosis/tools/effective" -OutFile (Join-Path $ev "agt07-tools.json")
Record $M "AGT-07" "effective tools" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# AGT-08 creators
$r = Api-Get "/v1/agents/creators"
Record $M "AGT-08" "creators 列表" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# AGT-09 删除测试 Agent
$r = Api-Delete "/v1/agents/test_realmachine_agent"
Record $M "AGT-09" "删除 Agent" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# AGT-10 删除后再查应 404/错误
$r = Api-Get "/v1/agents/test_realmachine_agent"
Record $M "AGT-10" "删除后查询应为错误" ($(if ($r.Code -ne "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
