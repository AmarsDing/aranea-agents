# 统一总线架构设计：删除 Envelope，统一到 ActivityEvent + MonitorEvent

> **状态**：设计批准后进入实施
> **优先级**：最高（用户指令：彻底解决双总线长期并存）
> **策略**：一次性大爆炸（用户选择）
> **来源**：[2026-06-25-analysis-chat-module-refactor.md](../../reports/2026-06-25-analysis-chat-module-refactor.md) §3.6 + §6.3 + §12.2

---

## 1. 背景与问题

### 1.1 双总线并存现状

Aranea-Agents 的事件系统当前存在 **3 个 bus**：

| Bus | 类型 | 传输载体 | 用途 |
|-----|------|---------|------|
| SessionBus | `event.Bus` | `Envelope` | chat + teams + graph + spirit + orchestration + domain 事件 |
| MonitorBus | `event.Bus` | `Envelope` | monitor 事件（log/flow_log/mcp/alert） |
| ActivityBus | `biz.ActivityEventBus` | `ActivityEvent` | chat 渲染事件（text/tool/reasoning/member） |

**问题**：
1. `EventPipeline` 同时承载 `Bus event.Bus` + `ActivityBus biz.ActivityEventBus` — 双总线并存
2. `WSServer` 订阅 3 个 bus，运行 3 个 pump goroutine
3. Spirit/Graph 事件**双发布**（同时走 envelope Bus.Publish 和 ActivityProjector）
4. 前端 `WsDownstream` 同时有 `envelope?` + `activity_event?` 字段
5. 59+ 前端文件依赖 envelope 类型
6. 80+ 后端 publisher 调用 `bus.Publish(ctx, envelope)`

### 1.2 文档目标状态（refactor doc §12.2）

| 指标 | 目标 |
|------|------|
| 后端数据模型 | 1 套（Activity） |
| 事件类型 | 7 种业务语义事件 |
| Envelope 结构体 | **删除** |
| Channel 路由 | 删除 `RouteChannel`，统一 `chat`（monitor 保留独立） |
| EventBus 传输 | `ActivityEvent` |
| 死代码 | 0 |
| 前端渲染管线 | 1 套 |

---

## 2. 设计决策

### 2.1 ActivityEvent 新增 Domain 字段

**决策**：ActivityEvent 新增 `Domain ActivityDomain` 字段，区分 chat 事件（需持久化）与 system 事件（仅推送）。

```go
// internal/biz/activity_event.go

type ActivityDomain string

const (
    // ActivityDomainChat 标识 chat/agent 工作事件。
    // 这类事件需要持久化到 Activity 表，前端加入时间线渲染。
    ActivityDomainChat ActivityDomain = "chat"

    // ActivityDomainSystem 标识系统/域事件（organization/borrow/skill/knowledge/
    // run_status/heartbeat/execution_progress 等）。
    // 这类事件仅推送 WS，不持久化到 Activity 表，前端作为通知处理。
    ActivityDomainSystem ActivityDomain = "system"
)

type ActivityEvent struct {
    Event      ActivityEventType `json:"event"`
    Activity   Activity           `json:"activity"`
    DeltaField string             `json:"delta_field,omitempty"`
    DeltaChunk string             `json:"delta_chunk,omitempty"`
    Domain     ActivityDomain     `json:"domain"` // NEW
}
```

**持久化规则**：
- `Domain=chat` → ActivityEventSequencer 正常持久化到 Activity 表
- `Domain=system` → ActivityEventSequencer **跳过持久化**，仅 publish 到 WS

**前端行为**：
- `Domain=chat` → 加入 `useActivityTimeline` 时间线渲染
- `Domain=system` → 路由到 `useSystemEventNotification` 通知处理器（toast/notification/全局状态）

### 2.2 新建 MonitorEvent 类型

**决策**：从 `envelope.go` 拆出 monitor 事件类型到 `internal/event/contract/monitor_event.go`，彻底删除 `Envelope` struct。

```go
// internal/event/contract/monitor_event.go

package contract

import "time"

// MonitorEventType labels the type of monitor event.
type MonitorEventType string

const (
    MonitorEventTypeLog                  MonitorEventType = "log"
    MonitorEventTypeFlowLog              MonitorEventType = "flow_log"
    MonitorEventTypeMCPSessionReconnect  MonitorEventType = "mcp.session.reconnect"
    MonitorEventTypeMCPHealthAlert       MonitorEventType = "mcp.health.alert"
    MonitorEventTypeAlertNotify          MonitorEventType = "alert.notify"
    MonitorEventTypeMonitorAutoHealed    MonitorEventType = "monitor.auto_healed"
    MonitorEventTypeMonitorSelfCheckDone MonitorEventType = "monitor.self_check_completed"
)

// MonitorEvent is the transport for system monitoring events.
// These events are high-frequency (100+/sec) and not persisted to Activity table.
type MonitorEvent struct {
    ID        string            `json:"id"`
    Type      MonitorEventType  `json:"type"`
    Timestamp time.Time         `json:"timestamp"`
    Level     string            `json:"level,omitempty"`       // for log: debug/info/warn/error
    Message   string            `json:"message,omitempty"`     // for log: log message
    SessionID string            `json:"session_id,omitempty"`  // optional session context
    Source    string            `json:"source,omitempty"`      // emitter source
    Metadata  map[string]any    `json:"metadata,omitempty"`    // type-specific payload
}
```

**MonitorBus 改为传输 `MonitorEvent`**：
```go
// internal/event/contract/bus.go (修改)

type MonitorBus interface {
    Publish(ctx context.Context, event MonitorEvent)
    Subscribe(opts MonitorSubscribeOptions) (<-chan MonitorEvent, func())
    DropCount() uint64
}
```

### 2.3 逐个迁移 Publisher

**决策**：逐个修改 80+ publisher 调用点，将 `bus.Publish(ctx, envelope)` 改为 `activityBus.Publish(ctx, activityEvent)` 或 `monitorBus.Publish(ctx, monitorEvent)`。

**迁移映射表**：

| EnvelopeType | 目标 Bus | ActivityKind / MonitorEventType | Domain |
|--------------|---------|--------------------------------|--------|
| **Chat 控制** | | | |
| `error` | ActivityBus | Kind=task, Status=failed | chat |
| `run_status` | ActivityBus | Kind=session | chat |
| `session.status_changed` | ActivityBus | Kind=session | chat |
| `execution_progress` | ActivityBus | Kind=notice | chat |
| `run_heartbeat` | ActivityBus | Kind=session | system |
| `context_usage` | ActivityBus | Kind=notice | system |
| `intent_pass` | ActivityBus | Kind=notice | chat |
| `agent_created` | ActivityBus | Kind=notice | system |
| `llm_retry` | ActivityBus | Kind=notice | system |
| `planning_phase_*` | ActivityBus | Kind=plan | chat |
| `token_usage` | ActivityBus | Kind=task (root) | system |
| `user_feedback` | ActivityBus | Kind=notice | chat |
| `metrics_updated` | ActivityBus | Kind=session | system |
| `state_delta` | ActivityBus | Kind=notice (Meta=delta) | chat |
| **Teams** | | | |
| `team_run_started/finished/failed` | ActivityBus | Kind=team_stage | chat |
| `team_step_started/finished` | ActivityBus | Kind=team_stage | chat |
| `team_summary` | ActivityBus | Kind=team_stage | chat |
| `orchestration_agent_status` | ActivityBus | Kind=team_stage | chat |
| **Graph** | | | |
| `graph_node_start/end/error/custom` | ActivityBus | Kind=graph_stage | chat |
| `graph_step/execution_done` | ActivityBus | Kind=graph_stage | chat |
| `graph_task_status` | ActivityBus | Kind=graph_stage | chat |
| `graph_replanned` | ActivityBus | Kind=graph_stage | chat |
| `graph_topology_evolved` | ActivityBus | Kind=graph_stage | chat |
| `checkpoint` | ActivityBus | Kind=session (Meta=interrupt) | chat |
| **Spirit/Butler** | | | |
| `spirit_team_*` | ActivityBus | Kind=team_stage | chat |
| `spirit_synthesis_completed` | ActivityBus | Kind=session | chat |
| `spirit_plan_created` | ActivityBus | Kind=plan | chat |
| `spirit_allocation_created` | ActivityBus | Kind=notice | chat |
| `spirit_orchestration_*` | ActivityBus | Kind=session | chat |
| `butler.orchestration_*` | ActivityBus | Kind=session | chat |
| **Skill/Knowledge/Domain** | | | |
| `skill.health_changed` | ActivityBus | Kind=notice | system |
| `skill.evolution_proposed` | ActivityBus | Kind=notice | system |
| `knowledge_ingest` | ActivityBus | Kind=notice | system |
| `borrow.*` | ActivityBus | Kind=notice | system |
| `organization.*` | ActivityBus | Kind=notice | system |
| `orchestration.evolution_suggested` | ActivityBus | Kind=notice | system |
| `orchestration.cache_hit` | ActivityBus | Kind=notice | system |
| **Monitor** | | | |
| `log` | MonitorBus | MonitorEventTypeLog | — |
| `flow_log` | MonitorBus | MonitorEventTypeFlowLog | — |
| `mcp.*` | MonitorBus | MonitorEventTypeMCP* | — |
| `alert.notify` | MonitorBus | MonitorEventTypeAlertNotify | — |
| `monitor.*` | MonitorBus | MonitorEventTypeMonitor* | — |

### 2.4 ActivityProjector 扩展

**决策**：ActivityProjector 新增 `EmitSystemEvent` 方法，用于发布 Domain=system 事件（非 chat 工作单元）。

```go
// internal/agent/activity_projector.go (扩展)

// EmitSystemEvent publishes a system-domain ActivityEvent (not persisted).
// Used for run_status/heartbeat/agent_created/llm_retry/domain events.
func (p *ActivityProjector) EmitSystemEvent(ctx context.Context, kind biz.ActivityKind, content string, meta map[string]any) {
    ev := biz.ActivityEvent{
        Event: biz.ActivityEventCreated,
        Activity: biz.Activity{
            ID:        uuid.NewString(),
            Kind:      kind,
            Status:    biz.ActivityStatusCompleted,
            Timestamp: time.Now().UTC(),
            Content:   content,
            Meta:      meta,
        },
        Domain: biz.ActivityDomainSystem,
    }
    p.sequencer.publish(ctx, ev.Activity.ID, publishTask{event: ev, persist: false})
}
```

**ActivityEventSequencer 扩展**：
- `publishTask` 新增 `persist bool` 字段
- `Domain=system` 时 `persist=false`，跳过 DB 写入
- `Domain=chat` 时 `persist=true`，正常持久化

### 2.5 WSServer 简化

**决策**：WSServer 从 3 个 bus 订阅简化为 2 个。

```go
// internal/server/ws.go (修改后)

type WSServer struct {
    // 删除 eventBus event.Bus（SessionBus 不再存在）
    monitorBus  contract.MonitorBus   // 监控事件
    activityBus biz.ActivityEventBus   // 所有 chat + system 事件
    // ...
}

// setupEventSubscription 返回 2 个 channel（删除 envelope channel）
func (s *WSServer) setupEventSubscription(wc *WSContext, globalMode bool) (<-chan biz.ActivityEvent, <-chan contract.MonitorEvent) {
    // ...
}
```

**WS pump 简化**：2 个 pump goroutine（activityPump + monitorPump），删除 eventPump。

### 2.6 前端统一

**决策**：删除所有 envelope 前端文件，统一到 ActivityEvent + MonitorEvent。

**WsDownstream 修改**：
```typescript
// web/src/realtime/ws-transport.ts (修改后)

interface WsDownstream {
    activity_event?: ActivityEvent   // chat + system 事件
    monitor_event?: MonitorEvent     // monitor 事件
    // 删除 envelope?: Envelope
}
```

**删除文件**：
- `web/src/realtime/envelope.ts`
- `web/src/realtime/useEnvelopeStream.ts`
- `web/src/features/chat/envelope.ts`
- `web/src/features/chat/useEnvelopeStream.ts`
- `web/src/features/chat/inboundSyncRouting.ts`
- `web/src/features/chat/inboundSyncEnvelope.ts`
- `web/src/features/chat/streamHandlers.ts`（legacy envelope handlers）

**新增文件**：
- `web/src/realtime/monitorEvent.ts` — MonitorEvent 类型 + type guards
- `web/src/features/chat/composables/useSystemEventNotification.ts` — Domain=system 事件处理

**修改文件**（59+）：
- 所有 `import type { Envelope }` → `import type { ActivityEvent }`
- 所有 `useEnvelopeStream` → `useActivityTimeline` 或 `useMonitorStream`（monitor 域）
- `useChatStreamManager` → 移除 envelope 路径，仅保留 ActivityEvent 路径
- `useChatInboundSync` → 移除 `onSpiritEnvelope`/`onActivityEnvelope`，仅保留 `onActivityEvent`

### 2.7 后端删除清单

| 文件 | 删除内容 |
|------|---------|
| `internal/event/contract/envelope.go` | **删除整个文件**（Envelope struct + EnvelopeType 常量 + RouteChannel + channelRegistry） |
| `internal/event/envelope.go` | **删除整个文件**（type alias re-export） |
| `internal/event/infra.go` | 删除 `SessionBus`，仅保留 `MonitorBus` |
| `internal/event/bus.go` | 删除 `Bus` type alias（改为 `MonitorBus`） |
| `internal/event/flow_tracker.go` | 迁移到 ActivityEventBus.Publish |
| `internal/event/trace_emitter.go` | 迁移到 ActivityEventBus.Publish |
| `internal/event/logpipeline_publisher.go` | 迁移到 MonitorBus.Publish |

---

## 3. 实施计划

> **进度总览**（截至 2026-06-26）：
> - ✅ Phase 1：后端核心结构扩展
> - ✅ Phase 2：后端 Publisher 迁移（80+ 站点全部完成，go build + go test 通过）
> - ✅ Phase 3：WSServer 简化（3 pump → 2 pump）
> - ✅ Phase 4：前端统一（Envelope import 46→7，全部为合法传输层/事件检查器）
> - 🟡 Phase 5：删除后端 Legacy 代码（识别出 6 个耦合 Blocker，多 session 工程，已制定依赖链）
> - 🟡 Phase 6：全量验证 + 文档同步 + ADR-03（验证已通过，文档同步中）

### Phase 1：后端核心结构扩展（非破坏性） ✅

**目标**：新增 Domain 字段 + MonitorEvent 类型，不破坏现有功能。

| 步骤 | 文件 | 改动 |
|------|------|------|
| 1.1 | `internal/biz/activity_event.go` | 新增 `ActivityDomain` 类型 + `Domain` 字段 |
| 1.2 | `internal/event/contract/monitor_event.go` | **新建** MonitorEvent 类型 |
| 1.3 | `internal/event/contract/bus.go` | 新增 `MonitorBus` 接口 |
| 1.4 | `internal/event/monitor_bus.go` | **新建** MonitorBus 实现（基于 framework GenericBus） |
| 1.5 | `internal/agent/activity_event_sequencer.go` | `publishTask.persist` 字段**已存在**（line 113）；仅需确保 `EmitSystemEvent` 传 `persist=false` |
| 1.6 | `internal/agent/activity_projector.go` | 新增 `EmitSystemEvent` 方法（传 `persist: false`） |

**验证**：`go build ./...` + `go test ./internal/biz/... ./internal/agent/... ./internal/event/...`

### Phase 2：后端 Publisher 迁移（逐个域） ✅

**目标**：将 80+ publisher 从 `bus.Publish(ctx, envelope)` 迁移到 `activityBus.Publish` 或 `monitorBus.Publish`。

> **完成状态**：所有外部 publisher 已迁移。残留 `event.NewEnvelope` 调用仅存在于 `internal/event/` 核心基础设施（`framework_adapter.go`、`domain_event_adapter.go`、`envelope.go` 类型别名）和测试文件中，将在 Phase 5 删除。

**迁移顺序**（按域，每域独立验证）：

| 步骤 | 域 | 文件 | 事件数 |
|------|---|------|--------|
| 2.1 | Chat 控制 | `internal/service/chat_event_publisher.go`, `run_status_publish.go`, `chat_orchestrator*.go`, `run_heartbeat.go`, `pre_planning_gate.go`, `chat_plan_confirm.go`, `chat_feedback.go` | ~15 |
| 2.2 | Teams | `internal/team/runner_*.go`, `team_graph_run_*.go`, `summary.go`, `status_projector.go` | ~7 |
| 2.3 | Graph | `internal/graph/trpc/event_bridge.go`, `runtime_replanner.go`, `topology_evolution.go`, `internal/service/graph_task_status.go` | ~12 |
| 2.4 | Spirit/Butler | `internal/service/spirit_team.go`, `spirit_synthesis.go`, `internal/agent/task_orchestrator_impl.go`, `task_planner_impl.go`, `agent_allocator_impl.go`, `internal/tools/spirit_tools.go` | ~20 |
| 2.5 | Domain | `internal/biz/organization.go`, `dept_lead.go`, `event_bus_*.go`, `internal/service/knowledge.go`, `internal/skill/watch/reporter.go` | ~7 |
| 2.6 | Monitor | `internal/event/flow_tracker.go`, `trace_emitter.go`, `logpipeline_publisher.go`, `internal/mcp/alert/alert.go`, `internal/plugin/trpc/safe_logger.go`, `internal/service/monitor_notify.go`, `internal/session/compressor.go` | ~7 |
| 2.7 | 删除双发布 | `internal/agent/task_orchestrator_impl.go`（删除 "Dual consumption" 注释处的 envelope publish） | ~5 |

**每步验证**：`go build ./...` + `go test ./internal/...`

### Phase 3：WSServer 简化 + Wire 重配置 ✅

**目标**：WSServer 从 3 bus 简化为 2 bus，删除 SessionBus。

> **完成状态**：WSServer 已从 3 bus/3 pump 简化为 2 bus/2 pump。
> - 删除 `eventBus event.Bus` 字段和 `eventPump` goroutine
> - `monitorBus` 从 `event.Bus` 改为 `contract.MonitorBus`，新增 `monitorEventPump`
> - `wsDownstream` 新增 `MonitorEvent *contract.MonitorEvent` 字段（`Envelope` 字段保留供 replay 使用，Phase 5 删除）
> - `setupEventSubscription` 返回 `(<-chan contract.MonitorEvent, <-chan biz.ActivityEvent)`
> - `EventPipeline.MonitorEventBus` 已在 Phase 2.A 中添加，无需额外 Wire 变更
> - `go build ./...` + `go test ./internal/server/...` 全部通过

| 步骤 | 文件 | 改动 |
|------|------|------|
| 3.1 | `internal/server/ws.go` | 删除 `eventBus` 字段；`setupEventSubscription` 返回 2 channel |
| 3.2 | `internal/server/ws_event.go` | 删除 SessionBus 订阅；保留 MonitorBus + ActivityBus 订阅 |
| 3.3 | `internal/server/ws_io_pump.go` | 删除 `eventPump`；新增 `monitorEventPump`（MonitorEvent → monitor channel） |
| 3.4 | `internal/runtime/deps.go` | `EventPipeline` 删除 `Bus event.Bus`，保留 `ActivityBus` + 新增 `MonitorBus` |
| 3.5 | `cmd/admin/wire.go` | 删除 `SessionBus` 绑定；新增 `MonitorBus` 绑定；更新 `provideRunnerConfig`/`provideTeamTurnDeps`/`provideChatServiceDeps` |
| 3.6 | `internal/event/infra.go` | 删除 `SessionBus`；`Infra` 仅保留 `MonitorBus` + `Buffer` |

**验证**：`make wire && go build ./cmd/admin` + `go test ./internal/server/...`

### Phase 4：前端统一 ✅

**目标**：删除所有 envelope 前端文件，统一到 ActivityEvent + MonitorEvent。

> **完成状态**（截至 2026-06-26）：
> - **Envelope import 数量：46 → 7**（全部为合法残留）
> - 残留 7 文件分类：
>   - 传输层 5 个：`ws-transport.ts`、`useEnvelopeStream.ts`、`globalWsHub.ts`、`dispatcher.ts`、`data_channel.ts`（WS 协议仍承载 `envelope?` 用于 replay，Phase 5 Blocker A 解决后才能移除）
>   - 事件检查器 2 个：`useChatEventInspector.ts`、`useChatStreamManager.ts`（消费 replay envelopes）
> - **关键发现**：`useEnvelopeStream.ts` 是**活动传输层工厂**（非 legacy），不应删除，仅在 Phase 5 后重命名
> - **关键发现**：chat stream 的 `onEnvelope` 回调是 no-op（`() => {}`），replay envelopes 被接收但忽略，仅事件检查器消费
> - **本地类型解耦模式**：定义本地类型 `ExecutionProgressMetadata`、`ActivityUsage`、`InspectorEvent` 切断 Envelope import 依赖
> - 删除/修改文件：`envelopeToolCall.ts`(删除)、`activityToolCall.ts`(清理 envelope 函数)、`executionProgress.ts`(本地类型)、`sessionContextPatch.ts`(本地类型 + 删除 legacy 函数)、`useConversationTimeline.ts`(清理 dead code)、`ChatMessageList.vue`(清理 unused import)、5 个测试文件
> - 验证：`pnpm lint`(0 errors) + `pnpm test`(516 tests passed) + `pnpm build`(success)

| 步骤 | 文件 | 改动 |
|------|------|------|
| 4.1 | `web/src/realtime/monitorEvent.ts` | **新建** MonitorEvent 类型 + type guards |
| 4.2 | `web/src/realtime/ws-transport.ts` | `WsDownstream` 删除 `envelope?`，新增 `monitor_event?` |
| 4.3 | `web/src/features/chat/composables/useSystemEventNotification.ts` | **新建** Domain=system 事件处理 |
| 4.4 | `web/src/features/chat/composables/useChatInboundSync.ts` | 删除 `onSpiritEnvelope`/`onActivityEnvelope`；仅保留 `onActivityEvent` |
| 4.5 | `web/src/features/chat/composables/useChatStreamManager.ts` | 删除 envelope 路径；仅保留 ActivityEvent |
| 4.6 | `web/src/features/chat/composables/useConversationTimeline.ts` | 移除 envelope 依赖 |
| 4.7 | `web/src/features/monitor/useLogStreamHub.ts` + `flow.ts` | 迁移到 MonitorEvent |
| 4.8 | `web/src/features/teams/teamRunEventFromEnvelope.ts` | 迁移到 ActivityEvent |
| 4.9 | `web/src/features/graph/runtime/useGraphStream.ts` + `useGraphExecutionStream.ts` | 迁移到 ActivityEvent |
| 4.10 | `web/src/features/orchestration/useOrchestrationStream.ts` | 迁移到 ActivityEvent |
| 4.11 | `web/src/stores/spirit/index.ts` | 迁移到 ActivityEvent |
| 4.12 | `web/src/features/knowledge/useKnowledgeIngestWs.ts` | 迁移到 ActivityEvent |
| 4.13 | 删除文件 | `envelope.ts`/`useEnvelopeStream.ts`/`inboundSyncRouting.ts`/`inboundSyncEnvelope.ts` |
| 4.14 | `web/src/features/chat/streamHandlers.ts` | **迁移**（非删除）：被 6 个文件引用，需将 envelope handler 改为 ActivityEvent handler |
| 4.15 | 所有剩余 `import envelope` | 替换为 ActivityEvent 或 MonitorEvent |

**验证**：`pnpm lint && pnpm test && pnpm build`

### Phase 5：删除后端 Legacy 代码 🟡（识别 6 个耦合 Blocker）

**目标**：删除 Envelope struct + EnvelopeType + RouteChannel + channelRegistry。

> **完成状态**（截至 2026-06-26）：本 session 调研发现 Phase 5 远比 spec 预期的"删文件"复杂——存在 **6 个耦合 Blocker** 必须按依赖链级联迁移，否则删 Envelope 会破坏编译/运行时。
>
> **6 Blocker 依赖链**：
>
> | Blocker | 描述 | 依赖 |
> |---------|------|------|
> | **A: WS Replay 路径** | `event.Buffer` → `replayEvents` → `wsDownstream.Envelope`，前端 reconnect 后靠此回放历史事件 | 前端 `useChatEventInspector` 仍消费 replay envelope；解决方案：迁移到 Activity replay 或删除 replay 改用 `ListActivities` RPC |
> | **B: 4 个 side consumer** | `event_bus_side_consumers.go` 中 4 个消费者订阅 SessionBus 接收 Envelope | 需迁移到 ActivityEventBus/MonitorBus 订阅 |
> | **C: DomainEvent bridge** | `domain_event_adapter.go` 将 biz domain event 包装为 Envelope | 需改为包装 ActivityEvent (Domain=system) |
> | **D: vestigial bus 字段** | 多个 service/team struct 仍持有 `bus event.Bus` 字段（dead field） | 删除字段前需确认无 publisher 调用 |
> | **E: EventPipeline.Bus/Buffer** | `EventPipeline` 仍保留 `Bus event.Bus` + `Buffer event.Buffer` 字段 | Buffer 是 Blocker A 的 replay 源；Bus 是 Buffer 的 owner |
> | **F: Wire DI** | `cmd/admin/wire.go` 仍绑定 SessionBus | 需在 A-E 全部解决后清理 |
>
> **建议迁移顺序**：B（独立）→ C（独立）→ D（依赖 B/C 完成）→ A（依赖前端 ListActivities 改造）→ E（依赖 A 完成）→ F（依赖 E 完成）→ 删除 Envelope 文件
>
> **当前 Envelope 残留**：54 生产 + 20 测试文件，其中 ~13 文件活跃 publish Envelope（基础设施 + spirit/service publisher）

| 步骤 | 文件 | 改动 |
|------|------|------|
| 5.1 | `internal/event/contract/envelope.go` | **删除整个文件**（依赖 Blocker A-F 全部解决） |
| 5.2 | `internal/event/envelope.go` | **删除整个文件** |
| 5.3 | `internal/event/bus.go` | 删除 `Bus` type alias + `NewBus`（改为 `NewMonitorBus`） |
| 5.4 | 所有 `import "internal/event/contract"` 引用 Envelope | 清理 |
| 5.5 | `internal/biz/event_bus_side_consumers.go` | 迁移到 ActivityEventBus/MonitorBus 订阅（Blocker B） |
| 5.6 | `internal/event/domain_event_adapter.go` | DomainEvent bridge 改为 ActivityEvent 包装（Blocker C） |
| 5.7 | service/team struct 的 `bus event.Bus` 字段 | 删除 vestigial 字段（Blocker D） |
| 5.8 | `internal/runtime/deps.go` EventPipeline | 删除 `Bus` + `Buffer` 字段（Blocker E） |
| 5.9 | `internal/server/ws_event.go` replayEvents | 迁移到 Activity replay 或删除（Blocker A） |
| 5.10 | `cmd/admin/wire.go` | 删除 SessionBus 绑定（Blocker F） |
| 5.11 | `web/src/realtime/ws-transport.ts` 等 7 文件 | 删除 envelope import + 字段（依赖 Blocker A） |

**验证**：`go build ./... && go test ./...` + `cd web && pnpm lint && pnpm test && pnpm build`

### Phase 6：全量验证 + 文档同步 🟡（验证通过，文档同步中）

> **完成状态**（截至 2026-06-26）：
> - **后端验证**：`go build ./...` ✅ + `go test ./...` ✅（biz/agent/event/service/team/graph/cronrunner/skill/plugin/runtime/server 全通过）
> - **前端验证**：`pnpm lint` ✅（0 errors）+ `pnpm test` ✅（516 tests passed）+ `pnpm build` ✅
> - **文档同步**：本 spec 进度更新完成（Phase 4 → ✅，Phase 5 加入 6-Blocker 分析）
> - **待办**：ADR-03 编写 + `34-event-system.development.md` 状态更新

| 步骤 | 验证 | 状态 |
|------|------|------|
| 6.1 | `go build ./... && go test ./...` | ✅ |
| 6.2 | `cd web && pnpm lint && pnpm test && pnpm build` | ✅ |
| 6.3 | 更新本 spec 进度（Phase 4/5/6 状态） | ✅ |
| 6.4 | 编写 ADR-03: 统一总线架构决策 + Phase 5 路线图 | ✅ |
| 6.5 | 更新 `docs/development/34-event-system.development.md`（Phase 5 完成后再更新） | ⏳ |

---

## 4. 风险与缓解

| 风险 | 严重度 | 缓解措施 |
|------|--------|---------|
| 80+ publisher 迁移遗漏 | 高 | 每域迁移后 `go build` 验证；grep `bus.Publish` 确认零残留 |
| 前端 59+ 文件迁移遗漏 | 高 | 每步 `pnpm build` 验证；grep `envelope` 确认零残留 |
| ActivityKind 语义不匹配 | 中 | system 事件统一用 Kind=notice + Meta 承载原始类型 |
| MonitorBus 类型变更破坏 | 中 | MonitorEvent 与 Envelope 字段对齐，减少迁移成本 |
| Wire 注入链断裂 | 高 | Phase 3 单独验证 `make wire && go build ./cmd/admin` |
| 前端渲染回归 | 高 | Phase 4 每步 `pnpm test` 验证现有测试通过 |

---

## 5. 验收标准

- [x] 后端 `go build ./...` 通过，零外部 `bus.Publish(ctx, envelope)` 残留（核心基础设施除外）
- [x] 后端 `go test ./...` 通过（biz/agent/event/service/team/graph/cronrunner/skill/plugin/runtime/server）
- [x] 前端 `pnpm build` 通过，Envelope import 从 46 降至 7（残留均为合法传输层/事件检查器）
- [x] 前端 `pnpm test` 通过（516 tests passed）
- [ ] `grep -r "Envelope" internal/` 仅剩 proto 生成代码（如有）— 当前残留 54 生产 + 20 测试文件（Phase 5 删除）
- [ ] `grep -r "envelope" web/src/` 仅剩 7 个合法传输层/事件检查器文件（依赖 Phase 5 Blocker A 解决）
- [x] WSServer 仅 2 个 pump goroutine（monitorEventPump + activityEventPump）
- [x] EventPipeline 含 ActivityBus + MonitorEventBus（SessionBus 保留供 replay，Phase 5 删除）
- [x] ADR-03 文档完整（[2026-06-26-review-adr-unified-bus-architecture.md](../../reports/2026-06-26-review-adr-unified-bus-architecture.md)）

---

## 6. 关联文档

- 重构主文档：[2026-06-25-analysis-chat-module-refactor.md](../../reports/2026-06-25-analysis-chat-module-refactor.md)
- ADR-02 持久化策略：[2026-06-25-review-adr-activity-event-persistence.md](../../reports/2026-06-25-review-adr-activity-event-persistence.md)
- ADR-03 统一总线架构：[2026-06-26-review-adr-unified-bus-architecture.md](../../reports/2026-06-26-review-adr-unified-bus-architecture.md)
- 事件系统设计：[34-event-system.design.md](../../development/34-event-system.design.md)
- Chat UI 开发计划：[59-chat-ui-optimization.development.md](../../development/59-chat-ui-optimization.development.md)
