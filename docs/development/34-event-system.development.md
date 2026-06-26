# Event 事件系统 — 开发计划

> **版本**：2026-06-26 | **状态**：🟢 P1 + P2 + P3 + P5(T3.4) 已实现，P4 工具生命周期事件未实现；🔄 统一总线迁移 Phase 1-3 已完成（Phase 4-6 待实施）
> **需求**：[34-event-system.md](./34-event-system.md) · **设计**：[34-event-system.design.md](./34-event-system.design.md)
> **统一总线迁移**：[2026-06-25-unified-bus-architecture-design.md](../superpowers/specs/2026-06-25-unified-bus-architecture-design.md)

---

## 1. 模块定位

Event 事件系统：基于事件总线的发布/订阅机制，支持系统内部组件间的异步事件通信。

> **统一总线架构（2026-06-26 迁移中）**：系统正在从双总线（Envelope + ActivityEvent）迁移到统一总线架构。
> - **ActivityEventBus**（`biz.ActivityEventBus`）：传输 `ActivityEvent`，承载所有 chat + system 事件。`Domain=chat` 持久化到 Activity 表，`Domain=system` 仅推送 WS。
> - **MonitorBus**（`contract.MonitorBus`）：传输 `MonitorEvent`，承载高频监控事件（log/flow_log/mcp/alert）。
> - **Legacy Envelope Bus**（`event.Bus`）：仅保留给核心基础设施内部使用（framework_adapter、domain_event_adapter、replay buffer），Phase 5 删除。
>
> 详见 [统一总线架构设计](../superpowers/specs/2026-06-25-unified-bus-architecture-design.md)。

**代码锚点**：

| 层 | 路径 | 职责 |
|----|------|------|
| contract | `internal/event/contract/envelope.go` | ⚠️ Legacy Envelope 结构 + EnvelopeType 枚举（Phase 5 删除） |
| contract | `internal/event/contract/monitor_event.go` | ✅ 新增：MonitorEvent 类型 + MonitorEventType 枚举 + MonitorBus 接口 |
| contract | `internal/event/contract/bus.go` | Bus 接口 + SubscribeOptions + DropPolicy + ChannelPriority |
| contract | `internal/event/contract/reliability.go` | EventReliability 分级 + ClassifyEventReliability / IsCriticalWBPFType / RequiresBlockUpTo |
| event | `internal/event/envelope.go` | contract 类型的向后兼容 type alias |
| event | `internal/event/bus.go` + `bus_adapter.go` | NewBus + 框架 bus.Bus 适配（DropLogger） |
| event | `internal/event/buffer.go` | 环形缓冲 + TTL 淘汰 + Replay |
| event | `internal/event/infra.go` | Infra：SessionBus + MonitorBus（legacy envelope）+ ✅ MonitorEventBus（`contract.MonitorBus`）+ Publish（split 路由）+ InfraProviderSet |
| event | `internal/event/monitor_bus.go` | ✅ 新增：MonitorBus 实现（基于 GenericBus，传输 `contract.MonitorEvent`） |
| event | `internal/event/activityevent/bus.go` | ✅ ActivityEventBus 实现（传输 `biz.ActivityEvent`） |
| event | ~~`internal/event/wal.go` + `wal_storage.go`~~ | 已删除（Phase 1c-2：WAL/WBPF 子系统下线，Critical 事件不再有 crash-recovery 保证，订阅者需幂等） |
| event | ~~`internal/event/event_reliability.go`~~ | 已删除（reliability 别名零调用，直接使用 `contract/reliability.go`） |
| event | `internal/event/flow_log.go` + `flow_tracker.go` + `trace_emitter.go` + `trace_context.go` + `flow_context.go` | Flow Log v2（替代 SlogBridge） |
| event | `internal/event/framework_adapter.go` + `framework_events.go` | trpc Event → Envelope 投影 + tee 框架事件 |
| event | `internal/event/span_collector.go` + `usage_aggregator.go` | Span 收集与 Usage 汇总 |
| biz | `internal/biz/event_bus_consumer.go` | EventBusConsumer 编排（核心 4 handler） |
| biz | `internal/biz/event_bus_buffer_handler.go` | 缓冲写入 handler |
| biz | `internal/biz/event_bus_runner_handler.go` | RunnerCompletion handler |
| biz | `internal/biz/event_bus_state_handler.go` | StateDelta handler |
| biz | `internal/biz/event_persist_handler.go` | 异步持久化 handler |
| biz | `internal/biz/event_bus_side_consumers.go` | EventBusSideConsumers 编排（旁路 6 typed consumer） |
| biz | `internal/biz/event_bus_tool_call_consumer.go` | ToolCall 记录 consumer |
| biz | `internal/biz/event_bus_callback_consumer.go` | Webhook 回调 consumer |
| biz | `internal/biz/event_bus_message_store_consumer.go` | 消息存储 consumer |
| biz | `internal/biz/event_bus_flow_log_consumer.go` | FlowLog 持久化 consumer |
| biz | `internal/biz/event_bus_user_feedback_consumer.go` | 用户反馈 consumer |
| biz | `internal/biz/event_bus_usage_rollup_consumer.go` | Usage 汇总 consumer |
| biz | `internal/biz/event_store.go` | EventStoreUsecase（List / PurgeExpired / Exists） |
| biz | `internal/biz/domain_event.go` + `domain_event_adapter.go` | DomainEvent 领域模型 + EventBus 适配 |
| biz | `internal/biz/session/state.go` + `state_usecase.go` | SessionUsecase.ApplyStateDelta / GetSessionState / SaveSessionState |
| data | `internal/data/session_state_repo.go` | Session State 持久化（json_set / json_remove） |
| data | `internal/data/event_store_repo.go` + `ent/schema/event_store.go` | event_store 表 Repo + Schema |
| server | `internal/server/ws.go` + `ws_*.go` | WebSocket 统一网关（`/v1/ws`）；✅ 统一总线：2 pump（monitorEventPump + activityEventPump），删除 envelope eventPump |
| server | `internal/server/ws_sync_request.go` | T3.4 sync_request 上行处理 + revision-based 重放（EventStoreLister 窄接口） |
| service | `internal/service/event.go` | 回放 API Service（`GET /v1/events`） |
| proto | `api/kratos/event/v1/event.proto` | EventService.ListEvents Proto 契约 |
| 前端 | `web/src/realtime/envelope.ts` + `ws-transport.ts` + `useEnvelopeStream.ts` | 前端 Envelope 类型 + WS 传输 + composable |
| 前端 | `web/src/realtime/event_replay.ts` | T3.4 RevisionTracker + buildSyncRequest + requestSyncReplay |
| 前端 | `web/src/components/monitor/RealtimeEvents.vue` | Monitor Events Tab |
| 前端 | `web/src/components/chat/SessionTimelineDialog.vue` | Chat Trace + Envelope 双 Tab |
| 前端 | `web/src/features/chat/composables/useChatTraceAndArtifacts.ts` | openSessionTrace / openSessionEvents |

---

## 2. 现状评估

### 2.1 能力清单（已实现）

| # | 能力 | 证据 |
|---|------|------|
| 1 | contract 子包（纯接口与值对象） | `internal/event/contract/` 三个文件 |
| 2 | Envelope 60+ 类型（按 Channel 分组） | `contract/envelope.go` EnvelopeType 枚举 + RegisterChannelRoute |
| 3 | 双 Bus 隔离（SessionBus / MonitorBus） | `internal/event/infra.go`（split 路由模式，MONITOR_BUS_ROUTING 已移除） |
| 4 | ~~EventWAL（WBPF for Critical events）~~ | 已删除（Phase 1c-2：订阅者需幂等，重放走 Activity 记录） |
| 5 | 事件可靠性分级（AS-EVT-01） | `contract/reliability.go` ClassifyEventReliability |
| 6 | EventBusConsumer 核心 4 handler | `event_bus_consumer.go` + buffer/runner/state/persist handler |
| 7 | EventBusSideConsumers 旁路 6 typed consumer | `event_bus_side_consumers.go` + 6 个 consumer 文件 |
| 8 | DomainEvent 适配（biz 领域事件双向桥接） | `domain_event.go` + `domain_event_adapter.go` |
| 9 | Session State Delta（ApplyStateDelta） | `session/state_usecase.go` + `data/session_state_repo.go` |
| 10 | 事件缓冲与重放（Buffer + Replay） | `internal/event/buffer.go` |
| 11 | WebSocket 传输（`/v1/ws`，6 Channel 路由） | `internal/server/ws.go` + `ws_*.go` |
| 12 | 前端架构（WsTransport + useEnvelopeStream） | `web/src/realtime/ws-transport.ts` + `useEnvelopeStream.ts` |
| 13 | 事件持久化（event_store + 异步 persist + TTL） | `event_store_repo.go` + `event_persist_handler.go` + cron 清理 |
| 14 | Chat 会话事件检视（Dialog 双 Tab） | `SessionTimelineDialog.vue` + Inspector 组件群 |
| 15 | Flow Log v2（替代 SlogBridge） | `flow_log.go` + `flow_tracker.go` + `trace_emitter.go` |
| 16 | Monitor 实时事件 UI | `web/src/components/monitor/RealtimeEvents.vue` |
| 17 | 事件回放 API（`GET /v1/events`） | `internal/service/event.go` + `event.proto` |
| 18 | 事件丢弃可观测（Prometheus） | `bus_adapter.go` DropLogger → `EventBusDropped` 指标 |
| 19 | Graph 事件桥接 | `internal/graph/trpc/event_bridge.go` |
| 20 | T3.4 revision-based sync replay（sync_request 上行 + EventStore 重放） | `internal/server/ws_sync_request.go` + `web/src/realtime/event_replay.ts` |

### 2.2 能力清单（未实现）

| # | 能力 | 缺口 | 对应需求 |
|---|------|------|---------|
| 1 | 工具生命周期事件与自动触发 | EnvelopeType 无 `ToolRegistered`/`ToolUpdated`/`ToolRemoved`；无自动触发链 | 需求 §1.10 |

### 2.3 状态总览

| 项 | 状态 | 证据 |
|----|------|------|
| 后端 Event 核心 | ✅ | Bus / Envelope / WS / StateDelta / event_store / 双 Bus（WAL 已于 Phase 1c-2 移除） |
| Monitor 实时事件 UI | ✅ | `RealtimeEvents.vue` |
| Chat 会话事件检视 | ✅ | `SessionTimelineDialog` 双 Tab + Inspector 组件群 |
| Monitor EventTimeline 原型 | ✅ | 已删除（O1） |
| T3.4 revision-based sync replay | ✅ | `ws_sync_request.go` + `event_replay.ts` + E2E 测试 |
| 工具生命周期事件 | ❌ | EnvelopeType 无 ToolRegistered/ToolUpdated/ToolRemoved |

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
| O4 | ✅ | domain_event_adapter 丢弃 SysLogWarn |
| O5 | ✅ | event_persist_handler 独立 SRP |
| O7 | ✅ | Chat Inspector vs Monitor 分工已定（设计 §13.1） |
| O8 | ✅ | ListEvents 会话存在性校验（SessionUsecase.Get） |
| O9 | ✅ | Inspector 复用 chat/team WS（subscribeSessionStream） |
| O10 | ✅ | 持久化有界队列 + 排除 text_delta/member_delta |
| O11 | ✅ | FilterKey 过滤 UI（EventFilterBar） |
| O12 | ✅ | createEventService + event_test 集成测试 |
| O13 | ✅ | contract 子包抽取（biz 仅 import contract，禁止 import 父 event） |
| O14 | ✅ | 双 Bus 隔离（SessionBus / MonitorBus，`MONITOR_BUS_ROUTING`） |
| O15 | ✅ | EventWAL 实现（WBPF for Critical events） |
| O16 | ✅ | Flow Log v2 替代 SlogBridge（`slog_bridge.go` 已删除） |
| O17 | ✅ | EventBusSideConsumers 旁路 6 typed consumer 拆分 |

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
1. 增加 `ToolRegistered` / `ToolUpdated` / `ToolRemoved` 三种 EnvelopeType
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
| 2 | Buffer 环形缓冲 + Replay | 1 | ✅ |
| 3 | WebSocket 传输（`/v1/ws`，6 Channel 路由） | 1 | ✅ |
| 4 | StateDelta 应用（ApplyStateDelta） | 1 | ✅ |
| 5 | contract 子包抽取 | 1 | ✅ |
| 6 | 双 Bus 隔离（SessionBus / MonitorBus） | 1 | ✅ |
| 7 | EventWAL（WBPF for Critical events） | 1 | ✅ |
| 8 | P2 持久化 / API / TTL | 2 | ✅ |
| 9 | 删除 Monitor EventTimeline 原型 | 3 | ✅ |
| 10 | useEventFilter + eventFilter.ts | 3 | ✅ |
| 11 | StateDeltaIndicator / TransferBadge | 3 | ✅ |
| 12 | BranchTree | 3 | ✅ |
| 13 | SessionEventInspectorPanel + Dialog Tab | 3 | ✅ |
| 14 | ChatMessagePanel 入口 | 3 | ✅ |
| 15 | Flow Log v2 替代 SlogBridge | 3 | ✅ |
| 16 | EventBusSideConsumers 旁路 6 typed consumer | 3 | ✅ |
| 17 | ToolRegistered / ToolUpdated / ToolRemoved EnvelopeType | 4 | ❌ |
| 18 | ToolRegistered → 自动生成描述 + embedding | 4 | ❌ |
| 19 | ToolUpdated → 缓存失效 | 4 | ❌ |
| 20 | ToolRemoved → Agent 配置告警 | 4 | ❌ |
| 21 | 异步触发 + FlowLog 记录 | 4 | ❌ |
| 22 | T3.4 后端 sync_request 处理（ws_sync_request.go + EventStoreLister 窄接口） | 5 | ✅ |
| 23 | T3.4 前端 RevisionTracker + requestSyncReplay（event_replay.ts + ws-transport.ts 集成） | 5 | ✅ |
| 24 | T3.4 E2E 测试（ws_sync_e2e_test.go） | 5 | ✅ |

---

## 6. 验收标准

- [x] 事件推流包含完整事件元数据（StateDelta / Extensions / FilterKey / Branch / Tag / Actions）
- [x] StateDelta 正确应用到 Session State（set / append / delete 三种操作）
- [x] 前端可按层级过滤事件流（FilterKey 前缀匹配）
- [x] 多 Agent 场景中可追踪执行链（Branch + InvocationID / ParentInvocationID）
- [x] 事件可携带自定义扩展元数据（Extensions 命名空间化）
- [x] Runner 正确处理 Actions 提示（SkipSummarization）
- [x] 系统重启后可查询历史事件
- [x] 可按时间范围回放事件
- [x] Chat 会话事件检视：Drawer/Dialog 双 Tab（Trace + 实时 Envelope），支持类型/分支/标签过滤与 Branch 树
- [x] 关键事件 WBPF（先写后发），进程崩溃不丢失
- [x] Monitor 高频事件与 Chat 业务事件走独立 Bus，避免相互挤压
- [x] T3.4 WS 断连重连后，客户端发送 `sync_request { after_revision }`，服务端从 EventStore 重放 `session_revision > after_revision` 的 Envelope
- [x] T3.4 重放按 `SessionRevision` 升序排序，仅投递当前连接已订阅 Channel 的事件
- [x] T3.4 EventStore 未配置时静默降级到 event-ID replay，不阻断连接
- [x] T3.4 sync_request 处理不阻塞 readPump（EventStore 查询通过 `safego.Go` 异步执行）
- [ ] 新工具注册后自动生成描述和 embedding
- [ ] 工具更新后相关 Agent 缓存自动失效
- [ ] 触发操作异步执行，不阻塞主流程
- [ ] 触发结果在 FlowLog 中可追踪

---

## 7. 依赖与风险

- Chat Inspector 依赖 P2 `GET /v1/events` 与 WS 同时在线
- 高流量 session 下 Envelope 列表需上限（Inspector 保留最近 N 条，`useChatEventInspector` MAX_EVENTS=2000）
- FlowLogger 落库与 event_store 已分流（exclude flow_log）
- Critical 事件 WAL 写失败会阻塞 publish（避免不一致），需监控 WAL 写延迟
- 双 Bus 路由模式由 `MONITOR_BUS_ROUTING` 控制，`split` 模式下若误配会导致事件丢失
- P4 工具生命周期事件需与 `internal/biz/tool/` 和 `internal/biz/skill/` 模块协同，触发链需遵守红线 #8（框架 plugin 回调不得直接写数据库）
- T3.4 sync_request 依赖 EventStore 持久化（P2），EventStore 未配置时降级到 event-ID replay；24h 回溯窗口外的历史事件不重放（客户端应在初次加载时通过 `GET /v1/events` 补齐）
- T3.4 单连接 sync_request 重放上限 500 条，超过的会话需客户端分页拉取
