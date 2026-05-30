# SubAgent 后台派生

## 一、需求文档

### 1.1 背景

当前 Agent 执行任务时是单线程的——一个 Agent 一次只能处理一个任务。对于复杂任务（如"调研 5 个竞品并生成对比报告"），Agent 需要串行执行，效率低下。

框架 `pkg/trpc-agent-go/openclaw/internal/subagentrun/` 提供了完整的后台子 Agent 派生参考实现：
- `Service`：管理子 Agent 生命周期（Spawn/List/Get/Cancel）
- `Tools`：4 个工具（spawn/list/get/cancel），让 Agent 可动态派生子 Agent
- `Store`：JSON 文件持久化运行记录

项目 `internal/agent/factory.go` 已有 `BizAgentFactoryOptions`，支持通过 `runner.WithAgentFactory()` 动态注册 Agent 工厂。但缺少：
- 后台子 Agent 的生命周期管理服务
- 让 Agent 可派生子 Agent 的工具
- 子 Agent 完成后的通知机制

### 1.2 目标

1. 实现后台子 Agent 派生服务，Agent 可通过工具动态创建子 Agent
2. 子 Agent 在独立 Session 中运行，不阻塞父 Agent
3. 子 Agent 完成后自动通知父 Agent（通过事件/渠道）
4. 支持子 Agent 的超时控制和取消

### 1.3 功能需求

| # | 功能 | 优先级 | 说明 |
|---|------|--------|------|
| F1 | SubAgentService 生命周期管理 | P0 | Spawn/List/Get/Cancel 四个操作 |
| F2 | SubAgent 工具集 | P0 | spawn/list/get/cancel 4 个工具注册到 Registry |
| F3 | 子 Agent 独立 Session | P0 | 每个子 Agent 在独立 Session 中运行 |
| F4 | 超时控制 | P0 | 支持 timeout_seconds 配置，超时自动取消 |
| F5 | 完成通知 | P1 | 子 Agent 完成后通过 OutboundRouter 通知（依赖模块 5） |
| F6 | 运行记录持久化 | P1 | 子 Agent 运行记录存入 SQLite（Ent ORM） |
| F7 | 嵌套限制 | P0 | 禁止子 Agent 再派生子 Agent（防止无限递归） |
| F8 | 并发限制 | P2 | 限制同一用户同时运行的子 Agent 数量 |

### 1.4 非功能需求

- 子 Agent Spawn 延迟 < 500ms（不含 LLM 首次响应）
- 运行记录持久化不阻塞主流程
- 子 Agent 运行使用 `pkg/safego.Go` 防止 panic 扩散
- 日志统一使用 `internal/event FlowLog`

### 1.5 验收标准

1. Agent 调用 `subagents_spawn` 工具后能创建后台子 Agent
2. 子 Agent 在独立 Session 中运行，父 Agent 不被阻塞
3. Agent 调用 `subagents_list` 能查看当前 Session 的子 Agent 列表
4. Agent 调用 `subagents_get` 能获取子 Agent 的状态和结果
5. Agent 调用 `subagents_cancel` 能取消运行中的子 Agent
6. 子 Agent 内调用 `subagents_spawn` 返回错误（嵌套限制）

---

## 二、设计文档

### 2.1 框架参考（trpc-agent-go / OpenClaw）

#### SubAgent Service

**`subagentrun.Service`** — `pkg/trpc-agent-go/openclaw/internal/subagentrun/service.go`

```go
type Service struct {
    path   string
    runner runner.Runner
    router *outbound.Router
    clock  func() time.Time
    mu      sync.Mutex
    runs    map[string]*runRecord
    running map[string]*runningRun
    persistMu sync.Mutex
    startOnce sync.Once
    baseCtx   context.Context
    cancel    context.CancelFunc
    wg        sync.WaitGroup
}

func NewService(
    stateDir string,
    r runner.Runner,
    router *outbound.Router,
) (*Service, error)

func (s *Service) Start(ctx context.Context)
func (s *Service) Close() error
func (s *Service) Spawn(ctx context.Context, req SpawnRequest) (publicsubagent.Run, error)
func (s *Service) ListForUser(userID string, filter publicsubagent.ListFilter) []publicsubagent.Run
func (s *Service) GetForUser(userID string, runID string) (*publicsubagent.Run, error)
func (s *Service) CancelForUser(userID string, runID string) (*publicsubagent.Run, bool, error)
```

关键实现：
- `Spawn()` 创建 `runRecord`，通过 `s.wg.Add(1)` + `go s.execute()` 异步执行
- `execute()` 调用 `s.runChild()` → `s.runner.Run()` 执行子 Agent
- `runChild()` 通过 `agent.WithRuntimeState()` 注入子 Agent 标记
- `finishRun()` 更新状态，`notifyCompletion()` 通过 OutboundRouter 发送通知

**`subagentrun.SpawnRequest`** — `pkg/trpc-agent-go/openclaw/internal/subagentrun/types.go`

```go
type SpawnRequest struct {
    OwnerUserID     string
    ParentSessionID string
    Task            string
    TimeoutSeconds  int
    Delivery        deliveryTarget
}
```

**`subagent.Run`（公共类型）** — `pkg/trpc-agent-go/openclaw/subagent/subagent.go`

```go
type Run struct {
    ID              string     `json:"id,omitempty"`
    ParentSessionID string     `json:"parent_session_id,omitempty"`
    ChildSessionID  string     `json:"child_session_id,omitempty"`
    Task            string     `json:"task,omitempty"`
    Status          Status     `json:"status,omitempty"`
    Summary         string     `json:"summary,omitempty"`
    Result          string     `json:"result,omitempty"`
    Error           string     `json:"error,omitempty"`
    CreatedAt       time.Time  `json:"created_at"`
    UpdatedAt       time.Time  `json:"updated_at"`
    StartedAt       *time.Time `json:"started_at,omitempty"`
    FinishedAt      *time.Time `json:"finished_at,omitempty"`
}

type Status string

const (
    StatusQueued    Status = "queued"
    StatusRunning   Status = "running"
    StatusCompleted Status = "completed"
    StatusFailed    Status = "failed"
    StatusCanceled  Status = "canceled"
)

type Service interface {
    ListForUser(userID string, filter ListFilter) []Run
    GetForUser(userID string, runID string) (*Run, error)
    CancelForUser(userID string, runID string) (*Run, bool, error)
}
```

#### SubAgent Tools

**`subagentrun.Tools`** — `pkg/trpc-agent-go/openclaw/internal/subagentrun/tool.go`

```go
type Tools struct {
    spawn       *spawnTool
    list        *listTool
    get         *getTool
    cancel      *cancelTool
    spawnAlias  *spawnTool
    listAlias   *listTool
    getAlias    *getTool
    cancelAlias *cancelTool
}

func NewTools(svc *Service) Tools
func (t *Tools) All() []tool.Tool
```

4 个工具：
- `subagents_spawn`：创建后台子 Agent，输入 `{task, timeout_seconds}`
- `subagents_list`：列出当前 Session 的子 Agent
- `subagents_get`：获取子 Agent 状态和结果，输入 `{id}`
- `subagents_cancel`：取消子 Agent，输入 `{id}`

嵌套检测：

```go
func isNestedSubagent(ctx context.Context) bool {
    nested, ok := agent.GetRuntimeStateValueFromContext[bool](
        ctx,
        runtimeStateSubagentRun,
    )
    return ok && nested
}
```

#### Runner AgentFactory

**`runner.AgentFactory`** — `pkg/trpc-agent-go/runner/runner.go`

```go
type AgentFactory func(
    ctx context.Context,
    ro agent.RunOptions,
) (agent.Agent, error)

func WithAgentFactory(name string, factory AgentFactory) Option
```

Runner 在 `selectAgent()` 中通过 `loadRegisteredAgent()` 解析 AgentFactory。

### 2.2 当前项目现状

| 文件 | 现状 |
|------|------|
| `internal/agent/factory.go` | 已有 `BizAgentFactoryOptions`，支持 AgentFactory 动态注册 |
| `internal/agent/trpc_build.go` | Agent 构建入口 |
| `internal/tools/toolset.go` Registry | 无 subagent 相关注册项 |
| `internal/tools/toolset.go` AssemblyConfig | 无 SubAgent 配置 |

**差距**：
1. 无 SubAgentService 实现
2. 无 SubAgent 工具集
3. 无子 Agent 运行记录持久化
4. 无嵌套限制检测

### 2.3 架构设计

#### 模块在四层架构中的位置

```
internal/service     ← Runner 装配 + SubAgentService 初始化
internal/agent       ← 已有 factory.go，无需改动
internal/tools       ← 新增 subagent 注册 + 工具构建
  subagent/          ← 新增：SubAgent 工具实现
internal/biz         ← 新增 SubAgentRun 领域模型 + Repo 接口
internal/data        ← 新增 SubAgentRun Repo 实现（Ent ORM）
```

#### 新增/修改的文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/tools/subagent/service.go` | 新增 | SubAgentService：Spawn/List/Get/Cancel |
| `internal/tools/subagent/tools.go` | 新增 | 4 个工具：spawn/list/get/cancel |
| `internal/tools/subagent/types.go` | 新增 | 类型定义 |
| `internal/tools/subagent/config.go` | 新增 | 配置类型 |
| `internal/biz/subagent_run.go` | 新增 | SubAgentRun 领域模型 + Usecase |
| `internal/biz/subagent_run_repo.go` | 新增 | SubAgentRun Repository 接口 |
| `internal/data/subagent_run.go` | 新增 | SubAgentRun Repo 实现（Ent ORM） |
| `internal/tools/toolset.go` | 修改 | Registry 新增 subagent 注册项 |
| `internal/tools/toolset.go` | 修改 | AssemblyConfig 新增 SubAgentConfig |
| `internal/tools/toolset.go` | 修改 | Assemble() 新增 subagent 分支 |
| `internal/service/chat.go` | 修改 | 初始化 SubAgentService 并注入 |

#### 接口设计

**SubAgentService**

```go
package subagent

type Service struct {
    runner  trpcrunner.Runner
    store   biz.SubAgentRunRepository
    clock   func() time.Time
    mu      sync.Mutex
    running map[string]*runningEntry
    baseCtx context.Context
    cancel  context.CancelFunc
    wg      sync.WaitGroup
}

func NewService(
    r trpcrunner.Runner,
    store biz.SubAgentRunRepository,
) *Service

func (s *Service) Start(ctx context.Context)
func (s *Service) Close() error
func (s *Service) Spawn(ctx context.Context, req SpawnRequest) (biz.SubAgentRun, error)
func (s *Service) ListForSession(ctx context.Context, sessionID string) ([]biz.SubAgentRun, error)
func (s *Service) Get(ctx context.Context, runID string) (*biz.SubAgentRun, error)
func (s *Service) Cancel(ctx context.Context, runID string) (*biz.SubAgentRun, error)
```

**SubAgent 工具**

```go
type SpawnInput struct {
    Task           string `json:"task"`
    TimeoutSeconds int    `json:"timeout_seconds"`
}

type ListOutput struct {
    Runs []biz.SubAgentRun `json:"runs"`
}

type GetInput struct {
    ID string `json:"id"`
}

type CancelInput struct {
    ID string `json:"id"`
}
```

**SubAgentRun 领域模型**

```go
package biz

type SubAgentRun struct {
    ID              string
    ParentSessionID string
    ChildSessionID  string
    OwnerUserID     string
    Task            string
    Status          SubAgentStatus
    Summary         string
    Result          string
    Error           string
    CreatedAt       time.Time
    UpdatedAt       time.Time
    StartedAt       *time.Time
    FinishedAt      *time.Time
}

type SubAgentStatus string

const (
    SubAgentStatusQueued    SubAgentStatus = "queued"
    SubAgentStatusRunning   SubAgentStatus = "running"
    SubAgentStatusCompleted SubAgentStatus = "completed"
    SubAgentStatusFailed    SubAgentStatus = "failed"
    SubAgentStatusCanceled  SubAgentStatus = "canceled"
)

type SubAgentRunRepository interface {
    Create(ctx context.Context, run *SubAgentRun) (*SubAgentRun, error)
    Get(ctx context.Context, id string) (*SubAgentRun, error)
    Update(ctx context.Context, run *SubAgentRun) (*SubAgentRun, error)
    ListBySession(ctx context.Context, sessionID string) ([]*SubAgentRun, error)
}
```

#### 数据流图

```
Agent 调用 subagents_spawn 工具
    ↓
SubAgentService.Spawn()
    ├→ 创建 SubAgentRun 记录（Status=queued）
    ├→ 持久化到 SQLite
    └→ pkg/safego.Go → execute()
         ↓
    markRunning() → Status=running, 创建子 Session
         ↓
    runner.Run(ctx, userID, childSessionID, userMessage, runOpts...)
         ↓
    子 Agent 在独立 Session 中执行任务
         ↓ 事件流消费
    replyAccumulator 汇总结果
         ↓
    finishRun() → Status=completed/failed/canceled
         ↓
    持久化结果到 SQLite
         ↓
    notifyCompletion() → OutboundRouter（依赖模块 5）
```

### 2.4 与框架的集成方式

1. **Runner 调用**：子 Agent 通过 `runner.Run()` 执行，遵循铁律 A4/A6
2. **AgentFactory**：复用 `BizAgentFactoryOptions` 已注册的 AgentFactory，子 Agent 使用与父 Agent 相同的构建逻辑
3. **RuntimeState**：通过 `agent.WithRuntimeState()` 注入 `openclaw.subagent.run=true` 标记，实现嵌套检测
4. **事件发射**：子 Agent 内部事件通过 Runner 事件流处理，遵循铁律 A2/A3
5. **持久化**：使用 Ent ORM（SQLite），遵循红线 #11（仅通过 `d.Ent()` 访问）
6. **并发安全**：`pkg/safego.Go` 包装 goroutine，遵循红线 #9

### 2.5 错误处理

| 错误场景 | 处理方式 |
|----------|----------|
| 嵌套派生 | `isNestedSubagent()` 检测，返回 `kerrors.BadRequest` |
| Runner 不可用 | `Spawn()` 返回 `kerrors.InternalServer` |
| 超时 | `context.WithTimeout` 自动取消，`finishRun()` 标记 Status=canceled |
| 持久化失败 | `Spawn()` 回滚（删除内存记录），返回错误 |
| 子 Agent panic | `pkg/safego.GoRecover` 捕获，`finishRun()` 标记 Status=failed |
| 取消已完成的子 Agent | 返回当前状态，`bool=false` 表示未实际取消 |
| 并发限制超限 | 返回 `kerrors.TooManyRequests` |

---

## 三、开发计划

### 3.1 任务拆解

| 任务ID | 描述 | 依赖 | 预估复杂度 |
|--------|------|------|-----------|
| P4-01 | 定义 `internal/biz/subagent_run.go` 领域模型 | 无 | S |
| P4-02 | 定义 `internal/biz/subagent_run_repo.go` Repo 接口 | P4-01 | S |
| P4-03 | 实现 `internal/data/subagent_run.go` Repo 实现 | P4-02 | M |
| P4-04 | 定义 `internal/tools/subagent/types.go` 类型 | 无 | S |
| P4-05 | 定义 `internal/tools/subagent/config.go` 配置 | 无 | S |
| P4-06 | 实现 `internal/tools/subagent/service.go` SubAgentService | P4-01, P4-03 | L |
| P4-07 | 实现 `internal/tools/subagent/tools.go` 4 个工具 | P4-06 | M |
| P4-08 | 修改 Registry 新增 subagent 注册项 | P4-07 | S |
| P4-09 | 修改 AssemblyConfig + Assemble() | P4-08 | M |
| P4-10 | 修改 service/chat.go 初始化 SubAgentService | P4-06 | M |
| P4-11 | Wire DI 集成 | P4-10 | M |
| P4-12 | 单元测试：Service 生命周期 | P4-06 | M |
| P4-13 | 单元测试：工具调用 | P4-07 | M |
| P4-14 | 集成测试：端到端派生 | P4-11 | L |

### 3.2 开发顺序

```
Phase 1（领域层）: P4-01 → P4-02 → P4-03
Phase 2（工具层）: P4-04/P4-05（并行）→ P4-06 → P4-07
Phase 3（集成层）: P4-08 → P4-09 → P4-10 → P4-11
Phase 4（验证）: P4-12/P4-13（并行）→ P4-14
```

### 3.3 验证方案

| 验证项 | 方法 |
|--------|------|
| Spawn 创建子 Agent | 单元测试：Mock Runner，验证 Spawn 返回 queued 状态的 Run |
| List 查询 | 单元测试：创建多个子 Agent，验证 List 返回正确列表 |
| Get 查询 | 单元测试：验证 Get 返回最新状态 |
| Cancel 取消 | 单元测试：验证 Cancel 后 Status 变为 canceled |
| 嵌套限制 | 单元测试：在子 Agent 上下文中调用 Spawn，验证返回错误 |
| 超时控制 | 单元测试：设置短超时，验证自动取消 |
| 持久化 | 集成测试：重启 Service 后验证运行记录恢复 |
| 端到端 | 手动测试：Agent 调用 spawn，子 Agent 执行任务，get 获取结果 |
