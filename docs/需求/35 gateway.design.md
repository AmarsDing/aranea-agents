# Gateway 网关模块 — 实现设计文档

> 对应需求：`35 gateway.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

会话网关：并发控制、运行状态查询、运行取消、AwaitUserReply 路由、消息排队。对标 trpc-agent-go `runner` 包网关能力。

---

## 二、Proto 层

### 2.1 待新增

```protobuf
service GatewayService {
  rpc GetRunStatus(GetRunStatusRequest) returns (RunStatus) {
    option (google.api.http) = { get: "/v1/gateway/runs/{run_id}/status" };
  }
  rpc CancelRun(CancelRunRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = { post: "/v1/gateway/runs/{run_id}/cancel" };
  }
  rpc AwaitUserReply(AwaitUserReplyRequest) returns (AwaitUserReplyResponse) {
    option (google.api.http) = { post: "/v1/gateway/await-reply" body: "*" };
  }
  rpc QueueMessage(QueueMessageRequest) returns (QueuedMessage) {
    option (google.api.http) = { post: "/v1/gateway/queue" body: "*" };
  }
  rpc ListQueuedMessages(ListQueuedMessagesRequest) returns (ListQueuedMessagesResponse) {
    option (google.api.http) = { get: "/v1/gateway/queue/{session_id}" };
  }
}
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type RunStatus struct {
    RunID      string
    SessionID  string
    AgentID    string
    Status     string  // "idle"/"running"/"waiting_user"/"completed"/"cancelled"
    StartedAt  string
    CurrentStep string
}

type QueuedMessage struct {
    ID        string
    SessionID string
    Content   string
    Status    string  // "pending"/"delivered"/"expired"
    CreatedAt string
}

type AwaitUserReply struct {
    SessionID string
    AgentID   string
    Prompt    string
    Timeout   int32
    Reply     string
}
```

### 3.2 Usecase

```go
type GatewayUsecase struct {
    runs    RunRegistry
    queue   MessageQueue
}

func (uc *GatewayUsecase) GetRunStatus(ctx, runID string) (RunStatus, error)
func (uc *GatewayUsecase) CancelRun(ctx, runID string) error
func (uc *GatewayUsecase) AwaitUserReply(ctx, req AwaitUserReply) (string, error)
func (uc *GatewayUsecase) QueueMessage(ctx, msg QueuedMessage) (QueuedMessage, error)
func (uc *GatewayUsecase) ListQueuedMessages(ctx, sessionID string) ([]QueuedMessage, error)
```

### 3.3 并发控制

```go
type RunRegistry struct {
    mu   sync.RWMutex
    runs map[string]*ActiveRun
}

type ActiveRun struct {
    RunID     string
    SessionID string
    Cancel    context.CancelFunc
    Status    string
}

func (r *RunRegistry) Start(runID, sessionID string, cancel context.CancelFunc) error
func (r *RunRegistry) IsRunning(sessionID string) bool
func (r *RunRegistry) Cancel(runID string) error
```

---

## 四、Data 层

### 4.1 消息队列

```go
// internal/gateway/queue.go
type MessageQueue struct {
    mu      sync.RWMutex
    queues  map[string][]QueuedMessage
}

func (q *MessageQueue) Enqueue(ctx, msg QueuedMessage) error
func (q *MessageQueue) Dequeue(ctx, sessionID string) ([]QueuedMessage, error)
```

---

## 五、Service 层

```go
func (s *GatewayService) GetRunStatus(ctx, req) (*RunStatus, error)
func (s *GatewayService) CancelRun(ctx, req) (*emptypb.Empty, error)
func (s *GatewayService) AwaitUserReply(ctx, req) (*AwaitUserReplyResponse, error)
func (s *GatewayService) QueueMessage(ctx, req) (*QueuedMessage, error)
```

---

## 六、Wire 注入

待新增：
```
biz.ProviderSet → NewGatewayUsecase, NewRunRegistry, NewMessageQueue
service.ProviderSet → NewGatewayService
```

---

## 七、Web 前端设计

### 7.1 组件

**RunStatusIndicator.vue**：运行状态指示器（嵌入 Chat 页面）

| 状态 | 显示 | 操作 |
|------|------|------|
| idle | 灰点 | — |
| running | 绿点动画 | 取消按钮 |
| waiting_user | 黄点 | 输入框聚焦 |
| completed | 蓝点 | — |
| cancelled | 红点 | — |

**QueuedMessageList.vue**：排队消息列表

### 7.2 API

```typescript
export async function getRunStatus(runId: string): Promise<RunStatus>
export async function cancelRun(runId: string): Promise<void>
export async function queueMessage(req: QueueMessageRequest): Promise<QueuedMessage>
export async function listQueuedMessages(sessionId: string): Promise<QueuedMessage[]>
```
