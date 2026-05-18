# Runner 运行器模块 — 实现设计文档

> 对应需求：`40 runner.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent 运行器完善：AgentFactory、ArtifactService 注入、SessionIngestor、AwaitUserReplyRouting（框架层）、AgentLookup、RalphLoop、CancelRun/EnqueueUserMessage RPC、RunnerRegistry/RunnerManager。对标 trpc-agent-go `runner` 包，将项目从当前 Service 层自管理的运行模式升级为框架层完整驱动的 ManagedRunner + SteerableRunner。

### 核心架构

```
用户消息 → ChatService.SendChatMessage
             ↓
         lockSession() → 会话级互斥
             ↓
         activeRuns 检查 → 运行中则入 pendingQueue
             ↓
         BuildTRPCLLMAgentCached → 构建/缓存 Agent
             ↓
         NewTRPCRunner(root, deps) → 创建 Runner（注入 Session/Memory/Plugins/Artifact/Ingestor/Routing/RalphLoop）
             ↓
         Runner.Run(userID, sessionID, message) → 框架层执行
             ↓
         ConsumeEventStream → 事件循环：持久化 + 转发 + 状态更新
             ↓
         sessionIngestor.IngestSession() → 会话完成后摄入外部记忆
             ↓
         processPendingQueue → 处理待执行队列
```

### trpc-agent-go runner 包结构

```
pkg/trpc-agent-go/runner/
├── runner.go              # Runner/ManagedRunner/SteerableRunner 接口和实现
├── agent_lookup.go        # Agent 查找（注册表 + 工厂回退）
├── await_user_reply.go    # AwaitUserReply 路由（会话状态驱动）
└── ralph_loop.go          # RalphLoop 迭代执行（带验证和完成承诺）
```

### Runner 接口层级

| 接口 | 方法 | 说明 |
|------|------|------|
| `Runner` | `Run` + `Close` | 基础运行器 |
| `ManagedRunner` | + `Cancel` + `RunStatus` | 可管理的运行器 |
| `SteerableRunner` | + `EnqueueUserMessage` | 可转向的运行器 |

### 现状与差距

| 能力 | trpc-agent-go | 当前项目 | 状态 |
|------|--------------|---------|------|
| Runner.Run / Close | ✅ | ✅ | `NewTRPCRunner` 返回 `ManagedRunner` |
| ManagedRunner.Cancel | ✅ | ✅ | `CancelTRPCRun` 辅助函数 |
| ManagedRunner.RunStatus | ✅ | ✅ | `TRPCRunStatus` 辅助函数 |
| SteerableRunner.EnqueueUserMessage | ✅ | ✅ | `EnqueueTRPCUserMessage` 辅助函数 |
| PluginManager 注入 | ✅ | ✅ | `plugintrpc.Runtime` + `WithPlugins` |
| AwaitUserReply（Service 层） | — | ✅ | `serviceawaitreply.ServiceTool` + `AwaitUserReply` RPC |
| GetRunStatus RPC | — | ✅ | `ChatService.GetRunStatus` |
| StopGeneration RPC | — | ✅ | `ChatService.StopGeneration` |
| PendingQueue | — | ✅ | `pendingQueue` + `processPendingQueue` |
| BuildCache LRU + TTL | — | ✅ | `cache.go` |
| ArtifactService 适配器 | ✅ | 🟡 | 适配器已有，未注入 Runner |
| AgentFactory | ✅ | ❌ | 未注册 `WithAgentFactory` |
| SessionIngestor | ✅ | ❌ | 未实现 `WithSessionIngestor` |
| AwaitUserReplyRouting（框架层） | ✅ | ❌ | 未启用 `WithAwaitUserReplyRouting` |
| AgentLookup | ✅ | ❌ | Runner 未维护 Agent 注册表 |
| RalphLoop | ✅ | ❌ | 未配置 `WithRalphLoop` |
| CancelRun RPC | — | ❌ | 仅有 `StopGeneration`，缺少按 requestID 取消 |
| EnqueueUserMessage RPC | — | ❌ | 仅有辅助函数，无 RPC 入口 |
| RunnerRegistry / RunnerManager | — | ❌ | 每次请求创建临时 Runner |

---

## 二、Proto 层

无需独立 Proto 服务。通过 Chat Service 暴露 Runner 控制接口。

### 当前 Proto 状态

`api/kratos/chat/v1/chat.proto` 已有：

| RPC | 路由 | 状态 |
|-----|------|------|
| `StopGeneration` | POST `/v1/chat/stop` | ✅ 已有 |
| `GetRunStatus` | GET `/v1/chat/run-status` | ✅ 已有 |
| `AwaitUserReply` | POST `/v1/chat/await-reply` | ✅ 已有 |
| `GetPendingMessages` | GET `/v1/chat/pending` | ✅ 已有 |
| `CancelPendingMessage` | POST `/v1/chat/pending/cancel` | ✅ 已有 |
| `UpdatePendingMessage` | POST `/v1/chat/pending/update` | ✅ 已有 |

### Chat Proto 扩展

```protobuf
// api/kratos/chat/v1/chat.proto — 新增 RPC

message CancelRunRequest {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
  string request_id = 2 [(google.api.field_behavior) = REQUIRED];
}

message CancelRunResponse {
  bool cancelled = 1;
}

message EnqueueUserMessageRequest {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
  string request_id = 2 [(google.api.field_behavior) = REQUIRED];
  string content = 3 [(google.api.field_behavior) = REQUIRED];
}

message EnqueueUserMessageResponse {
  bool enqueued = 1;
}

service ChatService {
  // ... 已有方法 ...

  rpc CancelRun(CancelRunRequest) returns (CancelRunResponse) {
    option (google.api.http) = { post: "/v1/chat/cancel-run" body: "*" };
  }
  rpc EnqueueUserMessage(EnqueueUserMessageRequest) returns (EnqueueUserMessageResponse) {
    option (google.api.http) = { post: "/v1/chat/enqueue-user-message" body: "*" };
  }
}
```

### GetRunStatus Proto 对齐

当前 `GetRunStatus` 返回的 `RunStatus` 消息仅包含 `run_id`、`status`、`error_message`、`updated_at`。需与 `ManagedRunner.RunStatus` 对齐，增加 `invocation_id`、`agent_name`、`started_at`、`last_event_at`、`event_count` 字段：

```protobuf
message RunStatus {
  string run_id = 1;
  string status = 2;
  string error_message = 3;
  string updated_at = 4;
  // 新增：与 ManagedRunner.RunStatus 对齐
  string invocation_id = 5;
  string agent_name = 6;
  string started_at = 7;
  string last_event_at = 8;
  int32 event_count = 9;
}
```

### AgentRuntimeSettings Ent Schema 扩展

```go
// internal/data/ent/schema/agent_runtime_setting.go — 新增字段

field.Bool("runner_await_user_reply_routing").Default(false),
field.Int("runner_max_run_duration_seconds").Default(0),
field.Bool("runner_detached_cancel").Default(false),
field.Int("ralph_loop_max_iterations").Default(0),
field.String("ralph_loop_completion_promise").Default(""),
field.String("ralph_loop_verify_command").Default(""),
field.Int("ralph_loop_verify_timeout_seconds").Default(0),
```

---

## 三、Biz 层

### 3.1 领域模型

```go
type RunnerConfig struct {
    AppName               string
    AwaitUserReplyRouting bool
    MaxRunDuration        time.Duration
    DetachedCancel        bool
}

type RalphLoopConfig struct {
    MaxIterations     int
    CompletionPromise string
    PromiseTagOpen    string
    PromiseTagClose   string
    VerifyCommand     string
    VerifyWorkDir     string
    VerifyTimeout     time.Duration
    VerifyEnv         map[string]string
}

type RunStatusInfo struct {
    RequestID    string
    InvocationID string
    AgentName    string
    SessionID    string
    UserID       string
    StartedAt    string
    LastEventAt  string
    EventCount   int
}
```

### 3.2 RunnerUsecase

当前运行控制逻辑分散在 `ChatService` 中（`activeRuns`、`runStatuses`、`pendingQueue` 等）。引入 `RunnerUsecase` 将运行控制逻辑下沉到 Biz 层：

```go
type RunnerUsecase struct {
    registry *RunnerRegistry
}

func NewRunnerUsecase(registry *RunnerRegistry) *RunnerUsecase

func (uc *RunnerUsecase) GetRunStatus(ctx context.Context, sessionID, requestID string) (*RunStatusInfo, error)
func (uc *RunnerUsecase) CancelRun(ctx context.Context, sessionID, requestID string) (bool, error)
func (uc *RunnerUsecase) EnqueueUserMessage(ctx context.Context, sessionID, requestID, content string) error
```

### 3.3 Runner 注册表

项目需要维护一个全局的 Runner 注册表，支持多 Runner 实例并行：

```go
type RunnerRegistry struct {
    mu      sync.RWMutex
    runners map[string]trpcrunner.Runner
}

func NewRunnerRegistry() *RunnerRegistry

func (r *RunnerRegistry) Register(key string, runner trpcrunner.Runner)
func (r *RunnerRegistry) Get(key string) (trpcrunner.Runner, bool)
func (r *RunnerRegistry) Unregister(key string)
func (r *RunnerRegistry) List() []string
```

---

## 四、运行时层

### 4.1 TRPCRunnerDeps 扩展

当前 `TRPCRunnerDeps` 仅有 `AppName`、`SessionService`、`MemoryService`、`Plugins`。需扩展：

```go
type TRPCRunnerDeps struct {
    AppName        string
    SessionService trpcsession.Service
    MemoryService  trpcmemory.Service
    Plugins        []trpcplugin.Plugin

    // 新增
    Ingestor              trpcsession.Ingestor
    ArtifactService       trpcartifact.Service
    AwaitUserReplyRouting bool
    AgentFactories        map[string]trpcrunner.AgentFactory
    RalphLoop             *trpcrunner.RalphLoopConfig
}
```

### 4.2 NewTRPCRunner 扩展

当前 `NewTRPCRunner` 已处理 `SessionService`、`MemoryService`、`Plugins`。需在现有逻辑基础上增加：

```go
if deps.Ingestor != nil {
    opts = append(opts, trpcrunner.WithSessionIngestor(deps.Ingestor))
}
if deps.ArtifactService != nil {
    opts = append(opts, trpcrunner.WithArtifactService(deps.ArtifactService))
}
if deps.AwaitUserReplyRouting {
    opts = append(opts, trpcrunner.WithAwaitUserReplyRouting(true))
}
if deps.RalphLoop != nil {
    opts = append(opts, trpcrunner.WithRalphLoop(*deps.RalphLoop))
}
for name, factory := range deps.AgentFactories {
    opts = append(opts, trpcrunner.WithAgentFactory(name, factory))
}
```

### 4.3 AgentFactory 实现

`BizAgentFactory` 根据名称从数据库查找 Agent 配置并动态构建 trpc Agent：

```go
type BizAgentFactory struct {
    agents   biz.AgentRepository
    agentsUC *biz.AgentUsecase
    deps     TRPCBuilderDeps
}

func NewBizAgentFactory(
    agents biz.AgentRepository,
    agentsUC *biz.AgentUsecase,
    deps TRPCBuilderDeps,
) *BizAgentFactory

func (f *BizAgentFactory) Create(
    ctx context.Context,
    ro trpcagent.RunOptions,
) (trpcagent.Agent, error)
```

`Create` 逻辑：从 `ro.AgentByName` 获取名称 → `agentsUC.Get` 查找 → `BuildTRPCLLMAgent` 构建。

### 4.4 SessionIngestor 实现

`BizSessionIngestor` 将完成的 Session 转录文本摄入外部记忆平台：

```go
type BizSessionIngestor struct {
    sessions *biz.SessionUsecase
    memory   trpcmemory.Service
}

func NewBizSessionIngestor(
    sessions *biz.SessionUsecase,
    memory trpcmemory.Service,
) *BizSessionIngestor

func (ing *BizSessionIngestor) IngestSession(
    ctx context.Context,
    sess *trpcsession.Session,
) error
```

`IngestSession` 逻辑：检查 `memory` 是否可用 → 提取 Session 事件中的对话转录 → 调用 `memory.Add` 摄入。摄入失败仅记录日志，不阻塞主流程。

### 4.5 AwaitUserReplyRouting（框架层）

当前项目通过 Service 层自行实现 AwaitUserReply（`serviceawaitreply.ServiceTool` + `ChatService.makeAwaitReplyFunc` + `ChatService.AwaitUserReply` RPC），实现了 mid-turn 阻塞等待用户回复。

框架层的 `WithAwaitUserReplyRouting` 提供的是跨 turn 的路由能力：Agent 调用 `await_user_reply` 后在 Session 状态中记录路由，下一轮用户消息自动路由到该 Agent。

两层机制互补：
- **Service 层**（已有）：mid-turn 阻塞，当前 turn 内等待用户回复
- **框架层**（待启用）：跨 turn 路由，下一轮用户消息自动路由到指定 Agent

启用方式：在 `NewTRPCRunner` 中传入 `WithAwaitUserReplyRouting(true)`，框架的 `runner.applyAwaitUserReplyRoute` 会自动处理路由逻辑。

### 4.6 RalphLoop 配置

`RalphLoopConfig` 从 `AgentRuntimeSettings` 数据库配置映射：

```go
func ralphLoopConfigFromSettings(settings *biz.AgentRuntimeSettings) *trpcrunner.RalphLoopConfig
```

映射规则：
- `RalphLoopMaxIterations > 0` 时启用
- `CompletionPromise` → `PromiseTagOpen`/`PromiseTagClose` 默认 `<promise>`/`</promise>`
- `VerifyCommand` → `VerifyCommand` + `VerifyTimeout`

### 4.7 RunnerManager

当前每次请求创建临时 Runner（`runSingleAgentViaTRPC` 中 `NewTRPCRunner` + `defer runner.Close()`）。引入 `RunnerManager` 支持长生命周期 Runner 实例：

```go
type RunnerManager struct {
    registry *RunnerRegistry
    deps     RunnerFactoryDeps
}

type RunnerFactoryDeps struct {
    SessionSvc   trpcsession.Service
    MemorySvc    trpcmemory.Service
    Ingestor     trpcsession.Ingestor
    ArtifactSvc  trpcartifact.Service
    Plugins      []trpcplugin.Plugin
}

func NewRunnerManager(deps RunnerFactoryDeps) *RunnerManager

func (m *RunnerManager) CreateRunner(
    ctx context.Context,
    key string,
    root trpcagent.Agent,
    opts ...trpcrunner.Option,
) (trpcrunner.Runner, error)

func (m *RunnerManager) CloseRunner(key string) error
```

`CreateRunner` 逻辑：构建 `TRPCRunnerDeps` → `NewTRPCRunner` → `registry.Register`。

---

## 五、Data 层

无需新增独立数据表。Runner 运行时状态通过 `runHandle` 内存管理。

### AgentRuntimeSettings Ent Schema 扩展

见 §二 AgentRuntimeSettings 扩展。

---

## 六、Service 层

### 6.1 ChatService 现有运行控制

当前 `ChatService` 已有：

| 字段/方法 | 说明 |
|-----------|------|
| `activeRuns sync.Map` | sessionID → Runner 实例 |
| `runStatuses sync.Map` | sessionID → runStatusEntry |
| `pendingQueue sync.Map` | sessionID → []pendingEntry |
| `pendingCancels sync.Map` | sessionID → context.CancelFunc |
| `awaitChans sync.Map` | sessionID → chan awaitReplyCh |
| `sessionMu sync.Map` | sessionID → *sync.Mutex |
| `GetRunStatus` | RPC：查询运行状态 |
| `StopGeneration` | RPC：停止生成 |
| `CancelRun` | 内部方法：取消运行 |
| `AwaitUserReply` | RPC：提交用户回复 |
| `makeAwaitReplyFunc` | 构建 await_reply 阻塞回调 |

### 6.2 ChatService 扩展

新增 RPC 实现：

```go
func (s *ChatService) CancelRunRPC(ctx context.Context, req *chatv1.CancelRunRequest) (*chatv1.CancelRunResponse, error)
func (s *ChatService) EnqueueUserMessageRPC(ctx context.Context, req *chatv1.EnqueueUserMessageRequest) (*chatv1.EnqueueUserMessageResponse, error)
```

`CancelRunRPC` 逻辑：从 `activeRuns` 获取 Runner → `CancelTRPCRun(r, requestID)`。

`EnqueueUserMessageRPC` 逻辑：从 `activeRuns` 获取 Runner → `EnqueueTRPCUserMessage(r, requestID, content)`。

### 6.3 GetRunStatus 对齐

当前 `GetRunStatus` 从 `runStatuses` sync.Map 读取 Service 层维护的状态。需同时查询 `ManagedRunner.RunStatus` 以获取框架层的完整状态信息（invocation_id、agent_name、event_count 等）。

---

## 七、Wire 注入

```go
// internal/agent/wire.go — 新增 Provider

var ProviderSet = wire.NewSet(
    // ... 已有 ...
    NewBizAgentFactory,
    NewBizSessionIngestor,
    NewRunnerRegistry,
    NewRunnerManager,
)

// internal/service/wire.go — ChatServiceDeps 扩展
type ChatServiceDeps struct {
    // ... 已有 ...
    RunnerManager *agent.RunnerManager
}
```

---

## 八、Web 前端设计

Runner 为运行时基础设施，无独立前端页面。相关 UI 通过 Chat 页面暴露。

### 当前前端状态

| 组件/文件 | 状态 | 说明 |
|-----------|------|------|
| `web/src/features/chat/api.ts` | ✅ | 已有 `getRunStatus`、`stopGeneration`、`awaitUserReply` |
| `web/src/composables/useRunStatus.ts` | ✅ | 已有轮询运行状态 + 提交回复 |
| `ChatRunnerStatus.vue` | ❌ | 未创建独立运行状态指示器组件 |
| `ChatEnqueueMessage.vue` | ❌ | 未创建运行中追加消息组件 |

### 8.1 Chat 页面集成

**运行状态指示器**：

| 元素 | 组件 | 说明 |
|------|------|------|
| 运行状态 | `QBadge` | 在聊天输入框旁显示当前运行状态 |
| 取消按钮 | `QBtn` | 运行中时显示取消按钮 |
| 追加消息 | `QInput` + `QBtn` | 运行中时允许用户追加消息 |

**文件结构**：

```
web/src/features/chat/
└── components/
    ├── ChatRunnerStatus.vue    ← 运行状态指示器
    └── ChatEnqueueMessage.vue  ← 运行中追加消息
```

### 8.2 ChatRunnerStatus.vue

| 区域 | 组件 | 说明 |
|------|------|------|
| 状态标签 | `QBadge` | `running` / `completed` / `cancelled` / `awaiting_user` |
| Agent 名称 | `QChip` | 当前运行的 Agent |
| 运行时长 | `span` | 从 startedAt 计算已运行时间 |
| 事件数 | `span` | 已处理的事件数量 |
| 取消按钮 | `QBtn` | 点击调用 CancelRun API |

### 8.3 ChatEnqueueMessage.vue

| 区域 | 组件 | 说明 |
|------|------|------|
| 输入框 | `QInput` | 追加消息输入 |
| 发送按钮 | `QBtn` | 点击调用 EnqueueUserMessage API |
| 状态提示 | `QTooltip` | 消息将在工具调用边界后注入 |

### 8.4 API 接口扩展

当前 `api.ts` 已有 `getRunStatus`、`stopGeneration`、`awaitUserReply`。需新增：

```typescript
export async function cancelRun(sessionId: string, requestId: string): Promise<boolean>
export async function enqueueUserMessage(sessionId: string, requestId: string, content: string): Promise<boolean>
```

### 8.5 RunStatus 类型扩展

当前 `RunStatus` 接口仅有 `runId`、`status`、`errorMessage`、`updatedAt`。需与 Proto 对齐新增：

```typescript
export interface RunStatus {
  runId: string;
  status: RunStatusValue;
  errorMessage: string;
  updatedAt: string;
  // 新增
  invocationId: string;
  agentName: string;
  startedAt: string;
  lastEventAt: string;
  eventCount: number;
}
```
