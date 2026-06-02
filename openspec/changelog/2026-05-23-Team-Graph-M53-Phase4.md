# 2026-05-23 Team×Graph M53: Phase 4 — Task 投影 / Channel async / Skip 语义

**影响**：🟡 中 | **模块**：Biz / Graph / Service / Channel / Event

## 变更摘要

完成 M53 Phase 4 三项交付：Graph Task 状态 WS 投影（`review_required` → `waiting_review`）、Channel 异步派发与 Team 编译路径对齐、FailurePolicy `skip` 节点编译与 Graph 执行语义。

## 关键变更

### TG-FP-03 — Task → waiting_review 投影

- `internal/event/envelope.go` — 新增 `graph_task_status` EnvelopeType，路由至 `graph` 通道
- `internal/biz/orchestration_status.go` — `applyGraphTaskStatus` 映射 task 细态（含 `review_required` → `waiting_review`）
- `internal/service/graph_task_status.go` — `GraphOrchestrationProjector.PublishGraphTaskStatus`
- `internal/service/graph.go` — `SubmitTaskResult` / `ReportBlocked` 挂钩发布

### TG-FP-04 — Channel async_graph 对齐编译路径

- `internal/biz/channel_async_target.go` — `ResolveChannelAsyncGraphTarget`；`async_team_id` 优先于 `async_graph_id`
- `internal/biz/channel_config_helpers.go` — `ChannelLongTaskConfig.AsyncTeamID`
- `internal/service/channel_async_graph.go` — `ExecuteGraphBuildConfig` + `team_graph` 异步执行（`CompileToGraphRuntimeConfig`）
- `internal/service/channel_ingress_async.go` — 统一 resolver；watch 覆盖 `team_graph` / `graph`
- 前端 `channelPlatformFields.ts` / `useChannelEditorForm.ts` — `async_team_id` 配置项

### Skip 节点 Graph 执行语义（TG-FP-02 部分）

- `internal/biz/failure_policy.go` — `ApplySkipNodeSemantics`；`FilterVisualizationEdges` 保留
- `internal/graph/trpc/skip_node.go` — `orchestration.skip` 函数节点，写入 `_skipped_nodes` state
- `internal/graph/trpc/node_wiring.go` / `event_bridge.go` — skip 接线 + `skipped` metadata
- `internal/biz/orchestration_status.go` — `GraphNodeEnd` skipped → `AgentNodeStatusSkipped`
- `internal/team/graph_runtime_config.go` — 编译链调用 `ApplySkipNodeSemantics`

## 架构边界

| 层 | 职责 |
|----|------|
| `biz` | failure policy、orchestration 投影、channel target 解析、BuildConfig 执行入口 |
| `graph/trpc` | skip 函数节点、节点接线、event bridge |
| `service` | task status 发布、channel async 桥接、graph build-config 执行 |
| `team` | `CompileToGraphRuntimeConfig` 编译入口不变 |

## 验收

```bash
go test ./internal/biz/... ./internal/team/... ./internal/graph/trpc/... ./internal/service/... -count=1
go build ./cmd/admin/...
```

## 待办（Phase 5）

- Channel async team_graph 集成测试

- `biz.ApplyParallelFailContinue` — 检测并行 join 拓扑（≥2 feeder → finish），为分支节点自动设置 `skip_on_failure`
- `biz.FinalizeGraphFailurePolicy` — 统一 `ParallelFail` + skip 编译 + state fields
- 保留 per-node override（`fail_fast` / `skip`）与 `fallback_agent` 优先级

## TG-FP-02 补充（2026-05-23）

- `internal/graph/trpc/failure_recovery.go` — `WithPostNodeCallback` 运行时恢复
- `policy: skip` → 编译期 `orchestration.skip`（不执行）
- `policy: skip_on_failure` → 执行失败后写入 `_skipped_nodes` 并继续
- `fallback_agent` → 主 Agent 失败后切换 SubAgent 重跑
- `ClaimTask` / `ReviewTask` → `graph_task_status` WS 投影
