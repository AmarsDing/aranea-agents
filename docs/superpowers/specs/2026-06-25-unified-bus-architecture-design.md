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

### Phase 1：后端核心结构扩展（非破坏性）

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

### Phase 2：后端 Publisher 迁移（逐个域）

**目标**：将 80+ publisher 从 `bus.Publish(ctx, envelope)` 迁移到 `activityBus.Publish` 或 `monitorBus.Publish`。

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

### Phase 3：WSServer 简化 + Wire 重配置

**目标**：WSServer 从 3 bus 简化为 2 bus，删除 SessionBus。

| 步骤 | 文件 | 改动 |
|------|------|------|
| 3.1 | `internal/server/ws.go` | 删除 `eventBus` 字段；`setupEventSubscription` 返回 2 channel |
| 3.2 | `internal/server/ws_event.go` | 删除 SessionBus 订阅；保留 MonitorBus + ActivityBus 订阅 |
| 3.3 | `internal/server/ws_io_pump.go` | 删除 `eventPump`；新增 `monitorEventPump`（MonitorEvent → monitor channel） |
| 3.4 | `internal/runtime/deps.go` | `EventPipeline` 删除 `Bus event.Bus`，保留 `ActivityBus` + 新增 `MonitorBus` |
| 3.5 | `cmd/admin/wire.go` | 删除 `SessionBus` 绑定；新增 `MonitorBus` 绑定；更新 `provideRunnerConfig`/`provideTeamTurnDeps`/`provideChatServiceDeps` |
| 3.6 | `internal/event/infra.go` | 删除 `SessionBus`；`Infra` 仅保留 `MonitorBus` + `Buffer` |

**验证**：`make wire && go build ./cmd/admin` + `go test ./internal/server/...`

### Phase 4：前端统一

**目标**：删除所有 envelope 前端文件，统一到 ActivityEvent + MonitorEvent。

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

### Phase 5：删除后端 Legacy 代码

**目标**：删除 Envelope struct + EnvelopeType + RouteChannel + channelRegistry。

| 步骤 | 文件 | 改动 |
|------|------|------|
| 5.1 | `internal/event/contract/envelope.go` | **删除整个文件** |
| 5.2 | `internal/event/envelope.go` | **删除整个文件** |
| 5.3 | `internal/event/bus.go` | 删除 `Bus` type alias + `NewBus`（改为 `NewMonitorBus`） |
| 5.4 | 所有 `import "internal/event/contract"` 引用 Envelope | 清理 |
| 5.5 | `internal/biz/event_bus_side_consumers.go` | 迁移到 ActivityEventBus/MonitorBus 订阅 |

**验证**：`go build ./... && go test ./...`

### Phase 6：全量验证 + 文档同步

| 步骤 | 验证 |
|------|------|
| 6.1 | `make api && make wire && make build && make test && make lint` |
| 6.2 | `cd web && pnpm lint && pnpm test && pnpm build` |
| 6.3 | 更新 `docs/development/34-event-system.*` 三件套 |
| 6.4 | 更新 `docs/development/59-chat-ui-optimization.development.md` |
| 6.5 | 编写 ADR-03: 统一总线架构决策 |

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

- [ ] 后端 `go build ./...` 通过，零 `bus.Publish(ctx, envelope)` 残留
- [ ] 后端 `go test ./...` 通过
- [ ] 前端 `pnpm build` 通过，零 `envelope` import 残留
- [ ] 前端 `pnpm test` 通过
- [ ] `grep -r "Envelope" internal/` 仅剩 proto 生成代码（如有）
- [ ] `grep -r "envelope" web/src/` 零结果
- [ ] WSServer 仅 2 个 pump goroutine
- [ ] EventPipeline 仅 ActivityBus + MonitorBus，无 Bus event.Bus
- [ ] ADR-03 文档完整

---

## 6. 关联文档

- 重构主文档：[2026-06-25-analysis-chat-module-refactor.md](../../reports/2026-06-25-analysis-chat-module-refactor.md)
- ADR-02 持久化策略：[2026-06-25-review-adr-activity-event-persistence.md](../../reports/2026-06-25-review-adr-activity-event-persistence.md)
- 事件系统设计：[34-event-system.design.md](../../development/34-event-system.design.md)
- Chat UI 开发计划：[59-chat-ui-optimization.development.md](../../development/59-chat-ui-optimization.development.md)
