# FlowLogger 流程日志 — 开发计划

> **版本**：2026-06-17 | **状态**：v2 Phase 1 + Phase 2 落库/HTTP ✅  
> **DocSync**：[changelog](../changelog/2026-05-21-Message-FlowLogger-DocSync.md)
> **需求**：[52-flow-logger.md](./52-flow-logger.md) · **设计**：[52-flow-logger.design.md](./52-flow-logger.design.md)  
> **步骤注册表**：[52-flow-logger.design.md](./52-flow-logger.design.md) §5.1  
> **进度**：[execution-plan.md](../guides/execution-plan.md)

---

## 1. 模块定位

**FlowLogger v2**：业务可观测「流程日志」——按 `trace_id` 聚合、severity 分级、人类可读 + AI 可解析；Turn 热路径统一 `TraceEmitter`，**不保留 v1**。

**代码锚点**：

| 层级 | 路径 |
|------|------|
| 核心 | `internal/event/trace_context.go`、`flow_log.go`、`flow_tracker.go`、`trace_emitter.go`、`infra.go` |
| 集成 | `internal/service/chat_orchestrator_turn.go`、`chat_orchestrator_turn_phases.go`、`chat_orchestrator_turn_metrics.go` |
| WS | `internal/server/ws.go` + `ws_io_pump.go`（`flow_log` 免 `logEnabled`） |
| 持久化 | `internal/biz/flowlog/flowlog.go`、`internal/biz/event_bus_flow_log_consumer.go`、`internal/data/flow_log_repo.go`、`internal/data/flow_log_schema.go`、`internal/data/sql/flow_log.sql`、`internal/data/ent/schema/flow_log_event.go` |
| TTL 清理 | `internal/cronrunner/jobs/flow_log_cleanup.go` |
| RPC | `internal/service/monitor_flow_log.go`、`api/kratos/monitor/v1/monitor.proto` |
| 前端 Logs | `web/src/features/monitor/flow.ts`、`useLogStreamHub.ts`、`web/src/components/monitor/LogStreamPanel.vue`、`FlowLogStream.vue`、`ProcessLogStream.vue` |
| 前端 Traces | `web/src/components/monitor/FlowTracePanel.vue`、`FlowLogExportButton.vue`、`TraceList.vue` |
| 前端测试 | `web/src/features/monitor/__tests__/flow.spec.ts` |

---

## 2. 现状评估（2026-06-17）

| 项 | 状态 | 证据 |
|----|------|------|
| v1 FlowLogger 删除 | ✅ | 已移除 `flow_logger.go` |
| `EnvelopeTypeFlowLog` | ✅ | `internal/event/envelope.go` |
| TraceEmitter + Span 合并 | ✅ | `trace_emitter.go` embeds `flow_tracker.go`；`turn_spans.go` 已删 |
| `chat_orchestrator_turn` 迁移 | ✅ | `NewTraceEmitterForRun`（`chat_orchestrator_turn.go:271`） |
| Monitor Logs 收 `flow_log` | ✅ | `useLogStreamHub` + `FlowLogStream.vue` |
| Traces 详情「流程」Tab | ✅ | `FlowTracePanel.vue` + `TraceList` Tab |
| JSONL 导出 | ✅ | `FlowLogExportButton.vue` + `buildFlowDiagnosticJsonl` |
| 详情按 trace 过滤 WS | ✅ | `flowLogMatchesTrace` + 详情页订阅 |
| `ListFlowLogs` 落库 | ✅ | `flow_log_events` + `GET /v1/monitor/flow-logs` |
| Team Runner `TraceEmitter` | ✅ | `runner_team_trpc.go:75`（迭代 7） |
| Knowledge rerank FlowLog | ✅ | `knowledge.rerank.fallback`（迭代 7） |
| EventBus 域（部分） | ✅ | 持久化/用量失败 `TraceEmitter.LogError`（迭代 7） |
| 全项目 slog 移除 / SlogBridge 删除 | ✅ | [changelog](../changelog/2026-05-20-FlowLog-V2-SlogRemoval.md) |
| 系统域步骤注册表 | ✅ | `flow_log.go` `stepTitleRegistry` + `infra.go` `BindInfra` |
| TTL 清理任务 | ✅ | `internal/cronrunner/jobs/flow_log_cleanup.go` |

---

## 3. 任务拆分

### Phase 1a — 后端 + Logs Tab（已完成）

| ID | 任务 | 状态 |
|----|------|------|
| FL-1a-01 | `trace_context.go` | ✅ |
| FL-1a-02 | `flow_log.go` + `EnvelopeTypeFlowLog` | ✅ |
| FL-1a-03 | `trace_emitter.go` + `flow_tracker.go` + 单测 | ✅ |
| FL-1a-04 | `chat_orchestrator_turn` 接 Emitter，删 TurnSpanCollector | ✅ |
| FL-1a-05 | `chat_turn_metrics` 用 `MetadataJSON()` | ✅ |
| FL-1a-06 | Chat 步骤 + 中文 title | ✅ |
| FL-1a-07 | WS `flow_log` 免 `logEnabled`（`ws_io_pump.go`） | ✅ |
| FL-1a-08 | `flow.ts` + `FlowLogStream` 展示流程日志 | ✅ |
| FL-1a-09 | 删除 `slog_bridge.go`，`internal/` slog→FlowLog | ✅ |
| FL-1a-10 | `BindInfra`（`infra.go`）替代 `SetGlobalBus` + 系统域步骤注册表 | ✅ |

### Phase 1b — Traces 详情 + 导出（已完成）

| ID | 任务 | 状态 |
|----|------|------|
| FL-1b-01 | `FlowTracePanel.vue` | ✅ |
| FL-1b-02 | `TraceList.vue` Tab「流程 \| 瀑布图 \| Span 树」 | ✅ |
| FL-1b-03 | `FlowLogExportButton.vue` | ✅ |
| FL-1b-04 | 详情按 `trace_id`/`run_id` 过滤 WS | ✅ |
| FL-1b-05 | `flow.spec.ts` | ✅ |

### Phase 1c — Logs Tab 拆分 + legacy log 清理（已完成 — 2026-05-21）

| ID | 任务 | 状态 |
|----|------|------|
| FL-1c-01 | `useLogStreamHub` + Panel/Flow/Process 组件 | ✅ |
| FL-1c-02 | WS `logEnabled` globalMode channel 修复 | ✅ |
| FL-1c-03 | 删除 intent/team 重复 `EnvelopeTypeLog` | ✅ |
| FL-1c-04 | `chat_native` / `session_compress` → flow_log | ✅ |
| FL-1c-05 | 文档 + changelog 同步 | ✅ |

### Phase 2 — 持久化（✅ 2026-05-21）

| ID | 任务 | 状态 |
|----|------|------|
| FL-2-01 | `internal/data/sql/flow_log.sql` + Ent Schema `flow_log_event.go` + `flow_log_repo.go` | ✅ |
| FL-2-02 | `monitor.proto` `ListFlowLogs` + `monitor_flow_log.go` Service | ✅ |
| FL-2-03 | Traces/Logs HTTP 拉历史 + `flowLogPersistConsumer` + `FlowLogCleanup` TTL | ✅ |

### Phase 3 — 扩展域（已完成 — 2026-05-20 迭代 7）

| ID | 任务 | 状态 |
|----|------|------|
| FL-3-01 | `runner_team_trpc.go` TraceEmitter | ✅ |
| FL-3-02 | Knowledge rerank fallback → FlowLog | ✅ |
| FL-3-03 | `event_bus` 用量失败 → `TraceEmitter.LogError` | ✅ |
| FL-3-04 | `chat_native` 统一步骤 ID（`chat.turn.enter`） | ✅ |

---

## 4. 实施顺序（后续 AI 施工）

1. 按需扩展步骤注册表（[设计文档 §5.1](./52-flow-logger.design.md#51-步骤注册表真相源)），新增步骤须同步 `flow_log.go` `stepTitleRegistry`。
2. Message/Event：按 [message-development.md](./message-development.md) P3 拆 `ToolCallConsumer` 等（与 FlowLog 正交）。

---

## 5. 验证命令

```bash
go test ./internal/event/... -count=1
go build ./...
cd web && pnpm lint && pnpm build
```

**测试覆盖状态**：

| 测试文件 | 状态 | 覆盖内容 |
|---------|------|---------|
| `internal/event/trace_emitter_test.go` | ✅ | `TestTraceEmitterPublishesFlowLog`、`TestTraceEmitterSkipsChatErrorForMonitorOnlySteps`、`TestTraceEmitterMetadataJSON` |
| `internal/event/trace_emitter_tool_test.go` | ✅ | 工具调用 span |
| `internal/event/trace_emitter_observe_test.go` | ✅ | 框架事件观察 |
| `internal/event/framework_events_otel_test.go` | ✅ | OTel span 对齐 |
| `web/src/features/monitor/__tests__/flow.spec.ts` | ✅ | `monitorLogLineFromFlowEnvelope`、`flowLogMatchesTrace`、`buildFlowDiagnosticJsonl` |
| Turn 产生有序 `flow_log` | 🟡 手动 | 集成验证 |

**手动**：

1. 发 Chat → Monitor **Logs** → 见中文流程行（无需开进程日志）。  
2. 失败 Turn → 见 `critical`/`error` 红色样式。  
3. Traces 详情 → **流程** Tab 见实时 flow_log；**导出 JSONL** 供 AI 排障。

---

## 6. PR 红线

- [x] Turn 热路径无 `slog`（`chat_orchestrator_turn` / `chat_turn_metrics`）  
- [x] `slog_bridge.go` 已删除；`LOG_BRIDGE_*` 已废弃  
- [x] 无 `TurnSpanCollector` / v1 `flow_step` 双写（`turn_spans.go` 已删）  
- [x] `internal/biz` 无 `trpc-agent-go` import（FlowLog 在 `internal/event`）  
- [x] Monitor 仍 6 个顶层 Tab  
- [x] 进程 `log` 与 `flow_log` 前端分流（`useLogStreamHub`）  
- [x] 日志统一 `pkg/loggateway.Logger`（红线 #16），无 `log/slog`、无 `loggateway.Global()`

---

## 7. 验收清单（v2 发布）

- [x] `flow_log` WS 推送 + Logs Tab 中文展示  
- [x] Usage `metadata` 含 `trace_id` + `spans`  
- [x] Traces 详情「流程」Tab  
- [x] JSONL 导出供 AI 排障  
- [x] HTTP 按 `trace_id` 查历史（Phase 2，`GET /v1/monitor/flow-logs`）  
- [x] `flow_log_events` 落库 + TTL 清理（`FLOW_LOG_TTL_DAYS` 默认 30 天）  
- [ ] 业务用户无需读 step_id 即可理解全流程  

---

## 8. 改动文件清单（v2 全量）

### 后端新增

| 文件 | 说明 |
|------|------|
| `internal/event/trace_context.go` | TraceContext + TraceDomain |
| `internal/event/flow_log.go` | FlowLogEntry schema + stepTitleRegistry |
| `internal/event/flow_tracker.go` | FlowTracker（Log* 方法 + emit） |
| `internal/event/trace_emitter.go` | TraceEmitter（embeds FlowTracker） |
| `internal/event/flow_context.go` | ctx 传播 + Deprecated 别名 |
| `internal/event/framework_events.go` | WrapFrameworkEvents / WrapFrameworkEventsWithOtel |
| `internal/event/infra.go` | Infra + BindInfra |
| `internal/event/span_collector.go` | SpanCollector（替代 turn_spans.go） |
| `internal/event/usage_aggregator.go` | UsageAggregator |
| `internal/biz/flowlog/flowlog.go` | Usecase + Repo 接口 |
| `internal/biz/flow_log.go` | 类型别名 |
| `internal/biz/event_bus_flow_log_consumer.go` | flowLogPersistConsumer |
| `internal/data/flow_log_repo.go` | Ent Repo |
| `internal/data/flow_log_schema.go` | EnsureFlowLogSchema |
| `internal/data/sql/flow_log.sql` | DDL |
| `internal/data/ent/schema/flow_log_event.go` | Ent Schema |
| `internal/service/monitor_flow_log.go` | FlowLogService.ListFlowLogs |
| `internal/cronrunner/jobs/flow_log_cleanup.go` | TTL 清理 |

### 后端修改

| 文件 | 说明 |
|------|------|
| `internal/service/chat_orchestrator_turn.go` | NewTraceEmitterForRun + 入口步骤 |
| `internal/service/chat_orchestrator_turn_phases.go` | 步骤打点 |
| `internal/service/chat_orchestrator_turn_metrics.go` | recordTurnUsage + MetadataJSON |
| `internal/service/turn_usage.go` | recordTurnUsage 委托 |
| `internal/service/turn_trace.go` | OTel root span（保留） |
| `internal/team/runner_team_trpc.go` | Team TraceEmitter |
| `internal/server/ws.go` + `ws_io_pump.go` | flow_log 免 logEnabled |
| `api/kratos/monitor/v1/monitor.proto` | ListFlowLogs RPC |

### 后端删除

| 文件 | 说明 |
|------|------|
| `internal/event/flow_logger.go` | v1 FlowLogger |
| `internal/event/turn_spans.go` | TurnSpanCollector |
| `internal/event/slog_bridge.go` | SlogBridge |

### 前端新增

| 文件 | 说明 |
|------|------|
| `web/src/components/monitor/LogStreamPanel.vue` | Logs 二级 Tab Panel |
| `web/src/components/monitor/FlowLogStream.vue` | 流程日志流 |
| `web/src/components/monitor/ProcessLogStream.vue` | 进程日志流 |
| `web/src/components/monitor/FlowTracePanel.vue` | Traces 流程 Tab |
| `web/src/components/monitor/FlowLogExportButton.vue` | JSONL 导出 |
| `web/src/features/monitor/useLogStreamHub.ts` | 单 WS 分流 |
| `web/src/features/monitor/__tests__/flow.spec.ts` | 前端单测 |

### 前端修改

| 文件 | 说明 |
|------|------|
| `web/src/components/monitor/TraceList.vue` | 详情 Tab：流程 \| 瀑布图 \| Span 树 |
| `web/src/features/monitor/flow.ts` | monitorLogLineFromFlowEnvelope + 导出 |
| `web/src/features/monitor/types.ts` | MonitorLogLine kind/severity |
| `web/src/features/monitor/api.ts` | ListFlowLogs HTTP |
