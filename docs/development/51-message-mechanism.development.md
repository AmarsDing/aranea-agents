# Message 消息 — 开发计划

> **版本**：2026-06-17 | **状态**：✅ 核心 + P2 搜索 + P3 独立消费者 + session_revision 已落地
> **M55 增量同步**：✅ session_revision 已实现 — [55-chat-channel-cursor-development.md §Phase B](./55-chat-channel-cursor-development.md#phase-b--session-sync-协议p0约-1-周)
> **需求**：[51 消息机制](./51-message-mechanism.md) | **设计**：[51-message-mechanism.design.md](./51-message-mechanism.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) · **EP**：—

---

## 1. 模块定位

Message 消息：统一的事件模型和传输机制，以 EventBus + Envelope 为核心，WebSocket（`/v1/ws`）为 Chat / Team / Graph / Monitor 的实时主传输通道。历史 Chat SSE 路由已从当前主链路移除，不得作为新功能入口。

**代码锚点**：

| 层 | 文件 | 职责 |
|----|------|------|
| 事件定义 | `internal/event/contract/envelope.go` | Envelope 结构 + **57 种** EnvelopeType 常量 |
| 事件再导出 | `internal/event/envelope.go` | 契约包再导出 |
| 事件路由 | `internal/event/contract/bus.go` + `bus.go` | Bus 接口定义 + 实现（Subscribe/Publish/DropPolicy） |
| 事件缓冲 | `internal/event/buffer.go` | 环形缓冲区 + TTL 驱逐 + WS 重放 |
| 流程日志 v2 | `internal/event/trace_emitter.go`、`flow_tracker.go` | `EnvelopeTypeFlowLog` → Monitor WS |
| Session Revision | `internal/event/session_revision.go` | SessionRevisionBumper + BumpAndPublishSessionRevision |
| 事件投影 | `internal/agent/event_projector.go` | trpc event.Event → Envelope 投影 + 发布 |
| 事件消费 | `internal/biz/event_bus_consumer.go` + `event_bus_*_handler.go` | 编排器 + buffer/runner/state/persist 四 handler |
| Side Consumers | `internal/biz/event_bus_side_consumers.go` | Side Consumers 编排器（统一启动/停止） |
| WS 服务 | `internal/server/ws.go` + `ws_*.go` | WebSocket 服务（挂入 Kratos HTTP Server，拆分为多子文件） |
| HTTP 后台入口 | `internal/service/chat.go` | `SendChatMessage` unary 入口；WS 上行、Channel、Cron 复用同一 native turn |
| FTS5 搜索 | `internal/data/sql/message_fts.sql` | FTS5 虚拟表 + 触发器 |
| 前端传输 | `web/src/realtime/ws-transport.ts` | createWsTransport（心跳/重连/pending 队列） |
| 前端分发 | `web/src/realtime/dispatcher.ts` | EnvelopeDispatcher 类（onType/onChannel/on） |
| 前端类型 | `web/src/realtime/envelope.ts` | Envelope 类型定义（57 种，与后端对齐） |
| 前端 Hooks | `web/src/features/chat/useEnvelopeStream.ts` | useChatStream / useTeamStream / useMonitorStream |
| 前端同步 | `web/src/features/chat/inboundSyncEnvelope.ts` | session_revision 入站同步处理 |

---

## 2. 现状评估

### 2.1 核心机制（✅ 已完成）

| 项 | 状态 | 证据 |
|----|------|------|
| 统一事件模型 | ✅ | Envelope + EnvelopeType（**72 种**），Chat/Team/Graph/Monitor/Knowledge/Spirit/Butler/Skill 共享 |
| 统一事件总线 | ✅ | `event.Bus` 接口，支持 session_id / team_id / channel / filter_key / event_types / selector 路由 |
| 事件投影 | ✅ | `EventProjector.Project`（**Deprecated**；AF 阶段引入 `ActivityProjector`），Service 层不再直接处理事件 |
| WebSocket 统一传输 | ✅ | 单连接多路复用（chat/monitor/team/graph/system），挂入 Kratos |
| 双向通信 | ✅ | cancel / user_message / enqueue_message（→ `EnqueueUserMessage` RPC）/ subscribe / enable_log 上行 |
| 背压控制 | ✅ | 三级 DropPolicy（DropOldest/DropNewest/BlockUpTo）+ ChannelPriority（Critical/Normal）+ Prometheus 丢弃计数 |
| 事件缓冲与重放 | ✅ | `event.Buffer`（环形缓冲区 + TTL 30min + 每会话 200 上限）+ WS 同步屏障 |
| Monitor 流程日志 | ✅ | `flow_log` 免 `enable_log`；进程 `log` 门控；`useLogStreamHub` 分流 |
| EventBusConsumer SRP | ✅ | buffer / runner / state / persist 四 handler |
| event_store 持久化 | ✅ | `event_persist_handler` 异步队列（与 34-event 对齐） |
| Side Consumers | ✅ | ToolCallConsumer / CallbackConsumer / MessageStoreConsumer / FlowLogPersistConsumer / UserFeedbackConsumer / UsageRollupConsumer |
| 全局监控模式 | ✅ | `session_id=*` 连接可订阅所有会话的 Monitor/Team/Graph 事件（限 3 连接） |
| 服务端优雅关闭 | ✅ | `server_shutdown` 系统消息广播 |
| WS Server 拆分 | ✅ | ws.go → ws.go + ws_conn.go + ws_conn_manager.go + ws_codec.go + ws_message_handler.go + ws_io_pump.go + ws_event.go + ws_priority.go |
| 前端传输层提升 | ✅ | 核心传输层从 `features/chat/` 提升到 `realtime/`，原位保留再导出桶 |
| 前端分发器 | ✅ | `EnvelopeDispatcher` 类（onType/onChannel/on + matchFilterKey） |
| 前端场景 Hooks | ✅ | useChatStream / useTeamStream / useMonitorStream |
| Chat SSE 主链路 | ✅ 已移除 | 实时事件统一走 `/v1/ws`；历史 SSE callback 类型仅可作为待清理兼容代码 |

### 2.2 已完成（按优先级排序）

| 项 | 优先级 | 说明 |
|----|--------|------|
| 消息搜索 | P2 | ✅ FTS5 + `SearchSessionMessages` |
| ToolCallConsumer | P3 | ✅ 独立订阅 `tool_result` |
| CallbackConsumer | P3 | ✅ 独立订阅 `run_status` 终态 → Webhook（`WebhookDispatcher`） |
| MessageStoreConsumer | P3 | ✅ `member_message_done` → `role=member` + `team_member` 元数据 |
| FlowLogPersistConsumer | P3 | ✅ 独立订阅 `flow_log` 持久化 |
| UserFeedbackConsumer | P3 | ✅ 独立订阅 `user_feedback` |
| UsageRollupConsumer | P3 | ✅ 独立订阅 `token_usage` / `runner_completion` 用量汇总 |
| session_revision 增量同步 | P0 | ✅ Turn 完成 revision++；`ListSessionMessages?after_revision=`；前端入站同步 |

### 2.3 待开发（按优先级排序）

| 项 | 优先级 | 说明 |
|----|--------|------|
| 消息引用/回复 | P3 | 引用历史消息（待做） |
| 事件持久化升级 | P3 | EventBuffer → 外部存储（Redis/SQLite WAL），支持跨重启重放 |

---

## 3. 差距与优化

### 3.1 P3 — 消息引用/回复

**现状**：消息无 `reply_to` / `quote` 字段，无法引用历史消息。

**方案**：
- `chat_messages` 表增加 `reply_to_id` 字段
- Envelope 扩展 `quote` 字段
- 前端引用 UI

### 3.2 P3 — 事件持久化升级

**现状**：`EventBuffer` 是内存环形缓冲区，进程重启后丢失。

**方案**：
- 可选外部存储后端（Redis Stream / SQLite WAL）
- 保持 `event.Buffer` 接口不变，替换底层实现
- 跨重启重放支持

---

## 4. 开发阶段

### Phase 0：核心机制 ✅ 已完成

| 任务 | 状态 |
|------|------|
| EventBus + Envelope 统一事件模型 | ✅ |
| EventProjector 事件投影 | ✅ |
| WebSocket 统一传输 + 多路复用 | ✅ |
| 双向通信（cancel/user_message/enqueue） | ✅ |
| 背压控制（三级 DropPolicy） | ✅ |
| EventBuffer + WS 重放同步屏障 | ✅ |
| Flow Log v2 + Monitor 流程日志 | ✅ |
| 全局监控模式 | ✅ |
| 前端 EnvelopeDispatcher + 场景 Hooks | ✅ |
| 服务端优雅关闭 | ✅ |

### Phase 1：消息搜索 ✅ 已完成

| 任务 | 优先级 | 依赖 | 状态 |
|------|--------|------|------|
| FTS5 虚拟表 + 索引构建 | P2 | — | ✅ |
| SearchMessages RPC | P2 | FTS5 | ✅ |
| 前端搜索组件 | P2 | RPC | ✅ |

### Phase 2：消费者拆分 ✅ 已完成

| 任务 | 优先级 | 依赖 | 状态 |
|------|--------|------|------|
| ToolCallConsumer | P3 | EventBus | ✅ |
| CallbackConsumer | P3 | EventBus | ✅ |
| MessageStoreConsumer | P3 | EventBus | ✅ |
| FlowLogPersistConsumer | P3 | EventBus | ✅ |
| UserFeedbackConsumer | P3 | EventBus | ✅ |
| UsageRollupConsumer | P3 | EventBus | ✅ |

### Phase 2.5：session_revision 增量同步 ✅ 已完成

| 任务 | 优先级 | 依赖 | 状态 |
|------|--------|------|------|
| SessionRevisionBumper 接口 + 实现 | P0 | EventBus | ✅ |
| BumpAndPublishSessionRevision | P0 | RevisionBumper | ✅ |
| 前端 inboundSyncEnvelope | P0 | 后端 revision | ✅ |

### Phase 3：传输扩展 + 持久化升级（待做）

| 任务 | 优先级 | 依赖 | 状态 |
|------|--------|------|------|
| 消息引用字段 + UI | P3 | — | ❌ |
| 事件持久化升级（Redis/SQLite WAL） | P3 | event.Buffer 接口 | ❌ |

---

## 5. 任务清单

| # | 任务 | 优先级 | Phase | 状态 |
|---|------|--------|-------|------|
| 1 | FTS5 虚拟表 + 索引 | P2 | 1 | ✅ |
| 2 | SearchMessages RPC | P2 | 1 | ✅ |
| 3 | 前端搜索组件 | P2 | 1 | ✅ |
| 4 | ToolCallConsumer 拆分 | P3 | 2 | ✅ |
| 5 | CallbackConsumer 实现 | P3 | 2 | ✅ |
| 6 | MessageStoreConsumer 实现 | P3 | 2 | ✅ |
| 7 | FlowLogPersistConsumer 实现 | P3 | 2 | ✅ |
| 8 | UserFeedbackConsumer 实现 | P3 | 2 | ✅ |
| 9 | UsageRollupConsumer 实现 | P3 | 2 | ✅ |
| 10 | session_revision 增量同步 | P0 | 2.5 | ✅ |
| 11 | 消息引用字段 + UI | P3 | 3 | ❌ |
| 12 | 事件持久化升级 | P3 | 3 | ❌ |

---

## 6. 验收标准

### Phase 0（✅ 已验收）

- [x] 统一事件模型：所有场景共享 Envelope + EnvelopeType
- [x] WebSocket 单连接多路复用：Chat/Monitor/Team/Graph/System
- [x] 双向通信：cancel / user_message / enqueue_message 上行
- [x] 背压控制：三级 DropPolicy + Prometheus 丢弃计数
- [x] 事件缓冲与重放：断连后自动重连 + 事件重放
- [x] Monitor 流程日志：Flow Log v2（`flow_log`）；SlogBridge 已移除
- [x] 全局监控：session_id=* 跨会话订阅
- [x] 前端分发器 + 场景 Hooks

### Phase 1（✅ 已验收）

- [x] 可搜索历史消息（关键词 + 分页 + FTS snippet 高亮）
- [ ] 搜索延迟 < 200ms（10 万条消息以内，待压测）

### Phase 2（✅ 已验收）

- [x] 工具调用独立持久化（`toolCallConsumer`）
- [x] 回调通知可发送到外部 URL（`callbackConsumer` + `WebhookDispatcher`）
- [x] Team 成员消息落库（`messageStoreConsumer`）
- [x] FlowLog 持久化（`flowLogPersistConsumer`）
- [x] 用户反馈处理（`userFeedbackConsumer`）
- [x] 用量汇总（`usageRollupConsumer`）

### Phase 2.5（✅ 已验收）

- [x] Turn 完成 revision 自增
- [x] `ListSessionMessages?after_revision=` 增量查询
- [x] 前端 `inboundSyncEnvelope` 入站同步处理

### Phase 3

- [ ] 消息可引用历史消息
- [ ] 事件持久化跨重启可重放

---

## 7. 依赖与风险

| 项 | 说明 |
|----|------|
| 消费者拆分 | ✅ 已完成：Side Consumers 独立 Bus 订阅，按 EventTypes 过滤，通过 `event_bus_side_consumers.go` 统一编排 |
| Webhook 回调 | ✅ 已完成：采用 `callbackConsumer` + `WebhookDispatcher` 模式（非 WebhookProjector 模式） |
| 事件持久化 | Redis 增加运维复杂度，SQLite WAL 需评估写入性能 |
| 全局监控安全 | session_id=* 连接限 3 个，需防止滥用 |
