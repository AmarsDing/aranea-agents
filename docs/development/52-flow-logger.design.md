# FlowLogger 流程日志 — 技术设计

> **版本**：2026-06-27 | **对应需求**：[52-flow-logger.md](./52-flow-logger.md)  
> **开发计划**：[52-flow-logger.development.md](./52-flow-logger.development.md)  
> **遵循**：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · [AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)  
> **关联**：[18 monitor.design.md](./18%20monitor.design.md) · [24-telemetry-development.md](./24-telemetry-development.md)  
> **步骤注册表**：本文 §5.1（新增/重命名步骤须同步更新）

---

## 1. 设计原则

1. **框架友好**：不改 `pkg/trpc-agent-go`；Aranea 在 `internal/event` 提供 `TraceContext` + `TraceEmitter`。
2. **一条链路一个 trace_id**：从 Turn 入口生成，贯穿 FlowLog、Usage、OTel。
3. **severity 与 phase 正交**：`phase` 表生命周期；`severity` 表告警级别。
4. **一次写入、多投影**：`TraceEmitter` → FlowLog + Span（+ OTel 可选）。
5. **Turn 热路径不用 slog**：仅 `TraceEmitter`；进程级日志统一走 `pkg/loggateway.Logger`（红线 #16）。
6. **不保留 v1**：仅 `EnvelopeTypeFlowLog`，不与进程 `log` 混发流程事件。

### 1.1 架构约束（从需求 §4.7 迁入）

- 实现位于 `internal/event`，**禁止** `internal/biz` import `trpc-agent-go`。
- Turn 热路径 **禁止** 使用 `slog`（SlogBridge 已移除）；统一 `TraceEmitter` + `TraceContext`。
- `internal/service` 创建上下文并注入 `context.Context`；`internal/agent` 仅通过 context 取 logger。
- 与 [AGENT_RUNTIME_BOUNDARY](../AGENT_RUNTIME_BOUNDARY.md) / [trpc-agent-framework-first](../../.cursor/rules/trpc-agent-framework-first.mdc) 一致。
- 日志统一使用 `pkg/loggateway.Logger`（构造注入 + `With()` 预设字段），禁止 `log/slog`、禁止 `loggateway.Global()`。

---

## 2. 架构总览

### 2.1 一次写入、多投影架构

```text
                    ┌─────────────────────────────────────┐
  业务代码一次打点   │  TraceEmitter（统一写入 API，v2）      │
                    └──────────────┬──────────────────────┘
                                   │ 同一 trace_id / run_id
           ┌───────────────────────┼───────────────────────┐
           ▼                       ▼                       ▼
   FlowLog 投影器          Span 投影器              OTel 投影器
   (Monitor 主输出)    (Usage 瀑布图)          (Jaeger，可选)
   WS flow_log + DB      metadata.spans         OTLP export
```

### 2.2 代码调用链

```text
internal/service/chat_orchestrator_turn.go
  NewTraceEmitterForRun(TraceEmitterOpts{Ctx, Bus, Buffer, SessionID, RunID, AgentKey, AgentID, Domain, LG})
  emitter.LogStart / LogDone / LogError / LogCritical / LogSkip / LogWarn
       │
       ▼
internal/event/trace_emitter.go  (TraceEmitter = wrapper)
  └─ internal/event/flow_tracker.go (FlowTracker: 实际 Log* 实现)
       ├─ publishFlowLog → EnvelopeTypeFlowLog, channel=monitor
       ├─ spanBuffer     → MetadataJSON() → recordTurnUsage
       └─ (OTel)         → startTurnSpan 仍由 service/turn_trace.go

WS internal/server/ws.go + ws_io_pump.go
  flow_log 始终下发（不要求 logEnabled）
  log       仅 logEnabled 时下发（进程日志）

web Monitor Logs
  useLogStreamHub → onType("flow_log") → FlowLogStream
```

### 2.3 三条跟踪能力的代码锚点

| 能力 | 代码锚点 | 主要输出 |
|------|---------|---------|
| **OTel Tracing** | `internal/service/turn_trace.go`、`internal/telemetry/turntrace/` | OTLP → Jaeger/Tempo |
| **Turn Span（业务 Span）** | `TraceEmitter.MetadataJSON()` → `chat_turn_metrics.go` `recordTurnUsage` | `model_token_usage_events.metadata_json.spans` |
| **FlowLogger v2（流程日志）** | `internal/event/trace_emitter.go` → WS `monitor` `flow_log` | `EnvelopeTypeFlowLog`（与进程 `log` 分流） |

---

## 3. 核心模型

### 3.1 TraceContext

```go
// internal/event/trace_context.go

type TraceDomain string

const (
    TraceDomainChat      TraceDomain = "chat"
    TraceDomainTeam      TraceDomain = "team"
    TraceDomainGraph     TraceDomain = "graph"
    TraceDomainChannel   TraceDomain = "channel"
    TraceDomainKnowledge TraceDomain = "knowledge"
    TraceDomainPlugin    TraceDomain = "plugin"
    TraceDomainSystem    TraceDomain = "system"
    TraceDomainSkill     TraceDomain = "skill"
    TraceDomainA2A       TraceDomain = "a2a"
    TraceDomainVoice      TraceDomain = "voice"
    TraceDomainClientTool TraceDomain = "client_tool"
)

type TraceContext struct {
    TraceID   string
    SessionID string
    RunID     string
    TeamID    string
    Domain    TraceDomain
    AgentKey  string
    AgentID   string
}

func NewTraceContext(ctx context.Context, opts TraceOpts) TraceContext
func WithTraceContext(ctx context.Context, tc TraceContext) context.Context
func TraceContextFromContext(ctx context.Context) (TraceContext, bool)
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
  "span_id": "otel_root_span_id",
  "parent_span_id": "",
  "extra": { "provider": "openai", "model": "gpt-4o" }
}
```

**`span_id` / `parent_span_id`**（Phase 1 of Problem 4，2026-06-27）：
- `span_id` = OTel turn-root span ID，由 `FlowTracker.SetOtelRefs(traceID, rootSpanID)` 注入；空表示未配置 OTel。
- `parent_span_id` = turn-root 的上游 OTel parent span ID；当前阶段留空（reserved for future per-step linkage）。
- 用途：将 FlowLog 与 OTel trace（Jaeger）通过共享 `span_id` 关联，实现"一次写入、多投影"的跨系统对齐（见需求 §2.3）。
- 落库：通过 `toMetadata()` 写入 `payload_json`，无需独立 DB 列。

实现：`internal/event/flow_log.go`（含 `stepTitleRegistry` 中文 title、`normalizeStepID` v1→v2 别名、`severityForPhase` 推导、`SpanID`/`ParentSpanID` 字段）。

### 3.3 severity 推导

| 条件 | severity |
|------|----------|
| `LogCritical` | `critical` |
| `LogError` / `phase=error` | `error` |
| `LogSkip` / `phase=skip` | `warn` |
| `LogDone` / `phase=done` | `ok` |
| `LogStart` / `phase=start` | `info` |
| `LogWarn`（显式） | `warn` |

### 3.4 TraceEmitter / FlowTracker API

```go
// internal/event/trace_emitter.go

// TraceEmitter embeds FlowTracker; Log* methods are promoted from FlowTracker.
type TraceEmitter struct {
    *FlowTracker
}

func NewTraceEmitter(bus Bus, buffer *Buffer, tc TraceContext, lg loggateway.Logger) *TraceEmitter
func NewTraceEmitterForRun(opts TraceEmitterOpts) *TraceEmitter

func (e *TraceEmitter) ObserveFrameworkEvent(ev *trpcevent.Event)
func (e *TraceEmitter) EmitProgress(ctx context.Context, stepID, phase, message, category string, extra ...Pair)
```

```go
// internal/event/trace_emitter.go — TraceEmitterOpts

type TraceEmitterOpts struct {
    Ctx       context.Context
    Bus       Bus
    Buffer    *Buffer
    SessionID string
    RunID     string
    AgentKey  string
    AgentID   string
    Domain    TraceDomain
    LG        loggateway.Logger
}
```

```go
// internal/event/flow_tracker.go — FlowTracker (embedded by TraceEmitter)

func NewFlowTracker(infra *Infra, buffer *Buffer, tc TraceContext, lg loggateway.Logger) *FlowTracker

func (ft *FlowTracker) LogStart(stepID, message string, extra ...Pair)
func (ft *FlowTracker) LogDone(stepID, message string, extra ...Pair)
func (ft *FlowTracker) LogSkip(stepID, message string, extra ...Pair)
func (ft *FlowTracker) LogWarn(stepID, title, message string, extra ...Pair)
func (ft *FlowTracker) LogError(stepID, message string, extra ...Pair)
func (ft *FlowTracker) LogCritical(stepID, message string, extra ...Pair)
func (ft *FlowTracker) Log(stepID string, phase FlowPhase, message string, extra ...Pair)

func (ft *FlowTracker) FinishRoot(status string)
func (ft *FlowTracker) MetadataJSON() string
func (ft *FlowTracker) TraceID() string
func (ft *FlowTracker) RunID() string
func (ft *FlowTracker) SpanCollector() *SpanCollector
func (ft *FlowTracker) UsageAggregator() *UsageAggregator
```

上下文传播：`WithTraceEmitter` / `TraceEmitterFromContext`（`flow_context.go`）。  
兼容别名（Deprecated）：`WithFlowLogger` / `FlowLoggerFromContext` / `NewFlowLogger` → 均委托至 `TraceEmitter` 等价 API，新代码应使用 `loggateway.Logger` + `With()`。

框架事件包装：

```go
// internal/event/framework_events.go
func WrapFrameworkEvents(in <-chan *trpcevent.Event, emitter *TraceEmitter, observer FrameworkSpanObserver) <-chan *trpcevent.Event
func WrapFrameworkEventsWithOtel(in <-chan *trpcevent.Event, emitter *TraceEmitter, observer FrameworkSpanObserver, otelSrc OtelSpanIDSource) <-chan *trpcevent.Event
```

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

发布：`FlowTracker.emit` → `infra.Publish`（异步 `bus.Publish`）；`Buffer.Append` 供回放。

**WS 门控**（`internal/server/ws_io_pump.go`）：`flow_log` **不**受 `logEnabled` 限制（始终下发）；`log` 仍受 `logEnabled` 限制。

### 4.2 Usage metadata

Turn 结束 `recordTurnUsage`（`internal/service/chat_turn_metrics.go`）写入：

```json
{
  "trace_id": "...",
  "trace_root_ms": 1716196800000,
  "spans": [ { "id", "name", "parent_id", "status", "start_ms", "duration_ms", "trace_id" } ]
}
```

由 `FlowTracker.MetadataJSON()` 生成（内部委托 `SpanCollector`）；已删除 `TurnSpanCollector` / `turn_spans.go`。

### 4.3 持久化（Phase 2）

**数据表**：`flow_log_events`（Ent Schema：`internal/data/ent/schema/flow_log_event.go`）

| 列 | 类型 | 说明 |
|----|------|------|
| `id` | TEXT PK | `fl_` + uuid |
| `trace_id` | TEXT | 索引 |
| `session_id` | TEXT | 索引 |
| `run_id` | TEXT | 索引 |
| `team_id` | TEXT | |
| `domain` | TEXT | |
| `agent_key` | TEXT | |
| `step_id` | TEXT | |
| `flow_phase` | TEXT | |
| `severity` | TEXT | 默认 `info` |
| `title` | TEXT | |
| `message` | TEXT | |
| `payload_json` | TEXT | 默认 `{}` |
| `created_at` | TEXT | RFC3339 |

**索引**：`idx_flow_log_trace_created`、`idx_flow_log_session_created`、`idx_flow_log_run_created`

**DDL**：`internal/data/sql/flow_log.sql`（由 `internal/data/flow_log_schema.go` embed，`EnsureFlowLogSchema` 执行）

**落库消费者**：`internal/biz/event_bus_flow_log_consumer.go` — `flowLogPersistConsumer` 订阅 `EnvelopeTypeFlowLog`，调用 `FlowLogUsecase.Save` 写入 `flow_log_events`。

**TTL 清理**：`internal/cronrunner/jobs/flow_log_cleanup.go` — `FlowLogCleanup` 定期调用 `FlowLogUsecase.PurgeExpired`，默认 30 天（`FLOW_LOG_TTL_DAYS` 可配置）。

### 4.4 Proto / API 契约

```protobuf
// api/kratos/monitor/v1/monitor.proto

message FlowLogEntry {
  string id = 1;
  string trace_id = 2;
  string session_id = 3;
  string run_id = 4;
  string team_id = 5;
  string domain = 6;
  string agent_key = 7;
  string step_id = 8;
  string flow_phase = 9;
  string severity = 10;
  string title = 11;
  string message = 12;
  string payload_json = 13;
  string created_at = 14;
}

message ListFlowLogsRequest {
  string trace_id = 1;
  string session_id = 2;
  string run_id = 3;
  string severity = 4;
  string domain = 5;
  int32 limit = 6;
  int32 offset = 7;
  string since = 8;   // RFC3339
  string until = 9;   // RFC3339
}

message ListFlowLogsResponse {
  repeated FlowLogEntry items = 1;
  int32 total = 2;
}

rpc ListFlowLogs(ListFlowLogsRequest) returns (ListFlowLogsResponse) {
  option (google.api.http) = {get: "/v1/monitor/flow-logs"};
}
```

**Service 实现**：`internal/service/monitor_flow_log.go` — `FlowLogService.ListFlowLogs` 委托 `biz.FlowLogUsecase.List`。

**Biz 层**：`internal/biz/flowlog/flowlog.go` — `Usecase` / `Repo` 接口 / `Query` / `Record`；`internal/biz/flow_log.go` 提供类型别名（`FlowLogUsecase = flowlog.Usecase`）。

**Data 层**：`internal/data/flow_log_repo.go` — `flowLogRepo`（Ent 实现 `Repo` 接口）。

---

## 5. 后端分层

```
internal/event/
  trace_context.go      # TraceContext + TraceDomain + OTel TraceID 对齐
  flow_log.go           # FlowLogEntry schema + stepTitleRegistry + normalizeStepID
  flow_tracker.go       # FlowTracker: Log* 方法 + emit + severity 推导
  trace_emitter.go      # TraceEmitter (embeds FlowTracker) + EmitProgress
  flow_context.go       # ctx 传播 + Deprecated NewFlowLogger 别名
  framework_events.go   # WrapFrameworkEvents / WrapFrameworkEventsWithOtel
  infra.go              # Infra + BindInfra (替代 SetGlobalBus)
  envelope.go           # EnvelopeTypeFlowLog 常量
  span_collector.go     # SpanCollector (替代已删 turn_spans.go)
  usage_aggregator.go   # UsageAggregator

internal/service/
  chat_orchestrator_turn.go          # NewTraceEmitterForRun + chat.receive/session_fetch/agent_hydrate
  chat_orchestrator_turn_phases.go   # 步骤打点（intent/llm/stream/agent.build/runner.create 等）
  chat_orchestrator_turn_metrics.go  # recordTurnUsage → emitter.MetadataJSON()
  turn_usage.go                      # recordTurnUsage 委托入口
  turn_trace.go                      # OTel root span（startTurnSpan/endTurnSpan）
  monitor_flow_log.go                # FlowLogService.ListFlowLogs RPC

internal/biz/
  flowlog/flowlog.go                 # Usecase + Repo 接口 + Query/Record
  flow_log.go                        # 类型别名（向后兼容）
  event_bus_flow_log_consumer.go     # flowLogPersistConsumer 落库消费者

internal/data/
  flow_log_repo.go                   # Ent Repo 实现
  flow_log_schema.go                 # EnsureFlowLogSchema (embed sql/flow_log.sql)
  sql/flow_log.sql                   # DDL
  ent/schema/flow_log_event.go       # Ent Schema

internal/team/
  runner_team_trpc.go                # Team TraceEmitter (NewTraceEmitterForRun)

internal/cronrunner/jobs/
  flow_log_cleanup.go                # TTL 清理任务

已删除：
  flow_logger.go (v1)
  turn_spans.go
  slog_bridge.go
```

**禁止**：`internal/biz` import `trpc-agent-go`；Turn 路径 `slog.Warn`。

### 5.1 步骤注册表（真相源）

> **迁移**：SlogBridge 已移除，系统域步骤见 [changelog](../changelog/2026-05-20-FlowLog-V2-SlogRemoval.md)。  
> **命名**：`{domain}.{subsystem}.{action}`，全小写，点分。新增/重命名步骤须更新下表。  
> **代码位置**：`internal/event/flow_log.go` `stepTitleRegistry`。

#### Chat（`domain=chat`）

| step_id | phase 典型 | severity（成功/失败） | title（用户可见） | 说明 |
|---------|------------|----------------------|-------------------|------|
| `chat.receive` | start | info / — | 收到消息 | 入口 |
| `chat.active_check` | done | info / — | 检查活跃运行 | |
| `chat.session_fetch` | done/error | ok / error | 加载会话 | |
| `chat.session_ownership` | done/error | ok / error | 会话归属校验 | |
| `chat.agent_hydrate` | done/error | ok / error | 加载 Agent 配置 | |
| `chat.provider_resolve` | done | ok / — | Provider/Model已解析 | |
| `chat.attachment.preflight` | done/error | ok / error | 附件预检 | |
| `chat.pre_planning_gate` | done | info / — | 规划门决策 | |
| `chat.clarification_gate` | done | info / — | 澄清门 | |
| `chat.proactive_recall` | done | info / — | 主动召回 | |
| `chat.turn.enter` | start | info / — | 进入Agent Turn执行 | |
| `chat.agent.build` | done/error | ok / error | 构建 Agent | |
| `chat.plugins_load` | done | ok / — | 加载插件 | |
| `chat.runner.create` | done/error | ok / error | 创建 Runner | |
| `chat.runner.ralph_loop` | done | info / — | Ralph Loop 配置 | |
| `chat.runner.rollback` | done/error | ok / error | Runner 会话回滚 | |
| `chat.runner.rollback_boundary` | info | info / — | Runner 回滚边界 | |
| `chat.user_msg_persist` | done/error | ok / error | 保存用户消息 | |
| `chat.intent.pass` | done/skip/error | ok / info / warn | 意图识别 | |
| `chat.llm.invoke` | start/done/error | info / ok / error | 调用语言模型 | 原 `chat.llm_call` |
| `chat.stream.consume` | done/error | ok / error | 处理模型输出 | |
| `chat.assistant_msg_persist` | done/error | ok / error | 保存助手回复 | |
| `chat.turn.execute` | done | ok / — | 对话轮次完成 | |
| `chat.turn.timeout` | error | critical | 对话超时 | |
| `chat.turn.timeout_with_reply` | error | warn | 超时但已保存回复 | |
| `chat.turn.empty_reply` | error | critical | 未收到模型回复 | |
| `chat.first_byte_timeout` | error | critical | 模型响应过慢 | |
| `chat.pending_dequeue` | start/done/error | info / ok / error | 处理排队消息 | |
| `chat.usage_record` | error | error | 用量记录失败 | Token/Span 落库 |
| `chat.turn.usage_source` | done | info | 用量来源追踪 | 仅当 UsageSource 为空或 estimated 时记录；诊断框架抑制 usage 的 TECH-DEBT |
| `chat.team.invoke` | start/done/error | info / ok / error | 委派团队会话 | |
| `run.start` | start | info / — | 创建会话运行 | |

#### Team（`domain=team`）

| step_id | severity（成功/失败） | title |
|---------|----------------------|-------|
| `team.run.start` | ok / error | 开始团队协作 |
| `team.run.execute` | ok / critical | 执行团队任务 |
| `team.run.finish` | ok / error | 团队任务结束 |
| `team.run.attachments` | error | 团队附件装配失败 |
| `team.run.graph` | ok | Team GraphAgent 已构建 |
| `team.intent.merge_fail` | warn | 团队意图合并失败 |
| `team.intent_anchor_fallback` | warn | 团队意图锚点回退 |
| `team.usage_record_fail` | warn | 团队成员用量记录失败 |
| `team.turn.usage` | info | 团队轮次用量 |
| `team.member.<nodeID>` | ok / warn(skip) / error | 团队成员执行 |

> `team.member.<nodeID>`：图节点级成员执行（start/done/skip/error），发射点 `PublishTeamStepStarted` / `PersistGraphRunStep`；nodeID 后缀隔离并行成员计时，title 经 `stepTitle` 前缀回退解析为 `team.member`。

#### Knowledge（`domain=knowledge`）

| step_id | severity | title |
|---------|----------|-------|
| `knowledge.search` | ok / error | 知识库检索 |
| `knowledge.rerank` | ok / warn | 结果重排 |
| `knowledge.rerank.fallback` | warn | 重排降级为向量排序 |

#### Plugin / System（`domain=plugin` / `system`）

| step_id | severity | title |
|---------|----------|-------|
| `plugin.cost_guard.block` | warn | 费用保护拦截 |
| `plugin.model_router.route` | info | 模型路由 |
| `event_bus.runner.completion` | ok / error | Runner 完成处理 |
| `event_bus.usage.record` | error | 用量事件写入失败 |
| `event_bus.state.persist` | error | 会话状态保存失败 |
| `event_bus.state.apply` | error | 会话状态应用失败 |
| `event_bus.monitor.persist` | error | 监控事件持久化失败 |
| `system.bus.drop` | warn | 事件总线丢弃消息 |
| `system.ws.*` | warn | WebSocket 连接/读写/解析 |
| `system.cron.*` | warn/error | 定时任务死信/重试/panic/跳过 |
| `system.telemetry.*` | info/warn/error | OTel 初始化 |
| `system.agent.*` | info/warn/error | Agent 构建（`build`：start/done/error 冷构建可视）/缓存（`cache_hit`/`cache_miss`）/DB 解析 |
| `system.provider.*` | info/error | 模型目录与预检 |
| `system.plugin.*` / `system.hook.*` | warn | 插件种子与 Hook 重载 |
| `system.auto_memory.*` | warn/info | 自动记忆提取 |
| `system.memory_canary.failed` | error | 记忆闭环金丝雀告警（写→召回→失效任一断言失败） |
| `system.memory_l1_archive.failed` | error | L1 归档连续失败告警（同一任务连续 3 次归档失败，任务保留在重试集合中） |
| `system.monitor.alert_*` | warn | 告警 Webhook/通道 |
| `system.session.*` | warn | 会话压缩/标题/回滚 |
| `system.graph.*` | error | 图任务启动/状态/恢复 |
| `system.task.*` | warn/error | 任务调度/声明/超时 |
| `system.channel.dead_letter` | warn | 渠道投递死信 |
| `system.knowledge.embed_fail` | warn | 知识嵌入失败 |
| `system.safego.panic` | warn | 协程 panic 已恢复 |
| `system.grpc.unauthenticated` | warn | gRPC 未认证请求 |
| `channel.feishu.ws.panic` | error | 飞书 WebSocket 入站 panic |
| `channel.feishu.ws.inbound_fail` | warn | 飞书 WebSocket ProcessInbound 失败 |
| `channel.runtime.credentials_fail` | warn | Channel Runtime Reload 读凭据失败 |
| `channel.inbound.accept` | info | 入站 ACK 已发送（Accept 阶段） |
| `channel.turn.execute` | info | Channel Turn Execute 开始 |
| `channel.turn.done` | info | Channel Turn 正常结束 |
| `channel.turn.timeout` | error | Channel Turn 超时或 deadline exceeded |
| `channel.turn.cancel` | info | 用户 IM 取消命令（CancelRun） |
| `channel.progress.patch` | info | 长静默进度 PATCH（debug 级 SysLog） |
| `chat.intent.merge_fail` | warn | 意图结果合并失败 |
| `chat.usage_record_fail` | warn | 用量/轮次记录失败 |

#### Cron（定时任务，2026-07-29 补齐 P0）

| step_id | severity（成功/失败） | title |
|---------|----------------------|-------|
| `cron.job.trigger` | info / — | 定时任务触发 |
| `cron.job.dispatch` | info / warn | 定时任务分发 |
| `cron.job.execute` | ok / error | 定时任务执行 |

#### Graph（`domain=graph`，2026-07-29 补齐 P0）

| step_id | severity（成功/失败） | title |
|---------|----------------------|-------|
| `graph.run.start` | info / error | 图运行开始 |
| `graph.run.finish` | ok / error | 图运行结束 |
| `graph.run.resume` | info / error | 图运行恢复 |
| `graph.node.execute` | ok / error | 图节点执行 |
| `graph.checkpoint.save` | ok / error | 检查点保存 |
| `graph.hitl.wait` | info / — | 等待人工确认 |

#### Skill（`domain=skill`，2026-07-29 补齐 P0）

| step_id | severity（成功/失败） | title |
|---------|----------------------|-------|
| `skill.import.start` | info / — | Skill 包导入开始 |
| `skill.import.validate` | ok / error | Skill 包校验 |
| `skill.import.conflict` | warn / — | Skill 冲突决策 |
| `skill.import.done` | ok / error | Skill 导入完成 |
| `skill.watch.reload` | ok / warn | Skill 热重载 |
| `skill.execute` | ok / error | Skill 运行时执行 |

#### Knowledge 摄取（2026-07-29 补齐 P0）

| step_id | severity（成功/失败） | title |
|---------|----------------------|-------|
| `knowledge.ingest.start` | info / — | 知识文档摄取开始 |
| `knowledge.ingest.parse` | ok / error | 文档解析分块 |
| `knowledge.ingest.embed` | ok / warn | 文档向量嵌入 |
| `knowledge.ingest.done` | ok / error | 知识摄取完成 |
| `knowledge.vault.sync` | ok / error | Vault 同步 |
| `knowledge.entity.merge` | ok / error | 知识实体合并 |
| `knowledge.block.promote` | info / ok / error | 知识块晋升 |
| `knowledge.rebuild_index` | info / ok / error | 知识块索引重建 |

#### A2A 联邦（`domain=a2a`，2026-07-29 补齐 P0）

| step_id | severity（成功/失败） | title |
|---------|----------------------|-------|
| `a2a.invoke.start` | info / — | A2A 联邦调用开始 |
| `a2a.invoke.governance` | ok / warn | A2A 治理链检查 |
| `a2a.invoke.remote` | ok / error | A2A 远端调用 |
| `a2a.invoke.done` | ok / error | A2A 调用完成 |

#### 系统启动/关闭（2026-07-29 补齐 P0）

| step_id | severity（成功/失败） | title |
|---------|----------------------|-------|
| `system.startup.migration` | ok / error | 数据库迁移 |
| `system.startup.seed` | ok / warn | 基础数据种子 |
| `system.startup.ready` | info / — | 服务就绪 |
| `system.startup.shutdown` | info / — | 服务关闭 |

#### 会话/Agent/平台管理（2026-07-29 补齐 P1）

| step_id | severity（成功/失败） | title |
|---------|----------------------|-------|
| `session.create` | ok / error | 会话创建 |
| `session.delete` | ok / error | 会话删除 |
| `session.rename` | ok / error | 会话重命名 |
| `agent.crud.create` | ok / error | Agent 创建 |
| `agent.crud.update` | ok / error | Agent 更新 |
| `agent.crud.delete` | ok / error | Agent 删除 |
| `provider.catalog.sync` | ok / error | 模型目录同步 |
| `mcp.server.add` | ok / error | MCP 服务器添加 |
| `mcp.server.remove` | ok / error | MCP 服务器移除 |
| `memory.auto.extract` | ok / error | 自动记忆提取 |
| `media.generate` | ok / error | 媒体生成 |
| `evaluation.run` | ok / error | 评测集运行 |
| `gateway.webhook.delivery` | ok / error | 出站 Webhook 投递 |
| `monitor.alert.evaluate` | ok / warn | 告警评估 |
| `monitor.selfcheck.run` | ok / warn | 系统自检 |
| `event_bus.flow_log.persist` | error | 流程日志落库失败 |

#### Channel 连接生命周期（2026-07-29 补齐 P1）

| step_id | severity（成功/失败） | title |
|---------|----------------------|-------|
| `channel.connect.open` | info / — | 渠道连接建立 |
| `channel.connect.close` | info / — | 渠道连接断开 |
| `channel.connect.error` | error | 渠道连接异常 |
| `channel.turn.background` | info / — | 渠道后台继续执行 |

#### 系统设置与生态（2026-07-29 补齐 P2）

| step_id | severity（成功/失败） | title |
|---------|----------------------|-------|
| `settings.update` | ok / error | 系统设置更新 |
| `settings.hot_reload` | info / warn | 配置热更新 |
| `ecosystem.pack.install` | ok / error | 生态包安装 |

#### Voice 语音伴侣（M74，2026-08-06）

| step_id | severity（成功/失败） | title |
|---------|----------------------|-------|
| `voice.session.start` | info / — | 语音会话开始 |
| `voice.session.done` | ok / — | 语音会话结束 |
| `voice.asr.final` | ok / — | 语音识别终稿 |
| `voice.asr.idle_reclaim` | info / — | 语音 ASR 空闲回收 |
| `voice.tts.start` | ok / — | 语音播报开始 |
| `voice.tts.end` | ok / — | 语音播报结束 |
| `voice.tts.enqueue_fail` | warn | 语音播报入队失败 |
| `voice.barge_in` | ok / — | 语音打断 |
| `voice.provider.fallback` | warn | 语音服务降级 |
| `voice.error` | error | 语音链路错误 |
| `voice.confirm.resolved` | ok / — | 语音确认决议（M74 V2-T5，2026-08-08） |
| `voice.archive.saved` | ok / — | 语音留档保存（M74 V2-T6，2026-08-08） |
| `voice.archive.degraded` | warn | 语音留档降级（开关读取失败/存储失败，消息正常派发；M74 V2-T6，2026-08-08） |
| `voice.archive.truncate` | warn | 语音留档截断（语句 PCM 超 8 MiB 上限；M74 V2-T6，2026-08-08） |

#### Client Tool 客户端工具桥（M74 V2-T3，2026-08-08）

| step_id | severity（成功/失败） | title |
|---------|----------------------|-------|
| `client_tool.invoke` | info / — | 调用客户端工具 |
| `client_tool.result` | ok / error | 客户端工具执行完成 |
| `client_tool.timeout` | error | 客户端工具执行超时 |

#### 别名（v1 → v2，兼容 1 版本）

| v1 `flow_step` | v2 `step_id` |
|----------------|--------------|
| `chat.llm_call` | `chat.llm.invoke` |
| `chat.turn_execute` | `chat.turn.execute` |
| `chat.turn_enter` | `chat.turn.enter` |
| `chat.agent_build` | `chat.agent.build` |
| `chat.intent_pass` | `chat.intent.pass` |
| `chat.empty_reply` | `chat.turn.empty_reply` |
| `chat.turn_timeout` | `chat.turn.timeout` |
| `chat.stream_consume` | `chat.stream.consume` |
| `chat.runner_create` | `chat.runner.create` |

### 5.2 环境变量

| 变量 | 默认 | 说明 |
|------|------|------|
| `FLOW_LOG_STDERR` | `0` | `1` 时 mirror 到 stderr |
| `FLOW_LOG_MAX_PER_TURN` | （未接线） | 规划 500 条上限 |
| `FLOW_LOG_PERSIST` | `0` | Phase 2 落库 |
| `FLOW_LOG_TTL_DAYS` | `30` | 落库日志保留天数（`flowLogTTL()`） |

---

## 6. 前端设计

### 6.1 Monitor Logs（流程/进程拆分）

| 组件 | 路径 | 行为 |
|------|------|------|
| `LogStreamPanel.vue` | `web/src/components/monitor/LogStreamPanel.vue` | 二级 Tab；挂载 `FlowLogStream` + `ProcessLogStream` |
| `FlowLogStream.vue` | `web/src/components/monitor/FlowLogStream.vue` | 仅 `flow_log`；默认连接 |
| `ProcessLogStream.vue` | `web/src/components/monitor/ProcessLogStream.vue` | 仅 `log`；`process_log_enabled` + Tab 切换自动恢复 |
| `useLogStreamHub.ts` | `web/src/features/monitor/useLogStreamHub.ts` | 单 WS、分流、`connected`/`live` 状态 |
| `flow.ts` | `web/src/features/monitor/flow.ts` | `monitorLogLineFromFlowEnvelope` |

> 废弃：`LogStream.vue` 单卡片 + viewMode 三态切换（由 Panel 替代）。

### 6.1.1 legacy `EnvelopeTypeLog` 迁移

| 发射点 | 动作 |
|--------|------|
| `chat_orchestrator_turn` / `runner_team_trpc` → `intent-pass` log | **删除**（已有 `TraceEmitter`） |
| `publishTeamMonitor` | **删除**（已有 `team.run.*` flow_log） |
| `chat_native` team_invoke | → `flow.LogStart("chat.team.invoke", ...)` |
| `session_compress` | → `TraceEmitter.LogWarn/LogError` |
| `plugin/trpc/safe_logger.go` | **保留** 进程 `log` |

### 6.2 Traces 详情「流程」Tab

| 组件 | 路径 | 职责 |
|------|------|------|
| `FlowTracePanel.vue` | `web/src/components/monitor/FlowTracePanel.vue` | 时间线 + severity 色条 |
| `FlowLogExportButton.vue` | `web/src/components/monitor/FlowLogExportButton.vue` | JSONL 导出 |
| `TraceList.vue` | `web/src/components/monitor/TraceList.vue` | 详情 Tab：流程 \| 瀑布图 \| Span 树 |
| `flow.ts` | `web/src/features/monitor/flow.ts` | `buildFlowDiagnosticJsonl`、`flowLogMatchesTrace` |

### 6.3 类型

```typescript
// web/src/features/monitor/types.ts — MonitorLogLine
kind?: "flow" | "process";
severity?: string;
title?: string;
step_id?: string;
trace_id?: string;
run_id?: string;
session_id?: string;
hint?: string;
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

## 9. 测试设计

| 层级 | 文件 | 覆盖内容 |
|------|------|---------|
| 单元 | `internal/event/trace_emitter_test.go` | `TestTraceEmitterPublishesFlowLog`、`TestTraceEmitterSkipsChatErrorForMonitorOnlySteps`、`TestTraceEmitterMetadataJSON` |
| 单元 | `internal/event/trace_emitter_tool_test.go` | 工具调用 span |
| 单元 | `internal/event/trace_emitter_observe_test.go` | 框架事件观察 |
| 单元 | `internal/event/framework_events_otel_test.go` | OTel span 对齐 |
| 集成 | Turn 产生有序 `flow_log` | 手动验证 |
| 前端 | `web/src/features/monitor/__tests__/flow.spec.ts` | `monitorLogLineFromFlowEnvelope`、`flowLogMatchesTrace`、`buildFlowDiagnosticJsonl` |

> 测试执行状态详见 [52-flow-logger.development.md §5](./52-flow-logger.development.md#5-验证命令)

验证：`go test ./internal/event/...`；`go build ./...`；`cd web && pnpm build`。
