# 多模式 Agent 编排

## 一、需求文档

### 1.1 背景

trpc-agent-go 框架提供了三种多 Agent 编排模式：ChainAgent（串行链式）、CycleAgent（循环迭代）、ParallelAgent（并行执行）。当前项目 `internal/agent/` 只有 LLMAgent + A2AProxy + ModelRegistrySync，缺少对这三种编排模式的支持。多模式编排是实现复杂工作流（如"搜索→分析→总结"串行链、"多视角并行评估"、"循环优化直到满意"）的关键能力。

### 1.2 目标

- 新增 ChainAgent / CycleAgent / ParallelAgent 构建入口
- 在 `internal/agent/` 中提供统一的构建函数，将 biz 层的编排配置转换为框架 Agent
- 前端可配置 Agent 的编排模式（单 Agent / 链式 / 循环 / 并行）
- 编排模式与现有 Team/Graph 编排共存

### 1.3 功能需求

| # | 功能 | 优先级 | 说明 |
|---|------|--------|------|
| F1 | ChainAgent 构建入口 | P0 | 串行执行子 Agent，前一个的输出影响后一个 |
| F2 | CycleAgent 构建入口 | P0 | 循环执行子 Agent，直到升级条件或最大迭代 |
| F3 | ParallelAgent 构建入口 | P0 | 并行执行子 Agent，合并事件流 |
| F4 | 编排模式配置 | P0 | Agent 配置中指定编排模式和子 Agent 列表 |
| F5 | 子 Agent 动态构建 | P1 | 子 Agent 通过 AgentKey 从 Catalog 动态构建 |
| F6 | CycleAgent 自定义升级函数 | P1 | 支持自定义 EscalationFunc |
| F7 | 前端编排模式配置 UI | P2 | Agent 设置页增加编排模式选择 |

### 1.4 非功能需求

- ChainAgent 串行执行，子 Agent 间事件正确传递
- CycleAgent 循环次数上限可配置，防止无限循环
- ParallelAgent 并行执行，事件流合并不丢失
- 所有 Agent 必须实现 `agent.Agent` 接口（5 方法）
- 子 Agent 构建失败时整体构建失败

### 1.5 验收标准

- ChainAgent 按顺序执行子 Agent，事件流正确
- CycleAgent 循环执行直到升级条件或最大迭代
- ParallelAgent 并行执行，事件流合并
- 子 Agent 可通过 AgentKey 动态构建
- 编排模式与现有 LLMAgent 构建共存

---

## 二、设计文档

### 2.1 框架参考（trpc-agent-go）

**核心包路径**：

| 编排模式 | 包路径 |
|----------|--------|
| ChainAgent | `pkg/trpc-agent-go/agent/chainagent/chain_agent.go` |
| CycleAgent | `pkg/trpc-agent-go/agent/cycleagent/cycle_agent.go` |
| ParallelAgent | `pkg/trpc-agent-go/agent/parallelagent/parallel_agent.go` |

**ChainAgent 核心类型和函数**：

```go
// ChainAgent 串行执行子 Agent
type ChainAgent struct {
    name              string
    subAgents         []agent.Agent
    channelBufferSize int
    agentCallbacks    *agent.Callbacks
}

func New(name string, opts ...Option) *ChainAgent

// Option
type Option func(*Options)
type Options struct {
    subAgents         []agent.Agent
    channelBufferSize int
    agentCallbacks    *agent.Callbacks
}

func WithSubAgents(subAgents []agent.Agent) Option
func WithChannelBufferSize(size int) Option
func WithAgentCallbacks(cb *agent.Callbacks) Option
```

**CycleAgent 核心类型和函数**：

```go
// CycleAgent 循环执行子 Agent
type CycleAgent struct {
    name              string
    subAgents         []agent.Agent
    maxIterations     *int
    channelBufferSize int
    agentCallbacks    *agent.Callbacks
    escalationFunc    EscalationFunc
}

func New(name string, opts ...Option) *CycleAgent

// EscalationFunc 判断是否升级（停止循环）
type EscalationFunc func(*event.Event) bool

// Option
func WithSubAgents(sub []agent.Agent) Option
func WithMaxIterations(max int) Option
func WithChannelBufferSize(size int) Option
func WithAgentCallbacks(cb *agent.Callbacks) Option
func WithEscalationFunc(f EscalationFunc) Option
```

**ParallelAgent 核心类型和函数**：

```go
// ParallelAgent 并行执行子 Agent
type ParallelAgent struct {
    name              string
    subAgents         []agent.Agent
    channelBufferSize int
    agentCallbacks    *agent.Callbacks
}

func New(name string, opts ...Option) *ParallelAgent

// Option
func WithSubAgents(sub []agent.Agent) Option
func WithChannelBufferSize(size int) Option
func WithAgentCallbacks(cb *agent.Callbacks) Option
```

**三种 Agent 共同特征**：
- 都实现了 `agent.Agent` 接口的 5 个方法（Run/Tools/Info/SubAgents/FindSubAgent）
- `Tools()` 返回空切片（编排 Agent 自身不持有工具）
- `Run()` 内部使用 `agent.RunWithPlugins` 执行子 Agent
- 事件流通过 `event.EmitEvent` 转发
- 支持 `agent.Callbacks` 生命周期回调

### 2.2 当前项目现状

| 位置 | 现状 |
|------|------|
| `internal/agent/trpc_build.go` | `BuildTRPCLLMAgent` 只构建 LLMAgent |
| `internal/agent/trpc_build_router.go` | `BuildTRPCAgent` 路由到 LLMAgent 或 A2A Agent |
| `internal/agent/factory.go` | `BizAgentFactoryOptions` 注册 AgentKey 工厂 |
| `internal/agent/builder_deps.go` | `TRPCBuilderDeps` 无编排相关依赖 |
| `internal/biz/agent.go` | `biz.Agent` 无编排模式字段 |
| `internal/team/` | Team 编排使用 Graph，与本次新增的编排模式互补 |

### 2.3 架构设计

**模块在四层架构中的位置**：

```
api/**/*.proto          ← 新增编排模式配置字段
        ↓
internal/service        ← Runner 装配时根据编排模式构建对应 Agent
        ↓
internal/biz            ← Agent 模型扩展编排模式配置
        ↓
internal/agent          ← 新增 Chain/Cycle/Parallel 构建函数
```

**新增/修改的文件清单**：

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/biz/agent_orchestration.go` | 新增 | 编排模式 biz 模型定义 |
| `internal/agent/chain.go` | 新增 | `BuildChainAgent` 构建函数 |
| `internal/agent/cycle.go` | 新增 | `BuildCycleAgent` 构建函数 |
| `internal/agent/parallel.go` | 新增 | `BuildParallelAgent` 构建函数 |
| `internal/agent/trpc_build_router.go` | 修改 | 路由逻辑增加编排模式分支 |
| `internal/biz/agent.go` | 修改 | `Agent` 新增编排模式字段 |
| `api/admin/v1/agent.proto` | 修改 | 新增编排模式消息类型 |
| `internal/data/ent/schema/` | 修改 | Agent 表扩展编排模式字段 |

**接口设计**：

```go
// internal/biz/agent_orchestration.go

type OrchestrationMode string

const (
    OrchestrationModeSingle    OrchestrationMode = "single"
    OrchestrationModeChain     OrchestrationMode = "chain"
    OrchestrationModeCycle     OrchestrationMode = "cycle"
    OrchestrationModeParallel  OrchestrationMode = "parallel"
)

type OrchestrationConfig struct {
    Mode           OrchestrationMode `json:"mode"`
    SubAgentKeys   []string          `json:"sub_agent_keys"`
    MaxIterations  *int              `json:"max_iterations,omitempty"`
    EscalationRule string            `json:"escalation_rule,omitempty"`
}

// internal/agent/chain.go

func BuildChainAgent(
    ctx context.Context,
    name string,
    subAgentKeys []string,
    deps TRPCBuilderDeps,
) (trpcagent.Agent, error)

// internal/agent/cycle.go

func BuildCycleAgent(
    ctx context.Context,
    name string,
    subAgentKeys []string,
    maxIterations *int,
    escalationFunc cycleagent.EscalationFunc,
    deps TRPCBuilderDeps,
) (trpcagent.Agent, error)

// internal/agent/parallel.go

func BuildParallelAgent(
    ctx context.Context,
    name string,
    subAgentKeys []string,
    deps TRPCBuilderDeps,
) (trpcagent.Agent, error)
```

**数据流图**：

```
前端编排模式配置
  → API UpdateAgent (orchestration JSON)
    → biz.Agent.OrchestrationConfig
      → BuildTRPCAgent 路由
        ├─ single → BuildTRPCLLMAgent（现有）
        ├─ chain  → BuildChainAgent → chainagent.New(name, WithSubAgents(subs))
        ├─ cycle  → BuildCycleAgent → cycleagent.New(name, WithSubAgents(subs), WithMaxIterations(n))
        └─ parallel → BuildParallelAgent → parallelagent.New(name, WithSubAgents(subs))
              → 子 Agent 通过 BuildTRPCLLMAgentCached 逐个构建
                → Runner 装配
```

### 2.4 与框架的集成方式

1. **直接使用框架类型**：`chainagent.New`/`cycleagent.New`/`parallelagent.New` 直接构建框架 Agent
2. **子 Agent 构建**：通过 `BuildTRPCLLMAgentCached` 从 Catalog 动态构建子 Agent
3. **路由扩展**：`BuildTRPCAgent` 根据 `OrchestrationConfig.Mode` 路由到不同构建函数
4. **EscalationFunc**：CycleAgent 的升级函数由项目自定义实现，如检测特定事件类型或 LLM 判断
5. **Callbacks**：编排 Agent 支持 `agent.Callbacks`，可与现有 `callback_chain.go` 集成

### 2.5 错误处理

| 场景 | 处理方式 |
|------|----------|
| 子 Agent 构建失败 | 整体构建失败，返回 `kerrors.InternalServer` |
| 子 Agent Key 不存在 | 返回 `kerrors.NotFound("AGENT", "sub agent not found")` |
| CycleAgent 无升级条件且无 maxIterations | 循环无限执行直到 ctx 取消 |
| ParallelAgent 子 Agent panic | `recover` 捕获，发送错误事件 |
| ChainAgent 子 Agent 执行错误 | 发送错误事件，终止链式执行 |
| ctx 取消 | `agent.CheckContextCancelled` 检测后正常退出 |

---

## 三、开发计划

### 3.1 任务拆解

| 任务ID | 描述 | 依赖 | 预估复杂度 |
|--------|------|------|-----------|
| OA-01 | `internal/biz/agent_orchestration.go`：定义编排模式 biz 模型 | 无 | S |
| OA-02 | `internal/biz/agent.go`：`Agent` 新增 `OrchestrationConfig` 字段 | OA-01 | S |
| OA-03 | `internal/agent/chain.go`：`BuildChainAgent` 构建函数 | OA-01 | M |
| OA-04 | `internal/agent/cycle.go`：`BuildCycleAgent` 构建函数 | OA-01 | M |
| OA-05 | `internal/agent/parallel.go`：`BuildParallelAgent` 构建函数 | OA-01 | M |
| OA-06 | `internal/agent/trpc_build_router.go`：路由逻辑增加编排模式分支 | OA-03, OA-04, OA-05 | M |
| OA-07 | `api/admin/v1/agent.proto`：新增编排模式消息类型 | OA-01 | M |
| OA-08 | `make api` 重新生成 proto 代码 | OA-07 | S |
| OA-09 | `internal/data/ent/schema/`：Agent 表扩展编排模式字段 | OA-02 | M |
| OA-10 | `go generate` 重新生成 Ent 代码 | OA-09 | S |
| OA-11 | Service 层 proto↔biz 映射函数 | OA-08, OA-02 | M |
| OA-12 | 单元测试：三种编排 Agent 构建 | OA-06 | M |
| OA-13 | 集成测试：编排模式端到端 | OA-11 | L |
| OA-14 | `make wire` 更新 Wire 注入 | OA-11 | S |

### 3.2 开发顺序

```
OA-01 → OA-02 → OA-03 ─┐
                OA-04 ─┤→ OA-06 → OA-07 → OA-08
                OA-05 ─┘              ↓
                              OA-09 → OA-10 → OA-11 → OA-12 → OA-13 → OA-14
```

### 3.3 验证方案

| 验证项 | 方法 |
|--------|------|
| ChainAgent 构建 | `go test ./internal/agent/... -run TestBuildChain -count=1` |
| CycleAgent 构建 | `go test ./internal/agent/... -run TestBuildCycle -count=1` |
| ParallelAgent 构建 | `go test ./internal/agent/... -run TestBuildParallel -count=1` |
| 路由逻辑 | `go test ./internal/agent/... -run TestBuildTRPCAgent -count=1` |
| Proto 生成 | `make api && go build ./...` |
| Wire 注入 | `make wire && go build ./cmd/admin` |
| 全量验证 | `make api && make wire && make build && make test` |
