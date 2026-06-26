# ADR-03: 统一总线架构（删除 Envelope，统一到 ActivityEvent + MonitorEvent）

## 状态：已接受（Phase 1-4 完成；Phase 5 全部完成 A/B/C/D/E/F/G——方案 C：活类型已提取到 envelope_types.go，envelope.go 死代码及 buffer.go 已删除）

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
  - 截至 2026-06-27：Blocker A/B/C/D/E/F/G 已完成。活类型 `EnvelopeError`/`EnvelopeTokenUsage` + 5 个活 ErrorCode 常量已提取到 `contract/envelope_types.go`；`contract/envelope.go`/`envelope_test.go`/`envelope_contract_test.go`/`reliability.go`/`reliability_test.go`/`internal/event/buffer.go`（WS replay Buffer 死代码）已删除；`internal/event/envelope.go` 重导出文件精简为 2 个 type alias + 4 个活 ErrorCode 常量；`internal/channel/preview/transcript.go` 和 `tool_display.go` 死方法已清理；`internal/team/parity_run_test.go`/`parity_run_e2e_test.go` 中的 envelope 死代码已清理。后端 Envelope 类型在生产代码中已彻底移除（前端仍有合法传输层 import，依赖前端后续清理）
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
| **F: Wire DI** | `cmd/admin/wire.go` 仍绑定 SessionBus | ✅ 已完成 | **Stage 1（死参数链清理）**：删除 `EventPipeline.Bus` 字段及 5 个 Wire provider 的 `eventBus` 死参数，删除 `configureMCPObserve` NO-OP stub，删除 `ChannelIngress.eventBus`/`TurnPreviewCoordinator.bus` 死字段及关联死方法。**Stage 2（死订阅修复）**：`SelfHealObserver`/`TraceProjector` 从旧 envelope bus 迁移到 `MonitorEventBus`（修复死订阅 bug）；删除 `Infra.MonitorBus` 字段、`ProvideMonitorBus` 函数、`monitorBusFromInfra` helper。**Stage 3（SessionBus 删除）**：确认 `wire_gen.go` 中无 `ProvideSessionBus`/`SessionBus` 引用（所有消费者已在 Stage 1 移除），删除 `Infra.SessionBus` 字段、`ProvideSessionBus` 函数、从 `InfraProviderSet` 移除。`Infra` 只剩 `MonitorEventBus` + `lg` |
| **G: 删除 Envelope 文件** | 删除 `contract/envelope.go` 及 bus 死代码 | ✅ 已完成（方案 C） | 采用方案 C：第一阶段删除 bus 死代码（`event.Bus`/`contract.Bus`/`busAdapter`/`RecordingBus`/`FromFrameworkEvent` + 7 个测试文件）；第二阶段提取活类型到 `contract/envelope_types.go`（`EnvelopeError`/`EnvelopeTokenUsage` + 5 个活 ErrorCode 常量），删除 `contract/envelope.go`/`envelope_test.go`/`envelope_contract_test.go`/`reliability.go`/`reliability_test.go`，删除 `internal/event/buffer.go`（WS replay Buffer 死代码），精简 `internal/event/envelope.go` 重导出，清理 `transcript.go`/`tool_display.go` 死方法，清理 `parity_run_test.go`/`parity_run_e2e_test.go` 中的 envelope 死代码 |

**当前迁移顺序**：~~B → C → D → A → E → F~~ → 实际进度：B✅ → C✅ → A✅ → E✅ → D✅ → F✅ → G✅（方案 C：活类型已提取，envelope.go 死代码已删除）

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

**保留的项目**（Stage 1 时仍被 `SelfHealObserver`/`TraceProjector` 等活跃使用）：
- `ProvideSessionBus` Wire 绑定（仍被 EventBusConsumer 等使用，待 Blocker F 主体迁移）
- `Infra.SessionBus` 字段（同上）
- ~~`monitorBusFromInfra` helper~~ — Stage 2 已删除
- ~~`Infra.MonitorBus` 字段~~ — Stage 2 已删除
- `event` import in `wire.go`（`ProvideSessionBus` 仍使用 `event.Bus`；`monitorBusFromInfra` 已删除）

**验证结果**：`go build ./...` ✅ | `make wire` ✅ | `go build ./cmd/admin` ✅ | `go test ./internal/event/... ./internal/agent/... ./internal/service/... ./internal/team/... ./internal/runtime/... -count=1` ✅ | `go vet ./...` ✅

### Blocker F Stage 2 完成总结（2026-06-26）：SelfHealObserver / TraceProjector 迁移到 MonitorEventBus

Stage 1 清理死参数链后，`SelfHealObserver` 和 `TraceProjector` 仍订阅旧 envelope bus（`contract.Bus` 传输 `contract.Envelope`），但 FlowTracker（发布者）已迁移到 `contract.MonitorBus`（传输 `contract.MonitorEvent`）。这两个订阅者是**死订阅**——收不到任何 FlowLog 事件，自愈观察与 trace 投影功能已失效。Stage 2 将它们迁移到 `MonitorEventBus`，实质上是修复 bug：让订阅者重新收到 FlowLog 事件。

**迁移的订阅者**（参考 `FlowFileAppender.Start` 的迁移模式）：

1. `SelfHealObserver.StartEventDrivenObservation`（`internal/biz/monitor/self_heal_observer.go`）
   - 参数类型：`buses ...contract.Bus` → `buses ...contract.MonitorBus`
   - 订阅选项：`contract.SubscribeOptions{EventTypes, BufferSize, DropPolicy}` → `contract.MonitorSubscribeOptions{Filter: ev.Type == MonitorEventTypeFlowLog, BufferSize: 1024, GlobalMode: true}`
   - channel 类型：`<-chan contract.Envelope` → `<-chan contract.MonitorEvent`
   - 处理调用：`o.ObserveFlowLogEvent(ctx, env.Metadata)` → `o.ObserveFlowLogEvent(ctx, ev.Metadata)`

2. `TraceProjector`（`internal/biz/monitor/trace_projector.go`）
   - `NewTraceProjector` 参数：`buses ...contract.Bus` → `buses ...contract.MonitorBus`
   - `buses` 字段类型：`[]contract.Bus` → `[]contract.MonitorBus`
   - `Start()` 订阅选项：同上迁移到 `MonitorSubscribeOptions` + `Filter`
   - `subscribeBus` / `handle` / `traceProjectorWorker` 全部从 `contract.Envelope` 改为 `contract.MonitorEvent`
   - `handle` 中 `env.TeamID`（MonitorEvent 无此字段）改为 `metaStr(m, "team_id")`（从 Metadata 读取，行为一致——FlowTracker.toMetadata 不写 team_id，原 Envelope.TeamID 在死订阅期间也是空值）

**Wire 注入更新**（`cmd/admin/wire.go` + `cmd/admin/workers.go`）：
- `wireOut.MonitorBus` 字段类型：`event.Bus` → `contract.MonitorBus`
- `provideWireOut` 赋值：`eventInfra.MonitorBus` → `eventInfra.MonitorEventBus`
- `provideTraceProjector`：`infra.MonitorBus` → `infra.MonitorEventBus`
- `backgroundWorkersConfig.MonitorBus` 字段类型：`event.Bus` → `contract.MonitorBus`
- import：`internal/event` → `internal/event/contract`（workers.go）

**删除的死代码**：
- `monitorBusFromInfra` helper（`cmd/admin/wire.go`）— 返回 `infra.MonitorBus`（旧 bus），无 Go 代码调用者
- `Infra.MonitorBus` 字段（`internal/event/infra.go`）— 所有使用点已迁移到 `MonitorEventBus`
- `ProvideMonitorBus` 函数（`internal/event/infra.go`）— 已从 `InfraProviderSet` 移除，无调用者
- `NewInfra` 中 `MonitorBus: NewBus(lg)` 初始化

**测试更新**：
- `mockBus`（`internal/biz/monitor/monitor_more_test.go`）— 从实现 `contract.Bus` 改为实现 `contract.MonitorBus`（Publish/Subscribe/DropCount 签名全部改为 MonitorEvent 类型）
- `ws_auth_test.go` / `ws_protocol_test.go`（`internal/server/`）— `event.Infra` 字面量中移除 `MonitorBus` 字段，保留 `MonitorEventBus`
- `NewWSServer`（`internal/server/ws.go`）— 遗留构造函数中移除 `MonitorBus: eventBus` 行（MonitorEventBus 为 nil，函数无 Go 调用者，仅保留兼容性）

**保留的项目**（待 Blocker F 主体迁移）：
- `ProvideSessionBus` Wire 绑定 — 仍被 EventBusConsumer 等 8+ 调用者使用
- `Infra.SessionBus` 字段 — 同上
- `event` import in `wire.go` — `ProvideSessionBus` 仍使用 `event.Bus`

**验证结果**：`make wire` ✅ | `go build ./...` ✅ | `go test ./internal/biz/monitor/... ./internal/event/... ./internal/service/... ./internal/server/... -count=1` ✅ | `go vet ./...` ✅

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
- ✅ 后续：迁移 `EventBusConsumer` 整体到 `ActivityEventBus` 订阅（Blocker F 主体完成总结确认 8 个 consumer 全部已迁移，详见下节）

**解耦后的影响**：`biz` 包对 `event` 父包的硬依赖消除，`EventBusConsumer` 可在后续 session 中迁移到 `ActivityEventBus`，迁移完成后即可删除 `ProvideSessionBus` 绑定（Blocker F 主体）和最终删除 `envelope.go`（Blocker G）。

### Blocker F 主体完成总结（2026-06-26）：死代码清理

**关键发现**：代码级核查显示 Blocker F 的主体迁移工作在 Blocker B/D/E 期间已完成，ADR-03 文档此前的状态记录严重过时：

1. **`event_bus_consumer.go` 不存在** — 旧 `EventBusConsumer` 已被拆分为 4 个 typed consumer（`event_bus_callback_consumer.go`/`event_bus_flow_log_consumer.go`/`event_bus_usage_rollup_consumer.go`/`event_bus_user_feedback_consumer.go`），全部订阅 `ActivityEventBus` 或 `contract.MonitorBus`（Blocker B 已完成）
2. **8 个声称的 `ProvideSessionBus` 消费者全部已迁移** — `wire_gen.go` 中无 `contractBus` 局部变量、无 `ProvideSessionBus` 调用、无 `infra.SessionBus` 引用
3. **硬依赖已解决** — `event_bus_consumer.go:40` 的 `event.NewEventDeduplicator` 调用因文件删除而消失；`biz` 包不再 import `internal/event` 父包（仅 import `internal/event/contract` 子包）

**本次清理的死代码**：

| 项目 | 文件 | 说明 |
|------|------|------|
| `NewWSServer` legacy 构造器 | `internal/server/ws.go` | 0 生产调用者；仅委托 `NewWSServerFromInfra` 并传死 `SessionBus` |
| `ProvideSessionBus` 函数 | `internal/event/infra.go` | 0 wire 消费者（wire_gen.go 无引用） |
| `Infra.SessionBus` 字段 | `internal/event/infra.go` | `NewInfra` 初始化但无生产读取者 |
| `internal/event/dedup.go` | 整个文件 | deprecated 别名，0 调用者（`contract.NewEventDeduplicator` 已是唯一入口） |

**测试更新**：
- `ws_protocol_test.go`：`newTestWSServer`/`newTestWSServerWithActivity` 移除 `bus event.Bus` 参数；6 处调用去掉 `event.NewBus(nil)`；`TestWSUpstreamCancelInvokesCanceller` 移除 `bus.Subscribe` 死行为验证（仅保留 `canceller.called` 断言）
- `ws_auth_test.go`：2 处 `event.Infra` 字面量移除 `SessionBus` 字段
- `ws_e2e_test.go`：已由前次提交清理（无 `event.NewBus` 引用）

**保留的项目**（Blocker G 范围）：
- `internal/event/bus.go`（`NewBus` 实现）— 仍被 `bus_*_test.go` 自测引用
- `internal/event/contract/envelope.go`（`Envelope`/`EnvelopeType`/`RouteChannel`）— 活类型 `EnvelopeError`/`EnvelopeTokenUsage` 仍被生产代码使用
- `internal/testutil/bus.go`（`RecordingBus`）— 测试工具，实现 `event.Bus` 接口

**验证结果**：`make wire` ✅ | `go build ./...` ✅ | `go vet ./...` ✅ | `go test ./internal/server/... ./internal/event/... ./internal/testutil/... ./internal/biz/... ./internal/agent/... ./internal/service/... ./internal/team/... ./internal/runtime/... -count=1` ✅

### Blocker G 第一阶段完成总结（2026-06-26）：方案 C——删除 bus 死代码，保留 envelope.go 活类型

Blocker F 完成后，`event.Bus` 接口和 `contract.Bus` 接口已无生产消费者（SessionBus/MonitorBus 均已删除）。但 `contract/envelope.go` 中有多个活类型（`EnvelopeError`/`EnvelopeTokenUsage` 等）仍被生产代码使用，不能删除整个文件。本任务执行方案 C 第一阶段：删除 bus 相关死代码，保留 envelope.go 中的活类型（活类型提取在第二阶段完成）。

**删除的死代码文件**（10 个）：

| 文件 | 内容 | 说明 |
|------|------|------|
| `internal/event/bus.go` | `event.Bus` 类型别名 + `NewBus` 函数 | `NewBus` 仅被 bus 测试调用 |
| `internal/event/bus_adapter.go` | `busAdapter` 结构 | 仅被 `NewBus` 使用 |
| `internal/event/framework_adapter.go` | `FromFrameworkEvent` 函数 + `FrameworkEventMeta` + `coalesceStr`/`isJSONString` | 仅被自身测试调用 |
| `internal/event/framework_adapter_test.go` | 测试 | 测试死代码 |
| `internal/event/bus_basic_test.go` | 测试 | 测试 `event.NewBus` |
| `internal/event/bus_race_test.go` | 测试 | 测试 `event.NewBus` |
| `internal/event/bus_backpressure_test.go` | 测试 | 测试 `event.NewBus` |
| `internal/event/contract/bus.go` | `contract.Bus`/`SubscribeOptions`/`DropPolicy`/`ChannelPriority` | `MonitorBus` 是独立接口，不依赖 `contract.Bus` |
| `internal/testutil/bus.go` | `RecordingBus`（实现 `event.Bus`） | 仅在自身测试中使用 |
| `internal/testutil/bus_test.go` | 测试 | 测试死代码 |

**修改的文件**（2 个）：

| 文件 | 修改内容 |
|------|----------|
| `internal/event/contract/envelope.go` | 更新过时的包注释（原注释引用已删除的 `event.Bus`），说明 Blocker G 方案 C 已删除 bus 层，本文件因活类型保留 |
| `internal/tools/memory_butler/registry.go` | 删除 `Deps.EventBus contract.Bus` 死字段（未被赋值/未被读取的"预留"字段）及其 `contract` import |

**保留的活代码**：

| 类型/函数 | 定义位置 | 保留原因 |
|-----------|----------|----------|
| `contract.EnvelopeError` | `contract/envelope.go` | 活跃生产使用（service 层事件投影） |
| `contract.EnvelopeTokenUsage` | `contract/envelope.go` | 活跃生产使用（usage 持久化） |
| `contract.Envelope`/`EnvelopeType`/`NewEnvelope`/`RouteChannel` 等 | `contract/envelope.go` | 仍被事件投影、activity-event bridge 等引用 |
| `contract.MonitorBus`/`MonitorEvent` | `contract/monitor_event.go` | 独立文件，活代码 |
| `EnvelopeSourceFromContext` 等 | `internal/event/source.go` | 独立文件，活跃使用 |
| `internal/event/envelope.go` 中的类型别名 | `internal/event/envelope.go` | 重导出 contract 类型（含 `EnvelopeError`），被 `internal/service/envelope_error.go` 等通过 `event.EnvelopeError` 引用 |

**后续工作**（第二阶段已完成，详见下节）：
1. ✅ 提取 `EnvelopeError`/`EnvelopeTokenUsage` 等活类型到 `contract/envelope_types.go`（第二阶段）
2. ⏳ 迁移剩余 Envelope 投影代码到 ActivityEvent（前端清理，不在后端范围）
3. ✅ 删除 `contract/envelope.go` 剩余部分（第二阶段）

**验证结果**：`go build ./...` ✅ | `go test ./internal/event/... ./internal/testutil/... -count=1` ✅ | `go vet ./...` ✅

### Blocker G 第二阶段完成总结（2026-06-27）：提取活类型 + 删除 envelope.go 死代码

第一阶段删除 bus 层死代码后，`contract/envelope.go` 中仍有大量死代码（`Envelope` struct、~60 个 `EnvelopeType*` 常量、`EnvelopeContent`/`EnvelopeToolCall` 等 helper struct、`NewEnvelope`/`EnvelopeFromFrameworkEvent` 等函数、`RouteChannel` 类型、`ValidErrorCodes`/`ValidateErrorCode` 等），但其中 `EnvelopeError`/`EnvelopeTokenUsage` + 5 个 `ErrorCode*` 常量仍被生产代码使用。第二阶段提取活类型到独立文件后删除剩余死代码。

**任务描述勘误**：原任务描述声称 `ErrorCodeTool*` 常量全部为死代码需删除，但代码级核查显示 5 个 `ErrorCode*` 常量全部活跃（`ErrorCodeToolTimeout`/`ErrorCodeToolError`/`ErrorCodeConfirmationRequired`/`ErrorCodeConfirmationDenied`/`ErrorCodeConfirmationTimeout`），分别被 `activity_projector.go`/`tool_invocation_recorder.go`/`tool_confirmation.go` 调用。按"不确定时停下来判断"原则保留全部活常量。原任务描述也未提及 `reliability.go` 依赖 `EnvelopeType`，核查后确认其为死代码（仅被自身测试调用），一并删除。

**提取的活类型**（`internal/event/contract/envelope_types.go`，新建文件）：

| 类型/常量 | 用途 | 调用位置 |
|-----------|------|----------|
| `EnvelopeError` struct | chat 事件投影错误载体 | `internal/service/envelope_error.go`、`internal/service/chat_event_publisher.go`（通过 `event.EnvelopeError` 别名） |
| `EnvelopeTokenUsage` struct | usage 持久化数据载体 | `internal/biz/event_bus_usage_rollup_consumer.go`（直接使用 `contract.EnvelopeTokenUsage`） |
| `ErrorCodeToolTimeout` const | 工具超时错误码 | `internal/agent/activity_projector.go:1303`（直接使用 `contract.ErrorCodeToolTimeout`） |
| `ErrorCodeToolError` const | 工具失败错误码 | `internal/agent/tool_invocation_recorder.go`（通过 `event.ErrorCodeToolError`） |
| `ErrorCodeConfirmationRequired` const | 工具确认必需错误码 | `internal/agent/tool_invocation_recorder.go`、`internal/agent/tool_confirmation.go`（通过 `event.*`） |
| `ErrorCodeConfirmationDenied` const | 工具确认拒绝错误码 | `internal/agent/tool_confirmation.go`（通过 `event.*`） |
| `ErrorCodeConfirmationTimeout` const | 工具确认超时错误码 | `internal/agent/tool_confirmation.go`（通过 `event.*`） |

**删除的死代码文件**（6 个）：

| 文件 | 内容 | 说明 |
|------|------|------|
| `internal/event/contract/envelope.go` | `Envelope` struct、~60 `EnvelopeType*` 常量、`EnvelopeContent`/`EnvelopeToolCall` 等 helper struct、`NewEnvelope`/`EnvelopeFromFrameworkEvent` 等函数、`RouteChannel` 类型、`ValidErrorCodes`/`ValidateErrorCode` | 活类型已提取到 `envelope_types.go`，剩余死代码 |
| `internal/event/contract/envelope_test.go` | 测试 | 测试已删除的 envelope.go |
| `internal/event/contract/envelope_contract_test.go` | 测试 | 测试已删除的 envelope.go |
| `internal/event/contract/reliability.go` | `ReliabilityForType`/`ParseReliability`/`ReliabilityFromContext`（依赖 `EnvelopeType`） | 仅被自身测试调用，依赖已删除的 `EnvelopeType` |
| `internal/event/contract/reliability_test.go` | 测试 | 测试死代码 |
| `internal/event/buffer.go` | `Buffer` struct（WS replay 环形缓冲区，使用 `Envelope` 类型） | Blocker A 已删除 WS replay 路径，`Buffer` 仅被注释引用，无生产调用者 |

**修改的文件**（4 个）：

| 文件 | 修改内容 |
|------|----------|
| `internal/event/envelope.go` | 精简为仅 2 个 type alias（`EnvelopeError`/`EnvelopeTokenUsage`）+ 4 个活 ErrorCode 常量（`ErrorCodeToolError`/`ErrorCodeConfirmationRequired`/`ErrorCodeConfirmationDenied`/`ErrorCodeConfirmationTimeout`），删除 `Envelope`/`EnvelopeType*`/`NewEnvelope` 等死别名 |
| `internal/channel/preview/transcript.go` | 删除 `Apply`/`appendText`/`setText`/`appendMember`/`setMember`/`breakTextSegment`/`breakReasoningSegment`/`breakSegmentID`/`segmentKindForTextID`/`toolMetaFromEnvelope`/`mergeToolMeta`/`HasInFlightTool` 等 12 个死方法（仅消费已删除的 Envelope 类型） |
| `internal/channel/preview/tool_display.go` | 删除 `excerptToolResult`/`extractJSONErrorMessage`/`compactJSONOneLine` 3 个死函数（仅被已删除的 `toolMetaFromEnvelope` 调用） |
| `internal/team/parity_run_test.go` + `parity_run_e2e_test.go` | 删除 `graphOnlyEnvelopeTypes`/`TestParityRunEnvelopeDiff_documented`/`envelopeTypeSet`/`intersectEnvelopeSets`/`envelopeTypeHash`/`envelopeTypeSetFromEnvs` 等 envelope 死代码 helper；保留 `TestParityRunSummary_AllModes`/`TestParityRunE2E_stubStreamAllModes`（独立于 envelope） |

**ErrorCodeToolTimeout 重导出说明**：`event.ErrorCodeToolTimeout` 无调用者，因此 `internal/event/envelope.go` 不重导出此常量；`internal/agent/activity_projector.go:1303` 直接使用 `contract.ErrorCodeToolTimeout`。

**验证结果**：`go build ./...` ✅ | `go test ./... -count=1` ✅ | `go vet ./...` ✅

## 关联文档

- 设计 spec：[2026-06-25-unified-bus-architecture-design.md](../superpowers/specs/2026-06-25-unified-bus-architecture-design.md)
- 重构主文档：[2026-06-25-analysis-chat-module-refactor.md](./2026-06-25-analysis-chat-module-refactor.md)
- ADR-02 持久化策略：[2026-06-25-review-adr-activity-event-persistence.md](./2026-06-25-review-adr-activity-event-persistence.md)
- 事件系统设计：[34-event-system.design.md](../development/34-event-system.design.md)
