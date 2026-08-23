# Gateway 网关模块 — 实现设计文档

> 对应需求：[35 gateway.md](./35%20gateway.md)
> 关联设计：[40 runner.design.md](./40%20runner.design.md)（Runner 运行器，Gateway 侧重网关编排层）

---

## 一、模块概述

会话网关：将运行编排（并发闸门、消息排队、状态查询、Steerable 入队）从 `ChatService` 提取为可复用组件，新增出站 Webhook 回调系统，并启用 trpc 框架原生 `AwaitUserReplyRouting` 和 `SteerableRunner`。

**设计目标**：
1. `RunRegistry` / `RunGateway` 位于 `internal/runtime`，供 Chat / Team / Cron / Channel / WS 共用
2. `ChatUsecase`（Biz 层）编排运行状态、排队、入队、await channel
3. `ChatOrchestrator`（Service 层）核心 Turn 编排，实现 `biz.TurnExecutor`；`ChatService` 为薄传输桥
4. `TurnPipeline` 显式 Ingress → Service → Executor → Projector 管道
5. `AdmissionGate` 准入控制：放行/入队/拒绝三路决策
6. `RunnerManager` 统一 TurnRunner 构建，条件启用 `AwaitUserReplyRouting`
7. SteerableRunner 入队优先，不支持时降级 `PendingMessageQueue`
8. 新增 Webhook 回调系统（持久化 + 分发）

**与 Runner 模块的边界**：
- **Runner（40）**：Agent 运行器构建和生命周期（AgentFactory、PluginManager、ManagedRunner 封装）
- **Gateway（本文）**：运行编排层（并发闸门、消息排队、状态查询、Webhook 出站），是 Runner 的上层编排

---

## 二、架构设计

### 2.1 分层架构

```
┌─────────────────────────────────────────────────────┐
│  Server 层                                          │
│  internal/server/http.go — 注册 ChatService         │
│  internal/server/ws.go   — WebSocket 事件通道       │
├─────────────────────────────────────────────────────┤
│  Service 层                                         │
│  internal/service/chat.go                — ChatService RPC（薄传输桥）│
│  internal/service/chat_orchestrator.go   — ChatOrchestrator（核心编排）│
│  internal/service/chat_orchestrator_turn.go — Turn 执行 + processPendingQueue│
│  internal/service/turn_pipeline.go       — TurnPipeline 管道         │
│  internal/service/chat_turn_admission.go — 准入适配器               │
│  internal/service/chat_native.go         — 发送/Team/A2A/Cron 入口  │
│  internal/service/chat_run_gateway.go    — Biz 适配器               │
│  internal/service/chat_enqueue.go        — 入队拒绝辅助             │
├─────────────────────────────────────────────────────┤
│  Biz 层                                             │
│  internal/biz/chat_usecase.go — 运行编排 Usecase    │
│    ↓ ChatRunGateway / ChatPendingQueue 接口         │
│  internal/biz/webhook.go — WebhookUsecase           │
│  internal/biz/webhook_dispatcher.go — WebhookDispatcher│
│  internal/biz/event_bus_callback_consumer.go — callbackConsumer│
├─────────────────────────────────────────────────────┤
│  Runtime 层（Agent 运行时边界，非 biz）              │
│  internal/runtime/run_registry.go  — RunRegistry    │
│  internal/runtime/gateway.go       — RunGateway 接口│
│  internal/runtime/runner_manager.go — RunnerManager │
│  internal/runtime/pending_queue.go — PendingMessageQueue│
│  internal/runtime/turn/admission_gate.go — AdmissionGate│
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
用户消息 → ChatService.SendChatMessage
              ↓
           ChatOrchestrator.Execute (biz.TurnExecutor)
              ↓
           TurnPipeline.Run（可选持久化路径）
              ├─ TurnIngress → 标准化为 TurnIntent
              ├─ TurnService.AdmitTurn → 准入判定
              │     └─ AdmissionGate.Check → Proceed / Queued / Reject
              ├─ TurnExecutor.ExecuteTurn → ChatOrchestrator.RunNativeAgentTurnWithOutcome
              │     ├─ 有活跃运行 → chatUC.EnqueueUserMessage
              │     │     ├─ RunRegistry.EnqueueUserMessage (SteerableRunner)
              │     │     └─ 降级 PendingMessageQueue.Enqueue
              │     └─ 无活跃运行 → RunRegistry.StoreCancelable → StoreRunner → Agent Turn
              │           ↓
              │        RunnerManager.NewTurnRunner (AwaitUserReplyRouting)
              │           ↓
              │        RunRegistry.Finish → processPendingQueue
              └─ TurnProjector → 投影到 WS / Channel / Monitor
```

**AwaitUserReply 流程**：

```
Agent 调用 await_user_reply 工具（ServiceTool + makeAwaitReplyFunc）
              ↓
         ChatOrchestrator.setRunStatusWithAwait → "awaiting_user"
              ↓
         前端轮询 GetRunStatus → 显示回复 UI
              ↓
         用户提交回复 → AwaitUserReply API
              ↓
         chatUC await channel 投递回复 → Agent 恢复执行
```

### 2.3 trpc 框架参照

```
pkg/trpc-agent-go/runner/
├── runner.go              # Runner/ManagedRunner/SteerableRunner 接口
├── await_user_reply.go    # AwaitUserReply 路由（会话状态驱动）
├── ralph_loop.go          # RalphLoop 迭代执行
└── agent_lookup.go        # Agent 查找
```

**Runner 接口层级**：

| 接口 | 方法 | 说明 |
|------|------|------|
| `Runner` | `Run` + `Close` | 基础运行器 |
| `ManagedRunner` | + `Cancel` + `RunStatus` | 可管理的运行器 |
| `SteerableRunner` | + `EnqueueUserMessage` | 可转向的运行器（排队消息注入） |

**AwaitUserReply**：当 Agent 调用 `await_user_reply` 工具时，Runner 记录路由信息到 Session State。`RunnerManager.NewTurnRunner` 在 `AwaitHook != nil` 时传入 `AwaitUserReplyRouting: true`。

**QueuedUserMessage / SteerableRunner**：`RunRegistry.EnqueueUserMessage` 调用 `trpcrunner.EnqueueUserMessage`；不支持时降级到 `PendingMessageQueue`。

---

## 三、接口设计

### 3.1 RunGateway — 运行控制面

```go
// internal/runtime/gateway.go

type RunGateway interface {
    HasActive(sessionID string) bool
    Cancel(sessionID, reason string) (bool, string)
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
func (r *RunRegistry) Cancel(sessionID, reason string) (bool, string)
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

func (uc *ChatUsecase) EnqueueUserMessage(sessionID, content string, mergeFollowup bool) (accepted, queued bool, pendingID, rejectReason string, err error)
func (uc *ChatUsecase) SetRunStatus(ctx context.Context, sessionID, runID, status, errMsg string)
func (uc *ChatUsecase) GetRunStatus(sessionID string) (ChatRunStatus, bool)
// ... GetPendingMessages / EnqueuePendingMessage / CancelPendingMessage / UpdatePendingMessage / DequeuePendingMessage
```

Service 适配器位于 `internal/service/chat_run_gateway.go`。

### 3.4 PendingMessageQueue — 消息排队（Runtime 层，已下沉）

```go
// internal/runtime/pending_queue.go

type PendingMessage struct { ID, Content, Status string; CreatedAt time.Time }
type PendingMessageQueue struct { /* queues map[string][]PendingMessage, optional disk snapshot + PendingQueueStore */ }

const MaxPendingPerSession = 32

func (q *PendingMessageQueue) Enqueue(sessionID, content string) string       // 上限 32
func (q *PendingMessageQueue) EnqueueFollowup(sessionID, content, separator string) string // 合并到最后一条
func (q *PendingMessageQueue) Dequeue(sessionID string) (PendingMessage, bool)
func (q *PendingMessageQueue) List(sessionID string) []PendingMessage
func (q *PendingMessageQueue) Remove(sessionID, entryID string) bool
func (q *PendingMessageQueue) Update(sessionID, entryID, newContent string) bool
func (q *PendingMessageQueue) Close()  // 含快照持久化
```

> **下沉完成**：`PendingMessageQueue` 已从 `internal/service/chat_pending.go`（已删除）迁移至 `internal/runtime/pending_queue.go`。Service 层通过 `pendingQueueAdapter`（`chat_run_gateway.go`）做类型适配。

### 3.5 RunnerManager — TurnRunner 构建

```go
// internal/runtime/runner_manager.go

type TurnRunnerSpec struct {
    Plugins               []trpcplugin.Plugin
    AwaitUserReplyRouting bool                // true when AwaitHook != nil
    BuilderDeps           chatagent.TRPCBuilderDeps
    AgentFactoryKeys      []string
    LookupAgents          map[string]trpcagent.Agent  // Agent 查找表
    RalphLoop             *trpcrunner.RalphLoopConfig // 可选迭代执行
    ExtraOpts             []trpcrunner.Option
    RegistryKey           string               // 长驻 runner 注册键
}

func (m *RunnerManager) NewTurnRunner(root trpcagent.Agent, spec TurnRunnerSpec) (trpcrunner.ManagedRunner, error)
func (m *RunnerManager) CloseRunner(key string) error  // 关闭并反注册长驻 runner
```

### 3.6 ChatOrchestrator — 核心编排器

```go
// internal/service/chat_orchestrator.go

type ChatOrchestrator struct {
    core         chatTurnCoreDeps      // 运行时核心依赖
    channelDeps  ChatChannelDeps       // 渠道集成
    usageDeps    ChatUsageDeps         // 用量统计
    teamExecDeps ChatTeamDeps          // 团队执行
    evoDeps      ChatEvolutionDeps     // 演化能力
    infraDeps    ChatInfraDeps         // 基础设施

    runs       *rt.RunRegistry          // 运行注册表
    chatUC     *biz.ChatUsecase         // Biz 编排
    turnLC     chatTurnLifecycle        // turn 生命周期（状态流转/记录/事件发布）
    runMgr     chatRunManager           // 运行管理（状态/排队/await/session run）
    agentBuild agentBuildDirector       // Agent 构建导向

    sweepStop chan struct{}
}

func (o *ChatOrchestrator) Execute(ctx, TurnInput) (TurnResult, error)  // biz.TurnExecutor
func (o *ChatOrchestrator) RunNativeAgentTurnWithOutcome(ctx, TurnInput) (NativeTurnResult, error)
func (o *ChatOrchestrator) CancelRun(ctx, sessionID string) bool
func (o *ChatOrchestrator) EnqueueUserMessage(sessionID, content string, mergeFollowup bool) (accepted, queued bool, pendingID, rejectReason string, err error)
// ... GetRunStatus / GetPendingMessages / processPendingQueue / setRunStatusWithAwait
```

> **ChatService 为薄传输桥**：`ChatService` 将所有编排工作委托给 `ChatOrchestrator`，自身仅负责 RPC 入口、`biz.NativeTurnGateway` 实现和 `RunGateway()` 暴露。

### 3.7 TurnPipeline — Turn 管道

```go
// internal/service/turn_pipeline.go

type TurnPipeline struct {
    Service   TurnService    // 准入 + 持久化生命周期
    Executor  TurnExecutor   // 执行已准入的 turn
    Projector TurnProjector  // 投影到 WS / Channel / Monitor
    Lg        loggateway.Logger
}

func (p *TurnPipeline) Run(ctx, TurnIntent) (Turn, NativeTurnResult, error)
```

管道阶段：`TurnIngress`（标准化）→ `TurnService.AdmitTurn`（准入）→ `TurnExecutor.ExecuteTurn`（执行）→ `TurnProjector.ProjectTurnEvent`（投影）。

### 3.8 AdmissionGate — 准入控制

```go
// internal/runtime/turn/admission_gate.go

type AdmissionAction int  // AdmissionProceed / AdmissionQueued / AdmissionRejectBusy / AdmissionRejectEnqueue
type AdmissionVerdict struct { Action AdmissionAction; PendingID, RejectReason string; Err error }

type AdmissionGate struct { /* deps: locker, enqueuer */ }
func (g *AdmissionGate) Check(input biz.TurnInput) AdmissionVerdict
```

Service 层适配器位于 `chat_turn_admission.go`，将 `ChatUsecase` 适配为 `turn.SessionLocker` 和 `turn.MessageEnqueuer`。

### 3.9 Follow-up Queue（对话阶段连续发送）

> 产品需求：[1 chat.md §1.9](./1%20chat.md#19-对话阶段连续发送follow-up-queue--待发送队列)

**双路径**：Steerable 直注（`RunRegistry.EnqueueUserMessage`）优先；不支持时 `PendingMessageQueue` FIFO 降级。

**WS 通知**：`ChatEventPublisher.PublishMessageQueued` → `run_status` Envelope，`metadata: { status: "queued", hint: "message_queued" }`。  
`ChatService.publishMessageQueued` 已废弃，禁止新增调用。

**出队执行**：`ChatOrchestrator.processPendingQueue`（定义于 `chat_orchestrator_turn_dispatch.go`，由 `chat_orchestrator_turn.go` 在 turn defer 中调用）调用 `chatUC.DequeuePendingMessage`，单 Agent / Team 共用。

---

## 四、Proto 层设计

### 4.1 现有 Chat Proto（不变）

RunStatus / Cancel / Pending / AwaitUserReply / EnqueueUserMessage 均在 `api/kratos/chat/v1/chat.proto`。  
Biz / Runtime 重构对 Proto 层透明，对外 API 路径不变。

### 4.2 Webhook Proto

`api/kratos/gateway/v1/gateway.proto` — `GatewayService` 提供 Webhook CRUD：

| RPC | HTTP | 说明 |
|-----|------|------|
| `CreateWebhook` | `POST /v1/gateway/webhooks` | 创建回调配置 |
| `ListWebhooks` | `GET /v1/gateway/webhooks` | 列表 |
| `UpdateWebhook` | `PUT /v1/gateway/webhooks/{id}` | 更新（`enabled` 为 optional，PUT 未传时保留原值） |
| `DeleteWebhook` | `DELETE /v1/gateway/webhooks/{id}` | 删除 |

---

## 五、Data 层设计（Webhook）

`internal/data/ent/schema/gateway_webhook.go` — 表 `gateway_webhooks`（通过 Ent Auto-Migration 创建，无独立 SQL 迁移文件）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | string | 主键，Immutable，MaxLen 64 |
| `name` | string | 显示名，MaxLen 256 |
| `url` | string | 回调 URL，MaxLen 2048 |
| `event_types_json` | Text | 订阅事件 JSON 数组，默认 `"[]"` |
| `secret` | string | HMAC 密钥，Sensitive，MaxLen 512 |
| `headers_json` | Text | 自定义请求头，默认 `"{}"` |
| `enabled` | bool | 是否启用，默认 `true` |
| `created_at` | string | 审计字段，默认 `""` |
| `updated_at` | string | 审计字段，默认 `""` |

> **安全**：`secret` 字段标记 `.Sensitive()`，防止日志泄漏。Service 层 `webhookToProto` 在 list/get 时脱敏返回（`maskSecret`），仅 create/update 返回明文。

---

## 六、Service 层设计

### 6.1 ChatService 结构

```go
type ChatService struct {
    chatv1.UnimplementedChatServiceServer
    orch         *ChatOrchestrator       // 核心编排（所有 turn 逻辑委托）
    turnPipeline *TurnPipeline          // 显式管道（可选持久化路径）
    lg           loggateway.Logger
}
```

`ChatService` 为薄传输桥，实现 `biz.NativeTurnGateway` 接口，将 RPC 调用委托给 `ChatOrchestrator`。`RunGateway()` 暴露 `*rt.RunRegistry` 给 WS / Channel / Cron。

### 6.2 AwaitUserReply 集成

`RunnerManager.NewTurnRunner` 在 `AwaitHook != nil` 时设置 `AwaitUserReplyRouting: true`。  
Service 层 `makeAwaitReplyFunc` 处理当前轮次暂停；框架路由处理下一轮消息路由。两者互补。

### 6.3 Webhook 出站

- **配置面**：`WebhookUsecase` + `GatewayService` — CRUD，校验 URL/事件类型
- **分发面**：`WebhookDispatcher.Dispatch` — 异步 fan-out，HMAC-SHA256 签名，3 次重试
- **触发面**：`callbackConsumer`（`internal/biz/event_bus_callback_consumer.go`）订阅 `run_status` 终态 → `WebhookDispatcher.Dispatch`
- **事件类型**：`run.completed` / `run.failed` / `run.cancelled` / `graph.task.status`

### 6.4 GatewayService

`internal/service/gateway.go` — Webhook CRUD，委托 `WebhookUsecase`；运行回调由 `biz.callbackConsumer`（EventBus `run_status`）触发，不经 GatewayService RPC。

---

## 七、Webhook 触发点

| 触发事件 | 触发位置 | eventType |
|----------|----------|-----------|
| 运行完成 | `ChatOrchestrator.setRunStatus` 终态 | `run.completed` |
| 运行失败 | `ChatOrchestrator.setRunStatus` 终态 | `run.failed` |
| 运行取消 | `StopGeneration` + `setRunStatus` 终态 | `run.cancelled` |
| 图任务状态 | Graph 执行引擎 | `graph.task.status` |

触发方式：`PublishRunStatus` → EventBus → `callbackConsumer` → `WebhookDispatcher.Dispatch`（异步，不阻塞主流程）。

> **事件类型常量**定义于 `internal/biz/webhook.go`：`WebhookEventRunCompleted` / `WebhookEventRunFailed` / `WebhookEventRunCancelled` / `WebhookEventGraphTaskStatus`。`RunStatusToWebhookEvent` 函数将终态 run status 映射为 webhook 事件类型。

---

## 八、涉及文件清单

> 文件的实现状态详见 [35-gateway.development.md §2 现状评估](./35-gateway.development.md#2-现状评估)

| 文件 | 说明 |
|------|------|
| `internal/runtime/run_registry.go` | RunRegistry 运行注册表 |
| `internal/runtime/gateway.go` | RunGateway 接口 |
| `internal/runtime/runner_manager.go` | RunnerManager TurnRunner 构建 |
| `internal/runtime/pending_queue.go` | PendingMessageQueue（已从 service 层下沉） |
| `internal/runtime/turn/admission_gate.go` | AdmissionGate 准入控制 |
| `internal/biz/chat_usecase.go` | ChatUsecase 编排 |
| `internal/biz/webhook.go` | WebhookConfig + WebhookUsecase + 事件常量 |
| `internal/biz/webhook_dispatcher.go` | WebhookDispatcher |
| `internal/biz/event_bus_callback_consumer.go` | callbackConsumer（EventBus → Webhook） |
| `internal/service/chat.go` | ChatService 薄传输桥 |
| `internal/service/chat_orchestrator.go` | ChatOrchestrator 核心编排器 |
| `internal/service/chat_orchestrator_turn.go` | Turn 执行 + processPendingQueue 调用 |
| `internal/service/chat_orchestrator_turn_dispatch.go` | processPendingQueue 定义 |
| `internal/service/turn_pipeline.go` | TurnPipeline 管道 |
| `internal/service/chat_turn_admission.go` | 准入适配器 |
| `internal/service/chat_native.go` | 发送/Team/A2A/Cron 入口 |
| `internal/service/chat_run_gateway.go` | Biz 适配器 + NewChatUsecaseFromDeps |
| `internal/service/chat_enqueue.go` | 入队拒绝辅助 |
| `internal/service/gateway.go` | GatewayService |
| `internal/data/webhook.go` | WebhookRepo |
| `internal/data/ent/schema/gateway_webhook.go` | GatewayWebhook Ent Schema |
| `api/kratos/gateway/v1/gateway.proto` | Webhook CRUD Proto |
