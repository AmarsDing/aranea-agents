# Event 事件系统模块 — 实现设计文档

> 对应需求：`34-event-system.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> 进度状态（已实现/未实现能力清单）见 [34-event-system.development.md](./34-event-system.development.md) §2。
>
> **架构变更依据**：
> - ADR-02 Activity-First 事件持久化（已归档，设计内容已并入本文档）
> - ADR-03 统一总线架构（已归档，设计内容已并入本文档）
> - Chat 模块重构方案（已归档，设计内容已并入本文档）

> **P0-6（2026-08-14）**：生产 WS 下行已收敛为 `v2_event`（`biz.EventBus` → `WSV2Subscriber`）+ `monitor_event`（`MonitorEventBus` → monitor pump）。前端 Graph/Teams/Knowledge/Orchestration 走 `createV2EventStream` / `useV2EventStream`；Monitor 日志走 `createMonitorStream`；Chat 主路径走 `createChatStream`（内部仍是 v2）。`useEnvelopeStream` 的 `activity_event` 分支已删除，该文件仅为隔离兼容别名，生产 features 禁止 import。
>
> **架构迁移状态（2026-06-27）**：legacy 3 Bus（SessionBus + MonitorBus + ActivityBus，均传输 `Envelope`）+ `EventProjector` + `EventBuffer` + `EventStore` + `EventWAL`（WBPF）架构**已彻底删除**。当前实际架构为：
> - `biz.EventBus`（v2）：传输 typed Event（Task/Turn/Step/Team/Graph/`system.*`）→ WS `v2_event`
> - `MonitorEventBus`（`contract.MonitorBus`）：传输 `contract.MonitorEvent`，承载高频监控事件（log/flow_log/mcp/alert）
> - 活类型 `EnvelopeError`/`EnvelopeTokenUsage` + 5 个 `ErrorCode*` 常量提取到 `internal/event/contract/envelope_types.go`
> - legacy `contract/envelope.go`/`bus.go`/`reliability.go`/`buffer.go`/`wal.go`/`event_store.go` 已删除
>
> 下文部分章节仍保留 ADR-02/ADR-03 历史 ActivityEvent 描述；**以本节 P0-6 口径与交叉参考 §1.12 为准**。

---

## 一、模块概述

基于 **Activity-First（AF）架构** 的事件系统，采用 **单一 Activity 模型 + 双 Bus（ActivityEventBus + MonitorEventBus）** 架构：

- `ActivityEventBus` 传输 `biz.ActivityEvent`，承载 chat 业务事件（`Domain=chat`，持久化到 `activities` 表）+ system 通知事件（`Domain=system`，仅推送 WS 不持久化）
- `MonitorEventBus` 传输 `contract.MonitorEvent`，承载高频监控事件（log / flow_log / mcp.* / alert.*），不持久化
- WebSocket 通过 2 pump（activityEventPump + monitorEventPump）多路复用下行
- 持久化采用**并行异步**模式（fire-and-forget），失败通过重试 + dead-letter + API Backfill 三重补偿
- WS 重连恢复采用 **API Backfill**（`ListActivities` RPC），不再使用服务端 Buffer replay

---

## 二、架构总览

```
trpc-agent-go Runner
       │
       │ *trpcevent.Event
       ▼
 ActivityProjector ──── 投影为 Activity ────┐
       │                                     │
       │ EmitActivityEvent / EmitSystemEvent │
       ▼                                     ▼
  ┌─────────────────────────────────────────────────────┐
  │           ActivityEventSequencer (per-activity FIFO) │
  │                                                       │
  │  processTask(activityID, task):                       │
  │   ├── 任务 1：持久化 fire-and-forget                  │
  │   │     persistChan (buffered) → worker goroutine     │
  │   │       ├── retry 预算（5 次，3100ms，done 可中断）  │
  │   │       └── retry 耗尽 → deadLetter 环形缓冲（512） │
  │   │                                                   │
  │   └── 任务 2：推送同步（per-activity FIFO）            │
  │         eventBus.Publish(ActivityEvent)               │
  │                                                       │
  │  monitor 事件独立路径：                                │
  │   FlowTracker → MonitorEventBus.Publish(MonitorEvent) │
  └──────┬──────────────────────────────┬─────────────────┘
         │                              │
         ▼                              ▼
   前端 WsTransport                前端 WsTransport
   + WSV2Subscriber               + monitorEventPump
   + useV2EventStream             + createMonitorStream
   + createChatStream             + globalWsHub
   + SessionPanelV2
```

**双 Bus 隔离**（`internal/event/infra.go`）：

| Bus | 传输类型 | 承载事件 | 持久化 |
|-----|---------|---------|--------|
| `biz.EventBus`（v2） | typed Event（Task/Turn/Step/Team/Graph/`system.*`） | 聊天与跨域业务事件 | v2 实体表 + 关键事件 outbox |
| `MonitorEventBus` | `contract.MonitorEvent` | 高频监控事件（log / flow_log / mcp.* / alert.*） | 不持久化（FlowLog 持久化由独立 consumer 处理） |

v1 `ActivityEventBus` 生产路径已退役。**已删除的 legacy Bus**：SessionBus（传输 Envelope）、旧 Envelope-based MonitorBus、ActivityBus（v2 双总线期）、`event.Bus` 接口、`contract.Bus` 接口、`RouteChannel` 路由机制。

---

## 三、核心数据模型

### 3.1 contract 子包（纯接口与值对象）

`internal/event/contract/`

biz 层应只 import `contract` 子包，禁止 import 父 `event` 包（含实现）。父 `event` 包通过 type alias 向后兼容旧调用点。

| 文件 | 职责 |
|------|------|
| `contract/monitor_event.go` | MonitorEvent 结构 + MonitorEventType 枚举 + MonitorBus 接口 + MonitorSubscribeOptions |
| `contract/envelope_types.go` | 活类型提取：`EnvelopeError` + `EnvelopeTokenUsage` + 5 个 `ErrorCode*` 常量（ErrorCodeToolTimeout / ErrorCodeToolError / ErrorCodeConfirmationRequired / ErrorCodeConfirmationDenied / ErrorCodeConfirmationTimeout） |
| `contract/dedup.go` | activityID 去重工具 |

**已删除的 legacy 契约文件**：
- ~~`contract/envelope.go`~~（ADR-03 Blocker G：活类型已提取，死代码删除）
- ~~`contract/bus.go`~~（ADR-03 Blocker G：`contract.Bus` 接口删除）
- ~~`contract/reliability.go`~~（ADR-02 D1：WBPF 模式废弃）
- ~~`contract/envelope_test.go`~~ / ~~`contract/envelope_contract_test.go`~~（随主文件删除）

### 3.2 ActivityEvent（chat + system 业务事件统一载体）

`internal/biz/activity_event.go`

`ActivityEvent` 是 chat+system 业务事件的唯一传输载体，封装在 `ActivityEventBus` 上流转。

| 字段 | 类型 | 说明 |
|------|------|------|
| Event | `ActivityEventType` | 7 种业务语义事件（见 §3.3） |
| Activity | `Activity` | Activity snapshot（见 §3.4） |
| Domain | `ActivityDomain` | `chat`（持久化） / `system`（仅推送 WS） |

### 3.3 ActivityKind / ActivityEventType / MonitorEventType 枚举

#### ActivityKind（10 种，无 error kind）

`internal/biz/activity.go`

```go
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

#### ActivityEventType（7 种业务语义事件）

`internal/biz/activity_event.go`

```go
type ActivityEventType string

const (
    ActivityEventCreated      ActivityEventType = "created"        // 新增 Block
    ActivityEventStreaming    ActivityEventType = "streaming"      // 流式追加（content/reasoning/tool_arguments）
    ActivityEventUpdated      ActivityEventType = "updated"        // 状态变更（非流式）
    ActivityEventCompleted    ActivityEventType = "completed"      // 正常完成
    ActivityEventFailed       ActivityEventType = "failed"         // 失败（meta.error_*）
    ActivityEventCancelled    ActivityEventType = "cancelled"      // 取消
    ActivityEventChildCreated ActivityEventType = "child_created"  // 父 Activity 通知：子 Activity 创建
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

#### MonitorEventType（高频监控事件）

`internal/event/contract/monitor_event.go`

| 类型 | 常量 | 说明 |
|------|------|------|
| log | `MonitorEventTypeLog` | 进程 Gateway 文本日志 |
| flow_log | `MonitorEventTypeFlowLog` | Flow Log v2 流程步骤（schema: flow_log/v1） |
| mcp.session.reconnect | `MonitorEventTypeMCPSessionReconnect` | MCP 会话重连通知 |
| mcp.health.alert | `MonitorEventTypeMCPHealthAlert` | MCP 健康告警 |
| alert.notify | `MonitorEventTypeAlertNotify` | 监控告警通知 |
| monitor.auto_healed | `MonitorEventTypeMonitorAutoHealed` | 自愈通知 |
| monitor.self_check_completed | `MonitorEventTypeMonitorSelfCheckCompleted` | 自检完成 |
| cron.dead_letter | `MonitorEventTypeCronDeadLetter` | Cron 死信通知 |
| skill.reload | `MonitorEventTypeSkillReload` | Skill 注册表重载 |
| skill.filesystem.updated | `MonitorEventTypeSkillFilesystemUpdated` | Skill 文件系统更新 |
| skill.filesystem.recovered | `MonitorEventTypeSkillFilesystemRecovered` | Skill 文件系统恢复 |
| skill.filesystem.imported | `MonitorEventTypeSkillFilesystemImported` | Skill 文件系统导入 |

**已删除的 legacy EnvelopeType**：60+ EnvelopeType 常量（text_delta / text_done / tool_call / tool_result / state_delta / transfer / runner_completion / run_status / error / llm_retry / execution_progress / activity_* / spirit_* / butler.* / skill.* / orchestration.* / organization.* / member_* / team_* / intent_pass / team_summary / knowledge_ingest / graph_* / checkpoint / token_usage / user_feedback / borrow.* 等）全部删除，由 ActivityKind + ActivityEventType + MonitorEventType 替代。

### 3.4 Activity 子结构

`internal/biz/activity.go`

Activity 是 `activities` 表的 ORM 映射，承载所有 chat/system 业务语义单元。

| 字段类别 | 字段 | 类型 | 说明 |
|---------|------|------|------|
| 主键 | `id` | string(64) | Activity 唯一 ID |
| 分类 | `kind` | string(32) | 10 种 ActivityKind |
| 分类 | `status` | string(32) | 9 种 ActivityStatus（见 §七状态机） |
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

### 3.5 MonitorEvent 子结构

`internal/event/contract/monitor_event.go`

| 字段 | 类型 | 说明 |
|------|------|------|
| ID | string | 事件唯一 ID（UUID） |
| Type | `MonitorEventType` | 事件类型枚举 |
| Timestamp | time.Time | 事件时间 |
| Level | string | 日志级别（可选，仅 type=log） |
| Message | string | 日志消息（可选） |
| SessionID | string | 所属会话 ID（可选） |
| Source | string | 事件来源（可选） |
| Metadata | map[string]any | 附加元数据（可选） |

**可靠性分级**（AS-EVT-01）：`Informational` — best-effort 投递，buffer 满可丢弃。丢失仅降低可观测性可见度，从不破坏状态。

### 3.6 活类型提取（envelope_types.go）

`internal/event/contract/envelope_types.go`

Envelope 结构体已删除，但以下活类型保留（被 ActivityToolCall / ActivityToolResult / UsageRollupConsumer 等活跃使用）：

| 类型 / 常量 | 说明 |
|------------|------|
| `EnvelopeError` | 错误信息结构（Type / Code / Message / Hint / PendingID） |
| `EnvelopeTokenUsage` | 详细 Token 用量记录（30+ 字段，用于 usage_rollup_consumer 汇总到 `model_token_usage_hourly` 表） |
| `ErrorCodeToolTimeout` | 工具超时错误码 |
| `ErrorCodeToolError` | 工具错误码（默认） |
| `ErrorCodeConfirmationRequired` | 需要确认错误码 |
| `ErrorCodeConfirmationDenied` | 确认拒绝错误码 |
| `ErrorCodeConfirmationTimeout` | 确认超时错误码 |

### 3.7 关键方法

| 方法 | 签名 | 说明 |
|------|------|------|
| `NewActivity` | `(kind, sessionID, turnID string) Activity` | 构造新 Activity，自动生成 ID / Timestamp |
| `IsActivityTerminal` | `(status ActivityStatus) bool` | 判定是否终态（Completed/Failed/Cancelled/Interrupted/PartialFailure） |
| `ActivityFromContext` | `(ctx) *Activity` | 从 ctx 获取当前 Activity（用于嵌套场景） |

---

## 四、双 Bus 接口设计

### 4.1 ActivityEventBus 接口

`internal/event/activityevent/bus.go`

```go
type ActivityEventBus interface {
    Publish(ctx context.Context, event biz.ActivityEvent) error
    Subscribe(handler func(biz.ActivityEvent)) Subscription
}
```

**实现要点**：
- `Publish` 同步执行（保留 per-activity FIFO 推送顺序）
- `Subscribe` 返回 `Subscription` 接口（含 `Unsubscribe()` 方法）
- 内部维护订阅者列表，publish 时遍历调用

### 4.2 MonitorBus 接口

`internal/event/contract/monitor_event.go`

```go
type MonitorBus interface {
    Publish(ctx context.Context, event MonitorEvent)
    Subscribe(opts MonitorSubscribeOptions) (<-chan MonitorEvent, func())
    DropCount() uint64
}
```

**MonitorSubscribeOptions**：

| 字段 | 类型 | 说明 |
|------|------|------|
| Filter | `func(MonitorEvent) bool` | 事件过滤函数 |
| BufferSize | int | Channel 容量（默认 128，上限 1024） |
| DropPolicy | `DropPolicy` | 背压策略（DropOldest / DropNewest） |
| GlobalMode | bool | 全局模式（不限 SessionID） |

### 4.3 背压策略

| 策略 | 行为 |
|------|------|
| DropOldest | 缓冲满时淘汰最旧事件 |
| DropNewest | 缓冲满时丢弃最新事件 |

**已删除的策略**：~~`BlockUpTo`~~（ADR-02 D1：Activity 事件不再使用 BlockUpTo，改用并行异步 + 重试 + dead-letter）

**丢弃可观测**：MonitorBus 的 `DropCount()` 返回累计丢弃数，通过 Prometheus `MonitorBusDropCount` 暴露。

### 4.4 Infra 容器

`internal/event/infra.go`

```go
type Infra struct {
    MonitorEventBus contract.MonitorBus // typed monitor event bus
    lg              loggateway.Logger
}
```

**已删除的字段**：~~`SessionBus`~~（ADR-03 Blocker F Stage 3）、~~`MonitorBus`~~（legacy Envelope-based，ADR-03 Blocker F Stage 2）。

---

## 五、事件可靠性分级（基于 ActivityDomain + 持久化策略）

> **架构变更**：legacy `event/contract/reliability.go` 已删除。WBPF（Write-Before-Publish-Fanout）模式废弃，由并行异步持久化 + 三重补偿替代。

| 级别 | 事件类型 | 可靠性保证 | 持久化 |
|------|---------|-----------|--------|
| **Important** | `Domain=chat` Activity 事件（task/thinking/action/reply/plan/confirm/notice/session/team_stage/graph_stage 的 created/streaming/updated/completed/failed/cancelled/child_created） | 并行异步持久化（fire-and-forget）+ 三重补偿（重试 + dead-letter + API Backfill）；订阅者需幂等 | ✅ `activities` 表 |
| **Informational** | `Domain=system` Activity 事件 | best-effort 推送（不持久化，丢失仅影响通知） | ❌ 不持久化 |
| **Informational** | Monitor 事件（log/flow_log/mcp.*/alert.*） | best-effort 推送（不持久化，丢失仅影响可观测性） | ❌ 不持久化（FlowLog 持久化由独立 consumer 异步处理） |

### 5.1 Activity-First 事件持久化（ADR-02）

> **详见**：ADR-02（Activity-First 事件持久化策略）

Activity 事件流（`biz.ActivityEventBus`）采用**并行异步**持久化，替代 legacy WBPF：

| 维度 | Legacy WBPF（已弃用） | AF 并行异步（现行） |
|------|----------------------|-------------------|
| 持久化时机 | 先写 WAL，成功后才 publish | fire-and-forget（独立 worker goroutine） |
| 推送时机 | 持久化成功后 | 同步（保留 per-activity FIFO） |
| 失败处理 | 不 publish（强一致） | 重试 5 次（100/200/400/800/1600ms）+ dead-letter 环形缓冲（cap=512）+ API backfill |
| 阻塞 | DB I/O 阻塞推送（~50-200ms） | 不阻塞（~5ms 推送延迟） |
| 适用范围 | Legacy envelope Critical 事件（已退役） | Activity 事件流（chat 渲染主路径） |

### 5.2 三重补偿机制

1. **重试预算**（`internal/agent/activity_event_sequencer.go`）：`persistMaxRetries=5`，`persistInitialBackoffMs=100`，`persistBackoffFactor=2`，指数退避（100/200/400/800/1600ms），总预算 3100ms。退避可通过 `s.done` channel 中断：Close() 期间 DB 不可用时立即放弃重试并转入 dead-letter，避免 shutdown 被退避睡眠阻塞。
2. **Dead-Letter 环形缓冲**：重试耗尽后，失败的 Activity 进入 `deadLetter` 环形缓冲（容量 512，FIFO 淘汰）。同一 activityID 的多次失败按最新快照去重，避免缓冲累积同一活动的过期中间态。通过 `ListDeadLetterActivities(sessionID)` 暴露给 WS 重连补偿路径。
3. **API Backfill**：前端在 WS 重连或显式 reload 时，通过 `listActivities(sessionId)` API 拉取最新持久化状态，作为最终一致兜底。

### 5.3 OnError 语义（ADR-02 D3）

错误不再产生独立的 `ActivityKindError` Activity（已删除）。统一模型：

- **存在 root task**：将 root task Activity 转换为 `status=failed`，错误信息存入 `Meta.error_message` / `Meta.error_type` / `Meta.error_code`
- **无 root task**：创建一个最小化的 failed task Activity 兜底
- **OnTurnEnd 终态保护**：若 root 已是终态（Completed/Failed/Cancelled/Interrupted/PartialFailure，由 `biz.IsActivityTerminal` 判定），OnTurnEnd 不覆盖状态，仅附加 token usage

---

## 六、事件投影

### 6.1 ActivityProjector（替代 EventProjector）

`internal/agent/activity_projector.go`

将 trpc-agent-go `*trpcevent.Event` 投影为 `biz.Activity`，保留完整元数据。

> **EventProjector 已删除**（ADR-02/03）：legacy `internal/agent/event_projector.go` + `internal/event/framework_adapter.go` + `internal/event/event_projector.go` 全部删除，由 `ActivityProjector` 替代。

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
| TurnPromptTokens / TurnCompletionTokens | 当前轮累计 Token |
| MemberAgentKeys | Team member_* 信封作者白名单 |
| Source | 事件来源 |
| TaskContent | 根任务用户输入文本（用于 Activity 标题） |
| **SpiritSessionID** | Spirit Session ID（跨 session 聚合） |
| **ParentSessionID** | 父 Session ID |
| **RootSessionID** | Root Session ID |

**投影规则**：
- trpc Event.Branch / FilterKey / Tag / Extensions / Actions 直接映射到 Activity meta
- ProjectMeta 中的字段作为默认值（Event 字段优先）
- LongRunningToolIDs 映射为 Activity.tool_duration_ms（长时运行由前端根据 duration 判断）
- LLM Response.Choices 拆分为 `kind=reply` + `event=streaming` / `event=completed`
- RunnerCompletion 映射为 `kind=task` + `event=completed` + token usage
- Error 映射为 `kind=task` + `event=failed` + Meta.error_*

### 6.2 EmitSystemEvent（ADR-03 D4）

```go
func (p *ActivityProjector) EmitSystemEvent(ctx context.Context, kind biz.ActivityKind, content string, meta map[string]any) {
    ev := biz.ActivityEvent{
        Event:    biz.ActivityEventCreated,
        Activity: biz.Activity{...},
        Domain:   biz.ActivityDomainSystem,
    }
    p.sequencer.publish(ctx, ev.Activity.ID, publishTask{event: ev, persist: false})
}
```

`publishTask.persist` 字段控制是否持久化：`EmitSystemEvent` 传 `persist=false`（Domain=system 不持久化），常规 Activity 投影传 `persist=true`（Domain=chat 持久化）。

### 6.3 ToolCategorizer

`internal/agent/activity_projector.go`

将工具名映射到 10 种 `tool_category`：

| 分类 | 工具示例 |
|------|---------|
| shell | bash / shell / terminal |
| browser | browser / web_browser / playwright |
| file_read | read_file / cat / head |
| file_write | write_file / edit_file / sed |
| file_search | grep / find / search_files |
| web_search | web_search / google / bing |
| mcp | mcp_* / mcp.* |
| code | code_executor / python / node |
| todo | todo / task_list |
| other | 其他 |

### 6.4 EventBridge（Graph）

`internal/graph/trpc/event_bridge.go`

将 Graph 执行事件桥接到 ActivityEventBus，映射 trpc-agent-go Graph ObjectType 到 `ActivityKindGraphStage` + `ActivityEventType`。

### 6.5 Flow Log v2（替代 SlogBridge）

| 文件 | 职责 |
|------|------|
| `internal/event/flow_log.go` | FlowLogEntry 数据结构、`FlowLogSchemaVersion = "flow_log/v1"`、stepTitleRegistry |
| `internal/event/flow_tracker.go` | FlowTracker：LogStart/LogDone/LogError + emit `MonitorEventTypeFlowLog` 到 MonitorEventBus |
| `internal/event/trace_emitter.go` | TraceEmitter = FlowTracker + ObserveFrameworkEvent（不再持有 Bus 参数，emit 对 nil infra 安全） |
| `internal/event/flow_context.go` | `WithTraceEmitter` / `TraceEmitterFromContext` / `NewTraceEmitterForRun`（`WithFlowLogger` / `FlowLoggerFromContext` / `NewFlowLogger` 别名已删除） |

- Monitor 业务日志主类型为 **`flow_log`**（`schema_version: flow_log/v1`），非全局 `slog` 桥接
- **`slog_bridge.go` 已删除**；`LOG_BRIDGE_ENABLED` 已废弃
- 进程 Gateway 文本日志仍为 `MonitorEventTypeLog`（如 `PluginSafeLogger`），与 `flow_log` 前端分流
- 详见 [52-flow-logger.design.md](./52-flow-logger.design.md)

---

## 七、Activity 生命周期状态机

`internal/biz/activity_state_machine.go`

### 7.1 ActivityStatus 枚举（9 种）

| 状态 | 说明 |
|------|------|
| `pending` | 待开始 |
| `running` | 进行中 |
| `streaming` | 流式输出中 |
| `awaiting` | 等待中（如等待用户确认） |
| `completed` | 正常完成（终态） |
| `failed` | 失败（终态） |
| `cancelled` | 取消（终态） |
| `interrupted` | 中断（终态） |
| `partial_failure` | 部分失败（终态） |

### 7.2 状态转换表

| From → To | 触发事件 | 守卫条件 |
|-----------|---------|---------|
| pending → running | created | — |
| pending → streaming | created | kind ∈ {thinking, reply, action} |
| running → streaming | streaming | — |
| streaming → streaming | streaming | delta_field 追加 |
| streaming → running | updated | 非文本变更 |
| running → awaiting | updated | 等待确认 |
| awaiting → running | updated | 确认恢复 |
| running → completed | completed | 终态 |
| streaming → completed | completed | 终态 |
| running → failed | failed | 终态，OnError 写入 Meta.error_* |
| streaming → failed | failed | 终态 |
| running → cancelled | cancelled | 终态 |
| running → interrupted | interrupted | 终态 |
| running → partial_failure | completed | 终态（部分子任务失败） |

### 7.3 OnTurnEnd 终态保护

```go
// internal/agent/activity_projector.go
func (p *ActivityProjector) OnTurnEnd(ctx context.Context, ...) {
    root := p.rootTask(ctx)
    if root != nil && biz.IsActivityTerminal(root.Status) {
        // 终态保护：不覆盖状态，仅附加 token usage
        p.updateMeta(ctx, root.ID, map[string]any{
            "prompt_tokens":     promptTokens,
            "completion_tokens": completionTokens,
        })
        return
    }
    // 非终态：转为 completed
    p.transition(ctx, root.ID, biz.ActivityEventCompleted)
}
```

---

## 八、Session State Delta

> 本节内容仍为现行设计，未受 ADR-02/ADR-03 影响。

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
- 会话表拆分后，`session_runtime` 表亦持有 `state_json`

---

## 九、事件回放与 API Backfill

> **架构变更**：legacy `event.Buffer`（环形缓冲 + WS replay）已删除（ADR-03 Blocker A）。WS 重连恢复改为 API Backfill。

### 9.1 API Backfill（替代 Buffer Replay）

WS 重连时，前端主动调用 `ListActivities` RPC 拉取持久化 Activity 列表，恢复一致视图。

| 维度 | Legacy Buffer Replay（已删除） | API Backfill（现行） |
|------|-------------------------------|---------------------|
| 数据源 | `event.Buffer` 环形缓冲（内存） | `activities` 表（持久化） |
| 回放窗口 | Buffer 容量（200） | 全部持久化记录 |
| 触发方式 | 服务端自动 replay | 前端主动 RPC 调用 |
| 服务端状态 | 需维护 Buffer + TTL | 无状态 |
| 一致性 | 内存与 DB 可能不一致 | 直接读 DB，强一致 |

### 9.2 Dead-Letter 查询

```go
// 通过 RPC 暴露
ListDeadLetterActivities(sessionID string) ([]Activity, error)
```

前端可通过此 RPC 查询某个 session 的 dead-letter Activity，用于诊断持久化失败的事件。

### 9.3 session_revision 增量同步

`internal/event/session_revision.go`

- `SessionRevisionBumper` 接口 + `BumpSessionRevision` 函数（仅 bump 半边）
- Turn 完成时递增 `sessions.session_revision`
- ~~`PublishSessionRevisionEnvelope`~~ 已删除（ADR-03 Blocker D：死发布者）
- 前端通过 `ListActivities` / `GetSession` RPCs 读取 revision，不依赖 envelope

---

## 十、WebSocket 传输

### 10.1 WSServer（2 pump）

`internal/server/ws.go`（+ `ws_conn.go` / `ws_conn_manager.go` / `ws_codec.go` / `ws_message_handler.go` / `ws_io_pump.go` / `ws_event.go` / `ws_priority.go`）

统一 WebSocket 网关，端点 `/v1/ws`。

**2 pump 架构**（ADR-03 D5 之后 + P0-6）：

| Pump | 订阅 Bus | 下行 |
|------|---------|------|
| WSV2Subscriber | `biz.EventBus`（v2） | 独立帧 `{ type: "v2_event", kind, payload }` |
| monitorEventPump | `MonitorEventBus` | `wsDownstream.monitor_event` |

**已删除的 pump**：~~`envelopeEventPump`~~（legacy SessionBus）、~~`activityEventPump`~~（v1 `activity_event?` 生产路径已退役）

**连接参数**：
| 参数 | 说明 |
|------|------|
| session_id | 会话 ID（必填，`*` 为全局监控模式） |
| token | 认证 Token |
| filter_key | 事件过滤键 |
| log_enabled | 是否接收日志事件 |
| probe / health | 探活连接（不订阅事件） |

**下行消息格式**：
```json
{
  "type": "v2_event",
  "kind": "task.created",
  "session_id": "...",
  "payload": { }
}
```

或

```json
{
  "direction": "server_to_client",
  "channel": "monitor",
  "monitor_event": { }
}
```

**已删除的下行字段**：~~`envelope?`~~（ADR-03 Blocker A：WS replay 删除后，envelope 字段已移除）

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

**已删除的上行消息**：~~`sync_request`~~（T3.4 revision-based 重放，依赖已删除的 EventStore，随 ADR-03 Blocker A 一起删除）

**日志门控**：`MonitorEventTypeLog` 受 `log_enabled` / `enable_log` 控制；`flow_log` 始终投递（不经 log 门控）。

**连接限制**（`conf/runtime_helpers.go` 默认值）：
- 每个 Session 最多 5 个连接（`WS_MAX_SESSION_CONNS`）
- 全局监控模式最多 3 个连接（`WS_MAX_GLOBAL_MONITOR_CONNS`）

---

## 十一、前端架构

### 11.1 类型定义

| 文件 | 职责 |
|------|------|
| `web/src/realtime/activityEvent.ts` | ActivityEvent TypeScript 类型定义（与后端 `biz.ActivityEvent` JSON 结构一一对应） |
| `web/src/realtime/monitorEvent.ts` | MonitorEvent TypeScript 类型定义（与后端 `contract.MonitorEvent` JSON 结构一一对应） |

**已删除的前端类型文件**：~~`web/src/realtime/envelope.ts`~~（ADR-03 D6）、~~`web/src/realtime/dispatcher.ts`~~（EnvelopeDispatcher）、~~`web/src/realtime/data_channel.ts`~~（DataChannel）、~~`web/src/realtime/event_replay.ts`~~（RevisionTracker + sync_request）

### 11.2 WsTransport

`web/src/realtime/ws-transport.ts`

WebSocket 传输层，职责：
- 连接管理（自动重连，指数退避，最大 30s）
- 心跳（25s 间隔）
- 消息发送（离线排队）
- 服务器关机通知
- **WS 重连后触发 API Backfill**：调用 `ListActivities` RPC 拉取断连期间的 Activity（替代 legacy `lastEventId` replay + `sync_request`）

### 11.3 useV2EventStream / createMonitorStream

`web/src/realtime/useV2EventStream.ts` · `web/src/realtime/useMonitorStream.ts` · `web/src/realtime/createWsSessionStream.ts`

Vue / 工厂封装 WsTransport + globalWsHub：
- `createV2EventStream` / `useV2EventStream` — Graph / Teams / Knowledge / Orchestration / Chat 订阅 `v2_event`
- `createMonitorStream` — Monitor / Computer-Use 订阅 `monitor_event`
- `createChatStream` / `createTeamStream`（`features/chat/useEnvelopeStream.ts`）— Chat 发送路径，内部走 `createV2EventStream`

`web/src/realtime/useEnvelopeStream.ts` 仅为隔离兼容别名（无 `activity_event` 分支），**生产 features 禁止 import**。

### 11.4 useActivityTimeline（API Backfill + 缓存）

`web/src/features/chat/useActivityTimeline.ts`

- `ensureActivitiesLoaded(sessionId, turnId)` — 缓存命中跳过 API，未命中调用 `ListActivities` RPC
- 失败时降级到空列表，不阻塞渲染
- 与 `useChatWorkspace.ts` 集成

### 11.5 ActivityStream（统一渲染入口）

`web/src/features/chat/ActivityStream.vue`

按 `activity.kind` 分发到对应组件：

| ActivityKind | 渲染组件 |
|--------------|---------|
| task | UserMessageBubble |
| thinking | ThinkingBlock |
| action | ToolCallBlock |
| reply | ReplyBlock |
| plan | PlanBlock |
| confirm | ConfirmBlock |
| notice | NoticeBlock |
| session | SessionStageBlock |
| team_stage | TeamStageBlock |
| graph_stage | GraphStageBlock |

### 11.6 Monitor 实时事件

`web/src/components/monitor/RealtimeEvents.vue`

Monitor `/monitor` Events Tab 生产组件：合并 WS 运行时 MonitorEvent 与持久化 Monitor Events，支持分类过滤、Runs 关联跳转、暂停/清除。

### 11.7 前端本地类型解耦（ADR-03 D6）

为切断 Envelope import 依赖，前端定义本地类型：

| 本地类型 | 用途 |
|---------|------|
| `ExecutionProgressMetadata` | 编排进度元数据 |
| `ActivityUsage` | Activity token 用量 |
| `InspectorEvent` | 事件检查器 |

这些类型不依赖 `Envelope`，Phase 5 删 Envelope 时仅需删除类型定义，无需修改业务逻辑。

---

## 十二、事件持久化

### 12.1 Activity 表（单一真相源）

`internal/data/ent/schema/activity.go` + `internal/data/activity_repo.go`

`activities` 表是事件系统的唯一真相源，承载所有 chat/system 业务语义单元。

**已删除的 legacy 表**：~~`messages`~~、~~`event_store`~~、~~`event_wal`~~（全部 DROP，由 `activities` 表替代）

### 12.2 并行异步持久化

`internal/agent/activity_event_sequencer.go`

**processTask 设计**：

```go
func (s *activityEventSequencer) processTask(activityID string, task publishTask) {
    // 任务 1：持久化 fire-and-forget
    if task.persist && s.activityRepo != nil {
        select {
        case s.persistChan <- task:
        default:
            s.persistWithRetry(context.Background(), activityID, task)
        }
    }
    // 任务 2：WS 推送同步（per-activity FIFO）
    if s.eventBus != nil {
        event := biz.ActivityEvent{Event: task.event, Activity: task.activity, Domain: s.domainFor(task)}
        s.eventBus.Publish(context.Background(), event)
    }
}
```

**persist worker goroutine**：从 `persistChan` 消费，FIFO 处理，保证 start→done 顺序。

**Close 三阶段关闭**：
1. consumers（停止订阅）
2. persistChan（关闭 channel）
3. worker（等待 worker goroutine 退出）

### 12.3 重试与 Dead-Letter

详见 §5.2 三重补偿机制。

### 12.4 LLM 上下文构建（替代 Message 查询）

`internal/biz/llm_context_builder.go`

```go
// 从 Activity 表构建 LLM 上下文（替代原 Message 查询）
//
// 角色映射规则（LLM API 只接受 user/assistant/tool/system）：
//   - task    → user
//   - reply   → assistant（含团队成员回复，通过 agent_key 标识来源，不改变 role）
//   - action  → tool
//   - notice  → system
func BuildLLMContext(ctx context.Context, repo ActivityReader, sessionID, turnID string) ([]LLMMessage, error)
```

### 12.5 数据迁移

`internal/data/activity_backfill_migrate.go`

- messages → activities 数据迁移（一次性）
- 通过 DDL Migration Registry 注册，幂等执行

---

## 十三、Chat 会话事件检视

> **产品决策**：不增加第四列固定侧边栏（左 Entity / 中 Message / 右 Session 已占满）。采用 **Dialog 双 Tab**，与现有 `SessionTimelineDialog` 入口合并。

### 13.1 与 Monitor 分工

| 维度 | Monitor `RealtimeEvents` | Chat Inspector |
|------|--------------------------|----------------|
| 范围 | 全局 / 多会话 | **当前 session** |
| 数据源 | WS MonitorEvent + Monitor Events API | WS ActivityEvent + `ListActivities` API |
| 入口 | `/monitor?tab=events` | Chat 会话菜单 / MessagePanel 工具栏 |
| 侧重 | Runs 关联、运维分类 | Activity 树、状态变更、Agent 路径 |

### 13.2 布局

```
ChatPage
├── 三栏布局（不变）
└── SessionTimelineDialog（扩展）
      Tab「历史 Trace」— 现有 HTTP SessionTimeline（从 Activity 表查询）
      Tab「实时 ActivityEvent」— SessionEventInspectorPanel
            ├─ EventFilterBar（按 kind/event/agent 过滤）
            ├─ ActivityTree（左 30%，基于 parent_activity_id 嵌套）
            └─ 事件列表（StatusIndicator / ToolCallBlock / ThinkingBlock）
```

**入口**：
1. `ChatSessionSidebar` 会话菜单「历史追踪」→ Dialog 默认 Trace Tab
2. `ChatMessagePanel` 头部「事件」按钮 → Dialog 默认 ActivityEvent Tab

### 13.3 前端分层

| 层级 | 路径 | 职责 |
|------|------|------|
| API | `features/event/api.ts` | `listSessionActivities` → `ListActivities` RPC |
| 纯函数 | `features/chat/eventFilter.ts` | filterActivities / buildActivityTree / EventFilterState |
| Composable | `features/chat/composables/useEventFilter.ts` | 过滤状态 + computed（filteredActivities / activityTree） |
| Composable | `features/chat/composables/useChatEventInspector.ts` | WS + 历史合并、暂停（MAX_EVENTS=2000） |
| Composable | `features/chat/composables/useChatTraceAndArtifacts.ts` | `openSessionTrace(id, tab?)` / `openSessionEvents(id)` |
| 展示 | `components/chat/EventFilterBar.vue` | 过滤控件 |
| 展示 | `components/chat/ActivityTree.vue` + `ActivityTreeNode.vue` | Activity 嵌套树 |
| 展示 | `components/chat/StatusIndicator.vue` | Activity 状态指示器 |
| 展示 | `components/chat/SessionEventInspectorPanel.vue` | Tab 内容容器 |
| 容器 | `components/chat/SessionTimelineDialog.vue` | Tab 切换 + Trace Tab（initialTab prop） |

Activity 树由 `parent_activity_id` 在线推导，不新增后端 API。

### 13.4 EventFilterState

| 字段 | 说明 |
|------|------|
| kindFilter | `all` 或 ActivityKind |
| eventFilter | `all` 或 ActivityEventType |
| agentFilter | agent_key 精确匹配 |
| keyword | 搜索 kind/content/tool_name/agent_name |

---

## 十四、涉及文件清单

### 14.1 后端 Live 文件

| 文件 | 说明 |
|------|------|
| `internal/biz/activity.go` | Activity struct + 10 种 ActivityKind 枚举 |
| `internal/biz/activity_event.go` | 7 种 ActivityEventType + ActivityDomain + ActivityEvent struct + IsActivityTerminal |
| `internal/biz/activity_state_machine.go` | 9 种 ActivityStatus + 状态转换表 + 转换校验 |
| `internal/biz/llm_context_builder.go` | 从 Activity 表构建 LLM 上下文（替代 Message 查询） |
| `internal/biz/event_bus_side_consumers.go` | EventBusSideConsumers 编排器（启动/停止 4 个 typed consumer + monitor 文件 appender） |
| `internal/biz/event_bus_callback_consumer.go` | ActivityEventBus 订阅 → WebhookDispatcher |
| `internal/biz/event_bus_flow_log_consumer.go` | MonitorEventBus 订阅 flow_log → 持久化 |
| `internal/biz/event_bus_user_feedback_consumer.go` | ActivityEventBus 订阅 user_feedback |
| `internal/biz/event_bus_usage_rollup_consumer.go` | ActivityEventBus 订阅 token_usage → 用量汇总 |
| `internal/biz/event_bus_async.go` | 异步订阅辅助 |
| `internal/biz/session/state.go` + `state_usecase.go` | SessionUsecase.ApplyStateDelta / GetSessionState / SaveSessionState（Facade） |
| `internal/data/session_state_repo.go` | Session State 持久化（json_set / json_remove） |
| `internal/data/ent/schema/session.go` + `session_runtime.go` | state_json 字段 |
| `internal/data/activity_repo.go` | Activity Repo（替代 message_repo.go + event_store_repo.go） |
| `internal/data/activity_backfill_migrate.go` | messages → activities 数据迁移 |
| `internal/event/activityevent/bus.go` | ActivityEventBus 实现（Publish + Subscribe + per-activity FIFO） |
| `internal/event/contract/monitor_event.go` | MonitorEvent + MonitorEventType + MonitorBus 接口 + MonitorSubscribeOptions |
| `internal/event/contract/envelope_types.go` | 活类型提取：EnvelopeError + EnvelopeTokenUsage + 5 个 ErrorCode 常量 |
| `internal/event/contract/dedup.go` | activityID 去重工具 |
| `internal/event/infra.go` | Infra 容器（仅 MonitorEventBus + lg） |
| `internal/event/flow_log.go` | FlowLogEntry + stepTitleRegistry |
| `internal/event/flow_tracker.go` | FlowTracker（LogStart/LogDone/LogError + emit MonitorEventTypeFlowLog） |
| `internal/event/trace_emitter.go` | TraceEmitter（FlowTracker + ObserveFrameworkEvent，无 Bus 参数） |
| `internal/event/flow_context.go` | FlowContext + WithTraceEmitter（含 Deprecated 别名） |
| `internal/event/session_revision.go` | SessionRevisionBumper + BumpSessionRevision（仅 bump 半边） |
| `internal/agent/activity_projector.go` | ActivityProjector（trpc Event → Activity 投影 + OnError + OnTurnEnd 终态保护 + EmitSystemEvent + ToolCategorizer） |
| `internal/agent/activity_meta.go` + `activity_meta_resolver.go` | ProjectMeta（含 SpiritSessionID/ParentSessionID/RootSessionID） |
| `internal/agent/activity_event_sequencer.go` | ActivityEventSequencer（processTask 并行异步 + persist worker + retry + dead-letter） |
| `internal/agent/turn_helpers.go` | ConsumeEventStream（无 eventBus 参数） |
| `internal/agent/turn_stream_helpers.go` | ConsumeWithFirstByteGuard（无 bus 参数） |
| `internal/graph/trpc/event_bridge.go` | Graph 事件桥接到 ActivityEventBus |
| `internal/server/ws.go` + `ws_*.go` | WSServer（2 pump：activityEventPump + monitorEventPump） |
| `internal/service/chat.go` | SendChatMessage unary 入口 |
| `internal/service/channel_ingress.go` | Channel 入站（无 eventBus 字段） |
| `internal/service/channel_turn_preview.go` | Turn 预览（无 bus 字段，订阅逻辑已删除） |
| `internal/cronrunner/runner.go` | Cron 运行器（无 EventBus 字段） |
| `cmd/admin/wire.go` | InfraProviderSet + biz ProviderSet（无 ProvideSessionBus / ProvideMonitorBus） |

### 14.2 后端已删除文件

| 文件 | 删除原因 |
|------|---------|
| ~~`internal/event/contract/envelope.go`~~ | ADR-03 Blocker G：活类型已提取，死代码删除 |
| ~~`internal/event/contract/envelope_test.go`~~ / ~~`envelope_contract_test.go`~~ | 随主文件删除 |
| ~~`internal/event/contract/bus.go`~~ | ADR-03 Blocker G：`contract.Bus` 接口删除 |
| ~~`internal/event/contract/reliability.go`~~ / ~~`reliability_test.go`~~ | ADR-02 D1：WBPF 模式废弃 |
| ~~`internal/event/buffer.go`~~ | ADR-03 Blocker A/G：WS replay Buffer 死代码 |
| ~~`internal/event/bus.go`~~ / ~~`bus_adapter.go`~~ / ~~`framework_adapter.go`~~ | ADR-03 Blocker G：Bus 接口删除 |
| ~~`internal/event/event_projector.go`~~ / ~~`activity_publish.go`~~ / ~~`activity_persist.go`~~ | ADR-02：由 activity_event_sequencer.go 替代 |
| ~~`internal/event/event_persist_handler.go`~~ / ~~`event_store.go`~~ / ~~`wal.go`~~ | ADR-02：EventStore/WAL 废弃 |
| ~~`internal/event/step_id.go`~~ / ~~`session_revision_publish.go`~~ / ~~`deco_session_sync_test.go`~~ | ADR-03 Blocker D：死发布者删除 |
| ~~`internal/biz/event_bus_consumer.go`~~ | ADR-03：拆分为 typed consumer |
| ~~`internal/biz/event_bus_buffer_handler.go`~~ / ~~`runner_handler.go`~~ / ~~`state_handler.go`~~ / ~~`message_store_consumer.go`~~ | ADR-03 Blocker A：四 handler 删除 |
| ~~`internal/biz/event_bus_tool_call_consumer.go`~~ | 已合并到 ActivityProjector（ToolCategorizer） |
| ~~`internal/biz/domain_event_adapter.go`~~ | ADR-03 Blocker C：DomainEvent bridge 删除 |
| ~~`internal/biz/event_store.go`~~ | ADR-02：EventStoreUsecase 废弃 |
| ~~`internal/data/message_repo.go`~~ | ADR-02：messages 表 DROP |
| ~~`internal/data/event_store_repo.go`~~ | ADR-02：event_store 表 DROP |
| ~~`internal/server/ws_sync_request.go`~~ | ADR-03 Blocker A：sync_request 上行处理删除 |
| ~~`internal/service/run_status_publish.go`~~ | ADR-03：run_status Envelope 发布删除 |
| ~~`internal/cronrunner/jobs/event_store_cleanup.go`~~ | ADR-02：EventStore TTL 清理废弃 |

### 14.3 前端 Live 文件

| 文件 | 说明 |
|------|------|
| `web/src/realtime/ws-transport.ts` | WsTransport（心跳/重连/pending 队列/API Backfill 触发） |
| `web/src/realtime/activityEvent.ts` | ActivityEvent TypeScript 类型定义 |
| `web/src/realtime/monitorEvent.ts` | MonitorEvent TypeScript 类型定义 |
| `web/src/realtime/useV2EventStream.ts` | typed `v2_event` 订阅（Graph/Teams/Knowledge/Orchestration/Chat） |
| `web/src/realtime/createWsSessionStream.ts` | WS 会话连接工厂（无 activity_event） |
| `web/src/realtime/useEnvelopeStream.ts` | 隔离兼容别名（生产 features 禁止 import） |
| `web/src/realtime/useMonitorStream.ts` | Monitor 事件流订阅 |
| `web/src/realtime/globalWsHub.ts` | 全局 WS 连接管理 |
| `web/src/realtime/command_channel.ts` | 上行命令（cancel/enqueue/subscribe/enable_log） |
| `web/src/realtime/graphState.ts` | Graph 节点状态聚合 |
| `web/src/realtime/timeout_model.ts` | 超时/重连策略 |
| `web/src/features/chat/useEnvelopeStream.ts` | `createChatStream` / `createTeamStream`（内部 `createV2EventStream`） |
| `web/src/features/chat/useActivityTimeline.ts` | API Backfill + 缓存（ensureActivitiesLoaded） |
| `web/src/features/chat/useTaskDeadLetters.ts` | ListDeadLetterActivities RPC 调用 |
| `web/src/features/chat/useChatBackgroundJobs.ts` | Chat 后台任务（API Backfill 触发） |
| `web/src/features/chat/ActivityStream.vue` | Activity-First 统一渲染入口 |
| `web/src/features/event/api.ts` | 回放 API 门面（listSessionActivities） |
| `web/src/features/event/types.ts` | 回放 API 类型 |
| `web/src/components/monitor/RealtimeEvents.vue` | Monitor Events Tab（生产） |

### 14.4 前端已删除文件

| 文件 | 删除原因 |
|------|---------|
| ~~`web/src/realtime/envelope.ts`~~ | ADR-03 D6：前端 Envelope 类型删除 |
| ~~`web/src/realtime/dispatcher.ts`~~ | ADR-03 D6：EnvelopeDispatcher 删除 |
| ~~`web/src/realtime/data_channel.ts`~~ | ADR-03 D6：DataChannel 删除 |
| ~~`web/src/realtime/event_replay.ts`~~ | ADR-03 Blocker A：RevisionTracker + sync_request 删除 |
| ~~`web/src/features/chat/dispatcher.ts`~~ | ADR-03 D6：re-export barrel 删除 |
| ~~`web/src/features/chat/inboundSyncEnvelope.ts`~~ | ADR-03 Blocker D：envelope 入站同步删除 |
| ~~`web/src/features/chat/envelopeRunStatus.ts`~~ / ~~`envelopeToolCall.ts`~~ | ADR-03 D6：Envelope 辅助类型删除 |
| ~~`web/src/features/spirit/TeamPanel.vue`~~ / ~~`OrchestrationTimeline.vue`~~ / ~~`TaskExecutionPanel.vue`~~ / ~~`MemberReadOnlyPanel.vue`~~ | ADR-02 遗留：legacy spirit 面板删除 |
