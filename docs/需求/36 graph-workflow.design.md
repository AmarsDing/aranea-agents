# Graph 工作流模块 — 实现设计文档（v2）

> 对应需求：`36 graph-workflow.md`（v2 四维需求架构）
> 遵循规范：`AI-DEVELOPMENT-SPECIFICATION.md`

---

## 一、模块概述

Graph 工作流引擎：基于 trpc-agent-go `graph` 包，构建"LangGraph for Go"级别的确定性工作流系统。

### 1.1 设计定位

Graph 的核心存在意义是解决**复杂流程的确定性执行**问题——Team 解决"多 Agent 如何协作"，Graph 解决"复杂流程如何确定性地执行"。二者互补而非替代。

| 维度 | Team | Graph |
|------|------|-------|
| 编排哲学 | 模式化协作 | 自由编排 |
| 控制权 | 框架决定执行顺序 | 用户决定流程骨架 |
| 条件分支 | 无 | 支持 |
| 人工介入 | 无 | HITL 中断/恢复 |
| 状态管理 | 隐式消息传递 | 显式 State Schema + Reducer |
| 可回溯性 | 步骤列表 | Checkpoint + TimeTravel |
| 并行控制 | 模式级 | 节点级（DAG 引擎） |

### 1.2 现有基础

| 组件 | 文件 | 状态 |
|------|------|------|
| Graph 构建器 | `internal/graph/trpc/builder.go` | ✅ BuildStateGraph + GraphAgent + StateSchema/Reducer + 子图 + DAG |
| 函数注册表 | `internal/graph/trpc/registry.go` | ✅ NodeFunc/CondFunc 注册表 |
| Checkpoint 适配器 | `internal/graph/trpc/checkpoint.go` | ✅ SQLite Checkpoint Saver |
| 事件桥接器 | `internal/graph/trpc/event_bridge.go` | ✅ 9 种 ObjectType 映射 |
| 可视化解析 | `internal/graph/trpc/visualize.go` | ✅ DOT 解析 + 结构化 JSON |
| 业务层 | `internal/biz/graph.go` | ✅ CRUD + Execute + Resume + TimeTravel + EventBridge |
| 数据层 | `internal/data/graph.go` | ✅ GraphRepo + GraphRunRepo |
| 服务层 | `internal/service/graph.go` | ✅ 15 个 RPC 方法 |
| Proto 定义 | `api/kratos/graph/v1/graph.proto` | ✅ 15 个 RPC 端点 |
| 前端编辑器 | `web/src/features/graph/` | ✅ Vue Flow 画布 + 节点面板 + 属性面板 |

### 1.3 本期设计目标

基于需求文档 v2 的四维架构，将设计从"功能实现"升级为"系统能力"：

| 维度 | 当前状态 | 目标 |
|------|----------|------|
| 图结构与混合控制 | ✅ 基础节点/边已实现 | 完善节点类型属性配置、混合控制语义 |
| 动态拓扑与状态共享 | ✅ 条件路由/State Schema 已实现 | 补全动态节点生成、State Schema 校验 |
| 人机协同与可观测性 | ✅ HITL/Checkpoint/EventBridge 已实现 | 补全 Checkpoint 持久化、执行摘要、设计时校验 |
| 设计辅助与资产复用 | ❌ 未实现 | 设计模式模板、设计时校验、资产复用 |

---

## 二、维度一：图结构与混合控制

> 系统底层采用有向图（DAG）定义流程骨架。Agent 节点内部拥有基于 LLM 的自主推理能力，但节点间的流转由确定性的图规则牢牢控制。

### 2.1 节点类型体系与属性配置

每种节点类型对应不同的属性配置面板和后端构建逻辑：

#### 2.1.1 Function 节点

**框架映射**：`sg.AddNode(nodeID, nodeFunc, opts...)`

| 属性 | Proto 字段 | 必填 | 说明 |
|------|-----------|------|------|
| 节点 ID | `node_id` | 是 | 唯一标识 |
| 描述 | `description` | 否 | 功能说明 |
| FuncRef | `func_ref` | 是 | Registry 注册的函数引用 |
| InterruptBefore | `interrupt_before` | 否 | 执行前中断 |
| InterruptAfter | `interrupt_after` | 否 | 执行后中断 |
| RetryPolicy | `retry_policy` | 否 | 重试策略 |
| CachePolicy | `cache_policy` | 否 | 缓存策略 |

**构建逻辑**：
```go
func buildFunctionNode(sg *trpcgraph.StateGraph, n *NodeDef, reg *Registry) error {
    fn, err := reg.ResolveNodeFunc(n.FuncRef)
    if err != nil {
        return fmt.Errorf("resolve func %s: %w", n.FuncRef, err)
    }
    opts := []trpcgraph.AddNodeOption{}
    if n.InterruptBefore { opts = append(opts, trpcgraph.WithInterruptBefore()) }
    if n.InterruptAfter { opts = append(opts, trpcgraph.WithInterruptAfter()) }
    if n.RetryPolicy != nil { opts = append(opts, trpcgraph.WithRetryPolicy(toRetryPolicy(n.RetryPolicy))) }
    if n.CachePolicy != nil { opts = append(opts, trpcgraph.WithNodeCachePolicy(toCachePolicy(n.CachePolicy))) }
    sg.AddNode(n.NodeID, fn, opts...)
    return nil
}
```

#### 2.1.2 LLM 节点

**框架映射**：`sg.AddLLMNode(nodeID, instruction, model, opts...)`

| 属性 | Proto 字段 | 必填 | 说明 |
|------|-----------|------|------|
| 节点 ID | `node_id` | 是 | 唯一标识 |
| 描述 | `description` | 否 | 功能说明 |
| Instruction | `instruction` | 是 | LLM 指令模板 |
| Model | `model` | 是 | 模型选择（provider/model 格式） |
| Tools | `tool_names` | 否 | 绑定工具列表 |
| UserInputKey | `user_input_key` | 否 | State 中作为 user_input 的字段 |
| GenerationConfig | `generation_config` | 否 | 生成参数（temperature/max_tokens 等） |
| InterruptBefore | `interrupt_before` | 否 | 执行前中断 |
| InterruptAfter | `interrupt_after` | 否 | 执行后中断 |

**构建逻辑**：
```go
func buildLLMNode(sg *trpcgraph.StateGraph, n *NodeDef, reg *Registry, modelFn ModelResolver) error {
    model, err := modelFn(n.Model)
    if err != nil {
        return fmt.Errorf("resolve model %s: %w", n.Model, err)
    }
    opts := []trpcgraph.AddLLMNodeOption{}
    if len(n.ToolNames) > 0 {
        tools, err := reg.ResolveTools(n.ToolNames)
        if err != nil { return err }
        opts = append(opts, trpcgraph.WithLLMNodeTools(tools...))
    }
    if n.UserInputKey != "" {
        opts = append(opts, trpcgraph.WithLLMNodeUserInputKey(n.UserInputKey))
    }
    if n.InterruptBefore { opts = append(opts, trpcgraph.WithInterruptBefore()) }
    if n.InterruptAfter { opts = append(opts, trpcgraph.WithInterruptAfter()) }
    sg.AddLLMNode(n.NodeID, n.Instruction, model, opts...)
    return nil
}
```

#### 2.1.3 Tool 节点

**框架映射**：`sg.AddToolNode(nodeID, toolNames, opts...)`

| 属性 | Proto 字段 | 必填 | 说明 |
|------|-----------|------|------|
| 节点 ID | `node_id` | 是 | 唯一标识 |
| 描述 | `description` | 否 | 功能说明 |
| ToolNames | `tool_names` | 是 | 工具名称列表 |
| EnableParallelTools | `enable_parallel_tools` | 否 | 并行执行多个工具调用 |
| ToolCallRetryPolicy | `tool_call_retry_policy` | 否 | 工具调用重试策略 |
| InterruptBefore | `interrupt_before` | 否 | 执行前中断 |
| InterruptAfter | `interrupt_after` | 否 | 执行后中断 |

#### 2.1.4 Agent 节点（混合控制核心）

**框架映射**：`sg.AddNode(nodeID, agentAsSubgraph, opts...)`

Agent 节点是"混合控制"的精髓——节点内部 Agent 自主推理，节点外部 Graph 控制流转。

| 属性 | Proto 字段 | 必填 | 说明 |
|------|-----------|------|------|
| 节点 ID | `node_id` | 是 | 唯一标识 |
| 描述 | `description` | 否 | 功能说明 |
| AgentName | `agent_name` | 是 | 引用的系统 Agent 名称 |
| InputMapper | `input_mapper` | 否 | State → Agent 运行时状态映射 |
| OutputMapper | `output_mapper` | 否 | Agent 结果 → State 更新映射 |
| IsolatedMessages | `isolated_messages` | 否 | 是否隔离会话历史 |
| InputFromLastResponse | `input_from_last_response` | 否 | 上游 last_response → 下游 user_input |
| EventScope | `event_scope` | 否 | 子图事件作用域 |
| InterruptBefore | `interrupt_before` | 否 | 执行前中断 |
| InterruptAfter | `interrupt_after` | 否 | 执行后中断 |

**构建逻辑**：
```go
func buildAgentNode(sg *trpcgraph.StateGraph, n *NodeDef, agentResolver AgentResolver) error {
    agent, err := agentResolver.Resolve(n.AgentName)
    if err != nil {
        return fmt.Errorf("resolve agent %s: %w", n.AgentName, err)
    }
    opts := []trpcgraph.AddNodeOption{}
    if n.InputMapper != nil {
        opts = append(opts, trpcgraph.WithSubgraphInputMapper(toInputMapper(n.InputMapper)))
    }
    if n.OutputMapper != nil {
        opts = append(opts, trpcgraph.WithSubgraphOutputMapper(toOutputMapper(n.OutputMapper)))
    }
    if n.IsolatedMessages {
        opts = append(opts, trpcgraph.WithSubgraphIsolatedMessages())
    }
    if n.InputFromLastResponse {
        opts = append(opts, trpcgraph.WithSubgraphInputFromLastResponse())
    }
    if n.InterruptBefore { opts = append(opts, trpcgraph.WithInterruptBefore()) }
    if n.InterruptAfter { opts = append(opts, trpcgraph.WithInterruptAfter()) }
    sg.AddNode(n.NodeID, agent, opts...)
    return nil
}
```

#### 2.1.5 Router 节点

**框架映射**：`sg.AddConditionalEdges(sourceNode, condFunc, pathMap)`

Router 不是独立的 AddNode 调用，而是通过 AddConditionalEdges 在源节点上定义条件路由。

| 属性 | Proto 字段 | 必填 | 说明 |
|------|-----------|------|------|
| 节点 ID | `node_id` | 是 | 唯一标识（同时也是源节点） |
| 描述 | `description` | 否 | 路由逻辑说明 |
| CondFuncRef | `cond_func_ref` | 是 | 条件函数引用 |
| PathMap | `path_map` | 是 | 分支路径映射 `{label: targetNodeID}` |
| Destinations | `destinations` | 否 | 声明可能的动态路由目标 |

**构建逻辑**：
```go
func buildConditionalEdge(sg *trpcgraph.StateGraph, e *EdgeDef, reg *Registry) error {
    condFn, err := reg.ResolveCondFunc(e.CondFuncRef)
    if err != nil {
        return fmt.Errorf("resolve cond func %s: %w", e.CondFuncRef, err)
    }
    sg.AddConditionalEdges(e.FromNode, condFn, e.PathMap)
    return nil
}
```

#### 2.1.6 Join 节点

Join 节点不需要显式添加，由 BSP/DAG 引擎自动处理并行分支的汇聚。在前端画布中作为视觉标记存在。

### 2.2 边类型与流转规则

| 边类型 | 框架 API | Proto 定义 | 视觉表示 | 构建逻辑 |
|--------|----------|-----------|----------|----------|
| Runtime Edge | `AddEdge("A", "B")` | `EdgeDef{Type: "runtime", From: "A", To: "B"}` | 实线箭头 | `sg.AddEdge(from, to)` |
| Conditional Edge | `AddConditionalEdges("A", fn, map)` | `EdgeDef{Type: "conditional", From: "A", CondFuncRef: "...", PathMap: {...}}` | 虚线箭头 + 标签 | `sg.AddConditionalEdges(from, fn, pathMap)` |
| Command Edge | `WithEndsMap` + `Command.GoTo` | `NodeDef{Destinations: [...]}` | 动态，运行时决定 | `sg.AddNode(id, fn, WithEndsMap(...))` |

### 2.3 执行引擎选择

| 引擎 | 配置 | 适用场景 | 框架 API |
|------|------|----------|----------|
| BSP（默认） | `execution_engine: "bsp"` | 需要确定性、可复现 | 默认 Compile |
| DAG | `execution_engine: "dag"` | 高吞吐、无复杂状态交互 | `graph.WithExecutionEngine(graph.DAGEngine)` |

### 2.4 子图嵌套

Graph 节点可以是另一个 Graph（子图），支持层级化工作流设计：

```
父图 State ──InputMapper──▶ 子图 Agent 运行时状态
                              │
                         子图执行
                              │
子图结果 ──OutputMapper──▶ 父图 State 更新
```

**构建逻辑**（已有实现）：
```go
func buildSubgraphNode(sg *trpcgraph.StateGraph, n *NodeDef, subgraphResolver SubgraphResolver) error {
    subAgent, err := subgraphResolver.Resolve(n.SubgraphID)
    if err != nil { return err }
    opts := []trpcgraph.AddNodeOption{}
    if n.InputMapper != nil { opts = append(opts, trpcgraph.WithSubgraphInputMapper(...)) }
    if n.OutputMapper != nil { opts = append(opts, trpcgraph.WithSubgraphOutputMapper(...)) }
    sg.AddNode(n.NodeID, subAgent, opts...)
    return nil
}
```

---

## 三、维度二：动态拓扑与状态共享

> 现代 Agent 工作流通常不再是静态固定的。应支持条件路由和动态节点生成，同时所有 Agent 应能访问一个全局共享的工作流状态。

### 3.1 条件路由

**已实现**：`ConditionalEdgeDef` + `CondFuncRef` + Registry 解析。

**前端配置**：Router 节点属性面板支持：
1. 选择已注册的条件函数（CondFuncRef 下拉）
2. 编辑路径映射表（PathMap：标签 → 目标节点 ID）
3. 声明动态路由目标（Destinations 列表）

### 3.2 动态节点生成（Command.GoTo）

**框架机制**：节点执行时通过 `Command` 动态决定下一步。

**设计**：
- 节点属性面板新增 `Destinations` 字段，声明可能的动态路由目标
- 运行时 Command.GoTo 事件通过 WS `graph_node_custom` 推送到前端
- 前端执行监控中动态高亮实际执行的路径（与预定义路径对比）

**Proto 扩展**：
```protobuf
message NodeDef {
  // ... 现有字段 ...
  repeated string destinations = 20; // WithEndsMap 声明的动态路由目标
}
```

### 3.3 全局共享工作流状态（State Schema + Reducer）

**已实现**：`GraphBuildConfig.StateFields` 定义 State Schema。

**Reducer 类型映射**：

| Proto Reducer | 框架 Reducer | 语义 |
|---------------|-------------|------|
| `default` | `graph.DefaultReducer` | 完全替换旧值 |
| `append` | `graph.AppendReducer` | 追加到列表 |
| `cover` | `graph.CoverReducer` | 覆盖（仅非零值） |
| `merge` | `graph.MergeReducer` | 深度合并 Map |

**State Schema 编辑面板设计**：

```
┌─────────────────────────────────────┐
│ State Schema                        │
├─────────────────────────────────────┤
│ 字段名    类型      Reducer   默认值 │
│ ──────── ──────── ──────── ──────── │
│ input    string   default   ""      │
│ messages []any    append    []      │
│ counter  int      default   0       │
│ config   map      merge     {}      │
│                                     │
│ [+ 添加字段]                        │
└─────────────────────────────────────┘
```

**节点属性面板中的 State 字段引用**：

每个节点属性面板底部显示该节点读写的 State 字段（通过 InputMapper/OutputMapper 推断），帮助用户理解数据流。

### 3.4 State Schema 校验（P1 新增）

**校验逻辑**：在 `UpdateGraph` 时执行产品层校验：

```go
// internal/graph/trpc/validator.go 新建

type ValidationResult struct {
    Errors   []ValidationError `json:"errors"`
    Warnings []ValidationWarning `json:"warnings"`
}

type ValidationError struct {
    Code    string `json:"code"`    // "missing_field"/"type_mismatch"/"orphan_node"
    NodeID  string `json:"node_id,omitempty"`
    Field   string `json:"field,omitempty"`
    Message string `json:"message"`
}

func ValidateGraph(def *GraphDefinition) *ValidationResult
```

**校验项**：

| 校验项 | 优先级 | 逻辑 |
|--------|--------|------|
| State Schema 字段完整性 | P1 | 所有节点 InputMapper/OutputMapper 引用的字段已在 Schema 中定义 |
| Reducer 类型匹配 | P1 | AppendReducer 字段类型必须是切片；MergeReducer 字段类型必须是 Map |
| Agent 引用存在性 | P0 | Agent 节点引用的 Agent 在系统中存在 |
| 基础拓扑校验 | P0 | 入口点存在、无孤立节点、所有节点可达 |

---

## 四、维度三：人机协同与可观测性

> 企业级应用不仅需要工作流图在设计态清晰，更要求在运行态透明。内置人工审批节点、支持状态检查点与恢复以及全链路运行轨迹记录，是保障流程可控可回溯的关键。

### 4.1 人工审批节点（HITL）

**已实现**：`InterruptBefore/After` + `ResumeExecution` API。

**前端交互流程**：

```
Graph 执行 → 遇到 InterruptBefore/After 节点
  ↓
WS 推送 checkpoint 事件（含 interrupt_key, node_id）
  ↓
前端弹出确认对话框（显示节点信息、当前 State 快照）
  ↓
用户输入确认/拒绝 + 附加信息
  ↓
ResumeGraph API（传入 user_input）
  ↓
Graph 从中断点恢复执行
```

**HITL 确认对话框设计**：
```
┌─────────────────────────────────────┐
│ ⏸ 人工审批：审批节点                 │
├─────────────────────────────────────┤
│ 节点：approval_node                 │
│ 类型：Function                      │
│ 描述：请确认是否继续执行             │
│                                     │
│ 当前状态：                           │
│ ┌─────────────────────────────────┐ │
│ │ input: "处理申请 #12345"         │ │
│ │ status: "pending_review"        │ │
│ └─────────────────────────────────┘ │
│                                     │
│ 您的输入：                           │
│ ┌─────────────────────────────────┐ │
│ │ 批准，请继续执行                  │ │
│ └─────────────────────────────────┘ │
│                                     │
│        [拒绝]        [确认继续]      │
└─────────────────────────────────────┘
```

### 4.2 状态检查点与恢复（Checkpoint）

**已实现**：InMemory Checkpoint + TimeTravel API（ListCheckpoints/GetStateSnapshot/EditState）。

**待完善**：SQLite Checkpoint 持久化。

**SQLite Checkpoint Saver 注入**：

```go
// internal/data/graph.go

func NewGraphCheckpointSaver(dataDir string) trpcgraph.CheckpointSaver {
    dbPath := filepath.Join(dataDir, "graph_checkpoints.db")
    saver, err := trpcgraphsqlite.NewSQLiteCheckpointSaver(dbPath)
    if err != nil {
        log.Warnf("failed to create SQLite checkpoint saver, fallback to inmemory: %v", err)
        return trpcgraph.NewInMemoryCheckpointSaver()
    }
    return saver
}
```

Wire 注入：替换当前 InMemory Saver 为 SQLite Saver。

### 4.3 时间旅行调试（TimeTravel）

**已实现**：TimeTravel API（History/GetState/EditState）。

**前端时间线组件设计**：

```
┌──────────────────────────────────────────────────────────────────┐
│ 时间线                                                           │
├──────────────────────────────────────────────────────────────────┤
│ Step 0    Step 1      Step 2      Step 3      Step 4            │
│ ──●──────────●──────────●──────────●──────────●──               │
│ START     analyze    review     [中断]     publish              │
│           ✅完成     ✅完成     ⏸等待确认   ⏳等待               │
│                                                                  │
│ 当前查看：Step 3                                                  │
│ ┌──────────────────────────────────────────────────────────────┐ │
│ │ 节点：review_node                                            │ │
│ │ 状态快照：                                                   │ │
│ │   input: "处理申请 #12345"                                    │ │
│ │   review_result: "approved"                                  │ │
│ │   score: 0.95                                                │ │
│ └──────────────────────────────────────────────────────────────┘ │
│                                                                  │
│ [编辑状态]  [从此点重新执行]                                       │
└──────────────────────────────────────────────────────────────────┘
```

### 4.4 全链路运行轨迹

**已实现**：EventBridge + WS graph channel 实时推送节点状态。

**执行摘要设计**（P1 新增）：

Graph 执行完成后，WS 推送 `graph_execution_done` 事件，前端展示执行摘要：

```
┌─────────────────────────────────────┐
│ ✅ Graph 执行完成                    │
├─────────────────────────────────────┤
│ 总步骤：5                           │
│ 总耗时：12.3s                       │
│ 总 Token：1,234                     │
│ 节点执行详情：                       │
│   analyze    ✅  2.1s  500 tokens   │
│   review     ✅  3.5s  400 tokens   │
│   approval   ⏸  等待人工确认        │
│   publish    ✅  1.2s  334 tokens   │
│   notify     ✅  0.5s  0 tokens     │
└─────────────────────────────────────┘
```

**Proto 扩展**：
```protobuf
message ExecutionSummary {
  string execution_id = 1;
  int32 total_steps = 2;
  int64 total_duration_ns = 3;
  int64 total_tokens = 4;
  repeated NodeExecutionSummary nodes = 5;
}

message NodeExecutionSummary {
  string node_id = 1;
  string node_type = 2;
  string status = 3;
  int64 duration_ns = 4;
  int64 token_count = 5;
  string error = 6;
}
```

### 4.5 运行时操作

| 操作 | API | WS 事件触发 | 前端交互 |
|------|-----|------------|----------|
| HITL 确认 | `ResumeGraph` | `checkpoint` | 确认对话框 |
| 取消执行 | `CancelGraphExecution` | — | 取消按钮 |
| 重试失败节点 | `ResumeGraph`（从失败检查点） | `graph_node_error` | 重试按钮 |
| 修改状态 | `EditState` + `ResumeGraph` | — | 时间线编辑 |
| 时间旅行 | `GetStateSnapshot` | — | 时间线点击 |

---

## 五、维度四：设计辅助与资产复用

> 引入设计模式建议和资产复用是显著降低用户设计门槛、提升一致性的有效策略。同时，应提供完善的设计时校验功能。

### 5.1 设计时校验

**校验架构**：

```
前端保存/运行时
  ↓ UpdateGraph/ExecuteGraph API
后端 GraphUsecase
  ↓ ValidateGraph(def)
校验引擎 → ValidationResult
  ↓ 返回 errors/warnings
前端展示校验结果
```

**校验实现**：

```go
// internal/graph/trpc/validator.go

func ValidateGraph(def *GraphDefinition) *ValidationResult {
    result := &ValidationResult{}

    // P0: 基础拓扑校验
    validateTopology(def, result)

    // P0: Agent 引用校验
    validateAgentRefs(def, result)

    // P1: State Schema 校验
    validateStateSchema(def, result)

    // P1: 循环退出校验
    validateLoopExits(def, result)

    return result
}

func validateTopology(def *GraphDefinition, result *ValidationResult) {
    if len(def.Nodes) == 0 {
        result.Errors = append(result.Errors, ValidationError{
            Code:    "empty_graph",
            Message: "Graph 必须包含至少一个节点",
        })
        return
    }

    nodeSet := make(map[string]bool)
    for _, n := range def.Nodes {
        nodeSet[n.NodeID] = true
    }

    reachable := make(map[string]bool)
    var walk func(nodeID string)
    walk = func(nodeID string) {
        if reachable[nodeID] { return }
        reachable[nodeID] = true
        for _, e := range def.Edges {
            if e.FromNode == nodeID { walk(e.ToNode) }
        }
    }
    walk(def.EntryPoint)

    for id := range nodeSet {
        if !reachable[id] {
            result.Warnings = append(result.Warnings, ValidationWarning{
                Code:    "unreachable_node",
                NodeID:  id,
                Message: fmt.Sprintf("节点 %s 不可达", id),
            })
        }
    }
}

func validateAgentRefs(def *GraphDefinition, result *ValidationResult) {
    for _, n := range def.Nodes {
        if n.Type == "agent" && n.AgentName != "" {
            if !agentExists(n.AgentName) {
                result.Errors = append(result.Errors, ValidationError{
                    Code:    "agent_not_found",
                    NodeID:  n.NodeID,
                    Field:   "agent_name",
                    Message: fmt.Sprintf("Agent %s 不存在", n.AgentName),
                })
            }
        }
    }
}

func validateStateSchema(def *GraphDefinition, result *ValidationResult) {
    fieldSet := make(map[string]bool)
    for _, f := range def.StateFields {
        fieldSet[f.Name] = true
    }
    for _, n := range def.Nodes {
        for _, ref := range n.StateReads {
            if !fieldSet[ref] {
                result.Warnings = append(result.Warnings, ValidationWarning{
                    Code:    "undefined_state_field",
                    NodeID:  n.NodeID,
                    Field:   ref,
                    Message: fmt.Sprintf("节点 %s 读取的 State 字段 %s 未在 Schema 中定义", n.NodeID, ref),
                })
            }
        }
    }
}
```

**Proto 扩展**：
```protobuf
message ValidateGraphRequest {
  string graph_id = 1;
}

message ValidateGraphResponse {
  repeated ValidationError errors = 1;
  repeated ValidationWarning warnings = 2;
}

message ValidationError {
  string code = 1;
  string node_id = 2;
  string field = 3;
  string message = 4;
}

message ValidationWarning {
  string code = 1;
  string node_id = 2;
  string field = 3;
  string message = 4;
}

// GraphService 新增 RPC
rpc ValidateGraph(ValidateGraphRequest) returns (ValidateGraphResponse) {
  option (google.api.http) = {post: "/v1/graph/{graph_id}/validate" body: "*"};
}
```

### 5.2 设计模式模板

**模板数据结构**：

```go
// internal/graph/trpc/templates.go

type GraphTemplate struct {
    ID          string         `json:"id"`
    Name        string         `json:"name"`
    Description string         `json:"description"`
    Category    string         `json:"category"` // "pipeline"/"approval"/"parallel"/"loop"/"dispatch"/"nested"
    Nodes       []TemplateNode `json:"nodes"`
    Edges       []TemplateEdge `json:"edges"`
    StateFields []StateFieldDef `json:"state_fields"`
}

type TemplateNode struct {
    NodeID      string `json:"node_id"`
    Type        string `json:"type"`
    Label       string `json:"label"`
    Description string `json:"description"`
}

type TemplateEdge struct {
    FromNode string `json:"from_node"`
    ToNode   string `json:"to_node"`
    Type     string `json:"type"` // "runtime"/"conditional"
    Label    string `json:"label,omitempty"`
}
```

**内置模板**：

| 模板 | 拓扑 | 节点 | 边 |
|------|------|------|-----|
| 顺序流水线 | A→B→C→D | 4 Function | 3 Runtime |
| 审批流 | A→[审批]→B/C | 2 Function + 1 Router | 1 Runtime + 1 Conditional |
| 并行评审 | A→(B∥C∥D)→汇总 | 4 Function + 1 Join | 4 Runtime |
| 生成-评审循环 | A→B→[评分]→A/D | 2 Function + 1 Router | 2 Runtime + 1 Conditional |
| 条件分发 | A→[路由]→B/C/D | 1 Function + 1 Router | 1 Runtime + 1 Conditional |
| 子图嵌套 | A→[子工作流]→B | 1 Function + 1 Agent + 1 Function | 2 Runtime |

**模板 API**：

```protobuf
message ListGraphTemplatesRequest {}

message GraphTemplateInfo {
  string id = 1;
  string name = 2;
  string description = 3;
  string category = 4;
  string preview_dot = 5; // DOT 预览
}

message ListGraphTemplatesResponse {
  repeated GraphTemplateInfo templates = 1;
}

message CreateGraphFromTemplateRequest {
  string template_id = 1;
  string name = 2;
  string description = 3;
}

// GraphService 新增 RPC
rpc ListGraphTemplates(ListGraphTemplatesRequest) returns (ListGraphTemplatesResponse) {
  option (google.api.http) = {get: "/v1/graph/templates"};
}
rpc CreateGraphFromTemplate(CreateGraphFromTemplateRequest) returns (CreateGraphResponse) {
  option (google.api.http) = {post: "/v1/graph/from-template" body: "*"};
}
```

### 5.3 资产复用

| 资产类型 | 实现方式 | 优先级 |
|----------|----------|--------|
| Graph 模板 | 内置模板 + 用户自定义模板 | P1 |
| Agent 仓库 | 系统已有 Agent 目录，Graph 节点直接引用 | ✅ 已有 |
| 子图复用 | 将常用流程片段封装为子图，跨 Graph 复用 | P1 |
| Graph 版本管理 | Graph 定义版本化，支持回滚 | P2 |
| 导入/导出 | Graph 定义 JSON 导入导出 | P2 |

---

## 六、后端架构

### 6.1 模块结构

```
internal/graph/trpc/
├── builder.go        # GraphBuildConfig → BuildStateGraph → GraphAgent
├── registry.go       # NodeFunc/CondFunc 注册表
├── checkpoint.go     # SQLite Checkpoint Saver 适配器
├── event_bridge.go   # Graph 事件 → EventBus 桥接器
├── visualize.go      # DOT 解析 + 结构化 JSON
├── validator.go      # 设计时校验引擎（新增）
└── templates.go      # 设计模式模板（新增）
```

### 6.2 GraphUsecase 完整方法

| 方法 | 说明 | 状态 |
|------|------|------|
| `CreateGraph` | 创建 Graph 定义 | ✅ |
| `GetGraph` | 获取 Graph 定义 | ✅ |
| `UpdateGraph` | 更新 Graph 定义 | ✅ |
| `DeleteGraph` | 删除 Graph | ✅ |
| `ListGraphs` | 列出 Graph | ✅ |
| `ExecuteGraph` | 执行 Graph（含 EventBridge） | ✅ |
| `ResumeExecution` | 恢复中断的执行 | ✅ |
| `CancelExecution` | 取消执行 | ✅ |
| `ListExecutions` | 列出执行记录 | ✅ |
| `ListCheckpoints` | 列出检查点 | ✅ |
| `GetStateSnapshot` | 获取状态快照 | ✅ |
| `EditState` | 编辑状态 | ✅ |
| `VisualizeGraph` | 可视化（结构化 JSON） | ✅ |
| `ValidateGraph` | 设计时校验 | ❌ P0 新增 |
| `ListGraphTemplates` | 列出设计模式模板 | ❌ P1 新增 |
| `CreateGraphFromTemplate` | 从模板创建 Graph | ❌ P1 新增 |

### 6.3 事件桥接器（已实现）

**映射规则**：

| trpc event.Object | EnvelopeType | Metadata 提取 |
|-------------------|--------------|---------------|
| `graph.node.start` | `graph_node_start` | NodeExecutionMetadata |
| `graph.node.complete` | `graph_node_end` | NodeExecutionMetadata |
| `graph.node.error` | `graph_node_error` | NodeExecutionMetadata |
| `graph.node.custom` | `graph_node_custom` | NodeCustomEventMetadata |
| `graph.pregel.step` | `graph_step` | PregelStepMetadata |
| `graph.checkpoint.interrupt` | `checkpoint` | PregelStepMetadata |
| `graph.checkpoint.created` | `checkpoint` | PregelStepMetadata |
| `graph.state.update` | `state_delta` | StateUpdateMetadata |
| `graph.execution`（done） | `graph_execution_done` | CompletionMetadata |

### 6.4 Checkpoint API（已实现）

| RPC | HTTP | 说明 |
|-----|------|------|
| `ListCheckpoints` | `GET /v1/graph/executions/{id}/checkpoints` | 列出检查点 |
| `GetStateSnapshot` | `GET /v1/graph/executions/{id}/state-snapshot` | 获取状态快照 |
| `EditState` | `POST /v1/graph/executions/{id}/edit-state` | 编辑状态 |

---

## 七、前端设计

### 7.1 页面结构

```
/graphs                         → Graph 列表页
/graphs/:id                     → Graph 编辑器页（编辑模式）
/graphs/:id/run/:execId         → Graph 执行监控页（运行模式）
```

### 7.2 Graph 编辑器页布局

```
┌──────────┬───────────────────────────┬──────────────┐
│ 组件面板  │        画布区域           │  属性面板    │
│          │                           │              │
│ Function │   ┌───┐   ┌───┐          │ 节点ID       │
│ LLM      │   │ A │──▶│ B │          │ 类型         │
│ Tool     │   └───┘   └─┬─┘          │ 指令         │
│ Agent    │             │             │ 模型         │
│ Router   │         ┌───▼───┐         │ 工具         │
│ Join     │         │   C   │         │ 中断设置     │
│          │         └───────┘         │              │
│ ──────── │                           │ ──────────── │
│ State    │                           │ State Schema │
│ Schema   │                           │ 字段列表     │
│          │                           │              │
│ ──────── │                           │ ──────────── │
│ 模板     │                           │ 校验结果     │
└──────────┴───────────────────────────┴──────────────┘
```

### 7.3 节点样式

| NodeType | 形状 | 填充色 | 边框色 | Vue Flow 样式 |
|----------|------|--------|--------|--------------|
| LLM | 矩形 | `#e3f2fd` | `#2196f3` | 蓝色边框节点 |
| Tool | 矩形 | `#fff3e0` | `#ff9800` | 橙色边框节点 |
| Agent | 矩形 | `#e8f5e9` | `#4caf50` | 绿色边框节点 |
| Router | 菱形 | `#eeeeee` | `#757575` | 灰色菱形节点 |
| Join | 菱形 | `#f3e5f5` | `#9c27b0` | 紫色菱形节点 |
| Function | 矩形 | `#f3e5f5` | `#9c27b0` | 紫色边框节点 |

### 7.4 执行状态样式

| 状态 | 节点样式 | 说明 |
|------|----------|------|
| idle | 默认样式 | 未执行 |
| running | 脉冲动画 + 蓝色光晕 | 正在执行 |
| completed | 绿色勾标记 | 执行完成 |
| failed | 红色叉标记 + 红色边框 | 执行失败 |
| interrupted | 黄色暂停标记 | HITL 中断 |
| waiting | 灰色 | 等待执行 |

### 7.5 WS 事件处理

```typescript
interface GraphNodeState {
  nodeId: string
  status: 'idle' | 'running' | 'completed' | 'failed' | 'interrupted' | 'waiting'
  startTime?: string
  endTime?: string
  duration?: number
  error?: string
}

function handleGraphEvent(envelope: Envelope) {
  switch (envelope.type) {
    case 'graph_node_start':
      updateNodeState(envelope.metadata.node_id, 'running', { startTime: envelope.timestamp })
      break
    case 'graph_node_end':
      updateNodeState(envelope.metadata.node_id, 'completed', {
        endTime: envelope.timestamp,
        duration: envelope.metadata.duration_ns / 1e6,
      })
      break
    case 'graph_node_error':
      updateNodeState(envelope.metadata.node_id, 'failed', { error: envelope.metadata.error })
      break
    case 'checkpoint':
      showInterruptDialog(envelope.metadata)
      break
    case 'graph_execution_done':
      showExecutionSummary(envelope.metadata)
      break
  }
}
```

---

## 八、数据流

### 8.1 设计态数据流

```
前端 Vue Flow 编辑器
  ↓ 拖拽节点/连线/配置属性
  ↓ CreateGraph/UpdateGraph API (HTTP)
后端 GraphUsecase → ValidateGraph(def)
  ↓ 校验通过
GraphRepo (Ent/SQLite) 持久化
```

### 8.2 运行态数据流

```
前端 ExecuteGraph 按钮
  ↓ ExecuteGraph API (HTTP)
后端 GraphUsecase → BuildStateGraph → GraphAgent.Run()
  ↓ trpc-agent-go event.Event channel
EventBridge → event.Envelope → EventBus.Publish()
  ↓ WS graph channel
前端 WS 客户端 → 节点状态更新 → Vue Flow 画布
```

### 8.3 HITL 数据流

```
Graph 执行 → 遇到 InterruptBefore/After
  ↓ EventBridge → WS checkpoint 事件
前端弹出确认对话框
  ↓ 用户输入
ResumeGraph API (HTTP)
  ↓ GraphUsecase → Executor.Resume(checkpoint, userInput)
Graph 从中断点恢复执行
  ↓ EventBridge → WS graph_node_start 事件
前端更新节点状态
```

### 8.4 TimeTravel 数据流

```
前端时间线点击检查点
  ↓ GetStateSnapshot API (HTTP)
后端 GraphUsecase → TimeTravel.GetState(ref)
  ↓ 返回 StateSnapshot
前端展示历史状态

用户编辑状态
  ↓ EditState API (HTTP)
后端 GraphUsecase → TimeTravel.EditState(ref, patch)
  ↓ 返回新 CheckpointRef
用户选择从此点重新执行
  ↓ ResumeGraph API (HTTP)
Graph 从编辑点恢复执行
```

---

## 九、实现步骤

### 阶段一：P0 核心能力补全

#### 步骤 1：设计时校验引擎

1. 新建 `internal/graph/trpc/validator.go`：实现 ValidateGraph
2. Proto 新增 `ValidateGraph` RPC
3. Service 层实现校验端点
4. 前端编辑器保存时调用校验，展示错误/警告

#### 步骤 2：Checkpoint SQLite 持久化

1. 修改 `internal/data/graph.go`：注入 SQLite Checkpoint Saver 替换 InMemory
2. Wire 注入更新
3. 验证重启后 Checkpoint 可恢复

#### 步骤 3：Agent 引用校验

1. 在 validator.go 中实现 validateAgentRefs
2. 集成 Agent 目录查询

### 阶段二：P1 增强能力

#### 步骤 4：设计模式模板

1. 新建 `internal/graph/trpc/templates.go`：定义 6 种内置模板
2. Proto 新增 `ListGraphTemplates`/`CreateGraphFromTemplate` RPC
3. 前端模板选择面板

#### 步骤 5：节点属性配置完善

1. LLM 节点：Instruction + Model + Tools 配置面板
2. Tool 节点：ToolNames + ParallelTools 配置面板
3. Agent 节点：InputMapper/OutputMapper 可视化配置
4. 节点重试配置：RetryPolicy 属性面板

#### 步骤 6：State Schema 校验

1. 在 validator.go 中实现 validateStateSchema
2. 前端 State Schema 编辑面板实时校验

#### 步骤 7：执行摘要与时间线

1. Proto 新增 ExecutionSummary 消息
2. 前端执行摘要组件
3. 前端时间线组件

#### 步骤 8：子图复用

1. 支持将常用流程片段封装为子图
2. 子图跨 Graph 引用

### 阶段三：P2 高级能力

#### 步骤 9：Graph 版本管理

1. Graph 定义版本化
2. 版本回滚

#### 步骤 10：导入/导出

1. Graph 定义 JSON 导入导出

#### 步骤 11：节点缓存配置

1. CachePolicy + CacheKeyFields 属性面板

---

## 十、涉及文件汇总

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/graph/trpc/validator.go` | 新建 | 设计时校验引擎（拓扑/Agent引用/StateSchema/循环） |
| `internal/graph/trpc/templates.go` | 新建 | 6 种内置设计模式模板 |
| `internal/graph/trpc/builder.go` | 修改 | 扩展节点类型构建逻辑（LLM/Tool/Agent 属性解析） |
| `internal/graph/trpc/event_bridge.go` | 已实现 | Graph 事件 → EventBus 桥接器 |
| `internal/graph/trpc/visualize.go` | 已实现 | DOT 解析 + 结构化 JSON |
| `internal/graph/trpc/checkpoint.go` | 已实现 | SQLite Checkpoint Saver |
| `internal/graph/trpc/registry.go` | 已实现 | NodeFunc/CondFunc 注册表 |
| `internal/biz/graph.go` | 修改 | 新增 ValidateGraph/ListGraphTemplates/CreateGraphFromTemplate |
| `internal/data/graph.go` | 修改 | 注入 SQLite Checkpoint Saver |
| `internal/service/graph.go` | 修改 | 新增校验/模板 RPC 方法 |
| `api/kratos/graph/v1/graph.proto` | 修改 | 新增校验/模板/摘要消息和 RPC |
| `internal/event/envelope.go` | 已实现 | Graph 事件类型 |
| `web/src/features/graph/types.ts` | 修改 | 新增模板/校验/摘要类型 |
| `web/src/features/graph/api.ts` | 修改 | 新增模板/校验 API |
| `web/src/components/graph/GraphPropertyPanel.vue` | 修改 | 完善各节点类型属性面板 |
| `web/src/components/graph/GraphTemplatePanel.vue` | 新建 | 模板选择面板 |
| `web/src/components/graph/GraphTimeline.vue` | 新建 | 时间线组件 |
| `web/src/components/graph/GraphValidationPanel.vue` | 新建 | 校验结果面板 |

---

## 十一、任务派工与执行规则设计

> 对应需求 §10-§14，设计任务派工、状态生命周期、审核门禁、可观测性、超时重试、对外集成等扩展能力。

### 11.1 Agent 角色与动态派工

**数据模型扩展**：

```go
type AgentRole struct {
    Name        string
    Parent      string
    Description string
}

type AgentProfile struct {
    AgentKey    string
    Roles       []AgentRole
    Capabilities []string
    MaxConcurrentTasks int
    CurrentTaskCount   int
}
```

**NodeDef 扩展**：

```protobuf
message NodeDef {
  // ... 现有字段 ...
  string required_role = 20;       // 所需角色（动态指派）
  string assignment_mode = 21;     // "static" | "dynamic"
  string assignment_strategy = 22; // "least_tasks" | "random" | "manual"
}
```

**派工流程**：

```
节点执行 → 检查 assignment_mode
  ├── static → 直接使用 agent_name
  └── dynamic → 按 required_role 从 Team 中匹配 Agent
        ├── 唯一匹配 → 指派
        ├── 多人匹配 → 按 assignment_strategy 选择
        └── 无匹配 → 任务进入 pending_assignment，发送通知
```

**安全原则**：无匹配 Agent 时，任务状态为 `pending_assignment`，通过 WS `task_pending_assignment` 事件通知前端，不随机指派。

### 11.2 任务模型与状态机

**数据模型**：

```go
type Task struct {
    TaskID      string
    NodeID      string
    ExecutionID string
    Assignee    string
    Status      TaskStatus
    Context     json.RawMessage
    Input       json.RawMessage
    Output      json.RawMessage
    Summary     string
    Metadata    json.RawMessage
    CreatedAt   time.Time
    ClaimedAt   *time.Time
    CompletedAt *time.Time
}
```

**状态机转换**：

```
pending ──ClaimTask──▶ claimed ──SubmitResult──▶ complete
                               ──ReportBlocked──▶ blocked ──Resume──▶ claimed
                               ──SubmitForReview──▶ review_required ──Approve──▶ complete
                                                                          ──Reject──▶ claimed
                               ──Fail──▶ failed ──Retry──▶ claimed
                                                    ──Cancel──▶ cancelled
                               ──Timeout──▶ timed_out ──Retry──▶ claimed
```

**Proto 扩展**：

```protobuf
enum TaskStatus {
  TASK_PENDING = 0;
  TASK_CLAIMED = 1;
  TASK_COMPLETE = 2;
  TASK_BLOCKED = 3;
  TASK_REVIEW_REQUIRED = 4;
  TASK_FAILED = 5;
  TASK_TIMED_OUT = 6;
  TASK_CANCELLED = 7;
  TASK_CRASHED = 8;
}

message Task {
  string task_id = 1;
  string node_id = 2;
  string execution_id = 3;
  string assignee = 4;
  TaskStatus status = 5;
  string context = 6;
  string input = 7;
  string output = 8;
  string summary = 9;
  string metadata = 10;
  string created_at = 11;
  string claimed_at = 12;
  string completed_at = 13;
}
```

### 11.3 审核与质量门禁

**审核节点设计**：

- **人工审核**：复用 HITL InterruptBefore/After 机制
- **自动审核（Reviewer Agent）**：新增 `reviewer` 节点类型，引用一个 Reviewer Agent 对上游输出进行审核

```protobuf
message NodeDef {
  // ... 现有字段 ...
  string reviewer_agent = 23;     // Reviewer Agent 名称
  string review_rules = 24;       // 审核规则（JSON）
}
```

**评论模型**：

```protobuf
message TaskComment {
  string comment_id = 1;
  string task_id = 2;
  string author = 3;
  string content = 4;
  string type = 5;    // "approve" | "reject" | "suggestion"
  string created_at = 6;
}
```

### 11.4 全链路可观测性

**结构化日志**：

```protobuf
message TaskLog {
  string task_id = 1;
  string stream = 2;    // "stdout" | "stderr"
  string content = 3;
  string level = 4;     // "debug" | "info" | "warn" | "error"
  string timestamp = 5;
}
```

**运行历史（task_run）**：

```protobuf
message TaskRun {
  string run_id = 1;
  string task_id = 2;
  string started_at = 3;
  string finished_at = 4;
  int32 exit_code = 5;
  string log_ref = 6;   // 日志引用
}
```

**事件追踪（task_events）**：

```protobuf
message TaskEvent {
  string event_id = 1;
  string task_id = 2;
  string event_type = 3;  // "task_created" | "task_claimed" | "heartbeat" | "task_completed" | ...
  string source_node = 4;
  string description = 5;
  string timestamp = 6;
}
```

### 11.5 智能超时与重试

**心跳感知超时**：

```protobuf
message NodeDef {
  // ... 现有字段 ...
  int32 timeout_seconds = 25;        // 最大执行时间
  int32 heartbeat_interval_seconds = 26; // 心跳间隔
  bool enable_lease_extension = 27;  // 是否允许租约延长
}
```

**熔断策略**：

```protobuf
message CircuitBreakerPolicy {
  int32 failure_threshold = 1;  // 连续失败阈值
  int32 reset_timeout_seconds = 2; // 熔断恢复超时
  string fallback_node = 3;    // 熔断后执行的补偿节点
}
```

### 11.6 对外集成

**Agent 交互 API**：

```protobuf
service GraphService {
  // ... 现有 RPC ...
  rpc ClaimTask(ClaimTaskRequest) returns (ClaimTaskResponse);
  rpc Heartbeat(HeartbeatRequest) returns (HeartbeatResponse);
  rpc SubmitTaskResult(SubmitTaskResultRequest) returns (SubmitTaskResultResponse);
  rpc ReportBlocked(ReportBlockedRequest) returns (ReportBlockedResponse);
}
```

**Webhook 配置**：

```protobuf
message WebhookConfig {
  string url = 1;
  string event_type = 2;   // "task_completed" | "task_failed" | "checkpoint_interrupt" | ...
  map<string, string> headers = 3;
}
```

---

## 十二、分阶段实现步骤

### P0（当前）

| 项目 | 状态 |
|------|------|
| 图结构基础（节点/边/State Schema） | ✅ 已实现 |
| Agent 节点引用 | ✅ 已实现 |
| 条件路由 | ✅ 已实现 |
| HITL 中断/恢复 | ✅ 已实现 |
| Checkpoint API | ✅ 已实现 |
| 事件桥接 | ✅ 已实现 |
| 设计时校验引擎 | ✅ 已实现 |
| 设计模式模板 | ✅ 已实现 |

### P1（下一阶段）

| 项目 | 说明 |
|------|------|
| 任务模型与状态机 | Task CRUD + 状态转换 + 强制完成回调 |
| Agent 角色与动态派工 | AgentProfile + required_role + assignment_mode |
| 审核节点 | Reviewer Agent + 评论反馈 |
| 结构化日志 | TaskLog + 日志视图 |
| 运行历史 | TaskRun + 历史查看 |
| 心跳感知超时 | heartbeat + 租约延长 |
| 前端模板/校验面板 | GraphTemplatePanel + GraphValidationPanel |
| 前端执行监控 | 节点状态高亮 + 甘特图/时间线 |

### P2（远期）

| 项目 | 说明 |
|------|------|
| Agent 交互 API | ClaimTask/Heartbeat/SubmitResult/ReportBlocked |
| Webhook 通知 | 节点级 Webhook 配置 |
| 自定义执行器 | spawn_fn 插件机制 |
| 熔断策略 | CircuitBreakerPolicy |
| 事件追踪 | TaskEvent + 事件流查询 |
| Graph 版本管理 | 定义版本化 + 回滚 |
| 用户自定义模板 | 保存已有 Graph 为模板 |
| 时间旅行 UI | 检查点时间线 + 状态编辑重放 |
