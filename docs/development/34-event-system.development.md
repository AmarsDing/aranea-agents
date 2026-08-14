# Event 事件系统 — 开发计划

> **版本**：2026-06-27 | **状态**：🟢 P1 + P2 + P3 + P5(T3.4) 已实现，P4 工具生命周期事件未实现；✅ 统一总线迁移 Phase 5 Blocker A-G 全部完成（legacy Envelope Bus / SessionBus / MonitorBus 已删除）
> **需求**：[34-event-system.md](./34-event-system.md) · **设计**：[34-event-system.design.md](./34-event-system.design.md)
> **统一总线迁移**：[2026-06-25-unified-bus-architecture-design.md](../superpowers/specs/2026-06-25-unified-bus-architecture-design.md) · **ADR-03**：统一总线架构（已归档，设计内容已并入 34-event-system.design.md）

---

## 1. 模块定位

Event 事件系统：基于事件总线的发布/订阅机制，支持系统内部组件间的异步事件通信。

> **统一总线架构（2026-06-27 迁移完成）**：系统已从双总线（Envelope + ActivityEvent）迁移到统一总线架构（ADR-03 Phase 5 Blocker A-G 全部完成）。
> - **ActivityEventBus**（`biz.ActivityEventBus`）：传输 `ActivityEvent`，承载所有 chat + system 事件。`Domain=chat` 持久化到 Activity 表，`Domain=system` 仅推送 WS。
> - **MonitorEventBus**（`contract.MonitorBus`）：传输 `MonitorEvent`，承载高频监控事件（log/flow_log/mcp/alert）。
> - ~~**Legacy Envelope Bus**（`event.Bus`）~~：已删除（Blocker F/G）。`contract/envelope.go` 已删，活类型 `EnvelopeError`/`EnvelopeTokenUsage` 提取到 `envelope_types.go`。`ProvideSessionBus`/`Infra.SessionBus` 已删，8 个 consumer 全部迁移到 `ActivityEventBus`/`MonitorEventBus`。
>
> 详见 [统一总线架构设计](../superpowers/specs/2026-06-25-unified-bus-architecture-design.md) 和 ADR-03（统一总线架构）。

**代码锚点**：

| 层 | 路径 | 职责 |
|----|------|------|
| contract | ~~`internal/event/contract/envelope.go`~~ | 已删除（Blocker G）：Envelope struct + 60 EnvelopeType 常量 + helper。活类型提取到 `envelope_types.go` |
| contract | `internal/event/contract/envelope_types.go` | ✅ Blocker G 提取：`EnvelopeError` + `EnvelopeTokenUsage` + 5 个 `ErrorCode*` 常量 |
| contract | `internal/event/contract/monitor_event.go` | ✅ MonitorEvent 类型 + MonitorEventType 枚举 + MonitorBus 接口 |
| contract | ~~`internal/event/contract/bus.go`~~ | 已删除（Blocker G）：Bus 接口 + SubscribeOptions + DropPolicy + ChannelPriority |
| contract | ~~`internal/event/contract/reliability.go`~~ | 已删除（Blocker G）：EventReliability 分级（仅被自身测试调用） |
| contract | `internal/event/contract/dedup.go` | ✅ EventDeduplicator 去重器（Blocker F 从 `internal/event/dedup.go` 迁移） |
| event | `internal/event/envelope.go` | contract 活类型的 type alias（`EnvelopeError`/`EnvelopeTokenUsage`/`ErrorCode*`） |
| event | ~~`internal/event/bus.go` + `bus_adapter.go`~~ | 已删除（Blocker G）：NewBus + 框架 bus.Bus 适配 |
| event | ~~`internal/event/buffer.go`~~ | 已删除（Blocker G）：环形缓冲（Blocker A 后无读取者） |
| event | `internal/event/infra.go` | Infra：仅 `MonitorEventBus`（`contract.MonitorBus`）+ InfraProviderSet。~~SessionBus/Publish~~ 已删（Blocker F） |
| event | `internal/event/monitor_bus.go` | ✅ MonitorBus 实现（基于 GenericBus，传输 `contract.MonitorEvent`） |
| event | `internal/event/activityevent/bus.go` | ✅ ActivityEventBus 实现（传输 `biz.ActivityEvent`） |
| event | ~~`internal/event/wal.go` + `wal_storage.go`~~ | 已删除（Phase 1c-2：WAL/WBPF 子系统下线，Critical 事件不再有 crash-recovery 保证，订阅者需幂等） |
| event | ~~`internal/event/framework_adapter.go`~~ | 已删除（Blocker G）：trpc Event → Envelope 投影（Envelope 删除后无调用者） |
| event | `internal/event/flow_log.go` + `flow_tracker.go` + `trace_emitter.go` + `trace_context.go` + `flow_context.go` | Flow Log v2（替代 SlogBridge） |
| event | `internal/event/span_collector.go` + `usage_aggregator.go` | Span 收集与 Usage 汇总 |
| biz | ~~`internal/biz/event_bus_consumer.go`~~ | 已删除（Blocker B）：旧 EventBusConsumer 拆分为 4 typed consumer |
| biz | `internal/biz/event_bus_callback_consumer.go` | Webhook 回调 consumer（订阅 ActivityEventBus） |
| biz | `internal/biz/event_bus_flow_log_consumer.go` | FlowLog 持久化 consumer（订阅 MonitorBus） |
| biz | `internal/biz/event_bus_usage_rollup_consumer.go` | Usage 汇总 consumer（订阅 ActivityEventBus） |
| biz | `internal/biz/event_bus_user_feedback_consumer.go` | 用户反馈 consumer（订阅 ActivityEventBus） |
| biz | `internal/biz/event_persist_handler.go` | 异步持久化 handler |
| biz | `internal/biz/event_bus_side_consumers.go` | EventBusSideConsumers 编排（旁路 typed consumer） |
| biz | `internal/biz/event_bus_tool_call_consumer.go` | ToolCall 记录 consumer |
| biz | `internal/biz/event_bus_message_store_consumer.go` | 消息存储 consumer |
| biz | `internal/biz/event_store.go` | EventStoreUsecase（List / PurgeExpired / Exists） |
| biz | `internal/biz/domain_event.go` | DomainEvent 领域模型（~~`domain_event_adapter.go`~~ 已删除：ADR-03 Blocker C bridge 移除后仅剩 TEST_ONLY 引用，2026-08-14 清理） |
| biz | `internal/biz/session/state.go` + `state_usecase.go` | SessionUsecase.ApplyStateDelta / GetSessionState / SaveSessionState |
| data | `internal/data/session_state_repo.go` | Session State 持久化（json_set / json_remove） |
| data | `internal/data/event_store_repo.go` + `ent/schema/event_store.go` | event_store 表 Repo + Schema |
| server | `internal/server/ws.go` + `ws_*.go` | WebSocket 统一网关（`/v1/ws`）；✅ 统一总线：2 pump（monitorEventPump + activityEventPump），envelope eventPump 已删 |
| server | `internal/server/ws_sync_request.go` | T3.4 sync_request 上行处理 + revision-based 重放（EventStoreLister 窄接口） |
| service | `internal/service/event.go` | 回放 API Service（`GET /v1/events`） |
| proto | `api/kratos/event/v1/event.proto` | EventService.ListEvents Proto 契约 |
| 前端 | `web/src/realtime/ws-transport.ts` + `useV2EventStream.ts` + `useMonitorStream.ts` | ✅ P0-6：typed `v2_event` / `monitor_event` hooks；`useEnvelopeStream` 隔离兼容别名 |
| 前端 | `web/src/realtime/event_replay.ts` | T3.4 RevisionTracker + buildSyncRequest + requestSyncReplay |
| 前端 | `web/src/components/monitor/RealtimeEvents.vue` | Monitor Events Tab |
| 前端 | `web/src/components/chat/SessionTimelineDialog.vue` | Chat Trace + Envelope 双 Tab |
| 前端 | `web/src/features/chat/composables/useChatTraceAndArtifacts.ts` | openSessionTrace / openSessionEvents |

---

## 2. 现状评估

### 2.1 能力清单（已实现）

| # | 能力 | 证据 |
|---|------|------|
| 1 | contract 子包（纯接口与值对象） | `internal/event/contract/` 含 `envelope_types.go`/`monitor_event.go`/`dedup.go` |
| 2 | ~~Envelope 60+ 类型（按 Channel 分组）~~ | 已删除（Blocker G）：EnvelopeType 枚举 + RegisterChannelRoute 随 `contract/envelope.go` 删除 |
| 3 | 双 Bus 隔离（ActivityEventBus / MonitorEventBus） | `internal/event/infra.go`（legacy SessionBus/MonitorBus 已删，Blocker F） |
| 4 | ~~EventWAL（WBPF for Critical events）~~ | 已删除（Phase 1c-2：订阅者需幂等，重放走 Activity 记录） |
| 5 | ~~事件可靠性分级（AS-EVT-01）~~ | 已删除（Blocker G）：`contract/reliability.go` 仅被自身测试调用 |
| 6 | Typed Consumer 拆分（4 typed consumer） | `event_bus_callback_consumer.go`/`flow_log_consumer.go`/`usage_rollup_consumer.go`/`user_feedback_consumer.go`（Blocker B 从旧 `event_bus_consumer.go` 拆分） |
| 7 | EventBusSideConsumers 旁路 typed consumer | `event_bus_side_consumers.go` + typed consumer 文件 |
| 8 | DomainEvent 适配（biz 领域事件双向桥接） | `domain_event.go`（~~`domain_event_adapter.go`~~ 已删除 2026-08-14） |
| 9 | Session State Delta（ApplyStateDelta） | `session/state_usecase.go` + `data/session_state_repo.go` |
| 10 | ~~事件缓冲与重放（Buffer + Replay）~~ | 已删除（Blocker G）：`buffer.go` 环形缓冲在 Blocker A 后无读取者 |
| 11 | WebSocket 传输（`/v1/ws`，2 pump） | `internal/server/ws.go` + `ws_*.go`（monitorEventPump + activityEventPump） |
| 12 | 前端架构（WsTransport + useEnvelopeStream） | `web/src/realtime/ws-transport.ts` + `useEnvelopeStream.ts`（🟡 待迁移到 ActivityEvent） |
| 13 | ~~事件持久化（event_store + 异步 persist + TTL）~~ | 已删除（ADR-02）：`event_store` 表 DROP，由 `activities` 表 + 并行异步持久化替代；`event_persist_handler.go`/`event_store_repo.go`/`event_store.go` 已删 |
| 14 | Chat 会话事件检视（Dialog 双 Tab） | `SessionTimelineDialog.vue` + Inspector 组件群 |
| 15 | Flow Log v2（替代 SlogBridge） | `flow_log.go` + `flow_tracker.go` + `trace_emitter.go` |
| 16 | Monitor 实时事件 UI | `web/src/components/monitor/RealtimeEvents.vue` |
| 17 | ~~事件回放 API（`GET /v1/events`）~~ | 已删除（ADR-03 Blocker A）：replay 改用 `ListActivities` RPC（API Backfill） |
| 18 | ~~事件丢弃可观测（Prometheus）~~ | 已删除（Blocker G）：`bus_adapter.go` DropLogger 随 `bus.go` 删除；MonitorBus 丢弃通过 `MonitorBusDropCount` 暴露 |
| 19 | Graph 事件桥接 | `internal/graph/trpc/event_bridge.go` |
| 20 | ~~T3.4 revision-based sync replay（sync_request 上行 + EventStore 重放）~~ | 已删除（ADR-03 Blocker A）：`ws_sync_request.go`/`event_replay.ts` 删除，重连改用 `ListActivities` RPC（API Backfill） |
| 21 | API Backfill（WS 重连恢复） | ✅ 现行：前端调用 `ListActivities` RPC 拉取持久化 Activity（替代 Buffer replay + sync_request） |
| 22 | Activity-First 并行异步持久化 | ✅ 现行（ADR-02）：`activity_event_sequencer.go` processTask + persistChan + worker goroutine + retry + dead-letter |
| 23 | Dead-letter 环形缓冲 | ✅ 现行（ADR-02）：容量 512，FIFO 淘汰，activityID 去重，`ListDeadLetterActivities` RPC 暴露 |

### 2.2 能力清单（未实现）

| # | 能力 | 缺口 | 对应需求 |
|---|------|------|---------|
| 1 | 工具生命周期事件与自动触发 | ActivityEventKind 无 `ToolRegistered`/`ToolUpdated`/`ToolRemoved`；无自动触发链 | 需求 §1.10 |

### 2.3 状态总览

| 项 | 状态 | 证据 |
|----|------|------|
| 后端 Event 核心 | ✅ | ActivityEventBus / MonitorEventBus / WS / StateDelta / Activity 表持久化（legacy Envelope Bus / event_store 已删） |
| Monitor 实时事件 UI | ✅ | `RealtimeEvents.vue` |
| Chat 会话事件检视 | ✅ | `SessionTimelineDialog` 双 Tab + Inspector 组件群 |
| Monitor EventTimeline 原型 | ✅ | 已删除（O1） |
| ~~T3.4 revision-based sync replay~~ | ✅ 已删除 | ADR-03 Blocker A：`ws_sync_request.go`/`event_replay.ts` 删除，改用 `ListActivities` RPC（API Backfill） |
| 统一总线迁移 | ✅ | ADR-03 Phase 5 Blocker A-G 全部完成 |
| Activity-First 持久化 | ✅ | ADR-02：并行异步 + 三重补偿（retry + dead-letter + API Backfill） |
| 工具生命周期事件 | ❌ | ActivityEventKind 无 ToolRegistered/ToolUpdated/ToolRemoved |
| 前端 Envelope → ActivityEvent 迁移 | ✅ | Envelope import 从 46 降至 7（残留为合法传输层/事件检查器，依赖后续清理） |

---

## 3. 差距与优化

### 3.1 功能差距

1. ~~**P1** 事件推流增强（StateDelta / Extensions / FilterKey / Branch / Tag / Actions）~~ ✅
2. ~~**P2** 事件持久化 + 回放 API~~ ✅
3. ~~**P3** Chat 会话事件检视（Dialog 双 Tab）~~ ✅
4. **P4** 工具生命周期事件与自动触发（ToolRegistered / ToolUpdated / ToolRemoved）— 见需求 §1.10

### 3.2 优化项

| # | 状态 | 项 |
|---|------|-----|
| O1 | ✅ | 删除未挂载 `monitor/EventTimeline.vue` |
| O4 | ✅ | domain_event_adapter 丢弃 SysLogWarn（文件已删除 2026-08-14） |
| O5 | ✅ | event_persist_handler 独立 SRP |
| O7 | ✅ | Chat Inspector vs Monitor 分工已定（设计 §13.1） |
| O8 | ✅ | ListEvents 会话存在性校验（SessionUsecase.Get） |
| O9 | ✅ | Inspector 复用 chat/team WS（subscribeSessionStream） |
| O10 | ✅ | 持久化有界队列 + 排除 text_delta/member_delta |
| O11 | ✅ | FilterKey 过滤 UI（EventFilterBar） |
| O12 | ✅ | createEventService + event_test 集成测试 |
| O13 | ✅ | contract 子包抽取（biz 仅 import contract，禁止 import 父 event） |
| O14 | ✅→🔄 | 双 Bus 隔离（~~SessionBus / MonitorBus~~ → ActivityEventBus / MonitorEventBus，`MONITOR_BUS_ROUTING` 已删；legacy Bus 已删 Blocker F/G） |
| O15 | ✅→❌ | ~~EventWAL 实现（WBPF for Critical events）~~（Phase 1c-2 下线，订阅者需幂等） |
| O16 | ✅ | Flow Log v2 替代 SlogBridge（`slog_bridge.go` 已删除） |
| O17 | ✅ | EventBusSideConsumers 旁路 typed consumer 拆分 |
| O18 | ✅ | 统一总线迁移 ADR-03 Phase 5 Blocker A-G（legacy Envelope Bus / SessionBus / MonitorBus 全部删除，8 consumer 迁移到 ActivityEventBus/MonitorEventBus） |

---

## 4. 开发阶段

### Phase 1：事件推流增强（P1）— ✅ 完成

Envelope 元数据扩展（StateDelta / Extensions / FilterKey / Branch / Tag / Actions）+ Buffer + WebSocket 传输 + StateDelta 应用。

### Phase 2：事件持久化（P2）— ✅ 完成

`event_store` 表 + 异步 `eventPersistHandler` + `GET /v1/events` 回放 API + TTL 清理 cron。

### Phase 3：Chat 会话事件检视（P3）— ✅ 完成

**方案**：扩展 `SessionTimelineDialog` 为双 Tab（Trace | Envelope），**不**新增第四列侧边栏。

**任务**：
1. ~~删除 `web/src/components/monitor/EventTimeline.vue`（O1）~~ ✅
2. ~~`web/src/features/event/api.ts` — `listSessionEvents`~~ ✅
3. ~~`web/src/features/chat/eventFilter.ts` — filterEnvelopes / buildBranchTree~~ ✅
4. ~~`web/src/features/chat/composables/useEventFilter.ts`~~ ✅
5. ~~`web/src/features/chat/composables/useChatEventInspector.ts`~~ ✅
6. ~~`components/chat/` — EventFilterBar / BranchTree / StateDeltaIndicator / TransferBadge / SessionEventInspectorPanel~~ ✅
7. ~~扩展 `SessionTimelineDialog` — q-tabs + `initialTab` prop~~ ✅
8. ~~`ChatMessagePanel` — 「事件」按钮 → 打开 Envelope Tab~~ ✅
9. ~~`useChatWorkspace` — `openSessionTrace(id, tab?)`~~ ✅

### Phase 4：工具生命周期事件与自动触发（P4）— ❌ 未实现

> 来源：BabyAGI Triggers 机制，竞品分析差距 #8。见需求 §1.10。

**任务**：
1. 增加 `ToolRegistered` / `ToolUpdated` / `ToolRemoved` 三种 ActivityEventKind
2. `ToolRegistered` 事件触发 LLM 自动生成工具描述和 embedding
3. `ToolUpdated` 事件触发 `BuildTRPCAgentCached` 缓存失效
4. `ToolRemoved` 事件触发依赖该工具的 Agent 配置告警
5. 所有触发操作经 broker/async 异步执行
6. 触发结果记录到 FlowLog

### Phase 5：revision-based sync replay（T3.4）— ✅ 完成

> 来源：`docs/reports/2026-06-18-review-issues-and-solutions.md` T3.4 — WS 断连事件持久化 + afterRevision 重放。

**背景**：原有 event-ID replay 基于 `event.Buffer` 环形缓冲（内存，容量 256），断连时间超过 Buffer 容量时丢失事件。T3.4 引入 revision-based sync：客户端重连后发送 `sync_request { after_revision }`，服务端从 `event_store` 表查询并重放 `session_revision > after_revision` 的 Envelope。

**任务**：
1. ~~后端 `internal/server/ws_sync_request.go` — `handleSyncRequest` + `runSyncReplay`~~ ✅
2. ~~后端 `internal/server/ws.go` — `EventStoreLister` 窄接口 + nil-interface 陷阱处理~~ ✅
3. ~~前端 `web/src/realtime/event_replay.ts` — `RevisionTracker` + `buildSyncRequest` + `requestSyncReplay`~~ ✅
4. ~~前端 `web/src/realtime/ws-transport.ts` — 集成 `RevisionTracker`，重连后调用 `requestSyncReplay`~~ ✅
5. ~~E2E 测试 `internal/server/ws_sync_e2e_test.go` — 覆盖正常重放 + 边界条件~~ ✅
6. ~~aranea-review 通过（0 blocking / 0 suggestions）~~ ✅

**关键设计决策**：
- 同步阶段（校验）在 readPump goroutine 中执行，异步阶段（EventStore 查询 + 重放）通过 `safego.Go` 启动独立 goroutine，避免阻塞 readPump
- Context 来源为 `wc.contextOrBackground()`（连接关闭时取消），而非 `context.Background()`
- 重放前按 `SessionRevision` 升序排序（INV4），保证因果顺序
- 仅重放当前连接已订阅的 Channel 的事件
- EventStore 未配置时静默降级到 event-ID replay

---

## 5. 任务清单

| # | 任务 | Phase | 状态 |
|---|------|-------|------|
| 1 | Envelope 元数据扩展（StateDelta / Extensions / FilterKey / Branch / Tag / Actions） | 1 | ✅ |
| 2 | ~~Buffer 环形缓冲 + Replay~~ | 1 | ✅→🗑️（Blocker G 删除，Blocker A 后无读取者） |
| 3 | WebSocket 传输（`/v1/ws`，2 pump） | 1 | ✅ |
| 4 | StateDelta 应用（ApplyStateDelta） | 1 | ✅ |
| 5 | contract 子包抽取 | 1 | ✅ |
| 6 | 双 Bus 隔离（~~SessionBus / MonitorBus~~ → ActivityEventBus / MonitorEventBus） | 1 | ✅→🔄（Blocker F 迁移完成） |
| 7 | ~~EventWAL（WBPF for Critical events）~~ | 1 | ✅→🗑️（Phase 1c-2 下线） |
| 8 | ~~P2 持久化 / API / TTL（event_store）~~ | 2 | ✅→🗑️（ADR-02：event_store 表 DROP，由 Activity 表 + 并行异步持久化替代） |
| 9 | 删除 Monitor EventTimeline 原型 | 3 | ✅ |
| 10 | useEventFilter + eventFilter.ts | 3 | ✅ |
| 11 | StateDeltaIndicator / TransferBadge | 3 | ✅ |
| 12 | BranchTree | 3 | ✅ |
| 13 | SessionEventInspectorPanel + Dialog Tab | 3 | ✅ |
| 14 | ChatMessagePanel 入口 | 3 | ✅ |
| 15 | Flow Log v2 替代 SlogBridge | 3 | ✅ |
| 16 | EventBusSideConsumers 旁路 typed consumer | 3 | ✅ |
| 17 | ToolRegistered / ToolUpdated / ToolRemoved ActivityEventKind | 4 | ❌ |
| 18 | ToolRegistered → 自动生成描述 + embedding | 4 | ❌ |
| 19 | ToolUpdated → 缓存失效 | 4 | ❌ |
| 20 | ToolRemoved → Agent 配置告警 | 4 | ❌ |
| 21 | 异步触发 + FlowLog 记录 | 4 | ❌ |
| 22 | ~~T3.4 后端 sync_request 处理（ws_sync_request.go + EventStoreLister 窄接口）~~ | 5 | ✅→🗑️（ADR-03 Blocker A：sync_request 删除，改用 API Backfill） |
| 23 | ~~T3.4 前端 RevisionTracker + requestSyncReplay（event_replay.ts + ws-transport.ts 集成）~~ | 5 | ✅→🗑️（ADR-03 Blocker A：event_replay.ts 删除） |
| 24 | ~~T3.4 E2E 测试（ws_sync_e2e_test.go）~~ | 5 | ✅→🗑️（随 sync_request 一起删除） |
| 25 | ADR-03 Blocker A：删除 WS replay Buffer 路径 | 5 | ✅ |
| 26 | ADR-03 Blocker B：side consumer 迁移到 ActivityEventBus/MonitorBus | 5 | ✅ |
| 27 | ADR-03 Blocker C：DomainEvent bridge 迁移 | 5 | ✅ |
| 28 | ADR-03 Blocker D：删除 SessionBus 死发布者 | 5 | ✅ |
| 29 | ADR-03 Blocker E：删除 Buffer 字段 + 死写入 | 5 | ✅ |
| 30 | ADR-03 Blocker F：删除 ProvideSessionBus + Infra.SessionBus + 8 consumer 迁移 | 5 | ✅ |
| 31 | ADR-03 Blocker G：提取活类型 + 删除 contract/envelope.go 死代码 | 5 | ✅ |
| 32 | 前端 Envelope → ActivityEvent 类型迁移（46→7 处残留） | 6 | ✅ |
| 33 | ADR-02 D1：Activity-First 并行异步持久化（processTask + persistChan + worker） | 6 | ✅ |
| 34 | ADR-02 D2：三重补偿（retry 5 次 + dead-letter 512 + API Backfill） | 6 | ✅ |
| 35 | ADR-02 D3：OnError 语义重构（删除 ActivityKindError，task.failed 统一） | 6 | ✅ |
| 36 | ADR-02 D4：Legacy Kind 清理（sub_task_board/error/delegate） | 6 | ✅ |
| 37 | 前端 Envelope 残留清理（7 处合法传输层 import） | 6 | ❌ |

---

## 6. 验收标准

- [x] 事件推流包含完整事件元数据（StateDelta / Extensions / FilterKey / Branch / Tag / Actions）
- [x] StateDelta 正确应用到 Session State（set / append / delete 三种操作）
- [x] 前端可按层级过滤事件流（FilterKey 前缀匹配）
- [x] 多 Agent 场景中可追踪执行链（Branch + InvocationID / ParentInvocationID）
- [x] 事件可携带自定义扩展元数据（Extensions 命名空间化）
- [x] Runner 正确处理 Actions 提示（SkipSummarization）
- [x] 系统重启后可查询历史 Activity 事件（`Domain=chat` 持久化到 `activities` 表，`Domain=system` 不持久化）
- [x] WS 重连后通过 `ListActivities` RPC（API Backfill）恢复一致视图，无需服务端 replay Buffer
- [x] Chat 会话事件检视：Drawer/Dialog 双 Tab（Trace + 实时 ActivityEvent），支持 kind/event/agent 过滤与 Activity 树
- [x] ~~关键事件 WBPF（先写后发），进程崩溃不丢失~~ → **并行异步持久化 + 三重补偿**：`Domain=chat` Activity 事件持久化与推送解耦，失败通过重试 5 次（100/200/400/800/1600ms）+ dead-letter 环形缓冲（容量 512，FIFO，activityID 去重）+ API Backfill 保证最终一致
- [x] 订阅者幂等：所有 typed consumer 重复投递无副作用
- [x] Monitor 高频事件与 Chat 业务事件走独立 Bus（ActivityEventBus + MonitorEventBus），避免相互挤压
- [x] ~~T3.4 WS 断连重连后 sync_request + EventStore 重放~~ → **已删除**（ADR-03 Blocker A：改用 `ListActivities` RPC，无需服务端维护 replay Buffer）
- [ ] 新工具注册后自动生成描述和 embedding
- [ ] 工具更新后相关 Agent 缓存自动失效
- [ ] 触发操作异步执行，不阻塞主流程
- [ ] 触发结果在 FlowLog 中可追踪

---

## 7. 依赖与风险

- Chat Inspector 依赖 `ListActivities` RPC 与 WS 同时在线
- 高流量 session 下 Activity 列表需上限（Inspector 保留最近 N 条，`useChatEventInspector` MAX_EVENTS=2000）
- FlowLogger 落库与 Activity 持久化已分流（FlowLog 走 `MonitorEventBus` → `FlowLogPersistConsumer`，Activity 走 `ActivityEventBus` → `ActivityEventSequencer`）
- ~~Critical 事件 WAL 写失败会阻塞 publish~~ → **已删除**（ADR-02：WAL 废弃，并行异步持久化 + 三重补偿替代；persist 失败不阻塞推送）
- ~~双 Bus 路由模式由 `MONITOR_BUS_ROUTING` 控制~~ → **已删除**（ADR-03：2 Bus 固定路由，ActivityEventBus 传输 chat+system，MonitorEventBus 传输 monitor，无路由模式配置）
- P4 工具生命周期事件需与 `internal/biz/tool/` 和 `internal/biz/skill/` 模块协同，触发链需遵守红线 #8（框架 plugin 回调不得直接写数据库）
- persist worker 单线程瓶颈：per-activity FIFO 要求串行处理，高并发 turn 可能成为瓶颈，需监控 `persistChan` 满载率
- Dead-letter 容量有限：512 条，极端场景（DB 长时间不可用）可能丢失部分 persist 失败记录，API Backfill 兜底
- 临时不一致窗口：persist 失败时前端实时状态与 DB 不一致，最长持续到下次 API Backfill（通常 < 5s）
- 前端 Envelope 残留：7 处合法传输层 import，依赖后续前端清理
- `Domain=system` 语义负担：publisher 必须正确声明 Domain，错误声明会导致 system 事件被持久化或 chat 事件被丢弃
