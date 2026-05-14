# Gateway 网关模块 — 实现设计文档

> 对应需求：`35 gateway.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

会话网关：并发控制、运行状态查询、运行取消、AwaitUserReply 路由、消息排队、Webhook 回调。对标 trpc-agent-go `runner` 包的 `ManagedRunner` 接口和网关能力。

**现有实现**：
- `ChatService.activeRuns`（`sync.Map`）— 已跟踪活跃运行
- `ChatService.pendingQueue`（`sync.Map`）— 已实现消息排队
- `StopGeneration` API — 已实现取消运行
- `GetPendingMessages` API — 已实现排队消息查询

**需增强**：
1. 将并发控制/排队逻辑从 ChatService 提取到独立的 Biz 层
2. 新增 `RunStatus` 查询 API（对齐 trpc `ManagedRunner.RunStatus`）
3. 集成 `WithAwaitUserReplyRouting(true)` 到 Runner 构建
4. 新增 Webhook 回调系统
5. 前端运行状态可视化

---

## 二、Proto 层

### 2.1 待新增

```protobuf
// api/kratos/gateway/v1/gateway.proto

syntax = "proto3";

package kratos.gateway.v1;

import "google/api/annotations.proto";
import "google/api/field_behavior.proto";
import "google/protobuf/empty.proto";
import "google/protobuf/timestamp.proto";

option go_package = "aranea-agents/api/kratos/gateway/v1;v1";

message GetRunStatusRequest {
  string run_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message RunStatus {
  string run_id = 1;
  string session_id = 2;
  string agent_id = 3;
  string agent_name = 4;
  string status = 5;          // "idle"/"running"/"waiting_user"/"completed"/"cancelled"/"failed"
  int32 event_count = 6;
  google.protobuf.Timestamp started_at = 7;
  google.protobuf.Timestamp last_event_at = 8;
}

message CancelRunRequest {
  string run_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message CancelRunResponse {
  bool cancelled = 1;
}

message QueueMessageRequest {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
  string content = 2 [(google.api.field_behavior) = REQUIRED];
}

message QueuedMessage {
  string id = 1;
  string session_id = 2;
  string content = 3;
  string status = 4;          // "pending"/"delivered"/"expired"
  string created_at = 5;
}

message ListQueuedMessagesRequest {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message ListQueuedMessagesResponse {
  repeated QueuedMessage items = 1;
}

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
  rpc GetRunStatus(GetRunStatusRequest) returns (RunStatus) {
    option (google.api.http) = { get: "/v1/gateway/runs/{run_id}/status" };
  }
  rpc CancelRun(CancelRunRequest) returns (CancelRunResponse) {
    option (google.api.http) = { post: "/v1/gateway/runs/{run_id}/cancel" body: "*" };
  }
  rpc QueueMessage(QueueMessageRequest) returns (QueuedMessage) {
    option (google.api.http) = { post: "/v1/gateway/queue" body: "*" };
  }
  rpc ListQueuedMessages(ListQueuedMessagesRequest) returns (ListQueuedMessagesResponse) {
    option (google.api.http) = { get: "/v1/gateway/queue/{session_id}" };
  }
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

## 三、Biz 层

### 3.1 领域模型

```go
// internal/biz/gateway.go

type RunStatusInfo struct {
    RunID       string
    SessionID   string
    AgentID     string
    AgentName   string
    Status      string  // "idle"/"running"/"waiting_user"/"completed"/"cancelled"/"failed"
    EventCount  int
    StartedAt   string
    LastEventAt string
}

type QueuedMsg struct {
    ID        string
    SessionID string
    Content   string
    Status    string  // "pending"/"delivered"/"expired"
    CreatedAt string
}

type WebhookConfig struct {
    ID             string
    Name           string
    URL            string
    EventTypesJSON string
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

type ActiveRun struct {
    RunID     string
    SessionID string
    AgentID   string
    AgentName string
    Cancel    context.CancelFunc
    Status    string
    StartedAt time.Time
    EventCount int32
    LastEventAt time.Time
}

type RunRegistry struct {
    mu   sync.RWMutex
    runs map[string]*ActiveRun  // runID → ActiveRun
    bySession map[string]string // sessionID → runID
}

func NewRunRegistry() *RunRegistry {
    return &RunRegistry{
        runs:      map[string]*ActiveRun{},
        bySession: map[string]string{},
    }
}

func (r *RunRegistry) Register(runID, sessionID, agentID, agentName string, cancel context.CancelFunc) error {
    r.mu.Lock()
    defer r.mu.Unlock()
    if _, exists := r.bySession[sessionID]; exists {
        return fmt.Errorf("session %s already has an active run", sessionID)
    }
    r.runs[runID] = &ActiveRun{
        RunID:     runID,
        SessionID: sessionID,
        AgentID:   agentID,
        AgentName: agentName,
        Cancel:    cancel,
        Status:    "running",
        StartedAt: time.Now(),
    }
    r.bySession[sessionID] = runID
    return nil
}

func (r *RunRegistry) Unregister(runID string) {
    r.mu.Lock()
    defer r.mu.Unlock()
    if run, ok := r.runs[runID]; ok {
        delete(r.bySession, run.SessionID)
        delete(r.runs, runID)
    }
}

func (r *RunRegistry) IsRunning(sessionID string) bool {
    r.mu.RLock()
    defer r.mu.RUnlock()
    _, ok := r.bySession[sessionID]
    return ok
}

func (r *RunRegistry) CancelRun(runID string) bool {
    r.mu.RLock()
    run, ok := r.runs[runID]
    r.mu.RUnlock()
    if !ok || run.Cancel == nil {
        return false
    }
    run.Cancel()
    run.Status = "cancelled"
    return true
}

func (r *RunRegistry) GetStatus(runID string) (RunStatusInfo, bool) {
    r.mu.RLock()
    defer r.mu.RUnlock()
    run, ok := r.runs[runID]
    if !ok {
        return RunStatusInfo{}, false
    }
    return RunStatusInfo{
        RunID:       run.RunID,
        SessionID:   run.SessionID,
        AgentID:     run.AgentID,
        AgentName:   run.AgentName,
        Status:      run.Status,
        EventCount:  int(atomic.LoadInt32(&run.EventCount)),
        StartedAt:   run.StartedAt.UTC().Format(time.RFC3339),
        LastEventAt: run.LastEventAt.UTC().Format(time.RFC3339),
    }, true
}

func (r *RunRegistry) IncrementEventCount(runID string) {
    r.mu.RLock()
    run, ok := r.runs[runID]
    r.mu.RUnlock()
    if ok {
        atomic.AddInt32(&run.EventCount, 1)
        run.LastEventAt = time.Now()
    }
}
```

### 3.3 MessageQueue — 消息排队

```go
// internal/biz/message_queue.go

type MessageQueue struct {
    mu     sync.RWMutex
    queues map[string][]QueuedMsg  // sessionID → []QueuedMsg
}

func NewMessageQueue() *MessageQueue {
    return &MessageQueue{queues: map[string][]QueuedMsg{}}
}

func (q *MessageQueue) Enqueue(sessionID, content string) QueuedMsg {
    msg := QueuedMsg{
        ID:        uuid.NewString(),
        SessionID: sessionID,
        Content:   content,
        Status:    "pending",
        CreatedAt: time.Now().UTC().Format(time.RFC3339),
    }
    q.mu.Lock()
    q.queues[sessionID] = append(q.queues[sessionID], msg)
    q.mu.Unlock()
    return msg
}

func (q *MessageQueue) Dequeue(sessionID string) (QueuedMsg, bool) {
    q.mu.Lock()
    defer q.mu.Unlock()
    queue := q.queues[sessionID]
    if len(queue) == 0 {
        return QueuedMsg{}, false
    }
    head := queue[0]
    head.Status = "delivered"
    q.queues[sessionID] = queue[1:]
    if len(q.queues[sessionID]) == 0 {
        delete(q.queues, sessionID)
    }
    return head, true
}

func (q *MessageQueue) List(sessionID string) []QueuedMsg {
    q.mu.RLock()
    defer q.mu.RUnlock()
    out := make([]QueuedMsg, len(q.queues[sessionID]))
    copy(out, q.queues[sessionID])
    return out
}
```

### 3.4 WebhookConfig Repo

```go
// internal/biz/webhook.go

type WebhookRepository interface {
    Create(ctx context.Context, w WebhookConfig) (WebhookConfig, error)
    Get(ctx context.Context, id string) (WebhookConfig, error)
    List(ctx context.Context) ([]WebhookConfig, error)
    Update(ctx context.Context, w WebhookConfig) (WebhookConfig, error)
    Delete(ctx context.Context, id string) error
}

type WebhookUsecase struct {
    repo WebhookRepository
}

func NewWebhookUsecase(repo WebhookRepository) *WebhookUsecase {
    return &WebhookUsecase{repo: repo}
}

func (uc *WebhookUsecase) Create(ctx context.Context, w WebhookConfig) (WebhookConfig, error) {
    if w.ID == "" {
        w.ID = uuid.NewString()
    }
    now := time.Now().UTC().Format(time.RFC3339)
    w.CreatedAt = now
    w.UpdatedAt = now
    return uc.repo.Create(ctx, w)
}

func (uc *WebhookUsecase) List(ctx context.Context) ([]WebhookConfig, error) {
    return uc.repo.List(ctx)
}

func (uc *WebhookUsecase) Update(ctx context.Context, w WebhookConfig) (WebhookConfig, error) {
    w.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
    return uc.repo.Update(ctx, w)
}

func (uc *WebhookUsecase) Delete(ctx context.Context, id string) error {
    return uc.repo.Delete(ctx, id)
}
```

### 3.5 Webhook Dispatcher

```go
// internal/biz/webhook_dispatcher.go

type WebhookDispatcher struct {
    repo   WebhookRepository
    client *http.Client
}

func NewWebhookDispatcher(repo WebhookRepository) *WebhookDispatcher {
    return &WebhookDispatcher{
        repo:   repo,
        client: &http.Client{Timeout: 10 * time.Second},
    }
}

type WebhookPayload struct {
    EventType string         `json:"event_type"`
    RunID     string         `json:"run_id"`
    SessionID string         `json:"session_id"`
    Status    string         `json:"status"`
    Timestamp string         `json:"timestamp"`
    Data      map[string]any `json:"data,omitempty"`
}

func (d *WebhookDispatcher) Dispatch(ctx context.Context, eventType, runID, sessionID, status string, data map[string]any) {
    hooks, err := d.repo.List(ctx)
    if err != nil {
        return
    }
    payload := WebhookPayload{
        EventType: eventType,
        RunID:     runID,
        SessionID: sessionID,
        Status:    status,
        Timestamp: time.Now().UTC().Format(time.RFC3339),
        Data:      data,
    }
    for _, hook := range hooks {
        if !hook.Enabled {
            continue
        }
        if !matchEventType(hook.EventTypesJSON, eventType) {
            continue
        }
        go d.send(ctx, hook, payload)
    }
}

func (d *WebhookDispatcher) send(ctx context.Context, hook WebhookConfig, payload WebhookPayload) {
    raw, err := json.Marshal(payload)
    if err != nil {
        return
    }
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, hook.URL, bytes.NewReader(raw))
    if err != nil {
        return
    }
    req.Header.Set("Content-Type", "application/json")
    for k, v := range hook.Headers {
        req.Header.Set(k, v)
    }
    if hook.Secret != "" {
        sig := hmacSHA256(raw, hook.Secret)
        req.Header.Set("X-Webhook-Signature", sig)
    }
    resp, err := d.client.Do(req)
    if err != nil {
        return
    }
    defer resp.Body.Close()
}

func matchEventType(typesJSON, eventType string) bool {
    var types []string
    if err := json.Unmarshal([]byte(typesJSON), &types); err != nil {
        return false
    }
    for _, t := range types {
        if t == eventType || t == "*" {
            return true
        }
    }
    return false
}

func hmacSHA256(data []byte, secret string) string {
    h := hmac.New(sha256.New, []byte(secret))
    h.Write(data)
    return hex.EncodeToString(h.Sum(nil))
}
```

### 3.6 GatewayUsecase

```go
// internal/biz/gateway.go

type GatewayUsecase struct {
    registry   *RunRegistry
    queue      *MessageQueue
    webhooks   *WebhookDispatcher
}

func NewGatewayUsecase(registry *RunRegistry, queue *MessageQueue, webhooks *WebhookDispatcher) *GatewayUsecase {
    return &GatewayUsecase{registry: registry, queue: queue, webhooks: webhooks}
}

func (uc *GatewayUsecase) GetRunStatus(ctx context.Context, runID string) (RunStatusInfo, error) {
    status, ok := uc.registry.GetStatus(runID)
    if !ok {
        return RunStatusInfo{}, sql.ErrNoRows
    }
    return status, nil
}

func (uc *GatewayUsecase) CancelRun(ctx context.Context, runID string) (bool, error) {
    cancelled := uc.registry.CancelRun(runID)
    if cancelled {
        uc.webhooks.Dispatch(ctx, "run.cancelled", runID, "", "cancelled", nil)
    }
    return cancelled, nil
}

func (uc *GatewayUsecase) QueueMessage(ctx context.Context, sessionID, content string) (QueuedMsg, error) {
    return uc.queue.Enqueue(sessionID, content), nil
}

func (uc *GatewayUsecase) ListQueuedMessages(ctx context.Context, sessionID string) ([]QueuedMsg, error) {
    return uc.queue.List(sessionID), nil
}
```

---

## 四、Data 层

### 4.1 Webhook 持久化

```go
// internal/data/webhook.go

type webhookRepo struct {
    data *Data
}

func NewWebhookRepo(d *Data) biz.WebhookRepository {
    return &webhookRepo{data: d}
}

func (r *webhookRepo) Create(ctx context.Context, w biz.WebhookConfig) (biz.WebhookConfig, error) {
    now := nowRFC3339()
    saved, err := r.data.entClient.GatewayWebhook.Create().
        SetID(w.ID).
        SetName(w.Name).
        SetURL(w.URL).
        SetEventTypesJSON(w.EventTypesJSON).
        SetSecret(w.Secret).
        SetHeadersJSON(mapToJSON(w.Headers)).
        SetEnabled(w.Enabled).
        SetCreatedAt(now).
        SetUpdatedAt(now).
        Save(ctx)
    if err != nil {
        return biz.WebhookConfig{}, err
    }
    return entWebhookToBiz(saved), nil
}

func (r *webhookRepo) Get(ctx context.Context, id string) (biz.WebhookConfig, error) {
    row, err := r.data.entClient.GatewayWebhook.Get(ctx, id)
    if err != nil {
        return biz.WebhookConfig{}, err
    }
    return entWebhookToBiz(row), nil
}

func (r *webhookRepo) List(ctx context.Context) ([]biz.WebhookConfig, error) {
    rows, err := r.data.entClient.GatewayWebhook.Query().
        Where(gatewaywebhook.EnabledEQ(true)).
        All(ctx)
    if err != nil {
        return nil, err
    }
    out := make([]biz.WebhookConfig, len(rows))
    for i, row := range rows {
        out[i] = entWebhookToBiz(row)
    }
    return out, nil
}

func (r *webhookRepo) Update(ctx context.Context, w biz.WebhookConfig) (biz.WebhookConfig, error) {
    saved, err := r.data.entClient.GatewayWebhook.UpdateOneID(w.ID).
        SetName(w.Name).
        SetURL(w.URL).
        SetEventTypesJSON(w.EventTypesJSON).
        SetSecret(w.Secret).
        SetHeadersJSON(mapToJSON(w.Headers)).
        SetEnabled(w.Enabled).
        SetUpdatedAt(nowRFC3339()).
        Save(ctx)
    if err != nil {
        return biz.WebhookConfig{}, err
    }
    return entWebhookToBiz(saved), nil
}

func (r *webhookRepo) Delete(ctx context.Context, id string) error {
    return r.data.entClient.GatewayWebhook.DeleteOneID(id).Exec(ctx)
}
```

### 4.2 Ent Schema

```go
// internal/data/ent/schema/gateway_webhook.go

type GatewayWebhook struct {
    ent.Schema
}

func (GatewayWebhook) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").DefaultFunc(uuid.NewString),
        field.String("name"),
        field.String("url"),
        field.String("event_types_json").Default("[]"),
        field.String("secret").Default(""),
        field.String("headers_json").Default("{}"),
        field.Bool("enabled").Default(true),
        field.String("created_at").DefaultFunc(nowRFC3339),
        field.String("updated_at").DefaultFunc(nowRFC3339),
    }
}

func (GatewayWebhook) Edges() []ent.Edge {
    return nil
}
```

---

## 五、Service 层

### 5.1 GatewayService

```go
// internal/service/gateway.go

type GatewayService struct {
    v1.UnimplementedGatewayServiceServer
    uc *biz.GatewayUsecase
    wh *biz.WebhookUsecase
}

func NewGatewayService(uc *biz.GatewayUsecase, wh *biz.WebhookUsecase) *GatewayService {
    return &GatewayService{uc: uc, wh: wh}
}

func (s *GatewayService) GetRunStatus(ctx context.Context, req *v1.GetRunStatusRequest) (*v1.RunStatus, error) {
    status, err := s.uc.GetRunStatus(ctx, req.GetRunId())
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return bizRunStatusToProto(status), nil
}

func (s *GatewayService) CancelRun(ctx context.Context, req *v1.CancelRunRequest) (*v1.CancelRunResponse, error) {
    cancelled, err := s.uc.CancelRun(ctx, req.GetRunId())
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return &v1.CancelRunResponse{Cancelled: cancelled}, nil
}

func (s *GatewayService) QueueMessage(ctx context.Context, req *v1.QueueMessageRequest) (*v1.QueuedMessage, error) {
    msg, err := s.uc.QueueMessage(ctx, req.GetSessionId(), req.GetContent())
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return bizQueuedMsgToProto(msg), nil
}

func (s *GatewayService) ListQueuedMessages(ctx context.Context, req *v1.ListQueuedMessagesRequest) (*v1.ListQueuedMessagesResponse, error) {
    msgs, err := s.uc.ListQueuedMessages(ctx, req.GetSessionId())
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    out := &v1.ListQueuedMessagesResponse{}
    for _, m := range msgs {
        out.Items = append(out.Items, bizQueuedMsgToProto(m))
    }
    return out, nil
}

func (s *GatewayService) CreateWebhook(ctx context.Context, req *v1.CreateWebhookRequest) (*v1.Webhook, error) {
    w := biz.WebhookConfig{
        Name:           req.GetName(),
        URL:            req.GetUrl(),
        EventTypesJSON: req.GetEventTypesJson(),
        Secret:         req.GetSecret(),
        Headers:        req.GetHeaders(),
        Enabled:        true,
    }
    created, err := s.wh.Create(ctx, w)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return bizWebhookToProto(created), nil
}

func (s *GatewayService) ListWebhooks(ctx context.Context, req *v1.ListWebhooksRequest) (*v1.ListWebhooksResponse, error) {
    hooks, err := s.wh.List(ctx)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    out := &v1.ListWebhooksResponse{}
    for _, h := range hooks {
        out.Items = append(out.Items, bizWebhookToProto(h))
    }
    return out, nil
}

func (s *GatewayService) UpdateWebhook(ctx context.Context, req *v1.UpdateWebhookRequest) (*v1.Webhook, error) {
    w := biz.WebhookConfig{
        ID:             req.GetId(),
        Name:           req.GetName(),
        URL:            req.GetUrl(),
        EventTypesJSON: req.GetEventTypesJson(),
        Secret:         req.GetSecret(),
        Headers:        req.GetHeaders(),
        Enabled:        req.GetEnabled(),
    }
    updated, err := s.wh.Update(ctx, w)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    return bizWebhookToProto(updated), nil
}

func (s *GatewayService) DeleteWebhook(ctx context.Context, req *v1.DeleteWebhookRequest) (*emptypb.Empty, error) {
    if err := s.wh.Delete(ctx, req.GetId()); err != nil {
        return nil, kerrors.FromError(err)
    }
    return &emptypb.Empty{}, nil
}
```

### 5.2 ChatService 集成

将 `activeRuns`/`pendingQueue` 从 ChatService 迁移到 `RunRegistry`/`MessageQueue`：

```go
// internal/service/chat.go 修改

type ChatService struct {
    chatv1.UnimplementedChatServiceServer
    // ... 现有字段 ...
    registry *biz.RunRegistry
    queue    *biz.MessageQueue
    webhooks *biz.WebhookDispatcher
}

// runNativeAgentTurn 中使用 registry 替代 activeRuns
func (s *ChatService) runNativeAgentTurn(ctx context.Context, req *chatv1.SendChatMessageRequest, stream *streamWriter) (biz.ChatMessage, biz.ChatMessage, error) {
    sessionID := strings.TrimSpace(req.GetSessionId())
    content := strings.TrimSpace(req.GetContent())
    if sessionID == "" || content == "" {
        return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "session_id and content are required")
    }

    if s.registry.IsRunning(sessionID) {
        s.queue.Enqueue(sessionID, content)
        return biz.ChatMessage{}, biz.ChatMessage{}, nil
    }
    // ... 其余逻辑不变 ...
}
```

### 5.3 AwaitUserReply 集成

在 `internal/agent/` 层构建 Runner 时启用 AwaitUserReply：

```go
// internal/agent/trpc_runtime.go 或 chatagent 构建处

runner, err := trpcrunner.NewRunner(
    root,
    trpcrunner.WithSessionService(sessSvc),
    trpcrunner.WithMemoryService(memSvc),
    trpcrunner.WithAwaitUserReplyRouting(true),
)
```

### 5.4 Webhook 触发

在 Runner 完成后触发 Webhook：

```go
// 在 runSingleAgentViaTRPC 的 defer 中
defer func() {
    s.registry.Unregister(runID)
    runner.Close()
    s.webhooks.Dispatch(context.Background(), "run.completed", runID, sessionID, "completed", nil)
    s.processPendingQueue(sessionID, sess, ag, dialogMode, prov, mod, stream)
}()
```

---

## 六、Wire 注入

### 6.1 Biz ProviderSet 新增

```go
// internal/biz/biz.go
var ProviderSet = wire.NewSet(
    // ... 现有 ...
    NewRunRegistry,
    NewMessageQueue,
    NewWebhookUsecase,
    NewWebhookDispatcher,
    NewGatewayUsecase,
)
```

### 6.2 Service ProviderSet 新增

```go
// internal/service/service.go
var ProviderSet = wire.NewSet(
    // ... 现有 ...
    NewGatewayService,
)
```

### 6.3 Server 注册

```go
// internal/server/http.go
gatewayv1.RegisterGatewayServiceHTTPServer(srv, gatewaySvc)
```

---

## 七、Web 前端设计

### 7.1 类型定义

```typescript
// web/src/features/gateway/types.ts

export type RunStatusInfo = {
  run_id: string;
  session_id: string;
  agent_id: string;
  agent_name: string;
  status: "idle" | "running" | "waiting_user" | "completed" | "cancelled" | "failed";
  event_count: number;
  started_at: string;
  last_event_at: string;
};

export type QueuedMsg = {
  id: string;
  session_id: string;
  content: string;
  status: "pending" | "delivered" | "expired";
  created_at: string;
};

export type WebhookConfig = {
  id: string;
  name: string;
  url: string;
  event_types_json: string;
  secret: string;
  headers: Record<string, string>;
  enabled: boolean;
  created_at: string;
  updated_at: string;
};
```

### 7.2 API

```typescript
// web/src/features/gateway/api.ts

import { kratosApi } from "../../services/axiosHandler";

export async function getRunStatus(runId: string): Promise<RunStatusInfo> {
  const { data } = await kratosApi.get(`/v1/gateway/runs/${runId}/status`);
  return data;
}

export async function cancelRun(runId: string): Promise<{ cancelled: boolean }> {
  const { data } = await kratosApi.post(`/v1/gateway/runs/${runId}/cancel`);
  return data;
}

export async function queueMessage(sessionId: string, content: string): Promise<QueuedMsg> {
  const { data } = await kratosApi.post("/v1/gateway/queue", { session_id: sessionId, content });
  return data;
}

export async function listQueuedMessages(sessionId: string): Promise<QueuedMsg[]> {
  const { data } = await kratosApi.get(`/v1/gateway/queue/${sessionId}`);
  return data.items ?? [];
}

export async function createWebhook(req: Omit<WebhookConfig, "id" | "created_at" | "updated_at">): Promise<WebhookConfig> {
  const { data } = await kratosApi.post("/v1/gateway/webhooks", req);
  return data;
}

export async function listWebhooks(): Promise<WebhookConfig[]> {
  const { data } = await kratosApi.get("/v1/gateway/webhooks");
  return data.items ?? [];
}

export async function updateWebhook(id: string, req: Partial<WebhookConfig>): Promise<WebhookConfig> {
  const { data } = await kratosApi.put(`/v1/gateway/webhooks/${id}`, req);
  return data;
}

export async function deleteWebhook(id: string): Promise<void> {
  await kratosApi.delete(`/v1/gateway/webhooks/${id}`);
}
```

### 7.3 组件设计

#### RunStatusIndicator.vue — 运行状态指示器

```
位置：web/src/features/chat/components/RunStatusIndicator.vue
用途：嵌入 Chat 页面，显示当前运行状态
```

**Props**：

| Prop | 类型 | 说明 |
|------|------|------|
| status | RunStatusInfo | null | 运行状态 |

**模板结构**：

```
<div v-if="status" class="run-status" :class="statusClass">
  <q-icon :name="statusIcon" size="sm" />
  <span class="status-text">{{ statusLabel }}</span>
  <q-badge v-if="status.event_count > 0" color="grey">
    {{ status.event_count }} events
  </q-badge>
  <q-btn
    v-if="status.status === 'running'"
    flat round dense icon="stop"
    color="negative" size="sm"
    @click="$emit('cancel', status.run_id)"
  />
</div>
```

**状态映射**：

| 状态 | 图标 | 颜色 | 操作 |
|------|------|------|------|
| idle | radio_button_unchecked | grey | — |
| running | autorenew (旋转动画) | positive | 取消按钮 |
| waiting_user | hourglass_empty | warning | 输入框聚焦 |
| completed | check_circle | positive | — |
| cancelled | cancel | negative | — |
| failed | error | negative | — |

#### QueuedMessageList.vue — 排队消息列表

```
位置：web/src/features/chat/components/QueuedMessageList.vue
用途：显示当前会话的排队消息
```

**Props**：

| Prop | 类型 | 说明 |
|------|------|------|
| messages | QueuedMsg[] | 排队消息列表 |

**模板结构**：

```
<div v-if="messages.length > 0" class="queued-messages">
  <div class="queue-header">
    <q-icon name="schedule" size="xs" />
    <span>{{ messages.length }} 条排队消息</span>
  </div>
  <q-list dense>
    <q-item v-for="msg in messages" :key="msg.id">
      <q-item-section>
        <q-item-label>{{ msg.content }}</q-item-label>
        <q-item-label caption>{{ formatTime(msg.created_at) }}</q-item-label>
      </q-item-section>
      <q-item-section side>
        <q-badge :color="statusColor(msg.status)">{{ msg.status }}</q-badge>
      </q-item-section>
    </q-item>
  </q-list>
</div>
```

#### WebhookManager.vue — Webhook 管理页面

```
位置：web/src/features/settings/components/WebhookManager.vue
用途：系统设置中管理 Webhook 配置
```

**模板结构**：

```
<div class="webhook-manager">
  <div class="header">
    <h6>Webhook 配置</h6>
    <q-btn icon="add" label="新建" color="primary" @click="showCreateDialog = true" />
  </div>
  <q-table :rows="webhooks" :columns="columns" row-key="id" flat>
    <template #body-cell-enabled="props">
      <q-toggle v-model="props.row.enabled" @update:model-value="toggleWebhook(props.row)" />
    </template>
    <template #body-cell-actions="props">
      <q-btn flat round dense icon="edit" @click="editWebhook(props.row)" />
      <q-btn flat round dense icon="delete" color="negative" @click="confirmDelete(props.row)" />
    </template>
  </q-table>

  <q-dialog v-model="showCreateDialog">
    <WebhookForm :webhook="editingWebhook" @save="onSaveWebhook" @cancel="showCreateDialog = false" />
  </q-dialog>
</div>
```

#### WebhookForm.vue — Webhook 表单

```
位置：web/src/features/settings/components/WebhookForm.vue
用途：创建/编辑 Webhook 的表单
```

**Props**：

| Prop | 类型 | 说明 |
|------|------|------|
| webhook | WebhookConfig | null | 编辑时传入，新建时为 null |

**表单字段**：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| name | text | 是 | Webhook 名称 |
| url | text | 是 | 回调 URL |
| event_types_json | multi-select | 是 | 事件类型（run.completed/run.failed/run.cancelled/*） |
| secret | text | 否 | HMAC 签名密钥 |
| headers | key-value | 否 | 自定义 HTTP 头 |

### 7.4 Chat 页面集成

在 `useChatWorkspace.ts` 中集成运行状态轮询：

```typescript
const runStatus = ref<RunStatusInfo | null>(null);
let statusPollTimer: ReturnType<typeof setInterval> | null = null;

function startStatusPoll(runId: string) {
  stopStatusPoll();
  statusPollTimer = setInterval(async () => {
    try {
      runStatus.value = await getRunStatus(runId);
      if (runStatus.value?.status !== "running") {
        stopStatusPoll();
      }
    } catch {
      stopStatusPoll();
    }
  }, 2000);
}

function stopStatusPoll() {
  if (statusPollTimer) {
    clearInterval(statusPollTimer);
    statusPollTimer = null;
  }
}

async function onCancelRun(runId: string) {
  const result = await cancelRun(runId);
  if (result.cancelled) {
    runStatus.value = null;
  }
}
```

---

## 八、涉及文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `api/kratos/gateway/v1/gateway.proto` | 新建 | Gateway Proto 定义 |
| `internal/biz/gateway.go` | 新建 | GatewayUsecase + 领域模型 |
| `internal/biz/run_registry.go` | 新建 | RunRegistry 运行注册表 |
| `internal/biz/message_queue.go` | 新建 | MessageQueue 消息排队 |
| `internal/biz/webhook.go` | 新建 | WebhookConfig 模型 + Usecase |
| `internal/biz/webhook_dispatcher.go` | 新建 | WebhookDispatcher 回调分发 |
| `internal/biz/biz.go` | 修改 | ProviderSet 新增 |
| `internal/data/webhook.go` | 新建 | WebhookRepo 实现 |
| `internal/data/ent/schema/gateway_webhook.go` | 新建 | Webhook Ent Schema |
| `internal/service/gateway.go` | 新建 | GatewayService |
| `internal/service/chat.go` | 修改 | 使用 RunRegistry/MessageQueue 替代 sync.Map |
| `internal/service/chat_native.go` | 修改 | 使用 registry.IsRunning + queue.Enqueue |
| `internal/server/http.go` | 修改 | 注册 GatewayService |
| `internal/agent/trpc_runtime.go` | 修改 | 启用 WithAwaitUserReplyRouting |
| `cmd/admin/wire.go` | 修改 | Wire 注入更新 |
| `web/src/features/gateway/types.ts` | 新建 | Gateway 类型定义 |
| `web/src/features/gateway/api.ts` | 新建 | Gateway API |
| `web/src/features/chat/components/RunStatusIndicator.vue` | 新建 | 运行状态指示器 |
| `web/src/features/chat/components/QueuedMessageList.vue` | 新建 | 排队消息列表 |
| `web/src/features/settings/components/WebhookManager.vue` | 新建 | Webhook 管理页面 |
| `web/src/features/settings/components/WebhookForm.vue` | 新建 | Webhook 表单 |

---

## 九、实现阶段

### 阶段一：运行管理核心

1. 新建 `internal/biz/run_registry.go` — RunRegistry
2. 新建 `internal/biz/message_queue.go` — MessageQueue
3. 修改 `internal/service/chat.go` — 使用 RunRegistry/MessageQueue
4. 修改 `internal/service/chat_native.go` — 集成 registry
5. 修改 `internal/biz/biz.go` — ProviderSet
6. 验证：并发控制、排队消息正常工作

### 阶段二：Gateway API

1. 新建 `api/kratos/gateway/v1/gateway.proto`
2. 运行 `make proto`
3. 新建 `internal/biz/gateway.go` — GatewayUsecase
4. 新建 `internal/service/gateway.go` — GatewayService
5. 修改 `internal/server/http.go` — 注册
6. 修改 `cmd/admin/wire.go` — Wire 注入
7. 验证：GetRunStatus/CancelRun/QueueMessage API 可用

### 阶段三：Webhook 系统

1. 新建 `internal/data/ent/schema/gateway_webhook.go`
2. 运行 `go generate ./internal/data/ent`
3. 新建 `internal/biz/webhook.go` — WebhookUsecase
4. 新建 `internal/biz/webhook_dispatcher.go` — WebhookDispatcher
5. 新建 `internal/data/webhook.go` — WebhookRepo
6. 集成到 GatewayService
7. 验证：Webhook 创建/更新/删除/触发

### 阶段四：AwaitUserReply

1. 修改 `internal/agent/` 层 Runner 构建 — 启用 WithAwaitUserReplyRouting
2. 验证：Agent 可指定下一轮用户消息路由

### 阶段五：前端

1. 新建 `web/src/features/gateway/types.ts` + `api.ts`
2. 新建 `RunStatusIndicator.vue`
3. 新建 `QueuedMessageList.vue`
4. 新建 `WebhookManager.vue` + `WebhookForm.vue`
5. 集成到 Chat 和 Settings 页面
6. 验证：运行状态可视化、排队消息展示、Webhook 管理

---

## 十、验收标准

1. ✅ 同一会话同时只有一个运行，其他请求被排队
2. ✅ 可通过 API 查询运行进度（状态、事件数、开始时间）
3. ✅ 可通过 API 取消正在运行的请求
4. ✅ Agent 可指定下一轮用户消息路由到特定 Agent（AwaitUserReply）
5. ✅ 用户消息可排队等待，轮次结束后自动处理
6. ✅ Agent 运行完成后自动回调外部系统（Webhook）
7. ✅ Webhook 支持 HMAC 签名验证
8. ✅ 前端运行状态可视化、排队消息展示
