# Plugin Phase 6 — 变更摘要

**日期**：2026-05-21  
**模块**：Plugin / Chat / Agent ConfirmGate

## 摘要

完成 Phase 6 四项深化：工具失败事件流反思、cost_guard 按 Agent scope 分桶、工具确认专用 UI（结构化 reply）、model_router `rules[]` 可视化编辑器。

## 变更

| 项 | 说明 |
|----|------|
| retry_and_reflect | AfterTool 返回 `CustomResult`（reflection_hint）并发布 `plugin.retry_reflect` 事件 |
| cost_guard scope | `CostGuardBudgetRegistry` 按 plugin scope / agent_id 分桶；全局插件默认 per-agent |
| 工具确认 UI | RunStatus 增加 `await_kind` / `await_tool_key`；Chat 横幅 Approve/Deny；结构化 token |
| rules[] 编辑器 | `ModelRouterRulesEditor.vue` 集成于 `PluginSchemaForm` |

## 关键文件

- `internal/plugin/trpc/retry_reflect.go`
- `internal/plugin/trpc/cost_guard_registry.go`
- `internal/tools/serviceawaitreply/tool_confirm.go`
- `internal/service/run_status_publish.go` / `chat.go`
- `internal/agent/tool_confirmation.go`
- `web/src/components/plugins/ModelRouterRulesEditor.vue`
- `web/src/components/chat/ChatMessagePanel.vue`
- `api/kratos/chat/v1/chat.proto`

## 验证

- `make api && go test ./internal/agent/... ./internal/plugin/trpc/... ./internal/service/...`
- `go build ./...`
- `cd web && pnpm build`
