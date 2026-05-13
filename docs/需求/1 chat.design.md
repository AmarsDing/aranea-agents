# Chat 对话模块 — 实现设计文档

> 对应需求：`1 chat.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Chat 是用户与 Agent/Team 交互的核心入口，负责 SSE 流式对话、上下文管理、用量记录。当前已实现完整功能，本设计文档记录现有实现架构并标注优化方向。

---

## 二、Proto 层

### 2.1 完整 Proto 定义

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
}
```

### 2.2 SSE 流式端点（非 Proto 定义，HTTP Server 层注册）

SSE 流式对话通过 HTTP Server 层手动注册路由，不在 Proto 中定义：

```
POST /v1/chat/messages/stream  →  ChatService.ProxyStream() (SSE)
```

### 2.3 待新增 Proto

```protobuf
message StopGenerationRequest {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
}

message StopGenerationResponse {}

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

// 新增到 ChatService
rpc StopGeneration(StopGenerationRequest) returns (StopGenerationResponse) {
  option (google.api.http) = { post: "/v1/chat/stop" body: "*" };
}
rpc GetPendingMessages(GetPendingMessagesRequest) returns (GetPendingMessagesResponse) {
  option (google.api.http) = { get: "/v1/chat/pending" };
}
```

### 2.4 消息字段说明

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

---

## 三、Biz 层

### 3.1 领域模型

Chat 模块无独立 Biz 模型，依赖以下已有模型：

| 模型 | 包 | 用途 |
|------|-----|------|
| `biz.Agent` | `internal/biz` | Agent 配置查询 |
| `biz.Session` | `internal/biz` | 会话上下文 |
| `biz.Team` | `internal/biz` | Team 编排 |

### 3.2 依赖的 Usecase/Repo 接口

```go
type AgentRepository interface {
    GetAgentByID(ctx context.Context, id string) (Agent, error)
    GetAgentByKey(ctx context.Context, key string) (Agent, error)
}

type SessionUsecase struct {
    repo    SessionRepository
    agents  AgentRepository
    teams   TeamRepository
}

func (uc *SessionUsecase) Get(ctx context.Context, id string) (Session, error)
func (uc *SessionUsecase) UpdateContextFromLLMUsage(ctx context.Context, id string, usage LLMUsage) error

type TeamRepository interface {
    GetTeamByID(ctx context.Context, id string) (Team, error)
}

type UsageUsecase struct { ... }
func (uc *UsageUsecase) RecordIngress(ctx context.Context, event TokenUsageEvent) error

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

---

## 四、Data 层

Chat 模块无独立 Data 层，通过以下已有表间接使用：

### 4.1 用量记录表

```
model_token_usage_events
├── id               TEXT (UUID)
├── session_id       TEXT
├── agent_id         TEXT
├── provider         TEXT
├── model            TEXT
├── prompt_tokens    INTEGER
├── completion_tokens INTEGER
├── total_tokens     INTEGER
├── created_at       TEXT
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
├── chat.go              ← ChatService 主结构 + SendChatMessage/GetChatOptions
├── chat_native.go       ← 原生对话入口（SSE + unary）
├── trpc_turn.go         ← trpc-agent-go 单 Agent turn 执行
├── chat_usage_ingress.go ← 用量记录
├── session_compress.go  ← L0 上下文压缩
├── compress_wire.go     ← 压缩用 HTTP Client Wire 注入
```

### 5.2 ChatService 结构体

```go
type ChatService struct {
    chatv1.UnimplementedChatServiceServer
    client      *http.Client
    teams       biz.TeamRepository
    teamsNative *team.Runner
    usage       *biz.UsageUsecase
    td          runtimedeps.TurnDeps
}

type ChatServiceDeps struct {
    Broker       *biz.TeamRunEventBroker
    Teams        biz.TeamRepository
    TeamsNative  *team.Runner
    Usage        *biz.UsageUsecase
    Sessions     *biz.SessionUsecase
    Agents       biz.AgentRepository
    AgentsUC     *biz.AgentUsecase
    ToolsCatalog biz.ToolRepo
    LLMCatalog   *biz.LlmProviderModelUsecase
    SkillUC      *biz.SkillUsecase
    Sys          biz.SystemSettingRepo
    RT           *runtimedeps.Runtime
    Compress     biz.NativeTurnCompressor
    MonitorLogs  *biz.MonitorLogBroker
}

func NewChatService(deps ChatServiceDeps, client *http.Client) *ChatService {
    return &ChatService{
        client:      client,
        teams:       deps.Teams,
        teamsNative: deps.TeamsNative,
        usage:       deps.Usage,
        td: runtimedeps.TurnDeps{
            Broker:       deps.Broker,
            Sessions:     deps.Sessions,
            Agents:       deps.Agents,
            AgentsUC:     deps.AgentsUC,
            ToolsCatalog: deps.ToolsCatalog,
            LLMCatalog:   deps.LLMCatalog,
            SkillUC:      deps.SkillUC,
            Sys:          deps.Sys,
            RT:           deps.RT,
            Compress:     deps.Compress,
            MonitorLogs:  deps.MonitorLogs,
        },
    }
}
```

### 5.3 RPC 方法实现

#### SendChatMessage（unary，非流式）

```go
func (s *ChatService) SendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
    if req.SessionId == "" {
        return nil, kerrors.BadRequest("CHAT", "session_id is required")
    }
    if req.Content == "" {
        return nil, kerrors.BadRequest("CHAT", "content is required")
    }
    // 1. 查询 Session
    sess, err := s.td.Sessions.Get(ctx, req.SessionId)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    // 2. 走原生对话路径
    userMsg, agentMsg, err := s.runNativeTurn(ctx, sess, req)
    if err != nil {
        return nil, kerrors.FromError(err)
    }
    resp := &chatv1.SendChatMessageResponse{}
    if userMsg != nil {
        resp.UserMessage = userMsg
    }
    if agentMsg != nil {
        resp.AgentMessage = agentMsg
    }
    return resp, nil
}
```

#### GetChatOptions

```go
func (s *ChatService) GetChatOptions(ctx context.Context, req *chatv1.GetChatOptionsRequest) (*chatv1.GetChatOptionsResponse, error) {
    items := []*chatv1.ChatOption{
        {Type: "dialog_mode", Key: "default", Label: "标准对话", Enabled: true, SortOrder: 1},
        {Type: "dialog_mode", Key: "plan", Label: "深思考", Enabled: true, SortOrder: 2},
        {Type: "dialog_mode", Key: "code", Label: "仅代码", Enabled: true, SortOrder: 3},
    }
    return &chatv1.GetChatOptionsResponse{Items: items}, nil
}
```

### 5.4 SSE 流式对话（ProxyStream）

SSE 路由在 `internal/server/http.go` 中手动注册，不在 Proto 中：

```go
// internal/server/http.go 中注册 SSE 路由
srv.Route("/v1/chat").POST("/messages/stream", s.chatSSEHandler)
```

**请求流转**：

```
POST /v1/chat/messages/stream (SSE)
  → ChatService.ProxyStream()
    → LEGACY_REST_ORIGIN 已配置?
      → 反向代理到旧后端 (legacychat)
    → 未配置 → proxyNativeStream()
      → session.owner_type == "team"?
        → team.Runner.RunTurn() → SSE 事件流
      → session.owner_type == "agent"?
        → runSingleAgentViaTRPC()
          → BuildTRPCLLMAgent() → NewTRPCRunner() → RunTRPCUserTurn()
          → SSE 事件流
```

### 5.5 SSE 事件协议

| 事件 | 方向 | 载荷 JSON | 说明 |
|------|------|-----------|------|
| `user_message` | server→client | `{"id":"...","session_id":"...","role":"user","content_markdown":"..."}` | 用户消息回显 |
| `delta` | server→client | `{"content":"..."}` 或 `{"reasoning_content":"..."}` | 流式增量文本 |
| `tool.call` | server→client | `{"session_id":"...","tool_name":"...","tool_call_id":"..."}` | 工具调用通知 |
| `done` | server→client | `{"agent_message":{"id":"...","content_markdown":"..."}}` | 生成完成 |
| `error` | server→client | `{"message":"..."}` | 错误信息 |

### 5.6 用量记录

```go
// internal/service/chat_usage_ingress.go
func (s *ChatService) recordChatIngressUsage(ctx context.Context, sess biz.Session, provider, model string, usage model.Usage) {
    if os.Getenv("CHAT_RECORD_USAGE_INGRESS") == "0" {
        return
    }
    event := biz.TokenUsageEvent{
        SessionID:       sess.ID,
        AgentID:         sess.OwnerID,
        Provider:        provider,
        Model:           model,
        PromptTokens:    usage.PromptTokens,
        CompletionTokens: usage.CompletionTokens,
        TotalTokens:     usage.TotalTokens,
    }
    _ = s.usage.RecordIngress(ctx, event)
}
```

---

## 六、运行时层

### 6.1 Agent 构建

```go
// internal/agent/trpc_build.go
type BuilderDeps struct {
    Catalog      *biz.LlmProviderModelUsecase
    AgentUC      *biz.AgentUsecase
    Agents       biz.AgentRepository
    ToolsCatalog biz.ToolRepo
    RT           *runtimedeps.Runtime
    Memory       session.Service
    Provider     string
    Model        string
}

func BuildTRPCLLMAgent(ctx context.Context, ag biz.Agent, deps BuilderDeps) (*llmagent.Agent, error) {
    // 1. 获取 LLM 模型
    llm, err := provider.ModelForProviderModel(ctx, deps.Provider, deps.Model)
    if err != nil {
        return nil, kerrors.InternalServer("AGENT", err.Error())
    }
    // 2. 挂载工具
    var tools []tool.Tool
    var toolsets []tool.Toolset
    mount := toolsPkg.TurnMount{
        AgentUC:    deps.AgentUC,
        Agents:     deps.Agents,
        Tools:      deps.ToolsCatalog,
        SkillUC:    deps.SkillUC,
        MCPServers: deps.MCPServers,
    }
    if err := mount.Attach(ctx, ag, "", &tools, &toolsets); err != nil {
        return nil, kerrors.InternalServer("AGENT", err.Error())
    }
    // 3. 构建 Agent
    opts := []llmagent.Option{
        llmagent.WithModel(llm),
        llmagent.WithInstruction(ag.SystemPrompt),
        llmagent.WithTools(tools...),
        llmagent.WithToolsets(toolsets...),
    }
    return llmagent.New(opts...)
}
```

### 6.2 Runner 构建

```go
// internal/agent/trpc_runtime.go
func NewTRPCRunner(root agent.Agent, sessSvc session.Service, rt *runtimedeps.Runtime) (*runner.Runner, error) {
    return runner.New(root,
        runner.WithSessionService(sessSvc),
        runner.WithMemoryService(rt.SessionMemory),
    ), nil
}

func RunTRPCUserTurn(ctx context.Context, r *runner.Runner, userID, sessionID, msg string) (<-chan agent.Event, error) {
    return r.Run(ctx, userID, sessionID, msg)
}
```

### 6.3 Team 编排

```go
// internal/team/runner.go
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

func (r *Runner) RunTurn(ctx context.Context, team biz.Team, session biz.Session, agents []biz.Agent, msg string) (<-chan agent.Event, error) {
    // 1. 构建 Team Root Agent
    root, err := BuildWorkflowRoot(ctx, team, agents, r.deps())
    // 2. 构建 Runner
    runner, err := NewTRPCRunner(root, r.sessSvc, r.rt)
    // 3. 执行
    return runner.Run(ctx, userID, session.ID, msg)
}
```

---

## 七、Wire 注入

### 7.1 ProviderSet

```go
// internal/service/service.go
var ProviderSet = wire.NewSet(
    NewChatService,
    provideChatServiceDeps,
    // ... 其他 Service
)
```

### 7.2 provideChatServiceDeps

```go
// cmd/admin/wire.go
func provideChatServiceDeps(
    broker *biz.TeamRunEventBroker,
    teams biz.TeamRepository,
    teamsNative *team.Runner,
    usage *biz.UsageUsecase,
    sessions *biz.SessionUsecase,
    agents biz.AgentRepository,
    agentsUC *biz.AgentUsecase,
    toolsCatalog biz.ToolRepo,
    llmCatalog *biz.LlmProviderModelUsecase,
    skillUC *biz.SkillUsecase,
    sys biz.SystemSettingRepo,
    rt *runtimedeps.Runtime,
    compress biz.NativeTurnCompressor,
    monitorLogs *biz.MonitorLogBroker,
) service.ChatServiceDeps {
    return service.ChatServiceDeps{
        Broker:       broker,
        Teams:        teams,
        TeamsNative:  teamsNative,
        Usage:        usage,
        Sessions:     sessions,
        Agents:       agents,
        AgentsUC:     agentsUC,
        ToolsCatalog: toolsCatalog,
        LLMCatalog:   llmCatalog,
        SkillUC:      skillUC,
        Sys:          sys,
        RT:           rt,
        Compress:     compress,
        MonitorLogs:  monitorLogs,
    }
}
```

---

## 八、Web 前端设计

### 8.1 文件结构

```
web/src/
├── services/index.ts              ← createChatService 导出
├── features/chat/
│   ├── api.ts                     ← Chat API 调用封装
│   ├── types.ts                   ← TypeScript 类型定义
│   ├── toolEventMarkdown.ts       ← 工具事件 Markdown 渲染
│   └── composables/
│       └── useChatWorkspace.ts    ← 对话工作区 composable
├── stores/chat/
│   └── index.ts                   ← Pinia Store（useChatStore）
├── components/chat/
│   ├── ChatLayout.vue             ← 三栏布局容器
│   ├── AgentTeamList.vue          ← 左侧 Agent/Team 列表
│   ├── SessionHistory.vue         ← 右侧 Session 历史
│   ├── ChatMessages.vue           ← 中间对话内容
│   ├── ChatInput.vue              ← 底部输入区域
│   ├── ChatToolbar.vue            ← 工具条
│   ├── ChatMessageBubble.vue      ← 单条消息气泡
│   └── ContextProgress.vue        ← 上下文额度圆环
```

### 8.2 页面布局

```
┌──────────┬─────────────────────────────────┬──────────┐
│ Agent/   │                                 │ Session  │
│ Team     │        对话内容区域               │ 历史     │
│ 列表     │     (q-chat-message)             │ 列表     │
│ 120px    │                                 │ 120px    │
│          ├─────────────────────────────────┤          │
│          │  输入框 (autogrow, max 400px)    │          │
│          │  [模式][Provider][上下文] [文件][发送] │          │
└──────────┴─────────────────────────────────┴──────────┘
```

### 8.3 TypeScript 类型定义

```typescript
// features/chat/types.ts
export interface SendChatMessageRequest {
  session_id: string
  agent_key?: string
  team_id?: string
  content: string
  options?: SendMessageOptions
}

export interface SendMessageOptions {
  dialog_mode?: 'default' | 'plan' | 'code'
  provider?: string
  model?: string
  attachments?: AttachmentRef[]
}

export interface AttachmentRef {
  id: string
}

export interface ChatOption {
  type: string
  key: string
  label: string
  enabled: boolean
  sort_order: number
  metadata_json?: string
}

export interface SSEEvent {
  type: 'user_message' | 'delta' | 'tool.call' | 'done' | 'error'
  data: Record<string, unknown>
}

export interface ToolCallEvent {
  session_id: string
  tool_name: string
  tool_call_id: string
}

export interface AgentMessage {
  id: string
  content_markdown: string
}

export interface PendingMessage {
  id: string
  content: string
  status: string
  created_at: string
}
```

### 8.4 API 调用

```typescript
// features/chat/api.ts
import { createChatService } from 'services/index'

const chatService = createChatService()

export async function getChatOptions(type?: string): Promise<ChatOption[]> {
  const resp = await chatService.getChatOptions({ type })
  return resp.items ?? []
}

export function streamChatMessage(
  req: SendChatMessageRequest,
  onDelta: (delta: string, isReasoning?: boolean) => void,
  onToolCall: (call: ToolCallEvent) => void,
  onDone: (msg: AgentMessage) => void,
  onError: (err: string) => void,
): AbortController {
  const controller = new AbortController()
  const backendOrigin = getBackendOrigin()
  const url = `${backendOrigin}/v1/chat/messages/stream`

  fetch(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
    signal: controller.signal,
    credentials: 'include',
  }).then(async (response) => {
    const reader = response.body!.getReader()
    const decoder = new TextDecoder()
    let buffer = ''
    while (true) {
      const { done, value } = await reader.read()
      if (done) break
      buffer += decoder.decode(value, { stream: true })
      const lines = buffer.split('\n')
      buffer = lines.pop()!
      for (const line of lines) {
        if (!line.startsWith('data: ')) continue
        const raw = line.slice(6)
        if (raw === '[DONE]') { onDone({ id: '', content_markdown: '' }); return }
        try {
          const evt = JSON.parse(raw)
          switch (evt.type) {
            case 'delta':
              onDelta(evt.content ?? '', !!evt.reasoning_content)
              break
            case 'tool.call':
              onToolCall(evt)
              break
            case 'done':
              onDone(evt.agent_message ?? { id: '', content_markdown: '' })
              break
            case 'error':
              onError(evt.message ?? 'unknown error')
              break
          }
        } catch { /* skip malformed */ }
      }
    }
  }).catch((err) => {
    if (err.name !== 'AbortError') onError(err.message)
  })

  return controller
}

export async function stopGeneration(sessionId: string): Promise<void> {
  await kratosApi.post('/v1/chat/stop', { session_id: sessionId })
}

export async function getPendingMessages(sessionId: string): Promise<PendingMessage[]> {
  const resp = await kratosApi.get(`/v1/chat/pending`, { params: { session_id: sessionId } })
  return resp.data.items ?? []
}
```

### 8.5 Pinia Store

```typescript
// stores/chat/index.ts
import { defineStore } from 'pinia'

export const useChatStore = defineStore('chat', {
  state: () => ({
    currentAgentId: null as string | null,
    currentTeamId: null as string | null,
    currentSessionId: null as string | null,
    dialogMode: 'default' as 'default' | 'plan' | 'code',
    selectedProvider: '',
    selectedModel: '',
    contextUsedRatio: 0,
    isGenerating: false,
    pendingMessages: [] as PendingMessage[],
    messages: [] as ChatMessageRow[],
    abortController: null as AbortController | null,
  }),
  actions: {
    async sendMessage(content: string, attachments?: AttachmentRef[]) {
      if (!this.currentSessionId) return
      this.isGenerating = true
      this.abortController = streamChatMessage(
        {
          session_id: this.currentSessionId,
          content,
          options: {
            dialog_mode: this.dialogMode,
            provider: this.selectedProvider || undefined,
            model: this.selectedModel || undefined,
            attachments,
          },
        },
        (delta, isReasoning) => { /* append to last message */ },
        (call) => { /* add tool call indicator */ },
        (msg) => { this.isGenerating = false; this.abortController = null },
        (err) => { this.isGenerating = false; this.abortController = null; /* show error */ },
      )
    },
    stopGeneration() {
      this.abortController?.abort()
      this.isGenerating = false
    },
  },
})
```

### 8.6 组件设计

#### ChatLayout.vue

```vue
<template>
  <q-layout>
    <AgentTeamList
      :agents="agents"
      :teams="teams"
      :selected-id="currentAgentId"
      @select="onSelectAgent"
      class="col-auto" style="width: 120px"
    />
    <div class="col column">
      <ChatMessages :messages="messages" class="col" />
      <ChatInput
        :is-generating="isGenerating"
        :dialog-mode="dialogMode"
        :provider="selectedProvider"
        :model="selectedModel"
        :context-ratio="contextUsedRatio"
        @send="onSend"
        @stop="onStop"
        @update:dialog-mode="onDialogModeChange"
        @update:provider="onProviderChange"
        @update:model="onModelChange"
      />
    </div>
    <SessionHistory
      :sessions="sessions"
      :selected-id="currentSessionId"
      @select="onSelectSession"
      class="col-auto" style="width: 120px"
    />
  </q-layout>
</template>
```

#### AgentTeamList.vue

| Prop | 类型 | 说明 |
|------|------|------|
| `agents` | `Agent[]` | Agent 列表 |
| `teams` | `Team[]` | Team 列表 |
| `selectedId` | `string \| null` | 当前选中 ID |

| Emit | 载荷 | 说明 |
|------|------|------|
| `select` | `{ type: 'agent' \| 'team', id: string }` | 选中 Agent/Team |

功能要点：
- 宽度 120px，高度 100%
- Agent 和 Team 分组显示
- 默认 Agent/Team 在最上方，不可拖拽调序
- 顶部搜索框：按名称搜索
- 条目：左侧工作状态指示灯 + 名称，右侧设置/删除按钮
- 选中时背景高亮
- 列表右侧中间折叠按钮，带动画

#### SessionHistory.vue

| Prop | 类型 | 说明 |
|------|------|------|
| `sessions` | `Session[]` | Session 列表 |
| `selectedId` | `string \| null` | 当前选中 ID |

| Emit | 载荷 | 说明 |
|------|------|------|
| `select` | `string` (session ID) | 选中 Session |
| `delete` | `string` (session ID) | 删除 Session |
| `create` | — | 创建新 Session |

功能要点：
- 宽度 120px，高度 100%
- 每条：右侧 session 名称，下角标时间，左侧圆环显示上下文额度比
- 底部：左侧新建 Session，右侧一键删除历史
- 列表左侧中间折叠按钮，带动画

#### ChatInput.vue

| Prop | 类型 | 说明 |
|------|------|------|
| `isGenerating` | `boolean` | 是否正在生成 |
| `dialogMode` | `string` | 对话模式 |
| `provider` | `string` | 当前 Provider |
| `model` | `string` | 当前模型 |
| `contextRatio` | `number` | 上下文使用比例 |

| Emit | 载荷 | 说明 |
|------|------|------|
| `send` | `{ content: string, attachments?: AttachmentRef[] }` | 发送消息 |
| `stop` | — | 停止生成 |
| `update:dialog-mode` | `string` | 切换对话模式 |
| `update:provider` | `string` | 切换 Provider |
| `update:model` | `string` | 切换模型 |

功能要点：
- 初始高度 100px，autogrow，最高 400px
- 底部工具条（固定高 40px）：
  - 左侧：对话模式 `QSelect`、Provider `QSelect`、上下文使用量 `QCircularProgress`
  - 右侧：文件导入 `QBtn`、发送/停止按钮 `QBtn`
- 文件导入时，输入框上方显示 30×30px 方框（进度、名称、关闭按钮）
- `Enter` 发送，`Shift + Enter` 换行
- 生成中发送按钮切换为停止图标

#### ChatMessages.vue

| Prop | 类型 | 说明 |
|------|------|------|
| `messages` | `ChatMessageRow[]` | 消息列表 |

使用 `q-chat-message` 组件显示头像、时间、内容。暗黑模式下确保正文、代码块、工具结果、时间戳等文本可读。

#### ContextProgress.vue

| Prop | 类型 | 说明 |
|------|------|------|
| `ratio` | `number` | 上下文使用比例 0-1 |

颜色阈值：`<0.6` 绿 / `0.6-0.8` 黄 / `>0.8` 红

### 8.7 UX 规范

- 玻璃材质：`background: var(--glass-surface); backdrop-filter: blur(var(--glass-blur-default))`
- 日间主操作色：`var(--color-accent)` = `#E9A23B`
- 夜间霓虹强调：`var(--color-neon-cyan)` = `#00E5FF`
- 输入框圆角：12-16px
- 消息气泡圆角：16-20px
- 暗黑模式可读性：聊天记录正文、代码块、工具结果、时间戳等文本必须保证对比度

---

## 九、待优化项

| 优先级 | 项目 | 说明 |
|--------|------|------|
| P0 | `NewChatService` 参数封装 | ✅ 已完成：改为 `ChatServiceDeps` struct |
| P1 | `firstNonEmpty` 统一 | 6 处重复定义 → `pkg/strutil.FirstNonEmpty` |
| P1 | `memory_decode.go` 提取 | `ifaceStr`/`ifaceBool` 等通用函数 → `pkg/` |
| P2 | `legacychat` 废弃 | `LEGACY_REST_ORIGIN` 模式长期应移除 |
| P2 | `compress_wire.go` 合并 | 仅含一个函数，合并到 `session_compress.go` |
| P2 | 停止生成 | 新增 `StopGeneration` RPC |
| P2 | 待执行队列 | 新增 `GetPendingMessages` RPC |
