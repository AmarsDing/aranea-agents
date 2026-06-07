# Graph 工作流模块 — 实现设计文档（v3）

> 对应需求：`36 graph-workflow.md`（v3 四维需求架构）
> 本文档范围：架构方案、接口设计、数据模型。不含实现步骤（见开发计划）和用户故事（见需求文档）。

---

## 一、模块概述

### 1.1 设计定位

Graph 工作流引擎：基于 trpc-agent-go `graph` 包，构建"LangGraph for Go"级别的确定性工作流系统。

核心命题：**Agent 节点内部自主推理，节点间流转由确定性图规则控制**。Team 解决"多 Agent 如何协作"，Graph 解决"复杂流程如何确定性地执行"。

### 1.2 现有基础

| 组件 | 文件 | 状态 |
|------|------|------|
| Graph 构建器 | `internal/graph/trpc/builder.go` | ✅ BuildStateGraph + GraphAgent + StateSchema/Reducer + 子图 + DAG |
| 节点接线 | `internal/graph/trpc/node_wiring.go` | ✅ LLM/Tool/Agent/Function/Router/Task/Review 节点接线 + Mapper + RetryPolicy + CachePolicy + WithEndsMap |
| 函数注册表 | `internal/graph/trpc/registry.go` | ✅ NodeFunc/CondFunc 注册表 |
| Checkpoint 适配器 | `internal/graph/trpc/checkpoint.go` | ✅ InMemory + SQLite Saver |
| 事件桥接器 | `internal/graph/trpc/event_bridge.go` | ✅ 9 种 ObjectType 映射 + ExecutionSummary 集成 |
| 执行摘要 | `internal/graph/trpc/execution_summary.go` | ✅ ExecutionSummaryTracker + NodeExecutionSummary |
| 可视化解析 | `internal/graph/trpc/visualize.go` | ✅ DOT 解析 + 结构化 JSON |
| 设计时校验 | `internal/graph/trpc/validator.go` | ✅ 拓扑/Agent引用/StateSchema/循环 |
| 设计模式模板 | `internal/graph/trpc/templates.go` | ✅ 6 种内置模板 + TemplateToBuildConfig |
| 版本管理 | `internal/biz/graph_version.go` | ✅ metadata._version_history 快照 + ListGraphVersions/RollbackGraphVersion |
| 运行时适配器 | `internal/graph/adapter/runtime_adapter.go` | ✅ GraphBuilderFactory + BuildAndRun/Resume/Visualize/Validate |
| 业务层 | `internal/biz/graph.go` + `graph_definition_usecase.go` | ✅ CRUD + Execute + Resume + Cancel + TimeTravel + Checkpoint + Visualize + Validate + Templates + Export/Import + Versions + SaveAsTemplate |
| 业务运行时 | `internal/biz/graph_runtime.go` | ✅ GraphRuntime 接口 + GraphBuilderFactory 接口 |
| 任务系统 | `internal/biz/task.go` | ✅ TaskUsecase + 状态机 + Claim/Submit/Heartbeat/Review/Timeout |
| 任务 Webhook | `internal/service/graph_task_runtime.go` | ✅ GraphTaskRuntime → dispatchGraphTaskWebhook |
| 数据层 | `internal/data/graph.go` + `task.go` | ✅ GraphRepo + TaskRepo + Ent 持久化 |
| 服务层 | `internal/service/graph_*.go` | ✅ 42 个方法（15 定义 + 9 执行 + 16 任务 + 2 异步） |
| Proto 定义 | `api/kratos/graph/v1/graph.proto` | ✅ 32 个 RPC 端点 |
| 前端类型 | `web/src/features/graph/types.ts` | ✅ 完整类型定义（含 Task/Version/ExecutionSummary 类型） |
| 前端 API | `web/src/features/graph/api.ts` | ✅ 完整 API 客户端（含 Export/Import/Versions/SaveAsTemplate） |
| 前端组件 | `web/src/components/graph/` | ✅ 22 个组件（Vue Flow 画布 + Run/HITL/Validation/Template/Checkpoint/TimeTravel/Kanban/Version） |
| 前端运行态 | `web/src/features/graph/runtime/` | ✅ useGraphExecutionStream + useGraphTimeTravel + 投影逻辑 |
| 前端编辑态 | `web/src/features/graph/editor/` | ✅ graphLayout 布局持久化 + dagre 自动布局 |
| 前端资产 | `web/src/features/graph/useGraphEditorAssets.ts` | ✅ 导入/导出/版本/模板 composable |

### 1.3 本期设计目标

| 维度 | 当前状态 | 目标 |
|------|----------|------|
| 图结构与混合控制 | ✅ 全部节点类型已接线（LLM/Tool/Agent/Function/Router/Join） | 补全子图嵌套编辑器 UI |
| 动态拓扑与状态共享 | ✅ 条件路由/State Schema/校验/Command.GoTo 已实现 | 补全动态任务节点插入（BabyAGI 模式） |
| 人机协同与可观测性 | ✅ HITL/Checkpoint/EventBridge/Task/ExecutionSummary 已实现 | 补全 Token 用量追踪 |
| 设计辅助与资产复用 | ✅ 校验/模板/版本管理/导入导出/用户自定义模板已实现 | 补全熔断策略接入 NodeDef |

---

## 二、架构总览

### 2.1 分层架构

```
┌──────────────────────────────────────────────────────────────────┐
│                        前端（Vue Flow）                          │
│  GraphsPage / GraphEditorPage / GraphRunPage                    │
├──────────────────────────────────────────────────────────────────┤
│                     服务层（Kratos gRPC/HTTP）                    │
│  GraphService: 28 RPC 方法（CRUD + Execute + Task + Validate）  │
├──────────────────────────────────────────────────────────────────┤
│                      业务层（Biz）                                │
│  GraphUsecase: 图定义管理 + 执行编排                             │
│  TaskUsecase: 任务生命周期 + 派工 + 审核 + 超时                   │
├──────────────────────────────────────────────────────────────────┤
│                    图引擎层（Graph/Trpc）                         │
│  Builder → StateGraph → GraphAgent → Executor                   │
│  Registry | Checkpoint | EventBridge | Validator | Templates    │
├──────────────────────────────────────────────────────────────────┤
│                     数据层（Data/Ent）                            │
│  GraphRepo | TaskRepo | Ent Schemas (7 tables)                  │
├──────────────────────────────────────────────────────────────────┤
│                    基础设施（Infra）                               │
│  SQLite | EventBus | WebSocket | trpc-agent-go/graph             │
└──────────────────────────────────────────────────────────────────┘
```

### 2.2 核心数据流

**设计态**：前端 → CreateGraph/UpdateGraph API → GraphUsecase → ValidateGraph → GraphRepo 持久化

**运行态**：前端 → ExecuteGraph API → GraphUsecase → BuildStateGraph → GraphAgent.Run() → EventBridge → WS → 前端

**HITL**：Graph 遇到 Interrupt → EventBridge → WS checkpoint 事件 → 前端确认对话框 → ResumeGraph API → 恢复执行

**TimeTravel**：GetStateSnapshot API → TimeTravel.GetState → 返回快照 → EditState + Resume 从编辑点重新执行

---

## 三、维度一：图结构与混合控制

### 3.1 节点类型体系

每种节点类型对应不同的框架 API 和属性配置：

| 节点类型 | 框架 API | 必填属性 | 状态 |
|----------|----------|----------|------|
| Function | `AddNode(id, fn, opts...)` | FuncRef | ✅ 已实现 |
| LLM | `AddLLMNode(id, instruction, model, opts...)` | Instruction + Model | ✅ 已实现（`node_wiring.go`） |
| Tool | `AddToolsNode(id, toolMap, opts...)` | ToolNames | ✅ 已实现（`node_wiring.go`） |
| Agent | `AddNode(id, agent, opts...)` | AgentName | ✅ 已实现（含 InputMapper/OutputMapper，`node_wiring.go`） |
| Router | `AddConditionalEdges(src, fn, pathMap)` | CondFuncRef + PathMap | ✅ 已实现 |
| Join | BSP/DAG 引擎自动处理 | 无 | ✅ 已实现 |

### 3.2 节点属性定义（NodeDef）

```protobuf
message NodeDef {
  string node_id = 1;
  string type = 2;              // "function"|"llm"|"tool"|"agent"|"router"|"join"
  string description = 3;

  // Function 节点
  string func_ref = 4;

  // LLM 节点
  string instruction = 5;
  string model = 6;
  repeated string tool_names = 7;
  string user_input_key = 8;

  // Tool 节点
  bool enable_parallel_tools = 9;

  // Agent 节点
  string agent_name = 10;
  string input_mapper = 11;     // JSON: State → Agent 运行时状态映射
  string output_mapper = 12;    // JSON: Agent 结果 → State 更新映射
  bool isolated_messages = 13;
  bool input_from_last_response = 14;

  // Router 节点
  string cond_func_ref = 15;
  map<string, string> path_map = 16;
  repeated string destinations = 17;  // Command.GoTo 动态路由目标

  // 通用
  bool interrupt_before = 18;
  bool interrupt_after = 19;

  // 任务派工
  string required_role = 20;
  string assignment_mode = 21;     // "static"|"dynamic"
  string assignment_strategy = 22; // "least_tasks"|"random"|"manual"

  // 审核
  string reviewer_agent = 23;
  string review_rules = 24;

  // 超时
  int32 timeout_seconds = 25;
  int32 heartbeat_interval_seconds = 26;
  bool enable_lease_extension = 27;

  // 重试策略（扁平字段，非嵌套 message）
  int32 retry_max_attempts = 20;   // > 0 时启用 WithSimpleRetry
  string failure_action = 28;      // "skip"|"retry_then_block"|"fail_fast"
  string fallback_agent = 29;

  // 缓存策略（扁平字段）
  bool cache_enabled = 23;
  int32 cache_ttl_seconds = 24;
}
```

> **设计说明**：RetryPolicy 和 CachePolicy 在 Proto 中以扁平字段实现（`retry_max_attempts`/`failure_action`/`fallback_agent`/`cache_enabled`/`cache_ttl_seconds`），而非嵌套 message。Builder 层 `node_wiring.go` 读取这些字段并调用 `WithRetryPolicy`/`WithNodeCachePolicy`。

### 3.3 边类型

| 边类型 | 框架 API | Proto 定义 | 视觉表示 |
|--------|----------|-----------|----------|
| Runtime Edge | `AddEdge(from, to)` | `EdgeDef{from, to}` | 实线箭头 |
| Conditional Edge | `AddConditionalEdges(src, fn, pathMap)` | `ConditionalEdgeDef{from, cond_func_ref, path_map}` | 虚线箭头 + 标签 |
| Command Edge | `WithEndsMap` + `Command.GoTo` | `NodeDef.destinations` | 动态，运行时决定 |

### 3.4 执行引擎

| 引擎 | 配置 | 框架 API | 适用场景 |
|------|------|----------|----------|
| BSP（默认） | `execution_engine: "bsp"` | 默认 Compile | 确定性、可复现 |
| DAG | `execution_engine: "dag"` | `WithExecutionEngine(DAGEngine)` | 高吞吐、无复杂状态交互 |

### 3.5 子图嵌套

子图编译后作为 Agent 节点注册，状态通过 InputMapper/OutputMapper 映射：

```
父图 State ──InputMapper──▶ 子图 Agent 运行时状态
                              │
                         子图执行
                              │
子图结果 ──OutputMapper──▶ 父图 State 更新
```

子图支持 `WithSubgraphIsolatedMessages`（隔离会话历史）和 `WithSubgraphInputFromLastResponse`（上游 last_response → 下游 user_input）。

---

## 四、维度二：动态拓扑与状态共享

### 4.1 条件路由（✅ 已实现）

Router 节点通过 `AddConditionalEdges` 定义条件路由，CondFuncRef 引用 Registry 中注册的条件函数，PathMap 定义分支映射。

### 4.2 动态路由（Command.GoTo）（✅ 已实现）

节点 `Destinations` 字段声明可能的动态路由目标，运行时通过 `Command.GoTo` 决定实际路径。`node_wiring.go` 中 `WithEndsMap` 已接线。

### 4.3 State Schema + Reducer（✅ 已实现）

| Reducer 类型 | 框架映射 | 语义 |
|-------------|---------|------|
| `default` | `DefaultReducer` | 完全替换旧值 |
| `append` | `AppendReducer` | 追加到列表 |
| `cover` | `CoverReducer` | 覆盖（仅非零值） |
| `merge` | `MergeReducer` | 深度合并 Map |

State Schema 定义在 `GraphBuildConfig.StateFields` 中，Builder 通过 `AddStateField` 注册到 StateGraph。

---

## 五、维度三：人机协同与可观测性

### 5.1 人工审批（HITL）（✅ 已实现）

节点配置 `InterruptBefore/After`，执行到中断点时 EventBridge 推送 WS `checkpoint` 事件，前端弹出确认对话框，用户通过 `ResumeGraph` API 恢复执行。

### 5.2 Checkpoint + TimeTravel（✅ 已实现）

| API | 说明 |
|-----|------|
| `ListCheckpoints` | 列出检查点 |
| `GetStateSnapshot` | 获取状态快照 |
| `EditState` | 编辑状态（创建新分支） |
| `TimeTravelGraph` | 回放到指定步骤 |

Checkpoint Saver 支持 InMemory 和 SQLite 两种后端，通过 `checkpoint.go` 适配器切换。

### 5.3 事件桥接（✅ 已实现）

| trpc event.Object | EnvelopeType | 说明 |
|-------------------|--------------|------|
| `graph.node.start` | `graph_node_start` | 节点开始执行 |
| `graph.node.complete` | `graph_node_end` | 节点执行完成 |
| `graph.node.error` | `graph_node_error` | 节点执行失败 |
| `graph.node.custom` | `graph_node_custom` | 自定义事件（Command.GoTo） |
| `graph.pregel.step` | `graph_step` | Pregel 步骤完成 |
| `graph.checkpoint.interrupt` | `checkpoint` | 中断事件 |
| `graph.checkpoint.created` | `checkpoint` | 检查点创建 |
| `graph.state.update` | `state_delta` | 状态更新 |
| `graph.execution`（done） | `graph_execution_done` | 执行完成 |

### 5.4 ExecutionSummary（✅ 已实现）

Graph 执行完成后推送 `graph_execution_done` 事件，包含执行摘要。实现位于 `execution_summary.go`（Go 结构体）+ `event_bridge.go`（WS 集成）。

> **注意**：`ExecutionSummary`/`NodeExecutionSummary` 仅存在于 Go 代码中，未映射为 Proto message。前端通过 WS `graph_execution_done` 事件的 `metadata.execution_summary` JSON 字段消费。前端类型为 `GraphRunExecutionSummary` + `GraphRunNodeSummary`。

```go
// internal/graph/trpc/execution_summary.go
type NodeExecutionSummary struct {
    NodeID     string `json:"node_id"`
    NodeType   string `json:"node_type"`
    Status     string `json:"status"`
    DurationMS int64  `json:"duration_ms"`
    Error      string `json:"error,omitempty"`
    StepNumber int    `json:"step_number,omitempty"`
}

type ExecutionSummary struct {
    ExecutionID    string                 `json:"execution_id"`
    GraphID        string                 `json:"graph_id"`
    TotalSteps     int                    `json:"total_steps"`
    DurationMS     int64                  `json:"duration_ms"`
    Nodes          []NodeExecutionSummary `json:"nodes"`
    FinalStateKeys int                    `json:"final_state_keys,omitempty"`
}
```

---

## 六、维度四：设计辅助与资产复用

### 6.1 设计时校验（✅ 已实现）

校验引擎 `ValidateGraph` 在 `UpdateGraph` 和 `ExecuteGraph` 时执行，返回 `ValidationResult{errors, warnings}`。

**校验项**：

| 校验项 | 错误码 | 级别 | 说明 |
|--------|--------|------|------|
| 空图 | `empty_graph` | Error | Graph 必须包含至少一个节点 |
| 无入口点 | `no_entry_point` | Error | 必须指定 entry_point |
| 不可达节点 | `unreachable_node` | Warning | 从入口点不可达的节点 |
| Agent 不存在 | `agent_not_found` | Error | Agent 节点引用的 Agent 不存在 |
| FuncRef 未注册 | `func_not_registered` | Error | Function 节点的 FuncRef 未在 Registry 注册 |
| CondFuncRef 未注册 | `cond_func_not_registered` | Error | Router 节点的 CondFuncRef 未注册 |
| State Schema 缺失 | `missing_state_field` | Warning | 节点引用的 State 字段未在 Schema 中定义 |
| Reducer 类型不匹配 | `reducer_type_mismatch` | Error | AppendReducer 字段类型不是切片 |
| 循环无退出 | `loop_no_exit` | Warning | 循环路径没有条件退出 |

### 6.2 设计模式模板（✅ 已实现）

6 种内置模板，通过 `ListGraphTemplates` / `CreateGraphFromTemplate` API 暴露：

| 模板 ID | 名称 | 拓扑 | 节点数 |
|---------|------|------|--------|
| `pipeline` | 顺序流水线 | A→B→C→D | 4 |
| `approval` | 审批流 | A→[审批]→B/C | 3 |
| `parallel_review` | 并行评审 | A→(B∥C∥D)→汇总 | 5 |
| `review_loop` | 生成-评审循环 | A→B→[评分]→A/D | 4 |
| `dispatch` | 条件分发 | A→[路由]→B/C/D | 4 |
| `nested_subgraph` | 子图嵌套 | A→[子工作流]→B | 3 |

### 6.3 资产复用（✅ 已实现）

| 资产类型 | 实现方式 | 状态 |
|----------|----------|------|
| 用户自定义模板 | `SaveGraphAsTemplate` RPC + `metadata.user_template` + `GraphTemplatePicker` | ✅ |
| Graph 版本管理 | `metadata._version_history` 快照 + `ListGraphVersions/RollbackGraphVersion` RPC + `GraphVersionPanel` | ✅ |
| 导入/导出 | `ExportGraph/ImportGraph` RPC + 编辑器 ⋮ 菜单 + `useGraphEditorAssets.ts` | ✅ |
| 子图复用 | 后端 `SubgraphDef` 已支持，前端子图节点编辑器待补 | 🟡 |

> **版本管理实现说明**：版本历史存储在 `GraphDefinition.metadata._version_history` JSON 字段中（最多 50 条），而非独立的数据库列。每次 `UpdateGraph` 时自动创建快照。Ent schema 中无 `version` 列——版本号和快照均通过 metadata 持久化。

---

## 七、任务派工与执行规则

### 7.1 任务模型（✅ 已实现）

```go
type GraphTask struct {
    TaskID             string
    NodeID             string
    ExecutionID        string
    Assignee           string
    Status             TaskStatus
    Context            string
    Input              string
    Output             string
    Summary            string
    Metadata           string
    RequiredRole       string
    AssignmentMode     string
    AssignmentStrategy string
    CreatedAt          time.Time
    ClaimedAt          *time.Time
    CompletedAt        *time.Time
}
```

### 7.2 任务状态机（✅ 已实现）

```
pending ──ClaimTask──▶ claimed ──SubmitResult──▶ complete
                               ──ReportBlocked──▶ blocked
                               ──SubmitForReview──▶ review_required ──Approve──▶ complete
                                                                          ──Reject──▶ claimed
                               ──Fail──▶ failed
                               ──Timeout──▶ timed_out
pending ──(无匹配Agent)──▶ pending_assignment
```

| 状态 | 说明 |
|------|------|
| `pending` | 等待领取 |
| `claimed` | 已领取，执行中 |
| `complete` | 已完成 |
| `blocked` | 执行受阻 |
| `review_required` | 待审核 |
| `failed` | 失败 |
| `timed_out` | 超时 |
| `cancelled` | 已取消 |
| `crashed` | 崩溃 |
| `pending_assignment` | 待指派（无匹配 Agent） |

### 7.3 Agent 角色与动态派工（✅ 已实现）

- **static 模式**：直接使用 `agent_name` 指定
- **dynamic 模式**：按 `required_role` 从 Agent 目录匹配
  - 唯一匹配 → 指派
  - 多人匹配 → 按 `assignment_strategy`（least_tasks/random/manual）选择
  - 无匹配 → `pending_assignment`

安全原则：无匹配 Agent 时不随机指派，任务进入 `pending_assignment` 等待。

### 7.4 审核与质量门禁（✅ 已实现）

- **人工审核**：复用 HITL InterruptBefore/After
- **自动审核**：`reviewer_agent` 字段指定 Reviewer Agent，审核通过 → complete，驳回 → claimed
- **评论反馈**：`TaskComment` 模型，支持 approve/reject/suggestion 类型

### 7.5 智能超时（✅ 已实现）

- Agent 通过 `Heartbeat` API 上报心跳
- 心跳感知超时：持续心跳时延长租约（`enable_lease_extension`）
- 超时后任务标记为 `timed_out`

### 7.6 可观测性（✅ 已实现）

| 模型 | API | 说明 |
|------|-----|------|
| TaskLog | `ListTaskLogs` | 任务级 stdout/stderr 日志 |
| TaskRun | `ListTaskRuns` | 运行历史（启动/结束时间、退出码） |
| TaskEvent | `ListTaskEvents` | 事件追踪（状态变更流） |
| TaskComment | `ListTaskComments` | 审核评论 |

### 7.7 对外集成 API（✅ 部分实现）

| API | 状态 | 说明 |
|-----|------|------|
| `ClaimTask` | ✅ | Agent 领取任务 |
| `Heartbeat` | ✅ | Agent 心跳上报 |
| `SubmitTaskResult` | ✅ | Agent 提交结果 |
| `ReportBlocked` | ✅ | Agent 报告阻塞 |
| `ReviewTask` | ✅ | 审核任务 |
| Webhook 通知 | ✅ | `GraphTaskRuntime.dispatchGraphTaskWebhook` → `graph.task.status` |
| 熔断策略 | 🟡 | Proto `CircuitBreakerPolicy` 已定义，未接入 NodeDef；biz 层 `tool/circuit_breaker.go` 已实现 Tool 级熔断 |

---

## 八、数据模型

### 8.1 Ent Schema 清单

| Schema | 表名 | 说明 |
|--------|------|------|
| `GraphDefinition` | `graph_definitions` | 工作流定义（节点/边/状态/配置 JSON） |
| `GraphExecution` | `graph_executions` | 运行实例（关联定义、当前状态、检查点引用） |
| `GraphTask` | `graph_tasks` | 任务（指派、状态、输入输出） |
| `GraphTaskComment` | `graph_task_comments` | 审核评论 |
| `GraphTaskLog` | `graph_task_logs` | 任务日志 |
| `GraphTaskRun` | `graph_task_runs` | 运行历史 |
| `GraphTaskEvent` | `graph_task_events` | 事件追踪 |

### 8.2 GraphDefinition 核心字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | UUID | 主键 |
| `name` | String | 名称 |
| `description` | String | 描述 |
| `state_fields` | JSON | State Schema 定义 |
| `nodes` | JSON | 节点定义列表 |
| `edges` | JSON | 边定义列表 |
| `conditional_edges` | JSON | 条件边定义列表 |
| `subgraphs` | JSON | 子图定义列表 |
| `entry_point` | String | 入口节点 ID |
| `finish_point` | String | 终止节点 ID |
| `enable_checkpoint` | Bool | 是否启用检查点 |
| `execution_engine` | String | 执行引擎（bsp/dag） |
| `interrupt_before` | JSON | 全局中断前节点列表 |
| `interrupt_after` | JSON | 全局中断后节点列表 |
| `metadata` | JSON | 扩展元数据（含 `_version`/`_version_history`/`user_template`/`layout`） |
| `sort_order` | Int | 排序序号 |
| `created_at` | Time | 创建时间 |
| `updated_at` | Time | 更新时间 |

> **版本管理存储**：版本号和快照历史存储在 `metadata._version`（int）和 `metadata._version_history`（GraphVersionEntry[] JSON）中，Ent schema 中无独立 `version` 列。

### 8.3 GraphExecution 核心字段

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | UUID | 主键 |
| `graph_id` | UUID | 关联 GraphDefinition |
| `session_id` | String | 会话 ID |
| `status` | String | 执行状态 |
| `current_node` | String | 当前执行节点 |
| `lineage_id` | String | 检查点血统 ID |
| `error_message` | Text | 错误信息 |

---

## 九、模块边界

### 9.1 Graph vs Agent

| 维度 | Graph | Agent |
|------|-------|-------|
| 关注点 | 流程编排（节点/边/状态/检查点） | Agent 生命周期（配置/运行/工具） |
| 交互方式 | Graph 节点引用 Agent 目录中的 Agent | Agent 独立运行，不感知 Graph |
| 状态管理 | Graph State（显式 Schema + Reducer） | Agent 会话历史（隐式消息传递） |

### 9.2 Graph vs Team

| 维度 | Graph | Team |
|------|-------|------|
| 编排方式 | 自由编排（画布定义任意拓扑） | 模式化协作（预定义模式） |
| 控制粒度 | 节点级（精确控制每一步） | 模式级（框架决定执行顺序） |
| 适用场景 | 复杂业务流程 | 简单协作 |

### 9.3 Graph 内部模块边界

| 模块 | 职责 | 不负责 |
|------|------|--------|
| `builder.go` | GraphBuildConfig → StateGraph → GraphAgent | 业务逻辑、持久化 |
| `node_wiring.go` | 节点类型接线（LLM/Tool/Agent/Function/Router/Task/Review）+ Mapper + RetryPolicy + CachePolicy + WithEndsMap | 图构建、持久化 |
| `registry.go` | NodeFunc/CondFunc 注册与解析 | 执行逻辑 |
| `validator.go` | 设计时校验 | 运行时校验 |
| `templates.go` | 内置模板定义与转换 | 用户模板管理 |
| `checkpoint.go` | Checkpoint Saver 适配 | 检查点业务逻辑 |
| `event_bridge.go` | Graph 事件 → EventBus 桥接 + ExecutionSummary 集成 | 事件消费 |
| `execution_summary.go` | ExecutionSummary 追踪与快照 | 事件推送 |
| `visualize.go` | DOT 解析 + 结构化 JSON | 画布渲染 |
| `graph_version.go` | 版本快照、回滚、用户模板元数据 | 持久化 |
| `GraphUsecase` | 图定义 CRUD + 执行编排 | 图构建细节 |
| `GraphDefinitionUsecase` | 图定义 CRUD + 模板 + 版本 + 导入导出 | 执行逻辑 |
| `TaskUsecase` | 任务生命周期管理 | 图执行逻辑 |
