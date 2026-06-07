# Event 事件系统模块 — 实现设计文档

> 对应需求：`34 event-system.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

基于事件总线的发布/订阅机制，支持系统内部组件间的异步事件通信。核心组件为 `event.Bus`（发布/订阅 + 背压策略）+ `event.Envelope`（统一事件信封）+ `event.Buffer`（环形缓冲 + 断连重放），通过 WebSocket 统一传输至前端。

**已实现能力**：
1. EventBus 发布/订阅 + 三种背压策略（DropOldest / DropNewest / BlockUpTo）+ 可靠订阅 + ChannelPriority 投递顺序
2. Envelope 统一事件信封，携带完整元数据（StateDelta / Extensions / FilterKey / Branch / Tag / Actions / Trace）
3. EventProjector 将 trpc-agent-go Event 投影为 Envelope（含 Activity 元数据、LongRunningToolIDs）
4. EventBusConsumer 拆分为 buffer / runner / state / persist 等多 handler，单一职责消费
5. WebSocket 统一传输（Channel 路由：chat / monitor / team / graph / knowledge / system）
6. EventBuffer 环形缓冲 + TTL 淘汰 + lastEventID 断连重放
7. Flow Log v2（`flow_log` + TraceEmitter / SysLog*，SlogBridge 已删除）
8. 前端 Envelope 类型 + WsTransport + useEnvelopeStream；Monitor `RealtimeEvents` 实时事件面板
9. 事件持久化（SQLite `event_store` + 异步 `eventPersistHandler`，排除 log/flow_log）
10. 事件回放 API（`GET /v1/events` 按 session/时间/类型分页查询）
11. Chat 会话事件检视（SessionTimelineDialog 双 Tab + Inspector 组件：EventFilterBar / BranchTree / StateDeltaIndicator / TransferBadge / SessionEventInspectorPanel）

**未实现能力**：
1. 工具生命周期事件与自动触发（ToolRegistered / ToolUpdated / ToolRemoved，见需求 §2.10）

---

## 二、架构总览

```
trpc-agent-go Runner
       │
       │ *trpcevent.Event
       ▼
 EventProjector ──── 投影为 Envelope ────┐
       │                                  │
       │ event.Bus.Publish()              │
       ▼                                  ▼
  ┌─────────────────────────────────────────────┐
  │                event.Bus                     │
  │  (发布/订阅 + 背压策略 + FilterKey 路由)      │
  └──────┬──────────┬──────────┬────────────────┘
         │          │          │
         ▼          ▼          ▼
   EventBusConsumer  ─┬─ eventBufferHandler   (环形缓冲 Append)
   (多 handler SRP)   ├─ runnerCompletionHandler (Monitor / Usage / TurnMemory)
                      ├─ stateDeltaHandler    (ApplyStateDelta)
                      └─ eventPersistHandler  (异步持久化 → event_store)
         │          │
         ▼          ▼
   SessionUsecase   前端 WsTransport
   (ApplyStateDelta)   + useEnvelopeStream
                        + Monitor RealtimeEvents
                        + Chat Inspector (SessionTimelineDialog 双 Tab)
```

---

## 三、核心数据模型

### 3.1 Envelope（事件信封）

`internal/event/envelope.go`

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
| Channel | string | 路由通道（chat / monitor / team / graph） |
| Content | *EnvelopeContent | 文本内容（可选） |
| ToolCall | *EnvelopeToolCall | 工具调用信息（可选） |
| StateDelta | *EnvelopeStateDelta | 状态增量（可选） |
| Transfer | *EnvelopeTransfer | Agent 转移信息（可选） |
| Error | *EnvelopeError | 错误信息（可选） |
| Usage | *EnvelopeUsage | Token 用量（可选） |
| Extensions | map[string]string | 命名空间化扩展元数据（可选） |
| Actions | *EnvelopeActions | 流控制提示（可选） |
| Trace | *EnvelopeTrace | 执行追踪（可选） |
| Metadata | map[string]any | 附加元数据（可选） |

### 3.2 EnvelopeType（事件类型）

| 类型 | 常量 | Channel | 说明 |
|------|------|---------|------|
| text_delta | EnvelopeTypeTextDelta | chat | 文本增量 |
| text_done | EnvelopeTypeTextDone | chat | 文本完成 |
| tool_call | EnvelopeTypeToolCall | chat | 工具调用开始 |
| tool_result | EnvelopeTypeToolResult | chat | 工具返回结果 |
| state_delta | EnvelopeTypeStateDelta | chat | 状态增量更新 |
| transfer | EnvelopeTypeTransfer | chat | Agent 转移控制权 |
| runner_completion | EnvelopeTypeRunnerCompletion | chat | 运行完成 |
| error | EnvelopeTypeError | chat | 错误事件 |
| log | EnvelopeTypeLog | monitor | 日志事件 |
| graph_node_start | EnvelopeTypeGraphNodeStart | graph | Graph 节点开始 |
| graph_node_end | EnvelopeTypeGraphNodeEnd | graph | Graph 节点结束 |
| graph_node_error | EnvelopeTypeGraphNodeError | graph | Graph 节点错误 |
| graph_node_custom | EnvelopeTypeGraphNodeCustom | graph | Graph 节点自定义 |
| graph_step | EnvelopeTypeGraphStep | graph | Graph 步骤 |
| graph_execution_done | EnvelopeTypeGraphExecutionDone | graph | Graph 执行完成 |
| checkpoint | EnvelopeTypeCheckpoint | graph | 检查点 |
| intent_pass | EnvelopeTypeIntentPass | team | 意图传递 |
| member_message_start | EnvelopeTypeMemberMessageStart | team | Team 成员消息开始 |
| member_delta | EnvelopeTypeMemberDelta | team | Team 成员增量 |
| member_message_done | EnvelopeTypeMemberMessageDone | team | Team 成员消息完成 |
| team_run_started | EnvelopeTypeTeamRunStarted | team | Team 运行开始 |
| team_run_finished | EnvelopeTypeTeamRunFinished | team | Team 运行完成 |
| team_run_failed | EnvelopeTypeTeamRunFailed | team | Team 运行失败 |
| team_step_started | EnvelopeTypeTeamStepStarted | team | Team 步骤开始 |
| team_step_finished | EnvelopeTypeTeamStepFinished | team | Team 步骤完成 |
| run_status | EnvelopeTypeRunStatus | chat | Chat 运行态（queued/running/await/cancelled） |
| flow_log | EnvelopeTypeFlowLog | monitor | Flow Log v2 流程步骤（schema: flow_log/v1） |
| team_summary | EnvelopeTypeTeamSummary | team | Team 运行摘要 |
| knowledge_ingest | EnvelopeTypeKnowledgeIngest | knowledge | 知识库文档入库进度 |
| mcp.session.reconnect | EnvelopeTypeMCPSessionReconnect | monitor | MCP 会话重连通知 |
| alert.notify | EnvelopeTypeAlertNotify | monitor | 监控告警通知 |

**Channel 路由补充**：`RouteChannel()` 将 `knowledge_ingest` 路由至 `knowledge`；`flow_log` / `log` / `mcp.session.reconnect` / `alert.notify` 路由至 `monitor`；TeamID 非空时默认回落 `team`。

### 3.3 Envelope 子结构

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
| ErrorCode | string | 错误码（可选） |
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
| Message | string | 错误消息 |
| PendingID | string | 关联的 pending 消息 ID（可选） |

**EnvelopeUsage**：
| 字段 | 类型 | 说明 |
|------|------|------|
| PromptTokens | int | 输入 Token 数 |
| CompletionTokens | int | 输出 Token 数 |
| TotalTokens | int | 总 Token 数 |

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

### 3.4 Envelope 关键方法

| 方法 | 签名 | 说明 |
|------|------|------|
| NewEnvelope | `(typ EnvelopeType, author, sessionID string) Envelope` | 构造新信封，自动生成 ID / Timestamp / Version |
| RouteChannel | `(env Envelope) string` | 根据 Type 自动路由到 Channel |
| MatchFilterKey | `(subscriberKey, eventKey string) bool` | FilterKey 前缀匹配 |
| Clone | `(e Envelope) Envelope` | 深拷贝（含所有指针字段和 map） |
| ContainsTag | `(e Envelope, tag string) bool` | 逗号分隔标签匹配（TrimSpace 后精确匹配） |

---

## 四、事件总线

### 4.1 Bus 接口

`internal/event/bus.go`

```go
type Bus interface {
    Publish(ctx context.Context, envelope Envelope)
    Subscribe(opts SubscribeOptions) (<-chan Envelope, func())
    DropCount() uint64
}
```

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

**可靠订阅**：`Reliable=true` 或关键事件类型（tool_result / error / runner_completion / graph_node_end / team_run_finished / team_run_failed）自动使用 BlockUpTo(100ms) 语义。Publish 时 Critical 优先级订阅者优先投递。

**丢弃可观测**：drop 时通过 `SessionSysLogWarn(system.bus.drop)` 打点，并递增 Prometheus `EventBusDropped`。

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

## 五、事件投影

### 5.1 EventProjector

`internal/agent/event_projector.go`

将 trpc-agent-go `*trpcevent.Event` 投影为 `event.Envelope`，保留完整元数据。

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
| MemberAgentKeys | Team member_* 信封作者白名单 |

**投影规则**：
- trpc Event.Branch / FilterKey / Tag / Extensions / Actions 直接映射到 Envelope
- ProjectMeta 中的字段作为默认值（Event 字段优先）
- LongRunningToolIDs 映射为 ToolCall.IsLongRunning
- Extensions 从 `map[string]json.RawMessage` 转为 `map[string]string`
- LLM Response.Choices 拆分为 text_delta / text_done 事件
- RunnerCompletion 映射为 runner_completion 事件

### 5.2 EventBridge（Graph）

`internal/graph/trpc/event_bridge.go`

将 Graph 执行事件桥接到 EventBus，映射 trpc-agent-go Graph ObjectType 到 EnvelopeType。

### 5.3 Flow Log v2（替代 SlogBridge）

| 文件 | 职责 |
|------|------|
| `internal/event/flow_log.go` | FlowLog 数据结构、schema_version、Envelope 构造 |
| `internal/event/trace_emitter.go` | Turn 热路径：`EnvelopeTypeFlowLog` + span 缓冲 |
| `internal/event/trace_context.go` | TraceContext（trace_id / run_id / agent_key） |
| `internal/event/system_flow.go` | 基础设施：`SysLog*` / `SessionSysLog*`（异步 Publish） |
| `internal/event/flow_context.go` | `CtxFlowLog*`、`SetGlobalBus` |

- Monitor 业务日志主类型为 **`flow_log`**（`schema_version: flow_log/v1`），非全局 `slog` 桥接。
- **`slog_bridge.go` 已删除**（2026-05-20）；`LOG_BRIDGE_ENABLED` 已废弃。
- 进程 Gateway 文本日志仍为 `EnvelopeTypeLog`（如 `PluginSafeLogger`），与 `flow_log` 前端分流。
- 详见 [52-flow-logger.design.md](./52-flow-logger.design.md)、[changelog](../changelog/2026-05-20-FlowLog-V2-SlogRemoval.md)。

---

## 六、事件消费

### 6.1 EventBusConsumer

`internal/biz/event_bus_consumer.go` + 多 handler（I5-SYS-03 拆分 + P2 持久化 + 后续扩展）：

| Handler | 文件 | 职责 |
|---------|------|------|
| eventBufferHandler | `event_bus_buffer_handler.go` | 所有 Envelope 写入环形 Buffer（断连重放） |
| runnerCompletionHandler | `event_bus_runner_handler.go` | RunnerCompletion → TurnMemoryWorker + Monitor 持久化 + Usage 记录（`CHAT_RECORD_RUNNER_USAGE`） |
| stateDeltaHandler | `event_bus_state_handler.go` | StateDelta → SessionUsecase.ApplyStateDelta / UpdateRunnerSnapshotJSON |
| eventPersistHandler | `event_persist_handler.go` | 异步持久化 Envelope → event_store（排除 log/flow_log，有界队列 512） |
| messageStoreConsumer | `event_bus_message_store_consumer.go` | 消息存储 |
| flowLogConsumer | `event_bus_flow_log_consumer.go` | FlowLog 消费 |
| toolCallConsumer | `event_bus_tool_call_consumer.go` | ToolCall 消费 |
| usageRollupConsumer | `event_bus_usage_rollup_consumer.go` | Usage 汇总 |
| callbackConsumer | `event_bus_callback_consumer.go` | 回调消费 |
| userFeedbackConsumer | `event_bus_user_feedback_consumer.go` | 用户反馈消费 |

`Start()` 以 `Reliable=true` 全局订阅 Bus，经 `envelopeToDomainEvent` 转换后按 Type 分派。

### 6.2 DomainEvent 适配

`internal/biz/domain_event.go` + `internal/biz/domain_event_adapter.go`

DomainEvent 是 biz 层的领域事件模型，通过 `eventBusAdapter` 与 EventBus 双向桥接：
- `DomainEventPublisher` → `eventBusAdapter.PublishDomainEvent()` → `Bus.Publish()`
- `Bus.Subscribe()` → `eventBusAdapter.SubscribeDomainEvents()` → `<-chan DomainEvent`

**DomainEventType 枚举**：
runner_completion / state_delta / error / graph_node_start / graph_node_end / graph_node_error / graph_interrupt / text_delta / tool_call / tool_result

---

## 七、Session State Delta

### 7.1 SessionRepository 接口

`internal/biz/session_usecase.go`

```go
GetSessionState(ctx context.Context, sessionID string) (map[string]string, error)
SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error
```

### 7.2 ApplyStateDelta

`internal/biz/session_usecase.go`

```go
func (uc *SessionUsecase) ApplyStateDelta(ctx context.Context, sessionID string, delta DomainStateDelta) error
```

操作类型：
| Operation | 行为 |
|-----------|------|
| set | `state[path] = valueJSON` |
| append | `state[path] = existing + valueJSON` |
| delete | `delete(state, path)` |

### 7.3 持久化

`internal/data/session_repo.go` + `internal/data/ent/schema/session.go`

Session 表 `state_json` 字段（TEXT，默认 `{}`）存储 `map[string]string` 的 JSON 序列化。

---

## 八、事件缓冲与重放

### 8.1 Buffer

`internal/event/buffer.go`

环形缓冲，按 SessionID 分组存储事件。

| 参数 | 值 | 说明 |
|------|-----|------|
| 容量 | 200 | 每个 Session 的最大事件数 |
| TTL | 30 分钟 | 无访问后淘汰 |
| 淘汰周期 | 5 分钟 | 后台 goroutine 定期清理 |

### 8.2 Replay

```go
func (b *Buffer) Replay(sessionID, lastEventID string) []Envelope
```

从 lastEventID 之后的事件开始返回，用于 WebSocket 断连重放。

---

## 九、WebSocket 传输

### 9.1 WSServer

`internal/server/ws.go`

统一 WebSocket 网关，端点 `/v1/ws`。

**连接参数**：
| 参数 | 说明 |
|------|------|
| session_id | 会话 ID（必填，`*` 为全局监控模式） |
| token | 认证 Token |
| last_event_id | 断连重放起始 ID |
| filter_key | 事件过滤键 |
| log_enabled | 是否接收日志事件 |

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
| subscribe | 订阅 Channel |
| unsubscribe | 取消订阅 Channel |
| cancel | 取消当前运行 |
| enable_log | 开关日志事件 |
| user_message | 发送用户消息 |
| enqueue_message | 排队用户消息 |

**Channel 路由**：
| Channel | 默认订阅 | 包含事件类型 |
|---------|----------|-------------|
| chat | ✅ | text_delta / text_done / tool_call / tool_result / state_delta / transfer / runner_completion / run_status / error |
| system | ✅ | connected / pong / server_shutdown / replay_* |
| monitor | ❌（需 enable_log 或 subscribe；global 默认开） | log / flow_log / mcp.session.reconnect / alert.notify |
| team | ❌（需 subscribe） | member_* / team_* / intent_pass / team_summary |
| graph | ❌（需 subscribe） | graph_* / checkpoint |
| knowledge | ❌（需 subscribe） | knowledge_ingest |

**日志门控**：`EnvelopeTypeLog` 受 `log_enabled` / `enable_log` 控制；`flow_log` 始终投递（不经 log 门控）。

**下行 replay 类型**：`replay_start` / `replay` / `replay_end`（非仅 `replay`）。

**背压策略**：
- 普通会话连接：Reliable=true，关键事件 BlockUpTo(100ms)
- 全局监控连接：Reliable=false，可容忍丢失

**连接限制**：
- 每个 Session 最多 5 个连接
- 全局监控模式最多 3 个连接

---

## 十、前端架构

### 10.1 类型定义

`web/src/features/chat/envelope.ts`

前端 Envelope 类型与后端 `event.Envelope` JSON 结构一一对应。辅助：`envelopeRunStatus.ts`（run_status 解析）、`teamRunEventFromEnvelope.ts`（Team 事件投影）。

### 10.2 WsTransport

`web/src/features/chat/ws-transport.ts`

WebSocket 传输层，职责：
- 连接管理（自动重连，指数退避，最大 30s）
- 心跳（25s 间隔）
- 消息发送（离线排队）
- lastEventId 跟踪（断连重放）
- 服务器关机通知

### 10.3 useEnvelopeStream

`web/src/features/chat/useEnvelopeStream.ts`

Vue composable，封装 WsTransport + EnvelopeDispatcher：
- `onType(type, handler)` — 按事件类型注册回调
- `onChannel(channel, handler)` — 按 Channel 注册回调
- `subscribe(channel)` / `unsubscribe(channel)` — 动态订阅/取消
- `enableLog(enabled)` — 开关日志
- `cancel()` — 取消运行

**衍生 composable**：
- `useChatStream(sessionId)` — 聚合 text_delta / tool_call / runner_completion / error
- `useTeamStream(sessionId, teamId?)` — 聚合 member_* / team_* 事件
- `useKnowledgeIngestWs(sessionId)` — 订阅 knowledge_ingest

### 10.4 Monitor 实时事件（已实现）

`web/src/components/monitor/RealtimeEvents.vue`

Monitor `/monitor` Events Tab 生产组件：合并 WS 运行时 Envelope 与持久化 Monitor Events，支持分类过滤、Runs 关联跳转、暂停/清除。

---

## 十一、事件持久化（已实现）

### 11.1 存储

`event_store` Ent 表（`internal/data/ent/schema/event_store.go`），字段：id / session_id / type / author / channel / envelope_json / created_at。索引：session_id+created_at、type、created_at。

写入：`eventPersistHandler` 异步持久化（排除 `log` / `flow_log`）。TTL：`EventStoreCleanup` 每小时，`EVENT_STORE_TTL_DAYS` 默认 7。

### 11.2 回放 API

`GET /v1/events?session_id=&since=&until=&type=&limit=&offset=` — `api/kratos/event/v1/event.proto` → `internal/service/event.go`。

---

## 十二、Chat 会话事件检视（已实现）

> **产品决策**：不增加第四列固定侧边栏（左 Entity / 中 Message / 右 Session 已占满）。采用 **Dialog 双 Tab**，与现有 `SessionTimelineDialog` 入口合并。

### 12.1 与 Monitor 分工

| 维度 | Monitor `RealtimeEvents` | Chat Inspector |
|------|--------------------------|----------------|
| 范围 | 全局 / 多会话 | **当前 session** |
| 数据源 | WS + Monitor Events API | WS + `GET /v1/events` |
| 入口 | `/monitor?tab=events` | Chat 会话菜单 / MessagePanel 工具栏 |
| 侧重 | Runs 关联、运维分类 | Branch 树、StateDelta、FilterKey/Tag |

### 12.2 布局

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

### 12.3 前端分层

| 层级 | 路径 | 职责 |
|------|------|------|
| API | `features/event/api.ts` | `GET /v1/events` |
| 纯函数 | `features/chat/eventFilter.ts` | filterEnvelopes / buildBranchTree |
| Composable | `features/chat/composables/useEventFilter.ts` | 过滤状态 + computed |
| Composable | `features/chat/composables/useChatEventInspector.ts` | WS + 历史合并、暂停 |
| 展示 | `components/chat/EventFilterBar.vue` | 过滤控件 |
| 展示 | `components/chat/BranchTree.vue` | 调用树 |
| 展示 | `components/chat/StateDeltaIndicator.vue` | StateDelta 行 |
| 展示 | `components/chat/TransferBadge.vue` | Transfer 标签 |
| 展示 | `components/chat/SessionEventInspectorPanel.vue` | Tab 内容容器 |
| 容器 | `components/chat/SessionTimelineDialog.vue` | Tab 切换 + Trace  Tab |

Branch 树由 `invocation_id` / `parent_invocation_id` 在线推导，不新增后端 API。

### 12.4 EventFilterState

| 字段 | 说明 |
|------|------|
| typeFilter | `all` 或 EnvelopeType |
| branchPrefix | Branch 前缀匹配 |
| tag | Tag 逗号分隔精确匹配 |
| keyword | 搜索 type/author/content/tool |
| filterKey | FilterKey 前缀匹配 |

---

## 十三、涉及文件清单

### 已实现

| 文件 | 说明 |
|------|------|
| `internal/event/bus.go` | Bus 接口 + 背压策略 + 路由匹配 |
| `internal/event/envelope.go` | Envelope 结构 + EnvelopeType 枚举 + Clone / MatchFilterKey / ContainsTag |
| `internal/event/buffer.go` | 环形缓冲 + TTL 淘汰 + Replay |
| `internal/event/flow_log.go` | FlowLog 数据结构与 Envelope 构造 |
| `internal/event/trace_context.go` | TraceContext |
| `internal/event/trace_emitter.go` | Flow Log v2 + usage spans |
| `internal/event/system_flow.go` | 系统域 FlowLog（`SetGlobalBus`） |
| `internal/event/flow_context.go` | CtxFlowLog* / SetGlobalBus |
| `internal/event/wire.go` | Wire ProviderSet（NewBus + NewBuffer） |
| `internal/agent/event_projector.go` | trpc Event → Envelope 投影 |
| `internal/agent/turn_helpers.go` | ConsumeEventStream 事件消费 |
| `internal/graph/trpc/event_bridge.go` | Graph 事件桥接 |
| `internal/biz/domain_event.go` | DomainEvent 领域模型 |
| `internal/biz/domain_event_adapter.go` | DomainEvent ↔ EventBus 适配 |
| `internal/biz/event_bus_consumer.go` | EventBus 消费者编排 |
| `internal/biz/event_bus_buffer_handler.go` | 缓冲写入 handler |
| `internal/biz/event_bus_runner_handler.go` | RunnerCompletion handler |
| `internal/biz/event_bus_state_handler.go` | StateDelta handler |
| `internal/biz/session_usecase.go` | ApplyStateDelta / GetSessionState / SaveSessionState |
| `internal/data/session_repo.go` | Session State 持久化 |
| `internal/data/ent/schema/session.go` | state_json 字段 |
| `internal/server/ws.go` | WebSocket 统一网关 |
| `internal/service/run_status_publish.go` | run_status Envelope 发布 |
| `internal/service/chat_run_gateway.go` | Chat 运行态 + run_status |
| `internal/tools/mcpobserve/observe.go` | mcp.session.reconnect 发布 |
| `internal/service/monitor_notify.go` | alert.notify 发布 |
| `internal/metrics/vars.go` | EventBusPublished / EventBusDropped 指标 |
| `web/src/features/chat/envelope.ts` | 前端 Envelope 类型 |
| `web/src/features/chat/envelopeRunStatus.ts` | run_status 解析 |
| `web/src/features/chat/ws-transport.ts` | WsTransport |
| `web/src/features/chat/useEnvelopeStream.ts` | useEnvelopeStream composable |
| `internal/biz/event_persist_handler.go` | 异步持久化 handler |
| `internal/data/event_store_repo.go` | EventStore Repo |
| `internal/service/event.go` | 回放 API Service |
| `internal/cronrunner/jobs/event_store_cleanup.go` | TTL 清理 |
| `web/src/features/event/api.ts` | 回放 API 门面 |
| `web/src/features/chat/eventFilter.ts` | 过滤 + Branch 树纯函数 |
| `web/src/features/chat/composables/useEventFilter.ts` | 过滤状态 + computed |
| `web/src/features/chat/composables/useChatEventInspector.ts` | WS + 历史合并、暂停 |
| `web/src/components/monitor/RealtimeEvents.vue` | Monitor Events Tab（生产） |
| `web/src/components/chat/SessionEventInspectorPanel.vue` | Envelope Tab 容器 |
| `web/src/components/chat/EventFilterBar.vue` | 过滤栏 |
| `web/src/components/chat/BranchTree.vue` | 分支追踪树 |
| `web/src/components/chat/BranchTreeNode.vue` | 分支追踪树节点 |
| `web/src/components/chat/StateDeltaIndicator.vue` | 状态变更指示器 |
| `web/src/components/chat/TransferBadge.vue` | Agent 转移标签 |
| `web/src/components/chat/SessionTimelineDialog.vue` | Trace + Envelope 双 Tab |
