# Event 事件系统模块 — 实现设计文档

> 对应需求：`34-event-system.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> 进度状态（已实现/未实现能力清单）见 [34-event-system.development.md](./34-event-system.development.md) §2。

---

## 一、模块概述

基于事件总线的发布/订阅机制，支持系统内部组件间的异步事件通信。核心组件为 `event.Bus`（发布/订阅 + 背压策略）+ `event.Envelope`（统一事件信封）+ `event.Buffer`（环形缓冲 + 断连重放），通过 WebSocket 统一传输至前端。

---

## 二、架构总览

```
trpc-agent-go Runner
       │
       │ *trpcevent.Event
       ▼
 EventProjector ──── 投影为 Envelope ────┐
       │                                  │
       │ event.Infra.Publish()            │
       ▼                                  ▼
  ┌─────────────────────────────────────────────┐
  │            event.Infra (dual-bus)            │
  │  SessionBus (chat/team/graph/knowledge)      │
  │  MonitorBus (log/flow_log/alert)            │
  │  + EventWAL (WBPF for Critical events)      │
  └──────┬──────────┬──────────┬────────────────┘
         │          │          │
         ▼          ▼          ▼
   EventBusConsumer  ─┬─ eventBufferHandler   (环形缓冲 Append)
   (核心 4 handler)   ├─ runnerCompletionHandler (Monitor / Usage / TurnMemory)
                      ├─ stateDeltaHandler    (ApplyStateDelta)
                      └─ eventPersistHandler  (异步持久化 → event_store)
   EventBusSideConsumers ─┬─ toolCallConsumer       (ToolInvocation 记录)
   (旁路 6 typed consumer)├─ callbackConsumer       (Webhook 回调)
                          ├─ messageStoreConsumer   (消息存储)
                          ├─ flowLogPersistConsumer (flow_log 持久化)
                          ├─ userFeedbackConsumer   (用户反馈)
                          └─ usageRollupConsumer    (Usage 汇总)
         │          │
         ▼          ▼
   SessionUsecase   前端 WsTransport
   (ApplyStateDelta)   + useEnvelopeStream
                        + Monitor RealtimeEvents
                        + Chat Inspector (SessionTimelineDialog 双 Tab)
```

**双 Bus 隔离**（`internal/event/infra.go`）：
- `SessionBus`：承载 chat / team / graph / knowledge 业务事件
- `MonitorBus`：承载 log / flow_log / mcp.health / alert 等高频运维事件
- 路由模式由 `MONITOR_BUS_ROUTING` 环境变量控制（`split` 默认 / `dual` 兼容）
- `Infra.Publish()` 对 Critical 事件先走 WAL（WBPF），再 fanout 到对应 Bus

---

## 三、核心数据模型

### 3.1 contract 子包（纯接口与值对象）

`internal/event/contract/`

biz 层应只 import `contract` 子包，禁止 import 父 `event` 包（含实现）。父 `event` 包通过 type alias 向后兼容旧调用点。

| 文件 | 职责 |
|------|------|
| `contract/envelope.go` | Envelope 结构 + EnvelopeType 枚举 + Clone / MatchFilterKey / ContainsTag / RouteChannel |
| `contract/bus.go` | Bus 接口 + SubscribeOptions + DropPolicy + ChannelPriority |
| `contract/reliability.go` | EventReliability 分级 + ClassifyEventReliability / IsCriticalWBPFType / RequiresBlockUpTo |

### 3.2 Envelope（事件信封）

`internal/event/contract/envelope.go`

Envelope 是事件系统唯一的传输单元，所有事件均封装为 Envelope 在 Bus 上流转。

| 字段 | 类型 | 说明 |
|------|------|------|
| ID | string | 事件唯一 ID（UUID） |
| Type | EnvelopeType | 事件类型枚举 |
| Author | string | 事件作者（agent_name / user / system） |
| SessionID | string | 所属会话 ID |
| TeamID | string | 所属 Team ID（可选） |
| RequestID | string | 请求 ID（可选） |
| InvocationID | string | 调用 ID（可选） |
| ParentInvocationID | string | 父调用 ID（可选） |
| Branch | string | 分支追踪（可选） |
| FilterKey | string | 层级过滤键（可选） |
| Tag | string | 业务标签，逗号分隔（可选；trpc Event 框架 TagDelimiter 为 `;`） |
| Timestamp | string | RFC3339Nano 时间戳 |
| Version | int | 版本号 |
| Channel | string | 路由通道（chat / monitor / team / graph / knowledge） |
| Content | *EnvelopeContent | 文本内容（可选） |
| ToolCall | *EnvelopeToolCall | 工具调用信息（可选） |
| StateDelta | *EnvelopeStateDelta | 状态增量（可选） |
| Transfer | *EnvelopeTransfer | Agent 转移信息（可选） |
| Error | *EnvelopeError | 错误信息（可选） |
| Usage | *EnvelopeUsage | Token 用量（可选） |
| TokenUsage | *EnvelopeTokenUsage | 详细 Token 用量记录（可选，用于 usage 汇总） |
| Extensions | map[string]string | 命名空间化扩展元数据（可选） |
| Actions | *EnvelopeActions | 流控制提示（可选） |
| Trace | *EnvelopeTrace | 执行追踪（可选） |
| Metadata | map[string]any | 附加元数据（可选） |
| SessionRevision | int64 | 会话版本号（可选） |
| Source | string | 事件来源（可选） |
| JobID | string | 后台任务 ID（可选） |
| TurnID | string | 轮次 ID（可选） |

### 3.3 EnvelopeType（事件类型）

> 完整枚举见 `internal/event/contract/envelope.go`。下表按 Channel 分组列出主要类型。

| Channel | 类型 | 常量 | 说明 |
|---------|------|------|------|
| chat | text_delta | EnvelopeTypeTextDelta | 文本增量 |
| chat | text_done | EnvelopeTypeTextDone | 文本完成 |
| chat | tool_call | EnvelopeTypeToolCall | 工具调用开始 |
| chat | tool_result | EnvelopeTypeToolResult | 工具返回结果 |
| chat | state_delta | EnvelopeTypeStateDelta | 状态增量更新 |
| chat | transfer | EnvelopeTypeTransfer | Agent 转移控制权 |
| chat | runner_completion | EnvelopeTypeRunnerCompletion | 运行完成 |
| chat | context_usage | EnvelopeTypeContextUsage | 中间轮上下文填充 |
| chat | run_status | EnvelopeTypeRunStatus | Chat 运行态（queued/running/await/cancelled） |
| chat | error | EnvelopeTypeError | 错误事件 |
| chat | llm_retry | EnvelopeTypeLLMRetry | LLM 调用重试通知（T1.2：含 attempt/max_retries/delay_ms/error 元数据） |
| chat | session.status_changed | EnvelopeTypeSessionStatusChanged | 会话状态变更 |
| chat | metrics_updated | EnvelopeTypeMetricsUpdated | 指标更新 |
| chat | execution_progress | EnvelopeTypeExecutionProgress | 编排步骤进度（5-15s 等待期） |
| chat | activity_start / activity_delta / activity_done / activity_child_start | EnvelopeTypeActivity* | Activity-First 生命周期 |
| chat | spirit_* (assembled/completed/failed/interrupted/progress/...) | EnvelopeTypeSpirit* | Spirit 编排生命周期 |
| chat | butler.orchestration.* | EnvelopeTypeButlerOrchestration* | Butler 编排 |
| chat | skill.health_changed / skill.evolution_proposed | EnvelopeTypeSkill* | 技能演化 |
| chat | orchestration.evolution_suggested / orchestration.cache_hit | EnvelopeTypeOrchestration* | 编排演化 |
| chat | organization.created/updated/deleted | EnvelopeTypeOrganization* | 组织 CRUD |
| monitor | log | EnvelopeTypeLog | 进程 Gateway 文本日志 |
| monitor | flow_log | EnvelopeTypeFlowLog | Flow Log v2 流程步骤（schema: flow_log/v1） |
| monitor | mcp.session.reconnect | EnvelopeTypeMCPSessionReconnect | MCP 会话重连通知 |
| monitor | mcp.health.alert | EnvelopeTypeMCPHealthAlert | MCP 健康告警 |
| monitor | alert.notify | EnvelopeTypeAlertNotify | 监控告警通知 |
| monitor | monitor.auto_healed / monitor.self_check_completed | EnvelopeTypeMonitor* | 自愈/自检 |
| graph | graph_node_start / graph_node_end / graph_node_error / graph_node_custom | EnvelopeTypeGraphNode* | Graph 节点生命周期 |
| graph | graph_step / graph_execution_done / graph_task_status | EnvelopeTypeGraph* | Graph 步骤/执行 |
| graph | checkpoint | EnvelopeTypeCheckpoint | 检查点 |
| team | intent_pass | EnvelopeTypeIntentPass | 意图传递 |
| team | member_message_start / member_delta / member_message_done | EnvelopeTypeMember* | Team 成员消息 |
| team | team_run_started / team_run_finished / team_run_failed | EnvelopeTypeTeamRun* | Team 运行生命周期 |
| team | team_step_started / team_step_finished | EnvelopeTypeTeamStep* | Team 步骤 |
| team | team_summary | EnvelopeTypeTeamSummary | Team 运行摘要 |
| team | orchestration_agent_status | EnvelopeTypeOrchestrationAgentStatus | 编排 Agent 状态 |
| knowledge | knowledge_ingest | EnvelopeTypeKnowledgeIngest | 知识库文档入库进度 |
| — | token_usage | EnvelopeTypeTokenUsage | Token 用量详细记录 |
| — | user_feedback | EnvelopeTypeUserFeedback | 用户反馈 |
| — | borrow.approved / borrow.rejected / borrow.auto_approved | EnvelopeTypeBorrow* | 借调审批 |

**Channel 路由机制**：`RegisterChannelRoute(typ, channel)` 在 `init()` 时注册（OCP 合规）。`RouteChannel(env)` 查注册表，未注册类型回落到 TeamID 推断或 `chat` 默认。

### 3.4 Envelope 子结构

**EnvelopeContent**：
| 字段 | 类型 | 说明 |
|------|------|------|
| Text | string | 文本内容 |
| Reasoning | string | 推理内容（可选） |
| IsPartial | bool | 是否为增量片段 |

**EnvelopeToolCall**：
| 字段 | 类型 | 说明 |
|------|------|------|
| ID | string | 工具调用 ID |
| Name | string | 工具名称 |
| ArgumentsJSON | string | 参数 JSON |
| ResultJSON | string | 结果 JSON（可选） |
| Status | string | 状态 |
| DurationMS | int64 | 耗时毫秒（可选） |
| IsLongRunning | bool | 是否长时运行（可选） |
| ActivityKind | string | 活动类型：tool / skill / mcp / subagent 等（可选） |
| DisplayLabel | string | 展示标签（可选） |
| IconKey | string | 图标键（可选） |
| Summary | string | 摘要（可选） |
| StartedAt / FinishedAt | string | RFC3339 时间（可选） |
| ErrorCode | string | 错误码（可选；取值见 `ErrorCodeToolTimeout` / `ErrorCodeToolError` / `ErrorCodeConfirmationRequired` / `ErrorCodeConfirmationDenied` / `ErrorCodeConfirmationTimeout`） |
| AgentKey / AgentID / AgentName | string | Agent 关联（可选） |
| RunID / TraceID | string | 运行/追踪 ID（可选） |

**EnvelopeStateDelta**：
| 字段 | 类型 | 说明 |
|------|------|------|
| Operation | string | 操作类型：set / append / delete |
| Path | string | 状态路径（如 `state.key.path`） |
| ValueJSON | string | 值 JSON 字符串 |

**EnvelopeTransfer**：
| 字段 | 类型 | 说明 |
|------|------|------|
| FromAgent | string | 源 Agent |
| ToAgent | string | 目标 Agent |

**EnvelopeError**：
| 字段 | 类型 | 说明 |
|------|------|------|
| Type | string | 错误类型 |
| Code | string | 错误码（可选） |
| Message | string | 错误消息 |
| Hint | string | 提示（可选） |
| PendingID | string | 关联的 pending 消息 ID（可选） |

**EnvelopeUsage**：
| 字段 | 类型 | 说明 |
|------|------|------|
| PromptTokens | int | 输入 Token 数 |
| CompletionTokens | int | 输出 Token 数 |
| TotalTokens | int | 总 Token 数 |
| MaxTokens | int | 模型上下文窗口（可选） |
| ContextPromptTokens | int | 当前轮最大 prompt_tokens（可选） |
| TurnTotalTokens | int | 当前轮累计 prompt+completion（可选，ReAct 多次调用安全） |

**EnvelopeTokenUsage**：详细 Token 用量记录（30+ 字段），用于 `usage_rollup_consumer` 汇总到 `model_token_usage_hourly` 表。完整字段见 `contract/envelope.go`。

**EnvelopeActions**：
| 字段 | 类型 | 说明 |
|------|------|------|
| SkipSummarization | bool | 跳过摘要（可选） |

**EnvelopeTrace**：
| 字段 | 类型 | 说明 |
|------|------|------|
| AgentName | string | Agent 名称 |
| InvocationID | string | 调用 ID |
| StepCount | int | 步骤数 |
| DurationMS | int64 | 耗时毫秒（可选） |

### 3.5 Envelope 关键方法

| 方法 | 签名 | 说明 |
|------|------|------|
| NewEnvelope | `(typ EnvelopeType, author, sessionID string) Envelope` | 构造新信封，自动生成 ID / Timestamp / Version |
| RouteChannel | `(env Envelope) string` | 根据 Type 自动路由到 Channel（查注册表） |
| RegisterChannelRoute | `(typ EnvelopeType, channel string)` | 注册类型→Channel 路由（OCP） |
| RegisterChannelRoutes | `(channel string, types ...EnvelopeType)` | 批量注册 |
| MatchFilterKey | `(subscriberKey, eventKey string) bool` | FilterKey 双向前缀匹配 |
| Clone | `(e Envelope) Envelope` | 深拷贝（含所有指针字段和 map） |
| ContainsTag | `(e Envelope, tag string) bool` | 逗号分隔标签匹配（TrimSpace 后精确匹配） |
| ValidateErrorCode | `(e *EnvelopeToolCall)` | 校验 ErrorCode，未知值回落到 `ErrorCodeToolError` |

---

## 四、事件总线

### 4.1 Bus 接口

`internal/event/contract/bus.go`

```go
type Bus interface {
    Publish(ctx context.Context, envelope Envelope)
    Subscribe(opts SubscribeOptions) (<-chan Envelope, func())
    DropCount() uint64
}
```

实现：`internal/event/bus_adapter.go` 委托给 trpc-agent-go `frameworkbus.Bus[Envelope]`。

### 4.2 SubscribeOptions

| 字段 | 类型 | 说明 |
|------|------|------|
| SessionID | string | 按 SessionID 过滤 |
| TeamID | string | 按 TeamID 过滤 |
| Channel | string | 按 Channel 过滤 |
| FilterKey | string | 按 FilterKey 前缀匹配 |
| EventTypes | []EnvelopeType | 按事件类型白名单过滤 |
| LevelFilter | string | 日志级别过滤（DEBUG/INFO/WARN/ERROR） |
| Priority | ChannelPriority | 订阅优先级：Critical 先于 Normal 投递 |
| BufferSize | int | Channel 容量（默认 128，上限 512） |
| Reliable | bool | 可靠订阅（关键事件 BlockUpTo 语义） |
| DropPolicy | DropPolicy | 背压策略 |
| BlockFor | time.Duration | BlockUpTo 等待时长 |
| Selector | func(EnvelopeType) bool | 自定义类型过滤器 |

### 4.3 背压策略

| 策略 | 常量 | 行为 |
|------|------|------|
| DropOldest | DropPolicy(iota) | 缓冲满时淘汰最旧事件 |
| DropNewest | DropPolicy(1) | 缓冲满时丢弃最新事件 |
| BlockUpTo | DropPolicy(2) | 阻塞等待指定时长，超时后回退到 DropOldest |

**可靠订阅**：`Reliable=true` 或关键事件类型（见 §五可靠性分级）自动使用 BlockUpTo 语义。Publish 时 Critical 优先级订阅者优先投递。

**丢弃可观测**：drop 时通过 `busAdapter` 的 `DropLogger` 回调递增 Prometheus `EventBusDropped` 并写 loggateway Warn 日志。

### 4.4 路由匹配

Publish 时自动执行：
1. 若 Envelope.Channel 为空，调用 `RouteChannel(env)` 自动填充
2. 遍历所有订阅者，按 SubscribeOptions 中的过滤条件匹配
3. 匹配顺序：SessionID → TeamID → Channel → FilterKey → EventTypes → LevelFilter → Selector

### 4.5 FilterKey 匹配规则

```go
func MatchFilterKey(subscriberKey, eventKey string) bool {
    if subscriberKey == "" || eventKey == "" {
        return true
    }
    sk := subscriberKey + "/"
    ek := eventKey + "/"
    return strings.HasPrefix(sk, ek) || strings.HasPrefix(ek, sk)
}
```

双向前缀匹配：`agent_a` 匹配 `agent_a/agent_b`，`agent_a/agent_b` 也匹配 `agent_a`。

---

## 五、事件可靠性分级（AS-EVT-01）

`internal/event/contract/reliability.go`

| 级别 | 事件类型 | 可靠性保证 | 持久化 |
|------|---------|-----------|--------|
| Critical | ToolResult / Error / RunnerCompletion / Checkpoint | ~~WBPF（先写后发）+ 重试~~ 已于 Phase 1c-2 移除；订阅者需幂等 | ~~SQLite WAL~~ 已删除 |
| Important | StateDelta / TokenUsage / RunStatus / SessionStatusChanged / GraphNodeEnd / TeamRunFinished / TeamRunFailed / Spirit* / UserFeedback / Activity* | BlockUpTo + 异步持久化 | SQLite EventStore |
| Informational | TextDelta / FlowLog / Log / MemberDelta / 其他 | 尽力而为 | 不持久化 |

**分类入口**：
- `ClassifyEventReliability(t) EventReliability` — 单一真相源
- `IsCriticalWBPFType(t) bool` — 是否需要 WBPF（legacy，WAL 已删除）
- `RequiresBlockUpTo(t) bool` — 是否必须 BlockUpTo 投递（Critical + Important）

**WAL 实现**：~~`internal/event/wal.go` + `wal_storage.go`~~ 已于 Phase 1c-2 删除
- ~~Critical 事件先写入 SQLite WAL 表，再 publish~~
- ~~WAL 写失败则**不 publish**（避免不一致）~~
- ~~进程重启时从 WAL 恢复未 publish 的事件~~
- 订阅者需幂等，重放走 Activity 记录（`listActivities` API）

### 5.1 Activity-First 事件持久化（ADR-02）

> **详见**：[ADR-02: Activity-First 事件持久化策略](../reports/2026-06-25-review-adr-activity-event-persistence.md)

Activity 事件流（`biz.ActivityEventBus`）采用**并行异步**持久化，替代 legacy WBPF：

| 维度 | Legacy WBPF（已弃用） | AF 并行异步（现行） |
|------|----------------------|-------------------|
| 持久化时机 | 先写 WAL，成功后才 publish | fire-and-forget（独立 worker goroutine） |
| 推送时机 | 持久化成功后 | 同步（保留 per-activity FIFO） |
| 失败处理 | 不 publish（强一致） | 重试 5 次（100/200/400/800/1600ms）+ dead-letter 环形缓冲（cap=512）+ API backfill |
| 阻塞 | DB I/O 阻塞推送（~50-200ms） | 不阻塞（~5ms 推送延迟） |
| 适用范围 | Legacy envelope Critical 事件（待退役） | Activity 事件流（chat 渲染主路径） |

---

## 六、事件投影

### 6.1 EventProjector

`internal/agent/event_projector.go`

将 trpc-agent-go `*trpcevent.Event` 投影为 `event.Envelope`，保留完整元数据。

> **Deprecated**：Activity-First 架构上线后，新代码应使用 `ActivityProjector`。`EventProjector` 将在前端完全迁移到 Activity 消费后移除。

**ProjectMeta**：
| 字段 | 说明 |
|------|------|
| SessionID | 会话 ID |
| RequestID | 请求 ID |
| InvocationID | 调用 ID |
| ParentInvocationID | 父调用 ID |
| TeamID | Team ID |
| Branch | 分支 |
| FilterKey | 过滤键 |
| RunID | 运行 ID |
| TraceID | 追踪 ID |
| AgentID | Agent ID |
| AgentDisplayName | Agent 展示名 |
| ContextWindow | 上下文窗口大小 |
| TurnPromptTokens / TurnCompletionTok | 当前轮累计 Token |
| MemberAgentKeys | Team member_* 信封作者白名单 |
| Source | 事件来源 |
| TaskContent | 根任务用户输入文本（用于 Activity 标题） |

**投影规则**（`internal/event/framework_adapter.go` `FromFrameworkEvent`）：
- trpc Event.Branch / FilterKey / Tag / Extensions / Actions 直接映射到 Envelope
- ProjectMeta 中的字段作为默认值（Event 字段优先）
- LongRunningToolIDs 映射为 ToolCall.IsLongRunning
- Extensions 从 `map[string]json.RawMessage` 转为 `map[string]string`（简单字符串去引号，否则保留原始 JSON）
- Actions.SkipSummarization 直接映射
- LLM Response.Choices 拆分为 text_delta / text_done 事件
- RunnerCompletion 映射为 runner_completion 事件

### 6.2 EventBridge（Graph）

`internal/graph/trpc/event_bridge.go`

将 Graph 执行事件桥接到 EventBus，映射 trpc-agent-go Graph ObjectType 到 EnvelopeType。

### 6.3 Flow Log v2（替代 SlogBridge）

| 文件 | 职责 |
|------|------|
| `internal/event/flow_log.go` | FlowLogEntry 数据结构、`FlowLogSchemaVersion = "flow_log/v1"`、stepTitleRegistry |
| `internal/event/flow_tracker.go` | FlowTracker：LogStart/LogDone/LogError + emit EnvelopeTypeFlowLog |
| `internal/event/trace_emitter.go` | TraceEmitter = FlowTracker + ObserveFrameworkEvent + EmitProgress（execution_progress） |
| `internal/event/trace_context.go` | TraceContext（trace_id / run_id / agent_key） |
| `internal/event/flow_context.go` | FlowContext（步骤计时）+ `WithTraceEmitter` / `TraceEmitterFromContext`（含 Deprecated `WithFlowLogger` / `FlowLoggerFromContext` / `NewFlowLogger` 别名） |
| `internal/event/span_collector.go` / `usage_aggregator.go` | Span 收集与 Usage 汇总 |

- Monitor 业务日志主类型为 **`flow_log`**（`schema_version: flow_log/v1`），非全局 `slog` 桥接。
- **`slog_bridge.go` 已删除**（2026-05-20）；`LOG_BRIDGE_ENABLED` 已废弃。
- 进程 Gateway 文本日志仍为 `EnvelopeTypeLog`（如 `PluginSafeLogger`），与 `flow_log` 前端分流。
- 详见 [52-flow-logger.design.md](./52-flow-logger.design.md)。

---

## 七、事件消费

### 7.1 EventBusConsumer（核心 4 handler）

`internal/biz/event_bus_consumer.go`

`Start()` 以 `Reliable=true` 全局订阅 SessionBus，经 `envelopeToDomainEvent` 转换后按 Type 分派。

| Handler | 文件 | 职责 |
|---------|------|------|
| eventBufferHandler | `event_bus_buffer_handler.go` | 所有 Envelope 写入环形 Buffer（断连重放） |
| runnerCompletionHandler | `event_bus_runner_handler.go` | RunnerCompletion → TurnMemoryWorker + Monitor 持久化（`RecordRunnerCompletion`）+ Usage 记录（`CHAT_RECORD_RUNNER_USAGE`）+ TraceProjector + 发布 token_usage |
| stateDeltaHandler | `event_bus_state_handler.go` | StateDelta → SessionUsecase.ApplyStateDelta + UpdateRunnerSnapshotJSON（Path=="__state__"）+ TRPC KV 同步 |
| eventPersistHandler | `event_persist_handler.go` | 异步持久化 Envelope → event_store（排除 log/flow_log/text_delta/member_delta，有界队列 512，Critical 同步写） |

### 7.2 EventBusSideConsumers（旁路 6 typed consumer）

`internal/biz/event_bus_side_consumers.go`

每个 consumer 独立订阅 SessionBus（部分同时订阅 MonitorBus），按 EventTypes 过滤。

| Consumer | 文件 | 职责 |
|----------|------|------|
| toolCallConsumer | `event_bus_tool_call_consumer.go` | tool_call / tool_result → ToolInvocation 记录 |
| callbackConsumer | `event_bus_callback_consumer.go` | runner_completion 终态 → Webhook 回调 |
| messageStoreConsumer | `event_bus_message_store_consumer.go` | member_message_* → 消息存储 |
| flowLogPersistConsumer | `event_bus_flow_log_consumer.go` | flow_log → flow_log_events 表（订阅 SessionBus + MonitorBus） |
| userFeedbackConsumer | `event_bus_user_feedback_consumer.go` | user_feedback → Monitor + Memory |
| usageRollupConsumer | `event_bus_usage_rollup_consumer.go` | token_usage → Usage 汇总 |

### 7.3 DomainEvent 适配

`internal/biz/domain_event.go` + `internal/biz/domain_event_adapter.go`

DomainEvent 是 biz 层的领域事件模型，通过 `eventBusAdapter` 与 EventBus 双向桥接：
- `DomainEventPublisher` → `eventBusAdapter.PublishDomainEvent()` → `Bus.Publish()`
- `Bus.Subscribe()` → `eventBusAdapter.SubscribeDomainEvents()` → `<-chan DomainEvent`

**DomainEventType 枚举**：
runner_completion / state_delta / error / graph_node_start / graph_node_end / graph_node_error / graph_interrupt / text_delta / tool_call / tool_result

---

## 八、Session State Delta

### 8.1 SessionRepository 接口

`internal/biz/session/usecase.go` + `internal/biz/session/state_usecase.go`

```go
GetSessionState(ctx context.Context, sessionID string) (map[string]string, error)
SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error
PatchSessionState(ctx context.Context, sessionID string, sets map[string]string, deletes []string) error
ApplyStateDelta(ctx context.Context, sessionID string, delta StateDelta) error
```

`SessionUsecase` 通过 Facade 模式委托到 `SessionMessageUsecase` → `SessionStateUsecase` → `StateRepo`。

### 8.2 ApplyStateDelta

`internal/biz/session/state_usecase.go`

```go
func (uc *SessionStateUsecase) ApplyStateDelta(ctx context.Context, sessionID string, delta StateDelta) error
```

操作类型：
| Operation | 行为 |
|-----------|------|
| set | `PatchSessionState(sets={path: valueJSON})` |
| delete | `PatchSessionState(deletes=[path])` |
| append / 其他 | 回落到 set 语义（当前实现未单独处理 append） |

### 8.3 持久化

`internal/data/session_state_repo.go` + `internal/data/ent/schema/session.go` + `internal/data/ent/schema/session_runtime.go`

- `sessions.state_json` 字段（TEXT，默认 `{}`）存储 `map[string]string` 的 JSON 序列化
- `PatchSessionState` 使用 SQLite `json_set` / `json_remove` 原子更新（避免读-改-写）
- 会话表拆分后，`session_runtime` 表亦持有 `state_json`（见迁移 `20260708_session_table_split.sql`）

---

## 九、事件缓冲与重放

### 9.1 Buffer

`internal/event/buffer.go`

环形缓冲，按 SessionID 分组存储事件。

| 参数 | 值 | 说明 |
|------|-----|------|
| 容量 | 200 | 每个 Session 的最大事件数 |
| TTL | 30 分钟 | 无访问后淘汰 |
| 淘汰周期 | 5 分钟 | 后台 goroutine 定期清理 |

### 9.2 Replay

```go
func (b *Buffer) Replay(sessionID, lastEventID string) []Envelope
```

从 lastEventID 之后的事件开始返回，用于 WebSocket 断连重放。

---

## 十、WebSocket 传输

### 10.1 WSServer

`internal/server/ws.go`（+ `ws_conn.go` / `ws_conn_manager.go` / `ws_codec.go` / `ws_message_handler.go` / `ws_io_pump.go` / `ws_event.go` / `ws_priority.go`）

统一 WebSocket 网关，端点 `/v1/ws`。

**连接参数**：
| 参数 | 说明 |
|------|------|
| session_id | 会话 ID（必填，`*` 为全局监控模式） |
| token | 认证 Token |
| last_event_id | 断连重放起始 ID |
| filter_key | 事件过滤键 |
| log_enabled | 是否接收日志事件 |
| probe / health | 探活连接（不订阅事件） |

**下行消息格式**：
```json
{
  "direction": "server_to_client",
  "channel": "chat",
  "type": "replay",
  "envelope": { ... }
}
```

**上行消息类型**：
| Type | 说明 |
|------|------|
| ping | 心跳 |
| subscribe | 订阅 Channel（可带 filter_key） |
| unsubscribe | 取消订阅 Channel（chat/system 不可取消） |
| cancel | 取消当前运行 |
| enable_log | 开关日志事件（受 `ProcessLogEnabled` 限制） |
| user_message | 发送用户消息 |
| enqueue_message | 排队用户消息 |
| sync_request | T3.4 断连重连后的 revision-based 同步重放请求，payload `{ session_id, after_revision }` |

**sync_request 协议（T3.4）**：

客户端在 WS 重连后发送 `sync_request` 上行消息，请求服务端重放 `session_revision > after_revision` 的 Envelope。与基于 `last_event_id` 的 replay 互补：

| 维度 | event-ID replay（`last_event_id` URL 参数） | revision sync（`sync_request` 上行消息） |
|------|---------------------------------------------|------------------------------------------|
| 触发时机 | 连接建立时 | 重连后客户端主动发起 |
| 数据源 | `event.Buffer` 环形缓冲（内存） | `event_store` 表（持久化） |
| 回放窗口 | Buffer 容量（默认 256） | 24 小时回溯窗口（`syncReplayLookbackWindow`） |
| 上限 | 无显式上限 | 500 条（`syncReplayMaxEnvelopes`） |
| 排序 | Buffer 入队顺序 | 按 `SessionRevision` 升序（INV4） |
| 通道过滤 | 无 | 仅回放当前连接已订阅的 Channel |
| 降级 | Buffer 满则丢失 | EventStore 未配置则静默跳过 |

**处理流程**（`internal/server/ws_sync_request.go`）：
1. 同步阶段（在 readPump goroutine 中）：校验 `eventStoreUsecase != nil`、解析 payload、提取 `session_id` / `after_revision`；`after_revision <= 0` 直接返回
2. 异步阶段（`safego.Go` 启动独立 goroutine）：以 `wc.contextOrBackground()` 派生 10s 超时 ctx，查询 EventStore，过滤 `revision > after_revision` 且 Channel 已订阅的记录，按 `SessionRevision` 升序排序后通过 `enqueueSystem` 投递

**接口窄化（BA6）**：`WSServer` 持有 `EventStoreLister` 窄接口（仅 `List` 方法）而非具体 `*biz.EventStoreUsecase`，构造函数处理 Go nil-interface 陷阱（`*T` 为 nil 时赋给接口会得到非 nil 接口包装 nil 指针）。

**Channel 路由**（`wsBuildChannels`）：
| Channel | 默认订阅 | 包含事件类型 |
|---------|----------|-------------|
| chat | ✅（始终） | text_delta / text_done / tool_call / tool_result / state_delta / transfer / runner_completion / run_status / error / llm_retry / execution_progress / activity_* / spirit_* / butler.* / skill.* / organization.* |
| system | ✅（始终） | connected / pong / server_shutdown / replay_* |
| monitor | ❌（需 enable_log 或 subscribe；global 默认开） | log / flow_log / mcp.session.reconnect / mcp.health.alert / alert.notify / monitor.* |
| team | ❌（需 subscribe；global 默认开） | member_* / team_* / intent_pass / team_summary / orchestration_agent_status |
| graph | ❌（需 subscribe；global 默认开） | graph_* / checkpoint |
| knowledge | ❌（需 subscribe；global 默认开） | knowledge_ingest |

**日志门控**：`EnvelopeTypeLog` 受 `log_enabled` / `enable_log` 控制；`flow_log` 始终投递（不经 log 门控）。

**下行 replay 类型**：`replay_start` / `replay` / `replay_end`（非仅 `replay`）。

**背压策略**（`setupEventSubscription`）：
- 普通会话连接：BufferSize=256，Reliable=true（关键事件 BlockUpTo）
- 全局监控连接：BufferSize=256，Reliable=false（可容忍丢失）
- MonitorBus 订阅：BufferSize=128，DropPolicy=DropNewest

**连接限制**（`conf/runtime_helpers.go` 默认值）：
- 每个 Session 最多 5 个连接（`WS_MAX_SESSION_CONNS`）
- 全局监控模式最多 3 个连接（`WS_MAX_GLOBAL_MONITOR_CONNS`）

---

## 十一、前端架构

### 11.1 类型定义

`web/src/realtime/envelope.ts`（基础类型）+ `web/src/features/chat/envelope.ts`（Chat 域 re-export + 辅助）

前端 Envelope 类型与后端 `contract.Envelope` JSON 结构一一对应。辅助：`envelopeRunStatus.ts`（run_status 解析）、`teamRunEventFromEnvelope.ts`（Team 事件投影）、`envelopeToolCall.ts`（工具调用解析）。

### 11.2 WsTransport

`web/src/realtime/ws-transport.ts`

WebSocket 传输层，职责：
- 连接管理（自动重连，指数退避，最大 30s）
- 心跳（25s 间隔）
- 消息发送（离线排队）
- lastEventId 跟踪（断连重放）
- 服务器关机通知
- T3.4：维护 `RevisionTracker`，每条携带 `session_revision` 的 Envelope 更新对应 session 的最后已知 revision；重连后调用 `requestSyncReplay` 发送 `sync_request` 上行消息

### 11.2.1 RevisionTracker（T3.4）

`web/src/realtime/event_replay.ts`

Revision-based 同步重放前端模块，与 event-ID replay 互补：

| 导出 | 职责 |
|------|------|
| `RevisionTracker` | Per-session revision 跟踪器（`update(sessionId, revision)` 单调递增 / `get(sessionId)` / `clear` / `clearAll`） |
| `buildSyncRequest(sessionId, afterRevision)` | 构造 `sync_request` 上行消息（`afterRevision <= 0` 返回 null） |
| `requestSyncReplay(send, sessionId, afterRevision)` | 通过 `send` 发送 `sync_request`，返回是否实际发送 |

### 11.3 useEnvelopeStream

`web/src/realtime/useEnvelopeStream.ts`

Vue composable，封装 WsTransport + EnvelopeDispatcher：
- `onType(type, handler)` — 按事件类型注册回调
- `onChannel(channel, handler)` — 按 Channel 注册回调
- `subscribe(channel)` / `unsubscribe(channel)` — 动态订阅/取消
- `enableLog(enabled)` — 开关日志
- `cancel()` — 取消运行

**Chat 域衍生 composable**（`web/src/features/chat/useEnvelopeStream.ts`）：
- `createChatStream(sessionId)` / `useChatStream(sessionId)` — 聚合 chat + system 通道
- `createTeamStream(sessionId, teamId?)` / `useTeamStream(...)` — 聚合 member_* / team_* 事件
- `useMonitorStream(sessionId)` — Monitor/log 流

**Knowledge 域**：`web/src/features/knowledge/useKnowledgeIngestWs.ts` — 订阅 knowledge_ingest

### 11.4 Monitor 实时事件

`web/src/components/monitor/RealtimeEvents.vue`

Monitor `/monitor` Events Tab 生产组件：合并 WS 运行时 Envelope 与持久化 Monitor Events，支持分类过滤、Runs 关联跳转、暂停/清除。

---

## 十二、事件持久化

### 12.1 存储

`event_store` Ent 表（`internal/data/ent/schema/event_store.go`），字段：id / session_id / type / author / channel / envelope_json / created_at。索引：session_id+created_at、type、created_at。

写入：`eventPersistHandler` 异步持久化（`shouldPersistEnvelope` 排除 `log` / `flow_log` / `text_delta` / `member_delta`）。
- 队列大小：512（`EVENT_STORE_PERSIST_QUEUE` 可配）
- Critical + Important 事件队列满时同步写（不丢弃）
- Informational 事件队列满时丢弃 + Warn 日志

TTL：`EventStoreCleanup` 每小时执行，`EVENT_STORE_TTL_DAYS` 默认 7 天。

### 12.2 回放 API

`GET /v1/events?session_id=&since=&until=&type=&limit=&offset=`

- Proto：`api/kratos/event/v1/event.proto` → `EventService.ListEvents`
- Service：`internal/service/event.go`
- 会话存在性校验：`SessionUsecase.Get(ctx, sessionID)`
- 时间解析：RFC3339Nano / RFC3339

---

## 十三、Chat 会话事件检视

> **产品决策**：不增加第四列固定侧边栏（左 Entity / 中 Message / 右 Session 已占满）。采用 **Dialog 双 Tab**，与现有 `SessionTimelineDialog` 入口合并。

### 13.1 与 Monitor 分工

| 维度 | Monitor `RealtimeEvents` | Chat Inspector |
|------|--------------------------|----------------|
| 范围 | 全局 / 多会话 | **当前 session** |
| 数据源 | WS + Monitor Events API | WS + `GET /v1/events` |
| 入口 | `/monitor?tab=events` | Chat 会话菜单 / MessagePanel 工具栏 |
| 侧重 | Runs 关联、运维分类 | Branch 树、StateDelta、FilterKey/Tag |

### 13.2 布局

```
ChatPage
├── 三栏布局（不变）
└── SessionTimelineDialog（扩展）
      Tab「历史 Trace」— 现有 HTTP SessionTimeline
      Tab「实时 Envelope」— SessionEventInspectorPanel
            ├─ EventFilterBar
            ├─ BranchTree（左 30%）
            └─ 事件列表（StateDeltaIndicator / TransferBadge / ToolCall）
```

**入口**：
1. `ChatSessionSidebar` 会话菜单「历史追踪」→ Dialog 默认 Trace Tab
2. `ChatMessagePanel` 头部「事件」按钮 → Dialog 默认 Envelope Tab

### 13.3 前端分层

| 层级 | 路径 | 职责 |
|------|------|------|
| API | `features/event/api.ts` | `listSessionEvents` → `GET /v1/events` |
| 纯函数 | `features/chat/eventFilter.ts` | filterEnvelopes / buildBranchTree / EventFilterState |
| Composable | `features/chat/composables/useEventFilter.ts` | 过滤状态 + computed（filteredEvents / branchTree） |
| Composable | `features/chat/composables/useChatEventInspector.ts` | WS + 历史合并、暂停（MAX_EVENTS=2000） |
| Composable | `features/chat/composables/useChatTraceAndArtifacts.ts` | `openSessionTrace(id, tab?)` / `openSessionEvents(id)` |
| 展示 | `components/chat/EventFilterBar.vue` | 过滤控件 |
| 展示 | `components/chat/BranchTree.vue` + `BranchTreeNode.vue` | 调用树 |
| 展示 | `components/chat/StateDeltaIndicator.vue` | StateDelta 行 |
| 展示 | `components/chat/TransferBadge.vue` | Transfer 标签 |
| 展示 | `components/chat/SessionEventInspectorPanel.vue` | Tab 内容容器 |
| 容器 | `components/chat/SessionTimelineDialog.vue` | Tab 切换 + Trace Tab（initialTab prop） |

Branch 树由 `invocation_id` / `parent_invocation_id` 在线推导，不新增后端 API。

### 13.4 EventFilterState

| 字段 | 说明 |
|------|------|
| typeFilter | `all` 或 EnvelopeType |
| branchPrefix | Branch 前缀匹配 |
| tag | Tag 逗号分隔精确匹配 |
| keyword | 搜索 type/author/content/tool |
| filterKey | FilterKey 前缀匹配 |

> Monitor 域 另有 `features/chat/useEventFilter.ts`（generic 过滤 composable，支持 types/channels/filterKey/author/keyword），与 Chat Inspector 的 `composables/useEventFilter.ts` 职责不同。

---

## 十四、涉及文件清单

### 后端

| 文件 | 说明 |
|------|------|
| `internal/event/contract/envelope.go` | Envelope 结构 + EnvelopeType 枚举 + Clone / MatchFilterKey / ContainsTag / RouteChannel / RegisterChannelRoute |
| `internal/event/contract/bus.go` | Bus 接口 + SubscribeOptions + DropPolicy + ChannelPriority |
| `internal/event/contract/reliability.go` | EventReliability 分级 + ClassifyEventReliability / IsCriticalWBPFType / RequiresBlockUpTo |
| `internal/event/envelope.go` | contract 类型的向后兼容 type alias |
| `internal/event/bus.go` + `bus_adapter.go` | NewBus + 框架 bus.Bus 适配（DropLogger） |
| `internal/event/buffer.go` | 环形缓冲 + TTL 淘汰 + Replay |
| `internal/event/infra.go` | Infra 双 Bus（SessionBus / MonitorBus）+ BindInfra + Publish（WBPF 路由）+ InfraProviderSet |
| `internal/event/wal.go` + `wal_storage.go` | EventWAL（WBPF for Critical events） |
| `internal/event/event_reliability.go` | reliability 兼容别名 |
| `internal/event/flow_log.go` | FlowLogEntry 数据结构 + stepTitleRegistry |
| `internal/event/flow_tracker.go` | FlowTracker（LogStart/LogDone/LogError + emit FlowLog） |
| `internal/event/trace_emitter.go` | TraceEmitter（FlowTracker + EmitProgress） |
| `internal/event/trace_context.go` | TraceContext |
| `internal/event/flow_context.go` | FlowContext + WithTraceEmitter（含 Deprecated 别名） |
| `internal/event/span_collector.go` / `usage_aggregator.go` | Span 收集与 Usage 汇总 |
| `internal/event/framework_adapter.go` | FromFrameworkEvent（trpc Event → Envelope） |
| `internal/event/framework_events.go` | WrapFrameworkEvents（tee 框架事件） |
| `internal/agent/event_projector.go` | trpc Event → Envelope 投影（Deprecated，改用 ActivityProjector） |
| `internal/agent/turn_helpers.go` | ConsumeEventStream 事件消费 |
| `internal/graph/trpc/event_bridge.go` | Graph 事件桥接 |
| `internal/biz/domain_event.go` | DomainEvent 领域模型 |
| `internal/biz/domain_event_adapter.go` | DomainEvent ↔ EventBus 适配 |
| `internal/biz/event_bus_consumer.go` | EventBusConsumer 编排（核心 4 handler） |
| `internal/biz/event_bus_buffer_handler.go` | 缓冲写入 handler |
| `internal/biz/event_bus_runner_handler.go` | RunnerCompletion handler |
| `internal/biz/event_bus_state_handler.go` | StateDelta handler |
| `internal/biz/event_persist_handler.go` | 异步持久化 handler |
| `internal/biz/event_bus_side_consumers.go` | EventBusSideConsumers 编排（旁路 6 typed consumer） |
| `internal/biz/event_bus_tool_call_consumer.go` | ToolCall 记录 consumer |
| `internal/biz/event_bus_callback_consumer.go` | Webhook 回调 consumer |
| `internal/biz/event_bus_message_store_consumer.go` | 消息存储 consumer |
| `internal/biz/event_bus_flow_log_consumer.go` | FlowLog 持久化 consumer |
| `internal/biz/event_bus_user_feedback_consumer.go` | 用户反馈 consumer |
| `internal/biz/event_bus_usage_rollup_consumer.go` | Usage 汇总 consumer |
| `internal/biz/event_store.go` | EventStoreUsecase（List / PurgeExpired / Exists） |
| `internal/biz/session/state.go` + `state_usecase.go` | SessionUsecase.ApplyStateDelta / GetSessionState / SaveSessionState（Facade） |
| `internal/data/session_state_repo.go` | Session State 持久化（json_set / json_remove） |
| `internal/data/ent/schema/session.go` + `session_runtime.go` | state_json 字段 |
| `internal/data/event_store_repo.go` | EventStore Repo |
| `internal/data/ent/schema/event_store.go` | event_store 表 Schema |
| `internal/server/ws.go` + `ws_*.go` | WebSocket 统一网关 |
| `internal/server/ws_sync_request.go` | T3.4 sync_request 上行处理 + revision-based 重放 |
| `internal/service/event.go` | 回放 API Service |
| `internal/service/run_status_publish.go` | run_status Envelope 发布 |
| `internal/service/chat_run_gateway.go` | Chat 运行态 + run_status |
| `internal/service/monitor_notify.go` | alert.notify 发布 |
| `internal/cronrunner/jobs/event_store_cleanup.go` | TTL 清理 |
| `internal/cronrunner/jobs/flow_log_cleanup.go` | FlowLog TTL 清理 |
| `internal/metrics/vars.go` | EventBusPublished / EventBusDropped 指标 |
| `api/kratos/event/v1/event.proto` | EventService.ListEvents Proto 契约 |

### 前端

| 文件 | 说明 |
|------|------|
| `web/src/realtime/envelope.ts` | 前端 Envelope 基础类型 |
| `web/src/realtime/ws-transport.ts` | WsTransport |
| `web/src/realtime/event_replay.ts` | T3.4 RevisionTracker + buildSyncRequest + requestSyncReplay |
| `web/src/realtime/useEnvelopeStream.ts` | useEnvelopeStream composable |
| `web/src/realtime/dispatcher.ts` | EnvelopeDispatcher |
| `web/src/realtime/globalWsHub.ts` | 全局 WS Hub |
| `web/src/features/chat/envelope.ts` | Chat 域 Envelope re-export |
| `web/src/features/chat/envelopeRunStatus.ts` | run_status 解析 |
| `web/src/features/chat/envelopeToolCall.ts` | 工具调用解析 |
| `web/src/features/chat/useEnvelopeStream.ts` | Chat/Team/Monitor 衍生 composable |
| `web/src/features/chat/useEventFilter.ts` | Monitor 域 generic 过滤 composable |
| `web/src/features/event/api.ts` | 回放 API 门面 |
| `web/src/features/event/types.ts` | 回放 API 类型 |
| `web/src/features/chat/eventFilter.ts` | 过滤 + Branch 树纯函数 |
| `web/src/features/chat/composables/useEventFilter.ts` | Chat Inspector 过滤状态 |
| `web/src/features/chat/composables/useChatEventInspector.ts` | WS + 历史合并、暂停 |
| `web/src/features/chat/composables/useChatTraceAndArtifacts.ts` | openSessionTrace / openSessionEvents |
| `web/src/features/knowledge/useKnowledgeIngestWs.ts` | knowledge_ingest WS |
| `web/src/components/monitor/RealtimeEvents.vue` | Monitor Events Tab（生产） |
| `web/src/components/chat/SessionEventInspectorPanel.vue` | Envelope Tab 容器 |
| `web/src/components/chat/EventFilterBar.vue` | 过滤栏 |
| `web/src/components/chat/BranchTree.vue` + `BranchTreeNode.vue` | 分支追踪树 |
| `web/src/components/chat/StateDeltaIndicator.vue` | 状态变更指示器 |
| `web/src/components/chat/TransferBadge.vue` | Agent 转移标签 |
| `web/src/components/chat/SessionTimelineDialog.vue` | Trace + Envelope 双 Tab |
