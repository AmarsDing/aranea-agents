# Chat 对话模块 — 实现设计文档

> 对应需求：`1 chat.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Chat 是用户与 Agent/Team 交互的核心入口，负责 HTTP/WS 发起对话、WebSocket + EventBus 实时事件、上下文管理、用量记录、停止生成、人工等待回复与待执行队列。当前主链路已端到端可用；本设计文档记录现有实现架构并标注仍需完善的成员级事件、工具事件、附件与可恢复状态。

> **2026-05-17 现状对齐**：独立 SSE `/v1/chat/messages/stream` 路由已从当前代码移除，`internal/server/register_chat.go` 只注册 proto HTTP 服务；实时流由 `internal/server/ws.go` 的 `/v1/ws` 承载。

---

## 二、Proto 层

### 2.1 主要 Proto 定义

文件：`api/kratos/chat/v1/chat.proto`

```protobuf
syntax = "proto3";

package kratos.chat.v1;

import "google/api/annotations.proto";
import "google/api/field_behavior.proto";
import "google/protobuf/struct.proto";

option go_package = "aranea-agents/api/kratos/chat/v1;v1";

message SendChatMessageRequest {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
  optional string agent_key = 2;
  optional string team_id = 3;
  string content = 4 [(google.api.field_behavior) = REQUIRED];
  SendMessageOptions options = 5;
}

message SendMessageOptions {
  optional string dialog_mode = 1;
  optional string provider = 2;
  optional string model = 3;
  repeated AttachmentRef attachments = 4;
}

message AttachmentRef {
  string id = 1;
}

message SendChatMessageResponse {
  google.protobuf.Struct user_message = 1;
  google.protobuf.Struct agent_message = 2;
}

message GetChatOptionsRequest {
  string type = 1;
}

message ChatOption {
  string type = 1;
  string key = 2;
  string label = 3;
  bool enabled = 4;
  int32 sort_order = 5;
  string metadata_json = 6;
}

message GetChatOptionsResponse {
  repeated ChatOption items = 1;
}

message StopGenerationRequest {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message StopGenerationResponse {
  bool stopped = 1;
}

message GetPendingMessagesRequest {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message PendingMessage {
  string id = 1;
  string content = 2;
  string status = 3;
  string created_at = 4;
}

message GetPendingMessagesResponse {
  repeated PendingMessage items = 1;
}

message CancelPendingMessageRequest {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
  string pending_id = 2 [(google.api.field_behavior) = REQUIRED];
}

message CancelPendingMessageResponse {
  bool cancelled = 1;
}

message UpdatePendingMessageRequest {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
  string pending_id = 2 [(google.api.field_behavior) = REQUIRED];
  string content = 3 [(google.api.field_behavior) = REQUIRED];
}

message UpdatePendingMessageResponse {
  bool updated = 1;
}

message RunStatus {
  string run_id = 1;
  string status = 2;
  string error_message = 3;
  string updated_at = 4;
}

message GetRunStatusRequest {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message AwaitUserReplyRequest {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
  optional string run_id = 2;
  string reply = 3 [(google.api.field_behavior) = REQUIRED];
}

message AwaitUserReplyResponse {
  bool accepted = 1;
}

service ChatService {
  rpc SendChatMessage(SendChatMessageRequest) returns (SendChatMessageResponse) {
    option (google.api.http) = {
      post: "/v1/chat/messages"
      body: "*"
    };
  }
  rpc GetChatOptions(GetChatOptionsRequest) returns (GetChatOptionsResponse) {
    option (google.api.http) = {get: "/v1/chat/options"};
  }
  rpc StopGeneration(StopGenerationRequest) returns (StopGenerationResponse) {
    option (google.api.http) = {
      post: "/v1/chat/stop"
      body: "*"
    };
  }
  rpc GetPendingMessages(GetPendingMessagesRequest) returns (GetPendingMessagesResponse) {
    option (google.api.http) = { get: "/v1/chat/pending" };
  }
  rpc CancelPendingMessage(CancelPendingMessageRequest) returns (CancelPendingMessageResponse) {
    option (google.api.http) = {
      post: "/v1/chat/pending/cancel"
      body: "*"
    };
  }
  rpc UpdatePendingMessage(UpdatePendingMessageRequest) returns (UpdatePendingMessageResponse) {
    option (google.api.http) = {
      post: "/v1/chat/pending/update"
      body: "*"
    };
  }
  rpc GetRunStatus(GetRunStatusRequest) returns (RunStatus) {
    option (google.api.http) = {get: "/v1/chat/run-status"};
  }
  rpc AwaitUserReply(AwaitUserReplyRequest) returns (AwaitUserReplyResponse) {
    option (google.api.http) = {
      post: "/v1/chat/await-reply"
      body: "*"
    };
  }
}
```

### 2.2 WebSocket 实时端点（非 Proto 定义，HTTP Server 层注册）

实时对话事件通过 WebSocket + EventBus 下发，不在 Proto 中定义：

```
GET /v1/ws?session_id=...  →  WSServer.handleWS()
```

客户端可在 WS 上行发送 `user_message` / `enqueue_message` / `cancel` / `ping` / `subscribe` / `unsubscribe` / `enable_log`，服务端复用 `ChatService.SendChatMessage` 与 `ChatService.CancelRun`，下行统一为 Envelope。

#### WS 上行消息格式

| 上行 type | 载荷字段 | 说明 |
|-----------|----------|------|
| `user_message` | `session_id`, `content`, `options` | 发送用户消息，触发 ChatService.SendChatMessage |
| `enqueue_message` | `session_id`, `content`, `options` | 发送用户消息，若当前有 run 则入队 pendingQueue |
| `cancel` | `session_id` | 取消当前 run（等同于 HTTP StopGeneration） |
| `ping` | — | 心跳探测，服务端回复 `pong` |
| `subscribe` | `session_id` / `team_id` | 订阅指定 session/team 的 EventBus 事件 |
| `unsubscribe` | `session_id` / `team_id` | 取消订阅 |
| `enable_log` | `enabled` | 开启/关闭运行日志推送（monitor channel） |

#### WS 下行控制消息

| 下行 type | Channel | 载荷字段 | 说明 |
|-----------|---------|----------|------|
| `connected` | system | `session_id`, `connection_id` | WS 连接建立成功 |
| `pong` | system | — | 心跳回复 |
| `replay_start` | system | `session_id` | EventBuffer 回放开始标记 |
| `replay_end` | system | `session_id` | EventBuffer 回放结束标记 |
| `server_shutdown` | system | `message` | 服务端即将关闭通知 |

### 2.3 消息字段说明

| 消息 | 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|------|
| `SendChatMessageRequest` | `session_id` | string | ✅ | 目标会话 ID |
| | `agent_key` | string | ❌ | Agent 标识（可选覆盖） |
| | `team_id` | string | ❌ | Team ID（可选覆盖） |
| | `content` | string | ✅ | 用户消息内容 |
| | `options` | SendMessageOptions | ❌ | 对话选项 |
| `SendMessageOptions` | `dialog_mode` | string | ❌ | 对话模式：default/plan/code |
| | `provider` | string | ❌ | 模型提供商覆盖 |
| | `model` | string | ❌ | 模型名覆盖 |
| | `attachments` | AttachmentRef[] | ❌ | 附件引用列表 |
| | `knowledge_bases` | string[] | ❌ | 本轮限定的知识库 collection ID（`knowledge_search` 白名单） |
| `SubmitMessageFeedbackRequest` | `message_id` / `session_id` / `rating` | — | 👍/👎 反馈（positive \| negative） |
| | `key` | string | — | 选项键（如 default/plan/code） |
| | `label` | string | — | 显示标签 |
| | `enabled` | bool | — | 是否启用 |
| | `sort_order` | int32 | — | 排序 |
| `StopGenerationRequest` | `session_id` | string | ✅ | 要停止的会话 ID |
| `GetPendingMessagesRequest` | `session_id` | string | ✅ | 要查询的会话 ID |
| `CancelPendingMessageRequest` | `session_id` | string | ✅ | 会话 ID |
| | `pending_id` | string | ✅ | 待执行消息 ID |
| `UpdatePendingMessageRequest` | `session_id` | string | ✅ | 会话 ID |
| | `pending_id` | string | ✅ | 待执行消息 ID |
| | `content` | string | ✅ | 新消息内容 |

---

## 三、Biz 层

### 3.1 领域模型

Chat 模块无独立 Biz 模型，依赖以下已有模型：

| 模型 | 包 | 用途 |
|------|-----|------|
| `biz.Agent` | `internal/biz` | Agent 配置查询 |
| `biz.Session` | `internal/biz` | 会话上下文 |
| `biz.Team` | `internal/biz` | Team 编排 |
| `biz.ChatMessage` | `internal/biz` | 对话消息 |
| `biz.TokenUsageEvent` | `internal/biz` | 用量记录事件 |
| `biz.SessionSummary` | `internal/biz` | 会话摘要 |

### 3.2 依赖的 Usecase/Repo 接口

```go
type AgentRepository interface {
    GetAgentByID(ctx context.Context, id string) (Agent, error)
    GetAgentByKey(ctx context.Context, key string) (Agent, error)
}

type SessionUsecase struct {
    repo           SessionRepository
    agents         AgentRepository
    teams          TeamRepository
    titleGenerator SessionTitleGenerator
}

func (uc *SessionUsecase) Get(ctx context.Context, id string) (Session, error)
func (uc *SessionUsecase) UpdateContextFromLLMUsage(ctx context.Context, id string, usage LLMUsage) error
func (uc *SessionUsecase) AppendChatTurn(ctx context.Context, sessionID string, user, assistant ChatMessage) error
func (uc *SessionUsecase) AppendChatMessage(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error

type TeamRepository interface {
    GetTeamByID(ctx context.Context, id string) (Team, error)
}

type UsageUsecase struct { ... }
func (uc *UsageUsecase) RecordTokenUsageEvent(ctx context.Context, event TokenUsageEvent) (TokenUsageEvent, error)

type LlmProviderModelUsecase struct { ... }
func (uc *LlmProviderModelUsecase) ModelForProviderModel(ctx context.Context, provider, model string) (model.LLM, error)

type SkillUsecase struct { ... }
func (uc *SkillUsecase) EffectiveSkills(ctx context.Context, agentID string) ([]Skill, error)

type SystemSettingRepo interface {
    Get(ctx context.Context, key string) (string, error)
}
```

### 3.3 Broker 接口

```go
type TeamRunEventBroker struct {
    mu    sync.RWMutex
    chans map[string]chan *TeamRunEvent
}

func (b *TeamRunEventBroker) Subscribe(id string) <-chan *TeamRunEvent
func (b *TeamRunEventBroker) Unsubscribe(id string)
func (b *TeamRunEventBroker) Publish(event *TeamRunEvent)

type MonitorLogBroker struct {
    mu    sync.RWMutex
    chans map[string]chan *MonitorLog
}

func (b *MonitorLogBroker) Subscribe(id string) <-chan *MonitorLog
func (b *MonitorLogBroker) Unsubscribe(id string)
func (b *MonitorLogBroker) Publish(log *MonitorLog)
```

### 3.4 NativeTurnCompressor 接口

```go
type NativeTurnCompressor interface {
    AfterNativeTurn(ctx context.Context, sessionID, agentID string) error
}
```

### 3.5 SessionTitleGenerator 接口

```go
type SessionTitleGenerator interface {
    Generate(ctx context.Context, userMessage string) (string, error)
}
```

两个实现：
- `noopSessionTitleGenerator`：空实现，不生成标题
- `LLMSessionTitleGenerator`（`internal/service/session_title_llm.go`）：使用轻量 LLM 模型生成标题

---

## 四、Data 层

Chat 模块无独立 Data 层，通过以下已有表间接使用：

### 4.1 用量记录表

```
model_token_usage_events
├── id                TEXT (UUID)
├── session_id        TEXT
├── agent_key         TEXT
├── team_id           TEXT
├── message_id        TEXT
├── model_api_id      TEXT
├── model_display_name TEXT
├── input_tokens      INTEGER
├── output_tokens     INTEGER
├── total_tokens      INTEGER
├── latency_ms        INTEGER
├── tokens_per_second REAL
├── status            TEXT
├── stream_enabled    BOOLEAN
├── usage_kind        TEXT
├── provider_code     TEXT
├── prompt_mode       TEXT
├── error_message     TEXT
├── error_code        TEXT
├── metadata_json     TEXT
├── created_at        TEXT
```

### 4.2 工具调用记录表

```
tool_invocations
├── id               TEXT (UUID)
├── session_id       TEXT
├── agent_id         TEXT
├── tool_key         TEXT
├── status           TEXT
├── input_preview    TEXT
├── output_preview   TEXT
├── duration_ms      INTEGER
├── created_at       TEXT
```

---

## 五、Service 层

### 5.1 文件结构

```
internal/service/
├── chat.go              ← ChatService RPC；入队/排队委托 chatUC
├── chat_run_gateway.go  ← Biz 适配器 + publishMessageQueuedToBus
├── chat_pending.go      ← PendingMessageQueue（Follow-up FIFO，待下沉 runtime）
├── chat_native.go       ← 原生对话入口（HTTP unary + WS 上行复用）+ hydratedAgent
├── trpc_turn.go         ← trpc-agent-go 单 Agent turn 执行 + EventBus 投影 + processPendingQueue
├── chat_usage_ingress.go ← 用量记录
├── session_compress.go  ← L0 上下文压缩
├── session_title_llm.go ← LLM 标题生成
```

### 5.2 ChatService 结构体

```go
type ChatService struct {
    chatv1.UnimplementedChatServiceServer

    teams          biz.TeamRepository
    teamsNative    *team.Runner
    usage          *biz.UsageUsecase
    td             rt.TurnDeps
    pluginRT       *plugintrpc.Runtime
    skillDBRepo    trpcskill.Repository
    runs           *RunRegistry   // internal/runtime：active run、cancel、run status
    chatUC         *biz.ChatUsecase // 编排：入队/排队/锁/await（Follow-up Queue）
    webhooks       *biz.WebhookDispatcher
    // awaitMetaCache / resumeInFlight — Service 层 resume 语义
}
```

`PendingMessageQueue` 由 `NewChatUsecaseFromDeps` 注入 `chatUC`；`ChatService` 不再直接持有 `pendingQueue sync.Map`。

#### Follow-up Queue（对话阶段连续发送）

详见 [35 gateway.design.md §3.6](./35%20gateway.design.md#36-follow-up-queue对话阶段连续发送) 与 [1 chat.md §1.9](./1%20chat.md#19-对话阶段连续发送follow-up-queue--待发送队列)。

**入队**：`chat_native` 检测 `HasActive` → `chatUC.EnqueueUserMessage` → Steerable 或 Pending → `PublishMessageQueued`（`run_status` + `hint: message_queued`）。

**出队**：`trpc_turn` / Team defer → `processPendingQueue` → `chatUC.DequeuePendingMessage` → 新 turn。

**废弃**：`ChatService.publishMessageQueued` — 使用 `ChatUsecase` + `publishMessageQueuedToBus`。

```go
// 历史结构（已移除）
type pendingEntry struct {
    ID        string
    Content   string
    Status    string
    CreatedAt string
}
```

### 5.3 RPC 方法实现

#### SendChatMessage（unary，非流式）

```
1. 检查 session_id 和 content 必填
2. 调 nativeSendChatMessage → runNativeAgentTurn
3. 返回 user_message + agent_message
4. 记录用度
5. 如果是 team 请求，触发 TeamRunEventBroker hint
```

#### GetChatOptions

```
1. 调 nativeGetChatOptions:
   - type="" 或 "dialog_mode" → 返回硬编码的 dialog_mode 选项
   - type="provider" → 从 LLM Catalog 动态获取可用 Provider 列表
   - type="model" → 从 LLM Catalog 动态获取可用 Model 列表
```

#### StopGeneration

```
1. 检查 session_id 必填
2. 从 activeRuns 加载 Runner
3. 优先尝试 chatagent.CancelTRPCRun(runner, sessionID)
4. 回退到 runner.Close()
5. 从 activeRuns 删除
6. 从 pendingCancels 加载 CancelFunc 并调用（取消待执行队列处理）
7. 从 pendingCancels 删除
```

当前 `RunRegistry` 由单 Agent **`StoreCancelable` → `StoreRunner`** 与 Team **`StoreCancelable`** 共同登记（2026-05-23 起 Agent 不再使用 `StorePlaceholder`）；`lockSession` per-session 互斥锁保护 admission。Turn 启动中且无 `ActiveRunner` 时返回 **`CHAT_TURN_BUSY`**；运行中追加消息经 `EnqueueUserMessage` 成功返回 **`ErrTurnMessageQueued`**（steer 或 Pending FIFO）。Team / 单 Agent turn 完成后 defer `Finish` 并调用 `processPendingQueue`（加锁；仍 active 则重新入队）。

#### GetPendingMessages

```
1. 检查 session_id 必填
2. 从 pendingQueue 加载 []pendingEntry
3. 转换为 proto PendingMessage 列表
```

#### CancelPendingMessage

```
1. 检查 session_id 和 pending_id 必填
2. 调用 removePending(sessionID, entryID)
3. 返回 cancelled 状态
```

#### UpdatePendingMessage

```
1. 检查 session_id、pending_id 和 content 必填
2. 调用 updatePending(sessionID, entryID, newContent)
3. updatePending 使用 CAS 循环更新 pendingQueue 中的条目内容
4. 返回 updated 状态
```

### 5.4 WebSocket 实时对话（/v1/ws）

Chat proto HTTP 路由在 `internal/server/register_chat.go` 中注册；实时通道由 `internal/server/ws.go` 单独注册：

```go
func RegisterChatIngress(srv *kratoshttp.Server, chat *service.ChatService) {
    chatv1.RegisterChatServiceHTTPServer(srv, chat)
}

func (s *WSServer) RegisterOnKratos(srv *kratoshttp.Server) {
    srv.HandleFunc("/v1/ws", s.handleWS)
}
```

**请求流转**：

```
GET /v1/ws?session_id=...
  → WSServer.handleWS()
    → 订阅 EventBus（session scoped，可靠订阅）
    → 支持 last_event_id 回放 EventBuffer
    → 上行 user_message / enqueue_message
      → ChatService.SendChatMessage()
        → runNativeAgentTurn()
    → 下行 EventBus Envelope
      → wsDownstream{direction, channel, envelope}

Channel 入口（飞书/Lark webhook 等）：
POST /v1/channel/{channel_type}/webhook
  → ChannelIngress.HandleWebhook()
    → ChatService.RunNativeTurnUnary()
      → 同一 runNativeAgentTurn() 主链路

Cron 入口：
Cron Scheduler
  → ChatService.RunCronTurn()
    → 同一 runNativeAgentTurn() 主链路
```

### 5.5 WebSocket Envelope 事件协议

#### 5.5.1 Envelope 完整结构

```go
type Envelope struct {
    ID         string           // 事件唯一 ID（单调递增，用于 EventBuffer 回放）
    Type       string           // 事件类型
    SessionID  string           // 目标 Session
    Content    *EnvelopeContent // 文本/reasoning 载荷
    ToolCall   *EnvelopeToolCall // 工具调用载荷
    StateDelta *EnvelopeStateDelta // Runner State 增量
    Transfer   *EnvelopeTransfer // Team 转交载荷
    Error      *EnvelopeError   // 错误载荷
    Usage      *EnvelopeUsage   // Token 用量
    Tag        string           // 事件标签（用于客户端过滤）
    FilterKey  string           // 过滤键（前缀匹配，用于 EventBus 订阅过滤）
    Branch     string           // 分支标识（多分支对话）
    Version    int64            // 事件版本号
    Extensions *EnvelopeExtensions // 扩展信息
    Actions    *EnvelopeActions // 动作标记
    Trace      *EnvelopeTrace   // 追踪信息
    Metadata   map[string]string // 通用元数据
}

type EnvelopeContent struct {
    Text      string // 文本内容
    Reasoning string // 推理/思维链内容
    IsPartial bool   // 是否为增量片段
}

type EnvelopeToolCall struct {
    ID            string // 工具调用 ID
    Name          string // 工具名称
    ArgumentsJSON string // 调用参数 JSON
    ResultJSON    string // 返回结果 JSON
    Status        string // 调用状态
    DurationMS    int64  // 执行耗时
    IsLongRunning bool   // 是否为长时运行工具
}

type EnvelopeStateDelta struct {
    Operation string // 操作类型
    Path      string // State 路径
    ValueJSON string // 值 JSON
}

type EnvelopeTransfer struct {
    FromAgent string // 来源 Agent
    ToAgent   string // 目标 Agent
}

type EnvelopeError struct {
    Type      string // 错误类型
    Code      string // 稳定错误码（与 TurnErrorCode / provider type 对齐）
    Message   string // 错误消息
    Hint      string // 用户可操作建议
    PendingID string // 关联的待执行消息 ID（待执行失败时填充）
}

type EnvelopeUsage struct {
    PromptTokens     int64 // 本轮 prompt（ReAct 多轮取 max）
    CompletionTokens int64 // 本轮 completion（ReAct 多轮累加）
    TotalTokens      int64 // 本轮合计
    MaxTokens        int64 // 上下文窗口上限（Agent context_window）
    TurnTotalTokens  int64 // 同 TotalTokens，显式语义字段
}

type EnvelopeExtensions struct {
    SkipSummarization bool // 跳过摘要压缩标记
}

type EnvelopeActions struct {
    SkipSummarization bool // 跳过摘要压缩动作
}

type EnvelopeTrace struct {
    AgentName    string // Agent 名称
    InvocationID string // 调用 ID
    StepCount    int64  // 步骤计数
    DurationMS   int64  // 持续时间
}
```

#### 5.5.2 事件类型与载荷映射

| Envelope type | Channel | 主要载荷字段 | 说明 |
|------|------|------|------|
| `text_delta` | chat/team | `content.text`, `content.reasoning`, `content.is_partial` | 模型增量文本 |
| `text_done` | chat/team | `content.text`, `usage` | 模型最终文本 |
| `tool_call` | chat/team | `tool_call.id/name/arguments_json/status/is_long_running` | 工具调用通知 |
| `tool_result` | chat/team | `tool_call.result_json/status/duration_ms` | 工具结果 |
| `state_delta` | chat/team | `state_delta.operation/path/value_json` | Runner State 增量 |
| `runner_completion` | chat/team | `usage` | 一轮运行完成 |
| `error` | chat/system | `error.type/code/message/hint/pending_id` | 错误信息；`pending_id` 在待执行消息失败时填充 |
| `user_feedback` | chat | `metadata.message_id/rating/comment` | 用户对助手消息的 👍/👎 反馈 |
| `intent_pass` | chat/team | `metadata` | 意图识别结果 |
| `transfer` | team | `transfer.from_agent/to_agent` | Team/Swarm 转交 |
| `member_message_start` | team | `author`, `content` | 成员消息开始；类型已定义，仍需 Team Runner 稳定发射 |
| `member_delta` | team | `author`, `content` | 成员消息增量 |
| `member_message_done` | team | `author`, `content`, `usage` | 成员消息完成 |

> **pending_id 字段约定**：待执行消息执行失败时，`error.pending_id` 应填充关联的待执行消息 ID。当前 `processPendingQueue` 中统一使用 `env.Error.PendingID = entry.ID`，metadata 双写已移除。

### 5.6 runNativeAgentTurn 核心流程

```
1. 校验 session_id 和 content
2. 检查 activeRuns → 如果正在运行则入队 pendingQueue 并返回
3. 查询 Session
4. 判断 owner_type:
   - "team" → teamsNative.RunTurn()
   - "agent" → runSingleAgentViaTRPC()
5. 对于 agent 路径:
   a. hydratedAgent() 获取完整 Agent 配置
   b. 解析 dialogMode/provider/model（优先级：请求 > Session > Agent）
   c. runSingleAgentViaTRPC()
```

注意：
- 步骤 2 的 `activeRuns` 检查由 ChatService 维护，`lockSession` per-session 互斥锁保护 Load/Store 原子性。Team 路径通过 `teamRunGuard` 接入 `activeRuns`，与单 Agent 的 stop/pending 行为一致。
- Team turn 完成后通过 defer 调用 `processPendingQueue`，内部按 `OwnerType` 路由到 `teamsNative.RunTurn` 或 `runSingleAgentViaTRPC`。

### 5.7 runSingleAgentViaTRPC 核心流程

```
1. 校验 agent_key 匹配
2. 构建 TRPCBuilderDeps
3. BuildTRPCLLMAgentCached() → root Agent
4. 构建 TRPCRunnerDeps（SessionService + MemoryService）
5. NewTRPCRunner() → runner
6. activeRuns.Store(sessionID, runner)
7. defer: activeRuns.Delete + runner.Close + processPendingQueue
8. 构建 UserOptionsJSON
9. 运行意图识别 intent.Run()
   - 成功时：MergeIntoUserOptionsJSON 合并到 options_json
   - 成功时：WrapUserMessage 嵌入 artifact 到用户消息
   - EventBus Publish intent_pass Envelope（单 Agent 和 Team 均发送）
10. 构造 userMsg → AppendChatMessage
11. RunTRPCUserTurn() → events channel
12. 遍历事件流:
    - EventProjector.ProjectAndPublish → EventBus Envelope
    - Response.Choices → 累积 reply/reasoning
    - ToolCalls → 投影为 tool_call / tool_result Envelope
    - Usage → 记录 promptTok/completionTok
    - ctx.Err() != nil 时退出循环（取消/超时）
13. 构造 assistantMsg
14. 持久化消息（AppendChatMessage）
15. recordSessionTurn() 写入 session_turns 表
16. patchSessionContextUsage
17. setRunStatus(completed)
```

### 5.7.1 WS 连接与取消

```
1. WSServer 限制每个 session 最多 5 条连接（maxSessionConns=5）
2. readPump/writePump 使用心跳与写超时维护连接
3. 客户端上行 cancel → ChatService.CancelRun(sessionID)
4. StopGeneration RPC 与 WS cancel 共用 activeRuns / pendingCancels
5. last_event_id + EventBuffer 支持断线后的事件回放
6. 全局监控连接（globalMode，sessionId="*"）可订阅所有 Session 事件流
```

### 5.7.2 processPendingQueue 错误处理

```
1. dequeuePending 取出待执行消息
2. 启动 goroutine，设置 600s 超时 + cancel 传播
3. 调用 runSingleAgentViaTRPC 执行
4. 执行失败时：发布 error Envelope
5. WS 前端收到 error 后显示通知
```

> **pending_id 约定**：待执行消息执行失败时，`error.pending_id` 应填充关联的待执行消息 ID。当前 `processPendingQueue` 中统一使用 `env.Error.PendingID = entry.ID`，metadata 双写已移除。

### 5.7.3 pendingQueue 容量控制

```
- maxPendingPerSession = 32
- enqueuePending 检查队列长度，超出时返回空 ID
- runNativeAgentTurn 中检测空 ID，返回 BadRequest 错误
```

### 5.8 用量记录

```go
func recordChatIngressUsage(ctx, uc, req, am, streamEnabled) {
    // 1. 检查 CHAT_RECORD_USAGE_INGRESS 环境变量
    // 2. 从 agent_message 提取 token_in/token_out/latency_ms
    // 3. 如果 API 未返回 usage，使用 roughTokenEstimateFromText 估算
    // 4. 计算 tokens_per_second
    // 5. 构造 TokenUsageEvent 并写入
    // 6. 使用独立 context.WithoutCancel 避免请求超时影响记录
}
```

### 5.9 L0 上下文压缩

```go
type SessionCompressor struct {
    Sessions    *biz.SessionUsecase
    Agents      biz.AgentRepository
    Compress    compress.Compressor
    RT          *runtimedeps.Runtime
    MonitorLogs *biz.MonitorLogBroker
    inFlight    sync.Map  // sessionID → bool（防重入）
}

func (c *SessionCompressor) AfterNativeTurn(ctx, sessionID, ag) {
    // 1. 异步执行，inFlight 防重入
    // 2. 查询 Session，检查 context_used_ratio 是否超过阈值（默认 0.6）
    // 3. 100% 使用率时立即压缩，否则检查距上次压缩是否超过 10 分钟
    // 4. 获取消息列表，计算需要压缩的范围
    // 5. 调用 Compress.Compress() 生成摘要
    // 6. 插入 SessionSummary 记录
    // 7. 合并所有摘要，更新 Session Runner Snapshot
}
```

### 5.10 Session 标题自动生成

```go
func (uc *SessionUsecase) maybeAutoTitleFromUserMessage(ctx, sessionID, content) error {
    // 1. 查询 Session，检查标题是否为默认占位符
    // 2. 先用截取方式快速设置标题（即时反馈）
    // 3. 异步调用 generateTitleAsync
}

func (uc *SessionUsecase) generateTitleAsync(sessionID, content) {
    // 1. 15s 超时
    // 2. 调用 titleGenerator.Generate()
    // 3. 成功则更新标题
}

type LLMSessionTitleGenerator struct {
    catalog *biz.LlmProviderModelUsecase
    rt      *provider.RoundTrip
}

func (g *LLMSessionTitleGenerator) Generate(ctx, userMessage) (string, error) {
    // 1. 从 catalog 选择轻量模型（mini/flash/lite/small）
    // 2. 构造请求：system prompt + user message
    // 3. 调用 LLM，流式读取响应
    // 4. 截取前 50 字符作为标题
}
```

### 5.11 EventBuffer 设计

```go
type EventBuffer struct {
    mu       sync.RWMutex
    buffers  map[string]*ringBuffer  // sessionID → ring buffer
    capacity int                     // 每个 buffer 容量，默认 200
}

type ringBuffer struct {
    events []*Envelope
    head   int
    tail   int
    size   int
}
```

- **容量**：每个 Session ring buffer 容量 200 条事件
- **写入**：`EventBus.Publish` 时同步写入对应 Session 的 ring buffer
- **回放**：WS 重连时客户端携带 `last_event_id`，`EventBuffer.Replay()` 从该 ID 之后开始回放
- **回放协议**：`replay_start` → 事件序列 → `replay_end`
- **清理策略**：TTL 30min 自动过期 + 5min eviction ticker + `Close()` 优雅停止；`lastAcc` 追踪最后访问时间

### 5.12 EventBus 订阅与背压

```go
type SubscribeOptions struct {
    SessionID  string
    TeamID     string
    Channel    string
    FilterKey  string
    EventTypes []string
    LevelFilter string
}

type Bus interface {
    Publish(ctx context.Context, env *Envelope) error
    Subscribe(opts SubscribeOptions) (<-chan *Envelope, UnsubscribeFunc)
}
```

- **订阅选项**：SessionID / TeamID / Channel / FilterKey / EventTypes / LevelFilter
- **FilterKey 匹配**：采用前缀匹配规则（`MatchFilterKey`），订阅者 FilterKey 为事件 FilterKey 的前缀时匹配
- **背压策略**：当订阅者消费速度落后于生产速度时，EventBus 丢弃非关键事件
- **关键事件（不丢弃）**：`tool_result`、`error`、`runner_completion`、`graph_node_end`、`team_run_finished`、`team_run_failed`
- **全局监控**：WS 连接 `globalMode`（sessionId=`*`）可订阅所有 Session 的事件流

### 5.13 Channel/Cron 入口

#### 5.13.1 Channel Ingress

```
POST /v1/channel/{channel_type}/webhook
  → ChannelIngress.HandleWebhook()
    → 解析 channel_type（lark/feishu/slack 等）
    → 验证签名
    → 转换为 SendChatMessageRequest
    → ChatService.RunNativeTurnUnary()
      → 同一 runNativeAgentTurn() 主链路
```

- Channel webhook 为并发入口，多个 webhook 可能同时触发同一 Session 的对话
- **并发保护**：`lockSession` per-session 互斥锁 + `runPlaceholder` 原子占位，确保 `RunNativeTurnUnary` 受 activeRuns/pendingQueue 保护

#### 5.13.2 Cron Turn

```
Cron Scheduler
  → ChatService.RunCronTurn()
    → 构造 SendChatMessageRequest（content 为 cron 触发内容）
    → 同一 runNativeAgentTurn() 主链路
```

- Cron turn 与手动对话共用 `runNativeAgentTurn`，受相同的 activeRuns/pendingQueue 保护

### 5.14 Team Run 持久化

```
team_runs
├── id               TEXT (UUID)
├── team_id          TEXT
├── session_id       TEXT
├── status           TEXT (running/completed/failed)
├── started_at       TEXT
├── completed_at     TEXT
├── error_message    TEXT
├── created_at       TEXT

team_run_steps
├── id               TEXT (UUID)
├── team_run_id      TEXT
├── agent_id         TEXT
├── agent_key        TEXT
├── step_type        TEXT
├── status           TEXT
├── input_preview    TEXT
├── output_preview   TEXT
├── duration_ms      INTEGER
├── created_at       TEXT
```

- Team turn 执行时 `CreateTeamRun()` 写入 `team_runs` 表
- Team turn 中每个成员 Agent 执行步骤通过 `CreateTeamRunStep()` 写入 `team_run_steps` 表
- Team 定义可配置 `intent_anchor_agent_id`（指定意图识别使用的成员 Agent）和 `TurnDeadlineDuration`（turn 超时时间）

### 5.15 SessionTurn 持久化

```
session_turns
├── id               TEXT (UUID)
├── session_id       TEXT
├── turn_index       INTEGER
├── role             TEXT (user/assistant/tool)
├── model_name       TEXT
├── input_tokens     INTEGER
├── output_tokens    INTEGER
├── latency_ms       INTEGER
├── created_at       TEXT
```

- 单 Agent turn 完成后，`recordSessionTurn()` 写入 `session_turns` 表
- Team turn 完成后，`recordTeamSessionTurn()` 写入 `session_turns` 表，与单 Agent 行为一致

### 5.16 Agent Settings Variables 注入

```go
func ParseVariablesJSON(variablesJSON string) (map[string]interface{}, error)
func MergeRuntimeState(state map[string]interface{}, variables map[string]interface{}) map[string]interface{}
```

- Agent 配置中的 `variables_json` 字段存储自定义变量
- `runSingleAgentViaTRPC` 执行时通过 `ParseVariablesJSON` → `MergeRuntimeState` 将变量注入 Runner State
- 变量可在 System Prompt 中通过占位符引用（如 `{{variable_name}}`）

### 5.17 可观测性

- Chat turn 耗时通过 `arametrics.ChatTurnDuration` Prometheus 指标记录
- 意图识别超时为 45 秒
- Context Window 默认值：当 Agent 配置的 `context_window` ≤ 0 时，默认使用 128000 tokens

---

## 六、运行时层

### 6.1 Agent 构建

```go
type TRPCBuilderDeps struct {
    Catalog    *biz.LlmProviderModelUsecase
    AgentUC    *biz.AgentUsecase
    Agents     biz.AgentRepository
    RT         *provider.RoundTrip
    SkillUC    *biz.SkillUsecase
    MCPTooling interface{}
    ToolUC     *biz.ToolUsecase
    Sys        biz.SystemSettingRepo
    Provider   string
    Model      string
    DialogMode string
}

func BuildTRPCLLMAgent(ctx, ag biz.Agent, deps TRPCBuilderDeps) (trpcagent.Agent, error) {
    // 1. 获取 LLM 模型
    // 2. 构建 System Prompt（含占位符变量替换）
    // 3. 挂载工具（Builtin + Skill + MCP）
    // 4. 构建 Agent
}
```

### 6.2 Runner 构建

```go
type TRPCRunnerDeps struct {
    AppName        string
    SessionService trpcsession.Service
    MemoryService  trpcmemory.Service
}

func NewTRPCRunner(root trpcagent.Agent, deps TRPCRunnerDeps, opts ...trpcrunner.Option) (trpcrunner.Runner, error) {
    // 注入 SessionService 和 MemoryService（可选）
    // 返回 trpcrunner.NewRunner(appName, root, opts...)
}

func RunTRPCUserTurn(ctx, r trpcrunner.Runner, userID, sessionID, content string, opts ...trpcagent.RunOption) (<-chan *trpcevent.Event, error) {
    // 执行一轮用户对话
}
```

### 6.3 Team 编排

```go
type Runner struct {
    teams    biz.TeamRepository
    sessions *biz.SessionUsecase
    agents   biz.AgentRepository
    agentsUC *biz.AgentUsecase
    tools    biz.ToolRepo
    llm      *biz.LlmProviderModelUsecase
    broker   *biz.TeamRunEventBroker
    skills   *biz.SkillUsecase
    sys      biz.SystemSettingRepo
    rt       *runtimedeps.Runtime
    compress biz.NativeTurnCompressor
    logs     *biz.MonitorLogBroker
}

func (r *Runner) RunTurn(ctx, sess biz.Session, req *chatv1.SendChatMessageRequest, emitter agent.StreamEmitter) (biz.ChatMessage, biz.ChatMessage, error) {
    // 1. 查询 Team 成员 Agent 列表
    // 2. 构建 Team Root Agent（Coordinator 或 Swarm）
    // 3. 构建 Runner
    // 4. 执行
    // 5. 投影事件流 → ChatMessage + EventBus Envelope
}
```

---

## 七、Wire 注入

### 7.1 ProviderSet

```go
var ProviderSet = wire.NewSet(
    NewChatService,
    provideChatServiceDeps,
    NewSessionCompressor,
    NewLLMSessionTitleGenerator,
    // ... 其他 Service
)
```

### 7.2 provideChatServiceDeps

```go
func provideChatServiceDeps(
    broker *biz.TeamRunEventBroker,
    teams biz.TeamRepository,
    teamsNative *team.Runner,
    usage *biz.UsageUsecase,
    sessions *biz.SessionUsecase,
    agents biz.AgentRepository,
    agentsUC *biz.AgentUsecase,
    toolsCatalog biz.ToolRepo,
    toolUC *biz.ToolUsecase,
    llmCatalog *biz.LlmProviderModelUsecase,
    skillUC *biz.SkillUsecase,
    sys biz.SystemSettingRepo,
    rt *runtimedeps.Runtime,
    compress biz.NativeTurnCompressor,
    monitorLogs *biz.MonitorLogBroker,
) service.ChatServiceDeps { ... }
```

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/
├── services/index.ts              ← createChatService 导出
├── features/chat/
│   ├── api.ts                     ← Chat API 调用封装（sendMessage/stop/listOptions/getPending/cancelPending/updatePending/getRunStatus/awaitUserReply）
│   ├── types.ts                   ← TypeScript 类型定义
│   ├── toolEventMarkdown.ts       ← 工具事件 Markdown 渲染 + toolEventToMessage 共享转换函数
│   └── composables/
│       ├── useChatWorkspace.ts    ← 对话工作区 composable（状态管理 + 交互逻辑）
│       ├── useChatStream.ts       ← 单 Agent WS 事件流消费（text_delta/tool_call/tool_result 等）
│       ├── useTeamStream.ts       ← Team WS 事件流消费（member_* / team_run_* 等）
│       ├── useEnvelopeStream.ts   ← WS Envelope 底层消费（EnvelopeDispatcher + 事件分发）
│       └── useRunStatus.ts        ← RunStatus 轮询 + AwaitUserReply 回复提交
├── composables/
│   ├── useWsTransport.ts          ← WS 连接管理（WsTransport：连接/重连/心跳/断线回放）
│   └── useEnvelopeDispatcher.ts   ← Envelope 分发器（按 type 分发到对应 handler）
├── components/chat/
│   ├── ChatWorkspaceShell.vue     ← 工作区外壳（标题 + 三栏布局容器）
│   ├── ChatEntitySidebar.vue      ← 左侧 Agent/Team 列表
│   ├── ChatSessionSidebar.vue     ← 右侧 Session 历史
│   ├── ChatMessagePanel.vue       ← 中间对话内容 + 输入区域
│   ├── ChatSideToggle.vue         ← 侧栏折叠按钮
│   ├── ChatSettingsDialog.vue     ← Agent/Team 设置弹框
│   ├── ChatDeleteDialog.vue       ← 删除确认弹框
│   ├── SessionTimelineDialog.vue  ← Session 历史追踪弹框
│   └── types.ts                   ← 组件级类型定义
├── config/chatOptions.ts          ← 对话模式/模型配置
├── stores/app.ts                  ← 全局状态（含 Agent/Session 选择）
```

### 8.1.1 前端事件流架构

```
WsTransport（WS 连接管理：连接/重连/心跳/last_event_id 回放）
  ↓ Envelope 流
EnvelopeDispatcher（按 type 分发：text_delta/tool_call/error/state_delta/...）
  ↓ 分流
useChatStream（单 Agent 事件：text_delta/text_done/tool_call/tool_result/runner_completion）
useTeamStream（Team 事件：member_*/team_run_*/transfer）
useRunStatus（RunStatus 轮询 + isAwaiting + submitReply）
  ↓ 状态聚合
useChatWorkspace（对话工作区：消息列表/pending/发送/停止/上下文比）
  ↓ 渲染
ChatMessagePanel（消息气泡/工具事件/待执行列表/输入框）
```

### 8.2 页面布局

```
┌──────────┬─────────────────────────────────┬──────────┐
│ Agent/   │        ChatWorkspaceShell        │ Session  │
│ Team     │  ┌───────────────────────────┐  │ 历史     │
│ 列表     │  │  Session 标题 + 上下文比   │  │ 列表     │
│          │  ├───────────────────────────┤  │          │
│ 120px    │  │                           │  │ 120px    │
│          │  │   对话内容区域             │  │          │
│          │  │   (q-chat-message)        │  │          │
│          │  │                           │  │          │
│          │  │   待执行消息列表           │  │          │
│          │  ├───────────────────────────┤  │          │
│          │  │  附件区域                  │  │          │
│          │  │  输入框 (autogrow)         │  │          │
│          │  │  [模式][Provider][上下文]  │  │          │
│          │  │              [文件][发送]  │  │          │
│          │  └───────────────────────────┘  │          │
└──────────┴─────────────────────────────────┴──────────┘
```

### 8.3 TypeScript 类型定义

```typescript
export type Message = {
  id: string;
  session_id: string;
  parent_message_id: string;
  turn_index: number;
  role: string;
  content_markdown: string;
  model_name: string;
  token_in: number;
  token_out: number;
  latency_ms: number;
  status: string;
  attachments_count: number;
  options_json: string;
  error_message: string;
  created_at: string;
};

export type ChatOption = {
  type: string;
  key: string;
  label: string;
  enabled: boolean;
  sort_order: number;
  metadata_json: string;
};

export type SendMessageOptions = {
  dialog_mode?: string;
  provider?: string;
  model?: string;
  attachments?: Array<{ id: string }>;
};

export type SendMessageResult = {
  user_message: Message;
  agent_message: Message;
};

export type SendMessageStreamCallbacks = {
  signal?: AbortSignal;
  onUserMessage?: (message: Message) => void;
  onDelta?: (content: string) => void;
  onDone?: (message: Message) => void;
  onToolEvent?: (event: ToolUseEvent) => void;
  onMemberMessageStart?: (message: Message) => void;
  onMemberDelta?: (messageID: string, content: string) => void;
  onMemberMessageDone?: (message: Message) => void;
  onIntentPass?: (result: IntentPassResult) => void;
  onStateDelta?: (sessionId: string, stateDelta: Record<string, unknown>) => void;
  onExtensions?: (sessionId: string, extensions: Record<string, unknown>) => void;
  onError?: (message: string, pendingId?: string) => void;
};

export type IntentPassResult = {
  outcome: string;
  duration_ms: number;
  session_id?: string;
  agent_id?: string;
  intent_kind?: string;
  refined_goal_len?: number;
  search_hints_count?: number;
};

export type ToolUseEvent = {
  id: string;
  phase: "before" | "after" | string;
  status: "running" | "success" | "error" | "failed" | "blocked" | string;
  agent_id: string;
  agent_key: string;
  agent_name: string;
  agent_icon: string;
  tool_name: string;
  tool_label: string;
  arguments?: Record<string, unknown>;
  result?: Record<string, unknown>;
  error?: string;
  occurred_at: string;
  duration_ms?: number;
  message_hint?: string;
};
```

> **⚠️ 废弃提示**：上述 `SendMessageStreamCallbacks` 类型来自历史 SSE API，当前 Chat 页面主路径使用 `useEnvelopeStream` / `useChatStream` / `useTeamStream` 消费 WS Envelope。该类型仅作为向后兼容保留，后续应删除或迁移残留 SSE callback 类型，避免误导新开发者。

### 8.4 API 调用

```typescript
export async function sendMessage(payload: {...}): Promise<SendMessageResult>
export async function listChatOptions(type?: string): Promise<ChatOption[]>
export async function stopGeneration(sessionId: string): Promise<boolean>
export async function getPendingMessages(sessionId: string): Promise<PendingMessage[]>
export async function cancelPendingMessage(sessionId: string, pendingId: string): Promise<boolean>
export async function updatePendingMessage(sessionId: string, pendingId: string, content: string): Promise<boolean>
export async function getRunStatus(sessionId: string): Promise<RunStatus>
export async function awaitUserReply(sessionId: string, reply: string, runId?: string): Promise<boolean>
```

> **⚠️ 废弃提示**：历史 `sendMessageStream()` 函数基于 SSE 实现，当前 Chat 页面主路径使用 WS `useEnvelopeStream` 消费实时事件。该函数不应在新代码中使用。

### 8.5 组件设计

#### ChatEntitySidebar

| Prop | 类型 | 说明 |
|------|------|------|
| `open` | `boolean` | 是否展开 |
| `search` | `string` | 搜索关键词 |
| `agents` | `Agent[]` | Agent 列表 |
| `teams` | `TeamRow[]` | Team 列表 |
| `categoryTree` | `PlatformResourceTreeNode[]` | 分类树 |
| `selectedKind` | `ChatEntityKind` | 选中类型 |
| `selectedAgentId` | `string \| null` | 选中 Agent ID |
| `selectedTeamId` | `string \| null` | 选中 Team ID |
| `isDark` | `boolean` | 暗黑模式 |

| Emit | 载荷 | 说明 |
|------|------|------|
| `select-agent` | `Agent` | 选中 Agent |
| `select-team` | `TeamRow` | 选中 Team |
| `settings` | `kind, id` | 打开设置 |
| `delete` | `kind, id` | 删除 |

#### ChatSessionSidebar

| Prop | 类型 | 说明 |
|------|------|------|
| `open` | `boolean` | 是否展开 |
| `sessions` | `SessionView[]` | Session 列表 |
| `selectedSessionId` | `string \| null` | 选中 Session ID |
| `isDark` | `boolean` | 暗黑模式 |

| Emit | 载荷 | 说明 |
|------|------|------|
| `select` | `string` | 选中 Session |
| `new-session` | — | 新建 Session |
| `rename` | `{id, title}` | 重命名 |
| `delete` | `kind, id` | 删除 |
| `trace` | `string` | 历史追踪 |

#### ChatMessagePanel

| Prop | 类型 | 说明 |
|------|------|------|
| `messages` | `Message[]` | 消息列表 |
| `pendingMessages` | `PendingMessage[]` | 待执行消息 |
| `sessionTitle` | `string` | Session 标题 |
| `contextRatio` | `number` | 上下文使用比例 |
| `isDark` | `boolean` | 暗黑模式 |
| `sending` | `boolean` | 是否正在发送 |
| `dialogMode` | `string` | 对话模式 |
| `modelProvider` | `string` | 当前 Provider/Model |
| `attachments` | `ChatAttachment[]` | 附件列表 |

| Emit | 载荷 | 说明 |
|------|------|------|
| `send` | `{content, attachments}` | 发送消息 |
| `stop` | — | 停止生成 |
| `cancel-pending` | `pendingId` | 取消待执行消息 |
| `update-pending` | `pendingId, content` | 编辑待执行消息 |
| `update:modelValue` | `string` | 输入框内容 |
| `update:dialogMode` | `string` | 切换对话模式 |
| `update:modelProvider` | `string` | 切换模型 |
| `remove-attachment` | `string` | 移除附件 |

### 8.6 UX 规范

- 玻璃材质：`background: var(--glass-surface); backdrop-filter: blur(var(--glass-blur-default))`
- 日间主操作色：`var(--color-accent)` = `#E9A23B`
- 夜间霓虹强调：`var(--color-neon-cyan)` = `#00E5FF`
- 输入框圆角：12-16px
- 消息气泡圆角：16-20px
- 暗黑模式可读性：聊天记录正文、代码块、工具结果、时间戳等文本必须保证对比度
- 上下文进度颜色阈值：`<0.6` 绿 / `0.6-0.8` 黄 / `>0.8` 红

---

## 九、待优化项

| 优先级 | 项目 | 说明 | 状态 |
|--------|------|------|------|
| P0 | `NewChatService` 参数封装 | 改为 `ChatServiceDeps` struct | ✅ 已完成 |
| P0 | `CancelPendingMessage` RPC | 新增取消待执行消息端点 | ✅ 已实现 |
| P1 | `firstNonEmpty` 统一 | 6 处重复定义 → `pkg/strutil.FirstNonEmpty` | ✅ 已完成 |
| P1 | `memory_decode.go` 提取 | `ifaceStr`/`ifaceBool` 等通用函数 → `pkg/jsonutil` | ✅ 已完成 |
| P1 | `compress_wire.go` 合并 | 仅含一个函数，合并到 `session_compress.go` | ✅ 已完成 |
| P1 | `err == sql.ErrNoRows` 修正 | 改为 `errors.Is(err, sql.ErrNoRows)` | ✅ 已修复 |
| P1 | 历史 SSE body 缺少 attachments | 旧 SSE handler 已移除；当前 WS 上行 `buildChatOptions` 已保留 attachments 引用 | ✅ 已替换为 WS 路径 |
| P1 | `hydratedAgent` 简化 | 逻辑冗余，移除 ephemeral AgentUsecase 分支 | ✅ 已优化 |
| P1 | `runAgentTurn` 移除 | 冗余中间方法，直接调用 `runSingleAgentViaTRPC` | ✅ 已移除 |
| P1 | Session 标题 LLM 生成 | 异步 LLM 生成高质量标题 | ✅ 已实现 |
| P2 | `legacychat` 废弃 | `LEGACY_REST_ORIGIN` 模式已移除，所有 Chat 请求直接由 admin 进程内处理 | ✅ 已移除 |
| P2 | `GetChatOptions` 动态化 | Provider/Model 选项从 LLM Catalog 动态获取 | ✅ 已实现 |
| P2 | `processPendingQueue` 超时 | 600 秒超时 + 取消传播（pendingCancels sync.Map） | ✅ 已实现 |
| P2 | `pendingEntry` ID 生成 | 使用 `github.com/google/uuid` 替代 `UnixNano` | ✅ 已实现 |
| P2 | `UpdatePendingMessage` RPC | 新增编辑待执行消息端点 | ✅ 已实现 |
| P2 | 意图识别增强 | 单 Agent/Team 通过 EventBus 发送 `intent_pass` Envelope + 前端展示 | ✅ 已实现 |
| P0 | WS 客户端断连与取消 | `WSServer` 心跳、连接上限、cancel 上行、EventBuffer 回放 | ✅ 已实现 |
| P0 | 实时事件统一投影 | `EventProjector` 将 trpc events 投影为 `text_delta/tool_call/state_delta/runner_completion` 等 Envelope | ✅ 已实现 |
| P0 | pendingQueue 大小限制 | `enqueuePending` 增加 `maxPendingPerSession=32` 上限，超出返回 `BadRequest` 错误 | ✅ 已修复 |
| P1 | processPendingQueue 错误上报 | 待执行消息执行失败时通过 WS `error` Envelope 通知前端；`pending_id` 字段统一到 `error.pending_id` | ✅ 已修复 |
| P1 | toolEventMessage 重复定义消除 | 提取 `toolEventToMessage` 到 `toolEventMarkdown.ts` 共享模块 | ✅ 已修复 |
| P1 | WS error 事件处理 | `useEnvelopeStream` / `useChatWorkspace` 监听 `error` Envelope 并通知用户 | ✅ 已修复 |
| P2 | state_delta/extensions 前后端覆盖 | Envelope 支持 `state_delta` / `extensions` 字段，前端类型已覆盖 | ✅ 已修复 |
| P1 | Team stop/pending 行为一致性 | Team turn 通过 `teamRunGuard` 接入 `activeRuns`，`lockSession` per-session 互斥锁保护 | ✅ 已修复 |
| P1 | Team processPendingQueue 缺失 | Team turn 完成后 defer 调用 `processPendingQueue`，内部按 `OwnerType` 路由 | ✅ 已修复 |
| P1 | AwaitUserReply 全链路 | 后端：Team Runner 通过 `SetAwaitHookProvider` 注入 `makeAwaitReplyFunc`；前端 Chat 页 UI 待闭环 | ✅ 后端 |
| P1 | EnvelopeError.PendingID 统一 | `env.Error.PendingID = entry.ID`，metadata 双写已移除 | ✅ 已修复 |
| P1 | WS 控制消息文档化 | `connected`/`pong`/`replay_*`/`server_shutdown` 已在本文档 §2.2 和需求文档 §1.4 描述；前端消费链路待确认 | ✅ 后端文档 |
| P2 | Team 成员级实时事件 | `member_*` 类型和前端处理存在，但 Team Runner 尚未稳定发射成员级 start/delta/done | ⏳ 待优化 |
| P2 | 工具事件结构化展示 | 当前有 `tool_call/tool_result` Envelope，但 Chat 面板仍是简化文本 | ⏳ 待优化 |
| P2 | SessionTurn 一致性 | 新增 `recordTeamSessionTurn`，Team turn 成功后调用 | ✅ 已修复 |
| P2 | Channel/Cron 并发保护 | `lockSession` per-session 互斥锁 + `runPlaceholder` 原子占位 | ✅ 已修复 |
| P2 | EventBuffer TTL 清理 | TTL 30min + 5min eviction ticker + `Close()` 优雅停止 | ✅ 已修复 |
| P2 | 前端 RunStatus 轮询改事件驱动 | `useRunStatus` 每 2s 轮询 HTTP，应改为 WS 事件驱动 | ⏳ 待优化 |
| P2 | Reasoning 展示规格 | 前端已消费 `content.reasoning`，但缺少展示规格定义 | ⏳ 待定义 |
| P3 | 模型选项来源统一 | 后端 `GetChatOptions("provider"|"model")` 已支持动态选项，Chat 前端主要读取 Platform Resource | ⏳ 待优化 |
| P3 | 多模态附件闭环 | proto/前端有 attachments，但后端缺上传、持久化、权限和 LLM Vision 输入 | ⏳ 待实现 |
| P3 | RunStatus/AwaitUserReply 可恢复性 | 当前为进程内 `sync.Map` / channel，服务重启不可恢复 | ⏳ 待优化 |
