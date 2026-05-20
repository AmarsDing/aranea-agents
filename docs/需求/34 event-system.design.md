# Event 事件系统模块 — 实现设计文档

> 对应需求：`34 event-system.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

基于事件总线的发布/订阅机制，支持系统内部组件间的异步事件通信。核心组件为 `event.Bus`（发布/订阅 + 背压策略）+ `event.Envelope`（统一事件信封）+ `event.Buffer`（环形缓冲 + 断连重放），通过 WebSocket 统一传输至前端。

**已实现能力**：
1. EventBus 发布/订阅 + 三种背压策略（DropOldest / DropNewest / BlockUpTo）+ 可靠订阅
2. Envelope 统一事件信封，携带完整元数据（StateDelta / Extensions / FilterKey / Branch / Tag / Actions / Trace）
3. EventProjector 将 trpc-agent-go Event 投影为 Envelope
4. EventBusConsumer 消费事件，自动应用 StateDelta 到 Session State
5. WebSocket 统一传输（Channel 路由：chat / monitor / team / graph / system）
6. EventBuffer 环形缓冲 + TTL 淘汰 + lastEventID 断连重放
7. 前端 Envelope 类型 + WsTransport + useEnvelopeStream composable

**未实现能力**：
1. 事件持久化（SQLite 存储）
2. 事件回放 API（HTTP 按时间范围查询）
3. 前端事件可视化组件（EventTimeline / BranchTree / StateDeltaIndicator）

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
   EventBusConsumer  WSServer  FlowLog (TraceEmitter / SysLog)
   (StateDelta应用)  (WS推流)  (日志桥接)
         │          │
         ▼          ▼
   SessionUsecase   前端 WsTransport
   (ApplyStateDelta)   + useEnvelopeStream
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
| Tag | string | 业务标签，逗号分隔（可选） |
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
| ContainsTag | `(e Envelope, tag string) bool` | 逗号分隔标签匹配 |

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

**可靠订阅**：`Reliable=true` 或关键事件类型（tool_result / error / runner_completion / graph_node_end / team_run_finished / team_run_failed）自动使用 BlockUpTo(100ms) 语义，确保不丢失。

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
| `internal/event/trace_emitter.go` | Turn 热路径：`EnvelopeTypeFlowLog` + span 缓冲 |
| `internal/event/system_flow.go` | 基础设施：`SysLog*` / `SessionSysLog*`（异步 Publish） |
| `internal/event/flow_context.go` | `CtxFlowLog*`、`SetGlobalBus` |

- Monitor 业务日志主类型为 **`flow_log`**（`schema_version: flow_log/v1`），非全局 `slog` 桥接。
- **`slog_bridge.go` 已删除**（2026-05-20）；`LOG_BRIDGE_ENABLED` 已废弃。
- 进程 Gateway 文本日志仍为 `EnvelopeTypeLog`（如 `PluginSafeLogger`），与 `flow_log` 前端分流。
- 详见 [52-flow-logger.design.md](./52-flow-logger.design.md)、[changelog](../changelog/2026-05-20-FlowLog-V2-SlogRemoval.md)。

---

## 六、事件消费

### 6.1 EventBusConsumer

`internal/biz/event_bus_consumer.go`

订阅 EventBus，处理两类关键事件：

**RunnerCompletion**：
- 提取 Usage 信息
- 更新 Session 用量统计

**StateDelta**：
- 调用 `SessionUsecase.ApplyStateDelta()` 应用到 Session State
- 特殊路径 `__state__` 走 `UpdateRunnerSnapshotJSON`

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
| chat | ✅ | text_delta / text_done / tool_call / tool_result / state_delta / transfer / runner_completion / error |
| system | ✅ | connected / pong / server_shutdown / replay_* |
| monitor | ❌（需 enable_log 或 subscribe） | log |
| team | ❌（需 subscribe） | member_* / team_* / intent_pass |
| graph | ❌（需 subscribe） | graph_* / checkpoint |

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

前端 Envelope 类型与后端 `event.Envelope` JSON 结构一一对应，包含所有字段和子结构（EnvelopeContent / EnvelopeToolCall / EnvelopeStateDelta / EnvelopeTransfer / EnvelopeError / EnvelopeUsage / EnvelopeActions / EnvelopeTrace）。

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

---

## 十一、待设计：事件持久化

### 11.1 存储方案

新增 `event_store` Ent 表：

| 字段 | 类型 | 说明 |
|------|------|------|
| id | string | 事件 ID（= Envelope.ID） |
| session_id | string | 会话 ID |
| type | string | 事件类型 |
| author | string | 作者 |
| channel | string | 路由通道 |
| envelope_json | string | Envelope 完整 JSON |
| created_at | time.Time | 事件时间 |

**索引**：session_id + created_at（复合）、type、created_at

**TTL 清理**：后台任务定期删除超过 N 天的事件记录。

### 11.2 回放 API

新增 HTTP API：

```
GET /v1/events?session_id=xxx&since=2025-01-01T00:00:00Z&until=2025-01-02T00:00:00Z&type=tool_call&limit=100&offset=0
```

返回 Envelope 列表 + 分页信息。

---

## 十二、待设计：前端事件可视化

### 12.1 EventTimeline 组件

位置：`web/src/features/chat/components/EventTimeline.vue`

在 Chat 页面侧边栏展示事件时间线。

**Props**：
| Prop | 类型 | 说明 |
|------|------|------|
| sessionId | string | 当前会话 ID |
| filterKey | string | 过滤键（可选） |
| visible | boolean | 是否显示 |

**功能**：
- 按事件类型 / 分支 / 标签过滤
- 事件详情展开（StateDelta 变更指示 / Transfer 标签 / ToolCall 详情）
- 长时运行工具进度指示
- 时间格式化

### 12.2 BranchTree 组件

位置：`web/src/features/chat/components/BranchTree.vue`

基于 InvocationID / ParentInvocationID 构建调用树，可视化多 Agent 执行链。

### 12.3 StateDeltaIndicator 组件

位置：`web/src/features/chat/components/StateDeltaIndicator.vue`

展示状态变更：路径、操作类型、值变更。

### 12.4 useEventFilter composable

位置：`web/src/features/chat/composables/useEventFilter.ts`

事件过滤逻辑：类型过滤、分支过滤、标签过滤、搜索过滤。

---

## 十三、涉及文件清单

### 已实现

| 文件 | 说明 |
|------|------|
| `internal/event/bus.go` | Bus 接口 + 背压策略 + 路由匹配 |
| `internal/event/envelope.go` | Envelope 结构 + EnvelopeType 枚举 + Clone / MatchFilterKey / ContainsTag |
| `internal/event/buffer.go` | 环形缓冲 + TTL 淘汰 + Replay |
| `internal/event/trace_emitter.go` | Flow Log v2 + usage spans |
| `internal/event/system_flow.go` | 系统域 FlowLog（`SetGlobalBus`） |
| `internal/event/wire.go` | Wire ProviderSet |
| `internal/agent/event_projector.go` | trpc Event → Envelope 投影 |
| `internal/agent/turn_helpers.go` | ConsumeEventStream 事件消费 |
| `internal/graph/trpc/event_bridge.go` | Graph 事件桥接 |
| `internal/biz/domain_event.go` | DomainEvent 领域模型 |
| `internal/biz/domain_event_adapter.go` | DomainEvent ↔ EventBus 适配 |
| `internal/biz/event_bus_consumer.go` | EventBus 消费者（StateDelta / Usage） |
| `internal/biz/session_usecase.go` | ApplyStateDelta / GetSessionState / SaveSessionState |
| `internal/data/session_repo.go` | Session State 持久化 |
| `internal/data/ent/schema/session.go` | state_json 字段 |
| `internal/server/ws.go` | WebSocket 统一网关 |
| `internal/metrics/vars.go` | EventBusPublished / EventBusDropped 指标 |
| `web/src/features/chat/envelope.ts` | 前端 Envelope 类型 |
| `web/src/features/chat/ws-transport.ts` | WsTransport |
| `web/src/features/chat/useEnvelopeStream.ts` | useEnvelopeStream composable |

### 待实现

| 文件 | 说明 |
|------|------|
| `internal/data/ent/schema/event_store.go` | 事件持久化表 |
| `internal/data/event_store_repo.go` | 事件存储 Repo |
| `internal/biz/event_store.go` | 事件存储 Usecase |
| `api/kratos/event/v1/event.proto` | 事件回放 API Proto |
| `internal/service/event.go` | 事件回放 Service |
| `web/src/features/chat/components/EventTimeline.vue` | 事件时间线 |
| `web/src/features/chat/components/BranchTree.vue` | 分支追踪树 |
| `web/src/features/chat/components/StateDeltaIndicator.vue` | 状态变更指示器 |
| `web/src/features/chat/components/TransferBadge.vue` | Agent 转移标签 |
| `web/src/features/chat/composables/useEventFilter.ts` | 事件过滤 composable |
