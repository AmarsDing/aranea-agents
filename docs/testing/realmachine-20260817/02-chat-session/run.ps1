# 02-chat-session 真机测试（真实 LLM 调用）
. (Join-Path (Split-Path $PSScriptRoot -Parent) "_lib.ps1")
$M = "02"
$ev = Join-Path $PSScriptRoot "evidence"
New-Item -ItemType Directory -Force -Path $ev | Out-Null

# 解析 agent id
$r = Api-Get "/v1/agents?limit=500"
$agentId = ($r.Body.items | Where-Object { $_.agentKey -eq "ops_fault_diagnosis" } | Select-Object -First 1).id

# CHAT-01 创建会话
$r = Api-Post "/v1/sessions" @{ agent_id = $agentId; title = "真机测试-对话模块"; owner_type = "agent" } -OutFile (Join-Path $ev "chat01-session.json")
$sid = $r.Body.id
Record $M "CHAT-01" "创建会话" ($(if ($r.Code -eq "200" -and $sid) { "PASS" } else { "FAIL" })) "code=$($r.Code) sid=$sid" $r.Ms

# CHAT-02 发送消息（真实 LLM 调用）
$r = Api-Post "/v1/chat/messages" @{ session_id = $sid; agent_key = "ops_fault_diagnosis"; content = "用一句话说明你的职责。" } -OutFile (Join-Path $ev "chat02-send.json") -TimeoutSec 180
$reply = ""
if ($r.Body.agent_message) { $reply = ($r.Body.agent_message | ConvertTo-Json -Depth 8 -Compress) }
Record $M "CHAT-02" "发送消息→LLM 回复" ($(if ($r.Code -eq "200" -and $r.Body.agent_message) { "PASS" } else { "FAIL" })) "code=$($r.Code) reply_len=$($reply.Length)" $r.Ms

# CHAT-03 会话消息列表
$r = Api-Get "/v1/sessions/$sid/messages" -OutFile (Join-Path $ev "chat03-messages.json")
$msgCount = 0; if ($r.Body.items) { $msgCount = @($r.Body.items).Count } elseif ($r.Body.messages) { $msgCount = @($r.Body.messages).Count }
Record $M "CHAT-03" "消息列表" ($(if ($r.Code -eq "200" -and $msgCount -ge 2) { "PASS" } else { "FAIL" })) "code=$($r.Code) msgs=$msgCount" $r.Ms

# CHAT-04 run-status
$r = Api-Get "/v1/chat/run-status?session_id=$sid"
Record $M "CHAT-04" "run-status 查询" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) $($r.Raw)" $r.Ms

# CHAT-05 会话检索
$r = Api-Get "/v1/sessions?query=真机测试"
Record $M "CHAT-05" "会话检索" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# CHAT-06 更新标题
$r = Api-Call -Method PATCH -Path "/v1/sessions/$sid" -Body @{ title = "真机测试-对话模块-改" }
Record $M "CHAT-06" "更新会话标题" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# CHAT-07 pin/unpin
$r1 = Api-Post "/v1/sessions/$sid/pin" @{}
$r2 = Api-Post "/v1/sessions/$sid/unpin" @{}
Record $M "CHAT-07" "pin/unpin" ($(if ($r1.Code -eq "200" -and $r2.Code -eq "200") { "PASS" } else { "FAIL" })) "pin=$($r1.Code) unpin=$($r2.Code)"

# CHAT-08 导出
$r = Api-Get "/v1/sessions/$sid/export" -OutFile (Join-Path $ev "chat08-export.json")
Record $M "CHAT-08" "会话导出" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code) len=$($r.Raw.Length)" $r.Ms

# CHAT-09 turns
$r = Api-Get "/v1/sessions/$sid/turns"
Record $M "CHAT-09" "turns 列表" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# CHAT-10 timeline
$r = Api-Get "/v1/sessions/$sid/timeline"
Record $M "CHAT-10" "timeline" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms

# CHAT-11 删除会话
$r = Api-Delete "/v1/sessions/$sid"
Record $M "CHAT-11" "删除会话" ($(if ($r.Code -eq "200") { "PASS" } else { "FAIL" })) "code=$($r.Code)" $r.Ms
