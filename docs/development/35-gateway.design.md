# Gateway 网关模块 — 实现设计文档

> 对应需求：[35 gateway.md](./35%20gateway.md)
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> 关联设计：[40 runner.design.md](./40%20runner.design.md)（Runner 运行器，Gateway 侧重网关编排层）

---

## 一、模块概述

会话网关：将运行编排（并发闸门、消息排队、状态查询、Steerable 入队）从 `ChatService` 提取为可复用组件，新增出站 Webhook 回调系统，并启用 trpc 框架原生 `AwaitUserReplyRouting` 和 `SteerableRunner`。

**设计目标**：
1. `RunRegistry` / `RunGateway` 位于 `internal/runtime`，供 Chat / Team / Cron / Channel / WS 共用
2. `ChatUsecase`（Biz 层）编排运行状态、排队、入队、await channel
3. `RunnerManager` 统一 TurnRunner 构建，条件启用 `AwaitUserReplyRouting`
4. SteerableRunner 入队优先，不支持时降级 `PendingMessageQueue`
5. 新增 Webhook 回调系统（持久化 + 分发）— **已实现**

**与 Runner 模块的边界**：
- **Runner（40）**：Agent 运行器构建和生命周期（AgentFactory、PluginManager、ManagedRunner 封装）
- **Gateway（本文）**：运行编排层（并发闸门、消息排队、状态查询、Webhook 出站），是 Runner 的上层编排

---

## 二、架构设计（已实现）

### 2.1 分层架构

```
┌─────────────────────────────────────────────────────┐
│  Server 层                                          │
│  internal/server/http.go — 注册 ChatService         │
│  internal/server/ws.go   — WebSocket 事件通道       │
├─────────────────────────────────────────────────────┤
│  Service 层                                         │
│  internal/service/chat.go        — ChatService RPC  │
│  internal/service/chat_native.go — 发送/Team 委派     │
│  internal/service/trpc_turn.go   — 单 Agent Turn    │
│  internal/service/chat_run_gateway.go — Biz 适配器  │
│  internal/service/chat_pending.go   — PendingMessageQueue（待下沉）│
├─────────────────────────────────────────────────────┤
│  Biz 层                                             │
│  internal/biz/chat_usecase.go — 运行编排 Usecase    │
│    ↓ ChatRunGateway / ChatPendingQueue 接口         │
├─────────────────────────────────────────────────────┤
│  Runtime 层（Agent 运行时边界，非 biz）              │
│  internal/runtime/run_registry.go  — RunRegistry    │
│  internal/runtime/gateway.go       — RunGateway 接口│
│  internal/runtime/runner_manager.go — RunnerManager │
├─────────────────────────────────────────────────────┤
│  Agent 层                                           │
│  internal/agent/trpc_runtime.go — NewTRPCRunner     │
│    ↓ AwaitUserReplyRouting 由 RunnerManager 注入    │
└─────────────────────────────────────────────────────┘
```

> **分层说明**：`RunRegistry` 放在 `internal/runtime` 而非 `internal/biz`，因其持有 `trpcrunner.Runner` 引用并直接调用框架 API（`EnqueueUserMessage` / `Cancel`）。Biz 层通过 `ChatRunGateway` 接口与之解耦，遵守「biz 不 import trpc-agent-go」红线。

### 2.2 数据流

**正常聊天流程**：

```
用户消息 → ChatService.nativeSendChatMessage
              ↓
           chatUC.HasActiveRun(sessionID)?
              ├─ Yes → chatUC.EnqueueUserMessage
              │           ├─ RunRegistry.EnqueueUserMessage (SteerableRunner)
              │           └─ 降级 PendingMessageQueue.Enqueue
              └─ No  → RunRegistry.StoreCancelable → StoreRunner → Agent Turn
                         ↓
                      RunnerManager.NewTurnRunner (AwaitUserReplyRouting)
                         ↓
                      RunRegistry.Finish → chatUC.DequeuePendingMessage
```

**AwaitUserReply 流程**：

```
Agent 调用 await_user_reply 工具（ServiceTool + makeAwaitReplyFunc）
              ↓
         RunRegistry.SetStatus → "awaiting_user"
              ↓
         前端轮询 GetRunStatus → 显示回复 UI
              ↓
         用户提交回复 → AwaitUserReply API
              ↓
         chatUC await channel 投递回复 → Agent 恢复执行
```

---

## 三、接口设计（已实现）

### 3.1 RunGateway — 运行控制面

```go
// internal/runtime/gateway.go

type RunGateway interface {
    HasActive(sessionID string) bool
    Cancel(sessionID string) (bool, string)
    EnqueueUserMessage(sessionID, content string) (bool, error)
    GetStatus(sessionID string) (RunStatusEntry, bool)
    ActiveRunner(sessionID string) (trpcrunner.Runner, string, bool)
}
```

`RunRegistry` 实现 `RunGateway`；`ChatService.RunGateway()` 暴露给 WS / Channel / Cron。

### 3.2 RunRegistry — 运行注册表

```go
// internal/runtime/run_registry.go

type RunRegistry struct { /* activeRuns, pendingCancels, runStatuses */ }

func (r *RunRegistry) HasActive(sessionID string) bool
func (r *RunRegistry) StorePlaceholder(sessionID string)
func (r *RunRegistry) StoreRunner(sessionID, runID string, runner trpcrunner.Runner)
func (r *RunRegistry) StoreCancelable(sessionID, runID string, cancel context.CancelFunc)
func (r *RunRegistry) Finish(sessionID string)
func (r *RunRegistry) Cancel(sessionID string) (bool, string)
func (r *RunRegistry) EnqueueUserMessage(sessionID, content string) (bool, error)
func (r *RunRegistry) SetStatus(sessionID, runID, status, errMsg string)
func (r *RunRegistry) GetStatus(sessionID string) (RunStatusEntry, bool)
func (r *RunRegistry) ActiveRunner(sessionID string) (trpcrunner.Runner, string, bool)
```

### 3.3 ChatUsecase — Biz 编排

```go
// internal/biz/chat_usecase.go

type ChatUsecase struct {
    runs      ChatRunGateway      // → runGatewayAdapter → RunRegistry
    locker    ChatSessionLocker   // → sessionLockerAdapter
    pending   ChatPendingQueue    // → pendingQueueAdapter → PendingMessageQueue
    persist   ChatRunStatusPersister
    publisher ChatEventPublisher
    awaitChans sync.Map
}

func (uc *ChatUsecase) EnqueueUserMessage(sessionID, content string) (accepted, queued bool, pendingID string, err error)
func (uc *ChatUsecase) SetRunStatus(sessionID, runID, status, errMsg string)
func (uc *ChatUsecase) GetRunStatus(sessionID string) (ChatRunStatus, bool)
// ... GetPendingMessages / CancelPendingMessage / UpdatePendingMessage / DequeuePendingMessage
```

Service 适配器位于 `internal/service/chat_run_gateway.go`。

### 3.4 PendingMessageQueue — 消息排队（Service 层，待下沉）

```go
// internal/service/chat_pending.go

type PendingMessageQueue struct { /* map[sessionID][]PendingMessage, optional disk snapshot */ }

func (q *PendingMessageQueue) Enqueue(sessionID, content string) string  // 上限 32
func (q *PendingMessageQueue) Dequeue(sessionID string) (PendingMessage, bool)
func (q *PendingMessageQueue) List / Remove / Update
```

### 3.5 RunnerManager — TurnRunner 构建

```go
// internal/runtime/runner_manager.go

type TurnRunnerSpec struct {
    Plugins               []trpcplugin.Plugin
    AwaitUserReplyRouting bool  // true when AwaitHook != nil
    BuilderDeps           chatagent.TRPCBuilderDeps
    AgentFactoryKeys      []string
    ExtraOpts             []trpcrunner.Option
}

func (m *RunnerManager) NewTurnRunner(root trpcagent.Agent, spec TurnRunnerSpec) (trpcrunner.ManagedRunner, error)
```

### 3.6 Follow-up Queue（对话阶段连续发送）

> 产品需求：[1 chat.md §1.9](./1%20chat.md#19-对话阶段连续发送follow-up-queue--待发送队列)

**双路径**：Steerable 直注（`RunRegistry.EnqueueUserMessage`）优先；不支持时 `PendingMessageQueue` FIFO 降级。

**WS 通知**：`ChatEventPublisher.PublishMessageQueued` → `run_status` Envelope，`metadata: { status: "queued", hint: "message_queued" }`。  
`ChatService.publishMessageQueued` 已废弃，禁止新增调用。

**出队执行**：`ChatService.processPendingQueue` 在 turn defer 中调用 `chatUC.DequeuePendingMessage`，单 Agent / Team 共用。

**待优化（P2 UX）**：
- 前端运行中解除 `sending` 阻塞，支持 Cursor 式连续 Enter
- 监听 `message_queued` 即时刷新 Pending 列表
- 可选 `pending_enqueued` 专用 Envelope（P3）

---

## 四、Proto 层设计

### 4.1 现有 Chat Proto（不变）

RunStatus / Cancel / Pending / AwaitUserReply / EnqueueUserMessage 均在 `api/kratos/chat/v1/chat.proto`。  
Biz / Runtime 重构对 Proto 层透明，对外 API 路径不变。

### 4.2 Webhook Proto（已实现）

`api/kratos/gateway/v1/gateway.proto` — `GatewayService` 提供 Webhook CRUD：

| RPC | HTTP | 说明 |
|-----|------|------|
| `CreateWebhook` | `POST /v1/gateway/webhooks` | 创建回调配置 |
| `ListWebhooks` | `GET /v1/gateway/webhooks` | 列表 |
| `UpdateWebhook` | `PUT /v1/gateway/webhooks/{id}` | 更新（`enabled` 为 proto3 bool，PUT 未传时默认为 false） |
| `DeleteWebhook` | `DELETE /v1/gateway/webhooks/{id}` | 删除 |

---

## 五、Data 层设计（Webhook，已实现）

`internal/data/ent/schema/gateway_webhook.go` — 表 `gateway_webhooks`：

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 主键 |
| `name` | string | 显示名 |
| `url` | string | 回调 URL |
| `event_types_json` | string | 订阅事件 JSON 数组 |
| `secret` | string | HMAC 密钥 |
| `headers_json` | string | 自定义请求头 |
| `enabled` | bool | 是否启用 |
| `created_at` / `updated_at` | time | 审计字段 |

SQL 迁移：`docs/sql/19_gateway_webhook.sql`

---

## 六、Service 层设计

### 6.1 ChatService 结构（已实现）

```go
type ChatService struct {
    runs     *rt.RunRegistry          // 生命周期：StoreRunner/Finish/ActiveRunner/Cancel
    chatUC   *biz.ChatUsecase          // 编排：状态/排队/入队/锁/await channel
    webhooks *biz.WebhookDispatcher     // 出站：run 终态回调
    // awaitMetaCache / resumeInFlight 保留在 Service（传输层 resume 逻辑）
}
```

`NewChatService` 通过 `NewChatUsecaseFromDeps` 组装共享 `RunRegistry` + `PendingMessageQueue` + `sessionLockManager`。

### 6.2 AwaitUserReply 集成（已实现）

`RunnerManager.NewTurnRunner` 在 `AwaitHook != nil` 时设置 `AwaitUserReplyRouting: true`。  
Service 层 `makeAwaitReplyFunc` 处理当前轮次暂停；框架路由处理下一轮消息路由。两者互补。

### 6.3 Webhook 出站（已实现）

- **配置面**：`WebhookUsecase` + `GatewayService` — CRUD，校验 URL/事件类型
- **分发面**：`WebhookDispatcher.Dispatch` — 异步 fan-out，HMAC-SHA256 签名
- **触发面**：`callbackConsumer` 订阅 `run_status` 终态 → `WebhookDispatcher.Dispatch`

### 6.4 GatewayService（已实现）

`internal/service/gateway.go` — Webhook CRUD，委托 `WebhookUsecase`；运行回调由 `biz.callbackConsumer`（EventBus `run_status`）触发，不经 GatewayService RPC。

---

## 七、Webhook 触发点（已实现）

| 触发事件 | 触发位置 | eventType |
|----------|----------|-----------|
| 运行完成 | `ChatService.setRunStatus` 终态 | `run.completed` |
| 运行失败 | `ChatService.setRunStatus` 终态 | `run.failed` |
| 运行取消 | `StopGeneration` + `setRunStatus` 终态 | `run.cancelled` |

触发方式：`PublishRunStatus` → EventBus → `callbackConsumer` → `WebhookDispatcher.Dispatch`（异步，不阻塞主流程）。

---

## 八、涉及文件清单

| 文件 | 状态 | 说明 |
|------|------|------|
| `internal/runtime/run_registry.go` | ✅ | RunRegistry 运行注册表 |
| `internal/runtime/gateway.go` | ✅ | RunGateway 接口 |
| `internal/runtime/runner_manager.go` | ✅ | RunnerManager TurnRunner 构建 |
| `internal/biz/chat_usecase.go` | ✅ | ChatUsecase 编排 |
| `internal/service/chat_run_gateway.go` | ✅ | Biz 适配器 + NewChatUsecaseFromDeps |
| `internal/service/chat_pending.go` | 🟡 | PendingMessageQueue（待下沉） |
| `internal/service/chat.go` | ✅ | ChatService 委托 chatUC |
| `internal/service/chat_native.go` | ✅ | 发送路径 + Team 委派 |
| `internal/service/trpc_turn.go` | ✅ | 单 Agent Turn + processPendingQueue |
| `internal/biz/webhook.go` | ✅ | WebhookConfig + WebhookUsecase |
| `internal/biz/webhook_dispatcher.go` | ✅ | WebhookDispatcher |
| `internal/data/webhook.go` | ✅ | WebhookRepo |
| `api/kratos/gateway/v1/gateway.proto` | ✅ | Webhook CRUD Proto |
| `internal/service/gateway.go` | ✅ | GatewayService |
