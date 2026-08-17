# 01-agent-mgmt 真机测试（修正版：使用完整资源 id = "agent_" + agent_key）
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "01"
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null

# AGT-02 详情
$r = Api-Get "/v1/agents/agent_ops_fault_diagnosis" -OutFile (Join-Path $ev "agt02-detail.json")
$ok = ($r.Code -eq "200")
Record $M "AGT-02" "Agent 详情" ($(if ($ok) { "PASS" } else { "FAIL" })) "code=$($r.Code) provider=$($r.Body.provider) model=$($r.Body.model)" $r.Ms

# AGT-03b 创建带 position 的 Agent（验证绕过方案）
$create = @{ agent_key = "test_rm_agent2"; display_name = "真机测试Agent2"; provider = "deepseek"; model = "deepseek-v4-flash"; agent_description = "带 position 创建"; position_key = "test_position_rm"; agent_variant = "general" }
$r = Api-Post "/v1/agents" $create -OutFile (Join-Path $ev "agt03b-create.json")
Record $M "AGT-03B" "创建 Agent(带 position)" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# AGT-04 更新
$r = Api-Call -Method PATCH -Path "/v1/agents/agent_test_rm_agent2" -Body @{ agent = @{ display_name = "真机测试Agent2-改" } }
Record $M "AGT-04" "更新 Agent 名称" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# AGT-05 收藏切换
$r = Api-Call -Method PATCH -Path "/v1/agents/agent_test_rm_agent2/favorite" -Body @{}
Record $M "AGT-05" "收藏切换" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# AGT-06 prompt 预览
$r = Api-Get "/v1/agents/agent_ops_fault_diagnosis/system-prompt/preview" -OutFile (Join-Path $ev "agt06-prompt.json")
Record $M "AGT-06" "System Prompt 预览" ($(if ($r.Code -eq "200" -and $r.Raw.Length -gt 200) { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# AGT-07 effective tools
$r = Api-Get "/v1/agents/agent_ops_fault_diagnosis/tools/effective" -OutFile (Join-Path $ev "agt07-tools.json")
Record $M "AGT-07" "effective tools" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# AGT-09 删除测试 Agent
$r = Api-Delete "/v1/agents/agent_test_rm_agent2"
Record $M "AGT-09" "删除 Agent" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# AGT-10 删除后查询应为错误
$r = Api-Get "/v1/agents/agent_test_rm_agent2"
Record $M "AGT-10" "删除后查询应为错误" ($(if ($r.Code -ne "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# AGT-11 分页参数 limit/offset 语义核对
$r = Api-Get "/v1/agents?limit=5&offset=0"
Record $M "AGT-11" "limit/offset 分页" ($(if ($r.Code -eq "200" -and @($r.Body.items).Count -eq 5) { "PASS" } else { "FAIL" })) "code=$($r.Code) items=$(@($r.Body.items).Count) limit=$($r.Body.limit)" $r.Ms
