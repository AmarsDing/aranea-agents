# Gateway 网关 — 开发计划

> **版本**：2026-06-17 | **状态**：🟢 核心 + Webhook + ChatOrchestrator 已落地；API 版本管理/文档待做
> **需求**：[35 gateway.md](./35%20gateway.md) · **设计**：[35-gateway.design.md](./35-gateway.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) M1 · **EP**：EP-RT-01/02 ✅

---

## 1. 模块定位

Gateway 网关：运行编排层，负责会话并发控制、**Follow-up Queue（对话阶段连续发送）**、运行状态管理、用户回复路由和出站 Webhook 回调。

**代码锚点**：
- `internal/runtime/run_registry.go` — RunRegistry + RunGateway
- `internal/runtime/gateway.go` — RunGateway 接口
- `internal/runtime/runner_manager.go` — RunnerManager TurnRunner 构建
- `internal/runtime/pending_queue.go` — PendingMessageQueue（Follow-up FIFO）
- `internal/runtime/turn/admission_gate.go` — AdmissionGate 准入控制
- `internal/biz/chat_usecase.go` — Biz 编排（状态/排队/入队/await）
- `internal/biz/webhook.go` — WebhookConfig + WebhookUsecase + 事件常量
- `internal/biz/webhook_dispatcher.go` — WebhookDispatcher
- `internal/biz/event_bus_callback_consumer.go` — callbackConsumer
- `internal/service/chat_run_gateway.go` — Biz 适配器
- `internal/service/chat.go` — ChatService 薄传输桥
- `internal/service/chat_orchestrator.go` — ChatOrchestrator 核心编排器
- `internal/service/chat_orchestrator_turn.go` — Turn 执行 + processPendingQueue 调用
- `internal/service/chat_orchestrator_turn_dispatch.go` — processPendingQueue 定义
- `internal/service/turn_pipeline.go` — TurnPipeline 管道
- `internal/service/chat_turn_admission.go` — 准入适配器
- `internal/service/chat_native.go` — 发送/Team/A2A/Cron 入口
- `internal/service/chat_enqueue.go` — 入队拒绝辅助
- `internal/service/gateway.go` — GatewayService Webhook CRUD
- `internal/data/webhook.go` — WebhookRepo
- `internal/data/ent/schema/gateway_webhook.go` — GatewayWebhook Ent Schema
- `internal/server/ws.go` — WebSocket 网关
- `pkg/auth/middleware.go` — JWT 认证

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| RunRegistry / RunGateway | ✅ | `internal/runtime/run_registry.go` + `gateway.go` |
| RunnerManager | ✅ | `internal/runtime/runner_manager.go` |
| ChatOrchestrator | ✅ | `internal/service/chat_orchestrator.go`，实现 `biz.TurnExecutor` |
| TurnPipeline | ✅ | `internal/service/turn_pipeline.go`，显式管道 |
| AdmissionGate | ✅ | `internal/runtime/turn/admission_gate.go` + `chat_turn_admission.go` |
| Chat/Team/Cron/Channel 共用 RunGateway | ✅ | `wire.go provideRunRegistry` + `ChatService.RunGateway()` |
| 会话并发控制 | ✅ | `RunRegistry.HasActive` + placeholder 清理 |
| SteerableRunner 入队 | ✅ | `RunRegistry.EnqueueUserMessage` |
| Follow-up Queue 后端 | ✅ | Steerable + Pending FIFO；`PublishMessageQueued` |
| Follow-up Queue 前端 UX | ✅ | 连续发送 + `message_queued` WS 刷新（`useFollowUpQueue.ts` / `useChatRunStatus.ts`） |
| 运行取消 | ✅ | `StopGeneration` + `RunRegistry.Cancel` |
| 运行状态查询 | ✅ | `GetRunStatus` + trpc RunStatus 合并 |
| 用户回复路由 | ✅ | `AwaitUserReply` + `makeAwaitReplyFunc` |
| AwaitUserReplyRouting | ✅ | `RunnerManager` 在 `AwaitHook != nil` 时启用 |
| Biz 编排 ChatUsecase | ✅ | `NewChatUsecaseFromDeps` 接入 ChatOrchestrator |
| WebSocket 网关 | ✅ | `ws.go`，双 bus + 三优先级队列 |
| 认证中间件 | ✅ | JWT + Workspace + Webhook 路径安全（EP-SEC-03） |
| PendingMessageQueue 下沉 | ✅ | `internal/runtime/pending_queue.go` + `pending_queue_entries`（DDL 20261240；启动回放优先于 JSON 快照） |
| 出站 Webhook | ✅ | `GatewayService` + `WebhookDispatcher` + 终态触发，含 `graph.task.status` |
| API 版本管理策略 | ❌ | 无文档 |
| API 文档自动生成 | ❌ | 无 Swagger |

---

## 3. 差距与优先级

| # | 差距 | 优先级 | 对应需求 | 说明 |
|---|------|--------|----------|------|
| 1 | Follow-up Queue 前端 UX | ✅ | 2.1 | Phase 1.5 已完成 |
| 2 | PendingMessageQueue 下沉 | ✅ | 2.2 | `internal/runtime/pending_queue.go` |
| 3 | 出站 Webhook | ✅ | 2.3 | Phase 3 已完成，含 `graph.task.status` |
| 4 | ChatOrchestrator 重构 | ✅ | — | `trpc_turn.go` 已拆分为 ChatOrchestrator 体系 |
| 5 | TurnPipeline + AdmissionGate | ✅ | — | 显式管道 + 准入控制 |
| 6 | API 版本管理策略 | P3 | 2.4 | 文档化版本演进规则 |
| 7 | API 文档自动生成 | P3 | 2.5 | protoc-gen-openapi |

---

## 4. 开发阶段

### Phase 1：Runtime + Biz 编排 ✅

| # | 任务 | 状态 |
|---|------|------|
| 1.1 | RunRegistry + RunGateway | ✅ |
| 1.2 | RunnerManager | ✅ |
| 1.3 | ChatUsecase + 适配器 | ✅ |
| 1.4 | ChatService 委托 ChatOrchestrator | ✅ |
| 1.5 | SteerableRunner + 降级 | ✅ |
| 1.6 | AwaitUserReplyRouting 条件启用 | ✅ |
| 1.7 | ChatOrchestrator 重构（`trpc_turn.go` 拆分） | ✅ |
| 1.8 | TurnPipeline 显式管道 | ✅ |
| 1.9 | AdmissionGate 准入控制 | ✅ |

**验收**：
- [x] Chat / Team / Cron / Channel 共用 RunRegistry
- [x] ChatUsecase 编排入队/排队/状态/锁/await channel
- [x] SteerableRunner 优先 + PendingMessageQueue 降级
- [x] ChatService 为薄传输桥，ChatOrchestrator 为核心编排器
- [x] TurnPipeline 显式 Ingress → Service → Executor → Projector 管道
- [x] AdmissionGate 放行/入队/拒绝三路决策

### Phase 1.5：Follow-up Queue UX（Cursor 对齐）（P2）✅

| # | 任务 | 涉及文件 | 验证 |
|---|------|----------|------|
| 1.5.1 | 运行中解除 `sending` 阻塞 | `web/src/features/chat/composables/useChatSender.ts` | 运行中可连续 Enter |
| 1.5.2 | 监听 `message_queued` 刷新 Pending | `web/src/features/chat/composables/useFollowUpQueue.ts`、`web/src/features/chat/composables/useChatRunStatus.ts`、`web/src/features/chat/envelopeRunStatus.ts` | 入队后列表即时更新 |
| 1.5.3 | 删除 `ChatService.publishMessageQueued` | `internal/service/chat.go` | 无引用 |
| 1.5.4 | （可选 P3）`pending_enqueued` Envelope | `internal/event/envelope.go`、`internal/service/chat_run_gateway.go` | 含 pending_id |

**验收**：
- [x] Agent 流式输出期间可连续发送 ≥3 条消息
- [x] Pending 列表在 WS `message_queued` 后即时更新
- [x] Steerable 直注与 Pending 降级行为与文档一致

### Phase 2：PendingMessageQueue 下沉（P2）✅

| # | 任务 | 涉及文件 | 验证 |
|---|------|----------|------|
| 2.1 | 迁移 PendingMessageQueue | `internal/runtime/pending_queue.go` | 类型与行为不变 |
| 2.2 | 更新适配器 | `internal/service/chat_run_gateway.go` | `pendingQueueAdapter` 工作 |
| 2.3 | 删除 Service 层实现 | `internal/service/chat_pending.go`（已删除） | 无残留引用 |

### Phase 3：出站 Webhook 系统（P2）✅

| # | 任务 | 状态 |
|---|------|------|
| 3.1 | GatewayWebhook Ent Schema | ✅ |
| 3.2 | WebhookRepository + Usecase + Dispatcher | ✅ |
| 3.3 | Gateway Proto + GatewayService | ✅ |
| 3.4 | 运行结束触发 Dispatch | ✅ |
| 3.5 | chat_native 入队拒绝文案 | ✅ |

**验收**：
- [x] Webhook CRUD API 可用
- [x] 运行完成/失败/取消触发 HMAC 签名回调
- [x] 可按 event_types_json 过滤

### Phase 4：API 版本管理 + 文档（P3）

| # | 任务 | 验证 |
|---|------|------|
| 4.1 | 文档化 API 版本演进策略 | 策略文档可查阅 |
| 4.2 | protoc-gen-openapi + Swagger UI | Swagger 可访问 |

---

## 5. 依赖与风险

| 风险 | 影响 | 缓解 |
|------|------|------|
| Webhook 目标不可达 | 通知丢失 | 异步发送 + 日志 + 3 次重试，不阻塞主流程 |
| ChatOrchestrator 职责膨胀 | 可维护性下降 | 按关注点拆分文件（turn / admission / durable / session_run / spirit） |

---

## 6. 与 Runner 模块（40）的协调

| 任务 | Gateway 负责 | Runner 负责 |
|------|-------------|-------------|
| Runner 构建 | — | AgentFactory、PluginManager、ManagedRunner |
| AwaitUserReplyRouting | RunnerManager 注入选项 | 框架提供路由能力 |
| SteerableRunner 联调 | RunRegistry.EnqueueUserMessage | 框架提供接口 |
| 运行编排 | ChatOrchestrator + ChatUsecase + RunRegistry | — |
| Turn 管道 | TurnPipeline + AdmissionGate | — |
| Follow-up Queue | ChatUsecase + PendingMessageQueue + processPendingQueue | SteerableRunner |
| Webhook 出站 | WebhookDispatcher + callbackConsumer | — |

---

## 7. 优化记录

| 优化项 | 说明 |
|--------|------|
| ChatUsecase 接入 ChatService | 消除 Service 层重复的入队/排队/锁/await 逻辑 |
| Steerable 入队 WS 通知 | `ChatUsecase.EnqueueUserMessage` 在 Steerable 成功时也 PublishMessageQueued |
| 文档与代码对齐 | RunRegistry 位于 runtime 层；设计文档更新分层说明 |
| 2026-05-21 Phase 3 | 出站 Webhook CRUD + HMAC 回调；chat_native 入队拒绝码（`CHAT_RUN_ENDED` / `CHAT_QUEUE_FULL`） |
| 2026-05-21 DocSync | Follow-up Queue 产品规格（Cursor 对齐）；`publishMessageQueued` 收敛至 ChatUsecase；Phase 1.5 前端 UX |
| 2026-06-06 DocSync | ChatOrchestrator 重构对齐：`trpc_turn.go` 已拆分为 ChatOrchestrator 体系；`chat_pending.go` 已删除；新增 TurnPipeline / AdmissionGate；Webhook 新增 `graph.task.status` 事件；认证新增 EP-SEC-03 |
| 2026-06-17 DocSync | 三件套内容边界重组：需求文档移除代码/进度内容；设计文档修正接口签名（RunGateway.Cancel/ChatUsecase/CloseRunner）、字段类型（Schema created_at/updated_at 为 string）、事件类型（`graph.task.status` 而非 `graph_task_status`）、SQL 迁移路径（Ent Auto-Migration 而非 `docs/sql/19_gateway_webhook.sql`）、ChatOrchestrator 结构；开发计划修正前端代码锚点（`useFollowUpQueue.ts`/`useChatRunStatus.ts`） |
