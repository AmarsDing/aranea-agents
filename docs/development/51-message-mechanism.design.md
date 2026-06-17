# 消息机制 — 设计文档

> **需求来源**：[51-message-mechanism.md](./51-message-mechanism.md)（用户故事、功能需求、验收标准、非功能需求）
> **开发计划**：[51-message-mechanism.development.md](./51-message-mechanism.development.md)（代码锚点、任务清单、Phase 划分、状态）
>
> 本文档定义消息机制的架构设计、代码分层、Proto/API 契约、数据模型、接口定义、技术选型、序列图、前端组件设计与 UX 规范。后端与前端设计原属 `51a/51b` 子模块，现合并到本设计文档。

---

## 一、设计目标与原则

### 1.1 设计目标

| 目标 | 说明 |
|------|------|
| 统一事件模型 | 所有通信（Chat/Team/Graph/Channel/Cron/A2A/Monitor）共享同一套事件定义与流转规则 |
| 框架对齐 | 事件模型以 trpc-agent-go `event.Event` 为真相源，项目层只做投影与扩展 |
| 分层清晰 | 事件产生在运行时层、路由在 Event 层、传输在 Server 层，各层职责不越界 |
| 双向通信 | WebSocket 原生支持上行（cancel/enqueue/subscribe），无需额外 HTTP 端点 |
| 通道复用 | 一个 WebSocket 连接承载所有事件类型（chat/monitor/team/graph/system），多路复用 |
| 可扩展 | 新场景（Graph 节点事件、A2A 消息、Artifact 通知）无需修改核心机制 |
| 可靠性 | 背压控制、心跳检测、事件缓冲与重放、自动重连 |
| 传输无关 | 同一 Envelope 可投射到 WebSocket / Webhook，传输层可替换 |

### 1.2 设计原则

```
原则 1：trpc-agent-go event.Event 是事件真相源，项目层不重新定义事件语义
原则 2：事件投影（Event → Envelope）是唯一需要项目层实现的逻辑
原则 3：事件路由由统一 EventBus 承担，Service 层不直接管理订阅
原则 4：传输协议（WS）与事件模型解耦，同一事件可投射到多种传输
原则 5：WebSocket 是 Chat / Team / Graph / Monitor 的主传输通道；历史 Chat SSE 不再作为当前实现入口
原则 6：一个 WS 连接通过 Channel 多路复用所有事件类型
原则 7：客户端通过 subscribe/unsubscribe/enable_log 动态控制订阅范围
```

---

## 二、现状复盘与问题

> 状态跟踪见 [development.md §2 现状评估](./51-message-mechanism.development.md#2-现状评估)。

### 2.1 原有问题与解决方案

| # | 问题 | 解决方案 |
|---|------|---------|
| 1 | 事件投影逻辑散落在 Service 层 | `ConsumeEventStream` + `EventProjector.Project` |
| 2 | 事件类型不统一 | 统一为 `EnvelopeType` 常量（**72 种**，见 §5.2） |
| 3 | 无统一事件总线 | `event.Bus` 接口（`internal/event/bus.go`） |
| 4 | 无事件持久化与重放 | `event.Buffer` + WS replay 同步屏障 |
| 5 | 无双向通信 | WS 上行 cancel / user_message / enqueue_message |
| 6 | 背压缺失 | 三级 DropPolicy + Reliable 关键事件保障 |
| 7 | Monitor 日志与 Agent 事件割裂 | `flow_log` + Flow Log v2 + WS channel:monitor |
| 8 | 连接浪费 | WS 单连接多路复用 |

### 2.2 已删除的旧模块

| 旧模块 | 说明 |
|--------|------|
| `TeamRunEventBroker` | 合并到 EventBus，Team 过滤变为 SubscribeOptions |
| `MonitorLogBroker` + 独立端口 | 合并到 EventBus + Flow Log v2 |
| `streamWriter.Emit()` | 替换为 `EventBus.Publish()` |
| 独立 SSE Server（`:8001`） | 已删除，统一走 WS 传输 |
| `team_run_sse.go` | 合并到 ws.go |
| `publishTeamMonitor` | 已删除（已有 `team.run.*` flow_log，见 [52-flow-logger.design.md](./52-flow-logger.design.md)） |

---

## 三、事件流转全景图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        事件产生层（运行时）                                   │
│                                                                             │
│  trpc Runner.Run()  →  <-chan *event.Event                                 │
│       │                                                                     │
│       │  事件类型：                                                          │
│       │  ├── chat.completion.chunk  (文本增量)                              │
│       │  ├── tool.call / tool.response  (工具调用/结果)                     │
│       │  ├── state.update  (状态增量)                                       │
│       │  ├── agent.transfer  (Agent 间转移)                                 │
│       │  ├── runner.completion  (运行完成)                                  │
│       │  ├── error  (错误)                                                  │
│       │  └── graph.node.* / checkpoint.*  (Graph/Checkpoint 事件)          │
│       ▼                                                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                        事件投影层（Agent 层）                                 │
│                                                                             │
│  ConsumeEventStream → TurnStreamConsumer → EventProjector.Project          │
│       │                                                                     │
│       │  职责：                                                              │
│       │  1. trpc Event → 统一 Envelope（含完整元数据）                      │
│       │  2. 自动填充 Channel（RouteChannel）                                │
│       │  3. 发布到 EventBus                                                 │
│       │                                                                     │
│       │  辅助方法（Team 场景）：                                             │
│       │  - BuildMemberMessageStartEnvelope                                  │
│       │  - BuildMemberDeltaEnvelope                                         │
│       │  - BuildMemberMessageDoneEnvelope                                   │
│       │  - BuildLogEnvelope                                                 │
│       │  - BuildIntentPassEnvelope                                          │
│       ▼                                                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                        事件路由层（Event 层 — EventBus）                     │
│                                                                             │
│  EventBus.Publish(Envelope)                                                │
│       │                                                                     │
│       ├──→ WSServer.eventPump    (WS 双向通道，按 session_id 路由)          │
│       │        ├── channel: chat      (Chat 事件)                          │
│       │        ├── channel: monitor   (Monitor 日志)                       │
│       │        ├── channel: team      (Team 事件)                          │
│       │        ├── channel: graph     (Graph 事件)                         │
│       │        └── channel: system    (系统通知)                           │
│       │                                                                     │
│       └──→ EventBusConsumer      (编排：Buffer/Persist/Runner/State 四 handler) │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                        传输层（Server 层）                                    │
│                                                                             │
│  WS /v1/ws?session_id=xxx   ← 统一传输（双向、多路复用、挂入 Kratos HTTP） │
│  HTTP unary / WS 上行      ← POST /v1/chat/messages 或 WS user_message     │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 四、统一信封模型（Envelope）设计

### 4.1 设计思路

trpc-agent-go 的 `event.Event` 是运行时内部结构，包含 `*model.Response`、`StateDelta`、`Extensions` 等框架细节。直接暴露给传输层会：

1. 违反分层原则（传输层不应依赖框架内部类型）
2. 导致前端需要理解 trpc 框架的事件语义
3. 无法在不修改框架的情况下扩展业务字段

因此设计 **Envelope（信封）** 作为项目层统一的事件传输单元：

- **Envelope 是 event.Event 的投影**，不是替代
- **Envelope 包含完整元数据**，前端可独立消费
- **Envelope 是 JSON 友好的**，所有字段可直接序列化
- **一个 trpc Event 投影为一个或多个 Envelope**（chat.completion.chunk 可能同时产出 text_delta + tool_call）
- **Envelope 服务于 WS 与内部消费者**，传输层只负责编码方式

### 4.2 Envelope 结构（Go）

```go
// internal/event/contract/envelope.go

type EnvelopeType string

// 共 72 种 EnvelopeType 常量，涵盖 Chat/Monitor/Graph/Team/Knowledge/System/
// Spirit/Butler/Skill/Monitor/Borrow/Organization/Activity 等场景。
// 完整列表见 §4.3 事件类型映射。

type Envelope struct {
    ID                 string       `json:"id"`
    Type               EnvelopeType `json:"type"`
    Author             string       `json:"author"`
    SessionID          string       `json:"session_id"`
    TeamID             string       `json:"team_id,omitempty"`
    RequestID          string       `json:"request_id,omitempty"`
    InvocationID       string       `json:"invocation_id,omitempty"`
    ParentInvocationID string       `json:"parent_invocation_id,omitempty"`
    Branch             string       `json:"branch,omitempty"`
    FilterKey          string       `json:"filter_key,omitempty"`
    Tag                string       `json:"tag,omitempty"`
    Timestamp          string       `json:"timestamp"`
    Version            int          `json:"version"`
    Channel            string       `json:"channel,omitempty"`

    Content    *EnvelopeContent    `json:"content,omitempty"`
    ToolCall   *EnvelopeToolCall   `json:"tool_call,omitempty"`
    StateDelta *EnvelopeStateDelta `json:"state_delta,omitempty"`
    Transfer   *EnvelopeTransfer   `json:"transfer,omitempty"`
    Error      *EnvelopeError      `json:"error,omitempty"`
    Usage      *EnvelopeUsage      `json:"usage,omitempty"`
    TokenUsage *EnvelopeTokenUsage `json:"token_usage,omitempty"`
    Extensions map[string]string   `json:"extensions,omitempty"`
    Actions    *EnvelopeActions    `json:"actions,omitempty"`
    Trace      *EnvelopeTrace      `json:"trace,omitempty"`
    Metadata   map[string]any      `json:"metadata,omitempty"`

    // 增量同步与追踪字段
    SessionRevision int64  `json:"session_revision,omitempty"`
    Source          string `json:"source,omitempty"`
    JobID           string `json:"job_id,omitempty"`
    TurnID          string `json:"turn_id,omitempty"`
}
```

### 4.3 事件类型映射（72 种）

> 完整常量定义见 `internal/event/contract/envelope.go`。前端类型定义见 `web/src/realtime/envelope.ts`（64 种，未含 Borrow/Organization/Evolution/CacheHit 等后端内部事件）。

| Envelope type | trpc model.ObjectType | 说明 | WS channel |
|---------------|----------------------|------|-----------|
| `text_delta` | `chat.completion.chunk` (IsPartial) | 文本增量 | chat |
| `text_done` | `chat.completion.chunk` (!IsPartial) | 文本完成 | chat |
| `tool_call` | `chat.completion.chunk` (ToolCalls) | 工具调用开始 | chat |
| `tool_result` | `tool.response` | 工具返回结果 | chat |
| `state_delta` | `state.update` | 状态增量 | chat |
| `transfer` | `agent.transfer` | Agent 转移 | chat / team |
| `runner_completion` | `runner.completion` | 运行完成 | chat |
| `run_status` | — | 运行生命周期 / Follow-up 入队（`message_queued`） | chat / team |
| `error` | `error` | 错误 | chat |
| `context_usage` | — | 上下文用量（ReAct 子步） | chat |
| `token_usage` | — | Token 用量事件 | chat |
| `intent_pass` | — | 意图识别结果 | chat |
| `user_feedback` | — | 用户反馈事件 | chat |
| `execution_progress` | — | 编排步骤进度（LLM 首字节等待） | chat |
| `log` | — | 进程/Gateway 文本日志（需 WS `enable_log`） | monitor |
| `flow_log` | — | 业务/系统流程日志（TraceEmitter / SysLog；**免** `enable_log`） | monitor |
| `mcp.session.reconnect` | — | MCP 会话重连通知 | monitor |
| `mcp.health.alert` | — | MCP 健康告警 | monitor |
| `alert.notify` | — | 告警通知 | monitor |
| `monitor.auto_healed` | — | Monitor 自动修复 | monitor |
| `monitor.self_check_completed` | — | Monitor 自检完成 | monitor |
| `graph_node_start` | `graph.node.start` | Graph 节点开始 | graph |
| `graph_node_end` | `graph.node.end` | Graph 节点结束 | graph |
| `graph_node_error` | `graph.node.error` | Graph 节点错误 | graph |
| `graph_step` | — | Graph 步骤进度 | graph |
| `graph_execution_done` | — | Graph 执行完成 | graph |
| `graph_node_custom` | — | Graph 自定义节点事件 | graph |
| `graph_task_status` | — | Graph 任务状态 | graph |
| `checkpoint` | `checkpoint.*` | 检查点事件 | graph |
| `member_message_start` | — | Team 成员消息开始 | team |
| `member_delta` | — | Team 成员增量 | team |
| `member_message_done` | — | Team 成员消息完成 | team |
| `team_run_started` | — | Team 运行开始 | team |
| `team_run_finished` | — | Team 运行完成 | team |
| `team_run_failed` | — | Team 运行失败 | team |
| `team_step_started` | — | Team 步骤开始 | team |
| `team_step_finished` | — | Team 步骤完成 | team |
| `team_summary` | — | Team 结构化汇总 | team |
| `orchestration_agent_status` | — | 编排 Agent 状态 | team |
| `spirit_team_assembled` | — | Spirit Team 组装完成 | chat |
| `spirit_team_completed` | — | Spirit Team 完成 | chat |
| `spirit_team_failed` | — | Spirit Team 失败 | chat |
| `spirit_team_cancelled` | — | Spirit Team 取消 | chat |
| `spirit_team_interrupted` | — | Spirit Team 中断 | chat |
| `spirit_team_progress` | — | Spirit Team 进度 | chat |
| `spirit_teams_all_completed` | — | Spirit Teams 全部完成 | chat |
| `spirit_synthesis_completed` | — | Spirit 合成完成 | chat |
| `spirit_plan_created` | — | Spirit 计划创建 | chat |
| `spirit_allocation_created` | — | Spirit 分配创建 | chat |
| `spirit_orchestration_started` | — | Spirit 编排开始 | chat |
| `spirit_orchestration_checkpoint` | — | Spirit 编排检查点 | chat |
| `spirit_orchestration_interrupted` | — | Spirit 编排中断 | chat |
| `butler.orchestration.started` | — | Butler 编排开始 | chat |
| `butler.orchestration.completed` | — | Butler 编排完成 | chat |
| `butler.orchestration.failed` | — | Butler 编排失败 | chat |
| `skill.health_changed` | — | Skill 健康变更 | chat |
| `skill.evolution_proposed` | — | Skill 演进提议 | chat |
| `orchestration.evolution_suggested` | — | 编排演进建议（DQ-score 闭环） | chat |
| `orchestration.cache_hit` | — | 编排缓存命中 | chat |
| `session.status_changed` | — | Session 状态变更 | chat |
| `metrics_updated` | — | 指标更新事件 | chat |
| `knowledge_ingest` | — | 知识库入库进度 | knowledge |
| `borrow.approved` | — | 借用请求批准 | chat |
| `borrow.rejected` | — | 借用请求拒绝 | chat |
| `borrow.auto_approved` | — | 借用请求自动批准 | chat |
| `organization.created` | — | 组织创建 | chat |
| `organization.updated` | — | 组织更新 | chat |
| `organization.deleted` | — | 组织删除 | chat |
| `activity_start` | — | Activity 开始（AF 阶段） | chat |
| `activity_delta` | — | Activity 增量（AF 阶段） | chat |
| `activity_done` | — | Activity 完成（AF 阶段） | chat |
| `activity_child_start` | — | Activity 子项开始（AF 阶段） | chat |

**关键设计决策**：同一 Envelope 类型可路由到不同 Channel（如 `transfer` 在单 Agent 场景走 `chat`，在 Team 场景走 `team`），由 `RouteChannel()` 根据 TeamID 自动判断。

### 4.4 Channel 自动路由

```go
// internal/event/contract/envelope.go
// 通过 RegisterChannelRoute 在 init() 时注册路由表（OCP 合规），
// 未注册类型回退到 TeamID 推断或 "chat" 默认值。

func RouteChannel(env Envelope) string {
    if ch, ok := channelRegistry[env.Type]; ok {
        return ch
    }
    if env.TeamID != "" {
        return "team"
    }
    return "chat"
}
```

### 4.5 辅助方法

```go
func NewEnvelope(typ EnvelopeType, author, sessionID string) Envelope
func (e Envelope) RouteChannel() string
func (e Envelope) MatchFilterKey(key string) bool
func (e Envelope) Clone() Envelope
func (e Envelope) ContainsTag(tag string) bool
```

---

## 五、EventBus 统一事件总线设计

### 5.1 设计思路

EventBus 统一所有事件路由：

1. **单一发布入口**：所有事件通过 `EventBus.Publish()` 发布
2. **多订阅者路由**：按 session_id / team_id / channel / filter_key / event_types / selector 路由
3. **内部消费**：EventBuffer 追加、StateDelta 持久化、Usage 记录等作为内部订阅者
4. **背压控制**：三级 DropPolicy + Reliable 关键事件保障
5. **Channel 路由**：Envelope 根据 type 和 session 类型自动分配到对应 channel

### 5.2 Bus 接口

```go
// internal/event/contract/bus.go

type Bus interface {
    Publish(ctx context.Context, envelope Envelope)
    Subscribe(opts SubscribeOptions) (<-chan Envelope, func())
    DropCount() uint64
}

type DropPolicy int

const (
    DropOldest DropPolicy = iota  // channel 满时丢弃最旧事件（默认）
    DropNewest                    // channel 满时丢弃最新事件
    BlockUpTo                     // channel 满时阻塞发送者（带超时保护）
)

type ChannelPriority int

const (
    ChannelPriorityCritical ChannelPriority = iota
    ChannelPriorityNormal
)

type SubscribeOptions struct {
    SessionID   string
    TeamID      string
    Channel     string
    FilterKey   string
    EventTypes  []EnvelopeType
    LevelFilter string
    Priority    ChannelPriority
    BufferSize  int
    Reliable    bool
    DropPolicy  DropPolicy
    BlockFor    time.Duration
    Selector    func(EnvelopeType) bool
}
```

### 5.3 关键实现细节

1. **三级背压策略**：
   - `DropOldest`：channel 满时丢弃最旧事件（默认）
   - `DropNewest`：channel 满时丢弃最新事件
   - `BlockUpTo`：channel 满时阻塞发送者（带超时保护）

2. **Reliable 关键事件保障**：
   - 订阅者设置 `Reliable: true` 时，关键事件类型自动升级为 `BlockUpTo` 策略
   - 关键事件：`tool_result` / `error` / `runner_completion` / `graph_node_end` / `team_run_finished` / `team_run_failed`
   - 确保关键事件不丢失

3. **Prometheus 可观测**：
   - `aranea_event_bus_published_total`：发布事件总数
   - `aranea_event_bus_dropped_total`：丢弃事件总数

### 5.4 FilterKey 匹配规则

对齐 trpc-agent-go `event.Event.Filter()` 的前缀匹配语义：

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

---

## 六、EventProjector 事件投影器设计

> **注意**：`EventProjector` 已标记为 Deprecated，Activity-First 架构下推荐使用 `ActivityProjector`（`internal/agent/activity_projector.go`）。详见 [59-chat-ui-optimization.design.md](./59-chat-ui-optimization.design.md)。

### 6.1 设计思路

EventProjector 是 trpc `event.Event` 到项目层 `Envelope` 的转换点。它承担：

1. **类型映射**：trpc ObjectType → Envelope type
2. **内容提取**：从 `model.Response.Choices` 提取文本/推理/工具调用
3. **Team 事件构建**：BuildMemberMessageStart/Delta/DoneEnvelope、BuildLogEnvelope、BuildIntentPassEnvelope
4. **发布到 EventBus**：`Project` 投影后由 `TurnStreamConsumer` 发布到 EventBus

### 6.2 EventProjector 接口

```go
// internal/agent/event_projector.go

type EventProjector struct {
    eventBus event.Bus
    // ... 内部缓存字段
}

type ProjectMeta struct {
    SessionID          string
    RequestID          string
    InvocationID       string
    ParentInvocationID string
    TeamID             string
    Branch             string
    FilterKey          string
}

// Project 将单个 trpc Event 投影为 0..N 个 Envelope（chat.completion.chunk 可能同时产出 text_delta + tool_call）
func (p *EventProjector) Project(ctx context.Context, ev *trpcevent.Event, meta ProjectMeta) []event.Envelope
```

### 6.3 Team 辅助方法

```go
func (p *EventProjector) BuildMemberMessageStartEnvelope(author, sessionID, teamID, branch string) event.Envelope
func (p *EventProjector) BuildMemberDeltaEnvelope(author, sessionID, teamID, text string) event.Envelope
func (p *EventProjector) BuildMemberMessageDoneEnvelope(author, sessionID, teamID, text string) event.Envelope
func (p *EventProjector) BuildLogEnvelope(level, message, source, sessionID string) event.Envelope
func (p *EventProjector) BuildIntentPassEnvelope(payload map[string]any, sessionID, teamID string) event.Envelope
```

### 6.4 事件循环实现

事件循环由 `ConsumeEventStream` 承担，位于 `internal/agent/turn_helpers.go`：

```go
// internal/agent/turn_helpers.go

func ConsumeEventStream(
    ctx context.Context,
    events <-chan *trpcevent.Event,
    eventBus event.Bus,
    projectMeta ProjectMeta,
    opts *StreamConsumeOptions,
    lg loggateway.Logger,
) EventStreamResult {
    return ConsumeEventStreamWithFirstByte(ctx, ctx, events, eventBus, projectMeta, nil, opts, lg)
}
```

`StreamConsumeOptions` 可注入 `ActivityProjector`（AF 阶段），将运行时事件投影为 Activity 语义单元。所有副作用（WS 写入、消息落库、用量记录）由 EventBus 的订阅者处理，事件循环本身只做投影+发布。

---

## 七、WebSocket Server 实现设计

### 7.1 Server 层注册

WSServer 通过 `RegisterOnKratos` 挂入 Kratos HTTP Server，不独立监听端口：

```go
// internal/server/ws.go

type WSServer struct {
    mu             sync.RWMutex
    conns          map[string][]*wsConn
    eventBus       event.Bus
    eventBuffer    *event.Buffer
    canceller      RunCanceller
    sender         ChatSender
    turnExecutor   WSTurnExecutor
    upgrader       websocket.Upgrader
    globalConns    []*wsConn
    maxSessionConns int
    maxGlobalConns  int
}

func NewWSServerFromInfra(c *conf.Server, infra *event.Infra, canceller RunCanceller, sender ChatSender, turnExecutor WSTurnExecutor, runtimeConf *conf.Runtime, lg loggateway.Logger) *WSServer

func (s *WSServer) RegisterOnKratos(srv *kratoshttp.Server) {
    srv.HandleFunc("/v1/ws", s.handleWS)
}
```

### 7.2 认证三级回退

```go
func extractToken(r *http.Request) string {
    if t := r.URL.Query().Get("token"); t != "" {
        return t
    }
    if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
        return strings.TrimPrefix(h, "Bearer ")
    }
    if c, err := r.Cookie("access_token"); err == nil && c.Value != "" {
        return c.Value
    }
    return ""
}
```

**设计决策**：浏览器 WebSocket API 无法设置自定义 Header，Cookie 认证是浏览器场景的必要回退。

### 7.3 Origin 校验

复用 `cors_filter.go` 中的 `OriginAllowed` 函数。白名单规则：localhost/127.0.0.1/[::1] 前缀 + 环境变量 `KRATOS_HTTP_EXTRA_CORS_ORIGINS`。

### 7.4 读泵（上行消息处理）

```go
// internal/server/ws_io_pump.go

func (s *WSServer) readPump(wc *wsConn) {
    defer wc.cancel()
    wc.conn.SetReadLimit(1 << 20)  // 1MB
    wc.conn.SetReadDeadline(time.Now().Add(defaultWSPongWait))
    wc.conn.SetPongHandler(func(string) error {
        wc.conn.SetReadDeadline(time.Now().Add(defaultWSPongWait))
        return nil
    })

    for {
        _, msg, err := wc.conn.ReadMessage()
        if err != nil { return }
        var up wsUpstream
        if json.Unmarshal(msg, &up) != nil { continue }
        switch up.Type {
        case "user_message":    s.handleUserMessage(wc, up)
        case "cancel":          s.handleCancel(wc, up)
        case "enqueue_message": s.handleEnqueueMessage(wc, up)
        case "subscribe":       s.handleSubscribe(wc, up)
        case "unsubscribe":     s.handleUnsubscribe(wc, up)
        case "enable_log":      s.handleEnableLog(wc, up)
        case "ping":            s.sendPong(wc)
        }
    }
}
```

### 7.5 写泵与三优先级发送队列

```go
// internal/server/ws_io_pump.go — writePump 从三优先级队列消费消息
// internal/server/ws_priority.go — wsPriority 分类 EnvelopeType 为 Critical/Normal

// 优先级规则：
// - Critical：runner_completion / error / tool_result / team_run_finished/failed / graph_node_end
// - Normal：其余所有类型
// Critical 队列满时 BlockUpTo；Normal 队列满时 DropNewest + 计数
```

### 7.6 事件泵与重放同步屏障

```go
// internal/server/ws_io_pump.go — eventPump 从 EventBus channel 转发到优先级队列
// internal/server/ws_event.go — replayEvents / sendConnected

func (s *WSServer) eventPump(wc *wsConn, eventCh <-chan event.Envelope) {
    if wc.replayDone != nil {
        <-wc.replayDone    // 阻塞直到重放完成
    }
    for env := range eventCh {
        // 转发到优先级队列
    }
}
```

**关键设计**：`eventPump` 在 `replayDone` 通道关闭前阻塞，确保重放事件全部发送完毕后才开始转发实时事件，避免两者交错。

### 7.7 全局监控模式

`session_id=*` 连接可订阅所有 Session 的 Monitor/Team/Graph 事件（限 `maxGlobalConns` 个连接）。

### 7.8 服务端优雅关闭

```go
func (s *WSServer) Stop(ctx context.Context) error {
    // 广播 server_shutdown 到所有连接
    // 关闭所有连接
    return nil
}
```

### 7.9 上行消息处理

| 上行类型 | 处理方法 | 说明 |
|---------|---------|------|
| `user_message` | `handleUserMessage` | 调用 ChatService native turn |
| `cancel` | `handleCancel` | 调用 ChatService.CancelRun |
| `enqueue_message` | `handleEnqueueMessage` | 调用 `ChatService.EnqueueUserMessage`（`POST /v1/chat/enqueue` 等价语义：steerable enqueue 或 pending 入队；无 active run 时返回 `enqueue_rejected` 错误 Envelope） |
| `subscribe` | `handleSubscribe` | 动态订阅通道（含 filter_key） |
| `unsubscribe` | `handleUnsubscribe` | 取消订阅通道 |
| `enable_log` | `handleEnableLog` | 开启/关闭 Monitor 日志流 |
| `ping` | `sendPong` | 心跳回复 |

### 7.10 配置

```protobuf
// internal/conf/conf.proto

message Server {
  message HTTP { ... }
  message GRPC { ... }

  message WS {
    bool enable = 1;
    string network = 2;
    string addr = 3;
  }

  HTTP http = 1;
  GRPC grpc = 2;
  WS ws = 4;
  string a2a_public_base_url = 5;
  Monitor monitor = 6;
  OpenAI openai = 7;
}

message Monitor {
  // Gateway process log (EnvelopeTypeLog) pushed on WS when true.
  bool process_log_enabled = 1;
}
```

---

## 八、Monitor 日志统一接入设计

### 8.1 Flow Log v2（已替代 SlogBridge）

业务与系统可观测日志通过 **Flow Log v2** 发布为 `EnvelopeTypeFlowLog`（非全局 `slog` 桥接）：

| 组件 | 文件 | 说明 |
|------|------|------|
| Turn | `internal/event/trace_emitter.go` | `NewTraceEmitterForRun` → `emitter.Log*` |
| 系统 | `internal/event/flow_log.go` | `SysLog*` / `SessionSysLog*` |
| 上下文 | `internal/event/flow_context.go` | `CtxFlowLogWarn/Done/Error`（遗留 API，新代码使用 `loggateway.Logger`） |

`internal/event/slog_bridge.go` **已删除**（2026-05-20）。详见 [52-flow-logger.design.md](./52-flow-logger.design.md)。

### 8.2 Monitor 事件来源

| 来源 | metadata.source | 说明 |
|------|----------------|------|
| Runner 生命周期 | `team-runner` / `chat-native` | Agent 启动/完成/错误 |
| Tool 执行 | `tool` | 工具调用开始/结束/错误 |
| LLM 调用 | `llm` | 模型请求/响应/重试 |
| 系统事件 | `system` | 内存/连接/配置变更 |
| Intent Pass | `intent-pass` | 意图识别日志 |
| Flow Log v2 | `step_id` | `TraceEmitter` / `SysLog*` → `flow_log` |

**关键设计**：`sessionID` 参数必须传入，因为 EventBus 按 `session_id` 路由事件到 WS 客户端。空 `sessionID` 会导致日志事件无法送达任何订阅者。

---

## 九、内部消费者设计

### 9.1 EventBusConsumer（编排器）

单一 **Reliable** 订阅；`handleEnvelope` 按职责委托 handler，避免单文件混合 Buffer / 持久化 / 用量 / 状态：

```go
// internal/biz/event_bus_consumer.go

type EventBusConsumer struct {
    eventBus event.Bus
    buffer   *eventBufferHandler      // event_bus_buffer_handler.go
    runner   *runnerCompletionHandler // event_bus_runner_handler.go
    state    *stateDeltaHandler       // event_bus_state_handler.go
    persist  *eventPersistHandler     // event_persist_handler.go（可选 EventStore）
}

func (c *EventBusConsumer) handleEnvelope(ctx context.Context, env event.Envelope) {
    c.buffer.Handle(env)
    if c.persist != nil {
        c.persist.Handle(ctx, env)
    }
    de := envelopeToDomainEvent(env)
    c.handleDomainEvent(ctx, *de) // runner_completion / state_delta
}
```

### 9.2 Handler 与 Side Consumers 列表

| 组件 | 触发 | 职责 |
|------|------|------|
| `eventBufferHandler` | 全量 | `event.Buffer.Append`（WS 重放） |
| `eventPersistHandler` | 全量（异步队列） | `event_store` 持久化（EventStore 启用时） |
| `runnerCompletionHandler` | `DomainEventRunnerCompletion` | Usage / Monitor / TurnMemory |
| `stateDeltaHandler` | `DomainEventStateDelta` | Session state 合并写 |
| `ToolCallConsumer` | `tool_result`（终态） | ToolInvocation 落库（upsert，`source=event_bus`） |
| `CallbackConsumer` | `run_status` 终态 | Webhook 回调（`WebhookDispatcher`） |
| `MessageStoreConsumer` | `member_message_done` | Team 成员 `role=member` + `options_json.team_member`（`chat.team_member/v1`） |
| `FlowLogPersistConsumer` | `flow_log` | FlowLog 持久化 |
| `UserFeedbackConsumer` | `user_feedback` | 用户反馈处理 |
| `UsageRollupConsumer` | `token_usage` / `runner_completion` | 用量汇总 |

Side Consumers 由 `event_bus_side_consumers.go` 统一编排启动/停止，独立 Bus 订阅，按 EventTypes 过滤。

### 9.3 StateDelta 持久化

实现见 `internal/biz/event_bus_state_handler.go`（`set` / `append` / `delete` 合并 Session state）。

### 9.4 用量记录

实现见 `internal/biz/event_bus_runner_handler.go`（`runner_completion` → `UsageUsecase` + `metadata_json.spans`）。

---

## 十、事件持久化与重放设计

### 10.1 内存缓冲区

```go
// internal/event/buffer.go

type Buffer struct {
    mu      sync.RWMutex
    buffers map[string]*ringBuffer
    maxSize int
    ttl     time.Duration
}

func NewBuffer(maxSizePerSession int) *Buffer
func (b *Buffer) Append(sessionID string, env Envelope)
func (b *Buffer) Replay(sessionID string, afterEventID string) []Envelope
```

### 10.2 WS 重连重放协议

```
1. 客户端重连时携带 last_event_id
   WS /v1/ws?session_id=sess-uuid&last_event_id=evt-100
2. 服务端从 EventBuffer 查询并重放
3. 先发送 replay_start → 重放事件 → replay_end
4. 重放期间实时事件缓冲，重放完成后发送
```

### 10.3 重放同步屏障

```go
type wsConn struct {
    // ...
    replayDone chan struct{}   // 重放完成后关闭
}

func (s *WSServer) eventPump(wc *wsConn, eventCh <-chan Envelope) {
    if wc.replayDone != nil {
        <-wc.replayDone    // 阻塞直到重放完成
    }
    for env := range eventCh {
        // 正常转发实时事件
    }
}
```

**关键设计**：`eventPump` 在 `replayDone` 通道关闭前阻塞，确保重放事件全部发送完毕后才开始转发实时事件，避免两者交错。

---

## 十一、场景事件流（序列图）

### 11.1 Chat 场景

```
用户消息
  → Runner.Run()
    → LLM 调用 → text_delta × N → text_done
    → Tool 调用 → tool_call → tool_result
    → text_delta × N → text_done
  → runner_completion
```

### 11.2 Team 场景

```
用户消息
  → Team Runner.Run()
    → team_run_started
    → Coordinator Agent
      → tool_call: transfer_to_agent(agent_b)
        → Agent B → member_message_start → member_delta × N → member_message_done
        → transfer back
      → team_step_finished
    → team_run_finished / team_run_failed
```

**Team 事件投影**：Team Runner 的事件循环同样使用 `ConsumeEventStream` 将 trpc 事件投影为 Envelope 并发布到 EventBus，与 Chat 场景共享同一套投影逻辑。Team 生命周期事件（team_run_started/finished/failed）由 Team Runner 直接构建并发布。

### 11.3 Graph 场景

```
用户消息
  → GraphAgent.Run()
    → graph_node_start (step_1) → text_delta → graph_node_end
    → graph_node_start (step_2) → tool_call → tool_result → graph_node_end
    → graph_step
    → graph_execution_done
    → checkpoint
```

### 11.4 Channel 场景

```
飞书 Webhook POST
  → ChannelIngress.HandleWebhook()
    → ChatService.RunNativeTurn()
      → runner.Run() → EventProjector → EventBus
        → EventBusConsumer → 状态持久化/用量记录
```

---

## 十二、分层实现清单（代码锚点）

> 状态与进度跟踪见 [development.md §1 代码锚点](./51-message-mechanism.development.md#1-模块定位)。

### 12.1 Event 层

| 文件 | 说明 |
|------|------|
| `internal/event/contract/envelope.go` | Envelope 领域模型（**72 种** EnvelopeType）、`RouteChannel`、MatchFilterKey、Clone、ContainsTag |
| `internal/event/contract/bus.go` | Bus 接口定义、DropPolicy、ChannelPriority、SubscribeOptions |
| `internal/event/contract/reliability.go` | 事件可靠性分级 |
| `internal/event/envelope.go` | 契约包再导出 |
| `internal/event/bus.go` | Bus 实现（三级背压策略、Reliable 关键事件保障、Prometheus 指标） |
| `internal/event/buffer.go` | 事件内存缓冲区（环形缓冲区、TTL 淘汰、Replay） |
| `internal/event/trace_emitter.go` | Flow Log v2 + UsageAggregator 桥接 |
| `internal/event/flow_context.go` | CtxFlowLog* 上下文辅助（遗留 API，新代码用 `loggateway.Logger`） |
| `internal/event/flow_log.go` | SysLog* / SessionSysLog* 系统流程日志 |
| `internal/event/flow_tracker.go` | FlowTracker 流程追踪 |
| `internal/event/session_revision.go` | SessionRevisionBumper 接口 + BumpAndPublishSessionRevision |
| `internal/event/wire.go` | Wire ProviderSet（InfraProviderSet，含 NewBus + NewBuffer） |
| `internal/event/infra.go` | event.Infra 双总线容器 |
| `internal/event/framework_adapter.go` | trpc 框架事件适配器 |
| `internal/event/source.go` | 事件来源标识 |

### 12.2 Server 层

| 文件 | 说明 |
|------|------|
| `internal/server/ws.go` | WSServer 主文件（挂入 Kratos HTTP、三级认证、全局监控模式、server_shutdown） |
| `internal/server/ws_conn.go` | WS 连接管理 |
| `internal/server/ws_conn_manager.go` | WS 连接管理器 |
| `internal/server/ws_codec.go` | WS 编解码 |
| `internal/server/ws_message_handler.go` | WS 上行消息处理 |
| `internal/server/ws_io_pump.go` | WS 读写泵 + eventPump |
| `internal/server/ws_event.go` | WS 事件订阅与推送（sendConnected / replayEvents） |
| `internal/server/ws_priority.go` | WS 三优先级发送队列 |

### 12.3 Agent 层

| 文件 | 说明 |
|------|------|
| `internal/agent/event_projector.go` | EventProjector（**Deprecated**，AF 阶段用 ActivityProjector）：trpc Event → Envelope 投影器 + Team/Log/Intent 辅助方法 |
| `internal/agent/activity_projector.go` | ActivityProjector（AF 阶段）：运行时事件 → Activity 语义单元 |
| `internal/agent/turn_helpers.go` | ConsumeEventStream / ConsumeEventStreamWithFirstByte：事件循环简化为投影+发布 |
| `internal/agent/turn_stream_helpers.go` | TurnStreamConsumer 实现 |
| `internal/agent/stream_consumer.go` | 流消费者 |

### 12.4 Biz 层

| 文件 | 说明 |
|------|------|
| `internal/biz/event_bus_consumer.go` | EventBusConsumer 编排器 |
| `internal/biz/event_bus_buffer_handler.go` | EventBuffer 追加 handler |
| `internal/biz/event_bus_runner_handler.go` | runner_completion → Usage / Monitor / Memory handler |
| `internal/biz/event_bus_state_handler.go` | state_delta 持久化 handler |
| `internal/biz/event_persist_handler.go` | event_store 异步持久化（可选） |
| `internal/biz/event_bus_side_consumers.go` | Side Consumers 编排器（统一启动/停止） |
| `internal/biz/event_bus_tool_call_consumer.go` | tool_result → ToolInvocation 落库 |
| `internal/biz/event_bus_callback_consumer.go` | run_status 终态 → Webhook 回调 |
| `internal/biz/event_bus_message_store_consumer.go` | member_message_done → 成员消息落库 |
| `internal/biz/event_bus_flow_log_consumer.go` | flow_log → FlowLog 持久化 |
| `internal/biz/event_bus_user_feedback_consumer.go` | user_feedback → 用户反馈处理 |
| `internal/biz/event_bus_usage_rollup_consumer.go` | token_usage / runner_completion → 用量汇总 |
| `internal/biz/event_bus_async.go` | 异步消费者辅助 |
| `internal/biz/domain_event.go` | DomainEvent 领域模型（与 Envelope 双向转换） |
| `internal/biz/domain_event_adapter.go` | DomainEvent ↔ Envelope 适配器 |

### 12.5 Service 层

| 文件 | 说明 |
|------|------|
| `internal/service/chat.go` | ChatService 主文件：`SendChatMessage` / `CancelRun` / `EnqueueUserMessage` |
| `internal/service/chat_native.go` | `RunNativeTurn` / `ExecuteTurn`：HTTP unary / WS 上行复用 native turn |
| `internal/service/chat_enqueue.go` | `EnqueueUserMessage` 实现（steerable enqueue / pending 入队） |
| `internal/service/chat_event_publisher.go` | Chat 事件发布 |
| `internal/service/chat_run_gateway.go` | Run 网关 |

### 12.6 Team 层

| 文件 | 说明 |
|------|------|
| `internal/team/runner_team_trpc.go` | `runTeamTRPCFromInput`：Team trpc 运行入口 |
| `internal/team/runner_team_turn.go` | Team turn 执行 |
| `internal/team/runner_team_observer.go` | Team 观察者 |
| `internal/team/runner_helpers.go` | Team runner 辅助方法 |

### 12.7 Data 层

| 文件 | 说明 |
|------|------|
| `internal/data/sql/message_fts.sql` | FTS5 全文搜索虚拟表 + 触发器（`messages_fts`） |

### 12.8 Metrics

| 文件 | 说明 |
|------|------|
| `internal/metrics/vars.go` | Prometheus 指标（`aranea_event_bus_published_total` / `aranea_event_bus_dropped_total`） |

### 12.9 配置

| 文件 | 说明 |
|------|------|
| `internal/conf/conf.proto` | `Server.WS` 消息（enable / network / addr）+ `Server.Monitor.process_log_enabled` |

### 12.10 Proto API

| 文件 | 说明 |
|------|------|
| `api/kratos/session/v1/session.proto` | `SearchSessionMessages` RPC（FTS5 搜索） |

---

## 十三、Wire 注入

```go
// internal/event/wire.go
// Deprecated: use InfraProviderSet for dual-bus wiring.
var ProviderSet = InfraProviderSet

// internal/biz/biz.go
var ProviderSet = wire.NewSet(
    NewEventBusConsumer,
    // ... 其他 biz providers
)

// internal/server/server.go
var ProviderSet = wire.NewSet(
    NewHTTPServer,
    NewGRPCServer,
    NewWSServerFromInfra,
)
```

---

## 十四、性能考量

### 14.1 背压策略

| 组件 | 策略 |
|------|------|
| EventBus → 慢订阅者 | 三级 DropPolicy（DropOldest / DropNewest / BlockUpTo） |
| Reliable 订阅者 | 关键事件自动升级为 BlockUpTo |
| WS 三优先级队列 | Critical BlockUpTo + Normal DropNewest + 计数 |
| WS 写入阻塞 | 设置写超时 10s，超时后断连触发重连 |
| WS 读超时 | 60s 无 pong → 断开 |
| Runner 事件通道满 | trpc 框架内部处理（`EmitEventTimeoutErr`） |

### 14.2 连接管理

| 组件 | 限制 |
|------|------|
| 每 Session 最大连接数 | 5（`maxSessionConns`） |
| 全局监控最大连接数 | 3（`maxGlobalConns`） |
| EventBus 订阅者 buffer | 默认 128，最大 512 |
| EventBuffer 每 Session | 200 条 Envelope |
| EventBuffer TTL | 30 分钟 |
| Envelope 大小 | 单条最大 1MB |

### 14.3 延迟优化

| 优化点 | 方法 |
|--------|------|
| WS 消息发送 | 无需 HTTP 解析，帧头仅 2-14 字节 |
| JSON 序列化 | 预分配 buffer，避免频繁分配 |
| EventBus 路由 | 读锁 + 无锁 channel 发送 |
| StateDelta 持久化 | 批量合并（同一 Session 的多个 Delta 合并为一次写） |

---

## 十五、安全考量

| 风险 | 缓解措施 |
|------|---------|
| WS 跨域 | Origin 白名单校验（`OriginAllowed`：localhost + 环境变量） |
| 事件泄露 | EventBus 订阅时按 session_id 路由，WS 连接校验 token 归属 |
| WS 消息注入 | 上行消息类型白名单，payload 校验 |
| XSS via content | Envelope content 做 HTML 转义后渲染（前端职责） |
| DDoS via 大量连接 | 限制每 Session 最大连接数（5）+ 全局监控连接数（3） |
| JWT 过期 | WS 连接期间定期校验 token 有效性（**未实现**，见 [development.md §3](./51-message-mechanism.development.md#3-差距与优化)） |
| 消息大小 | WS ReadLimit 1MB，超限断连 |
| WS 认证 | 三级回退认证（URL token → Authorization Header → Cookie） |

---

## 十六、与现有模块的关系

### 16.1 替代关系

| 旧模块 | 新设计 |
|--------|--------|
| `TeamRunEventBroker` | `EventBus` |
| `MonitorLogBroker` + 独立端口 | `EventBus` + Flow Log v2 + WS channel:monitor |
| `streamWriter.Emit()` | `EventBus.Publish()` |
| `trpc_turn.go` 事件循环 | `ConsumeEventStream` + `EventProjector` / `ActivityProjector` |
| 独立 SSE Server（`:8001`） | 删除 |
| `team_run_sse.go` | 合并到 ws.go |
| `publishTeamMonitor` | 删除（已有 `team.run.*` flow_log） |
| `slog_bridge.go` | 删除（Flow Log v2 替代） |

### 16.2 不变部分

| 模块 | 说明 |
|------|------|
| `Runner.Run()` | 框架 API 不变，返回 `<-chan *event.Event` |
| `BuildTRPCLLMAgent` | Agent 构建不变 |
| `NewTRPCRunner` | Runner 创建不变 |
| `ChatService` HTTP 入口 | POST /v1/chat/messages 保留为非流式 / 后台入口 |
| Proto API | 不修改 proto，WS 不走 proto |

---

# 前端消息机制设计

> 本节定义 Aranea-Agents 前端的通信消息机制，聚焦传输协议、客户端实现和场景适配。

---

## 十七、传输层选型

### 17.1 决策

```
WebSocket = 主传输通道（双向通信、多路复用、低延迟）
Chat HTTP = 非流式 / 后台入口（HTTP POST /v1/chat/messages）
```

WebSocket 在实时交互维度优于 HTTP unary：

| 维度 | 说明 |
|------|------|
| **方向性** | 双向通信，cancel/enqueue/subscribe 无需额外 HTTP |
| **连接数** | 1 个 WS 连接复用所有通道（Chat+Monitor+Team+Graph） |
| **浏览器限制** | 无硬限制，多 Session 不受连接数约束 |
| **协议开销** | 握手后仅 2-14 字节帧头，高频事件场景带宽节省显著 |
| **多路复用** | Channel 机制天然支持 monitor/team/chat/graph 统一连接 |

---

## 十八、WebSocket 协议设计

### 18.1 连接建立

```
WS /v1/ws?session_id=sess-uuid&token=jwt_token

认证方式（三级回退）：
1. URL token 参数（优先）
2. Authorization: Bearer Header（浏览器 WebSocket API 不支持，仅非浏览器客户端）
3. access_token Cookie（浏览器场景的必要回退，浏览器 WebSocket API 无法设置自定义 Header）

前端连接时自动从 Cookie 读取 token：
  buildWsUrl({ sessionId, lastEventId, token: readAccessTokenCookie() })

握手时校验：
1. JWT token 有效（从 URL / Header / Cookie 三处提取）
2. session_id 存在且用户有权限
3. Origin 白名单校验

成功后服务端发送：
{
  "direction": "server_to_client",
  "channel": "system",
  "type": "connected",
  "payload": {
    "session_id": "sess-uuid",
    "server_time": "2026-01-01T00:00:00.000Z",
    "subscribed_channels": ["chat", "system"],
    "last_event_id": "evt-100"
  }
}
```

### 18.2 下行消息格式（Server → Client）

所有下行消息统一为 Envelope，通过 `channel` 字段多路复用：

```json
{
  "direction": "server_to_client",
  "channel": "chat | monitor | team | graph | system",
  "envelope": {
    "id": "evt-uuid",
    "type": "text_delta | tool_call | ...",
    "author": "agent_name",
    "session_id": "sess-uuid",
    "team_id": "team-uuid",
    "request_id": "req-uuid",
    "invocation_id": "inv-uuid",
    "parent_invocation_id": "parent-inv-uuid",
    "branch": "agent_a/agent_b",
    "filter_key": "agent_a/agent_b",
    "tag": "code_execution_code;transfer",
    "timestamp": "2026-01-01T00:00:00.000Z",
    "version": 1,
    "channel": "chat",
    "content": { "text": "...", "reasoning": "...", "is_partial": true },
    "tool_call": { "..." },
    "state_delta": { "..." },
    "transfer": { "..." },
    "error": { "..." },
    "usage": { "..." },
    "extensions": { "..." },
    "actions": { "..." },
    "trace": { "..." },
    "metadata": {}
  }
}
```

**系统消息**（非 Envelope）：

```json
{
  "direction": "server_to_client",
  "channel": "system",
  "type": "connected | pong | server_shutdown | replay_start | replay_end",
  "payload": { ... }
}
```

### 18.3 上行消息格式（Client → Server）

```json
{
  "direction": "client_to_server",
  "channel": "chat | control",
  "type": "user_message | cancel | enqueue_message | subscribe | unsubscribe | enable_log | ping",
  "request_id": "req-uuid",
  "payload": {}
}
```

### 18.4 上行消息类型

#### user_message — 发送聊天消息

```json
{
  "direction": "client_to_server",
  "channel": "chat",
  "type": "user_message",
  "request_id": "req-uuid",
  "payload": {
    "content": "帮我分析一下这段代码",
    "agent_key": "default",
    "team_id": "",
    "options": {}
  }
}
```

#### cancel — 停止生成

```json
{
  "direction": "client_to_server",
  "channel": "chat",
  "type": "cancel",
  "request_id": "req-uuid",
  "payload": {}
}
```

#### enqueue_message — 中途插入消息（SteerableRunner）

```json
{
  "direction": "client_to_server",
  "channel": "chat",
  "type": "enqueue_message",
  "request_id": "req-uuid",
  "payload": {
    "content": "请同时考虑性能优化"
  }
}
```

#### subscribe — 动态订阅通道

```json
{
  "direction": "client_to_server",
  "channel": "control",
  "type": "subscribe",
  "payload": {
    "channel": "team",
    "filter_key": "coordinator/agent_b"
  }
}
```

#### unsubscribe — 取消订阅通道

```json
{
  "direction": "client_to_server",
  "channel": "control",
  "type": "unsubscribe",
  "payload": {
    "channel": "team"
  }
}
```

#### enable_log — 开启/关闭 Monitor 日志流

```json
{
  "direction": "client_to_server",
  "channel": "control",
  "type": "enable_log",
  "payload": {
    "enabled": true
  }
}
```

#### ping — 心跳

```json
{
  "direction": "client_to_server",
  "channel": "control",
  "type": "ping",
  "payload": {}
}
```

服务端回复：

```json
{
  "direction": "server_to_client",
  "channel": "system",
  "type": "pong",
  "payload": {
    "server_time": "2026-01-01T00:00:00.000Z"
  }
}
```

### 18.5 Channel 定义

| Channel | 方向 | 说明 | 默认订阅 |
|---------|------|------|---------|
| `chat` | 双向 | Chat 事件 + 用户消息上行 | ✅ 连接即订阅 |
| `monitor` | 下行 | 运维日志、系统事件 | ❌ 需 enable_log 开启 |
| `team` | 下行 | Team 运行事件 | ✅ 全局模式自动订阅 / 普通模式需 subscribe |
| `graph` | 下行 | Graph 工作流事件 | ✅ 全局模式自动订阅 / 普通模式需 subscribe |
| `system` | 下行 | 系统通知（connected/pong/server_shutdown） | ✅ 连接即订阅 |

### 18.6 心跳与断连检测

```
应用层心跳：
  客户端 → 服务端：每 25s 发送应用层 ping
  服务端 → 客户端：回复 pong（含 server_time）

协议层心跳：
  服务端 → 客户端：每 30s 发送 WebSocket Ping 帧
  客户端 → 服务端：自动回复 Pong 帧

客户端检测：
  - 60s 无 pong → 认为连接断开，触发重连
  - 重连策略：指数退避（1s/2s/4s/8s/16s/30s cap）
  - 重连时携带 last_event_id，服务端从 EventBuffer 重放

服务端关闭通知：
  - 服务端优雅关闭时发送 server_shutdown 系统消息
  - 客户端收到后不再自动重连
```

### 18.7 重连与事件重放

```
1. 客户端断连后发起重连
2. WS 握手携带 last_event_id 参数：
   WS /v1/ws?session_id=sess-uuid&last_event_id=evt-100
3. 服务端从 EventBuffer 查询 evt-100 之后的事件
4. 先发送重放事件（channel 不变），再切换到实时流

服务端同步屏障：
  重放期间 eventPump 阻塞等待 replayDone 通道关闭，
  确保重放事件全部发送完毕后才开始转发实时事件，避免两者交错。

重放消息标记：
{
  "direction": "server_to_client",
  "channel": "chat",
  "type": "replay_start",
  "payload": { "count": 15, "from_id": "evt-101", "to_id": "evt-115" }
}
{
  "direction": "server_to_client",
  "channel": "chat",
  "envelope": { ... }  // 重放事件
}
{
  "direction": "server_to_client",
  "channel": "chat",
  "type": "replay_end",
  "payload": { "last_event_id": "evt-115" }
}
```

---

## 十九、Envelope TypeScript 类型定义

### 19.1 类型定义

```typescript
// web/src/realtime/envelope.ts

export type EnvelopeType =
  // Chat
  | 'text_delta' | 'text_done' | 'tool_call' | 'tool_result'
  | 'state_delta' | 'transfer' | 'runner_completion' | 'error'
  | 'context_usage' | 'run_status' | 'intent_pass'
  | 'user_feedback' | 'token_usage'
  // Monitor
  | 'log' | 'flow_log'
  | 'mcp.session.reconnect' | 'mcp.health.alert' | 'alert.notify'
  | 'monitor.auto_healed' | 'monitor.self_check_completed'
  // Graph
  | 'graph_node_start' | 'graph_node_end' | 'graph_node_error'
  | 'graph_step' | 'graph_execution_done' | 'graph_node_custom'
  | 'graph_task_status' | 'checkpoint'
  // Team
  | 'member_message_start' | 'member_delta' | 'member_message_done'
  | 'team_run_started' | 'team_run_finished' | 'team_run_failed'
  | 'team_step_started' | 'team_step_finished' | 'team_summary'
  | 'orchestration_agent_status'
  // Spirit
  | 'spirit_team_assembled' | 'spirit_team_completed' | 'spirit_team_failed'
  | 'spirit_team_cancelled' | 'spirit_team_interrupted'
  | 'spirit_team_progress' | 'spirit_teams_all_completed'
  | 'spirit_synthesis_completed' | 'spirit_plan_created'
  | 'spirit_allocation_created' | 'spirit_orchestration_started'
  | 'spirit_orchestration_checkpoint' | 'spirit_orchestration_interrupted'
  // Butler
  | 'butler.orchestration.started' | 'butler.orchestration.completed'
  | 'butler.orchestration.failed'
  // Knowledge
  | 'knowledge_ingest'
  // System
  | 'session.status_changed' | 'metrics_updated'
  | 'skill.health_changed' | 'skill.evolution_proposed'
  // Chat-visible execution progress
  | 'execution_progress'
  // AF: Activity-First envelope types
  | 'activity_start' | 'activity_delta' | 'activity_done' | 'activity_child_start';

export interface Envelope {
  id: string;
  type: EnvelopeType;
  author: string;
  session_id: string;
  team_id?: string;
  request_id?: string;
  invocation_id?: string;
  parent_invocation_id?: string;
  branch?: string;
  filter_key?: string;
  tag?: string;
  timestamp: string;
  version: number;
  channel?: string;
  session_revision?: number;
  content?: EnvelopeContent;
  tool_call?: EnvelopeToolCall;
  state_delta?: EnvelopeStateDelta;
  transfer?: EnvelopeTransfer;
  error?: EnvelopeError;
  usage?: EnvelopeUsage;
  extensions?: Record<string, string>;
  actions?: EnvelopeActions;
  trace?: EnvelopeTrace;
  metadata?: Record<string, unknown>;
}

export interface EnvelopeContent {
  text: string;
  reasoning?: string;
  is_partial: boolean;
}

export interface EnvelopeToolCall {
  id: string;
  name: string;
  arguments_json: string;
  result_json?: string;
  status: string;
  duration_ms?: number;
  is_long_running?: boolean;
  activity_kind?: string;
  display_label?: string;
  icon_key?: string;
  summary?: string;
  started_at?: string;
  finished_at?: string;
  error_code?: string;
  agent_key?: string;
  agent_id?: string;
  agent_name?: string;
  run_id?: string;
  trace_id?: string;
}

export interface EnvelopeStateDelta {
  operation: 'set' | 'append' | 'delete';
  path: string;
  value_json: string;
}

export interface EnvelopeTransfer {
  from_agent: string;
  to_agent: string;
}

export interface EnvelopeError {
  type: 'run_error' | 'stream_error' | 'tool_error';
  code?: string;
  message: string;
  hint?: string;
  pending_id?: string;
}

export interface EnvelopeUsage {
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  max_tokens?: number;
  context_prompt_tokens?: number;
  turn_total_tokens?: number;
}

export interface EnvelopeActions {
  skip_summarization?: boolean;
}

export interface EnvelopeTrace {
  agent_name: string;
  invocation_id: string;
  step_count: number;
  duration_ms?: number;
}
```

### 19.2 WS 消息类型

```typescript
export interface WsDownstream {
  direction: 'server_to_client';
  channel: string;
  type?: string;
  payload?: Record<string, unknown>;
  envelope?: Envelope;
}

export interface WsUpstream {
  direction: 'client_to_server';
  channel: string;
  type: string;
  request_id?: string;
  payload?: Record<string, unknown>;
}
```

---

## 二十、传输层实现

### 20.1 createWsTransport

```typescript
// web/src/realtime/ws-transport.ts

export interface WsTransportOptions {
  sessionId: string;
  lastEventId?: string;
  token?: string;
  onEnvelope?: (env: Envelope) => void;
  onConnected?: (info: { sessionId: string; lastEventId?: string }) => void;
  onDisconnected?: () => void;
  onError?: (error: Event) => void;
}

export interface WsTransport {
  connect(): void;
  disconnect(): void;
  send(upstream: WsUpstream): void;
  connected(): boolean;
  lastEventId(): string | undefined;
}

export function createWsTransport(opts: WsTransportOptions): WsTransport
```

**关键实现细节**：

1. **Pending 队列**：连接未建立时上行消息入队，连接建立后自动刷新
2. **server_shutdown 处理**：收到服务端关闭通知后主动断开，不再自动重连
3. **Cookie token 回退**：`readAccessTokenCookie()` 从 Cookie 读取 token 作为浏览器场景的认证回退
4. **lastEventId 追踪**：自动记录最新事件 ID，重连时携带
5. **应用层心跳**：25s 间隔发送 ping
6. **指数退避重连**：1s/2s/4s/8s/16s/30s cap

### 20.2 URL 构建

```typescript
// web/src/config/runtime.ts

export function buildWsUrl(opts: {
  sessionId: string;
  lastEventId?: string;
  token?: string;
}): string {
  const base = getWsBaseUrl();
  const params = new URLSearchParams();
  params.set('session_id', opts.sessionId);
  if (opts.lastEventId) params.set('last_event_id', opts.lastEventId);
  if (opts.token) params.set('token', opts.token);
  return `${base}/v1/ws?${params.toString()}`;
}

export function readAccessTokenCookie(): string | undefined {
  if (typeof document === 'undefined') return undefined;
  const match = document.cookie.match(/(?:^|;\s*)access_token=([^;]*)/);
  return match ? match[1] : undefined;
}
```

---

## 二十一、事件分发器

### 21.1 EnvelopeDispatcher 类

```typescript
// web/src/realtime/dispatcher.ts

export type EnvelopeHandler = (env: Envelope) => void;

export type DispatcherFilter = {
  channels?: string[];
  types?: EnvelopeType[];
  sessionId?: string;
  teamId?: string;
  filterKey?: string;
};

export class EnvelopeDispatcher {
  onType(type: EnvelopeType, handler: EnvelopeHandler): () => void;
  onChannel(channel: string, handler: EnvelopeHandler): () => void;
  on(handler: EnvelopeHandler): () => void;
  dispatch(env: Envelope): void;
}

export function matchFilterKey(subscriberKey: string, eventKey: string): boolean;
```

**设计决策**：使用类而非 composable 函数，因为：
- 分发器需要在多个组件间共享实例
- 支持按 type / channel / global 三级过滤
- 提供 `matchFilterKey` 辅助方法对齐后端前缀匹配语义

---

## 二十二、场景 Hooks

### 22.1 useEnvelopeStream（核心）

```typescript
// web/src/realtime/useEnvelopeStream.ts

export function useEnvelopeStream(sessionId: string, opts?: UseEnvelopeStreamOptions): UseEnvelopeStreamReturn {
  // 通过 globalWsHub 共享 WS 连接（多 Session 复用）
  // 创建 EnvelopeDispatcher 实例
  // onMounted/onUnmounted 管理连接生命周期
  return { transport, dispatcher, send: transport.send };
}
```

### 22.2 场景特定 Hooks

```typescript
// web/src/features/chat/useEnvelopeStream.ts

export function createChatStream(sessionId: string, opts?: ChatStreamFactoryOpts): UseEnvelopeStreamReturn
export function useChatStream(sessionId: string): { text, reasoning, toolCalls, done, error, send, cancel }
export function createTeamStream(sessionId: string, opts?: ChatStreamFactoryOpts): UseEnvelopeStreamReturn
export function useTeamStream(sessionId: string): { members, transfers, done }
export function useMonitorStream(sessionId: string): { logs, enableLog, disableLog }
```

### 22.3 辅助模块

| 文件 | 说明 |
|------|------|
| `web/src/realtime/globalWsHub.ts` | 全局 WS 连接管理（多 Session 复用、引用计数） |
| `web/src/realtime/graphState.ts` | Graph 状态管理类型 |
| `web/src/features/chat/envelopeRunStatus.ts` | run_status 解析 |
| `web/src/features/chat/inboundSyncEnvelope.ts` | session_revision 入站同步处理 |
| `web/src/features/chat/useEventFilter.ts` | 事件过滤辅助 |
| `web/src/features/chat/eventFilter.ts` | 事件过滤器 |
| `web/src/features/monitor/useLogStreamHub.ts` | flow_log / log 分流；FlowTracePanel 按 trace 过滤 |

---

## 二十三、场景交互流程

### 23.1 Chat 对话

```
1. 前端建立 WS 连接
   WS /v1/ws?session_id=sess-1&token=jwt

2. 服务端发送 connected
   ← {channel:"system", type:"connected", payload:{subscribed_channels:["chat","system"]}}

3. 前端发送用户消息
   → {direction:"client_to_server", channel:"chat", type:"user_message", payload:{content:"分析代码"}}

4. 服务端推送事件
   ← {channel:"chat", envelope:{type:"text_delta", content:{text:"我来分析", is_partial:true}}}
   ← {channel:"chat", envelope:{type:"tool_call", tool_call:{name:"read_file",...}}}
   ← {channel:"chat", envelope:{type:"tool_result", tool_call:{name:"read_file", result_json:"..."}}}
   ← {channel:"chat", envelope:{type:"text_delta", content:{text:"这段代码...", is_partial:true}}}
   ← {channel:"chat", envelope:{type:"text_done", content:{text:"这段代码有问题", is_partial:false}}}
   ← {channel:"chat", envelope:{type:"runner_completion", usage:{context_prompt_tokens, max_tokens, turn_total_tokens, ...}}}

5. 前端可随时取消
   → {direction:"client_to_server", channel:"chat", type:"cancel", request_id:"req-1"}

6. 前端可动态开启 Monitor 日志
   → {direction:"client_to_server", channel:"control", type:"enable_log", payload:{enabled:true}}
   ← {channel:"monitor", envelope:{type:"log", metadata:{level:"ERROR",...}}}
```

### 23.2 Team 多 Agent 场景

```
1. 前端连接 WS，发送 subscribe 订阅 team 通道
   → {direction:"client_to_server", channel:"control", type:"subscribe", payload:{channel:"team"}}

2. 收到 Team 生命周期事件
   ← {channel:"team", envelope:{type:"team_run_started"}}
   ← {channel:"team", envelope:{type:"team_step_started"}}

3. 收到成员消息
   ← {channel:"team", envelope:{type:"member_message_start", author:"agent_b", branch:"coordinator/agent_b"}}
   ← {channel:"team", envelope:{type:"member_delta", author:"agent_b", content:{text:"从安全角度看", is_partial:true}}}
   ← {channel:"team", envelope:{type:"member_message_done", author:"agent_b", content:{text:"从安全角度看，这个方案需要...", is_partial:false}}}

4. 收到 Agent 转移
   ← {channel:"team", envelope:{type:"transfer", transfer:{from_agent:"coordinator", to_agent:"agent_b"}}}

5. 前端过滤特定 Agent
   → {direction:"client_to_server", channel:"control", type:"subscribe", payload:{channel:"team", filter_key:"coordinator/agent_b"}}

6. Team 运行完成
   ← {channel:"team", envelope:{type:"team_step_finished"}}
   ← {channel:"team", envelope:{type:"team_run_finished"}}
```

### 23.3 Graph 工作流场景

```
1. 前端连接 WS，发送 subscribe 订阅 graph 通道
   → {direction:"client_to_server", channel:"control", type:"subscribe", payload:{channel:"graph"}}

2. 收到节点事件
   ← {channel:"graph", envelope:{type:"graph_node_start", author:"step_1", trace:{...}}}
   ← {channel:"graph", envelope:{type:"text_delta", content:{text:"分析中...", is_partial:true}}}
   ← {channel:"graph", envelope:{type:"graph_node_end", author:"step_1", trace:{step_count:1}}}

3. 收到步骤进度
   ← {channel:"graph", envelope:{type:"graph_step"}}

4. 收到执行完成
   ← {channel:"graph", envelope:{type:"graph_execution_done"}}

5. 收到检查点
   ← {channel:"graph", envelope:{type:"checkpoint", state_delta:{path:"__checkpoint__", value_json:"..."}}}

6. HITL 中断恢复
   ← {channel:"chat", envelope:{type:"runner_completion", tag:"interrupt"}}
   → 用户审批
   → {direction:"client_to_server", channel:"chat", type:"user_message", payload:{content:"批准"}}
```

### 23.4 Monitor 日志场景

**流程日志（`flow_log`）**：Chat Turn / Team / 系统域经 `TraceEmitter` 推送，**无需** `enable_log`；前端 `useLogStreamHub` → Flow 面板（中文 title + severity 配色）。

**进程日志（`log`）**：Gateway/Plugin 等文本日志，需 `enable_log` 或连接参数 `log_enabled=1`（全局监控在 `ProcessLogEnabled` 时可默认开启）。

```
1. 发 Chat → Monitor Logs「流程」Tab 自动出现 flow_log（无需 enable_log）
   ← {channel:"monitor", envelope:{type:"flow_log", metadata:{severity:"ok", title:"…", step_id:"chat.llm.invoke", trace_id:"…"}}}

2. 可选：开启进程日志
   → {direction:"client_to_server", channel:"control", type:"enable_log", payload:{enabled:true}}
   ← {channel:"monitor", envelope:{type:"log", metadata:{level:"ERROR", source:"tool"}, content:{text:"…"}}}

3. 关闭进程日志（flow_log 仍下发）
   → enable_log enabled:false
```

### 23.5 服务端关闭场景

```
1. 服务端优雅关闭，广播 server_shutdown
   ← {channel:"system", type:"server_shutdown"}

2. 前端收到后不再自动重连
3. 用户可看到"服务已关闭"提示
```

---

## 二十四、前端文件结构

```
web/src/
  config/
    runtime.ts                     # buildWsUrl（含 token 参数）+ readAccessTokenCookie + buildHealthWsUrl
  realtime/                        # 实时传输核心层（从 features/chat/ 提升）
    ws-transport.ts                 # createWsTransport（含应用层心跳、Cookie token 回退、pending 队列、server_shutdown 处理）
    envelope.ts                     # Envelope 类型定义（64 种，与后端对齐）+ session_revision 字段
    dispatcher.ts                   # EnvelopeDispatcher 类（onType / onChannel / on + matchFilterKey）
    useEnvelopeStream.ts            # useEnvelopeStream 核心 composable（通过 globalWsHub 共享连接）
    globalWsHub.ts                  # 全局 WS 连接管理（多 Session 复用、引用计数）
    graphState.ts                   # Graph 状态管理类型
  features/
    chat/
      ws-transport.ts               # 再导出桶 → realtime/ws-transport
      envelope.ts                   # 再导出桶 → realtime/envelope
      dispatcher.ts                 # 再导出桶 → realtime/dispatcher
      useEnvelopeStream.ts          # useChatStream / useTeamStream / useMonitorStream + createChatStream / createTeamStream
      envelopeRunStatus.ts          # run_status 解析
      inboundSyncEnvelope.ts        # session_revision 入站同步处理
      inboundSyncRouting.ts         # 入站同步路由
      useEventFilter.ts             # 事件过滤辅助
      eventFilter.ts                # 事件过滤器
      globalWsHub.ts                # 再导出桶 → realtime/globalWsHub
      streamHandlers.ts             # 流处理器（context_usage / runner_completion 等）
      sessionContextPatch.ts        # session 上下文补丁
      conversationEventDispatcher.ts # 对话事件分发器
      channelWsCursor.ts            # Channel WS 游标
      channelInboundSession.ts      # Channel 入站会话
      channelInboundSessionRefresh.ts # Channel 入站会话刷新
    monitor/
      useLogStreamHub.ts            # flow_log / log 分流；FlowTracePanel 按 trace 过滤
      useMonitorRealtimeEvents.ts   # Monitor 实时事件
      useMonitorLogStreamPanel.ts   # Monitor 日志流面板
      useMonitorTraceFlow.ts        # Monitor trace 流
```

---

## 二十五、前端性能考量

| 优化点 | 方法 |
|--------|------|
| 消息合并 | 连续 `text_delta` 合并渲染（requestAnimationFrame） |
| 虚拟滚动 | 长对话使用虚拟列表 |
| 背压感知 | WS buffer 未清空时降低渲染频率 |
| 重连去抖 | 指数退避避免频繁重连 |
| 事件去重 | 重放期间跳过已处理的事件 ID |
| Pending 队列 | 连接未建立时上行消息入队，连接后自动刷新 |
| 全局 WS 连接复用 | `globalWsHub` 多 Session 共享同一 WS 连接，引用计数管理生命周期 |
