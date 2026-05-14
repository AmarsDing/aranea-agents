# Runner 运行器模块 — 实现设计文档

> 对应需求：`40 runner.md`
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Agent 运行器完善：AgentFactory、PluginManager、ArtifactService、SessionIngestor、AwaitUserReplyRouting、ManagedRunner（Status/Cancel）、SteerableRunner（EnqueueUserMessage）、AgentLookup、RalphLoop。对标 trpc-agent-go `runner` 包，将项目从基础 Runner 升级为完整的 ManagedRunner + SteerableRunner。

### 核心架构

```
用户消息 → Runner.Run(userID, sessionID, message)
             ↓
         getOrCreateSession() → 加载/创建会话
             ↓
         applyAwaitUserReplyRoute() → 检查待路由的用户回复
             ↓
         selectAgent() → 从注册表/工厂选择 Agent
             ↓
         resolveCurrentTurnMessages() → 解析当前轮消息
             ↓
         agent.RunWithPlugins() → 执行 Agent
             ↓
         processAgentEvents() → 事件循环：持久化 + 转发 + 状态更新
             ↓
         sessionIngestor.IngestSession() → 会话完成后摄入外部记忆
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
| Runner.Run | ✅ | ✅ | 已有 |
| Runner.Close | ✅ | ✅ | 已有 |
| ManagedRunner.Cancel | ✅ | ❌ | 缺失 |
| ManagedRunner.RunStatus | ✅ | ❌ | 缺失 |
| SteerableRunner.EnqueueUserMessage | ✅ | ❌ | 缺失 |
| AgentFactory | ✅ | ❌ | 缺失 |
| PluginManager | ✅ | ❌ | 缺失 |
| ArtifactService | ✅ | ❌ | 缺失 |
| SessionIngestor | ✅ | ❌ | 缺失 |
| AwaitUserReplyRouting | ✅ | ❌ | 缺失 |
| AgentLookup | ✅ | ❌ | 缺失 |
| RalphLoop | ✅ | ❌ | 缺失 |
| 多 Runner 实例 | ✅ | ❌ | 缺失 |

---

## 二、Proto 层

无需独立 Proto 服务。通过 Chat Service 和 Gateway 暴露 Runner 控制接口。

### Chat Proto 扩展

```protobuf
// api/kratos/chat/v1/chat.proto — Runner 控制接口扩展

message RunStatusRequest {
  string session_id = 1;
  string request_id = 2;
}

message RunStatusResponse {
  string request_id = 1;
  string invocation_id = 2;
  string agent_name = 3;
  string session_id = 4;
  string started_at = 5;
  string last_event_at = 6;
  int32 event_count = 7;
  bool running = 8;
}

message CancelRunRequest {
  string session_id = 1;
  string request_id = 2;
}

message CancelRunResponse {
  bool cancelled = 1;
}

message EnqueueUserMessageRequest {
  string session_id = 1;
  string request_id = 2;
  string content = 3;
}

message EnqueueUserMessageResponse {
  bool enqueued = 1;
}

service ChatService {
  // ... 已有方法 ...

  rpc GetRunStatus(RunStatusRequest) returns (RunStatusResponse) {
    option (google.api.http) = { get: "/v1/chat/run-status" };
  }
  rpc CancelRun(CancelRunRequest) returns (CancelRunResponse) {
    option (google.api.http) = { post: "/v1/chat/cancel-run" body: "*" };
  }
  rpc EnqueueUserMessage(EnqueueUserMessageRequest) returns (EnqueueUserMessageResponse) {
    option (google.api.http) = { post: "/v1/chat/enqueue-user-message" body: "*" };
  }
}
```

### Agent Proto 扩展

```protobuf
// api/kratos/agent/v1/agent.proto — AgentRuntimeSettings 消息扩展

message AgentRuntimeSettings {
  // ... 已有字段 ...

  // Runner 配置
  bool runner_await_user_reply_routing = 60;
  int32 runner_max_run_duration_seconds = 61;
  bool runner_detached_cancel = 62;

  // RalphLoop 配置
  int32 ralph_loop_max_iterations = 70;
  string ralph_loop_completion_promise = 71;
  string ralph_loop_verify_command = 72;
  int32 ralph_loop_verify_timeout_seconds = 73;
}
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
    MaxIterations    int
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

### 3.2 Usecase

```go
type RunnerUsecase struct {
    agents    AgentRepository
    agentsUC  *AgentUsecase
    sessions  *SessionUsecase
    catalog   *LlmProviderModelUsecase
    broker    *TeamRunEventBroker
}

func NewRunnerUsecase(
    agents AgentRepository,
    agentsUC *AgentUsecase,
    sessions *SessionUsecase,
    catalog *LlmProviderModelUsecase,
    broker *TeamRunEventBroker,
) *RunnerUsecase

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

```go
type TRPCRunnerDeps struct {
    AppName        string
    SessionService trpcsession.Service
    MemoryService  trpcmemory.Service

    Ingestor              trpcsession.Ingestor
    ArtifactService       trpcartifact.Service
    AwaitUserReplyRouting bool
    Plugins               []trpcplugin.Plugin
    AgentFactories        map[string]trpcrunner.AgentFactory
    RalphLoop             *trpcrunner.RalphLoopConfig
}
```

### 4.2 NewTRPCRunner 扩展

```go
func NewTRPCRunner(root trpcagent.Agent, deps TRPCRunnerDeps, opts ...trpcrunner.Option) (trpcrunner.Runner, error) {
    if root == nil {
        return nil, errors.New("trpc runtime: root agent is nil")
    }
    appName := strings.TrimSpace(deps.AppName)
    if appName == "" {
        appName = TRPCDefaultAppName
    }

    if deps.SessionService != nil {
        opts = append([]trpcrunner.Option{trpcrunner.WithSessionService(deps.SessionService)}, opts...)
    }
    if deps.MemoryService != nil {
        opts = append([]trpcrunner.Option{trpcrunner.WithMemoryService(deps.MemoryService)}, opts...)
    }
    if deps.Ingestor != nil {
        opts = append([]trpcrunner.Option{trpcrunner.WithSessionIngestor(deps.Ingestor)}, opts...)
    }
    if deps.ArtifactService != nil {
        opts = append([]trpcrunner.Option{trpcrunner.WithArtifactService(deps.ArtifactService)}, opts...)
    }
    if deps.AwaitUserReplyRouting {
        opts = append(opts, trpcrunner.WithAwaitUserReplyRouting(true))
    }
    if len(deps.Plugins) > 0 {
        opts = append(opts, trpcrunner.WithPlugins(deps.Plugins...))
    }
    if deps.RalphLoop != nil {
        opts = append(opts, trpcrunner.WithRalphLoop(*deps.RalphLoop))
    }
    for name, factory := range deps.AgentFactories {
        opts = append(opts, trpcrunner.WithAgentFactory(name, factory))
    }

    return trpcrunner.NewRunner(appName, root, opts...), nil
}
```

### 4.3 AgentFactory 实现

```go
type BizAgentFactory struct {
    agents  biz.AgentRepository
    agentsUC *biz.AgentUsecase
    deps    TRPCBuilderDeps
}

func NewBizAgentFactory(
    agents biz.AgentRepository,
    agentsUC *biz.AgentUsecase,
    deps TRPCBuilderDeps,
) *BizAgentFactory

func (f *BizAgentFactory) Create(
    ctx context.Context,
    ro trpcagent.RunOptions,
) (trpcagent.Agent, error) {
    agentName := ro.AgentByName
    if agentName == "" {
        return nil, fmt.Errorf("agent factory: agent name is empty")
    }
    ag, err := f.agentsUC.Get(ctx, agentName)
    if err != nil {
        return nil, fmt.Errorf("agent factory: lookup %q: %w", agentName, err)
    }
    return BuildTRPCLLMAgent(ctx, ag, f.deps)
}
```

### 4.4 SessionIngestor 实现

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
) error {
    if ing.memory == nil {
        return nil
    }
    events := sess.GetEvents()
    if len(events) == 0 {
        return nil
    }
    var transcript strings.Builder
    for _, evt := range events {
        if evt.Response == nil {
            continue
        }
        for _, ch := range evt.Response.Choices {
            if ch.Message.Content != "" {
                fmt.Fprintf(&transcript, "%s: %s\n", ch.Message.Role, ch.Message.Content)
            }
        }
    }
    if transcript.Len() == 0 {
        return nil
    }
    key := sess.Key()
    return ing.memory.Add(ctx, key.UserID, transcript.String(), map[string]string{
        "session_id": key.SessionID,
        "app_name":   key.AppName,
    })
}
```

### 4.5 AwaitUserReplyRouting 实现

```go
func applyAwaitUserReplyRoute(
    ctx context.Context,
    key trpcsession.Key,
    sess *trpcsession.Session,
    message trpcmodel.Message,
    ro trpcagent.RunOptions,
) (trpcagent.RunOptions, string, error) {
    if message.Role != trpcmodel.RoleUser {
        return ro, "", nil
    }
    if ro.Agent != nil || ro.AgentByName != "" {
        return ro, "", nil
    }
    route, ok, err := trpcagent.PendingAwaitUserReplyRoute(sess)
    if err != nil || !ok {
        return ro, "", nil
    }
    selected, rootName, ok, err := resolveAwaitUserReplyRoute(ctx, route, ro)
    if err != nil || !ok {
        return ro, "", nil
    }
    if err := clearAwaitUserReplyRoute(ctx, key, sess); err != nil {
        return ro, "", fmt.Errorf("consume await_user_reply route: %w", err)
    }
    ro.Agent = selected
    return ro, rootName, nil
}
```

### 4.6 ManagedRunner 扩展

```go
type ManagedTRPCRunner struct {
    trpcrunner.Runner
    registry *RunnerRegistry
}

func NewManagedTRPCRunner(root trpcagent.Agent, deps TRPCRunnerDeps, registry *RunnerRegistry) (*ManagedTRPCRunner, error) {
    r, err := NewTRPCRunner(root, deps)
    if err != nil {
        return nil, err
    }
    m := &ManagedTRPCRunner{Runner: r, registry: registry}
    return m, nil
}

func (m *ManagedTRPCRunner) Cancel(requestID string) bool {
    if mr, ok := m.Runner.(trpcrunner.ManagedRunner); ok {
        return mr.Cancel(requestID)
    }
    return false
}

func (m *ManagedTRPCRunner) RunStatus(requestID string) (trpcrunner.RunStatus, bool) {
    if mr, ok := m.Runner.(trpcrunner.ManagedRunner); ok {
        return mr.RunStatus(requestID)
    }
    return trpcrunner.RunStatus{}, false
}

func (m *ManagedTRPCRunner) EnqueueUserMessage(requestID string, message trpcmodel.Message) error {
    if sr, ok := m.Runner.(trpcrunner.SteerableRunner); ok {
        return sr.EnqueueUserMessage(requestID, message)
    }
    return trpcrunner.ErrQueuedUserMessageUnsupported
}
```

### 4.7 RalphLoop 实现

```go
func ralphLoopConfigFromSettings(settings *biz.AgentRuntimeSettings) *trpcrunner.RalphLoopConfig {
    if settings == nil || settings.RalphLoopMaxIterations <= 0 {
        return nil
    }
    cfg := &trpcrunner.RalphLoopConfig{
        MaxIterations:    settings.RalphLoopMaxIterations,
        CompletionPromise: settings.RalphLoopCompletionPromise,
    }
    if cfg.CompletionPromise != "" {
        cfg.PromiseTagOpen = "<promise>"
        cfg.PromiseTagClose = "</promise>"
    }
    if settings.RalphLoopVerifyCommand != "" {
        cfg.VerifyCommand = settings.RalphLoopVerifyCommand
        cfg.VerifyTimeout = time.Duration(settings.RalphLoopVerifyTimeoutSeconds) * time.Second
    }
    return cfg
}
```

### 4.8 多 Runner 实例管理

```go
type RunnerManager struct {
    registry *RunnerRegistry
    deps     RunnerFactoryDeps
}

type RunnerFactoryDeps struct {
    SessionSvc  trpcsession.Service
    MemorySvc   trpcmemory.Service
    Ingestor    trpcsession.Ingestor
    ArtifactSvc trpcartifact.Service
    Plugins     []trpcplugin.Plugin
}

func NewRunnerManager(deps RunnerFactoryDeps) *RunnerManager

func (m *RunnerManager) CreateRunner(
    ctx context.Context,
    key string,
    root trpcagent.Agent,
    opts ...trpcrunner.Option,
) (trpcrunner.Runner, error) {
    deps := TRPCRunnerDeps{
        AppName:        key,
        SessionService: m.deps.SessionSvc,
        MemoryService:  m.deps.MemorySvc,
        Ingestor:       m.deps.Ingestor,
        ArtifactService: m.deps.ArtifactSvc,
        Plugins:        m.deps.Plugins,
    }
    r, err := NewTRPCRunner(root, deps, opts...)
    if err != nil {
        return nil, err
    }
    m.registry.Register(key, r)
    return r, nil
}

func (m *RunnerManager) CloseRunner(key string) error {
    r, ok := m.registry.Get(key)
    if !ok {
        return nil
    }
    m.registry.Unregister(key)
    return r.Close()
}
```

---

## 五、Data 层

无需新增独立数据表。Runner 运行时状态通过 `runHandle` 内存管理。

### AgentRuntimeSettings Ent Schema 扩展

```go
// internal/data/ent/schema/agent_runtime_setting.go — 新增字段

func (AgentRuntimeSetting) Fields() []ent.Field {
    return []ent.Field{
        // ... 已有字段 ...

        field.Bool("runner_await_user_reply_routing").Default(false),
        field.Int("runner_max_run_duration_seconds").Default(0),
        field.Bool("runner_detached_cancel").Default(false),
        field.Int("ralph_loop_max_iterations").Default(0),
        field.String("ralph_loop_completion_promise").Default(""),
        field.String("ralph_loop_verify_command").Default(""),
        field.Int("ralph_loop_verify_timeout_seconds").Default(0),
    }
}
```

---

## 六、Service 层

### 6.1 ChatService 扩展

```go
func (s *ChatService) GetRunStatus(ctx context.Context, req *chatv1.RunStatusRequest) (*chatv1.RunStatusResponse, error) {
    info, err := s.runnerUC.GetRunStatus(ctx, req.GetSessionId(), req.GetRequestId())
    if err != nil {
        return nil, err
    }
    if info == nil {
        return &chatv1.RunStatusResponse{Running: false}, nil
    }
    return &chatv1.RunStatusResponse{
        RequestId:    info.RequestID,
        InvocationId: info.InvocationID,
        AgentName:    info.AgentName,
        SessionId:    info.SessionID,
        StartedAt:    info.StartedAt,
        LastEventAt:  info.LastEventAt,
        EventCount:   int32(info.EventCount),
        Running:      true,
    }, nil
}

func (s *ChatService) CancelRun(ctx context.Context, req *chatv1.CancelRunRequest) (*chatv1.CancelRunResponse, error) {
    cancelled, err := s.runnerUC.CancelRun(ctx, req.GetSessionId(), req.GetRequestId())
    if err != nil {
        return nil, err
    }
    return &chatv1.CancelRunResponse{Cancelled: cancelled}, nil
}

func (s *ChatService) EnqueueUserMessage(ctx context.Context, req *chatv1.EnqueueUserMessageRequest) (*chatv1.EnqueueUserMessageResponse, error) {
    err := s.runnerUC.EnqueueUserMessage(ctx, req.GetSessionId(), req.GetRequestId(), req.GetContent())
    if err != nil {
        return nil, err
    }
    return &chatv1.EnqueueUserMessageResponse{Enqueued: true}, nil
}
```

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
    NewManagedTRPCRunner,
)

// internal/service/wire.go — ChatServiceDeps 扩展
type ChatServiceDeps struct {
    // ... 已有 ...
    RunnerUC *biz.RunnerUsecase
}
```

---

## 八、Web 前端设计

Runner 为运行时基础设施，无独立前端页面。相关 UI 通过 Chat 和 Gateway 页面暴露。

### 8.1 Chat 页面集成

**运行状态指示器**：

| 元素 | 组件 | 说明 |
|------|------|------|
| 运行状态 | `QBadge` | 在聊天输入框旁显示当前运行状态 |
| 取消按钮 | `QBtn` | 运行中时显示取消按钮 |
| 排队消息 | `QInput` | 运行中时允许用户追加消息 |

**文件结构**：

```
web/src/features/chat/
└── components/
    ├── ChatRunnerStatus.vue    ← 运行状态指示器
    └── ChatEnqueueMessage.vue  ← 运行中追加消息
```

### 8.2 ChatRunnerStatus.vue

```typescript
interface Props {
  sessionId: string
  requestId: string
}

interface RunStatus {
  requestId: string
  agentName: string
  startedAt: string
  lastEventAt: string
  eventCount: number
  running: boolean
}

async function fetchRunStatus(sessionId: string, requestId: string): Promise<RunStatus>
async function cancelRun(sessionId: string, requestId: string): Promise<boolean>
```

| 区域 | 组件 | 说明 |
|------|------|------|
| 状态标签 | `QBadge` | `running` / `completed` / `cancelled` |
| Agent 名称 | `QChip` | 当前运行的 Agent |
| 运行时长 | `span` | 从 startedAt 计算已运行时间 |
| 事件数 | `span` | 已处理的事件数量 |
| 取消按钮 | `QBtn` | 点击调用 CancelRun API |

### 8.3 ChatEnqueueMessage.vue

```typescript
interface Props {
  sessionId: string
  requestId: string
  disabled: boolean
}

async function enqueueUserMessage(sessionId: string, requestId: string, content: string): Promise<boolean>
```

| 区域 | 组件 | 说明 |
|------|------|------|
| 输入框 | `QInput` | 追加消息输入 |
| 发送按钮 | `QBtn` | 点击调用 EnqueueUserMessage API |
| 状态提示 | `QTooltip` | 消息将在工具调用边界后注入 |

### 8.4 API 接口

```typescript
// web/src/api/runner.ts

export async function getRunStatus(sessionId: string, requestId: string): Promise<RunStatusResponse> {
  const { data } = await axios.get('/v1/chat/run-status', { params: { session_id: sessionId, request_id: requestId } })
  return data
}

export async function cancelRun(sessionId: string, requestId: string): Promise<CancelRunResponse> {
  const { data } = await axios.post('/v1/chat/cancel-run', { session_id: sessionId, request_id: requestId })
  return data
}

export async function enqueueUserMessage(sessionId: string, requestId: string, content: string): Promise<EnqueueUserMessageResponse> {
  const { data } = await axios.post('/v1/chat/enqueue-user-message', { session_id: sessionId, request_id: requestId, content })
  return data
}
```

### 8.5 SSE 事件扩展

Runner 运行状态通过 SSE 推送：

```typescript
interface RunnerSSEEvent {
  type: 'run_started' | 'run_completed' | 'run_cancelled'
  request_id: string
  agent_name: string
  started_at: string
  duration_ms: number
  event_count: number
}
```

---

## 九、实现计划

| 阶段 | 内容 | 涉及文件 |
|------|------|---------|
| P1 | ManagedRunner + Cancel/RunStatus | `internal/agent/trpc_runtime.go`, `api/kratos/chat/v1/chat.proto` |
| P2 | AgentFactory + AgentLookup | `internal/agent/factory.go`（新建）, `internal/agent/lookup.go`（新建） |
| P3 | SessionIngestor | `internal/agent/ingestor.go`（新建） |
| P4 | AwaitUserReplyRouting | `internal/agent/await_user_reply.go`（新建） |
| P5 | PluginManager 集成 | `internal/agent/trpc_runtime.go` |
| P6 | ArtifactService 集成 | `internal/agent/trpc_runtime.go` |
| P7 | SteerableRunner + EnqueueUserMessage | `internal/agent/trpc_runtime.go` |
| P8 | RalphLoop | `internal/agent/ralph_loop.go`（新建） |
| P9 | 多 Runner 实例 + RunnerManager | `internal/agent/runner_manager.go`（新建） |
| P10 | Web 前端 ChatRunnerStatus + ChatEnqueueMessage | `web/src/features/chat/components/` |

---

## 十、验收标准

1. Runner 可按名称动态创建 Agent（AgentFactory）
2. Runner 执行中回调正确触发（PluginManager）
3. Agent 可通过 ArtifactService 管理制品
4. Session 完成后自动摄入到外部记忆平台（SessionIngestor）
5. Agent 可指定下一轮用户消息路由（AwaitUserReplyRouting）
6. 可通过 API 查询运行状态和取消运行（ManagedRunner）
7. 运行中可追加用户消息（SteerableRunner）
8. TransferTool 可通过名称查找目标 Agent（AgentLookup）
9. 多个 Runner 实例可并行运行（RunnerManager）
10. RalphLoop 支持迭代执行和验证（RalphLoop）
11. Web 前端显示运行状态、支持取消和追加消息
