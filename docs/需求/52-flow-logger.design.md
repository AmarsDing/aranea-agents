# FlowLogger 流程日志 — 技术设计

> **版本**：2026-05-20 | **对应需求**：[52-flow-logger.md](./52-flow-logger.md)  
> **开发计划**：[52-flow-logger-development.md](./52-flow-logger-development.md)  
> **遵循**：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · [AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)  
> **关联**：[18 monitor.design.md](./18%20monitor.design.md) · [24-telemetry-development.md](./24-telemetry-development.md)  
> **步骤注册表**：[flow-log-step-registry.md](../guides/flow-log-step-registry.md)

---

## 1. 设计原则

1. **框架友好**：不改 `pkg/trpc-agent-go`；Aranea 在 `internal/event` 提供 `TraceContext` + `TraceEmitter`。
2. **一条链路一个 trace_id**：从 Turn 入口生成，贯穿 FlowLog、Usage、OTel。
3. **severity 与 phase 正交**：`phase` 表生命周期；`severity` 表告警级别。
4. **一次写入、多投影**：`TraceEmitter` → FlowLog + Span（+ OTel 可选）。
5. **Turn 热路径不用 slog**：仅 `TraceEmitter`；进程级保留 slog/OTel。
6. **不保留 v1**：仅 `EnvelopeTypeFlowLog`，不与进程 `log` 混发流程事件。

---

## 2. 架构总览

```text
internal/service/trpc_turn.go
  NewTraceEmitterForRun(ctx, bus, buffer, sessionID, runID, agentKey, agentID)
  emitter.LogStart / LogDone / LogError / LogCritical / LogSkip
       │
       ▼
internal/event/trace_emitter.go
  ├─ publishFlowLog → EnvelopeTypeFlowLog, channel=monitor
  ├─ spanBuffer     → MetadataJSON() → recordTurnUsage
  └─ (OTel)         → startTurnSpan 仍由 service/turn_trace.go

WS internal/server/ws.go
  flow_log 始终下发（不要求 logEnabled）
  log       仅 logEnabled 时下发（进程日志）

web Monitor Logs
  subscribeMonitorLogsWs → onType("flow_log") → LogStream
```

---

## 3. 核心模型

### 3.1 TraceContext

```go
// internal/event/trace_context.go

type TraceContext struct {
    TraceID   string
    SessionID string
    RunID     string
    TeamID    string
    Domain    TraceDomain  // chat | team | ...
    AgentKey  string
    AgentID   string
}

func NewTraceContext(ctx context.Context, opts TraceOpts) TraceContext
func WithTraceContext(ctx context.Context, tc TraceContext) context.Context
```

`TraceID`：若 OTel span 有效则复用 W3C TraceID；否则 `tr_` + uuid。

### 3.2 FlowLogEntry（`flow_log/v1`）

```json
{
  "schema_version": "flow_log/v1",
  "id": "fl_01J...",
  "timestamp": "2026-05-20T12:00:00.123Z",
  "correlation": {
    "trace_id": "...",
    "session_id": "sess_...",
    "run_id": "run_...",
    "domain": "chat",
    "agent_key": "default",
    "agent_id": "ag_..."
  },
  "step": { "id": "chat.llm.invoke", "phase": "done", "subsystem": "llm" },
  "severity": "ok",
  "title": "语言模型调用完成",
  "message": "模型已返回，开始处理输出流（3240ms）",
  "timing": { "duration_ms": 3240 },
  "error": null,
  "extra": { "provider": "openai", "model": "gpt-4o" }
}
```

实现：`internal/event/flow_log.go`（含 `stepTitleRegistry` 中文 title）。

### 3.3 severity 推导

| 条件 | severity |
|------|----------|
| `LogCritical` | `critical` |
| `LogError` / `phase=error` | `error` |
| `LogSkip` / `phase=skip` | `warn` |
| `LogDone` / `phase=done` | `ok` |
| `LogStart` / `phase=start` | `info` |

### 3.4 TraceEmitter API

```go
// internal/event/trace_emitter.go

func NewTraceEmitter(bus Bus, buffer *Buffer, tc TraceContext) *TraceEmitter
func NewTraceEmitterForRun(ctx, bus, buffer, sessionID, runID, agentKey, agentID string) *TraceEmitter

func (e *TraceEmitter) LogStart(stepID, message string, extra ...Pair)
func (e *TraceEmitter) LogDone(stepID, message string, extra ...Pair)
func (e *TraceEmitter) LogSkip(stepID, message string, extra ...Pair)
func (e *TraceEmitter) LogWarn(stepID, title, message string, extra ...Pair)
func (e *TraceEmitter) LogError(stepID, message string, extra ...Pair)
func (e *TraceEmitter) LogCritical(stepID, message string, extra ...Pair)

func (e *TraceEmitter) FinishRoot(status string)
func (e *TraceEmitter) MetadataJSON() string
func (e *TraceEmitter) ObserveFrameworkEvent(ev *trpcevent.Event)
func WrapEventsWithTraceEmitter(in <-chan *Event, e *TraceEmitter) <-chan *Event
```

上下文：`WithTraceEmitter` / `TraceEmitterFromContext`（`flow_context.go`）。  
兼容别名：`NewFlowLogger` → `NewTraceEmitter`（无 run_id 的短生命周期场景）。

---

## 4. 传输与存储

### 4.1 Envelope

```go
const EnvelopeTypeFlowLog EnvelopeType = "flow_log"
```

| 字段 | 值 |
|------|-----|
| `Type` | `flow_log` |
| `Channel` | `monitor` |
| `SessionID` | `correlation.session_id` |
| `Metadata` | 扁平字段：`severity`、`title`、`message`、`step_id`、`flow_phase`、`trace_id`、`run_id`… |
| `Content.Text` | `{title} — {message}` 摘要 |

发布：`safego` 异步 `bus.Publish`；`Buffer.Append` 供回放。

**WS 门控**（`internal/server/ws.go`）：`flow_log` **不**受 `logEnabled` 限制；`log` 仍受限制。

### 4.2 Usage metadata

Turn 结束 `recordTurnUsage` 写入：

```json
{
  "trace_id": "...",
  "trace_root_ms": 1716196800000,
  "spans": [ { "id", "name", "parent_id", "status", "start_ms", "duration_ms", "trace_id" } ]
}
```

由 `TraceEmitter.MetadataJSON()` 生成；已删除 `TurnSpanCollector`。

### 4.3 持久化（Phase 2，未实现）

表 `flow_log_events`；RPC `ListFlowLogs`；SQL `docs/sql/15_flow_log.sql`（待建）。

---

## 5. 后端分层

```
internal/event/
  trace_context.go
  flow_log.go           # schema + title 注册表
  trace_emitter.go      # FlowLog + Span 缓冲
  flow_context.go       # ctx 传播 + NewFlowLogger 别名
  envelope.go           # EnvelopeTypeFlowLog

internal/service/
  trpc_turn.go          # NewTraceEmitterForRun + 步骤打点
  turn_usage.go         # emitter.MetadataJSON()
  turn_trace.go         # OTel root span（保留）

已删除：
  flow_logger.go (v1)
  turn_spans.go
```

**禁止**：`internal/biz` import `trpc-agent-go`；Turn 路径 `slog.Warn`（`trpc_turn` 意图 merge 等待理）。

### 5.1 Chat 步骤（已实现）

| step_id | title（中文） |
|---------|---------------|
| `chat.agent.build` | 构建 Agent |
| `chat.plugins_load` | 加载插件 |
| `chat.runner.create` | 创建 Runner |
| `chat.turn.execute` | 对话轮次 |
| `chat.intent.pass` | 意图识别 |
| `chat.user_msg_persist` | 保存用户消息 |
| `chat.llm.invoke` | 调用语言模型 |
| `chat.stream.consume` | 处理模型输出 |
| `chat.assistant_msg_persist` | 保存助手回复 |
| `chat.turn.timeout` | 对话超时 |
| `chat.turn.empty_reply` | 未收到模型回复 |
| `chat.first_byte_timeout` | 模型响应过慢 |
| `chat.usage_record` | 用量记录失败 |

完整表见 [flow-log-step-registry.md](../guides/flow-log-step-registry.md)。

### 5.2 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `FLOW_LOG_STDERR` | `0` | `1` 时 mirror 到 stderr |
| `FLOW_LOG_MAX_PER_TURN` | （未接线） | 规划 500 条上限 |
| `FLOW_LOG_PERSIST` | `0` | Phase 2 落库 |

---

## 6. 前端设计

### 6.1 Monitor Logs（已实现）

| 组件 | 路径 | 行为 |
|------|------|------|
| `LogStream.vue` | `web/src/components/monitor/LogStream.vue` | 订阅 `flow_log`；筛选 全部/流程/进程 |
| `flow.ts` | `web/src/features/monitor/flow.ts` | `monitorLogLineFromFlowEnvelope` |
| `api.ts` | `subscribeMonitorLogsWs` | `onType("flow_log")` + `onType("log")` 分流 |

展示：`title` 加粗 + `message`；`severity` → level / CSS class。

### 6.2 Traces 详情「流程」Tab（Phase 1b，已实现）

| 组件 | 路径 | 职责 |
|------|------|------|
| `FlowTracePanel.vue` | `web/src/components/monitor/FlowTracePanel.vue` | 时间线 + severity 色条 |
| `FlowLogExportButton.vue` | `web/src/components/monitor/FlowLogExportButton.vue` | JSONL 导出 |
| `TraceList.vue` | `web/src/components/monitor/TraceList.vue` | 详情 Tab：流程 \| 瀑布图 \| Span 树 |
| `flow.ts` | `buildFlowDiagnosticJsonl`、`flowLogMatchesTrace` | 过滤与导出 |

### 6.3 类型

```typescript
// web/src/features/monitor/types.ts — MonitorLogLine
kind?: "flow" | "process";
severity?: string;
title?: string;
step_id?: string;
trace_id?: string;
run_id?: string;
```

---

## 7. AI 排障包（JSONL）

```jsonl
{"type":"flow_diagnostic_bundle","schema_version":"flow_log/v1","trace_id":"...","exported_at":"..."}
{"schema_version":"flow_log/v1", ...}
```

Prompt 要点：优先 `critical`/`error`；结合 `hint`、`error.message` 给根因与修复建议。

---

## 8. 安全与脱敏

- `extra` 禁止完整 user prompt / API key
- 工具参数仅 `tool_name`、状态、耗时
- 与 `plugin/trpc/sensitive_mask.go` 一致

---

## 9. 测试

| 层级 | 文件 | 状态 |
|------|------|------|
| 单元 | `internal/event/trace_emitter_test.go` | ✅ |
| 集成 | Turn 产生有序 `flow_log` | 手动 |
| 前端 | `web/src/features/monitor/__tests__/flow.spec.ts` | ✅ |

验证：`go test ./internal/event/...`；`go build ./...`；`cd web && pnpm build`。
