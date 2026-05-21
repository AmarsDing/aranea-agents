# Plugin Phase 5 深化

**日期**：2026-05-21

## 变更摘要

| 项 | 说明 |
|----|------|
| ConfirmGate 统一 | Chain 合并 catalog + confirmation_guard；AwaitUserReply 审批后放行 |
| model_router rules[] | `contains` / `regex` / `min_chars` / `priority` 解析并优先于启发式 |
| cost_guard 持久化 | `plugin_cost_guard_usage` 表 + `PluginCostGuardUsageRepo` |
| Schema 表单 | `PluginSchemaForm.vue` + PluginsPage 表单/JSON 双模式 |

## 文件

- `internal/agent/tool_confirm_gate.go`
- `internal/plugin/trpc/confirm_policy.go`
- `internal/plugin/trpc/model_router_rules.go`
- `internal/data/plugin_cost_guard_usage.go`
- `docs/sql/17_plugin_cost_guard_usage.sql`
- `web/src/components/plugins/PluginSchemaForm.vue`
