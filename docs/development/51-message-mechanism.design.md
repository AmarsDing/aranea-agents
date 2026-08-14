# 消息机制 — 设计文档

> **需求来源**：[51-message-mechanism.md](./51-message-mechanism.md)（用户故事、功能需求、验收标准、非功能需求）
> **开发计划**：[51-message-mechanism.development.md](./51-message-mechanism.development.md)（代码锚点、任务清单、Phase 划分、状态）
> **架构变更依据**：
> - ADR-02 Activity-First 事件持久化（已归档，设计内容已并入本文档）
> - ADR-03 统一总线架构（已归档，设计内容已并入本文档）
> - Chat 模块重构方案（已归档，设计内容已并入本文档）
>
> 本文档定义消息机制的架构设计、代码分层、数据模型、接口定义、技术选型、状态机、序列图与前端组件设计。后端与前端设计原属 `51a/51b` 子模块，现合并到本设计文档。

---

## 一、设计目标与原则

### 1.1 设计目标

| 目标 | 说明 |
|------|------|
| 单一 Activity 模型 | 所有 chat/system 业务事件统一为 Activity 语义单元，前端按 `kind` + `event` + `status` + `meta` 零推断渲染 |
| 双 Bus 职责清晰 | `ActivityEventBus` 传输 chat+system 业务事件；`MonitorEventBus` 传输高频监控事件（log/flow_log/mcp/alert） |
| 持久化与推送解耦 | 持久化 fire-and-forget，推送同步执行；DB I/O 不阻塞 WS 推送 |
| 最终一致性 | 持久化失败通过重试 + 死信缓冲 + API Backfill 补偿 |
| 双向通信 | WebSocket 原生支持上行（cancel/enqueue/subscribe/enable_log），无需额外 HTTP 端点 |
| 通道复用 | 一个 WebSocket 连接承载所有事件类型，多路复用 |
| 错误模型简化 | 删除 `ActivityKindError`，turn 失败统一用 `task.failed` 表达 |
| 可扩展 | 新场景（Graph 节点事件、A2A 消息、Artifact 通知）无需修改核心机制 |

### 1.2 设计原则

```
原则 1：trpc-agent-go event.Event 是运行时事件真相源，项目层只做投影（Event → Activity）
原则 2：Activity 是唯一的事件语义单元，Envelope/ChatMessage/WAL/EventStore 概念彻底废弃
原则 3：事件路由由双 Bus 承担（ActivityEventBus + MonitorEventBus），Service 层不直接管理订阅
原则 4：传输协议（WS）与事件模型解耦，WS 传输 ActivityEvent + MonitorEvent JSON
原则 5：WebSocket 是 Chat / Team / Graph / Monitor 的主传输通道；历史 Chat SSE 不再作为入口
原则 6：一个 WS 连接通过 Channel 多路复用所有事件类型
原则 7：客户端通过 subscribe/unsubscribe/enable_log 动态控制订阅范围
原则 8：错误是其他 Activity 的终态（failed 事件），不产生 parallel error Activity
```

---

## 二、架构总览

### 2.1 事件流转全景图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        事件产生层（运行时）                                   │
│                                                                             │
│  trpc Runner.Run()  →  <-chan *event.Event                                 │
│       │                                                                     │
│       │  事件类型：                                                          │
│       │  ├── chat.completion.chunk  (文本/推理/工具调用增量)                │
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
│  ConsumeEventStream → TurnStreamConsumer → ActivityProjector.Project       │
│       │                                                                     │
│       │  职责：                                                              │
│       │  1. trpc Event → Activity 语义单元（kind + event + status + meta） │
│       │  2. 投影到 ActivityEventSequencer（并行分发）                        │
│       │  3. EmitSystemEvent 发布 Domain=system 事件（不持久化）             │
│       ▼                                                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                   事件分发层（ActivityEventSequencer）                       │
│                                                                             │
│  processTask(activityID, task):                                             │
│   ├── 任务 1：持久化 fire-and-forget                                        │
│   │     persistChan（buffered）→ worker goroutine（FIFO）                   │
│   │       ├── retry 预算（5 次，100/200/400/800/1600ms，done 可中断）       │
│   │       └── retry 耗尽 → deadLetter 环形缓冲（512，FIFO，activityID 去重）│
│   │                                                                         │
│   └── 任务 2：推送同步（保留 per-activity FIFO）                             │
│         eventBus.Publish(ActivityEvent) → WSServer.activityEventPump       │
│                                                                             │
│  monitor 事件（log/flow_log/mcp/alert）独立路径：                            │
│   FlowTracker → MonitorEventBus.Publish(MonitorEvent) → WSServer.monitorEventPump │
├─────────────────────────────────────────────────────────────────────────────┤
│                        传输层（Server 层）                                    │
│                                                                             │
│  WS /v1/ws?session_id=xxx   ← 统一传输（双向、多路复用、挂入 Kratos HTTP） │
│  HTTP unary / WS 上行      ← POST /v1/chat/messages 或 WS user_message     │
│                                                                             │
│  下行协议：                                                                  │
│   ├── activity_event?  — ActivityEvent JSON（chat + system 业务事件）      │
│   ├── monitor_event?   — MonitorEvent JSON（log/flow_log/mcp/alert）       │
│   └── system 消息      — connected / pong / server_shutdown                │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 2.2 双 Bus 隔离

| Bus | 传输类型 | 承载事件 | 持久化 |
|-----|---------|---------|--------|
| `ActivityEventBus` | `biz.ActivityEvent` | chat 业务事件（Domain=chat）+ system 通知事件（Domain=system） | chat 持久化，system 不持久化 |
| `MonitorEventBus` | `contract.MonitorEvent` | 高频监控事件（log / flow_log / mcp.* / alert.*） | 不持久化（FlowLog 持久化由独立 consumer 处理） |

**已删除的 legacy Bus**：SessionBus（传输 Envelope）、旧 Envelope-based MonitorBus、ActivityBus（v2 双总线期）、`event.Bus` 接口、`contract.Bus` 接口、`RouteChannel` 路由机制。

### 2.3 已删除的旧模块

| 旧模块 | 说明 |
|--------|------|
| `Envelope` 结构体 + 60+ `EnvelopeType` 常量 | 已删除，活类型 `EnvelopeError`/`EnvelopeTokenUsage` + 5 个 `ErrorCode*` 常量提取到 `contract/envelope_types.go` |
| `RouteChannel` 路由表 | 已删除，所有 Activity 事件统一 chat channel |
| `event.Buffer`（环形缓冲 + WS replay） | 已删除，重连恢复改为 `ListActivities` RPC（API Backfill） |
| `EventProjector`（trpc Event → Envelope） | 已删除，由 `ActivityProjector` 替代 |
| `EventStore`（event_store 表 + 异步 persist） | 已删除，由 Activity 表 + ActivityEventSequencer 替代 |
| `EventWAL` / WBPF（先写后发） | 已删除，由并行异步持久化 + 重试 + 死信 + API Backfill 替代 |
| `EventBuffer` / `replayEvents` / `RevisionTracker` | 已删除，WS 重连不再走 replay，改用 API Backfill |
| `MessageStoreConsumer`（messages 表） | 已删除，messages 表已 DROP |
| `EventBusConsumer`（编排器） | 已删除，拆分为 4 个 typed consumer |
| `event_persist_handler.go` / `event_store.go` / `wal.go` / `buffer.go` / `reliability.go` / `bus.go` / `bus_adapter.go` / `framework_adapter.go` | 全部已删除 |
| `TeamRunEventBroker` + 独立端口 | 已删除，合并到 EventBus |
| `MonitorLogBroker` + 独立端口 | 已删除，合并到 MonitorEventBus + Flow Log v2 |
| 独立 SSE Server（`:8001`） | 已删除，统一走 WS 传输 |
| `slog_bridge.go` | 已删除，由 Flow Log v2 替代 |

---

## 三、核心数据模型（Activity 表）

### 3.1 Activity 表结构

`activities` 表是事件系统的唯一真相源，承载所有 chat/system 业务语义单元。`messages` / `event_store` / `event_wal` 表已 DROP。

| 字段类别 | 字段 | 类型 | 说明 |
|---------|------|------|------|
| 主键 | `id` | string(64) | Activity 唯一 ID |
| 分类 | `kind` | string(32) | 10 种 ActivityKind（见 §4.1） |
| 分类 | `status` | string(32) | 9 种 status（见 §7.1） |
| 归属 | `session_id` | string(128) | 所属会话 ID |
| 归属 | `turn_id` | string(128) | 所属轮次 ID |
| 归属 | `parent_activity_id` | string(64) | 父 Activity ID（树形嵌套） |
| 归属 | `spirit_session_id` | string(128) | Spirit Session ID（跨 session 聚合） |
| 归属 | `team_id` | string(128) | 所属 Team ID |
| 归属 | `dag_node_id` | string(128) | DAG 节点 ID（Graph 场景） |
| 时间 | `timestamp` | string | ISO8601 起始时间 |
| 时间 | `duration_ms` | int64 | 持续毫秒 |
| 时间 | `seq` | int64 | 全局发射序列号（前端稳定排序） |
| Token | `prompt_tokens` | int64 | kind=task 根 Activity 的输入 Token |
| Token | `completion_tokens` | int64 | kind=task 根 Activity 的输出 Token |
| 内容 | `content` | text | task/reply/session/team_stage/graph_stage 文本内容 |
| 内容 | `reasoning` | text | thinking 推理内容 |
| 工具 | `tool_name` | string(128) | kind=action 工具名 |
| 工具 | `tool_category` | string(32) | shell/browser/file_read/file_write/file_search/web_search/mcp/code/todo/other |
| 工具 | `tool_call_id` | string(128) | 工具调用 ID |
| 工具 | `tool_arguments` | text (Sensitive) | 工具参数 JSON |
| 工具 | `tool_result` | text (Sensitive) | 工具结果 JSON |
| 工具 | `tool_duration_ms` | int64 | 工具耗时 |
| 工具 | `tool_error_code` | string(64) | 工具错误码（ErrorCode* 常量） |
| 阶段 | `stage` | string(64) | kind=session/team_stage/graph_stage 阶段 |
| 阶段 | `depends_on` | JSON([]string) | DAG 依赖节点 |
| Agent | `agent_key` | string(128) | 发言 Agent key |
| Agent | `agent_name` | string(128) | 发言 Agent 名称 |
| 显示 | `collapsed` | bool | 折叠状态 |
| 显示 | `label` | string(128) | 标签 |
| 元数据 | `meta` | JSON(map) | Kind-specific 扩展（成员列表/DAG 节点/进度/token_usage/error_message 等） |

**索引**：
- `idx_activities_session_turn` (session_id, turn_id)
- `idx_activities_parent` (parent_activity_id)
- `idx_activities_spirit_session` (spirit_session_id)
- `idx_activities_team` (team_id)

**已删除的字段**：`role`（用 kind 表达）、`child_board_id`（用 parent_activity_id 表达，保留字段兼容 DB/proto 但无写入路径）、`tool_icon`（前端根据 tool_category 决定）。

### 3.2 LLM 上下文构建（替代 Message）

```go
// internal/biz/llm_context_builder.go
type LLMMessage struct {
    Role       string
    Content    string
    ToolCallID string
    ToolName   string
    Name       string // 发言者标识（用于 assistant 角色，标识团队成员）
}

// BuildLLMContext 从 Activity 表构建 LLM 上下文（替代原 Message 查询）
//
// 角色映射规则（LLM API 只接受 user/assistant/tool/system）：
//   - task    → user
//   - reply   → assistant（含团队成员回复，通过 agent_key 标识来源，不改变 role）
//   - action  → tool
//   - notice  → system
func BuildLLMContext(ctx context.Context, repo ActivityReader, sessionID, turnID string) ([]LLMMessage, error)
```

---

## 四、核心类型设计

### 4.1 ActivityKind 枚举（10 种，无 error kind）

```go
// internal/biz/activity.go
type ActivityKind string

const (
    // === 基础交互 ===
    ActivityKindTask       ActivityKind = "task"        // 用户消息/任务根/turn 容器
    ActivityKindThinking   ActivityKind = "thinking"    // 推理过程
    ActivityKindAction     ActivityKind = "action"      // 工具调用
    ActivityKindReply      ActivityKind = "reply"       // Agent 回复
    ActivityKindPlan       ActivityKind = "plan"        // 计划
    ActivityKindConfirm    ActivityKind = "confirm"     // 确认
    ActivityKindNotice     ActivityKind = "notice"      // 通知（含编排阶段、用户反馈、system 通知）

    // === Session 生命周期 ===
    ActivityKindSession    ActivityKind = "session"     // Session 创建/状态变更/完成

    // === Team/Graph 阶段 ===
    ActivityKindTeamStage  ActivityKind = "team_stage"  // 团队阶段
    ActivityKindGraphStage ActivityKind = "graph_stage" // Graph 阶段
)
```

**关键设计**：不保留 `ActivityKindError`。错误是其他 Activity 的终态，用 `event=failed` 表达（如工具失败 = `kind=action` + `event=failed`，团队失败 = `kind=team_stage` + `event=failed`），避免同一错误产生两个 Activity。

**已删除的 legacy kind**：`sub_task_board`（前端无 UI）、`delegate`（无调用方）、`error`（被 `task.failed` 替代）。

### 4.2 ActivityEventType 枚举（7 种业务语义事件）

```go
// internal/biz/activity_event.go
type ActivityEventType string

const (
    // ActivityEventCreated Activity 创建
    // 业务含义：新的思考/工具调用/回复/团队阶段等开始
    // 前端行为：新增对应 Block 组件
    ActivityEventCreated ActivityEventType = "created"

    // ActivityEventStreaming 流式追加（替代技术术语 "delta"）
    // 业务含义：思考流式文本、回复流式文本、工具参数流式输入
    // 前端行为：向现有 Block 追加文本，光标闪烁
    // meta.delta_field 标识追加字段：content/reasoning/tool_arguments
    ActivityEventStreaming ActivityEventType = "streaming"

    // ActivityEventUpdated 状态变更（非流式）
    // 业务含义：团队阶段变更（assembled → executing）、Graph 节点状态变更、进度更新
    // 前端行为：更新 Block 的状态/阶段/进度，不追加文本
    // meta.changed_fields 标识变更字段
    ActivityEventUpdated ActivityEventType = "updated"

    // ActivityEventCompleted 正常完成
    ActivityEventCompleted ActivityEventType = "completed"

    // ActivityEventFailed 失败（独立事件，非 completed + status=failed）
    // meta.error_code + meta.error_message 标识错误
    ActivityEventFailed ActivityEventType = "failed"

    // ActivityEventCancelled 取消（用户主动停止）
    // meta.cancel_reason 标识取消原因
    ActivityEventCancelled ActivityEventType = "cancelled"

    // ActivityEventChildCreated 子 Activity 创建
    // 是父 Activity 的事件，通知前端在父 Block 下新增子 Block
    // meta.child_activity_id 标识子 Activity
    ActivityEventChildCreated ActivityEventType = "child_created"
)
```

**streaming vs updated 边界**（必须遵守）：

| 维度 | streaming | updated |
|------|-----------|---------|
| 变更类型 | 文本追加（content/reasoning/tool_arguments） | 非文本变更（status/stage/progress/成员列表） |
| 频率 | 高频（每 token） | 低频（阶段变更） |
| 前端行为 | 追加文本，光标闪烁 | 更新状态/进度，不追加文本 |
| 批量合并 | 是（16ms 窗口） | 否 |
| meta 字段 | `meta.delta_field` | `meta.changed_fields` |

**child_created 语义**：是**父 Activity 的事件**，通知前端在父 Block 下新增子 Block。子 Activity 有自己完整的生命周期（独立发送 `created`/`streaming`/`completed`/...），父子解耦，子 Activity 可独立查询和渲染。

### 4.3 ActivityDomain 字段（chat / system）

```go
// internal/biz/activity_event.go
type ActivityDomain string

const (
    ActivityDomainChat   ActivityDomain = "chat"   // 持久化到 Activity 表
    ActivityDomainSystem ActivityDomain = "system" // 仅推送 WS，不持久化
)
```

| Domain | 含义 | 持久化 | 前端处理 |
|--------|------|--------|---------|
| `chat` | Chat 工作单元（task/thinking/action/reply/plan/confirm/notice/session/team_stage/graph_stage） | ✅ 持久化到 Activity 表 | 加入时间线渲染 |
| `system` | 系统通知（非 chat 工作单元，如编排阶段提示、临时通知） | ❌ 跳过持久化 | 作为通知处理（toast/notification），不加入时间线 |

**持久化规则实现**：`ActivityEventSequencer.publish` 中 `publishTask.persist` 字段控制；`EmitSystemEvent` 传 `persist=false`，普通 `Emit` 传 `persist=true`。

### 4.4 ActivityEvent 结构

```go
// internal/biz/activity_event.go

// ActivityEvent 是 ActivityEventBus 和 WS 传输的唯一格式
type ActivityEvent struct {
    Event    ActivityEventType `json:"event"`
    Activity Activity           `json:"activity"`
    Domain   ActivityDomain    `json:"domain,omitempty"` // 默认 chat
}
```

**WS 传输示例**：

```json
{
  "event": "created",
  "activity": {
    "id": "act_xxx",
    "kind": "team_stage",
    "status": "running",
    "session_id": "sess_team_xxx",
    "spirit_session_id": "sess_spirit_xxx",
    "team_id": "team_xxx",
    "stage": "assembled",
    "meta": { "members": [...], "task_summary": "..." },
    "timestamp": "2026-06-25T10:00:00Z",
    "seq": 12345
  },
  "domain": "chat"
}
```

### 4.5 Activity 关键方法

```go
// internal/biz/activity.go

// IsActivityTerminal 判断 Activity 是否处于终态（OnTurnEnd 保护用）
func IsActivityTerminal(status ActivityStatus) bool {
    switch status {
    case ActivityStatusCompleted, ActivityStatusFailed,
         ActivityStatusCancelled, ActivityStatusInterrupted,
         ActivityStatusPartialFailure:
        return true
    }
    return false
}
```

---

## 五、双 Bus 接口设计

### 5.1 ActivityEventBus 接口

```go
// internal/event/activityevent/bus.go

// ActivityEventBus 传输 biz.ActivityEvent，承载所有 chat + system 事件
type ActivityEventBus interface {
    Publish(ctx context.Context, event biz.ActivityEvent) error
    Subscribe(handler func(biz.ActivityEvent)) Subscription
}
```

**实现要点**：
- `Publish` 返回 `error`，用于 `processTask` 同步推送时检测失败
- 实现基于 `GenericBus[biz.ActivityEvent]`（trpc-agent-go 框架 Bus 泛型）
- 订阅者：WSServer.activityEventPump + 4 个 typed consumer（见 §11）

### 5.2 MonitorEventBus 接口与 MonitorEvent 类型

```go
// internal/event/contract/monitor_event.go

// MonitorEvent 从 envelope.go 拆出，承载高频监控事件
type MonitorEvent struct {
    ID        string            `json:"id"`
    Type      MonitorEventType  `json:"type"`           // log/flow_log/mcp.*/alert.notify/monitor.*
    Timestamp time.Time         `json:"timestamp"`
    Level     string            `json:"level,omitempty"`
    Message   string            `json:"message,omitempty"`
    SessionID string            `json:"session_id,omitempty"`
    Source    string            `json:"source,omitempty"`
    Metadata  map[string]any    `json:"metadata,omitempty"`
}

type MonitorEventType string

const (
    MonitorEventTypeLog               MonitorEventType = "log"
    MonitorEventTypeFlowLog           MonitorEventType = "flow_log"
    MonitorEventTypeMCPReconnect      MonitorEventType = "mcp.session.reconnect"
    MonitorEventTypeMCPHealthAlert    MonitorEventType = "mcp.health.alert"
    MonitorEventTypeAlertNotify       MonitorEventType = "alert.notify"
    MonitorEventTypeMonitorAutoHealed MonitorEventType = "monitor.auto_healed"
    MonitorEventTypeMonitorSelfCheck  MonitorEventType = "monitor.self_check_completed"
)

// MonitorBus 接口（传输 MonitorEvent）
type MonitorBus interface {
    Publish(ctx context.Context, event MonitorEvent)
    Subscribe(opts MonitorSubscribeOptions) (<-chan MonitorEvent, func())
    DropCount() uint64
}

type MonitorSubscribeOptions struct {
    SessionID  string
    Filter     func(MonitorEvent) bool // 类型/级别过滤
    BufferSize int
    GlobalMode bool // 全局监控模式（session_id=*）
}
```

**设计依据**：monitor 事件高频（100+/sec）、不持久化、与业务事件语义完全不同，独立类型 + 独立 bus 更清晰。

### 5.3 Infra 容器

```go
// internal/event/infra.go

// Infra 是事件层的依赖容器（精简后）
type Infra struct {
    MonitorEventBus contract.MonitorBus   // 监控事件 Bus
    lg              loggateway.Logger
}

// 已删除：SessionBus 字段、ProvideSessionBus、Infra.Publish、publishToBuses
//         MonitorBus 字段（旧 Envelope-based）、ProvideMonitorBus、monitorBusFromInfra
```

`Infra` 仅剩 `MonitorEventBus` + `lg` 两个字段。`ActivityEventBus` 由 `biz` 包独立 provider 提供，不放入 `event.Infra`（避免 `event→biz` 架构耦合）。

### 5.4 contract 子包（纯接口与值对象）

`internal/event/contract/`

| 文件 | 职责 |
|------|------|
| `monitor_event.go` | `MonitorEvent` 类型 + `MonitorEventType` 枚举 + `MonitorBus` 接口 + `MonitorSubscribeOptions` |
| `envelope_types.go` | 活类型提取：`EnvelopeError` + `EnvelopeTokenUsage` + 5 个 `ErrorCode*` 常量（`ErrorCodeToolTimeout`/`ErrorCodeToolError`/`ErrorCodeConfirmationRequired`/`ErrorCodeConfirmationDenied`/`ErrorCodeConfirmationTimeout`） |
| `dedup.go` | `EventDeduplicator` 去重器（从 `internal/event/dedup.go` 迁移） |

biz 层只 import `contract` 子包，禁止 import 父 `event` 包。父 `event` 包通过 type alias 向后兼容旧调用点。

**已删除的 contract 文件**：`envelope.go`（Envelope struct + 60 EnvelopeType 常量 + RouteChannel）、`bus.go`（Bus 接口 + SubscribeOptions + DropPolicy + ChannelPriority）、`reliability.go`（EventReliability 分级，仅被自身测试调用）。

---

## 六、ActivityProjector 投影器设计

### 6.1 设计思路

`ActivityProjector` 是 trpc `event.Event` 到 `Activity` 语义单元的转换点，替代已删除的 `EventProjector`。它承担：

1. **类型映射**：trpc ObjectType → ActivityKind + ActivityEventType
2. **内容提取**：从 `model.Response.Choices` 提取文本/推理/工具调用
3. **SpiritSessionID 修正**：通过 `ProjectMeta.SpiritSessionID` 正确传递跨 session 聚合 ID
4. **工具分类**：通过 `ToolCategorizer` 识别 `tool_category`
5. **OnError 重构**：见 §8
6. **EmitSystemEvent**：发布 Domain=system 事件（不持久化）

### 6.2 ActivityProjector 接口

```go
// internal/agent/activity_projector.go

type ActivityProjector struct {
    sequencer      ActivityEventSequencer
    toolCategorizer ToolCategorizer
    lg             loggateway.Logger
    // ... 内部缓存字段
}

// ProjectMeta 携带投影上下文
type ProjectMeta struct {
    SessionID        string   // 当前 session ID（team session 或 spirit session）
    SpiritSessionID  string   // Spirit session ID（跨 session 聚合查询）
    ParentSessionID  string   // 父 session ID
    RootSessionID    string   // 根 session ID
    RequestID        string
    TeamID           string
    MemberAgentKeys  []string
    // ... 其他字段
}

// Project 将单个 trpc Event 投影为 0..N 个 Activity 事件
func (p *ActivityProjector) Project(ctx context.Context, ev *trpcevent.Event, meta ProjectMeta) error

// EmitSystemEvent 发布 Domain=system 事件（非 chat 工作单元，不持久化）
func (p *ActivityProjector) EmitSystemEvent(ctx context.Context, kind biz.ActivityKind, content string, meta map[string]any)
```

### 6.3 事件循环实现

事件循环由 `ConsumeEventStream` 承担，位于 `internal/agent/turn_helpers.go`：

```go
func ConsumeEventStream(
    ctx context.Context,
    events <-chan *trpcevent.Event,
    projectMeta ProjectMeta,
    opts *StreamConsumeOptions,
    lg loggateway.Logger,
) EventStreamResult
```

`StreamConsumeOptions` 注入 `ActivityProjector`，将运行时事件投影为 Activity 语义单元。所有副作用（WS 写入、Activity 落库、用量记录）由 ActivityEventSequencer + ActivityEventBus 订阅者处理，事件循环本身只做投影。

### 6.4 工具类型识别

```go
// internal/agent/tool_category.go

type ToolCategory string

const (
    ToolCategoryShell      ToolCategory = "shell"
    ToolCategoryBrowser    ToolCategory = "browser"
    ToolCategoryFileRead   ToolCategory = "file_read"
    ToolCategoryFileWrite  ToolCategory = "file_write"
    ToolCategoryFileSearch ToolCategory = "file_search"
    ToolCategoryWebSearch  ToolCategory = "web_search"
    ToolCategoryMCP        ToolCategory = "mcp"
    ToolCategoryCode       ToolCategory = "code"
    ToolCategoryTodo       ToolCategory = "todo"
    ToolCategoryOther      ToolCategory = "other"
)

// ToolCategorizer 工具类型识别器（可注入）
type ToolCategorizer interface {
    Categorize(toolName string) ToolCategory
}
```

**识别策略**：优先查询工具注册表（`toolRegistry`，由 ToolService 启动时填充），前缀/名称匹配作为兜底（覆盖未注册工具）。`ActivityProjector` 通过构造函数注入 `ToolCategorizer`，便于测试 mock。

---

## 七、Activity 生命周期状态机

### 7.1 ActivityStatus 枚举（9 种）

```go
// internal/biz/activity.go
type ActivityStatus string

const (
    ActivityStatusPending       ActivityStatus = "pending"
    ActivityStatusRunning       ActivityStatus = "running"
    ActivityStatusToolRunning   ActivityStatus = "tool_running"
    ActivityStatusToolBlocked   ActivityStatus = "tool_blocked"
    ActivityStatusCompleted     ActivityStatus = "completed"
    ActivityStatusFailed        ActivityStatus = "failed"
    ActivityStatusPartialFailure ActivityStatus = "partial_failure"
    ActivityStatusCancelled     ActivityStatus = "cancelled"
    ActivityStatusInterrupted   ActivityStatus = "interrupted"
)
```

### 7.2 状态转换表

| from | event | to | 守卫条件 |
|------|-------|----|---------|
| (none) | created | pending / running | kind=task → pending；其余 → running |
| pending | streaming / updated | running | — |
| running | streaming | running | 文本追加，状态不变 |
| running | updated | running / tool_running / tool_blocked | 按 meta.changed_fields 决定 |
| running | child_created | running | 父 Activity 状态不变，子 Activity 独立流转 |
| running / tool_running / tool_blocked | completed | completed | 终态 |
| running / tool_running / tool_blocked | failed | failed | 终态；meta.error_code + meta.error_message |
| running / tool_running / tool_blocked | cancelled | cancelled | 终态；meta.cancel_reason |
| running | (HITL interrupt) | interrupted | 终态；用户审批后通过 user_message 恢复到 running |
| completed / failed / cancelled / interrupted / partial_failure | (OnTurnEnd) | (不变) | **终态保护**：OnTurnEnd 不覆盖终态，仅附加 token usage |

### 7.3 OnTurnEnd 终态保护

```go
// internal/agent/activity_projector.go

func (p *ActivityProjector) onTurnEnd(ctx context.Context, rootActivity *biz.Activity, usage Usage) {
    if biz.IsActivityTerminal(rootActivity.Status) {
        // 终态保护：仅附加 token usage，不覆盖状态
        rootActivity.PromptTokens = usage.PromptTokens
        rootActivity.CompletionTokens = usage.CompletionTokens
        p.sequencer.publish(ctx, rootActivity.ID, publishTask{
            event:   biz.ActivityEventUpdated,
            activity: *rootActivity,
            persist: true,
        })
        return
    }
    // 非终态：转换为 completed
    rootActivity.Status = biz.ActivityStatusCompleted
    rootActivity.PromptTokens = usage.PromptTokens
    rootActivity.CompletionTokens = usage.CompletionTokens
    p.sequencer.publish(ctx, rootActivity.ID, publishTask{
        event:    biz.ActivityEventCompleted,
        activity: *rootActivity,
        persist:  true,
    })
}
```

---

## 八、OnError 语义设计（无 ActivityKindError）

### 8.1 设计原则

错误是其他 Activity 的终态，用 `event=failed` 表达，不产生 parallel error Activity。这统一了「turn 失败」的表达：`task.failed` 即代表整 turn 失败，无需前端合并 parallel error kind。

### 8.2 OnError 处理流程

```go
// internal/agent/activity_projector.go

func (p *ActivityProjector) onError(ctx context.Context, rootActivity *biz.Activity, err error) {
    // 场景 1：存在 root task
    if rootActivity != nil {
        rootActivity.Status = biz.ActivityStatusFailed
        rootActivity.Meta["error_message"] = err.Error()
        rootActivity.Meta["error_type"] = reflect.TypeOf(err).String()
        rootActivity.Meta["error_code"] = extractErrorCode(err)
        p.sequencer.publish(ctx, rootActivity.ID, publishTask{
            event:    biz.ActivityEventFailed,
            activity: *rootActivity,
            persist:  true,
        })
        return
    }

    // 场景 2：无 root task，创建最小化 failed task Activity 兜底
    fallback := biz.Activity{
        ID:        generateID(),
        Kind:      biz.ActivityKindTask,
        Status:    biz.ActivityStatusFailed,
        SessionID: p.currentSessionID(),
        Meta: map[string]any{
            "error_message": err.Error(),
            "error_type":    reflect.TypeOf(err).String(),
            "fallback":      true,
        },
        Timestamp: now(),
    }
    p.sequencer.publish(ctx, fallback.ID, publishTask{
        event:    biz.ActivityEventFailed,
        activity: fallback,
        persist:  true,
    })
}
```

### 8.3 错误码常量（活类型，保留）

5 个 `ErrorCode*` 常量仍被生产代码使用，已从 `contract/envelope.go` 提取到 `contract/envelope_types.go`：

| 常量 | 用途 | 调用位置 |
|------|------|----------|
| `ErrorCodeToolTimeout` | 工具超时错误码 | `activity_projector.go` |
| `ErrorCodeToolError` | 工具失败错误码 | `tool_invocation_recorder.go` |
| `ErrorCodeConfirmationRequired` | 工具确认必需错误码 | `tool_invocation_recorder.go` / `tool_confirmation.go` |
| `ErrorCodeConfirmationDenied` | 工具确认拒绝错误码 | `tool_confirmation.go` |
| `ErrorCodeConfirmationTimeout` | 工具确认超时错误码 | `tool_confirmation.go` |

---

## 九、并行异步持久化设计

### 9.1 processTask 设计

```go
// internal/agent/activity_event_sequencer.go

type publishTask struct {
    event    biz.ActivityEventType
    activity biz.Activity
    persist  bool // Domain=system 传 false
}

func (s *activityEventSequencer) processTask(activityID string, task publishTask) {
    // 任务 1：持久化（fire-and-forget，不阻塞 consume）
    if task.persist && s.activityRepo != nil {
        select {
        case s.persistChan <- task: // 非阻塞投递到 buffered channel
        default:
            // persistChan 满，回退到同步 persistWithRetry（极端场景兜底）
            s.persistWithRetry(context.Background(), activityID, task)
        }
    }

    // 任务 2：WS 推送（同步，保留 per-activity FIFO 顺序）
    if s.eventBus != nil {
        event := biz.ActivityEvent{
            Event:    task.event,
            Activity: task.activity,
            Domain:   s.domainFor(task),
        }
        if err := s.eventBus.Publish(context.Background(), event); err != nil {
            s.lg.Warn("activity publish failed; frontend will reload via API",
                loggateway.StepID("agent.activity_sequencer.publish"),
                loggateway.Str("activity_id", activityID),
                loggateway.Err(err))
        }
    }
}
```

### 9.2 设计权衡

| 方案 | 总耗时 | consume 阻塞 | FIFO 保证 | 问题 |
|------|--------|-------------|----------|------|
| 旧方案（串行 WBPF） | persist + publish | 是 | 是 | DB I/O 阻塞推送 |
| 妥协方案（wg.Wait 两 goroutine） | max(persist, publish) | 是 | 是 | consume 仍等 max |
| **本方案（persist 异步 + publish 同步）** | publish（~5ms） | 否 | 是 | persist 失败需 backfill |

本方案让 consume 几乎不阻塞（只等 ~5ms 的推送），吞吐量提升 10x+，且保留 per-activity FIFO。

### 9.3 persist worker goroutine

```go
// internal/agent/activity_event_sequencer.go

func (s *activityEventSequencer) persistWorker() {
    defer s.wg.Done()
    for task := range s.persistChan {
        s.persistWithRetry(context.Background(), task.activity.ID, task)
    }
}
```

- 单 goroutine 消费 `persistChan`，保证 start→done 顺序（per-activity FIFO）
- `persistChan` 满时回退到同步 `persistWithRetry`（极端场景兜底）

### 9.4 Close 三阶段关闭

```go
func (s *activityEventSequencer) Close() error {
    // Stage 1：关闭消费者（停止接收新 task）
    close(s.consumeDone)
    s.consumeWg.Wait()

    // Stage 2：关闭 persistChan（让 worker 处理完剩余 task）
    close(s.persistChan)
    s.wg.Wait() // 等待 persist worker 完成

    // Stage 3：关闭 done 通道（让 retry 中的 sleep 立即返回，转入死信）
    close(s.done)
    return nil
}
```

**关键设计**：retry 退避通过 `select` 监听 `s.done` 通道，Close 期间立即放弃重试转入死信，避免 shutdown 被退避睡眠阻塞。

---

## 十、重试预算与死信缓冲设计

### 10.1 重试预算

```go
// internal/agent/activity_event_sequencer.go

const (
    persistMaxRetries        = 5
    persistInitialBackoffMs  = 100
    persistBackoffFactor     = 2
    // 退避序列：100 / 200 / 400 / 800 / 1600 ms，总预算 3100ms
    // 对齐 postgres busy_timeout=30000ms 的 1/10，避免占用写连接过久
)

func (s *activityEventSequencer) persistWithRetry(ctx context.Context, activityID string, task publishTask) {
    backoff := time.Duration(persistInitialBackoffMs) * time.Millisecond
    for attempt := 0; attempt < persistMaxRetries; attempt++ {
        if _, err := s.activityRepo.UpsertActivity(ctx, task.activity); err == nil {
            return // 成功
        }
        // 退避，可通过 s.done 中断
        select {
        case <-s.done:
            s.pushDeadLetter(activityID, task) // Close 期间转入死信
            return
        case <-time.After(backoff):
        }
        backoff *= persistBackoffFactor
    }
    // retry 耗尽，转入死信
    s.pushDeadLetter(activityID, task)
}
```

### 10.2 死信环形缓冲

```go
// internal/agent/activity_event_sequencer.go

const deadLetterCapacity = 512

type deadLetterBuffer struct {
    mu       sync.Mutex
    entries  []deadLetterEntry // 环形缓冲，FIFO 淘汰
    head     int               // 写入位置
    size     int               // 当前元素数
    seen     map[string]int    // activityID → 索引（去重用）
}

type deadLetterEntry struct {
    ActivityID string
    Activity   biz.Activity
    Event      biz.ActivityEventType
    FailedAt   time.Time
    Error      string
}

// pushDeadLetter 写入死信缓冲
// 同一 activityID 多次失败按最新快照去重（覆盖旧记录）
// 避免缓冲累积同一活动的过期中间态
func (s *activityEventSequencer) pushDeadLetter(activityID string, task publishTask) {
    s.deadLetter.push(deadLetterEntry{
        ActivityID: activityID,
        Activity:   task.activity,
        Event:      task.event,
        FailedAt:   time.Now(),
    })
}

// ListDeadLetterActivities 暴露给 WS 重连补偿路径
func (s *activityEventSequencer) ListDeadLetterActivities(sessionID string) []biz.Activity
```

**设计要点**：
- 容量 512，覆盖单 turn 最多 ~50 事件的 10 倍冗余
- FIFO 淘汰：缓冲满时淘汰最旧记录
- activityID 去重：同一 activityID 多次失败按最新快照覆盖，避免歧义消解
- 暴露方式：通过 `ListDeadLetterActivities(sessionID)` 供 WS 重连补偿路径查询

---

## 十一、内部消费者设计

### 11.1 Typed Consumer 拆分

旧 `EventBusConsumer`（编排器）已删除，拆分为 4 个 typed consumer，独立订阅 ActivityEventBus / MonitorEventBus：

| 消费者 | 订阅 Bus | 触发事件 | 职责 |
|--------|---------|---------|------|
| `ToolCallConsumer` | ActivityEventBus | `action.completed` / `action.failed` | 工具调用记录到 ToolInvocation（upsert） |
| `CallbackConsumer` | ActivityEventBus | `task.completed` / `task.failed` / `task.cancelled`（终态） | Webhook 回调（`WebhookDispatcher`） |
| `UsageRollupConsumer` | ActivityEventBus | `task.completed` | Token 用量汇总到 `model_token_usage_hourly` |
| `UserFeedbackConsumer` | ActivityEventBus | `notice.created`（meta.feedback） | 用户反馈处理 |
| `FlowLogPersistConsumer` | MonitorEventBus | `type=flow_log` | FlowLog 持久化 |

**编排**：`EventBusSideConsumers`（`internal/biz/event_bus_side_consumers.go`）统一编排启动/停止，独立 Bus 订阅，按 EventTypes 过滤。

### 11.2 已删除的 legacy consumer

| 旧 consumer | 删除原因 |
|------------|---------|
| `EventBusConsumer`（编排器） | 拆分为 4 个 typed consumer |
| `eventBufferHandler` | `event.Buffer` 已删除（WS replay 改用 API Backfill） |
| `eventPersistHandler` | `event_store` 表已删除，由 ActivityEventSequencer 替代 |
| `stateDeltaHandler` | StateDelta 改为 Activity 的 `streaming`/`updated` 事件 |
| `runnerCompletionHandler` | runner_completion 改为 `task.completed` 事件，由 UsageRollupConsumer 处理 |
| `MessageStoreConsumer` | messages 表已 DROP |

---

## 十二、WebSocket Server 实现设计

### 12.1 Server 层注册

WSServer 通过 `RegisterOnKratos` 挂入 Kratos HTTP Server，不独立监听端口：

```go
// internal/server/ws.go

type WSServer struct {
    mu             sync.RWMutex
    conns          map[string][]*wsConn
    monitorBus     contract.MonitorBus    // 监控事件
    activityBus    biz.ActivityEventBus   // 所有 chat + system 事件
    canceller      RunCanceller
    sender         ChatSender
    turnExecutor   WSTurnExecutor
    upgrader       websocket.Upgrader
    globalConns    []*wsConn
    maxSessionConns int
    maxGlobalConns  int
    lg             loggateway.Logger
}

func NewWSServerFromInfra(c *conf.Server, infra *event.Infra, activityBus biz.ActivityEventBus, canceller RunCanceller, sender ChatSender, turnExecutor WSTurnExecutor, runtimeConf *conf.Runtime, lg loggateway.Logger) *WSServer

func (s *WSServer) RegisterOnKratos(srv *kratoshttp.Server) {
    srv.HandleFunc("/v1/ws", s.handleWS)
}
```

**已删除的字段**：`eventBus event.Bus`（SessionBus 已删除）、`eventBuffer *event.Buffer`（WS replay Buffer 已删除）。

### 12.2 认证三级回退

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

### 12.3 Origin 校验

复用 `cors_filter.go` 中的 `OriginAllowed` 函数。白名单规则：localhost/127.0.0.1/[::1] 前缀 + 环境变量 `KRATOS_HTTP_EXTRA_CORS_ORIGINS`。

### 12.4 读泵（上行消息处理）

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

### 12.5 双 pump 设计（2 pump）

```go
// internal/server/ws_io_pump.go

// activityEventPump 从 ActivityEventBus 订阅，转发 ActivityEvent 到 WS
func (s *WSServer) activityEventPump(wc *wsConn) {
    sub := s.activityBus.Subscribe(func(ev biz.ActivityEvent) {
        s.sendToConn(wc, wsDownstream{
            Direction: "server_to_client",
            Channel:   "chat",
            ActivityEvent: &ev,
        })
    })
    defer sub.Unsubscribe()
    <-wc.ctx.Done()
}

// monitorEventPump 从 MonitorEventBus 订阅，转发 MonitorEvent 到 WS
func (s *WSServer) monitorEventPump(wc *wsConn) {
    ch, cancel := s.monitorBus.Subscribe(contract.MonitorSubscribeOptions{
        SessionID: wc.sessionID,
        GlobalMode: wc.globalMode,
    })
    defer cancel()
    for ev := range ch {
        s.sendToConn(wc, wsDownstream{
            Direction: "server_to_client",
            Channel:   "monitor",
            MonitorEvent: &ev,
        })
    }
}
```

**已删除的 pump**：`envelopeEventPump`（旧 Envelope 通道，Blocker A 已删除 WS replay 路径）。

### 12.6 全局监控模式

`session_id=*` 连接可订阅所有 Session 的 Monitor 事件（限 `maxGlobalConns` 个连接）。

### 12.7 服务端优雅关闭

```go
func (s *WSServer) Stop(ctx context.Context) error {
    // 广播 server_shutdown 到所有连接
    // 关闭所有连接
    return nil
}
```

### 12.8 上行消息处理

| 上行类型 | 处理方法 | 说明 |
|---------|---------|------|
| `user_message` | `handleUserMessage` | 调用 ChatService native turn |
| `cancel` | `handleCancel` | 调用 ChatService.CancelRun |
| `enqueue_message` | `handleEnqueueMessage` | 调用 `ChatService.EnqueueUserMessage`（steerable enqueue 或 pending 入队；无 active run 时返回 `enqueue_rejected` 错误） |
| `subscribe` | `handleSubscribe` | 动态订阅通道（含 filter_key） |
| `unsubscribe` | `handleUnsubscribe` | 取消订阅通道 |
| `enable_log` | `handleEnableLog` | 开启/关闭 Monitor 日志流 |
| `ping` | `sendPong` | 心跳回复 |

### 12.9 配置

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
  // Gateway process log (MonitorEventTypeLog) pushed on WS when true.
  bool process_log_enabled = 1;
}
```

---

## 十三、Monitor 日志统一接入设计

### 13.1 Flow Log v2（已替代 SlogBridge）

业务与系统可观测日志通过 **Flow Log v2** 发布为 `MonitorEventTypeFlowLog`（非全局 `slog` 桥接）：

| 组件 | 文件 | 说明 |
|------|------|------|
| Turn | `internal/event/trace_emitter.go` | `NewTraceEmitterForRun` → `emitter.Log*` |
| 系统 | `internal/event/flow_log.go` | `SysLog*` / `SessionSysLog*` |
| 上下文 | `internal/event/flow_context.go` | `WithTraceEmitter` / `TraceEmitterFromContext`（进程日志用 `loggateway.Logger`；FlowLogger 别名已删除） |

`internal/event/slog_bridge.go` **已删除**（2026-05-20）。详见 [52-flow-logger.design.md](./52-flow-logger.design.md)。

### 13.2 Monitor 事件来源

| 来源 | metadata.source / type | 说明 |
|------|------------------------|------|
| Runner 生命周期 | `team-runner` / `chat-native` | Agent 启动/完成/错误 |
| Tool 执行 | `tool` | 工具调用开始/结束/错误 |
| LLM 调用 | `llm` | 模型请求/响应/重试 |
| 系统事件 | `system` | 内存/连接/配置变更 |
| Intent Pass | `intent-pass` | 意图识别日志 |
| TraceEmitter / SysLog | `step_id` + 中文 title | 业务/系统 FlowLog → MonitorEvent |

**关键设计**：`sessionID` 参数必须传入，因为 MonitorEventBus 按 `session_id` 路由事件到 WS 客户端。空 `sessionID` 会导致日志事件无法送达任何订阅者。

### 13.3 订阅者迁移

`SelfHealObserver` 和 `TraceProjector` 已从旧 envelope bus 迁移到 `MonitorEventBus`（修复死订阅 bug）：

- `SelfHealObserver.StartEventDrivenObservation`：订阅 `MonitorEventTypeFlowLog`，全局模式
- `TraceProjector`：订阅 `MonitorEventTypeFlowLog`，按 session 过滤

---

## 十四、持久化失败补偿设计

### 14.1 三重保障

```
持久化失败
    │
    ▼
1. 重试预算（5 次，100/200/400/800/1600ms，done 可中断）
    │ retry 耗尽
    ▼
2. 死信环形缓冲（512，FIFO 淘汰，activityID 去重）
    │ 通过 ListDeadLetterActivities(sessionID) 暴露
    ▼
3. API Backfill（前端 WS 重连或 reload 时调用 listActivities(sessionId)）
    │ 最终一致兜底
    ▼
最终一致
```

### 14.2 可靠性分级（简化版）

彻底合并后，Activity 事件可靠性分级简化为：

| 级别 | Activity 事件 | 持久化 | 推送 |
|------|-------------|--------|------|
| Important | `created`/`completed`/`failed`/`cancelled`/`child_created` | 异步持久化，失败重试 | 同步推送，失败记录 |
| Informational | `streaming`/`updated` | 异步持久化，失败丢弃 | 同步推送（streaming 可批量合并），失败丢弃 |

**已删除 WAL**：Activity 表已是唯一真相源，无需额外 WAL 持久化。持久化失败通过 retry + API backfill 保证最终一致。

### 14.3 已删除的可靠性机制

| 旧机制 | 删除原因 |
|--------|---------|
| WBPF（Write-Before-Publish-Fanout） | DB I/O 阻塞 WS 推送，AF 场景下累计延迟不可接受 |
| EventWAL（SQLite WAL 持久化） | Activity 表替代，retry + API backfill 保证最终一致 |
| `BlockUpTo`（用于 Activity 事件） | 并行异步持久化不需要阻塞订阅者 |
| `EventReliability` 分级 | 仅被自身测试调用，已删除 |
| `event.Buffer`（WS replay 环形缓冲） | 重连恢复改用 `ListActivities` RPC（API Backfill） |
| `EventStore`（event_store 表持久化） | Activity 表替代 |

---

## 十五、场景事件流（序列图）

### 15.1 Chat 场景

```
用户消息
  → Runner.Run()
    → LLM 调用
      → thinking.created → thinking.streaming × N → thinking.completed
    → Tool 调用
      → action.created → action.streaming → action.completed
    → 回复
      → reply.created → reply.streaming × N → reply.completed
  → task.completed（含 token usage）
```

### 15.2 Team 场景

```
用户消息
  → Team Runner.Run()
    → team_stage.created（stage=assembled）
    → team_stage.updated（stage=executing）
    → Coordinator Agent
      → action.created（transfer_to_agent）
      → team_stage.child_created（meta.child_activity_id=成员 task）
        → 成员子 session：
          → task.created → thinking.* → reply.created → reply.streaming × N → reply.completed → task.completed
      → team_stage.updated（meta.member_status）
    → team_stage.completed（stage=completed）
```

**Team 事件投影**：Team Runner 的事件循环同样使用 `ConsumeEventStream` + `ActivityProjector` 将 trpc 事件投影为 Activity 事件，与 Chat 场景共享同一套投影逻辑。

### 15.3 Graph 场景

```
用户消息
  → GraphAgent.Run()
    → graph_stage.created（stage=planned）
    → graph_stage.updated（meta.current_node=step_1）
    → graph_stage.child_created（meta.child_activity_id=节点 task）
      → 节点子 session：task.created → action.* → task.completed
    → graph_stage.updated（meta.current_node=step_2）
    → graph_stage.completed（stage=completed）
```

### 15.4 Channel 场景

```
飞书 Webhook POST
  → ChannelIngress.HandleWebhook()
    → ChatService.RunNativeTurn()
      → runner.Run() → ActivityProjector → ActivityEventSequencer
        → ActivityEventBus.Publish → WS 推送 + 持久化 + typed consumer
```

### 15.5 断连恢复场景（API Backfill）

```
1. WS 断连，前端自动重连（指数退避：1s/2s/4s/8s/16s/30s cap）
2. 重连成功后调用 ListActivities(sessionId) RPC
3. 服务端返回该 session 当前所有持久化的 Activity（最新快照）
4. 前端用 API 返回值补齐缺失状态（最终一致性兜底）
5. 切换回实时流，继续接收新的 ActivityEvent
```

---

## 十六、分层实现清单（代码锚点）

> 状态与进度跟踪见 [development.md §1 代码锚点](./51-message-mechanism.development.md#1-模块定位)。

### 16.1 Event 层

| 文件 | 说明 |
|------|------|
| `internal/event/contract/monitor_event.go` | `MonitorEvent` 类型 + `MonitorEventType` 枚举 + `MonitorBus` 接口 + `MonitorSubscribeOptions` |
| `internal/event/contract/envelope_types.go` | 活类型提取：`EnvelopeError` + `EnvelopeTokenUsage` + 5 个 `ErrorCode*` 常量 |
| `internal/event/contract/dedup.go` | `EventDeduplicator` 去重器 |
| `internal/event/envelope.go` | contract 活类型的 type alias（`EnvelopeError`/`EnvelopeTokenUsage`）+ 4 个活 ErrorCode 常量重导出 |
| `internal/event/monitor_bus.go` | `MonitorBus` 实现（基于 `GenericBus`，传输 `contract.MonitorEvent`） |
| `internal/event/activityevent/bus.go` | `ActivityEventBus` 实现（传输 `biz.ActivityEvent`） |
| `internal/event/infra.go` | `Infra`：仅 `MonitorEventBus`（`contract.MonitorBus`）+ `lg` + `InfraProviderSet` |
| `internal/event/trace_emitter.go` | Flow Log v2 + UsageAggregator 桥接 |
| `internal/event/flow_context.go` | `WithTraceEmitter` / `TraceEmitterFromContext` / `NewTraceEmitterForRun`（FlowLogger 别名已删除） |
| `internal/event/flow_log.go` | `SysLog*` / `SessionSysLog*` 系统流程日志 |
| `internal/event/flow_tracker.go` | `FlowTracker` 流程追踪（发布到 `MonitorEventBus`） |
| `internal/event/session_revision.go` | `SessionRevisionBumper` 接口 + `BumpSessionRevision`（仅 bump 半边，publish 半边已删） |
| `internal/event/source.go` | 事件来源标识 |

**已删除的 Event 层文件**：`contract/envelope.go`、`contract/bus.go`、`contract/reliability.go`、`bus.go`、`bus_adapter.go`、`framework_adapter.go`、`buffer.go`、`wal.go`、`wire.go`（合并到 `infra.go`）。

### 16.2 Server 层

| 文件 | 说明 |
|------|------|
| `internal/server/ws.go` | WSServer 主文件（挂入 Kratos HTTP、三级认证、全局监控模式、server_shutdown） |
| `internal/server/ws_conn.go` | WS 连接管理 |
| `internal/server/ws_conn_manager.go` | WS 连接管理器 |
| `internal/server/ws_codec.go` | WS 编解码 |
| `internal/server/ws_message_handler.go` | WS 上行消息处理 |
| `internal/server/ws_io_pump.go` | WS 读写泵 + `activityEventPump` + `monitorEventPump` |
| `internal/server/ws_priority.go` | WS 优先级发送队列 |

**已删除的 Server 层文件**：`ws_event.go`（replayEvents / sendConnected replay 路径）、`ws_sync_request.go`（revision-based sync replay，已改用 ListActivities RPC）。

### 16.3 Agent 层

| 文件 | 说明 |
|------|------|
| `internal/agent/activity_projector.go` | `ActivityProjector`：运行时事件 → Activity 语义单元 + OnError 重构 + OnTurnEnd 终态保护 + `EmitSystemEvent` |
| `internal/agent/activity_event_sequencer.go` | `ActivityEventSequencer`：并行分发（persistChan + sync publish）+ retry + dead-letter |
| `internal/agent/tool_category.go` | `ToolCategorizer` 工具类型识别（10 类别前缀匹配 + 注册表覆盖） |
| `internal/agent/turn_helpers.go` | `ConsumeEventStream`：事件循环简化为投影+发布 |
| `internal/agent/turn_stream_helpers.go` | `TurnStreamConsumer` 实现 |
| `internal/agent/stream_consumer.go` | 流消费者 |

**已删除的 Agent 层文件**：`event_projector.go`（EventProjector Deprecated）、`activity_publish.go`（Legacy 工具卡片持久化）、`activity_persist.go`（ChatMessageFromToolActivity 转换）。

### 16.4 Biz 层

| 文件 | 说明 |
|------|------|
| `internal/biz/activity.go` | `Activity` 领域模型 + `ActivityKind`（10 种）+ `ActivityStatus`（9 种）+ `IsActivityTerminal` + `ToolCategory` 枚举 |
| `internal/biz/activity_event.go` | `ActivityEvent` 结构 + `ActivityEventType`（7 种）+ `ActivityDomain` |
| `internal/biz/event_bus_side_consumers.go` | `EventBusSideConsumers` 编排器（统一启动/停止 typed consumer） |
| `internal/biz/event_bus_tool_call_consumer.go` | `ToolCallConsumer`：action 终态 → ToolInvocation 落库 |
| `internal/biz/event_bus_callback_consumer.go` | `CallbackConsumer`：task 终态 → Webhook 回调 |
| `internal/biz/event_bus_usage_rollup_consumer.go` | `UsageRollupConsumer`：task.completed → 用量汇总 |
| `internal/biz/event_bus_user_feedback_consumer.go` | `UserFeedbackConsumer`：notice.created（meta.feedback）→ 用户反馈处理 |
| `internal/biz/event_bus_flow_log_consumer.go` | `FlowLogPersistConsumer`：flow_log → FlowLog 持久化 |
| `internal/biz/llm_context_builder.go` | `BuildLLMContext` 从 Activity 构建 LLM 上下文（替代 Message） |
| `internal/biz/domain_event.go` | `DomainEvent` 领域模型 |
| `internal/biz/session/state.go` + `state_usecase.go` | `SessionUsecase.ApplyStateDelta` / `GetSessionState` / `SaveSessionState` |
| `internal/biz/session/message_usecase.go` | Message 子用例（合法保留，复杂度管理模式） |

**已删除的 Biz 层文件**：`event_bus_consumer.go`（编排器，拆分为 typed consumer）、`event_bus_buffer_handler.go`、`event_bus_runner_handler.go`、`event_bus_state_handler.go`、`event_persist_handler.go`、`event_store.go`、`event_bus_message_store_consumer.go`、`domain_event_adapter.go`（DomainEvent bridge 已迁移到 ActivityEventBus）。

### 16.5 Service 层

| 文件 | 说明 |
|------|------|
| `internal/service/chat.go` | ChatService 主文件：`SendChatMessage` / `CancelRun` / `EnqueueUserMessage` |
| `internal/service/chat_native.go` | `RunNativeTurn` / `ExecuteTurn`：HTTP unary / WS 上行复用 native turn |
| `internal/service/chat_enqueue.go` | `EnqueueUserMessage` 实现（steerable enqueue / pending 入队） |
| `internal/service/chat_event_publisher.go` | Chat 事件发布 |
| `internal/service/chat_run_gateway.go` | Run 网关 |
| `internal/service/session.go` | SessionService（含 `toProtoSession` 映射，含 `session_type`/`execution_stage` 等字段） |
| `internal/service/activity.go` | ActivityService（`ListActivities` RPC，API Backfill 入口） |

### 16.6 Data 层

| 文件 | 说明 |
|------|------|
| `internal/data/activity_repo.go` | Activity Repo（`UpsertActivity` / `ListBySession` / `ListBySpiritSession` / `ListByTeam` / `ListByParentSession`） |
| `internal/data/ent/schema/activity.go` | Activity 表 Schema |
| `internal/data/ent/schema/session.go` | Session 表 Schema（含 `session_type`/`execution_stage` 等父子层级字段） |
| `internal/data/session_state_repo.go` | Session State 持久化（json_set / json_remove） |

**已删除的 Data 层文件**：`message_repo.go`、`event_store_repo.go`、`ent/schema/message.go`、`ent/schema/event_store.go`、`sql/message_fts.sql`（messages 表已 DROP）。

### 16.7 Metrics

| 文件 | 说明 |
|------|------|
| `internal/metrics/vars.go` | Prometheus 指标（持久化失败率、WS 推送延迟等） |

### 16.8 配置

| 文件 | 说明 |
|------|------|
| `internal/conf/conf.proto` | `Server.WS` 消息（enable / network / addr）+ `Server.Monitor.process_log_enabled` |

### 16.9 Proto API

| 文件 | 说明 |
|------|------|
| `api/kratos/session/v1/session.proto` | `SearchSessionMessages` RPC（FTS5 搜索，已迁移到 Activity 表） |
| `api/kratos/activity/v1/activity.proto` | `ListActivities` RPC（API Backfill 入口） |

---

## 十七、Wire 注入

```go
// internal/event/infra.go
var InfraProviderSet = wire.NewSet(
    NewInfra,
    ProvideMonitorEventBus,
    // 已删除：ProvideSessionBus、ProvideMonitorBus、ProvideBuffer
)

// internal/biz/biz.go
var ProviderSet = wire.NewSet(
    NewActivityEventBus,        // ActivityEventBus provider
    NewEventBusSideConsumers,   // typed consumer 编排器
    NewToolCallConsumer,
    NewCallbackConsumer,
    NewUsageRollupConsumer,
    NewUserFeedbackConsumer,
    NewFlowLogPersistConsumer,
    // ... 其他 biz providers
)

// internal/server/server.go
var ProviderSet = wire.NewSet(
    NewHTTPServer,
    NewGRPCServer,
    NewWSServerFromInfra,
)
```

**已删除的 Wire 绑定**：`ProvideSessionBus`（SessionBus）、`ProvideMonitorBus`（旧 Envelope-based MonitorBus）、`ProvideBuffer`、`monitorBusFromInfra` helper。

---

## 十八、性能考量

### 18.1 持久化与推送

| 组件 | 策略 |
|------|------|
| ActivityEventSequencer | persist fire-and-forget（persistChan buffered）+ publish 同步 |
| persistChan 满 | 回退到同步 `persistWithRetry`（极端场景兜底） |
| retry 退避 | `select` 监听 `done` 通道，Close 期间立即放弃 |
| WS 写入阻塞 | 设置写超时 10s，超时后断连触发重连 |
| WS 读超时 | 60s 无 pong → 断开 |
| Runner 事件通道满 | trpc 框架内部处理（`EmitEventTimeoutErr`） |

### 18.2 连接管理

| 组件 | 限制 |
|------|------|
| 每 Session 最大连接数 | 5（`maxSessionConns`） |
| 全局监控最大连接数 | 3（`maxGlobalConns`） |
| ActivityEventBus 订阅者 buffer | 默认 128，最大 512 |
| 死信缓冲容量 | 512 条/Session（FIFO 淘汰） |
| 重试预算 | 5 次，总 3100ms（100/200/400/800/1600ms 指数退避） |
| ActivityEvent 大小 | 单条最大 1MB |
| WS 写超时 | 10s |
| WS 读超时（无 pong） | 60s |
| 协议层心跳间隔 | 30s |
| 应用层心跳间隔 | 25s |

### 18.3 延迟优化

| 优化点 | 方法 |
|--------|------|
| WS 消息发送 | 无需 HTTP 解析，帧头仅 2-14 字节 |
| JSON 序列化 | 预分配 buffer，避免频繁分配 |
| ActivityEventBus 路由 | 读锁 + 无锁 channel 发送 |
| persist 异步 | fire-and-forget，consume 不阻塞 |
| streaming 批量合并 | 16ms 窗口合并多个 streaming 事件 |

### 18.4 性能指标

| 指标 | 目标 |
|------|------|
| WS 推送延迟 P99 | < 50ms（不受 DB I/O 阻塞） |
| Activity 持久化延迟 P99 | < 100ms |
| Activity 持久化失败率 | < 0.1% |
| 前端 backfill 触发率 | < 5%（WS 重连或 reload 时） |
| 前端渲染 | 60fps（虚拟滚动 + 按需展开） |

---

## 十九、安全考量

| 风险 | 缓解措施 |
|------|---------|
| WS 跨域 | Origin 白名单校验（`OriginAllowed`：localhost + 环境变量 `KRATOS_HTTP_EXTRA_CORS_ORIGINS`） |
| 事件泄露 | ActivityEventBus 订阅时按 session_id 路由，WS 连接校验 token 归属 |
| WS 消息注入 | 上行消息类型白名单，payload 校验 |
| XSS via content | Activity content 做 HTML 转义后渲染（前端职责） |
| DDoS via 大量连接 | 限制每 Session 最大连接数（5）+ 全局监控连接数（3） |
| JWT 过期 | WS 连接期间定期校验 token 有效性（**未实现**） |
| 消息大小 | WS ReadLimit 1MB，超限断连 |
| WS 认证 | 三级回退认证（URL token → Authorization Header → Cookie） |

---

# 前端消息机制设计

> 本节定义 Aranea-Agents 前端的通信消息机制，聚焦传输协议、客户端实现和场景适配。前端已彻底删除 Envelope 路径，统一到 ActivityEvent + MonitorEvent。

---

## 二十、传输层选型

### 20.1 决策

```
WebSocket = 主传输通道（双向通信、多路复用、低延迟）
Chat HTTP = 非流式 / 后台入口（HTTP POST /v1/chat/messages）
ListActivities RPC = 断连恢复入口（API Backfill，替代旧 WS replay）
```

WebSocket 在实时交互维度优于 HTTP unary：

| 维度 | 说明 |
|------|------|
| **方向性** | 双向通信，cancel/enqueue/subscribe/enable_log 无需额外 HTTP |
| **连接数** | 1 个 WS 连接复用所有通道（chat+monitor+system） |
| **浏览器限制** | 无硬限制，多 Session 不受连接数约束 |
| **协议开销** | 握手后仅 2-14 字节帧头，高频事件场景带宽节省显著 |
| **多路复用** | Channel 机制天然支持 chat/monitor/system 统一连接 |

---

## 二十一、WebSocket 协议设计

### 21.1 连接建立

```
WS /v1/ws?session_id=sess-uuid&token=jwt_token

认证方式（三级回退）：
1. URL token 参数（优先）
2. Authorization: Bearer Header（浏览器 WebSocket API 不支持，仅非浏览器客户端）
3. access_token Cookie（浏览器场景的必要回退，浏览器 WebSocket API 无法设置自定义 Header）

前端连接时自动从 Cookie 读取 token：
  buildWsUrl({ sessionId, token: readAccessTokenCookie() })

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
    "subscribed_channels": ["chat", "system"]
  }
}
```

**已删除的连接参数**：`last_event_id`（WS replay 已删除，改用 ListActivities RPC）。

### 21.2 下行消息格式（Server → Client）

下行消息通过 `activity_event?` / `monitor_event?` / `type` 字段区分：

```json
// ActivityEvent 下行（chat + system 业务事件）
{
  "direction": "server_to_client",
  "channel": "chat",
  "activity_event": {
    "event": "created",
    "activity": {
      "id": "act-uuid",
      "kind": "thinking",
      "status": "running",
      "session_id": "sess-uuid",
      "timestamp": "2026-01-01T00:00:00.000Z",
      "seq": 12345,
      "content": "",
      "reasoning": ""
    },
    "domain": "chat"
  }
}

// MonitorEvent 下行（log/flow_log/mcp/alert）
{
  "direction": "server_to_client",
  "channel": "monitor",
  "monitor_event": {
    "id": "mon-uuid",
    "type": "flow_log",
    "timestamp": "2026-01-01T00:00:00.000Z",
    "session_id": "sess-uuid",
    "source": "step_id",
    "metadata": { "severity": "ok", "title": "..." }
  }
}
```

**系统消息**（非 ActivityEvent / MonitorEvent）：

```json
{
  "direction": "server_to_client",
  "channel": "system",
  "type": "connected | pong | server_shutdown",
  "payload": { ... }
}
```

**已删除的下行字段**：`envelope?`（旧 Envelope 通道，Blocker A 已删除 WS replay 路径）、`replay_start` / `replay_end`（WS replay 已删除）。

### 21.3 上行消息格式（Client → Server）

```json
{
  "direction": "client_to_server",
  "channel": "chat | control",
  "type": "user_message | cancel | enqueue_message | subscribe | unsubscribe | enable_log | ping",
  "request_id": "req-uuid",
  "payload": {}
}
```

### 21.4 上行消息类型

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
    "channel": "monitor",
    "filter_key": "agent_a/agent_b"
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
    "channel": "monitor"
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

### 21.5 Channel 定义

| Channel | 方向 | 说明 | 默认订阅 |
|---------|------|------|---------|
| `chat` | 双向 | ActivityEvent（chat + system 域） + 用户消息上行 | ✅ 连接即订阅 |
| `monitor` | 下行 | MonitorEvent（运维日志、系统事件） | ❌ 需 enable_log 开启 |
| `system` | 下行 | 系统通知（connected/pong/server_shutdown） | ✅ 连接即订阅 |

**已删除的 legacy channel**：`team` / `graph` / `knowledge`（统一到 chat，所有 Activity 事件走 chat channel）。

### 21.6 心跳与断连检测

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
  - 重连后调用 ListActivities(sessionId) API 拉取最新状态（API Backfill）

服务端关闭通知：
  - 服务端优雅关闭时发送 server_shutdown 系统消息
  - 客户端收到后不再自动重连
```

### 21.7 重连与 API Backfill

```
1. 客户端断连后发起重连（指数退避）
2. WS 握手成功，服务端发送 connected
3. 客户端调用 ListActivities(sessionId) RPC
4. 服务端返回该 session 当前所有持久化的 Activity（最新快照）
5. 客户端用 API 返回值补齐缺失状态（最终一致性兜底）
6. 切换回实时流，继续接收新的 ActivityEvent
```

**已删除的重连机制**：`last_event_id` 参数、`replay_start`/`replay_end` 消息、`RevisionTracker`、`requestSyncReplay`、`event.Buffer` 环形缓冲（WS replay 已删除）。

---

## 二十二、TypeScript 类型定义

### 22.1 ActivityEvent 类型

```typescript
// web/src/features/chat/activityTypes.ts

export type ActivityKind =
  | 'task' | 'thinking' | 'action' | 'reply' | 'plan'
  | 'confirm' | 'notice' | 'session' | 'team_stage' | 'graph_stage';

export type ActivityEventType =
  | 'created' | 'streaming' | 'updated'
  | 'completed' | 'failed' | 'cancelled' | 'child_created';

export type ActivityDomain = 'chat' | 'system';

export type ActivityStatus =
  | 'pending' | 'running' | 'tool_running' | 'tool_blocked'
  | 'completed' | 'failed' | 'partial_failure' | 'cancelled' | 'interrupted';

export interface Activity {
  id: string;
  kind: ActivityKind;
  status: ActivityStatus;
  session_id: string;
  turn_id?: string;
  parent_activity_id?: string;
  spirit_session_id?: string;
  team_id?: string;
  dag_node_id?: string;
  timestamp: string;
  duration_ms?: number;
  seq?: number;
  prompt_tokens?: number;
  completion_tokens?: number;
  content?: string;
  reasoning?: string;
  tool_name?: string;
  tool_category?: string;
  tool_call_id?: string;
  tool_arguments?: string;
  tool_result?: string;
  tool_duration_ms?: number;
  tool_error_code?: string;
  stage?: string;
  depends_on?: string[];
  agent_key?: string;
  agent_name?: string;
  collapsed?: boolean;
  label?: string;
  meta?: Record<string, unknown>;
}

export interface ActivityEvent {
  event: ActivityEventType;
  activity: Activity;
  domain?: ActivityDomain;
}
```

### 22.2 MonitorEvent 类型

```typescript
// web/src/features/monitor/monitorTypes.ts

export type MonitorEventType =
  | 'log' | 'flow_log'
  | 'mcp.session.reconnect' | 'mcp.health.alert' | 'alert.notify'
  | 'monitor.auto_healed' | 'monitor.self_check_completed';

export interface MonitorEvent {
  id: string;
  type: MonitorEventType;
  timestamp: string;
  level?: string;
  message?: string;
  session_id?: string;
  source?: string;
  metadata?: Record<string, unknown>;
}
```

### 22.3 WS 消息类型

```typescript
// web/src/realtime/ws-transport.ts（内联定义）

export interface WsDownstream {
  direction: 'server_to_client';
  channel: string;
  type?: string;                    // 系统消息：connected/pong/server_shutdown
  payload?: Record<string, unknown>;
  activity_event?: ActivityEvent;   // chat + system 业务事件
  monitor_event?: MonitorEvent;     // 监控事件
}

export interface WsUpstream {
  direction: 'client_to_server';
  channel: string;
  type: string;
  request_id?: string;
  payload?: Record<string, unknown>;
}
```

**已删除的前端类型**：`Envelope`、`EnvelopeType`（64 种）、`EnvelopeContent`/`EnvelopeToolCall`/`EnvelopeStateDelta`/`EnvelopeTransfer`/`EnvelopeError`/`EnvelopeUsage`/`EnvelopeActions`/`EnvelopeTrace`、`WsDownstream.envelope?` 字段。

---

## 二十三、传输层实现

### 23.1 createWsTransport

```typescript
// web/src/realtime/ws-transport.ts

export interface WsTransportOptions {
  sessionId: string;
  token?: string;
  onActivityEvent?: (ev: ActivityEvent) => void;
  onMonitorEvent?: (ev: MonitorEvent) => void;
  onConnected?: (info: { sessionId: string }) => void;
  onDisconnected?: () => void;
  onError?: (error: Event) => void;
}

export interface WsTransport {
  connect(): void;
  disconnect(): void;
  send(upstream: WsUpstream): void;
  connected(): boolean;
}

export function createWsTransport(opts: WsTransportOptions): WsTransport
```

**关键实现细节**：

1. **Pending 队列**：连接未建立时上行消息入队，连接建立后自动刷新
2. **server_shutdown 处理**：收到服务端关闭通知后主动断开，不再自动重连
3. **Cookie token 回退**：`readAccessTokenCookie()` 从 Cookie 读取 token 作为浏览器场景的认证回退
4. **应用层心跳**：25s 间隔发送 ping
5. **指数退避重连**：1s/2s/4s/8s/16s/30s cap
6. **重连后 API Backfill**：重连成功后调用 `listActivities(sessionId)` 拉取最新状态

**已删除的传输层特性**：`lastEventId` 追踪、`onEnvelope`/`onReplayState` 回调、`hasConnectedBefore` 状态、sync replay 块、`replay_start`/`replay_end` 处理。

### 23.2 URL 构建

```typescript
// web/src/config/runtime.ts

export function buildWsUrl(opts: {
  sessionId: string;
  token?: string;
}): string {
  const base = getWsBaseUrl();
  const params = new URLSearchParams();
  params.set('session_id', opts.sessionId);
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

## 二十四、事件分发与场景 Hooks

### 24.1 useEnvelopeStream（核心 composable）

```typescript
// web/src/realtime/useEnvelopeStream.ts

export function useEnvelopeStream(sessionId: string, opts?: UseEnvelopeStreamOptions): UseEnvelopeStreamReturn {
  // 通过 globalWsHub 共享 WS 连接（多 Session 复用）
  // 透传 onActivityEvent / onMonitorEvent 回调
  return { transport, send: transport.send };
}
```

**保留的活路径**：`createEnvelopeStream` / `useEnvelopeStream` / `onActivityEvent` / `onMonitorEvent` — 这些是 Activity-First 架构的前端入口，WS 传输层通过它们消费 `activity_event?`/`monitor_event?` 消息。

**已删除的前端路径**：`EnvelopeDispatcher` 类、`onType`/`onChannel` 函数、`matchFilterKey` 辅助、`RevisionTracker`、`requestSyncReplay`、`useChatStream`/`useTeamStream`/`useMonitorStream` 工厂函数、`subscribeSessionStream`、`LIVE_TYPES` 常量。

### 24.2 useActivityTimeline（按 session_id 隔离）

```typescript
// web/src/features/chat/composables/useActivityTimeline.ts

const activitiesBySession = shallowRef<Map<string, Map<string, Activity>>>(new Map());

function getSessionActivities(sessionId: string): Map<string, Activity> {
    let map = activitiesBySession.value.get(sessionId);
    if (!map) {
        map = new Map();
        activitiesBySession.value.set(sessionId, map);
    }
    return map;
}

// ensureActivitiesLoaded 缓存跳过语义（Phase E 实现）：
// - 缓存命中（含空 Map）时跳过 API 调用
// - 失败时不写缓存以便下次自动重试
// - WS replay 负责重连后补齐缺失事件
async function ensureActivitiesLoaded(sessionId: string): Promise<void>
```

### 24.3 场景特定 Hooks

| 文件 | 说明 |
|------|------|
| `web/src/realtime/globalWsHub.ts` | 全局 WS 连接管理（多 Session 复用、引用计数） |
| `web/src/features/chat/composables/useActivityTimeline.ts` | Activity Timeline 按 session 隔离 + `ensureActivitiesLoaded` 缓存跳过 |
| `web/src/features/chat/composables/useChatWorkspace.ts` | Chat 工作区 composable（`bindSessionView` 调用 `ensureActivitiesLoaded`） |
| `web/src/features/chat/inboundSyncRouting.ts` | 入站同步路由（已迁移到 ActivityEvent） |
| `web/src/features/chat/inboundSyncEnvelope.ts` | session_revision 入站同步处理（已迁移到 ActivityEvent） |
| `web/src/features/monitor/useLogStreamHub.ts` | flow_log / log 分流；FlowTracePanel 按 trace 过滤 |
| `web/src/features/monitor/useMonitorRealtimeEvents.ts` | Monitor 实时事件 |

---

## 二十五、场景交互流程

### 25.1 Chat 对话

```
1. 前端建立 WS 连接
   WS /v1/ws?session_id=sess-1&token=jwt

2. 服务端发送 connected
   ← {channel:"system", type:"connected", payload:{subscribed_channels:["chat","system"]}}

3. 前端发送用户消息
   → {direction:"client_to_server", channel:"chat", type:"user_message", payload:{content:"分析代码"}}

4. 服务端推送 ActivityEvent
   ← {channel:"chat", activity_event:{event:"created", activity:{kind:"task", status:"pending"}}}
   ← {channel:"chat", activity_event:{event:"created", activity:{kind:"thinking", status:"running"}}}
   ← {channel:"chat", activity_event:{event:"streaming", activity:{kind:"thinking", meta:{delta_field:"reasoning"}}}}
   ← {channel:"chat", activity_event:{event:"completed", activity:{kind:"thinking", status:"completed"}}}
   ← {channel:"chat", activity_event:{event:"created", activity:{kind:"action", tool_name:"read_file", tool_category:"file_read"}}}
   ← {channel:"chat", activity_event:{event:"completed", activity:{kind:"action", tool_result:"..."}}}
   ← {channel:"chat", activity_event:{event:"created", activity:{kind:"reply", status:"running"}}}
   ← {channel:"chat", activity_event:{event:"streaming", activity:{kind:"reply", meta:{delta_field:"content"}}}}
   ← {channel:"chat", activity_event:{event:"completed", activity:{kind:"reply", status:"completed"}}}
   ← {channel:"chat", activity_event:{event:"completed", activity:{kind:"task", status:"completed", prompt_tokens:100, completion_tokens:200}}}

5. 前端可随时取消
   → {direction:"client_to_server", channel:"chat", type:"cancel", request_id:"req-1"}
   ← {channel:"chat", activity_event:{event:"cancelled", activity:{kind:"task", status:"cancelled", meta:{cancel_reason:"user_interrupted"}}}}

6. 前端可动态开启 Monitor 日志
   → {direction:"client_to_server", channel:"control", type:"enable_log", payload:{enabled:true}}
   ← {channel:"monitor", monitor_event:{type:"log", metadata:{level:"ERROR",...}}}
```

### 25.2 Team 多 Agent 场景

```
1. 前端连接 WS，订阅 team_stage 事件（通过 chat channel）

2. 收到 Team 生命周期 ActivityEvent
   ← {channel:"chat", activity_event:{event:"created", activity:{kind:"team_stage", stage:"assembled", team_id:"team-1", meta:{members:[...]}}}}
   ← {channel:"chat", activity_event:{event:"updated", activity:{kind:"team_stage", stage:"executing", meta:{changed_fields:["stage"]}}}}

3. 收到成员子 Activity 事件
   ← {channel:"chat", activity_event:{event:"child_created", activity:{kind:"team_stage", meta:{child_activity_id:"act_member_1", member_agent_key:"agent_b"}}}}

4. 展开成员子 session，懒加载其 Activity 流
   前端调用 listActivities(memberSessionId) API
   ← 返回该成员 session 的所有 Activity（task/thinking/action/reply）

5. Team 运行完成
   ← {channel:"chat", activity_event:{event:"completed", activity:{kind:"team_stage", stage:"completed"}}}
```

### 25.3 Graph 工作流场景

```
1. 前端连接 WS，订阅 graph_stage 事件（通过 chat channel）

2. 收到 Graph 阶段 ActivityEvent
   ← {channel:"chat", activity_event:{event:"created", activity:{kind:"graph_stage", stage:"planned", meta:{nodes:[...]}}}}
   ← {channel:"chat", activity_event:{event:"updated", activity:{kind:"graph_stage", meta:{changed_fields:["current_node"], current_node:"step_1"}}}}
   ← {channel:"chat", activity_event:{event:"child_created", activity:{kind:"graph_stage", meta:{child_activity_id:"act_node_1"}}}}

3. 收到执行完成
   ← {channel:"chat", activity_event:{event:"completed", activity:{kind:"graph_stage", stage:"completed"}}}

4. HITL 中断恢复
   ← {channel:"chat", activity_event:{event:"completed", activity:{kind:"task", status:"interrupted", tag:"interrupt"}}}
   → 用户审批
   → {direction:"client_to_server", channel:"chat", type:"user_message", payload:{content:"批准"}}
```

### 25.4 Monitor 日志场景

**流程日志（`flow_log`）**：Chat Turn / Team / 系统域经 `TraceEmitter` 推送，**无需** `enable_log`；前端 `useLogStreamHub` → Flow 面板（中文 title + severity 配色）。

**进程日志（`log`）**：Gateway/Plugin 等文本日志，需 `enable_log` 或连接参数 `log_enabled=1`（全局监控在 `ProcessLogEnabled` 时可默认开启）。

```
1. 发 Chat → Monitor Logs「流程」Tab 自动出现 flow_log（无需 enable_log）
   ← {channel:"monitor", monitor_event:{type:"flow_log", metadata:{severity:"ok", title:"…", step_id:"chat.llm.invoke", trace_id:"…"}}}

2. 可选：开启进程日志
   → {direction:"client_to_server", channel:"control", type:"enable_log", payload:{enabled:true}}
   ← {channel:"monitor", monitor_event:{type:"log", metadata:{level:"ERROR", source:"tool"}, message:"…"}}

3. 关闭进程日志（flow_log 仍下发）
   → enable_log enabled:false
```

### 25.5 断连恢复场景（API Backfill）

```
1. WS 断连，前端自动重连（指数退避）
2. 重连成功，服务端发送 connected
3. 前端调用 ListActivities(sessionId) RPC
   → GET /v1/activities?session_id=sess-1
4. 服务端返回该 session 当前所有持久化的 Activity（最新快照）
5. 前端用 API 返回值补齐缺失状态（最终一致性兜底）
6. 切换回实时流，继续接收新的 ActivityEvent
```

### 25.6 服务端关闭场景

```
1. 服务端优雅关闭，广播 server_shutdown
   ← {channel:"system", type:"server_shutdown"}

2. 前端收到后不再自动重连
3. 用户可看到"服务已关闭"提示
```

---

## 二十六、前端文件结构

```
web/src/
  config/
    runtime.ts                     # buildWsUrl（含 token 参数）+ readAccessTokenCookie + buildHealthWsUrl
  realtime/                        # 实时传输核心层
    ws-transport.ts                 # createWsTransport（含应用层心跳、Cookie token 回退、pending 队列、server_shutdown 处理、内联 WsDownstream/WsUpstream 类型）
    useEnvelopeStream.ts            # useEnvelopeStream 核心 composable（通过 globalWsHub 共享连接，透传 onActivityEvent/onMonitorEvent）
    globalWsHub.ts                  # 全局 WS 连接管理（多 Session 复用、引用计数）
  features/
    chat/
      activityTypes.ts              # Activity / ActivityKind / ActivityEventType / ActivityDomain / ActivityStatus 类型定义
      useEnvelopeStream.ts          # createEnvelopeStream 工厂 + onActivityEvent/onMonitorEvent 透传
      inboundSyncRouting.ts         # 入站同步路由（基于 ActivityEvent）
      inboundSyncEnvelope.ts        # session_revision 入站同步处理（基于 ActivityEvent）
      composables/
        useActivityTimeline.ts      # Activity Timeline 按 session 隔离 + ensureActivitiesLoaded 缓存跳过
        useChatWorkspace.ts         # Chat 工作区 composable（bindSessionView 调用 ensureActivitiesLoaded）
        useChatEventInspector.ts    # Chat 事件检视
        useChatTraceAndArtifacts.ts # openSessionTrace / openSessionEvents
      components/
        ActivityStream.vue          # 统一渲染入口（按 activity.kind 动态分发到 Block 组件）
        UserMessageBubble.vue       # task → 用户消息
        ThinkingBlock.vue           # thinking → 推理过程
        ActionBlock.vue             # action → 工具调用（按 tool_category 细分）
        ReplyBlock.vue              # reply → Agent 回复
        PlanBlock.vue               # plan → 计划
        ConfirmBlock.vue            # confirm → 确认
        NoticeBlock.vue             # notice → 通知
        SessionStageBlock.vue       # session → Session 生命周期
        TeamStageBlock.vue          # team_stage → 团队阶段（含成员折叠）
        GraphStageBlock.vue         # graph_stage → Graph 阶段（DAG）
        SessionTreeSidebar.vue      # Session 树侧栏
        SessionTreeNode.vue         # 递归树节点（支持任意深度）
        tools/                      # 工具详情组件（按 tool_category 细分）
          ShellToolDetail.vue
          BrowserToolDetail.vue
          FileReadToolDetail.vue
          FileWriteToolDetail.vue
          FileSearchToolDetail.vue
          WebSearchToolDetail.vue
          McpToolDetail.vue
          CodeToolDetail.vue
          TodoToolDetail.vue
          GenericToolDetail.vue
    monitor/
      monitorTypes.ts               # MonitorEvent / MonitorEventType 类型定义
      useLogStreamHub.ts            # flow_log / log 分流；FlowTracePanel 按 trace 过滤
      useMonitorRealtimeEvents.ts   # Monitor 实时事件
      useMonitorLogStreamPanel.ts   # Monitor 日志流面板
      useMonitorTraceFlow.ts        # Monitor trace 流
    session/
      types.ts                      # SessionTreeNode 递归类型（支持任意深度）
      api.ts                        # listActivities / getSessionTree RPC
```

**已删除的前端文件**：`realtime/envelope.ts`、`realtime/dispatcher.ts`、`realtime/data_channel.ts`、`realtime/event_replay.ts`、`realtime/graphState.ts`、`features/chat/dispatcher.ts`（re-export barrel）、`features/chat/ws-transport.ts`（re-export barrel）、`features/chat/envelope.ts`（re-export barrel）、`features/chat/globalWsHub.ts`（re-export barrel）、`features/chat/streamHandlers.ts`、`features/chat/useEventFilter.ts`、`features/chat/eventFilter.ts`、`features/chat/envelopeRunStatus.ts`、`features/chat/sessionContextPatch.ts`、`features/chat/conversationEventDispatcher.ts`、`features/chat/channelWsCursor.ts`、`features/chat/channelInboundSession.ts`、`features/chat/channelInboundSessionRefresh.ts`、`features/chat/ConversationTurn.vue`、`features/chat/composables/useConversationTimeline.ts`、`components/chat/TeamPanel.vue`、`components/chat/OrchestrationTimeline.vue`、`components/spirit/TaskExecutionPanel.vue`、`components/spirit/MemberReadOnlyPanel.vue`。

---

## 二十七、前端性能考量

| 优化点 | 方法 |
|--------|------|
| 消息合并 | 连续 `streaming` 事件合并渲染（16ms 窗口 / requestAnimationFrame） |
| 虚拟滚动 | 长对话使用虚拟列表（DynamicScroller） |
| 背压感知 | WS buffer 未清空时降低渲染频率 |
| 重连去抖 | 指数退避避免频繁重连 |
| API Backfill 缓存 | `ensureActivitiesLoaded` 缓存命中时跳过 API 调用，失败时不写缓存以便重试 |
| Pending 队列 | 连接未建立时上行消息入队，连接后自动刷新 |
| 全局 WS 连接复用 | `globalWsHub` 多 Session 共享同一 WS 连接，引用计数管理生命周期 |
| 子 session 懒加载 | 点击成员展开时才加载子 session 的 Activity 流 |
| Session 隔离 | `useActivityTimeline` 按 session_id 隔离 Map，切换 session 无需 reset |

---

## 二十八、与现有模块的关系

### 28.1 替代关系

| 旧模块 | 新设计 |
|--------|--------|
| `Envelope` 结构体 + 60+ `EnvelopeType` | `Activity` 模型 + 10 `ActivityKind` + 7 `ActivityEventType` |
| `EventProjector`（trpc Event → Envelope） | `ActivityProjector`（trpc Event → Activity） |
| `event.Bus`（传输 Envelope） | `ActivityEventBus`（传输 ActivityEvent）+ `MonitorEventBus`（传输 MonitorEvent） |
| `event.Buffer`（WS replay 环形缓冲） | `ListActivities` RPC（API Backfill） |
| `EventStore`（event_store 表持久化） | `Activity` 表 + `ActivityEventSequencer`（并行异步持久化 + retry + dead-letter） |
| `EventWAL` / WBPF | 并行异步持久化 + retry + dead-letter + API Backfill |
| `EventBusConsumer`（编排器） | 4 个 typed consumer（ToolCall/Callback/UsageRollup/UserFeedback）+ FlowLogPersistConsumer |
| `messages` 表 | `Activity` 表（role 用 kind 表达） |
| `event_persist_handler.go` / `event_store.go` / `wal.go` | `activity_event_sequencer.go` |
| `buffer.go` / `reliability.go` / `bus.go` / `bus_adapter.go` / `framework_adapter.go` | 已删除 |
| `TeamRunEventBroker` + 独立端口 | `ActivityEventBus` |
| `MonitorLogBroker` + 独立端口 | `MonitorEventBus` + Flow Log v2 |
| 独立 SSE Server（`:8001`） | 删除，统一走 WS 传输 |
| `slog_bridge.go` | Flow Log v2 替代 |
| 前端 `Envelope` 类型 + `EnvelopeDispatcher` + `RevisionTracker` | 前端 `ActivityEvent` 类型 + `useActivityTimeline` + API Backfill |

### 28.2 不变部分

| 模块 | 说明 |
|------|------|
| `Runner.Run()` | 框架 API 不变，返回 `<-chan *event.Event` |
| `BuildTRPCLLMAgent` | Agent 构建不变 |
| `NewTRPCRunner` | Runner 创建不变 |
| `ChatService` HTTP 入口 | POST /v1/chat/messages 保留为非流式 / 后台入口 |
| Proto API | 不修改 proto，WS 不走 proto |
