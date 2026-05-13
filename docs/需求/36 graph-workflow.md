# M4: Graph 工作流 — 详细需求

> 对标 `pkg/trpc-agent-go/graph` 包，实现完整的 Graph 工作流引擎。

---

## 1. 现状分析

项目已有 `internal/graph/trpc/builder.go`，实现了基础的 Graph 构建：
- `GraphBuildConfig`：节点/边/条件边/入口/出口定义
- `BuildStateGraph`：构建 `trpcgraph.StateGraph` 并编译
- `GraphAgent`：将编译后的 Graph 包装为 `trpcagent.Agent`

**缺失能力**：
1. 无 State Schema + Reducer（状态管理）
2. 无条件路由的完整支持（仅有 `AddConditionalEdges` 入口）
3. 无 HITL（Human-in-the-Loop）中断/恢复
4. 无检查点（Checkpoint）持久化
5. 无时间旅行（Time Travel）调试
6. 无子图（Subgraph）嵌套
7. 无 DAG 执行引擎
8. 无 API 端点暴露
9. 无可视化编辑器

---

## 2. trpc 框架参照

```
pkg/trpc-agent-go/graph/
├── graph.go              # Graph 核心结构：AddNode/AddEdge/AddConditionalEdges/Compile
├── state_graph.go        # StateGraph 构建器：NewStateGraph/NewStateSchema/AddField
├── state.go              # State 管理：StateSchema/StateField/Reducer
├── executor.go           # Executor：执行编译后的 Graph
├── executor_dag.go       # DAG 执行引擎：并行节点调度
├── checkpoint.go         # Checkpoint：持久化执行状态
├── interrupt.go          # HITL 中断：Interrupt/Resume
├── resume.go             # 恢复执行：从检查点恢复
├── time_travel.go        # 时间旅行：回放历史状态
├── cache.go              # 缓存：节点结果缓存
├── retry.go              # 重试：节点执行重试
├── stream.go             # 流式：事件流输出
├── events.go             # 事件：Graph 执行事件
├── callbacks.go          # 回调：节点执行前后回调
├── visualize.go          # 可视化：生成 DOT/Mermaid 格式
├── errors.go             # 错误：Graph 执行错误
├── message_ops.go        # 消息操作：状态消息处理
├── completion_control.go # 完成控制：Graph 终止条件
├── external_interrupt.go # 外部中断：运行时中断 Graph
├── static_interrupt.go   # 静态中断：编译时中断点
└── trace_task.go         # 追踪：执行追踪
```

---

## 3. 需求清单

### 3.1 State Schema + Reducer

**需求**：支持定义 Graph 状态的结构和更新策略

**trpc 接口**：
```go
schema := trpcgraph.NewStateSchema()
schema.AddField("counter", trpcgraph.StateField{
    Type:         "integer",
    DefaultValue: 0,
    Reducer:      trpcgraph.ReducerAppend,
})
```

**实现要点**：
- 扩展 `GraphBuildConfig` 支持 `StateFields` 定义
- 在 `BuildStateGraph` 中创建 `StateSchema` 并添加字段
- 支持 Reducer 类型：`append`/`replace`/`merge`/`custom`

**验收标准**：Graph 节点可通过 State 读写共享状态，Reducer 正确合并更新

### 3.2 条件路由

**需求**：支持基于状态的条件分支

**trpc 接口**：
```go
sg.AddConditionalEdges("node_a", condFunc, map[string]string{
    "yes": "node_b",
    "no":  "node_c",
})
```

**实现要点**：
- `ConditionalEdgeDef.CondFunc` 需要支持 `func(state map[string]any) string` 签名
- 路径映射 `PathMap` 定义分支到节点的映射

**验收标准**：条件路由根据状态值正确选择下一个节点

### 3.3 HITL 中断/恢复

**需求**：Graph 执行到中断点暂停，等待人工确认后恢复

**trpc 接口**：
```go
// 在节点函数中触发中断
func myNode(ctx context.Context, state map[string]any) (map[string]any, error) {
    return map[string]any{"__interrupt__": "需要确认"}, nil
}

// 恢复执行
executor.Resume(ctx, checkpoint, resumeValue)
```

**实现要点**：
- 节点函数返回 `__interrupt__` 键触发中断
- 中断时保存 Checkpoint
- 提供 Resume API 接受用户输入并恢复执行

**验收标准**：Graph 执行到中断点暂停，用户确认后恢复继续执行

### 3.4 检查点持久化

**需求**：Graph 执行状态可持久化，支持断点续执行

**trpc 接口**：
```go
// Redis 检查点后端
import "trpc.group/trpc-go/trpc-agent-go/graph/checkpoint/redis"
```

**实现要点**：
- 集成 trpc `checkpoint` 包
- 先实现 SQLite 后端，后续增加 Redis
- 检查点包含：当前节点、状态快照、事件历史

**验收标准**：Graph 中断后可从检查点恢复执行

### 3.5 时间旅行调试

**需求**：可回放 Graph 执行的任意历史状态

**trpc 接口**：
```go
executor.TimeTravel(ctx, checkpointID, stepIndex)
```

**实现要点**：
- 集成 trpc `time_travel` 包
- 记录每个节点的输入/输出状态快照
- 提供 API 查询历史状态

**验收标准**：可通过 API 查看 Graph 执行的任意步骤状态

### 3.6 子图嵌套

**需求**：Graph 节点可以是另一个 Graph（子图）

**trpc 接口**：
```go
sg.AddNode("sub_workflow", subGraph.NodeFunc())
```

**实现要点**：
- 子图编译后作为节点函数注册
- 子图的状态映射到父图状态

**验收标准**：子图作为节点正常执行，状态正确传递

### 3.7 DAG 执行引擎

**需求**：支持并行节点执行

**trpc 接口**：
```go
trpcgraph.NewExecutor(graph, trpcgraph.WithExecutionEngine(trpcgraph.EngineDAG))
```

**实现要点**：
- 集成 trpc `executor_dag` 包
- 分析节点依赖关系，并行执行无依赖节点
- 支持 BSP（默认）和 DAG 两种执行模式

**验收标准**：无依赖的节点并行执行，有依赖的节点按序执行

### 3.8 API 端点

**需求**：通过 REST/gRPC 暴露 Graph 工作流定义和执行

**实现要点**：
- 新建 `api/kratos/graph/v1/graph.proto`
- `CreateGraph`：创建 Graph 定义
- `ExecuteGraph`：执行 Graph
- `GetGraphStatus`：查询执行状态
- `ResumeGraph`：恢复中断的 Graph
- `TimeTravelGraph`：时间旅行查询

**验收标准**：通过 API 可创建、执行、查询、恢复 Graph 工作流

### 3.9 可视化（超越层）

**需求**：前端拖拽编辑 Graph 工作流

**实现要点**：
- 集成 `trpcgraph.Visualize` 生成 DOT/Mermaid 格式
- 前端使用 Vue Flow 或类似库渲染
- 拖拽编辑后序列化为 `GraphBuildConfig`

**验收标准**：前端可拖拽构建 Graph 工作流并执行

---

## 4. 涉及文件

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/graph/trpc/builder.go` | 修改 | 扩展 StateSchema/Reducer/条件路由 |
| `internal/graph/trpc/state.go` | 新建 | State 定义和 Reducer |
| `internal/graph/trpc/hitl.go` | 新建 | HITL 中断/恢复 |
| `internal/graph/trpc/checkpoint.go` | 新建 | 检查点持久化 |
| `internal/graph/trpc/subgraph.go` | 新建 | 子图支持 |
| `internal/graph/trpc/dag.go` | 新建 | DAG 执行引擎配置 |
| `internal/service/graph.go` | 新建 | Graph 服务层 |
| `internal/server/register_graph.go` | 新建 | Graph HTTP/gRPC 端点 |
| `api/kratos/graph/v1/graph.proto` | 新建 | Graph Proto 定义 |
| `web/src/features/graph/` | 新建 | 前端 Graph 编辑器 |

---

## 5. 验收标准总览

1. 能通过 API 定义并执行包含条件路由的 Graph 工作流
2. Graph 执行到中断点暂停，用户确认后恢复
3. 执行状态持久化到 SQLite，中断后可恢复
4. 可回放 Graph 执行的任意历史状态
5. 子图作为节点正常执行
6. 无依赖节点并行执行
7. 前端可拖拽构建 Graph 工作流（超越层）
