# Chat 对话模块 — 实现设计文档

> 对应需求：`1 chat.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`
>
> **文档边界**：本文档包含架构设计、代码分层、Proto/API 契约、数据模型、接口定义、技术选型、状态机、序列图、前端组件设计、UX 规范。用户故事、功能需求清单、验收标准见 [1-chat.md](./1-chat.md)；开发进度、任务清单、技术债务见 [1-chat.development.md](./1-chat.development.md)。

---

## 一、模块概述

Chat 是用户与 Agent/Team 交互的核心入口，负责 HTTP/WS 发起对话、WebSocket + EventBus 实时事件、上下文管理、用量记录、停止生成、人工等待回复与待执行队列。当前主链路已端到端可用；本设计文档记录现有实现架构并标注仍需完善的成员级事件、工具事件、附件与可恢复状态。

> **2026-05-17 现状对齐**：独立 SSE `/v1/chat/messages/stream` 路由已从当前代码移除，`internal/server/register_chat.go` 只注册 proto HTTP 服务；实时流由 `internal/server/ws.go` 的 `/v1/ws` 承载。

---

## 二、代码分层（遵循 AI-DEVELOPMENT-SPECIFICATION.md）

```
api/kratos/chat/v1/chat.proto        ← 对话 API 契约（发送、选项、停止、待执行、RunStatus、AwaitUserReply、Enqueue、Feedback、Confirm、Jobs）
        ↓
internal/server/ws.go                 ← WebSocket 实时通道（订阅、回放、取消、user_message 上行）
internal/server/ws_message_handler.go ← WS 上行消息分发（user_message/enqueue_message/cancel/ping/subscribe/unsubscribe/enable_log）
internal/server/ws_event.go           ← WS 事件订阅、回放、connected 握手
internal/server/ws_io_pump.go         ← WS read/write/event pump goroutines
internal/server/ws_priority.go        ← WS 三优先级发送队列 + 背压
internal/server/ws_codec.go           ← WS 协议类型（wsUpstream/wsDownstream）
internal/server/ws_conn.go            ← WS 连接生命周期 + connStore
internal/server/ws_conn_manager.go    ← WS 连接数限制与移除
        ↓
internal/service/chat.go              ← ChatService 薄传输桥（委托 ChatOrchestrator）
internal/service/chat_orchestrator.go ← ChatOrchestrator 编排核心
internal/service/chat_orchestrator_turn.go          ← Turn 编排
internal/service/chat_orchestrator_turn_phases.go   ← Turn 阶段实现
internal/service/chat_orchestrator_turn_dispatch.go ← Turn 分发
internal/service/chat_orchestrator_turn_api.go      ← Turn API
internal/service/chat_orchestrator_turn_metrics.go  ← Turn 指标
internal/service/chat_orchestrator_context_admission.go ← 上下文 admission
internal/service/chat_orchestrator_session_run.go   ← Session run 生命周期
internal/service/chat_orch_session_run_lifecycle.go ← Session run 生命周期细节
internal/service/chat_orch_run_status.go            ← RunStatus 管理
internal/service/chat_orch_pending_queue.go         ← Pending Queue 编排
internal/service/chat_orch_await.go                 ← AwaitUserReply 编排
internal/service/chat_orch_agent_build.go           ← Agent 构建
internal/service/chat_run_gateway.go                ← Biz 适配器 + publishMessageQueuedToBus
internal/service/chat_native.go                      ← 原生对话入口（HTTP unary + WS 上行复用）+ hydratedAgent
internal/service/chat_enqueue.go                     ← EnqueueUserMessage RPC
internal/service/chat_await_route.go                 ← AwaitUserReply 路由
internal/service/chat_await_resume.go                ← AwaitUserReply 恢复
internal/service/chat_durable_resume.go              ← Durable resume
internal/service/chat_attachments.go                 ← 附件处理
internal/service/chat_activity.go                    ← Activity 确认
internal/service/chat_confirm.go                     ← ConfirmActivity RPC
internal/service/chat_feedback.go                    ← SubmitMessageFeedback RPC
internal/service/chat_jobs.go                        ← 后台任务 RPC
internal/service/chat_event_publisher.go             ← Chat 事件发布
internal/service/chat_turn_admission.go              ← Turn admission（CHAT_TURN_BUSY）
internal/service/chat_turn_metrics.go                ← Chat turn 指标
internal/service/chat_usage_ingress.go               ← 用量记录
internal/service/chat_wire.go                        ← Wire 注入
internal/service/chat_session_run_cancel.go          ← Session run 取消
internal/service/turn_outcome.go                     ← ErrTurnMessageQueued / CHAT_TURN_BUSY
        ↓
internal/agent/trpc_build.go         ← Agent 构建（BuildTRPCLLMAgent）
internal/agent/trpc_build_router.go  ← Agent 构建路由
internal/agent/trpc_runtime.go       ← Runner 构建（NewTRPCRunner + RunTRPCUserTurn）
internal/agent/event_projector.go    ← trpc-agent-go event → EventBus Envelope
internal/agent/activity_projector.go ← Activity 投影
internal/agent/activity_publish.go   ← Activity 发布
internal/agent/activity_persist.go   ← Activity 持久化
internal/agent/activity_meta.go      ← ActivityKind 分类
internal/agent/activity_meta_resolver.go ← ActivityMetaResolver 接口
internal/agent/choice_stream.go      ← 流式 delta
internal/agent/stream_consumer.go    ← turn 消费
internal/agent/options.go            ← options_json 构建
internal/agent/intent/               ← 意图识别与消息增强
        ↓
internal/team/runner.go              ← Team Runner（Coordinator / Swarm）
internal/team/trpc_build.go          ← Team 构建（BuildTRPCTeam）
        ↓
internal/runtime/deps.go             ← 运行时依赖注入 DTO（TurnDeps / Runtime）
internal/runtime/run_registry.go     ← RunRegistry（active run / cancel / run status）
internal/runtime/pending_queue.go    ← PendingMessageQueue FIFO
internal/biz/chat_usecase.go         ← Follow-up Queue 编排（EnqueueUserMessage / Pending CRUD）
internal/biz/session/usecase.go      ← Session Usecase（含标题自动生成）
internal/biz/session/title.go        ← SessionTitleGenerator 接口
internal/biz/session/                ← Session 子包（status / turns / state / timeline / summary 等）
```

---

## 三、请求流转

```
前端 GET /v1/ws?session_id=... 建立 WebSocket
  → 上行 user_message / enqueue_message / cancel / ping / subscribe / unsubscribe / enable_log
    → WSServer.handleUserMessage() / handleEnqueueMessage()
      → ChatService.SendChatMessage() / EnqueueUserMessage()
        → ChatOrchestrator.nativeSendChatMessage() → runNativeAgentTurn()
          → session.owner_type == "team"?
            → team.Runner.RunTurn() → BuildTRPCTeam → trpc Runner → EventBus Envelope
          → session.owner_type == "agent"?
            → runSingleAgentViaTRPC()
              → BuildTRPCLLMAgentCached() → NewTRPCRunner() → RunTRPCUserTurn()
              → EventProjector → EventBus → WS 下行 Envelope

后台/非流式入口：
POST /v1/chat/messages
  → ChatService.SendChatMessage()
  → 同一 runNativeAgentTurn() 主链路

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

---

## 四、Proto 层

### 4.1 API 端点

| 方法 | 路径 | 协议 | 说明 |
|------|------|------|------|
| POST | `/v1/chat/messages` | unary | 非流式对话 |
| GET | `/v1/ws?session_id=...` | WebSocket | 实时事件主通道；支持订阅、回放、取消、user_message 上行 |
| GET | `/v1/chat/options` | unary | 获取对话选项 |
| POST | `/v1/chat/stop` | unary | 停止生成 |
| GET | `/v1/chat/pending` | unary | 获取待执行消息列表 |
| POST | `/v1/chat/pending/cancel` | unary | 取消待执行消息 |
| POST | `/v1/chat/pending/update` | unary | 编辑待执行消息 |
| POST | `/v1/chat/pending/interrupt-and-send` | unary | 中断并发送待执行消息 |
| GET | `/v1/chat/run-status` | unary | 查询当前/最近一次 Run 状态 |
| POST | `/v1/chat/await-reply` | unary | 提交人工等待回复 |
| POST | `/v1/chat/enqueue` | unary | 显式入队（WS `enqueue_message` 等价） |
| GET | `/v1/chat/jobs` | unary | 列出后台任务（按 session/agent/status 过滤） |
| POST | `/v1/chat/jobs/{id}/cancel` | unary | 取消后台任务 |
| POST | `/v1/chat/messages/{message_id}/feedback` | unary | 提交 👍/👎 反馈 |
| POST | `/v1/chat/activities/{activity_id}/confirm` | unary | 提交工具确认（approved=true 恢复，approved=false 取消） |

### 4.2 主要 Proto 定义

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
  // knowledge_bases limits knowledge_search to these collection IDs for this turn.
  repeated string knowledge_bases = 5;
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

message InterruptAndSendMessageRequest {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
  string pending_id = 2 [(google.api.field_behavior) = REQUIRED];
}

message InterruptAndSendMessageResponse {
  bool sent = 1;
}

// RunStatus represents the lifecycle state of an agent run within a session.
message RunStatus {
  string run_id = 1;
  // status is one of: idle | pending | running | awaiting_user | completed | failed | cancelled | sync.
  string status = 2;
  string error_message = 3;
  string updated_at = 4;
  string invocation_id = 5;
  string agent_name = 6;
  string started_at = 7;
  string last_event_at = 8;
  int32 event_count = 9;
  // await_kind distinguishes awaiting_user reasons: "" | "reply" | "tool_confirm".
  string await_kind = 10;
  string await_tool_key = 11;
  string await_tool_call_id = 12;
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

message EnqueueUserMessageRequest {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
  string content = 2 [(google.api.field_behavior) = REQUIRED];
}

message EnqueueUserMessageResponse {
  bool accepted = 1;
  bool queued = 2;
  string pending_id = 3;
}

message ListChatBackgroundJobsRequest {
  optional string session_id = 1;
  optional string agent_id = 2;
  optional string status = 3;
  optional int32 limit = 4;
}

message ChatBackgroundJob {
  string id = 1;
  string source = 2;
  string session_id = 3;
  string agent_id = 4;
  string status = 5;
  string target_type = 6;
  string target_id = 7;
  string created_at = 8;
  string updated_at = 9;
  optional string summary = 10;
  optional string error_message = 11;
  string channel_id = 12;
  optional string graph_id = 13;
  optional string turn_id = 14;
  optional string session_run_id = 15;
  optional string phase = 16;
}

message ListChatBackgroundJobsResponse {
  repeated ChatBackgroundJob items = 1;
}

message CancelChatBackgroundJobRequest {
  string id = 1 [(google.api.field_behavior) = REQUIRED];
  string source = 2;
}

message CancelChatBackgroundJobResponse {
  bool cancelled = 1;
}

message SubmitMessageFeedbackRequest {
  string message_id = 1 [(google.api.field_behavior) = REQUIRED];
  string session_id = 2 [(google.api.field_behavior) = REQUIRED];
  string rating = 3 [(google.api.field_behavior) = REQUIRED];
  optional string comment = 4;
}

message SubmitMessageFeedbackResponse {
  bool accepted = 1;
}

message ConfirmActivityRequest {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
  string activity_id = 2 [(google.api.field_behavior) = REQUIRED];
  bool approved = 3 [(google.api.field_behavior) = REQUIRED];
}

message ConfirmActivityResponse {
  bool accepted = 1;
  string status = 2;
}

service ChatService {
  rpc SendChatMessage(SendChatMessageRequest) returns (SendChatMessageResponse) {
    option (google.api.http) = { post: "/v1/chat/messages" body: "*" };
  }
  rpc GetChatOptions(GetChatOptionsRequest) returns (GetChatOptionsResponse) {
    option (google.api.http) = {get: "/v1/chat/options"};
  }
  rpc StopGeneration(StopGenerationRequest) returns (StopGenerationResponse) {
    option (google.api.http) = { post: "/v1/chat/stop" body: "*" };
  }
  rpc GetPendingMessages(GetPendingMessagesRequest) returns (GetPendingMessagesResponse) {
    option (google.api.http) = { get: "/v1/chat/pending" };
  }
  rpc CancelPendingMessage(CancelPendingMessageRequest) returns (CancelPendingMessageResponse) {
    option (google.api.http) = { post: "/v1/chat/pending/cancel" body: "*" };
  }
  rpc UpdatePendingMessage(UpdatePendingMessageRequest) returns (UpdatePendingMessageResponse) {
    option (google.api.http) = { post: "/v1/chat/pending/update" body: "*" };
  }
  rpc InterruptAndSendMessage(InterruptAndSendMessageRequest) returns (InterruptAndSendMessageResponse) {
    option (google.api.http) = { post: "/v1/chat/pending/interrupt-and-send" body: "*" };
  }
  rpc GetRunStatus(GetRunStatusRequest) returns (RunStatus) {
    option (google.api.http) = {get: "/v1/chat/run-status"};
  }
  rpc AwaitUserReply(AwaitUserReplyRequest) returns (AwaitUserReplyResponse) {
    option (google.api.http) = { post: "/v1/chat/await-reply" body: "*" };
  }
  rpc EnqueueUserMessage(EnqueueUserMessageRequest) returns (EnqueueUserMessageResponse) {
    option (google.api.http) = { post: "/v1/chat/enqueue" body: "*" };
  }
  rpc ListChatBackgroundJobs(ListChatBackgroundJobsRequest) returns (ListChatBackgroundJobsResponse) {
    option (google.api.http) = {get: "/v1/chat/jobs"};
  }
  rpc CancelChatBackgroundJob(CancelChatBackgroundJobRequest) returns (CancelChatBackgroundJobResponse) {
    option (google.api.http) = { post: "/v1/chat/jobs/{id}/cancel" body: "*" };
  }
  rpc SubmitMessageFeedback(SubmitMessageFeedbackRequest) returns (SubmitMessageFeedbackResponse) {
    option (google.api.http) = { post: "/v1/chat/messages/{message_id}/feedback" body: "*" };
  }
  rpc ConfirmActivity(ConfirmActivityRequest) returns (ConfirmActivityResponse) {
    option (google.api.http) = { post: "/v1/chat/activities/{activity_id}/confirm" body: "*" };
  }
}
```

### 4.3 WebSocket 实时端点（非 Proto 定义，HTTP Server 层注册）

实时对话事件通过 WebSocket + EventBus 下发，不在 Proto 中定义：

```
GET /v1/ws?session_id=...  →  WSServer.handleWS()
```

客户端可在 WS 上行发送 `user_message` / `enqueue_message` / `cancel` / `ping` / `subscribe` / `unsubscribe` / `enable_log`，服务端复用 `ChatService.SendChatMessage` 与 `ChatService.CancelRun`，下行统一为 Envelope。

### 4.4 消息字段说明

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
| `StopGenerationRequest` | `session_id` | string | ✅ | 要停止的会话 ID |
| `GetPendingMessagesRequest` | `session_id` | string | ✅ | 要查询的会话 ID |
| `CancelPendingMessageRequest` | `session_id` | string | ✅ | 会话 ID |
| | `pending_id` | string | ✅ | 待执行消息 ID |
| `UpdatePendingMessageRequest` | `session_id` | string | ✅ | 会话 ID |
| | `pending_id` | string | ✅ | 待执行消息 ID |
| | `content` | string | ✅ | 新消息内容 |
| `EnqueueUserMessageRequest` | `session_id` | string | ✅ | 会话 ID |
| | `content` | string | ✅ | 入队内容 |
| `EnqueueUserMessageResponse` | `accepted` | bool | — | 是否被接受（steer 或 pending） |
| | `queued` | bool | — | 是否降级到 pending queue |
| | `pending_id` | string | — | 入队后的 pending ID |
| `SubmitMessageFeedbackRequest` | `message_id` / `session_id` / `rating` | — | 👍/👎 反馈（positive \| negative） |
| `ConfirmActivityRequest` | `session_id` / `activity_id` / `approved` | — | 工具确认（true 恢复 / false 取消） |

---

## 五、WebSocket 协议

### 5.1 上行消息类型

| 上行 type | 说明 |
|-----------|------|
| `user_message` | 发送用户消息，触发 ChatService.SendChatMessage |
| `enqueue_message` | 发送用户消息，若当前有 run 则入队 pendingQueue |
| `cancel` | 取消当前 run（等同于 HTTP StopGeneration） |
| `ping` | 心跳探测，服务端回复 `pong` |
| `subscribe` | 订阅指定 session/team 的 EventBus 事件 |
| `unsubscribe` | 取消订阅 |
| `enable_log` | 开启运行日志推送（monitor channel） |

### 5.2 下行消息类型

| 下行 type | Channel | 说明 |
|-----------|---------|------|
| `connected` | system | WS 连接建立成功，携带 session_id 和连接元信息 |
| `pong` | system | 心跳回复 |
| `replay_start` | system | EventBuffer 回放开始标记 |
| `replay_end` | system | EventBuffer 回放结束标记 |
| `server_shutdown` | system | 服务端即将关闭通知 |
| `text_delta` / `text_done` | chat/team | 模型增量文本与最终文本 |
| `tool_call` / `tool_result` | chat/team | 工具调用与工具结果 |
| `state_delta` | chat/team | Runner State 增量 |
| `runner_completion` | chat/team | 一轮 Runner 完成，携带 usage |
| `error` | chat/system | 错误信息；待执行失败时携带 `pending_id`（见 §5.6） |
| `intent_pass` | chat/team | 意图识别结果 |
| `transfer` | team | Team/Swarm 转交 |
| `team_run_started` / `team_run_finished` / `team_run_failed` | team | Team run 生命周期 |
| `member_message_start` / `member_delta` / `member_message_done` | team | 成员级实时消息；`EventProjector` 在 `MemberAgentKeys` 下投影，前端 `useChatWorkspace` 消费 |
| `run_status` | chat/team | 运行生命周期；`metadata.hint=message_queued` 表示 Follow-up 入队成功 |
| `log` | monitor | 运行日志，需客户端通过 `enable_log` 上行开启订阅 |

### 5.3 下行 Envelope 结构

```json
{
  "direction": "server_to_client",
  "channel": "chat|team|monitor|graph|system",
  "envelope": {
    "id": "...",
    "type": "text_delta",
    "session_id": "...",
    "content": {"text": "...", "reasoning": "...", "is_partial": true},
    "tool_call": {"id": "...", "name": "...", "arguments_json": "...", "status": "...", "is_long_running": false},
    "state_delta": {"operation": "...", "path": "...", "value_json": "..."},
    "transfer": {"from_agent": "...", "to_agent": "..."},
    "error": {"type": "...", "message": "...", "pending_id": "..."},
    "usage": {"prompt_tokens": 0, "completion_tokens": 0},
    "tag": "...",
    "filter_key": "...",
    "branch": "...",
    "version": 0,
    "extensions": {"skip_summarization": false},
    "actions": {"skip_summarization": false},
    "trace": {"agent_name": "...", "invocation_id": "...", "step_count": 0, "duration_ms": 0}
  }
}
```

> **字段说明**：`content`/`tool_call`/`state_delta`/`transfer`/`error`/`usage` 为载荷字段，按 `type` 选择性填充；`tag`/`filter_key`/`branch`/`version`/`extensions`/`actions`/`trace` 为元数据字段，所有类型均可能携带。

---

## 六、Biz 层

### 6.1 领域模型

Chat 模块无独立 Biz 模型，依赖以下已有模型：

| 模型 | 包 | 用途 |
|------|-----|------|
| `biz.Agent` | `internal/biz` | Agent 配置查询 |
| `biz.Session` | `internal/biz` | 会话上下文 |
| `biz.Team` | `internal/biz` | Team 编排 |
| `biz.ChatMessage` | `internal/biz` | 对话消息 |
| `biz.TokenUsageEvent` | `internal/biz` | 用量记录事件 |
| `biz.SessionSummary` | `internal/biz` | 会话摘要 |

### 6.2 依赖的 Usecase/Repo 接口

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

### 6.3 Broker 接口

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

### 6.4 NativeTurnCompressor 接口

```go
type NativeTurnCompressor interface {
    AfterNativeTurn(ctx context.Context, sessionID, agentID string) error
}
```

### 6.5 SessionTitleGenerator 接口

```go
type SessionTitleGenerator interface {
    Generate(ctx context.Context, userMessage string) (string, error)
}
```

两个实现：
- `noopSessionTitleGenerator`：空实现，不生成标题
- `LLMSessionTitleGenerator`（`internal/service/session_title_llm.go`）：使用轻量 LLM 模型生成标题

---

## 七、Data 层

Chat 模块无独立 Data 层，通过以下已有表间接使用：

### 7.1 用量记录表

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

### 7.2 工具调用记录表

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

## 八、Service 层

### 8.1 文件结构

```
internal/service/
├── chat.go                          ← ChatService 薄传输桥（委托 ChatOrchestrator）
├── chat_orchestrator.go             ← ChatOrchestrator 编排核心
├── chat_orchestrator_turn.go        ← Turn 编排
├── chat_orchestrator_turn_phases.go ← Turn 阶段实现
├── chat_orchestrator_turn_dispatch.go ← Turn 分发
├── chat_orchestrator_turn_api.go    ← Turn API
├── chat_orchestrator_turn_metrics.go ← Turn 指标
├── chat_orchestrator_context_admission.go ← 上下文 admission
├── chat_orchestrator_session_run.go ← Session run 生命周期
├── chat_orch_session_run_lifecycle.go ← Session run 生命周期细节
├── chat_orch_run_status.go          ← RunStatus 管理
├── chat_orch_pending_queue.go       ← Pending Queue 编排
├── chat_orch_await.go               ← AwaitUserReply 编排
├── chat_orch_agent_build.go         ← Agent 构建
├── chat_run_gateway.go              ← Biz 适配器 + publishMessageQueuedToBus
├── chat_native.go                   ← 原生对话入口（admission + team/agent 路由）
├── chat_enqueue.go                  ← EnqueueUserMessage RPC
├── chat_await_route.go              ← AwaitUserReply 路由
├── chat_await_resume.go             ← AwaitUserReply 恢复
├── chat_durable_resume.go           ← Durable resume
├── chat_attachments.go              ← 附件处理
├── chat_activity.go                 ← Activity 确认
├── chat_confirm.go                  ← ConfirmActivity RPC
├── chat_feedback.go                 ← SubmitMessageFeedback RPC
├── chat_jobs.go                     ← 后台任务 RPC
├── chat_event_publisher.go          ← Chat 事件发布
├── chat_turn_admission.go           ← Turn admission（CHAT_TURN_BUSY）
├── chat_turn_metrics.go             ← Chat turn 指标
├── chat_usage_ingress.go            ← 用量记录
├── chat_wire.go                     ← Wire 注入
├── chat_session_run_cancel.go       ← Session run 取消
├── turn_outcome.go                  ← ErrTurnMessageQueued / CHAT_TURN_BUSY
└── session_title_llm.go             ← LLM 标题生成
```

### 8.2 ChatService 结构体

```go
// ChatService is the thin transport bridge between proto/HTTP/WS and the
// ChatOrchestrator. It only handles request validation, proto mapping, and
// delegates all orchestration work to ChatOrchestrator.
type ChatService struct {
    chatv1.UnimplementedChatServiceServer

    orch         *ChatOrchestrator
    turnPipeline *TurnPipeline
    lg           loggateway.Logger
}

func NewChatService(deps ChatOrchestratorDeps) *ChatService
```

`ChatService` 仅做协议桥接，所有编排逻辑下沉到 `ChatOrchestrator`。`PendingMessageQueue` 由 `internal/runtime/pending_queue.go` 提供，`RunRegistry` 由 `internal/runtime/run_registry.go` 提供，均通过 `ChatOrchestrator` 持有。

#### Follow-up Queue（对话阶段连续发送）

详见 [35 gateway.design.md §3.6](./35%20gateway.design.md#36-follow-up-queue对话阶段连续发送) 与 [1 chat.md §2.3](./1%20chat.md#23-待执行队列follow-up-queue)。

**入队**：`chat_native` 检测 `HasActive` → `chatUC.EnqueueUserMessage` → Steerable 或 Pending → `PublishMessageQueued`（`run_status` + `hint: message_queued`）。

**出队**：turn defer → `processPendingQueue`（迭代式循环，T1.3）→ `chatUC.DequeuePendingMessage` → 新 turn（同一 goroutine 内串行处理，避免递归 goroutine 链）。

**废弃**：`ChatService.publishMessageQueued` — 使用 `ChatUsecase` + `publishMessageQueuedToBus`。

### 8.3 RPC 方法实现

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
2. 调用 ChatOrchestrator.CancelRun(sessionID)
3. 返回 stopped 状态
```

当前 `RunRegistry` 由单 Agent **`StoreCancelable` → `StoreRunner`** 与 Team **`StoreCancelable`** 共同登记；`lockSession` per-session 互斥锁保护 admission。Turn 启动中且无 `ActiveRunner` 时返回 **`CHAT_TURN_BUSY`**；运行中追加消息经 `EnqueueUserMessage` 成功返回 **`ErrTurnMessageQueued`**（steer 或 Pending FIFO）。Team / 单 Agent turn 完成后 defer `Finish` 并调用 `processPendingQueue`（加锁；仍 active 则重新入队；defer 中检查 `inPendingLoop(ctx)` 抑制递归触发，T1.3）。

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

### 8.4 WebSocket 实时对话（/v1/ws）

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
      → ChatService.SendChatMessage() / EnqueueUserMessage()
        → ChatOrchestrator.nativeSendChatMessage() → runNativeAgentTurn()
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

### 8.5 WebSocket Envelope 事件协议

#### 8.5.1 Envelope 完整结构

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

#### 8.5.2 事件类型与载荷映射

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
| `member_message_start` | team | `author`, `content` | 成员消息开始；`EventProjector` 在 `MemberAgentKeys` 下稳定投影 |
| `member_delta` | team | `author`, `content` | 成员消息增量 |
| `member_message_done` | team | `author`, `content`, `usage` | 成员消息完成 |

> **pending_id 字段约定**：待执行消息执行失败时，`error.pending_id` 应填充关联的待执行消息 ID。当前 `processPendingQueue` 中统一使用 `env.Error.PendingID = entry.ID`，metadata 双写已移除。

### 8.6 runNativeAgentTurn 核心流程

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
- 步骤 2 的 `activeRuns` 检查由 `RunRegistry` 维护，`lockSession` per-session 互斥锁保护 Load/Store 原子性。Team 路径通过 `teamRunGuard` 接入 `activeRuns`，与单 Agent 的 stop/pending 行为一致。
- Team turn 完成后通过 defer 调用 `processPendingQueue`（迭代式循环，T1.3），内部按 `OwnerType` 路由到 `teamsNative.RunTurn` 或 `runSingleAgentViaTRPC`。defer 中检查 `inPendingLoop(ctx)` 标志，避免在 pending 循环内重复触发。

### 8.7 runSingleAgentViaTRPC 核心流程

> **No-Timeout（T1.1）**：`turnCtx, turnCancel := context.WithCancel(ctx)` — 仅 cancel，无 `WithTimeout`。原 24h `longTaskHardDeadline` 已移除，任务持续运行直到完成或用户取消。

```
1. 校验 agent_key 匹配
2. 构建 TRPCBuilderDeps
3. BuildTRPCLLMAgentCached() → root Agent
4. 构建 TRPCRunnerDeps（SessionService + MemoryService）
5. NewTRPCRunner() → runner
6. activeRuns.Store(sessionID, runner)
7. defer: activeRuns.Delete + runner.Close + processPendingQueue（inPendingLoop 抑制递归）
8. 构建 UserOptionsJSON
9. 运行意图识别 intent.Run()
   - 成功时：MergeIntoUserOptionsJSON 合并到 options_json
   - 成功时：WrapUserMessage 嵌入 artifact 到用户消息
   - EventBus Publish intent_pass Envelope（单 Agent 和 Team 均发送）
10. 构造 userMsg → AppendChatMessage
11. RunTRPCUserTurn() → events channel（使用 RoundTripForSession 注入 llm_retry 回调，T1.2）
12. 遍历事件流:
    - EventProjector.ProjectAndPublish → EventBus Envelope
    - Response.Choices → 累积 reply/reasoning
    - ToolCalls → 投影为 tool_call / tool_result Envelope
    - Usage → 记录 promptTok/completionTok
    - ctx.Err() != nil 时退出循环（仅用户取消；无超时）
13. 构造 assistantMsg
14. 持久化消息（AppendChatMessage）
15. recordSessionTurn() 写入 session_turns 表
16. patchSessionContextUsage
17. setRunStatus(completed)
```

### 8.8 WS 连接与取消

```
1. WSServer 限制每个 session 最多 5 条连接（maxSessionConns=5）
2. readPump/writePump 使用心跳与写超时维护连接
3. 客户端上行 cancel → ChatService.CancelRun(sessionID)
4. StopGeneration RPC 与 WS cancel 共用 RunRegistry / pendingCancels
5. last_event_id + EventBuffer 支持断线后的事件回放
6. 全局监控连接（globalMode，sessionId="*"）可订阅所有 Session 事件流
```

### 8.9 processPendingQueue 错误处理

> **No-Timeout 原则（Sprint 1 / T1.1, 2026-06-18）**：任务执行无任何时间上限，持续运行直到完成或用户取消。原 24h `longTaskHardDeadline`（`turnTimeout * 12`）已移除；单 Agent / Team / pending queue / resumeAwait 路径统一使用 `context.WithCancel`（不施加 `WithTimeout`）。`turnTimeout()`（默认 10min）仅作为 sync-cap 通知阈值，不作为硬截止。

```
1. dequeuePending 取出待执行消息
2. 迭代式循环（while loop + 深度计数器，替代原 goroutine 递归）：
   a. 加锁 admission 检查（HasActive → 重新入队并退出）
   b. context.WithCancel(loopCtx)（无超时；用户取消经 StoreCancelable 传播）
   c. 调用 runSingleAgentViaTRPC / teamsNative.RunTurn 执行
   d. 执行失败时：发布 error Envelope（含 pending_id）
   e. defer 中检查 inPendingLoop(ctx) 标志，避免递归 defer 重复触发
3. WS 前端收到 error 后显示通知
```

**关键设计**：
- **迭代式替代递归**（T1.3）：原 `processPendingQueue` 在 turn defer 中递归启动新 goroutine，深度无界。改为 `for {}` 循环 + `depth` 计数器（上限保护），同一 goroutine 内串行处理 pending 队列，避免 goroutine 链爆炸。
- **inPendingLoop context flag**（T1.3）：循环内通过 `contextWithPendingLoop(ctx)` 注入标志，turn defer 中的 `processPendingQueue` 调用检查 `inPendingLoop(ctx)` 抑制递归触发。
- **pending queue 持久化**（T1.4）：`PendingMessageQueue` 支持 snapshot 持久化（`NewPendingMessageQueueWithDirAndLogger`），进程重启后从磁盘恢复未消费的 pending 消息。

> **pending_id 约定**：待执行消息执行失败时，`error.pending_id` 应填充关联的待执行消息 ID。当前 `processPendingQueue` 中统一使用 `env.Error.PendingID = entry.ID`，metadata 双写已移除。

### 8.10 pendingQueue 容量控制与持久化

```
- maxPendingPerSession = 32
- enqueuePending 检查队列长度，超出时返回空 ID
- runNativeAgentTurn 中检测空 ID，返回 BadRequest 错误
- 持久化（T1.4）：PendingMessageQueue 支持 snapshot 到 dataDir，进程重启后恢复
```

### 8.11 上下文管理

- **上下文用量追踪**：每次 turn 后通过 `UpdateSessionContextFromLLMUsage` 更新 `context_used_tokens` / `context_used_ratio`（`prompt_tokens / context_window`）
- **Context Window 解析**：`llmcontext.ResolveWindow`（provider model `context_window_k` → session default → agent → 128000）；ChatOrchestrator 在 turn 结束与 `runner_completion` 投影时使用
- **L0 压缩**：当 `context_used_ratio` 超过阈值（默认 0.6）时，`SessionCompressor.AfterNativeTurn()` 异步触发摘要压缩；完成后 WS 推送带新 ratio 的 `system.session.compress` 通知
- **实时 UI**：`context_usage`（ReAct 子步）与 `runner_completion.usage`（含 `context_prompt_tokens`、`max_tokens`、`turn_total_tokens`）及压缩 notice 经 `sessionContextPatch` 乐观更新 Composer；Composer 副行合并展示 ctx/in/out/Σ/费用（与 Usage 大盘口径一致）
- **记忆服务**：通过 `runtimedeps.Runtime.SessionMemory` 注入 SQLite 适配器，由 trpc Runner 自动管理 L0-L4

### 8.12 用量记录

- 每次对话后通过 `recordTurnUsage()`（`chat_orchestrator_turn` defer）写入 `model_token_usage_events`；`recordChatIngressUsage` 仅 `CHAT_RECORD_USAGE_INGRESS=1` 时备用
- 支持流式/非流式两种记录路径
- 可通过 `CHAT_RECORD_USAGE_INGRESS=0` 禁用（用于双写过渡期）
- 记录字段包含：session_id、agent_key、team_id、model_api_id、input/output_tokens、latency_ms、tokens_per_second、stream_enabled、usage_kind、provider_code、prompt_mode

#### 8.12.1 SessionTurn 持久化

- 单 Agent turn 完成后，`recordSessionTurn()` 写入 `session_turns` 表，记录 turn 索引、角色、模型、token 用量和耗时
- Team turn 完成后，`recordTeamSessionTurn()` 写入 `session_turns` 表，与单 Agent 行为一致

#### 8.12.2 Team Run 持久化

- Team turn 执行时，`CreateTeamRun()` 写入 `team_runs` 表，记录 team_id、session_id、状态和起止时间
- Team turn 中每个成员 Agent 执行步骤通过 `CreateTeamRunStep()` 写入 `team_run_steps` 表
- Team 定义可配置 `intent_anchor_agent_id`（指定意图识别使用的成员 Agent）和 `TurnDeadlineDuration`（turn 超时时间）

#### 8.12.3 可观测性

- Chat turn 耗时通过 `arametrics.ChatTurnDuration` Prometheus 指标记录
- 意图识别超时为 45 秒
- Context Window 默认值：当 Agent 配置的 `context_window` ≤ 0 时，默认使用 128000 tokens

### 8.13 停止生成与运行中追加消息

- **`internal/runtime.RunRegistry`** 跟踪每 session 的 active run（`trpcrunner.Runner` 或 Team `context.CancelFunc` 或占位符）、pending 处理 cancel、run status
- `runSingleAgentViaTRPC`：`RunRegistry.StoreRunner`；defer `Finish` + `runner.Close` + `processPendingQueue`
- `RunTRPCUserTurn` 使用 `trpcagent.WithRequestID(sessionID)`，与 `ManagedRunner.Cancel(sessionID)` 对齐
- **停止**：HTTP `StopGeneration` 或 WS `cancel` → `RunRegistry.Cancel`（含 pending 后台 turn 的 cancel）
- **运行中追加（Follow-up Queue）**：HTTP `POST /v1/chat/enqueue`、`EnqueueUserMessage` RPC，或 WS `user_message` / `enqueue_message`；`SendChatMessage` 在 active run 时自动入队而非拒绝
- **Team cancel**：`RunRegistry.StoreCancelable` 登记 Team turn，与单 Agent 停止行为一致
- **连接管理**：`WSServer` 负责心跳、连接数限制、断线回放和 EventBus 订阅

#### 8.13.1 EventBuffer 回放

- WS 连接断线重连时，客户端携带 `last_event_id` 请求回放
- `EventBuffer` 为 ring buffer，容量 200 条/Session，基于事件 ID 匹配回放起始位置
- 回放期间依次发送 `replay_start` → 事件序列 → `replay_end` 控制消息
- **清理策略**：TTL 30min 自动过期 + 5min eviction ticker + `Close()` 优雅停止；`lastAcc` 追踪最后访问时间

#### 8.13.2 EventBus 订阅与背压

- `EventBus.Subscribe()` 支持 `SubscribeOptions`：SessionID / TeamID / Channel / FilterKey / EventTypes / LevelFilter
- `FilterKey` 采用前缀匹配规则（`MatchFilterKey`）
- 当订阅者消费速度落后时，EventBus 丢弃非关键事件；关键事件类型（不丢弃）包括：`tool_result`、`error`、`runner_completion`、`graph_node_end`、`team_run_finished`、`team_run_failed`
- WS 全局监控连接（`globalMode`，sessionId=`*`）可订阅所有 Session 的事件流

#### 8.13.3 Agent Settings Variables 注入

- Agent 配置中的 `variables_json` 字段存储自定义变量
- `runSingleAgentViaTRPC` 执行时通过 `ParseVariablesJSON` → `MergeRuntimeState` 将变量注入 Runner State
- 变量可在 System Prompt 中通过占位符引用

### 8.14 RunStatus 与 AwaitUserReply

- `GetRunStatus` 返回 `idle | pending | running | awaiting_user | completed | failed | cancelled | sync`
- `runStatuses sync.Map` 记录当前/最近一次 run 状态、run_id、错误信息和更新时间
- `makeAwaitReplyFunc` 注入 service await-reply tool，工具阻塞时将状态置为 `awaiting_user`
- `AwaitUserReply` 向 `awaitChans` 投递人工回复，恢复正在等待的 run
- 单 Agent 和 Team 路径均通过 `makeAwaitReplyFunc` 注入 AwaitHook；Team Runner 通过 `SetAwaitHookProvider` 注入，`runCtx` 注入 `serviceawaitreply.WithReplyFunc`
- 前端：`useEnvelopeStream` / `useChatStreamManager` / `useChatRunStatus` 消费 WS；`useChatWorkspace` 轮询 `GetRunStatus` 并在 `awaiting_user` 时展示提交回复横幅（`ChatMessagePanel` + `AwaitUserReply` RPC）
- 当前状态通过 `state_json` 持久化；`awaitChans` 仍为进程内结构，服务重启后 awaiting_user 状态可通过 `PendingAwaitUserReplyRoute` 恢复；`resumeInFlight` 防双 turn

### 8.15 Session 标题自动生成

- 首次对话时，`maybeAutoTitleFromUserMessage` 触发标题生成
- 先用截取方式快速设置标题（用户消息前 22 字符，即时反馈）
- 异步调用 `LLMSessionTitleGenerator` 生成高质量标题（15s 超时，失败静默）
- `LLMSessionTitleGenerator` 优先选择轻量模型（mini/flash/lite/small）

### 8.16 L0 上下文压缩

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

### 8.17 Session 标题自动生成实现

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

### 8.18 EventBuffer 设计

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

### 8.19 EventBus 订阅与背压

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

### 8.20 Channel/Cron 入口

#### 8.20.1 Channel Ingress

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

#### 8.20.2 Cron Turn

```
Cron Scheduler
  → ChatService.RunCronTurn()
    → 构造 SendChatMessageRequest（content 为 cron 触发内容）
    → 同一 runNativeAgentTurn() 主链路
```

- Cron turn 与手动对话共用 `runNativeAgentTurn`，受相同的 activeRuns/pendingQueue 保护

### 8.21 Team Run 持久化

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

### 8.22 SessionTurn 持久化

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

### 8.23 Agent Settings Variables 注入

```go
func ParseVariablesJSON(variablesJSON string) (map[string]interface{}, error)
func MergeRuntimeState(state map[string]interface{}, variables map[string]interface{}) map[string]interface{}
```

- Agent 配置中的 `variables_json` 字段存储自定义变量
- `runSingleAgentViaTRPC` 执行时通过 `ParseVariablesJSON` → `MergeRuntimeState` 将变量注入 Runner State
- 变量可在 System Prompt 中通过占位符引用（如 `{{variable_name}}`）

### 8.24 可观测性

- Chat turn 耗时通过 `arametrics.ChatTurnDuration` Prometheus 指标记录
- 意图识别超时为 45 秒
- Context Window 默认值：当 Agent 配置的 `context_window` ≤ 0 时，默认使用 128000 tokens

---

## 九、运行时层

### 9.1 Agent 构建

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

### 9.2 Runner 构建

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

### 9.3 Team 编排

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

## 十、Wire 注入

### 10.1 ProviderSet

```go
var ProviderSet = wire.NewSet(
    NewChatService,
    provideChatServiceDeps,
    NewSessionCompressor,
    NewLLMSessionTitleGenerator,
    // ... 其他 Service
)
```

### 10.2 provideChatServiceDeps

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

## 十一、Web 前端设计

### 11.1 文件结构

```
web/src/
├── services/index.ts              ← createChatService 导出
├── features/chat/
│   ├── api.ts                     ← Chat API 调用封装（sendMessage/stop/listOptions/getPending/cancelPending/updatePending/getRunStatus/awaitUserReply/enqueue/listJobs/cancelJob/submitFeedback/confirmActivity）
│   ├── types.ts                   ← TypeScript 类型定义
│   ├── envelope.ts                ← WS Envelope 类型定义
│   ├── dispatcher.ts              ← Envelope 分发器
│   ├── conversationEventDispatcher.ts ← 对话事件分发器
│   ├── eventFilter.ts             ← 事件过滤
│   ├── useEventFilter.ts          ← 事件过滤 composable
│   ├── useEnvelopeStream.ts       ← WS Envelope 底层消费
│   ├── ws-transport.ts            ← WS 连接管理（WsTransport：连接/重连/心跳/断线回放；T1.8: 无限重连 + pendingQueue FIFO 保护 100 上限）
│   ├── globalWsHub.ts             ← 全局 WS Hub
│   ├── streamHandlers.ts          ← WS 事件处理器（withSessionFilter + onRunActivity + errorCodeHints）
│   ├── streamEventTypes.ts        ← 流事件类型
│   ├── streamContentPatch.ts      ← 流内容补丁
│   ├── envelopeToolCall.ts        ← Envelope → ToolUseEvent v2 映射 + upsert
│   ├── envelopeRunStatus.ts       ← Envelope → RunStatus 映射
│   ├── sessionRunStatus.ts        ← Session RunStatus
│   ├── activityPresentation.ts    ← activity_kind → icon/label/summary 前端 fallback
│   ├── activityTypes.ts           ← Activity 类型
│   ├── activityMessageAdapter.ts  ← Activity 消息适配器
│   ├── activityTimelineTypes.ts   ← Activity 时间线类型
│   ├── executionProgress.ts       ← 执行进度
│   ├── executionCardHelpers.ts    ← 执行卡片辅助函数
│   ├── toolEventMarkdown.ts       ← 工具事件 Markdown 渲染 + toolEventToMessage 共享转换函数
│   ├── errorCodeHints.ts          ← 错误码 → 用户可读提示映射
│   ├── chatMessageMarkdown.ts     ← Chat 消息 Markdown
│   ├── sessionContextPatch.ts     ← Session 上下文补丁
│   ├── contextBreakdown.ts        ← 上下文分解
│   ├── composerUsageMetrics.ts    ← Composer 用量指标
│   ├── messageSourceMeta.ts       ← 消息来源元数据
│   ├── messageAttachments.ts      ← 消息附件
│   ├── messageOrigin.ts           ← 消息来源
│   ├── messageStoreBatch.ts       ← 消息 Store 批处理
│   ├── mergeSessionMessages.ts    ← 合并 Session 消息
│   ├── parseMessageOptions.ts     ← 解析消息选项
│   ├── modelCapabilities.ts       ← 模型能力
│   ├── todoPresentation.ts        ← Todo 展示
│   ├── diffEditHelpers.ts         ← diff 编辑辅助
│   ├── awaitConstants.ts          ← Await 常量
│   ├── chatFocusCoordinator.ts    ← Chat 焦点协调
│   ├── useChatScrollTitle.ts      ← Chat 滚动标题
│   ├── useTaskDeadLetters.ts      ← Task 死信
│   ├── useChatBackgroundJobs.ts   ← Chat 后台任务
│   ├── jobFormatters.ts           ← Job 格式化
│   ├── inboundSyncRouting.ts      ← 入站同步路由
│   ├── inboundSyncEnvelope.ts     ← 入站同步 Envelope
│   ├── channelInboundSession.ts   ← Channel 入站 Session
│   ├── channelInboundSessionRefresh.ts ← Channel 入站 Session 刷新
│   ├── channelFocusLoad.ts        ← Channel 焦点加载
│   ├── channelSessionMeta.ts      ← Channel Session 元数据
│   ├── channelWsCursor.ts         ← Channel WS 游标
│   ├── agentPlannerSettings.ts    ← Agent Planner 设置
│   ├── agentTreeTypes.ts          ← Agent 树类型
│   ├── agentTreeUtils.ts          ← Agent 树工具
│   ├── a2uiParse.ts               ← A2UI 解析
│   ├── a2uiBind.ts                ← A2UI 绑定
│   ├── a2uiChildren.ts            ← A2UI 子组件
│   ├── a2uiSurfaceState.ts        ← A2UI Surface 状态
│   ├── a2uiUserAction.ts          ← A2UI 用户动作
│   ├── a2uiUserActionDisplay.ts   ← A2UI 用户动作展示
│   └── composables/
│       ├── useChatWorkspace.ts    ← 对话工作区 composable（状态管理 + 交互逻辑；拆分后聚焦编排）
│       ├── useChatSender.ts       ← 发送策略模式（Agent/Team 统一发送路径 + enqueue_message；T1.5: 移除超时失败标记；T1.7: HTTP fallback loadMessages 失败隔离）
│       ├── useChatStreamManager.ts ← WS 事件流管理（单 Agent + Team 统一；T1.6: onActivityEnvelope 实时 AF 接入）
│       ├── useChatRunStatus.ts    ← RunStatus 轮询 + AwaitUserReply 回复提交
│       ├── useFollowUpQueue.ts    ← Follow-up Queue 状态管理（pending 列表 + WS 刷新）
│       ├── useAwaitReply.ts       ← AwaitUserReply 交互（提交回复 + 横幅状态）
│       ├── useChatDialogs.ts      ← dialog 状态聚合 composable
│       ├── useChatComposerActions.ts ← composer action 方法 composable
│       ├── useChatProviderOptions.ts ← Provider/Model 选项加载（Store 调用）
│       ├── useChatEntityNav.ts    ← Chat 实体导航
│       ├── useChatEntityCollapse.ts ← Chat 实体折叠
│       ├── useChatDeleteFlow.ts   ← Chat 删除流程
│       ├── useChatInboundSync.ts  ← Chat 入站同步
│       ├── useChatAttachments.ts  ← Chat 附件
│       ├── useChatTraceAndArtifacts.ts ← Chat 追踪与 Artifact
│       ├── useChatSettingsDialog.ts ← Chat 设置弹框
│       ├── useChatEventInspector.ts ← Chat 事件检查器
│       ├── useChatMessageScroll.ts ← Chat 消息滚动
│       ├── useConversationTimeline.ts ← 对话时间线
│       ├── useActivityTimeline.ts ← Activity 时间线
│       ├── useTodoBoard.ts        ← Todo 看板
│       ├── useStatusPulse.ts      ← 状态脉冲
│       ├── useContextualLoadingMessage.ts ← 上下文加载消息
│       ├── useContextBreakdown.ts ← 上下文分解
│       ├── useReasoningSidebar.ts ← Reasoning 侧栏
│       ├── useChatSidebarOrder.ts ← Chat 侧栏顺序
│       ├── useEventFilter.ts      ← 事件过滤
│       ├── chatWorkspaceUtils.ts  ← Chat 工作区工具
│       └── todoColumnFingerprint.ts ← Todo 列指纹
├── components/chat/
│   ├── ChatWorkspaceShell.vue     ← 工作区外壳（标题 + 三栏布局容器）
│   ├── ChatEntitySidebar.vue      ← 左侧 Agent/Team 列表
│   ├── ChatEntityGroup.vue        ← Agent/Team 分组
│   ├── ChatEntityItem.vue         ← Agent/Team 条目
│   ├── ChatSessionSidebar.vue     ← 右侧 Session 历史
│   ├── ChatMessagePanel.vue       ← 中间对话内容 + 输入区域（Container: approved）
│   ├── ChatMessageList.vue        ← 消息列表
│   ├── ConversationTurn.vue       ← 一轮对话容器（User → ToolStrip → Assistant）
│   ├── ChatReasoningPeek.vue      ← Reasoning 折叠展示（live tail + 展开）
│   ├── ChatReasoningDrawer.vue    ← Reasoning 抽屉
│   ├── ChatExecutionCard.vue      ← 执行过程卡片（原 ChatToolCallCard；默认折叠 + v2 元数据）
│   ├── ChatEnqueueMessage.vue     ← 待执行消息条目
│   ├── ChatPendingQueue.vue       ← 待执行队列
│   ├── ChatRunnerStatus.vue       ← 运行状态指示
│   ├── ChatComposer.vue           ← 输入框组件（emit 事件，不直接调 API/Store）
│   ├── ChatBackgroundJobsPanel.vue ← 后台任务面板（emit 事件，不直接调 API）
│   ├── ChatMessageAttachments.vue ← 附件展示（emit 事件，不直接调 Store）
│   ├── ChatSideToggle.vue         ← 侧栏折叠按钮
│   ├── ChatSettingsDialog.vue     ← Agent/Team 设置弹框
│   ├── ChatDeleteDialog.vue       ← 删除确认弹框
│   ├── SessionTimelineDialog.vue  ← Session 历史追踪弹框
│   ├── ChatMentionPopup.vue       ← @ 引用弹窗
│   ├── ChatDiffViewer.vue         ← Diff 查看器
│   ├── ChatHeaderUsagePanel.vue   ← 头部用量面板
│   ├── ChatHeaderPromptBar.vue    ← 头部 Prompt 栏
│   ├── ChatContextBreakdownPopover.vue ← 上下文分解弹窗
│   ├── ChatSectionHeader.vue      ← 区段标题
│   ├── ChatSessionArtifactsPanel.vue ← Session Artifact 面板
│   ├── ChatSkillHintBar.vue       ← Skill 提示栏
│   ├── ChatSkillCatalogStrip.vue  ← Skill Catalog 条
│   ├── ChatTeamMemberStrip.vue    ← Team 成员条
│   ├── ChatA2UIPreview.vue        ← A2UI 预览
│   ├── ChatA2UISurface.vue        ← A2UI Surface
│   ├── EventStream.vue            ← 事件流
│   ├── EventFilterBar.vue         ← 事件过滤栏
│   ├── SessionEventInspectorPanel.vue ← Session 事件检查器面板
│   ├── BranchTree.vue             ← 分支树
│   ├── BranchTreeNode.vue         ← 分支树节点
│   ├── StateDeltaIndicator.vue    ← State Delta 指示器
│   ├── TransferBadge.vue          ← 转交徽章
│   ├── UiConfigToggle.vue         ← UI 配置切换
│   ├── CodeBlock.vue              ← 代码块
│   ├── ThinkingBlock.vue          ← 思考块
│   ├── ReplyBlock.vue             ← 回复块
│   ├── PlanBlock.vue              ← 计划块
│   ├── NoticeBlock.vue            ← 通知块
│   ├── ConfirmBlock.vue           ← 确认块
│   ├── ActionBlock.vue            ← 动作块
│   ├── UserMessageBubble.vue      ← 用户消息气泡
│   ├── ErrorBlock.vue             ← 错误块
│   ├── AgentWorkPanel.vue         ← Agent 工作面板
│   ├── TeamPanel.vue              ← Team 面板
│   ├── TeamProgressSection.vue    ← Team 进度区段
│   ├── DelegateActivity.vue       ← 委托 Activity
│   ├── DagSection.vue             ← DAG 区段
│   ├── TaskBoardSection.vue       ← 任务看板区段
│   ├── TodoBoardTabs.vue          ← Todo 看板标签
│   ├── TodoColumn.vue             ← Todo 列
│   ├── TodoKanbanBoard.vue        ← Todo 看板
│   ├── TodoCard.vue               ← Todo 卡片
│   ├── TodoInlineList.vue         ← Todo 内联列表
│   └── A2UIComponentNode.vue      ← A2UI 组件节点
├── config/chatOptions.ts          ← 对话模式/模型配置
├── stores/chat/
│   ├── index.ts                   ← Chat Store re-export（facade 已移除）
│   ├── messageStore.ts            ← 消息 Store（sessionId 必传，无 sessionStore 硬依赖）
│   ├── runtimeStore.ts            ← 运行时 Store（含 submitFeedback/cancelBackgroundJob/listChatOptions）
│   ├── sessionStore.ts            ← Session Store
│   └── conversationStore.ts       ← 对话 Store
├── stores/app.ts                  ← 全局状态（含 Agent/Session 选择）
```

### 11.1.1 前端事件流架构

```
WsTransport（WS 连接管理：连接/重连/心跳/last_event_id 回放）
  ↓ Envelope 流
EnvelopeDispatcher（按 type 分发：text_delta/tool_call/error/state_delta/...）
  ↓ 分流
useChatStreamManager（统一管理单 Agent + Team 事件流）
useChatRunStatus（RunStatus 轮询 + isAwaiting + submitReply）
  ↓ 状态聚合
useChatWorkspace（对话工作区：消息列表/pending/发送/停止/上下文比）
  ↓ 渲染
ChatMessagePanel（消息气泡/工具事件/待执行列表/输入框）
```

### 11.2 页面布局

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

### 11.3 TypeScript 类型定义

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
  knowledge_bases?: string[];
};

export type SendMessageResult = {
  user_message: Message;
  agent_message: Message;
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

> **⚠️ 废弃提示**：历史 `SendMessageStreamCallbacks` 类型来自 SSE API，当前 Chat 页面主路径使用 `useEnvelopeStream` / `useChatStreamManager` 消费 WS Envelope。该类型仅作为向后兼容保留，后续应删除或迁移残留 SSE callback 类型，避免误导新开发者。

### 11.4 API 调用

```typescript
export async function sendMessage(payload: {...}): Promise<SendMessageResult>
export async function listChatOptions(type?: string): Promise<ChatOption[]>
export async function stopGeneration(sessionId: string): Promise<boolean>
export async function getPendingMessages(sessionId: string): Promise<PendingMessage[]>
export async function cancelPendingMessage(sessionId: string, pendingId: string): Promise<boolean>
export async function updatePendingMessage(sessionId: string, pendingId: string, content: string): Promise<boolean>
export async function interruptAndSendMessage(sessionId: string, pendingId: string): Promise<boolean>
export async function getRunStatus(sessionId: string): Promise<RunStatus>
export async function awaitUserReply(sessionId: string, reply: string, runId?: string): Promise<boolean>
export async function enqueueUserMessage(sessionId: string, content: string): Promise<EnqueueUserMessageResponse>
export async function listChatBackgroundJobs(filter?: {...}): Promise<ChatBackgroundJob[]>
export async function cancelChatBackgroundJob(id: string, source?: string): Promise<boolean>
export async function submitMessageFeedback(messageId: string, sessionId: string, rating: string, comment?: string): Promise<boolean>
export async function confirmActivity(sessionId: string, activityId: string, approved: boolean): Promise<ConfirmActivityResponse>
```

> **⚠️ 废弃提示**：历史 `sendMessageStream()` 函数基于 SSE 实现，当前 Chat 页面主路径使用 WS `useEnvelopeStream` 消费实时事件。该函数不应在新代码中使用。

### 11.5 组件设计

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
| `favoriteIds` | `string[]` | 收藏 ID 列表 |
| `isDark` | `boolean` | 暗黑模式 |

| Emit | 载荷 | 说明 |
|------|------|------|
| `select` | `string` | 选中 Session |
| `new-session` | — | 新建 Session |
| `rename` | `{id, title}` | 重命名 |
| `delete` | `kind, id` | 删除 |
| `trace` | `string` | 历史追踪 |
| `toggle-favorite` | `string` | 切换收藏 |

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
| `download-artifact` | `meta` | 下载附件 |
| `cancel-job` | `{id, source}` | 取消后台任务 |
| `paste-unsupported` | — | 粘贴不支持提示 |
| `retry` | `messageId` | 重试失败消息 |
| `dismiss-failed` | `messageId` | 忽略失败消息 |
| `regenerate` | `messageId` | 重新生成 |

### 11.6 UX 规范

- 玻璃材质：`background: var(--glass-surface); backdrop-filter: blur(var(--glass-blur-default))`
- 日间主操作色：`var(--color-accent)` = `#E9A23B`
- 夜间霓虹强调：`var(--color-neon-cyan)` = `#00E5FF`
- 输入框圆角：12-16px
- 消息气泡圆角：16-20px
- 暗黑模式可读性：聊天记录正文、代码块、工具结果、时间戳等文本必须保证对比度
- 上下文进度颜色阈值：`<0.6` 绿 / `0.6-0.8` 黄 / `>0.8` 红
- 成功色 fallback：`#4CAF7C`（不使用 `#4caf50`）

---

## 子模块：Chat 执行过程卡片 — 技术设计

> **版本**：2026-05-20
> **对应需求**：[1 chat-execution-trace.md](./1%20chat-execution-trace.md)
> **遵循**：[AI-DEVELOPMENT-SPECIFICATION.md](../guides/AI-DEVELOPMENT-SPECIFICATION.md) · [AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md) · [frontend-guide.md](../guides/frontend-guide.md) · `aranea-frontend-guide` SKILL §6
> **关联**：[1 chat.design.md](./1%20chat.design.md) · [52-flow-logger.design.md](./52-flow-logger.design.md) · [23 tools.design.md](./23%20tools.design.md)

---

## 1. 设计原则

1. **复用 WS Envelope 主通道**：不新增 HTTP 轮询；实时与回放均走 `/v1/ws` + EventBuffer（与 [1 chat.design.md §8.5](#85-websocket-envelope-事件协议) 一致）。
2. **框架边界不变**：`internal/biz` 不 import `pkg/trpc-agent-go`；投影在 `internal/agent/event_projector.go`，组装在 `internal/service/chat_orchestrator_turn*.go`。
3. **与 Monitor 分流**：`flow_log` → Monitor；`tool_call` / `tool_result`（增强元数据）→ Chat。`TraceEmitter.ObserveFrameworkEvent` 继续写 Span 供 Usage / Traces，**不**把 FlowLog 正文塞进 Chat。
4. **一张调用一张卡片**：以 `tool_call_id`（框架 ToolCall.ID）为 upsert 键；`tool_call` 创建 running 态，`tool_result` 合并完成态。
5. **默认折叠、按需展开**：UI 层约束；协议层携带完整 `arguments_json` / `result_json`。
6. **向后兼容**：在现有 `EnvelopeToolCall` 上**扩展 optional 字段**；旧客户端忽略新字段仍可显示名称 + 状态。

---

## 2. 现状与差距

| 能力 | 现状 | 差距 |
|------|------|------|
| WS 事件 | `tool_call` / `tool_result` 已投影 | `tool_result` 缺少稳定 `id/name` 时 upsert 失败；无 `activity_kind` / 展示名 |
| 前端 | `ChatExecutionCard.vue` + `upsertToolMessage` | 参数区 **默认展开**（`details open`）；Skill/MCP 无专用图标与摘要 |
| 持久化 | `options_json.tool_event` 写入 Message | 无独立 `message_kind`；历史加载依赖 markdown 旁路 |
| Skill / MCP | 走统一 ToolCall 名 | 需映射 `display_label`、`icon_key`、摘要行 |
| Team | `author` 在 Envelope | 卡片未统一展示成员 |
| Monitor | FlowLog 完整 | 与 Chat 用户视图需隔离（已违反时会 toast flow 错误） |

本设计在**不新增 EnvelopeType** 的前提下，完成 v2 元数据扩展 + 前端卡片规范化（P0）；可选 P1 引入 `activity` 别名类型见 §5.3。

---

## 3. 架构总览

```text
trpc-agent-go Runner
  → framework Event (ToolCall / ToolResponse)
       │
       ▼
internal/agent/event_projector.go
  ├─ projectChatCompletionChunk → tool_call Envelope (status=calling→running)
  ├─ buildToolResultEnvelope    → tool_result Envelope (id/name/duration/status)
  └─ enrichActivityMeta()       ← 新增：kind/label/icon/summary/脱敏
       │
       ▼
internal/event.EventBus → internal/server/ws.go → 前端 WS
       │
       ├─ useChatWorkspace: upsertToolMessage(tool_call|tool_result)
       └─ ChatMessagePanel: ChatExecutionCard (默认折叠)

并行（不进入 Chat UI）：
internal/event/trace_emitter.go → flow_log (monitor) + spans → usage.metadata_json
```

```mermaid
sequenceDiagram
  participant LLM as tRPC Agent
  participant Proj as EventProjector
  participant Bus as EventBus
  participant WS as WSServer
  participant UI as ChatExecutionCard

  LLM->>Proj: ToolCall id=tc_1 name=skill_run
  Proj->>Bus: tool_call running + activity meta
  Bus->>WS: Envelope
  WS->>UI: upsert card running

  LLM->>Proj: ToolResponse tool_id=tc_1
  Proj->>Bus: tool_result success duration=1200ms
  Bus->>WS: Envelope
  WS->>UI: merge card success + 1.2s
```

---

## 4. 数据模型

### 4.1 ActivityKind 枚举

| `activity_kind` | 判定规则（优先级从高到低） |
|-----------------|---------------------------|
| `skill` | `name` ∈ `skill_load`,`skill_run`,`skill_search`,`use_skill` 或前缀 `skill_` |
| `mcp` | `name` ∈ `mcp_call`,`mcp_list_tools`,`mcp_list_servers`,`mcp_inspect_tools` 或 MCP ToolSet 前缀 `mcp:` |
| `subagent` | `transfer_to_agent`,`spawn_subagent`,`call_agent` |
| `memory` | `load_memory`,`preload_memory`,`memory_*`,`working_memory.*` |
| `knowledge` | `knowledge_search` |
| `session` | `await_user_reply` |
| `tool` | 默认 |

实现位置：`internal/agent/activity_meta.go`（纯函数，可单测）。

### 4.2 EnvelopeToolCall v2 扩展字段

在 [`internal/event/envelope.go`](../../internal/event/envelope.go) 的 `EnvelopeToolCall` 增加 **optional JSON 字段**（向后兼容）：

```go
type EnvelopeToolCall struct {
    ID            string `json:"id"`
    Name          string `json:"name"`
    ArgumentsJSON string `json:"arguments_json"`
    ResultJSON    string `json:"result_json,omitempty"`
    Status        string `json:"status"`
    DurationMS    int64  `json:"duration_ms,omitempty"`
    IsLongRunning bool   `json:"is_long_running,omitempty"`

    // --- v2 Chat execution trace ---
    ActivityKind  string `json:"activity_kind,omitempty"`  // skill|mcp|tool|...
    DisplayLabel  string `json:"display_label,omitempty"`  // 卡片标题
    IconKey       string `json:"icon_key,omitempty"`       // Quasar icon name
    Summary       string `json:"summary,omitempty"`        // 折叠态副标题
    StartedAt     string `json:"started_at,omitempty"`     // RFC3339
    FinishedAt    string `json:"finished_at,omitempty"`
    ErrorCode     string `json:"error_code,omitempty"`
    AgentKey      string `json:"agent_key,omitempty"`      // Team 成员
    AgentName     string `json:"agent_name,omitempty"`
    RunID         string `json:"run_id,omitempty"`
    TraceID       string `json:"trace_id,omitempty"`
}
```

`tool_call` 与 `tool_result` **均携带相同 `id`**；result 侧填充 `duration_ms`、`finished_at`、`result_json`。

### 4.3 前端 ToolUseEvent v2

扩展 [`web/src/features/chat/types.ts`](../../web/src/features/chat/types.ts) 中 `ToolUseEvent`：

```typescript
export type ActivityKind = "tool" | "skill" | "mcp" | "subagent" | "memory" | "knowledge" | "session";

export type ToolUseEvent = {
  // ...现有字段...
  activity_kind?: ActivityKind;
  display_label?: string;
  icon_key?: string;
  summary?: string;
  started_at?: string;
  finished_at?: string;
  error_code?: string;
  run_id?: string;
  trace_id?: string;
  /** UI-only: 用户是否展开详情，不持久化到后端 */
  expanded?: boolean;
};
```

映射函数：[`envelopeToolCall.ts`](../../web/src/features/chat/envelopeToolCall.ts) 的 `envelopeToToolEvent` 读取 v2 字段。

### 4.4 持久化（messages 表）

**策略**：沿用 `messages` 行 + `options_json` 嵌入结构化事件（与现网 `tool_event` 一致），不新增表。

| 字段 | 值 |
|------|-----|
| `role` | `assistant` |
| `status` | `tool_running` / `tool_success` / `tool_failed` |
| `content_markdown` | 简短 fallback 文本（供搜索 / 纯文本客户端） |
| `options_json` | `{ "schema": "chat.activity/v1", "tool_event": { ...ToolUseEvent } }` |
| `latency_ms` | 完成后写入 `duration_ms` |

**稳定主键**：`message.id = "act-" + tool_call_id`（取代当前 `tool-{agent}-{name}` 组合，避免同名工具重复）。

**落库时机**（Service 层，非 biz）：

1. `tool_result` 投影后、WS 发布前：异步写入或随 turn 结束批量写入（与 assistant 消息同一事务，见 [1 chat.design.md §8.7](#87-runsingleagentviatrpc-核心流程)）。
2. `running` 态可选择**仅 WS 不落库**，刷新后只见已完成卡片（P0 可接受）；P1 增加 `tool_running` 行软删除或 turn 结束清理。

---

## 5. Envelope 协议

### 5.1 事件类型（不变）

| type | 触发 | status |
|------|------|--------|
| `tool_call` | LLM 发起 function call | `calling` → 前端归一化为 `running` |
| `tool_result` | 工具返回 | `success` / `failed` |

### 5.2 Metadata 双写（可选）

`Envelope.Metadata` 冗余常用字段便于过滤 / 日志：

```json
{
  "activity_kind": "skill",
  "display_label": "skill_run",
  "run_id": "run_...",
  "trace_id": "tr_..."
}
```

### 5.3 备选：EnvelopeType `activity`（P2，非 P0）

若未来 tool_call/tool_result 语义过载，可新增：

- `activity_start` / `activity_update` / `activity_end`

P0 **不采用**，避免前后端与 EventBuffer 大规模迁移。

---

## 6. 展示映射

### 6.1 图标（Quasar `icon`）

| activity_kind | icon | 备注 |
|---------------|------|------|
| `tool` | `build` | 通用工具 |
| `skill` | `auto_awesome` | Skill |
| `mcp` | `hub` | MCP |
| `subagent` | `group` | 子 Agent |
| `memory` | `psychology` | 记忆 |
| `knowledge` | `menu_book` | 知识库 |
| `session` | `forum` | 等待用户 |

按 `name` 细化（可选覆盖）：

| name | icon |
|------|------|
| `read_file` / `save_file` | `description` |
| `exec_command` / `workspace_exec` | `terminal` |
| `skill_run` | `play_circle` |
| `skill_load` | `download` |

### 6.2 DisplayLabel 解析

```
1. tools 表 display_name（ToolUC.GetTool，按 catalog key 或 runtime alias 反查）
2. stepTitleRegistry 风格内置 map（skill_run →「运行 Skill」）
3. runtime name 原样
```

实现：`internal/agent/activity_meta_resolver.go` + 注入 `ToolUC` / 内存 registry。

### 6.3 Summary 一行摘要

| kind | 规则 |
|------|------|
| file tools | `` `path` `` from arguments.path |
| shell | command 前 80 字符 |
| skill | arguments.skill / skill_name |
| mcp | `server_key` + `/` + `tool_name` |
| knowledge | collection_id + query 前 40 字符 |

---

## 7. 后端实现要点

### 7.1 EventProjector

文件：[`internal/agent/event_projector.go`](../../internal/agent/event_projector.go)

| 函数 | 变更 |
|------|------|
| `projectChatCompletionChunk` | `tool_call` 填充 `StartedAt`、`ActivityKind`、`DisplayLabel`、`Summary`；`Status=calling` |
| `buildToolResultEnvelope` | 必须带 `ToolID`、`ToolName`、`DurationMS`；失败读 `Response.Error` |
| `enrichActivityMeta`（新） | 集中 kind/label/icon/summary + 脱敏 |

**脱敏**：复用 [`internal/service/tool.go`](../../internal/service/tool.go) 或 `biz` 层既有 `sanitize` 规则；对 `arguments_json` / `result_json` 中 key 名含 `password|secret|token|api_key` 的值替换为 `***`。

### 7.2 与 TraceEmitter 协作

- `ObserveFrameworkEvent` 已记录 `tool.call` span；扩展 attrs：`tool_name`、`activity_kind`、`status`。
- **禁止** `LogError(chat.usage_record|system.agent.tool_build)` 推 Chat error toast（见 Monitor 分流约定）。

### 7.3 依赖注入

`EventProjector` 增加可选 `ActivityMetaResolver` 接口（由 `service` 注入 `ToolUC` + `AgentUC`）：

```go
type ActivityMetaResolver interface {
    Resolve(ctx context.Context, agentID, toolName string, argsJSON []byte) ActivityPresentation
}
```

避免 `agent` 包 import `data`。

### 7.4 Team 路径

- Envelope.`Author` = 成员 agent_key；`AgentName` 由 `ActivityMetaResolver` 查 agents 表。
- Team 成员气泡与执行卡片**并存**：卡片插在成员子流或统一 session 时间线（与现 `member_delta` 顺序约定：工具卡片紧随该成员 turn 内顺序）。

---

## 8. 前端实现要点

> 遵循 [frontend-guide.md](../guides/frontend-guide.md)：**展示组件不直连 API**；状态由 `useChatWorkspace` + `upsertToolMessage` 维护。

### 8.1 组件结构

```
web/src/components/chat/
├── ChatMessagePanel.vue          ← 消息列表；识别 tool_* status
├── ChatExecutionCard.vue         ← 重命名自 ChatToolCallCard（或别名导出）
└── ChatExecutionCardDetails.vue  ← 参数/结果分区（可选拆分）

web/src/features/chat/
├── envelopeToolCall.ts           ← upsert + v2 映射
├── activityPresentation.ts       ← icon/label/summary 前端 fallback
└── types.ts                      ← ToolUseEvent v2
```

### 8.2 折叠交互

- 使用 **Quasar `q-expansion-item`** 或 native `<details>` **不带 `open` 属性**（默认折叠）。
- `expanded` 状态仅存组件本地 `ref` 或 session 级 Map（**不**写回后端）。
- Header 整行可点击；`:aria-expanded` 绑定。

### 8.3 视觉（UX.md）

| Token | 用途 |
|-------|------|
| `--glass-surface` / `--glass-border` | 卡片背景与边 |
| `--color-success` / `--color-danger` / `--color-warning` | 状态 badge |
| `chat-tool-card--running` | 左边框 accent 动画（现有 class 延续） |

禁止硬编码青紫霓虹；日夜模式跟随 `body.body--dark`。

### 8.4 消息流插入规则

[`useChatWorkspace.ts`](../../web/src/features/chat/composables/useChatWorkspace.ts)：

```typescript
chatStream.onType("tool_call", (env) => { upsertToolMessage(..., "before"); });
chatStream.onType("tool_result", (env) => { upsertToolMessage(..., "after"); });
```

`upsertToolMessage` 改用 **`act-${tc.id}`**  message id；phase `after` 合并 result / duration / status。

### 8.5 历史加载

`GET /v1/sessions/{id}/messages` 返回的 `options_json.schema === "chat.activity/v1"` 时，**直接渲染 `ChatExecutionCard`**，不再解析 markdown 主文案。

---

## 9. 状态机

```mermaid
stateDiagram-v2
  [*] --> running: tool_call
  running --> success: tool_result ok
  running --> failed: tool_result error
  running --> blocked: TOOL_CONFIRMATION_REQUIRED
  blocked --> running: user approved
  blocked --> failed: user denied
  running --> cancelled: StopGeneration
  success --> [*]
  failed --> [*]
  cancelled --> [*]
```

| 转换 | WS 事件 |
|------|---------|
| → running | `tool_call` |
| → success/failed | `tool_result` |
| → blocked | `tool_result` status=blocked 或 `error` type=tool_confirmation |
| → cancelled | `run_status` cancelled + 卡片强制 failed/cancelled |

---

## 10. 安全与合规

| 项 | 措施 |
|----|------|
| 参数脱敏 | 后端投影前扫描 JSON key；前端二次 mask 仅 display |
| 大 payload | `result_json` WS 上限 256KB；超出截断 + `truncated: true` |
| MCP 凭据 | 永不进入 `summary`；详情默认折叠 |
| 审计 | 完整 arguments 仍可通过 Monitor Traces / tool_invocations 表排查（见 [23 tools.design.md](./23%20tools.design.md)） |

---

## 11. 实施分期

| 阶段 | 内容 | 文件触点 | 状态 |
|------|------|----------|------|
| **P0** | EnvelopeToolCall v2 + Projector 填 id/name/duration/kind/label | `envelope.go`, `event_projector.go`, `activity_meta.go` | ✅ |
| **P0** | 前端默认折叠卡片 + stable upsert id | `ChatExecutionCard.vue`, `envelopeToolCall.ts` | ✅ |
| **P0** | 脱敏 + summary | `activity_meta.go` | ✅ |
| **P1** | messages 持久化 schema `chat.activity/v1` + 历史还原 | `activity_persist.go`, `session_repo.go` | ✅ |
| **P1** | catalog display_name 查表 | `ActivityMetaResolver` + ToolUC | ✅ |
| **P1** | Team 成员标识 | `envelope` author + UI | ✅ |
| **P2** | running 态落库 / 取消态 | session messages | ✅ running upsert + StopGeneration 取消落库 |
| **P2** | `activity_*` Envelope 类型评估 | 仅当 tool_call 过载时 | 📋 暂不需要 |

---

## 12. 测试计划

| 层 | 用例 |
|----|------|
| Go 单测 | `ClassifyActivityKind`、`BuildSummary`、脱敏、`buildToolResultEnvelope` 带 ToolID |
| Go 集成 | 模拟 tool_call → tool_result WS 序列，EventBuffer 回放 idempotent |
| 前端 vitest | `envelopeToToolEvent` v2 字段、`upsertToolMessage` 同 id 合并 |
| E2E | 发消息 → 见 running 卡片 → 完成见耗时；默认折叠；展开见 JSON |

---

## 13. 文档与索引更新（实现后）

| 文档 | 动作 |
|------|------|
| [1 chat.md](./1%20chat.md) | §验收 增加执行卡片条目 |
| [1 chat.design.md](./1%20chat.design.md) | §8.5 补充 EnvelopeToolCall v2；§前端 替换 ChatToolCallCard 说明 |
| [1-chat-development.md](./1-chat-development.md) | 新增迭代任务与勾选 |
| [frontend-pages.md](./frontend-pages.md) | Chat 页能力描述 |
| [52-flow-logger.design.md §5.1](./52-flow-logger.design.md) | 可选注册 `chat.activity.project` 步骤（非必须） |

---

## 14. 与现有 ChatToolCallCard 的差异摘要

| 项 | 现有 | 目标 |
|----|------|------|
| 默认折叠 | 参数 `details open` | 全部默认折叠 |
| 覆盖范围 | 通用 tool | + Skill / MCP 图标与摘要 |
| upsert 键 | 易冲突 | `act-{tool_call_id}` |
| 持久化 schema | 隐式 `tool_event` | 明示 `chat.activity/v1` |
| 执行中文案 | 英文 status 原样 | i18n「正在执行」 |

本设计 **不修改** `pkg/trpc-agent-go`；所有增强均在 Aranea 投影层与前端完成，符合 [AGENT_RUNTIME_BOUNDARY.md](../AGENT_RUNTIME_BOUNDARY.md)。
