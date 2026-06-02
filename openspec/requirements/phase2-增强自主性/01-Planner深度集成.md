# Planner 深度集成

## 一、需求文档

### 1.1 背景

当前 Aranea-Agents 的 Agent 本质上是"工具型 Agent"——接收用户指令，调用工具，返回结果。用户必须逐步下达指令，Agent 无法自主规划多步骤执行路径。

框架 `pkg/trpc-agent-go/planner/` 提供了 `Planner` 接口及其三种实现（`builtin`/`react`/`a2ui`），项目 `internal/agent/planner/` 已有构建器但仅做配置解析，未与 Graph 执行引擎深度结合。

A2UI Planner 是 Phase 2 的质变点：Agent 接收目标后，通过 A2UI 协议自主生成结构化执行计划（Graph），将计划渲染为可交互 UI（Surface），用户审批后 Agent 按计划自主执行。这实现了从"工具型 Agent"到"Human-Agent"的跃迁。

### 1.2 目标

1. 将 A2UI Planner 深度集成到 Agent 构建流程，使 Agent 具备自主规划能力
2. 实现 Planner → Graph 的转换管道，将 A2UI 输出的结构化计划转为可执行的 Graph
3. 前端 AgentPlannerSection.vue 支持 A2UI Surface 渲染与用户交互
4. 支持 Plan-Execute-Observe 循环，Agent 可根据执行结果动态调整计划

### 1.3 功能需求

| # | 功能 | 优先级 | 说明 |
|---|------|--------|------|
| F1 | A2UI Planner 配置化注入 | P0 | Agent 配置中指定 planner=a2ui 时自动注入 Planner |
| F2 | Plan → Graph 转换器 | P0 | 将 A2UI 输出的结构化计划转为 `graph.Graph` 定义 |
| F3 | A2UI Surface 事件流 | P0 | Agent 通过 `beginRendering`/`surfaceUpdate`/`dataModelUpdate` 向前端推送计划 UI |
| F4 | 用户审批交互 | P0 | 前端通过 `userAction` 事件将用户审批结果回传 Agent |
| F5 | Plan-Execute-Observe 循环 | P1 | Agent 执行计划步骤后观察结果，动态调整后续步骤 |
| F6 | 计划持久化 | P1 | 将生成的计划存入 Session State，支持断点续执行 |
| F7 | 多 Planner 策略切换 | P2 | 运行时根据任务类型动态选择 builtin/react/a2ui |

### 1.4 非功能需求

- A2UI 协议消息必须为 JSONL 格式，每行一个完整 JSON 对象
- Planner 注入不得影响无 Planner Agent 的现有行为
- Plan → Graph 转换延迟 < 500ms（10 步以内计划）
- 前端 Surface 渲染首帧 < 1s

### 1.5 验收标准

1. 配置 `planner=a2ui` 的 Agent 收到用户目标后，能生成包含 `beginRendering` + `surfaceUpdate` 的事件流
2. 前端 AgentPlannerSection.vue 能渲染 A2UI Surface 并回传 `userAction`
3. Agent 收到用户审批后能按计划执行 Graph 节点
4. 执行过程中 Agent 能通过 `dataModelUpdate` 更新进度

---

## 二、设计文档

### 2.1 框架参考（trpc-agent-go / OpenClaw）

#### 核心接口

**`planner.Planner`** — `pkg/trpc-agent-go/planner/planner.go`

```go
type Planner interface {
    BuildPlanningInstruction(
        ctx context.Context,
        invocation *agent.Invocation,
        llmRequest *model.Request,
    ) string

    ProcessPlanningResponse(
        ctx context.Context,
        invocation *agent.Invocation,
        response *model.Response,
    ) *model.Response
}
```

**`a2ui.New`** — `pkg/trpc-agent-go/planner/a2ui/a2ui.go`

```go
func New(opts ...Option) planner.Planner
```

**A2UI Option 函数** — `pkg/trpc-agent-go/planner/a2ui/options.go`

```go
func WithInstruction(instruction string) Option
func WithServerToClientWithStandardCatalogSchema(schema string) Option
func WithClientToServerSchema(schema string) Option
func WithClientCapabilitiesSchema(schema string) Option
func WithServerToClientSchema(schema string) Option
func WithStandardCatalogDefinition(definition string) Option
func WithCatalogDescriptionSchema(schema string) Option
```

**A2UI Schema** — `pkg/trpc-agent-go/planner/a2ui/schema.go`

- `defaultServerToClientWithStandardCatalogSchema`：定义 `beginRendering`/`surfaceUpdate`/`dataModelUpdate`/`deleteSurface` 四种消息类型
- `defaultClientToServerSchema`：定义 `userAction`/`error` 两种客户端消息

**`graph.Graph`** — `pkg/trpc-agent-go/graph/`

Graph 是框架的执行引擎，支持节点定义、边定义、条件路由。

**`agent.EmitEvent`** — `pkg/trpc-agent-go/agent/`

```go
func EmitEvent(ctx context.Context, inv *agent.Invocation, ch chan<- *event.Event, evt *event.Event) error
```

### 2.2 当前项目现状

| 文件 | 现状 |
|------|------|
| `internal/agent/planner/build.go` | 已有 `buildA2UI()`/`buildBuiltin()`/`buildReact()` 构建器，但仅返回 `planner.Planner` 实例 |
| `internal/agent/planner/config.go` | 已有 `a2uiConfigJSON` 解析，支持 7 个 A2UI 配置字段 |
| `internal/agent/planner/selector.go` | 已有 Planner 类型选择逻辑 |
| `internal/agent/trpc_build.go` | Agent 构建入口，已集成 Planner 到 LLMAgent |
| `internal/agent/trpc_runtime.go` | TRPC 运行时，Runner 装配 |
| 前端 `AgentPlannerSection.vue` | 已有 Planner UI 组件骨架 |

**差距**：
1. A2UI Planner 仅注入了 system prompt 约束，未实现 Plan → Graph 转换
2. 无 A2UI Surface 事件流的编码/解码管道
3. 前端未实现 A2UI 协议的 `beginRendering`/`surfaceUpdate`/`dataModelUpdate` 渲染
4. 无用户 `userAction` 回传机制

### 2.3 架构设计

#### 模块在四层架构中的位置

```
internal/service     ← Runner 装配（已有，无需改动）
internal/agent       ← 新增 A2UI 管道 + Plan→Graph 转换器
  planner/           ← 已有，扩展 A2UI 配置
  a2ui/              ← 新增：A2UI 事件编码/解码/Surface 管理
internal/biz         ← 新增 Plan 领域模型 + Repo 接口
internal/data        ← 新增 Plan 持久化 Repo 实现
```

#### 新增/修改的文件清单

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/agent/a2ui/pipeline.go` | 新增 | A2UI 事件管道：编码/解码/Surface 生命周期管理 |
| `internal/agent/a2ui/surface.go` | 新增 | Surface 状态管理器：跟踪活跃 Surface、组件树、数据模型 |
| `internal/agent/a2ui/encoder.go` | 新增 | A2UI 消息编码器：将结构化计划转为 JSONL 事件流 |
| `internal/agent/a2ui/decoder.go` | 新增 | A2UI 消息解码器：解析前端 `userAction` 回传 |
| `internal/agent/a2ui/converter.go` | 新增 | Plan → Graph 转换器：将 A2UI 计划结构转为 `graph.Graph` |
| `internal/agent/a2ui/types.go` | 新增 | A2UI 内部类型定义 |
| `internal/agent/planner/build.go` | 修改 | 扩展 `buildA2UI()` 返回增强的 Planner 包装 |
| `internal/agent/trpc_build.go` | 修改 | 集成 A2UI Pipeline 到 Agent 构建流程 |
| `internal/biz/plan.go` | 新增 | Plan 领域模型 + Usecase |
| `internal/biz/plan_repo.go` | 新增 | Plan Repository 接口 |
| `internal/data/plan.go` | 新增 | Plan Repo 实现（Ent ORM） |
| 前端 `features/chat/api.ts` | 修改 | 新增 A2UI 事件类型 |
| 前端 `components/chat/A2UISurface.vue` | 新增 | A2UI Surface 渲染组件 |

#### 接口设计

**A2UI Pipeline**

```go
package a2ui

type Pipeline interface {
    EncodePlan(ctx context.Context, plan *Plan) ([]*event.Event, error)
    DecodeUserAction(ctx context.Context, payload []byte) (*UserAction, error)
    UpdateSurface(ctx context.Context, surfaceID string, update *SurfaceUpdate) (*event.Event, error)
    UpdateDataModel(ctx context.Context, surfaceID string, contents []DataEntry) (*event.Event, error)
    DeleteSurface(ctx context.Context, surfaceID string) (*event.Event, error)
}

type Plan struct {
    ID          string
    Goal        string
    Steps       []PlanStep
    Dependencies map[string][]string
}

type PlanStep struct {
    ID          string
    Name        string
    Description string
    AgentName   string
    Tools       []string
    DependsOn   []string
}

type UserAction struct {
    Name            string
    SurfaceID       string
    SourceComponentID string
    Timestamp       time.Time
    Context         map[string]any
}

type SurfaceUpdate struct {
    SurfaceID  string
    Components []Component
}

type DataEntry struct {
    Key          string
    ValueString  *string
    ValueNumber  *float64
    ValueBoolean *bool
    ValueMap     []DataEntry
}
```

**Plan → Graph 转换器**

```go
type PlanToGraphConverter interface {
    Convert(ctx context.Context, plan *Plan) (*graph.Graph, error)
}
```

**Plan 领域模型**

```go
package biz

type Plan struct {
    ID          string
    SessionID   string
    AgentKey    string
    Goal        string
    Steps       []PlanStep
    Status      PlanStatus
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type PlanStatus string

const (
    PlanStatusDraft     PlanStatus = "draft"
    PlanStatusApproved  PlanStatus = "approved"
    PlanStatusExecuting PlanStatus = "executing"
    PlanStatusCompleted PlanStatus = "completed"
    PlanStatusFailed    PlanStatus = "failed"
)

type PlanRepository interface {
    Create(ctx context.Context, plan *Plan) (*Plan, error)
    Get(ctx context.Context, id string) (*Plan, error)
    Update(ctx context.Context, plan *Plan) (*Plan, error)
    ListBySession(ctx context.Context, sessionID string) ([]*Plan, error)
}
```

#### 数据流图

```
用户输入目标
    ↓
Agent (LLMAgent + A2UI Planner)
    ↓ BuildPlanningInstruction() 注入 A2UI 协议约束
    ↓ LLM 生成结构化计划（JSONL）
    ↓ ProcessPlanningResponse() 解析计划
    ↓
A2UI Pipeline
    ├→ EncodePlan() → beginRendering + surfaceUpdate 事件 → 前端
    ├→ PlanToGraphConverter.Convert() → graph.Graph → Runner 执行
    └→ 等待用户 userAction
         ↓
    DecodeUserAction() → 审批结果
         ↓
    Graph 开始执行
         ↓ 每个 Graph 节点完成
    UpdateDataModel() → 更新进度 → 前端
```

### 2.4 与框架的集成方式

1. **Planner 注入**：复用 `planner.Planner` 接口，在 `internal/agent/planner/build.go` 的 `buildA2UI()` 中返回增强包装
2. **事件发射**：通过 `agent.EmitEvent(ctx, inv, ch, evt)` 发射 A2UI 事件，遵循铁律 A2
3. **Graph 执行**：通过 `graph.Graph` 定义计划执行流程，由 Runner 调度
4. **Session 持久化**：计划状态通过 Session State 持久化，支持断点续执行
5. **前端通信**：通过 WebSocket Envelope 传输 A2UI 事件

### 2.5 错误处理

| 错误场景 | 处理方式 |
|----------|----------|
| LLM 未输出有效 A2UI JSONL | `ProcessPlanningResponse` 返回 nil，Agent 回退到普通对话模式 |
| Plan → Graph 转换失败 | 发射 `ObjectTypeError` 事件，提示用户重新规划 |
| 用户拒绝计划 | Agent 收到 `userAction.name="reject"`，重新进入规划对话 |
| Graph 节点执行失败 | 通过 `dataModelUpdate` 更新失败状态，Agent 决定重试或调整 |
| Surface 渲染超时 | 前端降级显示纯文本计划摘要 |

---

## 三、开发计划

### 3.1 任务拆解

| 任务ID | 描述 | 依赖 | 预估复杂度 |
|--------|------|------|-----------|
| P1-01 | 定义 `internal/agent/a2ui/types.go` A2UI 内部类型 | 无 | S |
| P1-02 | 实现 `internal/agent/a2ui/encoder.go` A2UI 消息编码器 | P1-01 | M |
| P1-03 | 实现 `internal/agent/a2ui/decoder.go` A2UI 消息解码器 | P1-01 | M |
| P1-04 | 实现 `internal/agent/a2ui/surface.go` Surface 状态管理 | P1-01 | M |
| P1-05 | 实现 `internal/agent/a2ui/pipeline.go` A2UI Pipeline | P1-02, P1-03, P1-04 | L |
| P1-06 | 实现 `internal/agent/a2ui/converter.go` Plan→Graph 转换器 | P1-01 | L |
| P1-07 | 修改 `internal/agent/planner/build.go` 集成 A2UI Pipeline | P1-05 | M |
| P1-08 | 修改 `internal/agent/trpc_build.go` 集成到 Agent 构建 | P1-07 | M |
| P1-09 | 新增 `internal/biz/plan.go` Plan 领域模型 + Usecase | 无 | M |
| P1-10 | 新增 `internal/biz/plan_repo.go` Plan Repository 接口 | P1-09 | S |
| P1-11 | 新增 `internal/data/plan.go` Plan Repo 实现 | P1-10 | M |
| P1-12 | 前端 A2UI 事件类型定义 | P1-01 | S |
| P1-13 | 前端 A2UISurface.vue 渲染组件 | P1-12 | L |
| P1-14 | 前端 userAction 回传机制 | P1-13 | M |
| P1-15 | 端到端集成测试 | P1-08, P1-11, P1-14 | L |

### 3.2 开发顺序

```
Phase 1（基础层）: P1-01 → P1-02/P1-03/P1-04（并行）→ P1-05
Phase 2（转换层）: P1-06（与 Phase 1 并行）
Phase 3（集成层）: P1-07 → P1-08
Phase 4（持久化）: P1-09 → P1-10 → P1-11
Phase 5（前端）: P1-12 → P1-13 → P1-14
Phase 6（验证）: P1-15
```

### 3.3 验证方案

| 验证项 | 方法 |
|--------|------|
| A2UI 编码器输出合规 | 单元测试：验证输出为 JSONL 格式，每行一个完整 JSON |
| Plan→Graph 转换正确性 | 单元测试：给定 Plan 结构，验证生成的 Graph 节点/边/条件路由 |
| Surface 生命周期 | 单元测试：beginRendering → surfaceUpdate → dataModelUpdate → deleteSurface |
| 用户交互闭环 | 集成测试：模拟 userAction 回传，验证 Agent 响应 |
| 端到端流程 | 手动测试：配置 planner=a2ui 的 Agent，发送目标，验证前端渲染+审批+执行 |
| 无 Planner 回归 | 回归测试：配置无 Planner 的 Agent，验证行为不变 |

### 1.6 BabyAGI 启发：记忆驱动任务规划

> 来源：BabyAGI 向量记忆驱动的 Task Creation Agent（GitHub 22k+ stars），竞品分析差距 #8

BabyAGI 的经典版核心是一个"执行→创建→排序"循环，其中 **Task Creation Agent 基于向量数据库中存储的历史任务结果来生成新任务**。这一思想对本需求的 F5（Plan-Execute-Observe 循环）有直接启发：

| BabyAGI 能力 | 本需求对应 | 差异 |
|-------------|-----------|------|
| 向量存储任务结果供后续规划参考 | F5 Plan-Execute-Observe 循环 | BabyAGI 用 Pinecone，本需求可用 L0-L4 记忆系统 |
| Task Creation Agent 基于 LLM + 记忆生成新任务 | F1-F5 整体流程 | BabyAGI 是无限循环，本需求是单次 Plan→Graph 执行 |
| Prioritization Agent 按目标相关性重排 | 无对应 | **新增启发**：Plan 步骤可增加优先级评分 |

**可借鉴的增强方向**（对应 `docs/competitive-gap-requirements-2026-05-31.md` P2-11）：

1. **Planner 记忆注入**：在 `buildA2UI()` 中注入 `memory.Service.SearchMemories` 结果，让 LLM 在规划时参考历史执行经验
2. **Plan 步骤优先级评分**：Plan 生成后对每个 Step 进行优先级评分（基于目标相关性和记忆检索匹配度），Graph 执行时按优先级调度可并行步骤
3. **动态计划调整**：F5 的 Observe 阶段不仅更新进度，还可基于观察结果触发 `PlanToGraphConverter` 重新生成后续步骤（类似 BabyAGI 的 Task Creation Agent）
