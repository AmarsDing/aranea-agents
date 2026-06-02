# 2026-05-20 — 迭代 5：MCP 持久化 / Monitor Runner 指标 / cost_guard / EventBus 拆分 / Ecosystem MVP

## 摘要

落实 [execution-plan](../guides/execution-plan.md) 迭代 5 主项：MCP 重连元数据测试与 UI、Monitor Runner 指标与 Channel 告警下拉、StopGeneration 发布 `run_status`、cost_guard ModelSelector、EventBusConsumer 按类型拆分、Ecosystem 后端 MVP、Chat turn OTel Span、前端 mapper 模块化。

## 变更

### I5-MCP-01

- `internal/biz/mcp_server_test.go`：`RecordReconnectMetadata` 单测
- `web/src/features/mcp/McpServerItem.vue`：展示累计 `reconnect_count`

### I5-MON-01 / I5-MON-02

- Proto：`GetRunnerMetrics` / `RunnerMetricsSummary`
- `internal/biz/monitor.go`：`GetRunnerMetrics`
- `web/src/components/monitor/RunnerMetricsPanel.vue` + `MonitorPage` Usage 页嵌入
- `MonitorAlertRules.vue`：Channel `q-select` 下拉

### SYS-02

- `internal/service/chat.go`：`StopGeneration` 成功后 `publishRunStatus(cancelled)`
- `internal/service/chat_stop_generation_test.go`

### PLG-03

- `CostGuardConfig` + `CostGuardConfigForAgent` + `PluginCostGuardSelector` + `ChainedModelSelector`
- `internal/agent/trpc_build.go`：与 `model_router` 链式 ModelSelector

### SYS-03

- `event_bus_buffer_handler.go` / `event_bus_runner_handler.go` / `event_bus_state_handler.go`

### I6-ECO-01

- `api/kratos/ecosystem/v1/ecosystem.proto` + biz/data/service + SQL 表
- `web/src/pages/EcosystemPage.vue` + `features/ecosystem/api.ts`

### I6-TEL-01

- `internal/service/turn_trace.go` + `trpc_turn.go`：`chat.turn` Span

### FE-02 / SYS-06

- `web/src/features/knowledge/mappers.ts`、`evaluation/mappers.ts`
- `domain-mappers.spec.ts` 引用独立 mapper
- `AgentSettingsPromptSection.vue`（可复用组件）

## 验证

```bash
make api && make wire && make build && make test && make runtime-boundary
cd web && pnpm test && pnpm build
```
