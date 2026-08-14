# Message 消息 — 开发计划

> **版本**：2026-06-26 | **状态**：✅ ADR-02 + ADR-03 全量落地（Phase 5 Blocker A-G 全部完成）
> **需求**：[51 消息机制](./51-message-mechanism.md) | **设计**：[51-message-mechanism.design.md](./51-message-mechanism.design.md)
> **架构变更依据**：
> - ADR-02 Activity-First 事件持久化（已归档，设计内容已并入本文档）
> - ADR-03 统一总线架构（已归档，设计内容已并入本文档）
> - Chat 模块重构方案（已归档，设计内容已并入本文档）
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Message 消息：基于 **Activity-First（AF）架构** 的统一事件模型与传输机制。以 **单一 Activity 模型 + 双 Bus（ActivityEventBus + MonitorEventBus）** 为核心，WebSocket（`/v1/ws`）为 Chat / Team / Graph / Monitor 的实时主传输通道。历史 Chat SSE 路由已彻底移除，不得作为新功能入口。

**架构演进**（三阶段）：

| 阶段 | 模型 | 状态 |
|------|------|------|
| Legacy | Envelope + SessionBus + MonitorBus + ActivityBus（3 Bus 并存） | ❌ 已废弃 |
| AF v1（双总线期） | Activity + Envelope 并存，ActivityProjector 双发布 | ❌ 已废弃 |
| **AF v2（当前）** | **单一 Activity + 双 Bus（ActivityEventBus + MonitorEventBus）+ 并行异步持久化** | ✅ 已落地 |

---

## 2. 代码锚点

### 2.1 后端代码锚点（Live）

| 层 | 文件 | 职责 |
|----|------|------|
| Activity 类型 | `internal/biz/activity.go` | 10 种 ActivityKind 枚举 + Activity struct |
| Activity 事件 | `internal/biz/activity_event.go` | 7 种 ActivityEventType + ActivityDomain（chat/system）+ ActivityEvent struct + IsActivityTerminal |
| 状态机 | `internal/biz/activity_state_machine.go` | 9 种 ActivityStatus + 状态转换表 + 转换校验 |
| LLM 上下文 | `internal/biz/llm_context_builder.go` | 从 Activity 表构建 LLM 上下文（替代 Message 查询） |
| Activity Repo | `internal/data/activity_repo.go` | Activity 表 CRUD（替代 message_repo.go） |
| Activity 回填 | `internal/data/activity_backfill_migrate.go` | messages → activities 数据迁移 |
| ActivityEventBus | `internal/event/activityevent/bus.go` | ActivityEventBus 实现（Publish + Subscribe + per-activity FIFO） |
| Monitor 事件契约 | `internal/event/contract/monitor_event.go` | MonitorEvent 类型 + MonitorBus 接口 + MonitorSubscribeOptions |
| Envelope 活类型 | `internal/event/contract/envelope_types.go` | EnvelopeError + EnvelopeTokenUsage + 5 个 ErrorCode 常量（活类型提取） |
| 去重工具 | `internal/event/contract/dedup.go` | activityID 去重 |
| Infra 容器 | `internal/event/infra.go` | `Infra` struct（仅 MonitorEventBus + lg，SessionBus/MonitorBus 已删除） |
| Activity 投影 | `internal/agent/activity_projector.go` | trpc event.Event → Activity 投影 + OnError 重构 + OnTurnEnd 终态保护 + EmitSystemEvent |
| Activity Meta | `internal/agent/activity_meta.go` + `activity_meta_resolver.go` | ProjectMeta（SpiritSessionID/ParentSessionID/RootSessionID） |
| Activity 序列化 | `internal/agent/activity_event_sequencer.go` | processTask 并行异步 + persist worker + retry + dead-letter |
| Flow Tracker | `internal/event/flow_tracker.go` | FlowLog v2（MonitorEvent 发布到 MonitorEventBus） |
| Flow Context | `internal/event/flow_context.go` | TraceEmitter ctx 传播（FlowLogger 别名已删除；进程日志用 loggateway.Logger） |
| Trace Emitter | `internal/event/trace_emitter.go` | TraceEmitter（不再持有 Bus 参数，emit 对 nil infra 安全） |
| Flow Log | `internal/event/flow_log.go` | FlowLog v2 持久化 |
| Session Revision | `internal/event/session_revision.go` | SessionRevisionBumper 接口 + BumpSessionRevision（仅 bump 半边，publish 已删除） |
| Side Consumers | `internal/biz/event_bus_side_consumers.go` | 编排器：启动/停止 4 个 typed consumer + monitor 文件 appender |
| Callback Consumer | `internal/biz/event_bus_callback_consumer.go` | ActivityEventBus 订阅 → WebhookDispatcher |
| FlowLog Persist | `internal/biz/event_bus_flow_log_consumer.go` | MonitorEventBus 订阅 flow_log → 持久化 |
| User Feedback | `internal/biz/event_bus_user_feedback_consumer.go` | ActivityEventBus 订阅 user_feedback |
| Usage Rollup | `internal/biz/event_bus_usage_rollup_consumer.go` | ActivityEventBus 订阅 token_usage → 用量汇总 |
| Async Helper | `internal/biz/event_bus_async.go` | 异步订阅辅助 |
| WS 服务 | `internal/server/ws.go` + `ws_conn.go` + `ws_conn_manager.go` + `ws_codec.go` + `ws_message_handler.go` + `ws_io_pump.go` + `ws_event.go` + `ws_priority.go` | WSServer（2 pump：activityEventPump + monitorEventPump，挂入 Kratos HTTP） |
| HTTP 后台入口 | `internal/service/chat.go` | `SendChatMessage` unary 入口；WS 上行、Channel、Cron 复用同一 native turn |
| Channel Ingress | `internal/service/channel_ingress.go` | Channel 入站（无 eventBus 字段，已清理） |
| Turn Preview | `internal/service/channel_turn_preview.go` | Turn 预览（无 bus 字段，订阅逻辑已删除） |
| Turn Helpers | `internal/agent/turn_helpers.go` | ConsumeEventStream（无 eventBus 参数） |
| Turn Stream | `internal/agent/turn_stream_helpers.go` | ConsumeWithFirstByteGuard（无 bus 参数） |
| Cron Runner | `internal/cronrunner/runner.go` | Cron 运行器（无 EventBus 字段） |
| Wire DI | `cmd/admin/wire.go` | InfraProviderSet + biz ProviderSet（无 ProvideSessionBus / ProvideMonitorBus） |

### 2.2 前端代码锚点（Live）

| 层 | 文件 | 职责 |
|----|------|------|
| 前端传输 | `web/src/realtime/ws-transport.ts` | createWsTransport（心跳/重连/pending 队列） |
| 前端 Activity 类型 | `web/src/realtime/activityEvent.ts` | ActivityEvent TypeScript 类型定义 |
| 前端 Monitor 类型 | `web/src/realtime/monitorEvent.ts` | MonitorEvent TypeScript 类型定义 |
| 前端 Hooks | `web/src/realtime/useEnvelopeStream.ts` | useChatStream / useTeamStream / useMonitorStream（保留命名，内部消费 ActivityEvent） |
| 前端 Monitor Hooks | `web/src/realtime/useMonitorStream.ts` | Monitor 事件流订阅 |
| 前端全局 Hub | `web/src/realtime/globalWsHub.ts` | 全局 WS 连接管理 |
| 前端命令通道 | `web/src/realtime/command_channel.ts` | 上行命令（cancel/enqueue/subscribe/enable_log） |
| 前端 Graph 状态 | `web/src/realtime/graphState.ts` | Graph 节点状态聚合 |
| 前端超时模型 | `web/src/realtime/timeout_model.ts` | 超时/重连策略 |
| 前端死信查询 | `web/src/features/chat/useTaskDeadLetters.ts` | ListDeadLetterActivities RPC 调用 |
| 前端后台任务 | `web/src/features/chat/useChatBackgroundJobs.ts` | Chat 后台任务（API Backfill 触发） |
| 前端滚动/标题 | `web/src/features/chat/useChatScrollTitle.ts` | UI 辅助 |

### 2.3 已删除文件清单（✅ Deleted — ADR-02/ADR-03）

| 文件 | 删除原因 |
|------|---------|
| `internal/event/contract/envelope.go` | ADR-03 Blocker G：活类型已提取到 `envelope_types.go`，死代码删除 |
| `internal/event/contract/envelope_test.go` | ADR-03 Blocker G：契约测试随 envelope.go 一起删除 |
| `internal/event/contract/envelope_contract_test.go` | ADR-03 Blocker G：契约测试随 envelope.go 一起删除 |
| `internal/event/contract/reliability.go` | ADR-02 D1：WBPF 模式废弃，并行异步持久化替代 |
| `internal/event/contract/reliability_test.go` | ADR-02 D1：随 reliability.go 一起删除 |
| `internal/event/contract/bus.go` | ADR-03 Blocker G：`contract.Bus` 接口删除（双 Bus 已无 Envelope 载体） |
| `internal/event/contract/bus_adapter.go` | ADR-03 Blocker G：busAdapter 删除 |
| `internal/event/buffer.go` | ADR-03 Blocker A/G：WS replay Buffer 死代码，重连改用 ListActivities RPC |
| `internal/event/bus.go` | ADR-03 Blocker G：`event.Bus` 接口删除 |
| `internal/event/bus_adapter.go` | ADR-03 Blocker G：busAdapter 删除 |
| `internal/event/framework_adapter.go` | ADR-03 Blocker G：FromFrameworkEvent 删除 |
| `internal/event/event_projector.go`（legacy） | ADR-02/03：EventProjector 由 ActivityProjector 替代 |
| `internal/event/activity_publish.go` | ADR-02：由 activity_event_sequencer.go 替代 |
| `internal/event/activity_persist.go` | ADR-02：由 activity_event_sequencer.go 替代 |
| `internal/event/event_persist_handler.go` | ADR-02：EventStore 持久化废弃 |
| `internal/event/event_store.go` | ADR-02：EventStore 表废弃，由 Activity 表替代 |
| `internal/event/wal.go` | ADR-02 D1：WAL 废弃，并行异步 + dead-letter + API Backfill 替代 |
| `internal/event/step_id.go` | ADR-03 Blocker D：TraceEmitter.EmitProgress 死发布者删除 |
| `internal/event/session_revision_publish.go` | ADR-03 Blocker D：PublishSessionRevisionEnvelope 死发布者删除 |
| `internal/event/deco_session_sync_test.go` | ADR-03 Blocker D：验证已死 envelope→web sync 路径的测试 |
| `internal/biz/event_bus_consumer.go` | ADR-03：EventBusConsumer 编排器拆分为 typed consumer |
| `internal/biz/event_bus_buffer_handler.go` | ADR-03 Blocker A：buffer/runner/state/persist 四 handler 删除 |
| `internal/biz/event_bus_runner_handler.go` | ADR-03 Blocker A：同上 |
| `internal/biz/event_bus_state_handler.go` | ADR-03 Blocker A：同上 |
| `internal/biz/event_bus_message_store_consumer.go` | ADR-03：MessageStoreConsumer 删除（messages 表 DROP） |
| `internal/biz/domain_event_adapter.go` | ADR-03 Blocker C：DomainEvent bridge 删除，迁移到 ActivityEventBus + ActivityDomainSystem |
| `internal/data/message_repo.go` | ADR-02：messages 表 DROP，由 activity_repo.go 替代 |
| `web/src/realtime/envelope.ts` | ADR-03 D6：前端 Envelope 类型删除 |
| `web/src/realtime/dispatcher.ts` | ADR-03 D6：EnvelopeDispatcher 删除 |
| `web/src/realtime/data_channel.ts` | ADR-03 D6：DataChannel 删除 |
| `web/src/realtime/event_replay.ts` | ADR-03 Blocker A：前端 replay 删除，改用 ListActivities RPC |
| `web/src/features/chat/dispatcher.ts` | ADR-03 D6：re-export barrel 删除 |
| `web/src/features/chat/inboundSyncEnvelope.ts` | ADR-03 Blocker D：envelope 入站同步删除 |
| `web/src/features/spirit/TeamPanel.vue` | ADR-02 遗留：legacy spirit 面板删除 |
| `web/src/features/spirit/OrchestrationTimeline.vue` | ADR-02 遗留：legacy spirit 面板删除 |
| `web/src/features/spirit/TaskExecutionPanel.vue` | ADR-02 遗留：legacy spirit 面板删除 |
| `web/src/features/spirit/MemberReadOnlyPanel.vue` | ADR-02 遗留：legacy spirit 面板删除 |

---

## 3. 现状评估

### 3.1 核心机制（✅ 已完成 — AF v2）

| 项 | 状态 | 证据 |
|----|------|------|
| 单一 Activity 事件模型 | ✅ | 10 种 ActivityKind + 7 种 ActivityEventType + ActivityDomain（chat/system） |
| 双 Bus 职责清晰 | ✅ | ActivityEventBus（chat+system 业务事件）+ MonitorEventBus（log/flow_log/mcp/alert） |
| ActivityProjector | ✅ | trpc event.Event → Activity 投影 + EmitSystemEvent（Domain=system 不持久化） |
| 并行异步持久化 | ✅ | processTask：persistChan fire-and-forget + 同步 publish（per-activity FIFO） |
| 持久化失败补偿三重保障 | ✅ | 重试预算（5 次，3100ms）+ dead-letter 环形缓冲（512，FIFO，activityID 去重）+ API Backfill |
| OnError 语义重构 | ✅ | 删除 ActivityKindError，root task → status=failed + Meta.error_*，无 root → 最小化 failed task |
| OnTurnEnd 终态保护 | ✅ | IsActivityTerminal 判定，终态不被覆盖 |
| WebSocket 2-pump | ✅ | activityEventPump + monitorEventPump（删除 envelopeEventPump） |
| WS 双向通信 | ✅ | cancel / user_message / enqueue_message / subscribe / enable_log 上行 |
| WS 重连 = API Backfill | ✅ | 删除 event.Buffer replay，改用 ListActivities RPC |
| 5 个 typed consumer | ✅ | CallbackConsumer / FlowLogPersistConsumer / UserFeedbackConsumer / UsageRollupConsumer（ActivityEventBus）+ FlowLogPersistConsumer（MonitorEventBus） |
| 全局监控模式 | ✅ | session_id=* 跨会话订阅（限 3 连接） |
| 服务端优雅关闭 | ✅ | server_shutdown 系统消息广播 + Close 三阶段（consumers → persistChan → worker） |
| Infra 容器精简 | ✅ | 仅 MonitorEventBus + lg（SessionBus/MonitorBus 删除） |
| Wire DI 清理 | ✅ | ProvideSessionBus / ProvideMonitorBus 删除，InfraProviderSet 精简 |
| 前端类型统一 | ✅ | Envelope import 从 46 降至 7（残留为合法传输层/事件检查器） |
| 前端本地类型解耦 | ✅ | ExecutionProgressMetadata / ActivityUsage / InspectorEvent 切断 Envelope 依赖 |
| Chat SSE 主链路 | ✅ 已移除 | 实时事件统一走 /v1/ws |
| FTS5 全文搜索 | ✅ | `internal/data/sql/message_fts.sql`（保留，搜索 Activity content 字段） |

### 3.2 已完成的迁移工作

| 项 | 优先级 | 说明 |
|----|--------|------|
| ADR-02 D1 并行异步持久化 | P0 | ✅ processTask + persistChan + worker goroutine |
| ADR-02 D2 三重补偿 | P0 | ✅ retry + dead-letter + API Backfill |
| ADR-02 D3 OnError 重构 | P0 | ✅ 删除 ActivityKindError，task.failed 统一表达 |
| ADR-02 D4 Legacy Kind 清理 | P0 | ✅ 删除 sub_task_board/error/delegate |
| ADR-03 D1 Domain 字段 | P0 | ✅ ActivityDomain chat/system |
| ADR-03 D2 MonitorEvent 类型 | P0 | ✅ contract/monitor_event.go |
| ADR-03 D3 Publisher 迁移 | P0 | ✅ 80+ publisher 全部迁移 |
| ADR-03 D4 EmitSystemEvent | P0 | ✅ ActivityProjector 扩展 |
| ADR-03 D5 WSServer 2-pump | P0 | ✅ 删除 envelopeEventPump |
| ADR-03 D6 前端统一 | P0 | ✅ Envelope import 46→7 |
| ADR-03 Phase 5 Blocker A | P0 | ✅ WS Replay 路径（方案 A2：删除 replay 改用 ListActivities RPC） |
| ADR-03 Phase 5 Blocker B | P0 | ✅ 4 个 side consumer 迁移到 ActivityEventBus/MonitorBus |
| ADR-03 Phase 5 Blocker C | P0 | ✅ DomainEvent bridge 迁移 + DomainEventPublisher/Subscriber 接口删除 |
| ADR-03 Phase 5 Blocker D | P0 | ✅ 3 个死发布者删除（EmitProgress / LogError publish / PublishSessionRevisionEnvelope） |
| ADR-03 Phase 5 Blocker E | P0 | ✅ EventPipeline.Bus/Buffer 字段删除 |
| ADR-03 Phase 5 Blocker F | P0 | ✅ Wire DI 清理（Stage 1-3：死参数链 + SelfHealObserver/TraceProjector 迁移 + SessionBus 删除） |
| ADR-03 Phase 5 Blocker G | P0 | ✅ Envelope 文件删除（方案 C：活类型提取 + 死代码删除） |

### 3.3 待开发（按优先级排序）

| 项 | 优先级 | 说明 |
|----|--------|------|
| 消息引用/回复 | P3 | Activity 表 parent_activity_id 已支持，需前端 UI 实现 |
| persist worker 水平扩展 | P3 | 当前单 worker，高并发 turn 可能成为瓶颈，监控 persistChan 满载率后决定 |
| 前端 Envelope 残留清理 | P3 | 7 处合法传输层 import，依赖后续前端清理 |

---

## 4. 差距与优化

### 4.1 P3 — 消息引用/回复

**现状**：Activity 表已支持 `parent_activity_id` 字段（树形嵌套），但前端无引用/回复 UI。

**方案**：
- Activity meta 扩展 `quote_activity_id` 字段
- 前端引用 UI（基于 parent_activity_id 已有能力，仅需 UI 组件）

### 4.2 P3 — persist worker 监控

**现状**：persist worker 单线程，per-activity FIFO 要求串行处理。

**方案**：
- 监控 `persistChan` 满载率（Prometheus 指标）
- 超过阈值告警
- 必要时横向扩展为 per-session worker pool（per-activity FIFO 跨 worker 难以保证，需评估）

### 4.3 P3 — 前端 Envelope 残留清理

**现状**：前端仍有 7 处 `import type { Envelope }`（合法传输层/事件检查器）。

**方案**：
- 评估每处残留是否可替换为本地类型
- 依赖 Phase 5 Blocker A 已完成（WS replay 已删除），残留可逐步清理

---

## 5. 开发阶段

### Phase 0：Legacy 核心机制 ✅ 已完成（已废弃）

| 任务 | 状态 |
|------|------|
| EventBus + Envelope 统一事件模型 | ✅ 已废弃（ADR-03 替代） |
| EventProjector 事件投影 | ✅ 已废弃（ActivityProjector 替代） |
| WebSocket 统一传输 + 多路复用 | ✅ 已废弃（2-pump 替代 3-pump） |
| EventBuffer + WS 重放同步屏障 | ✅ 已废弃（API Backfill 替代） |
| 前端 EnvelopeDispatcher + 场景 Hooks | ✅ 已废弃（ActivityEvent/MonitorEvent 替代） |

### Phase 1：消息搜索 ✅ 已完成

| 任务 | 优先级 | 依赖 | 状态 |
|------|--------|------|------|
| FTS5 虚拟表 + 索引构建 | P2 | — | ✅ |
| SearchMessages RPC | P2 | FTS5 | ✅ |
| 前端搜索组件 | P2 | RPC | ✅ |

### Phase 2：消费者拆分 ✅ 已完成（AF v2 重构）

| 任务 | 优先级 | 依赖 | 状态 |
|------|--------|------|------|
| CallbackConsumer（ActivityEventBus） | P3 | ActivityEventBus | ✅ |
| FlowLogPersistConsumer（MonitorEventBus） | P3 | MonitorEventBus | ✅ |
| UserFeedbackConsumer（ActivityEventBus） | P3 | ActivityEventBus | ✅ |
| UsageRollupConsumer（ActivityEventBus） | P3 | ActivityEventBus | ✅ |
| MessageStoreConsumer 删除 | P3 | messages 表 DROP | ✅ |

### Phase 2.5：session_revision 增量同步 ✅ 已完成（重构后）

| 任务 | 优先级 | 依赖 | 状态 |
|------|--------|------|------|
| SessionRevisionBumper 接口 + 实现 | P0 | — | ✅ |
| BumpSessionRevision（仅 bump 半边） | P0 | RevisionBumper | ✅ |
| PublishSessionRevisionEnvelope 删除 | P0 | ADR-03 Blocker D | ✅ |
| 前端通过 ListActivities/GetSession RPC 读取 | P0 | API Backfill | ✅ |

### Phase 3：ADR-02 Activity-First 事件持久化 ✅ 已完成

| 任务 | 优先级 | 依赖 | 状态 |
|------|--------|------|------|
| D1 并行异步持久化（processTask + persistChan + worker） | P0 | — | ✅ |
| D2 三重补偿（retry + dead-letter + API Backfill） | P0 | D1 | ✅ |
| D3 OnError 语义重构（删除 ActivityKindError） | P0 | D1 | ✅ |
| D4 Legacy Kind 清理（sub_task_board/error/delegate） | P0 | D3 | ✅ |
| persistWithRetry 使用 done channel（避免 Close 阻塞） | P0 | D2 | ✅ |
| deadLetter activityID 去重 | P0 | D2 | ✅ |
| sequencer 测试（retry 耗尽→dead-letter / 去重 / 容量淘汰） | P0 | D2 | ✅ |
| projector 测试（OnError 分支 / 终态保护） | P0 | D3 | ✅ |

### Phase 4：ADR-03 统一总线架构 ✅ 已完成

| 任务 | 优先级 | 依赖 | 状态 |
|------|--------|------|------|
| D1 ActivityEvent.Domain 字段 | P0 | — | ✅ |
| D2 MonitorEvent 类型 + MonitorBus 接口 | P0 | — | ✅ |
| D3 80+ Publisher 迁移 | P0 | D1/D2 | ✅ |
| D4 ActivityProjector.EmitSystemEvent | P0 | D1 | ✅ |
| D5 WSServer 2-pump（activityEventPump + monitorEventPump） | P0 | D3 | ✅ |
| D6 前端统一到 ActivityEvent + MonitorEvent | P0 | D5 | ✅ |
| 前端本地类型解耦（ExecutionProgressMetadata 等） | P0 | D6 | ✅ |
| WsDownstream 删除 envelope? 字段 | P0 | Blocker A | ✅ |

### Phase 5：ADR-03 Phase 5 Blocker 级联迁移 ✅ 已完成

| Blocker | 任务 | 状态 |
|---------|------|------|
| A | WS Replay 路径删除（方案 A2：改用 ListActivities RPC） | ✅ |
| B | 4 个 side consumer 迁移到 ActivityEventBus/MonitorBus | ✅ |
| C | DomainEvent bridge 迁移 + 接口删除 | ✅ |
| D | 3 个死发布者删除（EmitProgress / LogError publish / PublishSessionRevisionEnvelope） | ✅ |
| E | EventPipeline.Bus/Buffer 字段删除 | ✅ |
| F Stage 1 | 死参数链清理（EventPipeline.Bus + 5 个 Wire provider + TurnPreviewCoordinator 订阅） | ✅ |
| F Stage 2 | SelfHealObserver/TraceProjector 迁移到 MonitorEventBus（修复死订阅 bug） | ✅ |
| F Stage 3 | SessionBus 删除（ProvideSessionBus + Infra.SessionBus 字段） | ✅ |
| G | Envelope 文件删除（方案 C：活类型提取 + 死代码删除） | ✅ |

### Phase 6：待开发

| 任务 | 优先级 | 依赖 | 状态 |
|------|--------|------|------|
| 消息引用字段 + UI | P3 | parent_activity_id | ❌ |
| persist worker 监控 + 水平扩展 | P3 | persistChan 满载率 | ❌ |
| 前端 Envelope 残留清理 | P3 | Blocker A 已完成 | ❌ |

---

## 6. 任务清单

| # | 任务 | 优先级 | Phase | 状态 |
|---|------|--------|-------|------|
| 1 | FTS5 虚拟表 + 索引 | P2 | 1 | ✅ |
| 2 | SearchMessages RPC | P2 | 1 | ✅ |
| 3 | 前端搜索组件 | P2 | 1 | ✅ |
| 4 | CallbackConsumer（ActivityEventBus） | P3 | 2 | ✅ |
| 5 | FlowLogPersistConsumer（MonitorEventBus） | P3 | 2 | ✅ |
| 6 | UserFeedbackConsumer（ActivityEventBus） | P3 | 2 | ✅ |
| 7 | UsageRollupConsumer（ActivityEventBus） | P3 | 2 | ✅ |
| 8 | MessageStoreConsumer 删除 | P3 | 2 | ✅ |
| 9 | session_revision bump（仅 bump 半边） | P0 | 2.5 | ✅ |
| 10 | ADR-02 D1 并行异步持久化 | P0 | 3 | ✅ |
| 11 | ADR-02 D2 三重补偿 | P0 | 3 | ✅ |
| 12 | ADR-02 D3 OnError 重构 | P0 | 3 | ✅ |
| 13 | ADR-02 D4 Legacy Kind 清理 | P0 | 3 | ✅ |
| 14 | ADR-03 D1 Domain 字段 | P0 | 4 | ✅ |
| 15 | ADR-03 D2 MonitorEvent 类型 | P0 | 4 | ✅ |
| 16 | ADR-03 D3 Publisher 迁移 | P0 | 4 | ✅ |
| 17 | ADR-03 D4 EmitSystemEvent | P0 | 4 | ✅ |
| 18 | ADR-03 D5 WSServer 2-pump | P0 | 4 | ✅ |
| 19 | ADR-03 D6 前端统一 | P0 | 4 | ✅ |
| 20 | Phase 5 Blocker A（WS Replay 删除） | P0 | 5 | ✅ |
| 21 | Phase 5 Blocker B（side consumer 迁移） | P0 | 5 | ✅ |
| 22 | Phase 5 Blocker C（DomainEvent bridge 删除） | P0 | 5 | ✅ |
| 23 | Phase 5 Blocker D（3 个死发布者删除） | P0 | 5 | ✅ |
| 24 | Phase 5 Blocker E（EventPipeline.Bus/Buffer 删除） | P0 | 5 | ✅ |
| 25 | Phase 5 Blocker F Stage 1（死参数链清理） | P0 | 5 | ✅ |
| 26 | Phase 5 Blocker F Stage 2（SelfHealObserver/TraceProjector 迁移） | P0 | 5 | ✅ |
| 27 | Phase 5 Blocker F Stage 3（SessionBus 删除） | P0 | 5 | ✅ |
| 28 | Phase 5 Blocker G（Envelope 文件删除） | P0 | 5 | ✅ |
| 29 | 消息引用字段 + UI | P3 | 6 | ❌ |
| 30 | persist worker 监控 + 水平扩展 | P3 | 6 | ❌ |
| 31 | 前端 Envelope 残留清理 | P3 | 6 | ❌ |

---

## 7. 验收标准

### Phase 1（✅ 已验收）

- [x] 可搜索历史消息（关键词 + 分页 + FTS snippet 高亮）
- [ ] 搜索延迟 < 200ms（10 万条消息以内，待压测）

### Phase 2（✅ 已验收 — AF v2）

- [x] CallbackConsumer 独立订阅 ActivityEventBus → WebhookDispatcher
- [x] FlowLogPersistConsumer 独立订阅 MonitorEventBus → 持久化
- [x] UserFeedbackConsumer 独立订阅 ActivityEventBus
- [x] UsageRollupConsumer 独立订阅 ActivityEventBus → 用量汇总
- [x] MessageStoreConsumer 已删除（messages 表 DROP）

### Phase 2.5（✅ 已验收 — 重构后）

- [x] Turn 完成 revision 自增（仅 bump 半边）
- [x] PublishSessionRevisionEnvelope 已删除
- [x] 前端通过 ListActivities/GetSession RPC 读取 revision

### Phase 3（✅ 已验收 — ADR-02）

- [x] processTask 并行异步：persistChan fire-and-forget + 同步 publish
- [x] 重试预算：5 次，100/200/400/800/1600ms，done channel 可中断
- [x] Dead-letter 环形缓冲：512 容量，FIFO 淘汰，activityID 去重
- [x] API Backfill：ListActivities RPC 作为最终一致兜底
- [x] OnError 无 ActivityKindError：root task → status=failed + Meta.error_*
- [x] OnTurnEnd 终态保护：IsActivityTerminal 判定，终态不被覆盖
- [x] Legacy Kind 清理：sub_task_board/error/delegate 已删除
- [x] persistWithRetry 使用 select on done channel（避免 Close 阻塞）
- [x] sequencer 测试覆盖（retry 耗尽 / 去重 / 容量淘汰）
- [x] projector 测试覆盖（OnError 分支 / 终态保护）
- [x] `go test ./... -race` 无回归

### Phase 4（✅ 已验收 — ADR-03）

- [x] ActivityEvent.Domain 字段（chat/system）
- [x] MonitorEvent 类型 + MonitorBus 接口（contract/monitor_event.go）
- [x] 80+ Publisher 全部迁移
- [x] ActivityProjector.EmitSystemEvent（Domain=system 不持久化）
- [x] WSServer 2-pump（activityEventPump + monitorEventPump）
- [x] 前端 Envelope import 从 46 降至 7
- [x] 前端本地类型解耦（ExecutionProgressMetadata / ActivityUsage / InspectorEvent）
- [x] WsDownstream.envelope? 字段已删除
- [x] `go build ./...` + `go test ./...` + `pnpm test`（516 tests）全通过

### Phase 5（✅ 已验收 — Blocker A-G）

- [x] Blocker A：WS Replay 路径删除，重连改用 ListActivities RPC
- [x] Blocker B：4 个 side consumer 迁移到 ActivityEventBus/MonitorBus
- [x] Blocker C：DomainEvent bridge 删除，DomainEventPublisher/Subscriber 接口删除
- [x] Blocker D：3 个死发布者删除（EmitProgress / LogError publish / PublishSessionRevisionEnvelope）
- [x] Blocker E：EventPipeline.Bus/Buffer 字段删除
- [x] Blocker F Stage 1：死参数链清理（EventPipeline.Bus + 5 个 Wire provider + TurnPreviewCoordinator）
- [x] Blocker F Stage 2：SelfHealObserver/TraceProjector 迁移到 MonitorEventBus（修复死订阅 bug）
- [x] Blocker F Stage 3：SessionBus 删除（ProvideSessionBus + Infra.SessionBus 字段）
- [x] Blocker G：Envelope 文件删除（方案 C：活类型提取到 envelope_types.go + 死代码删除）
- [x] Infra 容器仅剩 MonitorEventBus + lg
- [x] 后端 Envelope 类型在生产代码中已彻底移除
- [x] `go build ./...` + `make wire` + `go build ./cmd/admin` + `go test ./...` + `go vet ./...` 全通过

### Phase 6

- [ ] 消息可引用历史消息（前端 UI）
- [ ] persistChan 满载率监控 + 必要时水平扩展
- [ ] 前端 7 处 Envelope 残留清理

---

## 8. 依赖与风险

| 项 | 说明 |
|----|------|
| ADR-02/ADR-03 落地 | ✅ 已完成：双 Bus 架构 + 并行异步持久化 + 三重补偿 + Envelope 删除 |
| 持久化失败补偿 | ✅ 已完成：retry（5 次，3100ms）+ dead-letter（512，FIFO，activityID 去重）+ API Backfill（ListActivities RPC） |
| WS 重连恢复 | ✅ 已完成：删除 event.Buffer replay，改用 ListActivities RPC（API Backfill） |
| persist worker 瓶颈 | ⚠️ 监控项：单 worker per-activity FIFO，高并发 turn 可能成为瓶颈，监控 persistChan 满载率 |
| Dead-letter 容量 | ⚠️ 监控项：512 条，极端场景（DB 长时间不可用）可能丢失部分记录，API Backfill 兜底 |
| 临时不一致窗口 | ⚠️ 已知：persist 失败时前端实时状态与 DB 不一致，最长持续到下次 API Backfill（通常 < 5s） |
| 前端 Envelope 残留 | ⚠️ 已知：7 处合法传输层 import，依赖后续前端清理 |
| 全局监控安全 | ✅ 已完成：session_id=* 连接限 3 个，防止滥用 |
