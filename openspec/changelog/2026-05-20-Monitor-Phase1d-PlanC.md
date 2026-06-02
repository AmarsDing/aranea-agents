# Monitor Phase 1d — 方案 C 落地

**日期**：2026-05-20  
**范围**：Runs（Traces）为单次 Chat Turn 真相源；Events 收窄 persisted `runner.completion`。

## 后端

- `internal/biz/runner_completion.go`：`runner.completion/v1` metadata、`RecordRunnerCompletion` 幂等、`LinkRunnerCompletionUsage` 补丁
- `internal/biz/domain_event_adapter.go`：Envelope metadata → `DomainEvent` correlation 字段
- `internal/agent/event_projector.go`：`ProjectMeta` 扩展；completion Envelope 携带 `run_id` / `trace_id`
- `internal/data/monitor.go`：`ExistsRunnerCompletion`、`PatchRunnerCompletionMetadata`（sqlite `json_extract`）
- `internal/service/trpc_turn.go`：`RegisterTurnStart` + 富化 `projectMeta`
- `internal/service/turn_usage.go`：用量写入后关联 `usage_event_id` / `trace_id`
- `cmd/admin/wire.go`：`ChatService` 注入 `MonitorUsecase`

## 前端

- `web/src/features/monitor/runCorrelation.ts`：过滤/降级/跳转辅助
- `web/src/features/monitor/useMonitorRunNavigation.ts`：会话与 Runs Tab 导航
- `RealtimeEvents.vue`：隐藏已关联 completion；降级卡片 +「在 Runs 中查看」
- `TraceList.vue`：Runs 语义 +「打开会话」
- `RunnerMetricsPanel.vue`：指标点击下钻 `?tab=traces`
- `MonitorPage.vue`：`usage_event_id` query 打开 Runs 详情

## 验收（RUN-01～06）

手工：Chat 一轮 → Monitor Runs 见 usage 行 → Events 无重复 completion → Runner 指标可下钻 Traces。

## Review 修复（同日）

- `TurnCompletionBridge`：pending usage + `ClearTurn` 修复竞态/幂等后无法 PATCH
- `RecordRunnerCompletion`：已存在行仍 PATCH；INSERT 后补 PATCH
- `PatchRunnerCompletionMetadata`：返回 `patched`；`invocation_id` 回退匹配
- completion Envelope 携带 `Error`；Team `RegisterTurnStart`
- 前端：`completionCanOpenInRuns`、WS `runner_completion` 过滤、深链等 traces 加载
