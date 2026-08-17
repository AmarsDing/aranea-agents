# 05 Spirit 动态编排 测试用例与结果

## 用例

| ID | 用例 | 预期 |
|----|------|------|
| SPIRIT-01 | GET /v1/agents/agent___spirit__ | 200 + key=__spirit__ |
| SPIRIT-02 | GET /v1/chat/options | 200 |
| SPIRIT-03 | 创建 spirit 会话 | 200 + sid |
| SPIRIT-04 | 发送平凡指令（真实 LLM） | 200 + agentMessage |
| SPIRIT-05 | GET /v1/spirit/{sid}/teams | 200 |
| SPIRIT-06 | GET /v1/chat/plans?session_id | 200 |
| SPIRIT-07 | POST /v1/spirit/{sid}/synthesize（空团队） | 明确错误码 |

## 结果：7/7 PASS（1 项为测试脚本误报，已复核）

| ID | 结果 | 耗时 | 说明 |
|----|------|------|------|
| SPIRIT-01 | PASS | 40ms | 精灵助手在内，profile=spirit |
| SPIRIT-02 | PASS | 23ms | 324B 选项 |
| SPIRIT-03 | PASS | 35ms | 会话创建成功 |
| SPIRIT-04 | PASS(复核) | 9.9s | LLM 真实回复 "PONG"，未误建团队 |
| SPIRIT-05 | PASS | 21ms | 空 teams `{"items":[]}` |
| SPIRIT-06 | PASS | 22ms | 空 plans `{"items":[]}` |
| SPIRIT-07 | PASS | 22ms | 400 + 明确原因 `no completed or failed teams to synthesize`（错误语义清晰） |

## 原因分析

- **链路可用**：spirit agent 会话 → 真实 LLM 应答闭环正常，平凡指令未触发不必要的编排（无 plan/team 泄漏）。
- **测试误报（已澄清）**：run.ps1 初判 FAIL 系脚本读取 `agent_message`（snake_case），实际响应为 camelCase `agentMessage`。响应一致性本身无缺陷。
- **性能观察（PERF-S1）**：spirit 单次平凡问答 `token_in=24199 / token_out=19 / 9.9s`。系统提示词约 24k token，成本与首 token 延迟显著高于普通 agent（02 模块同模型对话 token_in 约 2-4k）。多团队编排场景将成倍放大。
- **空态语义良好**：teams/plans 空态返回 200+空列表；synthesize 前置不满足返回 400 + 明确 reason，前端可精确提示。

## 解决方案

- PERF-S1（建议，低优）：spirit 系统提示词分级装载——平凡对话（无 plan_and_execute 意图）走精简 prompt；或开启 prompt cache。可显著降低单轮成本与延迟。
- 监控场景的真实编排（任务分解→团队组装→综合）在 10-monitor-scenario 模块以告警闭环任务实测，此处不重复消耗 token。
