# Review Optimization — Phase D (2026-05-21)

## Phase D — P1 测试与前端分层

- **A2A**：`invoke_integration_test.go`（本地 chat capability、跨 workspace 拒绝）；修复 `callee_resolve_test` / `invoker_capability_test` 的 `UpdateRemoteAgentHealth` mock
- **Team**：`team_modes_test.go` 扩展 sequential/adaptive
- **Graph**：`graph_def_test.go`（`defToBuildConfig`）
- **Plugin**：`ResolvePluginVersion` 单测
- **Agent LIST-04**：`BatchUpdateAgents` biz API + `agent_batch_test.go`（前端多选 UI 仍 backlog）
- **ResourceManager**：`useResourceManagerPage.ts`；`ResourceManagerPage.vue` ~604 行（composable 分层）
- **Agent 设置页**：样式迁至 `agent-settings-page.scss`；页壳 ~251 行
- **Chat i18n**：ReAct 步骤标题 `chat.react.*`（`reactPlannerParse.ts` + zh-CN/en-US）

## 仍排 P2+

- Artifact Chat 附件 part 契约、CodeExecutor→Artifact 管道
- Graph HITL / Checkpoint UI、Evolution 趋势图
- LIST-04 列表批量 UI、pgvector 多租户测、Telemetry gRPC 采样

## 验证

```bash
make runtime-boundary
go test ./internal/biz/... ./internal/a2a/...
cd web && pnpm test && pnpm build
```
