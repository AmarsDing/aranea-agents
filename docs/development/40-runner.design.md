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
| Runner.Run / Close | ✅ | ✅ | `NewTRPCRunner` → `ManagedRunner` |
| ManagedRunner.Cancel | ✅ | ✅ | `RunRegistry.Cancel` + `CancelTRPCRun` |
| ManagedRunner.RunStatus | ✅ | ✅ | `GetRunStatus` 合并 `FrameworkRunStatusFromRunner` |
| SteerableRunner.EnqueueUserMessage | ✅ | ✅ | `EnqueueUserMessage` RPC + `RunRegistry` |
| PluginManager 注入 | ✅ | ✅ | `plugintrpc.Runtime` + `WithPlugins` |
| AwaitUserReply（Service 层） | — | ✅ | `serviceawaitreply` + `AwaitUserReply` RPC |
| GetRunStatus RPC | — | ✅ | `ChatService.GetRunStatus` |
| StopGeneration / WS cancel | — | ✅ | `RunRegistry.Cancel` |
| PendingQueue | — | ✅ | `internal/runtime/pending_queue.go` |
| BuildCache LRU + TTL | — | ✅ | `internal/agent/cache.go` |
| ArtifactService | ✅ | ✅ | `PersistenceSet` → `WithArtifactService` |
| AgentFactory | ✅ | ✅ | `BizAgentFactoryOptions` |
| SessionIngestor | ✅ | 🟡 | `BizSessionIngestor` 注入；外部 backend 待扩展 |
| AwaitUserReplyRouting（框架层） | ✅ | ✅ | `AwaitHook != nil` 时启用 |
| AgentLookup | ✅ | ✅ | `BizAgentRegistryOptions` + Team lookup map |
| RalphLoop | ✅ | ✅ | Ent + `RalphLoopConfigFromSettings` |
| 独立 CancelRun RPC | — | — | 非目标；沿用 StopGeneration |
| RunnerInstanceRegistry / RunnerManager | — | ✅ | `RunnerManager.NewTurnRunner`；每 turn 仍 `Close` |
| Web 运行状态 UI | — | 🟡 | API/类型层已就绪；`ChatRunnerStatus.vue`、`ChatEnqueueMessage.vue` 组件未创建 |

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

// 已实现（2026-05-19）— 以 chat.proto 为准
message EnqueueUserMessageRequest {
  string session_id = 1 [(google.api.field_behavior) = REQUIRED];
  string content = 2 [(google.api.field_behavior) = REQUIRED];
}

message EnqueueUserMessageResponse {
  bool accepted = 1;   // steerable enqueue 或 pending 入队成功
  bool queued = 2;     // true 表示落入 pendingQueue（非 steerable）
  string pending_id = 3;
}

service ChatService {
  rpc EnqueueUserMessage(EnqueueUserMessageRequest) returns (EnqueueUserMessageResponse) {
    option (google.api.http) = { post: "/v1/chat/enqueue" body: "*" };
  }
}

// 取消：沿用 StopGeneration（HTTP）与 WS cancel → ChatService.CancelRun → RunRegistry.Cancel
// 独立 CancelRun RPC 仍为规划项，非当前实现
```

### GetRunStatus Proto 对齐

`RunStatus` 消息已与 `ManagedRunner.RunStatus` 对齐，包含完整字段：

```protobuf
message RunStatus {
  string run_id = 1;
  string status = 2;
  string error_message = 3;
  string updated_at = 4;
  // 与 ManagedRunner.RunStatus 对齐
  string invocation_id = 5;
  string agent_name = 6;
  string started_at = 7;
  string last_event_at = 8;
  int32 event_count = 9;
  // AwaitUserReply 元数据
  string await_kind = 10;
  string await_tool_key = 11;
  string await_tool_call_id = 12;
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
field.String("ralph_loop_promise_tag_open").Default(""),
field.String("ralph_loop_promise_tag_close").Default(""),
field.String("ralph_loop_verify_work_dir").Default(""),
```

**校验与接线**（2026-05-21）：

- 持久化：`biz.ValidateRalphLoopSettings`（Create/Update Agent 与 Planner 同级）。
- 运行时映射：`internal/agent.RalphLoopConfigFromSettings`；Turn 统一 `ResolveRalphLoopTurn`（Chat / Team / A2A）。
- **Team**：Ralph 取自 **第一个成员 Agent** 的 `agent_runtime_settings`（领队/编排 Agent 配置生效）。
- 无效配置：保存拒绝；历史脏数据 Turn 时 FlowLog Warn 并跳过 Ralph。

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

运行控制已通过 `internal/runtime.RunRegistry`（active run、cancel、status）+ `RunnerManager`（统一装配）实现。`pendingQueue` 经 `PendingMessageQueue` 管理，`awaitChans` 在 `ChatOrchestrator` 中。无需再引入独立的 `RunnerUsecase`：

```go
// 运行控制入口（已实现）
type RunRegistry struct { ... }       // internal/runtime/run_registry.go
type RunnerManager struct { ... }     // internal/runtime/runner_manager.go
type PendingMessageQueue struct { ... } // internal/runtime/pending_queue.go
```

### 3.3 Runner 注册表

`RunRegistry` 已实现（`internal/runtime/run_registry.go`），基于 `sync.Map` 提供类型安全的并发访问：

```go
type RunRegistry struct {
    activeRuns     activeRunMap     // sessionID → activeRun
    pendingCancels cancelMap        // sessionID → context.CancelFunc
    runStatuses    statusMap        // sessionID → RunStatusEntry
}

func NewRunRegistry() *RunRegistry

func (r *RunRegistry) HasActive(sessionID string) bool
func (r *RunRegistry) StorePlaceholder(sessionID, runID string)
func (r *RunRegistry) StoreRunner(sessionID string, runner trpcrunner.Runner)
func (r *RunRegistry) StoreCancelable(sessionID string, cancel context.CancelFunc)
func (r *RunRegistry) Finish(sessionID string)
func (r *RunRegistry) Cancel(sessionID string) bool
func (r *RunRegistry) EnqueueUserMessage(sessionID, content string) bool
func (r *RunRegistry) SetStatus(sessionID string, entry RunStatusEntry)
func (r *RunRegistry) ActiveRunner(sessionID string) (trpcrunner.Runner, bool)
func (r *RunRegistry) GetStatus(sessionID string) (RunStatusEntry, bool)
func (r *RunRegistry) SetPendingCancel(sessionID string, cancel context.CancelFunc)
func (r *RunRegistry) ClearPendingCancel(sessionID string)
```

---

## 四、运行时层

### 4.1 TRPCRunnerDeps

`TRPCRunnerDeps` 已扩展为完整结构（`internal/agent/trpc_runtime.go`）：

```go
type TRPCRunnerDeps struct {
    AppName               string
    SessionService        trpcsession.Service
    MemoryService         trpcmemory.Service
    ArtifactService       trpcartifact.Service
    Plugins               []trpcplugin.Plugin
    Ingestor              trpcsession.Ingestor
    AwaitUserReplyRouting bool
    RalphLoop             *trpcrunner.RalphLoopConfig
    LG                    loggateway.Logger
}
```

### 4.2 NewTRPCRunner

`NewTRPCRunner` 已完整实现所有 option 注入（`internal/agent/trpc_runtime.go`）：

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
```

AgentFactory 通过 `RunnerManager.NewTurnRunner` 的 `TurnRunnerSpec.AgentFactoryKeys` 注入，不在 `TRPCRunnerDeps` 中。

### 4.3 AgentFactory 实现

`BizAgentFactoryOptions` 按 `agent_key` 注册工厂闭包（`internal/agent/factory.go`）：

```go
func BizAgentFactoryOptions(
    keys []string,
    deps TRPCBuilderDeps,
    lg loggateway.Logger,
) []trpcrunner.Option
```

工厂闭包逻辑：`resolveBizAgentByKey` 从数据库解析 Agent → `BuildTRPCAgentCached` 构建/缓存。查找顺序：已注册实例 → AgentFactory → 未找到。

### 4.4 SessionIngestor 实现

`BizSessionIngestor` 已实现（`internal/agent/ingestor.go`），当前记录摄入元数据，为外部 backend 预留扩展点：

```go
type BizSessionIngestor struct {
    memory trpcmemory.Service
    lg     loggateway.Logger
}

func NewBizSessionIngestor(
    memory trpcmemory.Service,
    lg loggateway.Logger,
) *BizSessionIngestor

func (ing *BizSessionIngestor) IngestSession(
    ctx context.Context,
    sess *trpcsession.Session,
) error
```

`IngestSession` 当前仅做日志记录。摄入失败不阻塞主流程。外部 Mem0 等 backend 待扩展。

### 4.5 AwaitUserReplyRouting（框架层）

当前项目通过 Service 层自行实现 AwaitUserReply（`serviceawaitreply.ServiceTool` + `ChatOrchestrator` await hook + `ChatService.AwaitUserReply` RPC），实现了 mid-turn 阻塞等待用户回复。

框架层的 `WithAwaitUserReplyRouting` 提供跨 turn 的路由能力：Agent 调用 `await_user_reply` 后在 Session 状态中记录路由，下一轮用户消息自动路由到该 Agent。

两层机制互补：
- **Service 层**（已实现）：mid-turn 阻塞，当前 turn 内等待用户回复
- **框架层**（已启用）：跨 turn 路由，下一轮用户消息自动路由到指定 Agent

启用方式：`RunnerManager.NewTurnRunner` 中 `TurnRunnerSpec.AwaitUserReplyRouting` 为 true 时（即 `AwaitHook != nil`），传入 `WithAwaitUserReplyRouting(true)`。

### 4.6 RalphLoop 配置

`RalphLoopConfig` 从 `AgentRuntimeSettings` 数据库配置映射（`internal/agent/ralph_loop.go`）：

```go
func RalphLoopConfigFromSettings(settings *biz.AgentRuntimeSettings) *trpcrunner.RalphLoopConfig
func ResolveRalphLoopTurn(settings *biz.AgentRuntimeSettings) RalphLoopTurnResult
```

映射规则：
- `RalphLoopMaxIterations > 0` 时启用
- `CompletionPromise` → `PromiseTagOpen`/`PromiseTagClose` 默认 `<promise>`/`</promise>`
- `VerifyCommand` → `VerifyCommand` + `VerifyWorkDir` + `VerifyTimeout`
- 无效配置：`RalphLoopTurnResult.SkipErr` 非空时跳过并记录 Warn

### 4.7 RunnerManager

`RunnerManager` 已实现（`internal/runtime/runner_manager.go`），统一 Runner 装配入口：

```go
type RunnerFactoryDeps struct {
    Persist PersistenceSet
}

type TurnRunnerSpec struct {
    Plugins               []trpcplugin.Plugin
    AwaitUserReplyRouting bool
    BuilderDeps           TRPCBuilderDeps
    AgentFactoryKeys      []string
    LookupAgents          map[string]trpcagent.Agent
    RalphLoop             *trpcrunner.RalphLoopConfig
    ExtraOpts             []trpcrunner.Option
    RegistryKey           string  // 非空时支持长生命周期 Runner
}

type RunnerManager struct {
    factory  RunnerFactoryDeps
    registry *RunRegistry
    lg       loggateway.Logger
}

func NewRunnerManager(deps RunnerFactoryDeps, registry *RunRegistry, lg loggateway.Logger) *RunnerManager

func (m *RunnerManager) Registry() *RunRegistry
func (m *RunnerManager) NewTurnRunner(ctx context.Context, root trpcagent.Agent, spec TurnRunnerSpec) (trpcrunner.Runner, error)
func (m *RunnerManager) CloseRunner(key string) error
```

`NewTurnRunner` 逻辑：构建 `TRPCRunnerDeps` → 注册 AgentFactory → 注册 LookupAgents → `NewTRPCRunner` → `registry.StoreRunner`。每 turn 默认 `Close`；`RegistryKey` 非空时支持长生命周期实例。

---

## 五、Data 层

无需新增独立数据表。Runner 运行时状态通过 `runHandle` 内存管理。

### AgentRuntimeSettings Ent Schema 扩展

见 §二 AgentRuntimeSettings 扩展。

---

## 六、Service 层

### 6.1 ChatService 与 Biz 分工

| 组件 | 职责 |
|------|------|
| `runtime.RunRegistry` | 每 session active runner、cancel、steerable enqueue、服务层 status |
| `runtime.RunnerManager` | 统一 `NewTRPCRunner` 装配（Session/Memory/Artifact/Plugins/Factory/Routing） |
| `runtime.PendingMessageQueue` | FIFO 待执行队列（快照持久化、followup 合并） |
| `biz.ChatUsecase` | 会话锁、Enqueue 编排（steerable 失败则入队） |
| `ChatOrchestrator` | turn 执行（`runSingleAgentViaTRPC`）、`RunGateway`、pending 处理 |
| `ChatService` | RPC/WS 桥接、`setRunStatus`（含 webhook/await meta） |

`ChatService` 不再持有 `activeRuns`/`pendingQueue` sync.Map；运行控制经 Wire 注入的 `RunRegistry` 与 `PendingMessageQueue`。

### 6.2 ChatService RPC 实现

已实现的 RPC 方法：

```go
func (s *ChatService) StopGeneration(ctx context.Context, req *chatv1.StopGenerationRequest) (*chatv1.StopGenerationResponse, error)
    // → orch.CancelRun(sessionID)

func (s *ChatService) EnqueueUserMessage(ctx context.Context, req *chatv1.EnqueueUserMessageRequest) (*chatv1.EnqueueUserMessageResponse, error)
    // → orch.EnqueueUserMessage(sessionID, content)

func (s *ChatService) GetRunStatus(ctx context.Context, req *chatv1.GetRunStatusRequest) (*chatv1.GetRunStatusResponse, error)
    // → RunRegistry.GetStatus + 运行中合并 ManagedRunner.RunStatus + awaiting_user 元数据

func (s *ChatService) AwaitUserReply(ctx context.Context, req *chatv1.AwaitUserReplyRequest) (*chatv1.AwaitUserReplyResponse, error)
    // → 重启恢复逻辑
```

取消运行沿用 `StopGeneration`（HTTP）与 WS `cancel`，不设独立 `CancelRun` RPC。

### 6.3 GetRunStatus 对齐

`GetRunStatus` 已实现双源合并：从 `RunRegistry.GetStatus` 读取服务层状态，运行中时额外查询 `ManagedRunner.RunStatus` 获取框架层完整信息（invocation_id、agent_name、event_count、await_kind 等）。

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
| `web/src/features/chat/api.ts` | ✅ | 已有 `getRunStatus`、`stopGeneration`、`enqueueMessage`、`awaitUserReply` |
| `web/src/domain/types.ts` | ✅ | `RunStatus` 接口含完整字段（含 awaitKind/awaitToolKey/awaitToolCallId） |
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
    ├── ChatRunnerStatus.vue    ← 运行状态指示器（待创建）
    └── ChatEnqueueMessage.vue  ← 运行中追加消息（待创建）
```

### 8.2 ChatRunnerStatus.vue

| 区域 | 组件 | 说明 |
|------|------|------|
| 状态标签 | `QBadge` | `running` / `completed` / `cancelled` / `awaiting_user` |
| Agent 名称 | `QChip` | 当前运行的 Agent |
| 运行时长 | `span` | 从 startedAt 计算已运行时间 |
| 事件数 | `span` | 已处理的事件数量 |
| 取消按钮 | `QBtn` | 点击调用 StopGeneration API |

### 8.3 ChatEnqueueMessage.vue

| 区域 | 组件 | 说明 |
|------|------|------|
| 输入框 | `QInput` | 追加消息输入 |
| 发送按钮 | `QBtn` | 点击调用 EnqueueUserMessage API |
| 状态提示 | `QTooltip` | 消息将在工具调用边界后注入 |

### 8.4 API 接口

`api.ts` 已实现：

```typescript
export async function enqueueMessage(sessionId: string, content: string): Promise<EnqueueMessageResult>
export async function getRunStatus(sessionId: string): Promise<RunStatus | null>
export async function stopGeneration(sessionId: string): Promise<void>
export async function awaitUserReply(sessionId: string, toolCallId: string, content: string): Promise<void>
```

注意：前端使用 `stopGeneration` 而非 `cancelRun`；`enqueueUserMessage` 已废弃，委托给 `enqueueMessage`。

### 8.5 RunStatus 类型

`web/src/domain/types.ts` 已定义完整类型：

```typescript
export type RunStatusValue = 'idle' | 'pending' | 'running' | 'awaiting_user' | 'sync' | 'completed' | 'failed' | 'cancelled'

export interface RunStatus {
  runId: string;
  status: RunStatusValue;
  errorMessage: string;
  updatedAt: string;
  invocationId?: string;
  agentName?: string;
  startedAt?: string;
  lastEventAt?: string;
  eventCount?: number;
  awaitKind?: string;
  awaitToolKey?: string;
  awaitToolCallId?: string;
}
```
