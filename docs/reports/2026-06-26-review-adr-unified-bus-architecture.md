# ADR-03: 统一总线架构（删除 Envelope，统一到 ActivityEvent + MonitorEvent）

## 状态：已接受（Phase 1-4 完成；Phase 5 中 A/B/C/E 已完成，D 解耦中，F 部分完成）

## 背景

Aranea-Agents 事件系统长期存在 **3 个 bus 并存** 的架构债务：

| Bus | 类型 | 传输载体 | 用途 |
|-----|------|---------|------|
| SessionBus | `event.Bus` | `Envelope` | chat + teams + graph + spirit + orchestration + domain 事件 |
| MonitorBus | `event.Bus` | `Envelope` | monitor 事件（log/flow_log/mcp/alert） |
| ActivityBus | `biz.ActivityEventBus` | `ActivityEvent` | chat 渲染事件（text/tool/reasoning/member） |

**核心问题**：

1. **双总线并存**：`EventPipeline` 同时承载 `Bus event.Bus` + `ActivityBus biz.ActivityEventBus`，相同语义事件在两条总线上重复流转
2. **Spirit/Graph 双发布**：publisher 同时调用 `bus.Publish(ctx, envelope)` 和 `ActivityProjector`，导致前端收到重复事件
3. **WSServer 三 pump**：订阅 3 个 bus，运行 3 个 pump goroutine，复杂度高
4. **前端类型膨胀**：59+ 前端文件依赖 `Envelope` 类型，与 `ActivityEvent` 类型并存增加心智负担
5. **80+ publisher 调用**：`bus.Publish(ctx, envelope)` 散布在各域，维护成本高

该债务在 [2026-06-25-analysis-chat-module-refactor.md](./2026-06-25-analysis-chat-module-refactor.md) §3.6 + §6.3 + §12.2 中被识别为最高优先级重构项。用户明确指令：**"双总线架构（Envelope + ActivityEvent）长期并存。这个设计问题的等级提高到最高。按照文档进行优化。要彻底解决。"**

## 决策

### D1：ActivityEvent 新增 Domain 字段，统一 chat + system 事件

ActivityEvent 新增 `Domain ActivityDomain` 字段，区分两类事件：

```go
type ActivityDomain string

const (
    ActivityDomainChat   ActivityDomain = "chat"   // 持久化到 Activity 表
    ActivityDomainSystem ActivityDomain = "system" // 仅推送 WS，不持久化
)
```

**持久化规则**：
- `Domain=chat` → ActivityEventSequencer 正常持久化到 Activity 表，前端加入时间线渲染
- `Domain=system` → ActivityEventSequencer **跳过持久化**，仅 publish 到 WS，前端作为通知处理（toast/notification）

**替代方案**：考虑过保留 Envelope 用于 system 事件，但被否决——会延续双总线债务。Domain 字段模式让 ActivityEventBus 成为唯一的 chat+system 事件传输载体，MonitorBus 仅处理 monitor 事件。

### D2：新建 MonitorEvent 类型，从 Envelope 拆出 monitor 事件

从 `envelope.go` 拆出 monitor 事件类型到 `internal/event/contract/monitor_event.go`：

```go
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
```

**MonitorBus 接口**改为传输 `MonitorEvent`（而非 `Envelope`）：

```go
type MonitorBus interface {
    Publish(ctx context.Context, event MonitorEvent)
    Subscribe(opts MonitorSubscribeOptions) (<-chan MonitorEvent, func())
    DropCount() uint64
}
```

**设计依据**：monitor 事件高频（100+/sec）、不持久化、与业务事件语义完全不同，独立类型 + 独立 bus 更清晰。

### D3：逐个迁移 80+ Publisher

按域逐个修改 publisher 调用点（详见 spec §2.3 迁移映射表），将 `bus.Publish(ctx, envelope)` 改为：
- 业务事件 → `activityBus.Publish(ctx, activityEvent)`（Domain=chat 或 system）
- 监控事件 → `monitorBus.Publish(ctx, monitorEvent)`

**迁移顺序**（按域独立验证）：Chat 控制 → Teams → Graph → Spirit/Butler → Domain → Monitor → 删除双发布

### D4：ActivityProjector 扩展 EmitSystemEvent

新增 `EmitSystemEvent` 方法，用于发布 `Domain=system` 事件（非 chat 工作单元）：

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

`publishTask.persist` 字段已存在，仅需确保 `EmitSystemEvent` 传 `persist=false`。

### D5：WSServer 从 3 bus/3 pump 简化为 2 bus/2 pump

```go
type WSServer struct {
    monitorBus  contract.MonitorBus   // 监控事件
    activityBus biz.ActivityEventBus   // 所有 chat + system 事件
    // 删除 eventBus event.Bus（SessionBus 不再存在）
}
```

- 2 个 pump goroutine：`monitorEventPump` + `activityEventPump`
- `wsDownstream` 协议：`activity_event?` + `monitor_event?` + `envelope?`（replay 专用，Phase 5 移除）

### D6：前端统一到 ActivityEvent + MonitorEvent

- `WsDownstream` 删除 `envelope?` 字段（依赖 Phase 5 Blocker A 解决 WS replay）
- 所有 `import type { Envelope }` → `import type { ActivityEvent }` 或 `MonitorEvent`
- 新增 `useSystemEventNotification.ts` 处理 `Domain=system` 事件
- **本地类型解耦模式**：定义本地类型（`ExecutionProgressMetadata`、`ActivityUsage`、`InspectorEvent`）切断 Envelope import 依赖，避免传输层未迁移前的循环依赖

## 后果

### 正面

- **架构简化**：3 bus → 2 bus（ActivityEventBus + MonitorBus），职责清晰
- **前端类型统一**：Envelope import 从 46 降至 7（残留均为合法传输层/事件检查器，依赖 Phase 5 Blocker A 解决）
- **双发布消除**：Spirit/Graph 事件不再同时走 envelope Bus 和 ActivityProjector
- **WSServer 复杂度降低**：3 pump → 2 pump，删除 SessionBus 订阅逻辑
- **持久化语义清晰**：Domain 字段显式声明持久化意图，system 事件不再无谓写入 DB
- **测试覆盖**：后端 `go test ./...` + 前端 `pnpm test`（516 tests）全通过

### 负面

- **Phase 5 复杂度超预期**：原 spec 估计"删文件"即可，实际识别出 6 个耦合 Blocker（WS replay、side consumer、DomainEvent bridge、vestigial 字段、EventPipeline、Wire DI），需多 session 级联迁移
- **临时性 Envelope 残留**：Phase 5 完成前，后端仍有生产/测试文件引用 Envelope，前端仍有文件 import Envelope 类型（均为合法传输层）
  - 截至 2026-06-26：Blocker A/B/C/E 已完成，D 因 `event→biz` 架构耦合阻塞（非编译期 cycle，详见下方「Phase 5 实际状态评估」），F 仅完成死参数清理。残留主要源于 `EventBusConsumer` 仍订阅 `contract.Bus`（Envelope 总线），其 Envelope→DomainEvent 转换函数仍被活跃使用
- **WS replay 路径迁移风险**：Blocker A 已通过方案 A2（删除 replay 改用 `ListActivities` RPC）解决，`wsDownstream.Envelope` 字段已删除
- **Domain 字段语义负担**：publisher 必须正确声明 Domain，错误声明会导致 system 事件被持久化或 chat 事件被丢弃

### 风险缓解

- **逐 Phase 验证**：每个 Phase 完成后 `go build` + `go test` + `pnpm test` 全量验证，避免回归积累
- **Blocker 依赖链明确**：B（独立）→ C（独立）→ D（依赖 B/C）→ A（依赖前端改造）→ E（依赖 A）→ F（依赖 E），避免顺序错误
- **本地类型解耦**：前端通过定义本地类型切断 Envelope import，Phase 5 删 Envelope 时仅需删除类型定义，无需修改业务逻辑

## 替代方案

### A1：保留 Envelope，仅修复双发布

**方案**：保留 3 bus 架构，仅删除 Spirit/Graph 的双发布代码。

**否决原因**：
- 不解决前端 59+ 文件依赖 Envelope 类型的问题
- 不解决 WSServer 3 pump 复杂度
- 不解决 EventPipeline 双总线并存
- 用户明确要求"彻底解决"，保留 Envelope 不符合指令

### A2：用 ActivityEvent 替换 Envelope，但保留单一 Bus

**方案**：删除 Envelope，所有事件（chat + system + monitor）都用 ActivityEvent 在单一 Bus 上传输。

**否决原因**：
- monitor 事件高频（100+/sec）且不持久化，与业务事件混在一起会导致 Bus channel 拥塞
- monitor 事件语义与业务事件完全不同（log/flow_log vs text/tool/reasoning），统一类型增加心智负担
- 违反"单一职责"原则

### A3：用 Domain 字段区分持久化，但保留 Envelope 结构

**方案**：Envelope 新增 Domain 字段，保留 Envelope struct 作为唯一传输载体。

**否决原因**：
- 前端仍需处理 Envelope 类型，不解决类型膨胀问题
- Envelope 设计为"通用信封"，承载过多语义（chat/system/monitor），违反单一职责
- ActivityEvent 已有 Activity snapshot，Envelope 的 metadata packing 模式是历史包袱

## Phase 5 剩余路线图（6 Blocker 依赖链）

### Phase 5 实际状态评估（2026-06-26）

经代码级核查，各 Blocker 的真实完成状态如下：

| Blocker | 描述 | 状态 | 实际阻塞原因 |
|---------|------|------|-------------|
| **A: WS Replay 路径** | `event.Buffer` → `replayEvents` → `wsDownstream.Envelope` | ✅ 已完成 | 采用方案 A2（删除 replay 改用 `ListActivities` RPC），`wsDownstream.Envelope` 字段已删除 |
| **B: 4 个 side consumer** | `event_bus_side_consumers.go` 订阅 SessionBus 接收 Envelope | ✅ 已完成 | 4 个 side consumer 已迁移到 ActivityEventBus/MonitorBus 订阅；残留占位赋值已清理（B-DEBT-4） |
| **C: DomainEvent bridge** | `domain_event_adapter.go` 将 biz domain event 包装为 Envelope | ✅ 已完成 | 已迁移到 ActivityEventBus + `ActivityDomainSystem`；`eventBusAdapter` 死代码已清理；`DomainEventPublisher`/`DomainEventSubscriber` 接口已删除 |
| **D: vestigial bus 字段** | service/team struct 仍持有 `bus event.Bus` 字段 | ✅ 已完成 | 3 个死发布者全部删除：`EmitProgress`（ExecutionProgress envelope）、`FlowTracker.LogError` Error envelope publish、`PublishSessionRevisionEnvelope`（RunStatus envelope）。顺带删除 `Infra.Publish`/`publishToBuses` 死路由代码（无调用者）。保留 bump 半边（`BumpSessionRevision`）——前端通过 ListActivities/GetSession RPCs 读取 |
| **E: EventPipeline.Bus/Buffer** | `EventPipeline` 保留 `Bus event.Bus` + `Buffer event.Buffer` | ✅ 已完成 | `Buffer event.Buffer` 已删除（replay 源已不需要）。`Bus event.Bus` 字段已在 Blocker F Stage 1 中删除（死参数链清理） |
| **F: Wire DI** | `cmd/admin/wire.go` 仍绑定 SessionBus | 🟡 Stage 1 完成 | **Stage 1（死参数链清理）已完成**：删除 `EventPipeline.Bus` 字段及 5 个 Wire provider 的 `eventBus` 死参数，删除 `configureMCPObserve` NO-OP stub，删除 `ChannelIngress.eventBus`/`TurnPreviewCoordinator.bus` 死字段及关联死方法。主体 `ProvideSessionBus` 仍绑定，被 8 处 consumer 活跃使用（D 完成后已无 direct publisher）。需先完成 `EventBusConsumer` 整体迁移到 `ActivityEventBus`/`MonitorEventBus` |

**当前迁移顺序**：~~B → C → D → A → E → F~~ → 实际进度：B✅ → C✅ → A✅ → E✅ → D✅ → F(Stage 1 ✅) → 删除 Envelope 文件

### Blocker D 完成总结（2026-06-26）

Blocker D 的核心工作是删除 3 个发布到 SessionBus 的死发布者。SessionBus 是僵尸总线——唯一生产订阅者 `TurnPreviewCoordinator.consume` 在 Phase 1c-5 已变为 NO-OP（IM channel preview rendering 改为 no-op for chat content）。因此所有发布到 SessionBus 的 envelope 都是死发布者。

**删除的死发布者**：
1. `TraceEmitter.EmitProgress`（ExecutionProgress envelope）— 删除方法 + `step_id.go` 整文件 + 14 处调用点 + 5 个死测试
2. `FlowTracker.LogError` 中的 Error envelope publish 块 — 简化 LogError + 删除死代码 `flowStepsSkipChatError`/`shouldPublishFlowChatError`
3. `PublishSessionRevisionEnvelope`（RunStatus envelope）— 重写 `session_revision.go`，保留 `SessionRevisionBumper` 接口 + `BumpSessionRevision` 函数（bump 半边有效），删除 publish 半边 + 所有调用点更新

**顺带删除的死代码**：
- `Infra.Publish` / `publishToBuses` 路由方法（删除 3 个死发布者后无调用者）
- `session_revision_publish.go`（`ChatService.bumpSessionRevisionAndPublish` 无调用者）
- `deco_session_sync_test.go`（整个测试验证已死的 envelope→web sync 路径）

**保留的 bump 半边**：`SessionRevisionBumper` 接口 + `BumpSessionRevision` 函数递增 `sessions.session_revision`，前端通过 `ListActivities`/`GetSession` RPCs 读取，不依赖 envelope。

### Blocker F Stage 1 完成总结（2026-06-26）：死参数链清理

Blocker D 删除所有 SessionBus 发布者后，`EventPipeline.Bus` 字段及其下游参数链成为死参数。Stage 1 清理整条死参数链，不保留 deprecated 包装。

**删除的死参数**：
1. `EventPipeline.Bus` 字段（`internal/runtime/deps.go`）— SessionBus 已无发布者，字段无消费者
2. `NewTraceEmitter(bus Bus, ...)` 的 `bus` 参数（`internal/event/trace_emitter.go`）— 内部改用 `NewFlowTracker(nil, tc, lg)`，`emit()` 对 nil infra 是安全的（line 166-168: `if ft.infra == nil { return }`）
3. `NewFlowLogger(sessionID, agentKey, bus, lg)` 的 `bus` 参数（`internal/event/flow_context.go`）— 同上
4. `TraceEmitterOpts.Bus` 字段（`internal/event/flow_context.go`）— `NewTraceEmitterForRun` 不再使用
5. `ConsumeEventStream` / `ConsumeEventStreamWithFirstByte` 的 `eventBus` 参数（`internal/agent/turn_helpers.go`）
6. `ConsumeWithFirstByteGuard` 的 `bus` 参数（`internal/agent/turn_stream_helpers.go`）
7. `ChannelIngress.eventBus` 字段 + `NewChannelIngress` 的 `eventBus` 参数（`internal/service/channel_ingress.go`）
8. `TurnPreviewCoordinator.bus` 字段 + `turnPreviewParams.Bus` 字段（`internal/service/channel_turn_preview.go`）
9. `cronrunner.Deps.EventBus` 字段（`internal/cronrunner/runner.go`）

**删除的 NO-OP stub**：
- `configureMCPObserve`（`internal/service/chat.go`）— 函数体已空，无任何调用效果

**删除的死方法（TurnPreviewCoordinator 级联）**：
- `TurnPreviewCoordinator.Start()` 中的 SessionBus 订阅块（`c.bus.Subscribe(...)` → `consume` goroutine）
- `maybeHeartbeat`、`envelopeMatchesRun`、`maybeBindRunID`、`consume` 方法（订阅删除后无调用者）
- `Start()` 简化为：发完 initialAck 后立即返回空 cancel func

**更新的 Wire provider 函数**（`cmd/admin/wire.go`，删除 `eventBus event.Bus` 参数）：
- `provideCronRunnerDeps` — 删除 `eventBus` 参数 + `EventBus` 字段
- `provideRunnerConfig` — 删除未使用的 `eventBus` 参数
- `provideTeamTurnDeps` — 删除 `eventBus` 参数 + `Bus: eventBus` from `EventPipeline`
- `provideChatServiceDeps` — 删除 `eventBus` 参数 + `Bus: eventBus` from `EventPipeline`
- `provideChannelIngress` — 删除 `eventBus` 参数 + `eventBus` from `NewChannelIngress` 调用

**保留的项目**（仍被 `SelfHealObserver` 等活跃使用，不在 Stage 1 范围）：
- `ProvideSessionBus` Wire 绑定
- `Infra.SessionBus` 字段
- `monitorBusFromInfra` helper
- `Infra.MonitorBus` 字段
- `event` import in `wire.go`（`ProvideSessionBus`/`monitorBusFromInfra` 仍使用 `event.Bus`）

**验证结果**：`go build ./...` ✅ | `make wire` ✅ | `go build ./cmd/admin` ✅ | `go test ./internal/event/... ./internal/agent/... ./internal/service/... ./internal/team/... ./internal/runtime/... -count=1` ✅ | `go vet ./...` ✅

### Blocker F 真实阻塞：`event→biz` 架构耦合 + 消费者迁移

Blocker D 完成后，SessionBus 已无任何发布者。剩余的 8 处 consumer 订阅一条不再接收事件的 bus，本质上是死订阅。真实阻塞是 `event→biz` 架构耦合（非编译期 cycle）：

```
internal/event/activityevent → internal/biz   (单向，类型定义依赖)
internal/biz                 → internal/event  (单向，父包，唯一硬依赖)
```

**唯一硬依赖点**：`internal/biz/event_bus_consumer.go:40` 调用 `event.NewEventDeduplicator(event.DefaultDedupCapacity)`

**解耦方案 A（已执行第一步）**：
- ✅ 把 `internal/event/dedup.go` 整体移到 `internal/event/contract/dedup.go`（零依赖文件移动）
- ✅ 在父 `event` 包保留 deprecated type alias（`EventDeduplicator = contract.EventDeduplicator`）+ 包装构造函数 + 重导出常量
- ✅ 修改 `event_bus_consumer.go` 改用 `contract.NewEventDeduplicator(contract.DefaultDedupCapacity)`
- ⏳ 后续：迁移 `EventBusConsumer` 整体到 `ActivityEventBus` 订阅（多 session 工作量，不在本次会话范围）

**解耦后的影响**：`biz` 包对 `event` 父包的硬依赖消除，`EventBusConsumer` 可在后续 session 中迁移到 `ActivityEventBus`，迁移完成后即可删除 `ProvideSessionBus` 绑定（Blocker F 主体）和最终删除 `envelope.go`（Blocker G）。

## 关联文档

- 设计 spec：[2026-06-25-unified-bus-architecture-design.md](../superpowers/specs/2026-06-25-unified-bus-architecture-design.md)
- 重构主文档：[2026-06-25-analysis-chat-module-refactor.md](./2026-06-25-analysis-chat-module-refactor.md)
- ADR-02 持久化策略：[2026-06-25-review-adr-activity-event-persistence.md](./2026-06-25-review-adr-activity-event-persistence.md)
- 事件系统设计：[34-event-system.design.md](../development/34-event-system.design.md)
