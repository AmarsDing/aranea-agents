# 08 工具 / TwinOps 连通 测试用例与结果

## 用例

| ID | 用例 | 预期 |
|----|------|------|
| TOOL-01 | GET /v1/tools | 200 + 分页正确 |
| TOOL-02 | 监控工具键在位（gns3/twin 17 个） | 全在 |
| TOOL-03 | 高危工具 requires_confirmation | fault_inject/clear/alarm_ack=true |
| TOOL-04 | GET /v1/tools/{id} | 200 |
| TOOL-05 | GET /v1/tools/runs | 200 |
| TOOL-06 | GET /v1/tools/audits | 200 |
| TOOL-07 | GET tools/{id}/agent-bindings | 200 |
| TOOL-08 | POST tools/{id}/test 在线测试 | 支持或明确拒绝 |
| TOOL-10/11 | 启用 17 个监控工具并核验 | 17/17 enabled |

## 结果：全部通过（发现 1 个环境级缺陷并已修复）

| ID | 结果 | 说明 |
|----|------|------|
| TOOL-01B/C | PASS | total=108；enabled=true 63 / enabled=false 45；分页 page_size 生效（limit 参数不生效） |
| TOOL-02D | PASS | 17 个 gns3/twin 工具全部注册（DB+API 一致） |
| TOOL-03D | PASS | 高危确认标记正确：fault_inject/fault_clear/alarm_ack requiresConfirmation=true |
| TOOL-04C | PASS | 工具详情（含 params schema/riskLevel/runtime 统计） |
| TOOL-05 | PASS | 调用历史 6.8KB |
| TOOL-06 | PASS | 审计 2.9KB |
| TOOL-07C | PASS | 绑定投影 77KB（多 agent override 明细） |
| TOOL-08C | PASS(设计) | 在线 test 对 builtin gns3 工具明确拒绝：`not supported for online test yet`（400 语义清晰；真实连通由模块 10 实测） |
| TOOL-10/11 | PASS | 15 个普通工具直接启用；2 个高危工具需 `confirm_intent=I_UNDERSTAND_RISK` → 全部 17/17 enabled |

## 原因分析

- **ISSUE-T1（环境缺陷，已修复）**：17 个监控工具平台层全部 disabled，导致 gns3_health_check 等在线测试 301/400、监控闭环不可用。属昨日 tools_allow_json 事故恢复期的遗留状态。处置：经 API 全部启用并复核 17/17。
- **高危启用二次确认（良好设计）**：high/critical 风险工具启用强制 `confirm_intent=I_UNDERSTAND_RISK`，错误信息精确。防止脚本误开高危能力。
- **字段命名观察**：列表项主键字段为 `key`（id 带 `tool_` 前缀），与部分文档中的 `tool_key` 不一致，前端/脚本需注意。
- **分页参数**：`page_size` 生效、`limit` 忽略，默认仅 20 条——前端若用 limit 会漏数据。

## 解决方案

- ISSUE-T1：已通过 API 恢复（evidence: tool-state-before/after.json）。建议：在「工具被平台级禁用」时 agent 侧给出可观测提示（当前模型只会发现工具不在 effective 集合，排障需查库）。
- 建议（低优）：ListTools 支持 `limit` 别名；`/test` 对 builtin 工具补「请通过真实对话验证」的引导文案。
