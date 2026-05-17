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

客户端可在 WS 上行发送 `user_message` / `enqueue_message` / `cancel`，服务端复用 `ChatService.SendChatMessage` 与 `ChatService.CancelRun`，下行统一为 Envelope。

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
| `ChatOption` | `type` | string | — | 选项类型（如 dialog_mode） |
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
├── chat.go              ← ChatService 主结构 + SendChatMessage/GetChatOptions/StopGeneration/GetPendingMessages/CancelPendingMessage/UpdatePendingMessage/GetRunStatus/AwaitUserReply + pending queue
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
    activeRuns     sync.Map    // sessionID → trpcrunner.Runner
    pendingQueue   sync.Map    // sessionID → []pendingEntry
    pendingCancels sync.Map    // sessionID → context.CancelFunc
    runStatuses    sync.Map    // sessionID → *runStatusEntry
    awaitChans     sync.Map    // sessionID → chan awaitReplyCh
}

type ChatServiceDeps struct {
    Teams        biz.TeamRepository
    TeamsNative  *team.Runner
    Usage        *biz.UsageUsecase
    Sessions     *biz.SessionUsecase
    Agents       biz.AgentRepository
    AgentsUC     *biz.AgentUsecase
    ToolsCatalog biz.ToolRepo
    ToolUC       *biz.ToolUsecase
    LLMCatalog   *biz.LlmProviderModelUsecase
    SkillUC      *biz.SkillUsecase
    Sys          biz.SystemSettingRepo
    Persist      rt.PersistenceSet
    Compress     biz.NativeTurnCompressor
    EventBus     event.Bus
    PluginRT     *plugintrpc.Runtime
    SkillDBRepo  trpcskill.Repository
}

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

当前 `activeRuns` 由单 Agent `runSingleAgentViaTRPC` 登记；Team turn 由 `team.Runner` 独立执行，尚未纳入同一套 run registry，因此 Team 停止生成与待执行串行保护需后续补齐。

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
```

### 5.5 WebSocket Envelope 事件协议

| Envelope type | Channel | 载荷字段 | 说明 |
|------|------|------|------|
| `text_delta` | chat/team | `content.text`, `content.reasoning`, `content.is_partial` | 模型增量文本 |
| `text_done` | chat/team | `content.text`, `usage` | 模型最终文本 |
| `tool_call` | chat/team | `tool_call.id/name/arguments_json/status` | 工具调用通知 |
| `tool_result` | chat/team | `tool_call.result_json/status/duration_ms` | 工具结果 |
| `state_delta` | chat/team | `state_delta.operation/path/value_json` | Runner State 增量 |
| `runner_completion` | chat/team | `usage` | 一轮运行完成 |
| `error` | chat/system | `error.type/message/pending_id` | 错误信息 |
| `intent_pass` | chat/team | `metadata` | 意图识别结果 |
| `transfer` | team | `transfer.from_agent/to_agent` | Team/Swarm 转交 |
| `member_message_start/member_delta/member_message_done` | team | `author`, `content` | 成员级实时消息；类型已定义，仍需 Team Runner 稳定发射 |

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

注意：步骤 2 的 `activeRuns` 检查由 ChatService 维护，但当前只有单 Agent 路径会在运行期写入 `activeRuns`。Team 路径需要补会话级 run registry 或等价互斥，才能与单 Agent 的 stop/pending 行为完全一致。

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
15. patchSessionContextUsage
16. setRunStatus(completed)
```

### 5.7.1 WS 连接与取消

```
1. WSServer 限制每个 session 最多 5 条连接，全局监控连接最多 3 条
2. readPump/writePump 使用心跳与写超时维护连接
3. 客户端上行 cancel → ChatService.CancelRun(sessionID)
4. StopGeneration RPC 与 WS cancel 共用 activeRuns / pendingCancels
5. last_event_id + EventBuffer 支持断线后的事件回放
```

### 5.7.2 processPendingQueue 错误处理

```
1. dequeuePending 取出待执行消息
2. 启动 goroutine，设置 600s 超时 + cancel 传播
3. 调用 runSingleAgentViaTRPC 执行
4. 执行失败时：发布 error Envelope
5. WS 前端收到 error 后显示通知；当前 `pending_id` 写在 Envelope metadata，建议统一到 `error.pending_id` 或明确前端消费 metadata
```

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
│   ├── api.ts                     ← Chat API 调用封装（sendMessage/streamMessage/stop/listOptions/getPending/cancelPending/updatePending）
│   ├── types.ts                   ← TypeScript 类型定义
│   ├── toolEventMarkdown.ts       ← 工具事件 Markdown 渲染 + toolEventToMessage 共享转换函数
│   └── composables/
│       └── useChatWorkspace.ts    ← 对话工作区 composable（状态管理 + 交互逻辑）
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

> 兼容说明：上述 `SendMessageStreamCallbacks` 类型来自历史 SSE API。当前 Chat 页面主路径使用 `useEnvelopeStream` / `useChatStream` / `useTeamStream` 消费 WS Envelope；后续应删除或迁移残留 SSE callback 类型，避免误导。

### 8.4 API 调用

```typescript
export async function sendMessage(payload: {...}): Promise<SendMessageResult>
export async function sendMessageStream(payload: {...}, callbacks: SendMessageStreamCallbacks): Promise<void>
export async function listChatOptions(type?: string): Promise<ChatOption[]>
export async function stopGeneration(sessionId: string): Promise<boolean>
export async function getPendingMessages(sessionId: string): Promise<PendingMessage[]>
export async function cancelPendingMessage(sessionId: string, pendingId: string): Promise<boolean>
export async function updatePendingMessage(sessionId: string, pendingId: string, content: string): Promise<boolean>
export async function getRunStatus(sessionId: string): Promise<RunStatus>
export async function awaitUserReply(sessionId: string, reply: string, runId?: string): Promise<boolean>
```

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
| P1 | processPendingQueue 错误上报 | 待执行消息执行失败时通过 WS `error` Envelope 通知前端；`pending_id` 字段位置仍需统一 | 🟡 部分完成 |
| P1 | toolEventMessage 重复定义消除 | 提取 `toolEventToMessage` 到 `toolEventMarkdown.ts` 共享模块 | ✅ 已修复 |
| P1 | WS error 事件处理 | `useEnvelopeStream` / `useChatWorkspace` 监听 `error` Envelope 并通知用户 | ✅ 已修复 |
| P2 | state_delta/extensions 前后端覆盖 | Envelope 支持 `state_delta` / `extensions` 字段，前端类型已覆盖 | ✅ 已修复 |
| P1 | Team stop/pending 行为一致性 | Team turn 未登记 `ChatService.activeRuns`，停止与待执行队列主要覆盖单 Agent | ⏳ 待修复 |
| P1 | AwaitUserReply 全链路 | `AwaitHook` 仅注入单 Agent builder，Team builder 与 Chat 页 UI 未闭环 | ⏳ 待修复 |
| P2 | pending_id 字段统一 | pending 失败当前将 `pending_id` 写入 metadata，前端 error handler 未关联队列项 | ⏳ 待优化 |
| P2 | Team 成员级实时事件 | `member_*` 类型和前端处理存在，但 Team Runner 尚未稳定发射成员级 start/delta/done | ⏳ 待优化 |
| P2 | 工具事件结构化展示 | 当前有 `tool_call/tool_result` Envelope，但 Chat 面板仍是简化文本 | ⏳ 待优化 |
| P3 | 模型选项来源统一 | 后端 `GetChatOptions("provider"|"model")` 已支持动态选项，Chat 前端主要读取 Platform Resource | ⏳ 待优化 |
| P3 | 多模态附件闭环 | proto/前端有 attachments，但后端缺上传、持久化、权限和 LLM Vision 输入 | ⏳ 待实现 |
| P3 | RunStatus/AwaitUserReply 可恢复性 | 当前为进程内 `sync.Map` / channel，服务重启不可恢复 | ⏳ 待优化 |
