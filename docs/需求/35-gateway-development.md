# Gateway 网关 — 开发计划

> **版本**：2026-05-21 | **状态**：🟢 核心 + Webhook 已落地；Follow-up Queue 前端 UX / PendingQueue 下沉待做
> **需求**：[35 gateway.md](./35%20gateway.md) · **设计**：[35 gateway.design.md](./35%20gateway.design.md)
> **进度真相**：[execution-plan.md](../guides/execution-plan.md) M1 · **EP**：EP-RT-01/02 ✅

---

## 1. 模块定位

Gateway 网关：运行编排层，负责会话并发控制、**Follow-up Queue（对话阶段连续发送）**、运行状态管理、用户回复路由和出站 Webhook 回调。

**代码锚点**：
- `internal/runtime/run_registry.go` — RunRegistry + RunGateway
- `internal/runtime/runner_manager.go` — RunnerManager TurnRunner 构建
- `internal/biz/chat_usecase.go` — Biz 编排（状态/排队/入队/await）
- `internal/service/chat_run_gateway.go` — Biz 适配器
- `internal/runtime/pending_queue.go` — PendingMessageQueue（Follow-up FIFO）
- `internal/service/chat.go` / `chat_native.go` / `trpc_turn.go` — Chat RPC + Turn
- `internal/biz/webhook.go` / `webhook_dispatcher.go` — 出站 Webhook
- `internal/service/gateway.go` / `chat_enqueue.go` — CRUD + 终态经 Bus `callbackConsumer`
- `internal/server/ws.go` — WebSocket 网关
- `pkg/auth/middleware.go` — JWT 认证

---

## 2. 现状评估

| 项 | 状态 | 证据 |
|----|------|------|
| RunRegistry / RunGateway | ✅ | `internal/runtime/run_registry.go` + `gateway.go` |
| RunnerManager | ✅ | `internal/runtime/runner_manager.go` |
| Chat/Team/Cron/Channel 共用 RunGateway | ✅ | `wire.go provideRunRegistry` + `ChatService.RunGateway()` |
| 会话并发控制 | ✅ | `RunRegistry.HasActive` + placeholder 清理 |
| SteerableRunner 入队 | ✅ | `RunRegistry.EnqueueUserMessage` |
| Follow-up Queue 后端 | ✅ | Steerable + Pending FIFO；`PublishMessageQueued` |
| Follow-up Queue 前端 UX | 🟡 | 连续发送 / `message_queued` 监听待 Phase 1.5 |
| 运行取消 | ✅ | `StopGeneration` + `RunRegistry.Cancel` |
| 运行状态查询 | ✅ | `GetRunStatus` + trpc RunStatus 合并 |
| 用户回复路由 | ✅ | `AwaitUserReply` + `makeAwaitReplyFunc` |
| AwaitUserReplyRouting | ✅ | `RunnerManager` 在 `AwaitHook != nil` 时启用 |
| Biz 编排 ChatUsecase | ✅ | `NewChatUsecaseFromDeps` 接入 ChatService |
| WebSocket 网关 | ✅ | `ws.go` |
| 认证中间件 | ✅ | JWT + Workspace |
| PendingMessageQueue 下沉 | ✅ | `internal/runtime/pending_queue.go` |
| Follow-up Queue 前端 UX | ✅ | 连续发送 + `message_queued` WS 刷新 |
| 出站 Webhook | ✅ | `GatewayService` + `WebhookDispatcher` + 终态触发 |
| API 版本管理策略 | ❌ | 无文档 |
| API 文档自动生成 | ❌ | 无 Swagger |

---

## 3. 差距与优先级

| # | 差距 | 优先级 | 对应需求 | 说明 |
|---|------|--------|----------|------|
| 1 | Follow-up Queue 前端 UX | ✅ | 3.0 | Phase 1.5 |
| 2 | PendingMessageQueue 下沉 | ✅ | 3.1 | `internal/runtime/pending_queue.go` |
| 3 | 出站 Webhook | ✅ | 3.2 | 2026-05-21 Phase 3 |
| 3 | setRunStatus 委托 ChatUsecase | P3 | — | await meta 发布仍留 Service |
| 4 | API 版本管理策略 | P3 | 3.3 | 文档化版本演进规则 |
| 5 | API 文档自动生成 | P3 | 3.4 | protoc-gen-openapi |

---

## 4. 开发阶段

### Phase 1：Runtime + Biz 编排 ✅

| # | 任务 | 状态 |
|---|------|------|
| 1.1 | RunRegistry + RunGateway | ✅ |
| 1.2 | RunnerManager | ✅ |
| 1.3 | ChatUsecase + 适配器 | ✅ |
| 1.4 | ChatService 委托 chatUC | ✅ 2026-05-21 |
| 1.5 | SteerableRunner + 降级 | ✅ |
| 1.6 | AwaitUserReplyRouting 条件启用 | ✅ |

**验收**：
- [x] Chat / Team / Cron / Channel 共用 RunRegistry
- [x] ChatUsecase 编排入队/排队/状态/锁/await channel
- [x] SteerableRunner 优先 + PendingMessageQueue 降级
- [x] 现有 Chat API 行为不变

### Phase 1.5：Follow-up Queue UX（Cursor 对齐）（P2）

| # | 任务 | 涉及文件 | 验证 |
|---|------|----------|------|
| 1.5.1 | 运行中解除 `sending` 阻塞 | `useChatSender.ts` | 运行中可连续 Enter |
| 1.5.2 | 监听 `message_queued` 刷新 Pending | `useChatWorkspace.ts`, `envelopeRunStatus.ts` | 入队后列表即时更新 |
| 1.5.3 | 删除 `ChatService.publishMessageQueued` | `chat.go` | 无引用 |
| 1.5.4 | （可选 P3）`pending_enqueued` Envelope | `event/envelope.go`, `chat_run_gateway.go` | 含 pending_id |

**验收**：
- [x] Agent 流式输出期间可连续发送 ≥3 条消息
- [x] Pending 列表在 WS `message_queued` 后即时更新
- [x] Steerable 直注与 Pending 降级行为与文档一致

### Phase 2：PendingMessageQueue 下沉（P2）

| # | 任务 | 涉及文件 | 验证 |
|---|------|----------|------|
| 2.1 | 迁移 PendingMessageQueue | ✅ |
| 2.2 | 更新适配器 | ✅ |
| 2.3 | 删除 Service 层实现 | ✅ |

### Phase 3：出站 Webhook 系统（P2） ✅

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
| PendingMessageQueue 迁移引入回归 | 排队行为变化 | 保留现有单元测试 + 快照逻辑 |
| Webhook 目标不可达 | 通知丢失 | 异步发送 + 日志，不阻塞主流程 |
| await meta 发布与 ChatUsecase 合并 | 行为变化 | Phase 3 前保持 Service 层 setRunStatusWithAwait |

---

## 6. 与 Runner 模块（40）的协调

| 任务 | Gateway 负责 | Runner 负责 |
|------|-------------|-------------|
| Runner 构建 | — | AgentFactory、PluginManager、ManagedRunner |
| AwaitUserReplyRouting | RunnerManager 注入选项 | 框架提供路由能力 |
| SteerableRunner 联调 | RunRegistry.EnqueueUserMessage | 框架提供接口 |
| 运行编排 | RunRegistry + ChatUsecase | — |
| Follow-up Queue | ChatUsecase + PendingMessageQueue + processPendingQueue | SteerableRunner |
| Webhook 出站 | WebhookDispatcher | — |

---

## 7. 2026-05-21 优化记录

| 优化项 | 说明 |
|--------|------|
| ChatUsecase 接入 ChatService | 消除 Service 层重复的入队/排队/锁/await 逻辑 |
| Steerable 入队 WS 通知 | `ChatUsecase.EnqueueUserMessage` 在 Steerable 成功时也 PublishMessageQueued |
| 文档与代码对齐 | RunRegistry 位于 runtime 层；设计文档更新分层说明 |
| 待优化 | PendingMessageQueue 下沉；setRunStatusWithAwait 合并到 Biz |
| 2026-05-21 Phase 3 | 出站 Webhook CRUD + HMAC 回调；chat_native 入队拒绝码（`CHAT_RUN_ENDED` / `CHAT_QUEUE_FULL`） |
| 2026-05-21 DocSync | Follow-up Queue 产品规格（Cursor 对齐）；`publishMessageQueued` 收敛至 ChatUsecase；Phase 1.5 前端 UX |
