# Gateway 网关模块 — 实现设计文档

> 对应需求：[35 gateway.md](./35%20gateway.md)
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
> 关联设计：[40 runner.design.md](./40%20runner.design.md)（Runner 运行器，Gateway 侧重网关编排层）

---

## 一、模块概述

会话网关：将散落在 `ChatService` 的并发控制、消息排队、运行状态、用户回复路由逻辑提取为独立 Biz 层，新增出站 Webhook 回调系统，并启用 trpc 框架原生 `AwaitUserReplyRouting` 和 `SteerableRunner`。

**设计目标**：
1. 将 `activeRuns`/`pendingQueue`/`runStatuses`/`awaitChans` 从 `ChatService` 提取到 Biz 层
2. 启用 trpc `WithAwaitUserReplyRouting(true)`，与现有 `makeAwaitReplyFunc` 协同
3. 联调 `SteerableRunner.EnqueueUserMessage`，替代手动 `pendingQueue`
4. 新增 Webhook 回调系统（持久化 + 分发）
5. 保持现有 Chat API 行为不变

**与 Runner 模块的边界**：
- Runner（40 runner.design.md）：负责 Agent 运行器的构建和生命周期（AgentFactory、PluginManager、ManagedRunner 封装）
- Gateway（本文）：负责运行编排层（并发闸门、消息排队、状态查询、Webhook 出站），是 Runner 的上层编排

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
│  internal/service/chat.go — ChatService             │
│    ↓ 调用 Biz 层接口，不再直接持有 sync.Map          │
├─────────────────────────────────────────────────────┤
│  Biz 层                                             │
│  internal/biz/run_registry.go   — 运行注册表        │
│  internal/biz/message_queue.go  — 消息排队          │
│  internal/biz/gateway.go        — GatewayUsecase    │
│  internal/biz/webhook.go        — WebhookConfig +   │
│                                    WebhookUsecase   │
│  internal/biz/webhook_dispatcher.go — 回调分发      │
├─────────────────────────────────────────────────────┤
│  Data 层                                            │
│  internal/data/webhook.go       — WebhookRepo       │
│  internal/data/ent/schema/      — GatewayWebhook    │
├─────────────────────────────────────────────────────┤
│  Agent 层                                           │
│  internal/agent/trpc_runtime.go — Runner 构建       │
│    ↓ 启用 WithAwaitUserReplyRouting(true)           │
│    ↓ 使用 SteerableRunner.EnqueueUserMessage        │
└─────────────────────────────────────────────────────┘
```

### 2.2 数据流

**正常聊天流程**（提取后）：

```
用户消息 → ChatService.SendChatMessage
              ↓
           RunRegistry.IsRunning(sessionID)?
              ├─ Yes → MessageQueue.Enqueue → 返回排队确认
              └─ No  → RunRegistry.Register → 执行 Agent Turn
                         ↓
                      Runner.Run (with AwaitUserReplyRouting)
                         ↓
                      RunRegistry.Unregister → WebhookDispatcher.Dispatch
                         ↓
                      MessageQueue.Dequeue → 处理排队消息
```

**AwaitUserReply 流程**：

```
Agent 调用 await_user_reply 工具
              ↓
         trpc Runner 记录路由到 Session State（框架原生）
              ↓
         RunRegistry.UpdateStatus → "awaiting_user"
              ↓
         前端轮询 GetRunStatus → 显示回复 UI
              ↓
         用户提交回复 → AwaitUserReply API
              ↓
         awaitChans 投递回复 → Agent 恢复执行
```

---

## 三、Biz 层接口设计

### 3.1 领域模型

```go
// internal/biz/gateway.go

type RunStatusInfo struct {
    RunID       string
    SessionID   string
    AgentID     string
    AgentName   string
    Status      string  // idle | pending | running | awaiting_user | completed | failed | cancelled
    ErrMsg      string
    EventCount  int
    StartedAt   time.Time
    LastEventAt time.Time
}

type QueuedMsg struct {
    ID        string
    SessionID string
    Content   string
    Status    string  // pending | delivered | expired
    CreatedAt string
}

type WebhookConfig struct {
    ID             string
    Name           string
    URL            string
    EventTypesJSON string   // ["run.completed","run.failed","run.cancelled"]
    Secret         string
    Headers        map[string]string
    Enabled        bool
    CreatedAt      string
    UpdatedAt      string
}
```

### 3.2 RunRegistry — 运行注册表

```go
// internal/biz/run_registry.go

type RunRegistry struct {
    // 内部维护: runs map[runID]*ActiveRun, bySession map[sessionID]runID
}

// 注册一个新运行，若 sessionID 已有活跃运行则返回错误
func (r *RunRegistry) Register(runID, sessionID, agentID, agentName string, cancel context.CancelFunc) error

// 注销运行，清理 sessionID 映射
func (r *RunRegistry) Unregister(runID string)

// 检查 sessionID 是否有活跃运行
func (r *RunRegistry) IsRunning(sessionID string) bool

// 取消运行（调用 cancel func + 更新状态）
func (r *RunRegistry) CancelRun(runID string) bool

// 按 runID 查询运行状态
func (r *RunRegistry) GetStatus(runID string) (RunStatusInfo, bool)

// 按 sessionID 查询运行状态
func (r *RunRegistry) GetStatusBySession(sessionID string) (RunStatusInfo, bool)

// 更新运行状态
func (r *RunRegistry) UpdateStatus(runID, status, errMsg string)

// 递增事件计数
func (r *RunRegistry) IncrementEventCount(runID string)
```

### 3.3 MessageQueue — 消息排队

```go
// internal/biz/message_queue.go

type MessageQueue struct {
    // 内部维护: queues map[sessionID][]QueuedMsg
}

// 排队一条消息，返回排队 ID；若达到上限（32 条/会话）返回空字符串
func (q *MessageQueue) Enqueue(sessionID, content string) string

// 取出队首消息
func (q *MessageQueue) Dequeue(sessionID string) (QueuedMsg, bool)

// 列出某会话的所有排队消息
func (q *MessageQueue) List(sessionID string) []QueuedMsg

// 取消指定排队消息
func (q *MessageQueue) Cancel(sessionID, entryID string) bool

// 更新指定排队消息内容
func (q *MessageQueue) Update(sessionID, entryID, newContent string) bool
```

### 3.4 WebhookRepository — Webhook 持久化接口

```go
// internal/biz/webhook.go

type WebhookRepository interface {
    Create(ctx context.Context, w WebhookConfig) (WebhookConfig, error)
    Get(ctx context.Context, id string) (WebhookConfig, error)
    List(ctx context.Context) ([]WebhookConfig, error)
    Update(ctx context.Context, w WebhookConfig) (WebhookConfig, error)
    Delete(ctx context.Context, id string) error
}
```

### 3.5 WebhookUsecase

```go
// internal/biz/webhook.go

type WebhookUsecase struct {
    repo WebhookRepository
}

func NewWebhookUsecase(repo WebhookRepository) *WebhookUsecase

func (uc *WebhookUsecase) Create(ctx context.Context, w WebhookConfig) (WebhookConfig, error)
func (uc *WebhookUsecase) List(ctx context.Context) ([]WebhookConfig, error)
func (uc *WebhookUsecase) Update(ctx context.Context, w WebhookConfig) (WebhookConfig, error)
func (uc *WebhookUsecase) Delete(ctx context.Context, id string) error
```

### 3.6 WebhookDispatcher — 回调分发

```go
// internal/biz/webhook_dispatcher.go

type WebhookDispatcher struct {
    repo   WebhookRepository
    client *http.Client  // Timeout: 10s
}

type WebhookPayload struct {
    EventType string         `json:"event_type"`  // run.completed | run.failed | run.cancelled
    RunID     string         `json:"run_id"`
    SessionID string         `json:"session_id"`
    Status    string         `json:"status"`
    Timestamp string         `json:"timestamp"`
    Data      map[string]any `json:"data,omitempty"`
}

// 异步分发回调给所有匹配事件类型的 Webhook
func (d *WebhookDispatcher) Dispatch(ctx context.Context, eventType, runID, sessionID, status string, data map[string]any)
```

**签名机制**：回调请求携带 `X-Webhook-Signature` Header，值为 `HMAC-SHA256(payload, secret)` 的十六进制编码。

### 3.7 GatewayUsecase — 编排入口

```go
// internal/biz/gateway.go

type GatewayUsecase struct {
    registry   *RunRegistry
    queue      *MessageQueue
    webhooks   *WebhookDispatcher
}

func NewGatewayUsecase(registry *RunRegistry, queue *MessageQueue, webhooks *WebhookDispatcher) *GatewayUsecase

func (uc *GatewayUsecase) GetRunStatus(ctx context.Context, runID string) (RunStatusInfo, error)
func (uc *GatewayUsecase) GetRunStatusBySession(ctx context.Context, sessionID string) (RunStatusInfo, error)
func (uc *GatewayUsecase) CancelRun(ctx context.Context, runID string) (bool, error)
func (uc *GatewayUsecase) QueueMessage(ctx context.Context, sessionID, content string) (string, error)
func (uc *GatewayUsecase) ListQueuedMessages(ctx context.Context, sessionID string) ([]QueuedMsg, error)
func (uc *GatewayUsecase) CancelQueuedMessage(ctx context.Context, sessionID, entryID string) (bool, error)
func (uc *GatewayUsecase) UpdateQueuedMessage(ctx context.Context, sessionID, entryID, newContent string) (bool, error)
```

---

## 四、Proto 层设计

### 4.1 设计决策：不新建独立 Gateway Proto

**理由**：
- RunStatus / Cancel / Pending / AwaitUserReply 已在 `api/kratos/chat/v1/chat.proto` 中定义
- 这些 API 的消费者是 Chat 前端，语义上属于 Chat 会话管理
- 新建独立 Gateway Proto 会引入服务间调用复杂度，且与现有前端 API 不兼容

**方案**：保持现有 Chat Proto 不变，Biz 层重构对 Proto 层透明。ChatService 内部委托给 `GatewayUsecase`，对外 API 路径不变。

### 4.2 新增 Webhook Proto

仅 Webhook CRUD 需要新增 Proto 定义：

```protobuf
// api/kratos/gateway/v1/gateway.proto

syntax = "proto3";

package kratos.gateway.v1;

import "google/api/annotations.proto";
import "google/api/field_behavior.proto";
import "google/protobuf/empty.proto";

option go_package = "aranea-agents/api/kratos/gateway/v1;v1";

message CreateWebhookRequest {
  string name = 1 [(google.api.field_behavior) = REQUIRED];
  string url = 2 [(google.api.field_behavior) = REQUIRED];
  string event_types_json = 3;  // ["run.completed","run.failed","run.cancelled"]
  string secret = 4;
  map<string, string> headers = 5;
}

message Webhook {
  string id = 1;
  string name = 2;
  string url = 3;
  string event_types_json = 4;
  string secret = 5;
  map<string, string> headers = 6;
  bool enabled = 7;
  string created_at = 8;
  string updated_at = 9;
}

message ListWebhooksRequest {}

message ListWebhooksResponse {
  repeated Webhook items = 1;
}

message UpdateWebhookRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string name = 2;
  string url = 3;
  string event_types_json = 4;
  string secret = 5;
  map<string, string> headers = 6;
  bool enabled = 7;
}

message DeleteWebhookRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
}

service GatewayService {
  rpc CreateWebhook(CreateWebhookRequest) returns (Webhook) {
    option (google.api.http) = { post: "/v1/gateway/webhooks" body: "*" };
  }
  rpc ListWebhooks(ListWebhooksRequest) returns (ListWebhooksResponse) {
    option (google.api.http) = { get: "/v1/gateway/webhooks" };
  }
  rpc UpdateWebhook(UpdateWebhookRequest) returns (Webhook) {
    option (google.api.http) = { put: "/v1/gateway/webhooks/{id}" body: "*" };
  }
  rpc DeleteWebhook(DeleteWebhookRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { delete: "/v1/gateway/webhooks/{id}" };
  }
}
```

---

## 五、Data 层设计

### 5.1 Ent Schema

```go
// internal/data/ent/schema/gateway_webhook.go

type GatewayWebhook struct {
    ent.Schema
}

Fields:
- id        String  DefaultFunc: uuid.NewString
- name      String
- url       String
- event_types_json  String  Default: "[]"
- secret    String  Default: ""
- headers_json      String  Default: "{}"
- enabled   Bool    Default: true
- created_at String  DefaultFunc: nowRFC3339
- updated_at String  DefaultFunc: nowRFC3339, UpdateDefault: nowRFC3339
```

### 5.2 WebhookRepo 实现

```go
// internal/data/webhook.go

type webhookRepo struct {
    data *Data
}

func NewWebhookRepo(d *Data) biz.WebhookRepository
```

实现 `biz.WebhookRepository` 接口，通过 Ent Client 操作 `gateway_webhook` 表。

---

## 六、Service 层设计

### 6.1 ChatService 重构

ChatService 将 `activeRuns`/`pendingQueue`/`runStatuses`/`awaitChans` 四个 `sync.Map` 替换为 `GatewayUsecase`：

```go
type ChatService struct {
    chatv1.UnimplementedChatServiceServer
    // ... 现有字段 ...
    gateway *biz.GatewayUsecase  // 替代 activeRuns/pendingQueue/runStatuses/awaitChans
}
```

**方法映射**：

| 原实现 | 重构后 |
|--------|--------|
| `s.activeRuns.Load(sessionID)` | `s.gateway.registry.IsRunning(sessionID)` |
| `s.activeRuns.Store(sessionID, guard)` | `s.gateway.registry.Register(...)` |
| `s.activeRuns.Delete(sessionID)` | `s.gateway.registry.Unregister(runID)` |
| `s.pendingQueue.Load/Store` | `s.gateway.queue.Enqueue/Dequeue/List/Cancel/Update` |
| `s.runStatuses.Store/Load` | `s.gateway.registry.UpdateStatus/GetStatusBySession` |
| `s.awaitChans.Store/Load/Delete` | 保留在 ChatService（与 `makeAwaitReplyFunc` 紧耦合） |

### 6.2 GatewayService（新增）

```go
// internal/service/gateway.go

type GatewayService struct {
    v1.UnimplementedGatewayServiceServer
    wh *biz.WebhookUsecase
}

func NewGatewayService(wh *biz.WebhookUsecase) *GatewayService
```

仅处理 Webhook CRUD，运行管理仍通过 ChatService 暴露。

### 6.3 AwaitUserReply 集成

在 `internal/agent/trpc_runtime.go` 的 `NewTRPCRunner` 中启用 `WithAwaitUserReplyRouting(true)`：

```go
opts = append(opts, trpcrunner.WithAwaitUserReplyRouting(true))
```

**兼容性**：现有 `makeAwaitReplyFunc` 通过 ServiceTool 注入，与框架原生 `await_user_reply` 路由不冲突。框架路由处理"下一轮消息路由"，`makeAwaitReplyFunc` 处理"当前轮次暂停等待回复"，两者互补。

### 6.4 SteerableRunner 联调

在 `runSingleAgentViaTRPC` 中，当用户在运行中发送消息时：

```go
// 替代 s.enqueuePending(sessionID, content)
if sr, ok := runner.(trpcrunner.SteerableRunner); ok {
    sr.EnqueueUserMessage(sessionID, trpcmodel.NewUserMessage(content))
}
```

**降级策略**：若 Runner 不实现 `SteerableRunner`，回退到 `MessageQueue.Enqueue`。

---

## 七、Webhook 触发点

| 触发事件 | 触发位置 | eventType |
|----------|----------|-----------|
| 运行完成 | `ChatService` 运行结束后 | `run.completed` |
| 运行失败 | `ChatService` 运行出错后 | `run.failed` |
| 运行取消 | `StopGeneration` / `CancelRun` | `run.cancelled` |

触发方式：在 `RunRegistry.Unregister` 或 `RunRegistry.UpdateStatus` 后调用 `WebhookDispatcher.Dispatch`。

---

## 八、涉及文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/biz/run_registry.go` | 新建 | RunRegistry 运行注册表 |
| `internal/biz/message_queue.go` | 新建 | MessageQueue 消息排队 |
| `internal/biz/gateway.go` | 新建 | GatewayUsecase + 领域模型 |
| `internal/biz/webhook.go` | 新建 | WebhookConfig 模型 + WebhookUsecase + WebhookRepository 接口 |
| `internal/biz/webhook_dispatcher.go` | 新建 | WebhookDispatcher 回调分发 |
| `internal/biz/biz.go` | 修改 | ProviderSet 新增 |
| `internal/data/webhook.go` | 新建 | WebhookRepo 实现 |
| `internal/data/ent/schema/gateway_webhook.go` | 新建 | Webhook Ent Schema |
| `internal/data/data.go` | 修改 | Wire 注入 WebhookRepo |
| `api/kratos/gateway/v1/gateway.proto` | 新建 | Webhook CRUD Proto |
| `internal/service/gateway.go` | 新建 | GatewayService（Webhook CRUD） |
| `internal/service/chat.go` | 修改 | 使用 GatewayUsecase 替代 sync.Map |
| `internal/service/chat_native.go` | 修改 | 使用 registry/queue 替代 activeRuns/pendingQueue |
| `internal/service/trpc_turn.go` | 修改 | 使用 registry/queue |
| `internal/agent/trpc_runtime.go` | 修改 | 启用 WithAwaitUserReplyRouting + SteerableRunner |
| `internal/server/http.go` | 修改 | 注册 GatewayService |
| `cmd/admin/wire.go` | 修改 | Wire 注入更新 |
