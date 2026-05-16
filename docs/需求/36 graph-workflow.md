# M4: Graph 工作流 — 详细需求（v2）

> 对标 `pkg/trpc-agent-go/graph` 包，构建"LangGraph for Go"级别的确定性工作流引擎。
> 本文档从四个核心维度重新梳理 Graph 的产品定位、需求层级和演进路线。

---

## 0. Graph 的存在意义：为什么需要独立的工作流图？

### 0.1 Team vs Graph 的本质差异

系统已有 Team 编排（sequential/parallel/coordinator/critic_loop/swarm），为什么还需要独立的 Graph 工作流？

| 维度 | Team | Graph |
|------|------|-------|
| **编排哲学** | 模式化协作——选择预定义模式，配置成员 | 自由编排——画布定义任意拓扑，精确控制每一步 |
| **控制权** | 框架决定执行顺序，Agent 自主推理 | 用户决定流程骨架，Agent 在节点内自主推理 |
| **条件分支** | 无（模式固定） | 支持（基于状态的条件路由、Command.GoTo） |
| **人工介入** | 无 | HITL 中断/恢复（审批节点） |
| **状态管理** | 隐式（消息传递） | 显式 State Schema + Reducer |
| **可回溯性** | TeamRun 步骤列表 | Checkpoint + TimeTravel 任意状态回放 |
| **并行控制** | 模式级（parallel 整体并行） | 节点级（DAG 引擎自动并行无依赖节点） |
| **适用场景** | 简单协作（多人讨论、流水线） | 复杂业务流程（审批流、数据处理管线、多阶段决策） |

**核心洞察**：Team 解决的是"多 Agent 如何协作"的问题，Graph 解决的是"复杂流程如何确定性地执行"的问题。二者互补而非替代——Graph 中的节点本身可以是 Team。

### 0.2 Graph 的三大核心价值

1. **确定性流程控制**：解决纯 Agent 系统"状态漂移"的难题。Agent 节点内部拥有基于 LLM 的自主推理能力，但节点间的流转由确定性的图规则牢牢控制
2. **运行态透明性**：企业级应用不仅需要设计态清晰，更要求运行态透明——每个节点执行到哪、输入输出是什么、卡在哪里，一目了然
3. **流程可干预可回溯**：内置人工审批节点、检查点与恢复、全链路运行轨迹记录，保障流程可控可回溯

---

## 1. 四维需求架构

Graph 工作流的需求从以下四个核心维度展开，每个维度下有业界验证的设计方案：

```
┌─────────────────────────────────────────────────────────────────┐
│                    维度四：设计辅助与资产复用                      │
│         设计模式建议 / Agent 仓库 / 任务模板 / 设计时校验          │
├─────────────────────────────────────────────────────────────────┤
│                    维度三：人机协同与可观测性                      │
│     人工审批节点 / 状态检查点与恢复 / 全链路运行轨迹               │
├─────────────────────────────────────────────────────────────────┤
│                    维度二：动态拓扑与状态共享                      │
│     条件路由 / 动态节点生成 / 全局共享工作流状态                   │
├─────────────────────────────────────────────────────────────────┤
│                    维度一：图结构与混合控制                        │
│     有向图骨架 / Agent 节点自主推理 / 确定性流转规则               │
└─────────────────────────────────────────────────────────────────┘
```

---

## 2. 维度一：图结构与混合控制

> 系统底层采用有向图（DAG）定义流程骨架。Agent 节点内部拥有基于 LLM 的自主推理能力，但节点间的流转由确定性的图规则牢牢控制。

### 2.1 节点类型体系

Graph 不仅包含 Agent，还包含多种节点类型（对齐 trpc-agent-go `NodeType`）：

| 节点类型 | NodeType | 用途 | 视觉样式 | 框架支持 |
|----------|----------|------|----------|----------|
| Function | `function` | 纯逻辑处理（数据转换、条件判断、格式化） | 紫色矩形 | `AddNode` + `NodeFunc` |
| LLM | `llm` | 轻量级 LLM 调用（不需要完整 Agent 上下文） | 蓝色矩形 | `AddLLMNode` + instruction + model + tools |
| Tool | `tool` | 直接调用工具（不需要 Agent 中介） | 橙色矩形 | `AddToolNode` |
| Agent | `agent` | 引用系统 Agent 目录中的 Agent | 绿色矩形 | `AddAgentNode` + SubgraphInputMapper/OutputMapper |
| Router | `router` | 条件路由（根据状态选择分支） | 灰色菱形 | `AddConditionalEdges` |
| Join | `join` | 汇聚并行分支 | 紫色菱形 | BSP/DAG 引擎自动处理 |

### 2.2 Agent 节点的混合控制

Agent 节点是 Graph 中最核心的节点类型，它体现了"混合控制"的精髓：

- **节点内部**：Agent 拥有完整的 LLM 推理能力，可自主决策、调用工具
- **节点外部**：Agent 的输入来自 Graph State（通过 InputMapper 映射），输出写回 Graph State（通过 OutputMapper 映射）
- **流转控制**：Agent 执行完毕后，下一个节点由 Graph 的边规则决定，而非 Agent 自身

**Agent 节点配置**：
- **身份**：引用系统 Agent 目录中的某个 Agent（通过 `agent_key`）
- **输入映射**：`SubgraphInputMapper` — 从 Graph State 中投影哪些字段作为 Agent 的运行时状态
- **输出映射**：`SubgraphOutputMapper` — Agent 的输出如何写回 Graph State
- **隔离模式**：`WithSubgraphIsolatedMessages` — 是否隔离 Agent 的会话历史
- **结果传递**：`WithSubgraphInputFromLastResponse` — 上游节点的 last_response 直接作为下游 Agent 的 user_input
- **中断控制**：`InterruptBefore` / `InterruptAfter` — 执行前/后是否需要人工确认

### 2.3 边类型与流转规则

| 边类型 | 说明 | 框架 API | 视觉表示 |
|--------|------|----------|----------|
| Runtime Edge | 确定性流转，A 完成后必定到 B | `AddEdge("A", "B")` | 实线箭头 |
| Conditional Edge | 条件路由，根据状态选择分支 | `AddConditionalEdges("A", condFunc, pathMap)` | 虚线箭头 + 条件标签 |
| Command Edge | 节点内部通过 Command.GoTo 动态路由 | `WithEndsMap` + `Command.GoTo("approved")` | 动态，运行时决定 |

### 2.4 执行引擎

| 引擎 | 说明 | 适用场景 |
|------|------|----------|
| BSP（默认） | Pregel 式整体同步并行，每步执行所有活跃节点后同步 | 需要确定性、可复现的场景 |
| DAG | 分析节点依赖关系，无依赖节点并行执行 | 高吞吐、节点间无复杂状态交互的场景 |

### 2.5 子图嵌套

Graph 节点可以是另一个 Graph（子图），支持层级化工作流设计：

- 子图编译后作为 Agent 节点注册
- 子图的状态通过 InputMapper/OutputMapper 与父图映射
- 子图支持独立的 Checkpoint 和 TimeTravel
- 子图事件通过 `WithSubgraphEventScope` 限定作用域

---

## 3. 维度二：动态拓扑与状态共享

> 现代 Agent 工作流通常不再是静态固定的。应支持条件路由和动态节点生成，同时所有 Agent 应能访问一个全局共享的工作流状态。

### 3.1 条件路由

条件路由是 Graph 区别于 Team 的核心能力之一：

**框架接口**：
```go
sg.AddConditionalEdges("review_node", condFunc, map[string]string{
    "approved": "publish_node",
    "rejected": "revise_node",
})
```

**条件函数签名**：
- `ConditionalFunc`：`func(ctx, state) (string, error)` — 返回单个下一节点
- `MultiConditionalFunc`：`func(ctx, state) ([]string, error)` — 返回多个下一节点（并行）
- `UniversalCondFunc`：`func(ctx, state) (ConditionResult, error)` — 统一接口

**需求**：
- 用户在编辑器中为 Router 节点定义条件分支和路径映射
- 条件函数通过 Registry 注册，支持 `CondFuncRef` 引用
- 前端条件分支以虚线箭头 + 标签展示

### 3.2 动态节点生成（Command.GoTo）

节点执行时可通过 `Command` 动态决定下一步：

```go
func myNode(ctx context.Context, state graph.State) (any, error) {
    if condition {
        return graph.Command{GoTo: "node_b", Update: graph.State{...}}, nil
    }
    return graph.State{...}, nil
}
```

**需求**：
- 节点属性面板支持配置 `WithEndsMap`（声明可能的动态路由目标）
- 运行时 Command.GoTo 事件通过 WS 推送到前端
- 前端执行监控中动态高亮实际执行的路径

### 3.3 全局共享工作流状态（State Schema + Reducer）

所有节点共享同一个 Graph State，实现无感的上下文传递：

**State Schema 定义**：
```go
schema := graph.NewStateSchema().
    AddField("input", graph.StateField{Type: reflect.TypeOf(""), Reducer: graph.DefaultReducer}).
    AddField("messages", graph.StateField{Type: reflect.TypeOf([]any{}), Reducer: graph.AppendReducer}).
    AddField("counter", graph.StateField{Type: reflect.TypeOf(0), Reducer: graph.DefaultReducer})
```

**Reducer 类型**：

| Reducer | 说明 | 适用场景 |
|---------|------|----------|
| `DefaultReducer` | 完全替换旧值 | 标量更新（counter、status） |
| `AppendReducer` | 追加到列表 | 消息累积、日志追加 |
| `CoverReducer` | 覆盖（仅非零值） | 可选更新 |
| `MergeReducer` | 深度合并 Map | 配置合并、元数据更新 |

**需求**：
- 编辑器提供 State Schema 编辑面板
- 支持定义字段名、类型、Reducer、默认值
- 节点属性面板显示该节点读写的 State 字段
- 运行时可查看每个节点的 State 读写快照

---

## 4. 维度三：人机协同与可观测性

> 企业级应用不仅需要工作流图在设计态清晰，更要求在运行态透明。内置人工审批节点、支持状态检查点与恢复以及全链路运行轨迹记录，是保障流程可控可回溯的关键。

### 4.1 人工审批节点（HITL）

**框架机制**：
```go
// 方式一：编译时静态中断
sg.AddNode("approval", approvalFunc, graph.WithInterruptBefore())

// 方式二：运行时动态中断
func approvalFunc(ctx context.Context, state graph.State) (any, error) {
    return graph.Interrupt(ctx, state, "approval_key", "请确认是否继续")
}
```

**恢复执行**：
```go
resumeCmd := graph.NewResumeCommand().WithResume(userInput)
executor.Resume(ctx, checkpoint, resumeCmd)
```

**需求**：

| 需求 | 优先级 | 说明 |
|------|--------|------|
| 静态中断点 | P0 | 节点属性面板支持勾选 InterruptBefore/After |
| 动态中断 | P1 | Function 节点内调用 `graph.Interrupt()` |
| 前端中断弹框 | P0 | WS `checkpoint_interrupt` 事件触发确认对话框 |
| 恢复执行 | P0 | Resume API 接受用户输入并恢复执行 |
| 多中断点 | P1 | ResumeMap 支持多个并行中断点的分别恢复 |

### 4.2 状态检查点与恢复（Checkpoint）

**框架能力**：
- 每个步骤执行后自动保存 Checkpoint
- Checkpoint 包含：当前节点、状态快照、Channel 版本、中断状态
- 支持 SQLite / InMemory / Redis 多种持久化后端
- 支持从任意 Checkpoint 恢复执行

**需求**：

| 需求 | 优先级 | 说明 |
|------|--------|------|
| Checkpoint 自动保存 | P0 | 启用 Checkpoint 的 Graph 每步自动保存 |
| Checkpoint 列表查询 | P0 | ListCheckpoints API |
| 状态快照查看 | P0 | GetStateSnapshot API |
| 状态编辑 | P1 | EditState API — 修改状态后从新 Checkpoint 恢复 |
| Checkpoint 持久化 | P1 | SQLite Checkpoint Saver（已实现 InMemory） |
| Redis Checkpoint | P2 | 分布式场景下的 Checkpoint 共享 |

### 4.3 时间旅行调试（TimeTravel）

**框架能力**：
- 记录每个节点的输入/输出状态快照
- 支持回放到任意历史步骤
- 支持编辑历史状态并从编辑点重新执行

**需求**：

| 需求 | 优先级 | 说明 |
|------|--------|------|
| 历史状态查看 | P1 | TimeTravel API 查询任意步骤状态 |
| 时间线组件 | P1 | 前端检查点时间线，点击跳转 |
| 状态编辑重放 | P2 | EditState + Resume 从历史点重新执行 |
| 分支切换 | P2 | 在条件路由处选择不同分支重新执行 |

### 4.4 全链路运行轨迹

**可观测维度**：

| 观测维度 | 具体需求 | 优先级 | 实现方式 |
|----------|----------|--------|----------|
| **执行进度** | 当前执行到哪个节点、已执行/总节点数 | P0 | WS `graph` channel 实时推送 |
| **节点状态** | 每个节点的运行/等待/完成/失败/中断状态 | P0 | WS `graph_node_start/end` 事件 |
| **数据流** | 每个节点的输入/输出 State 快照 | P0 | GetGraphExecution API |
| **时间信息** | 每个节点的开始/结束时间、耗时 | P1 | NodeExecutionMetadata |
| **资源消耗** | 每个节点的 Token 用量、成本 | P1 | ModelExecutionMetadata |
| **工具调用** | Tool 节点的工具调用详情 | P1 | ToolExecutionMetadata |
| **模型调用** | LLM 节点的模型调用详情 | P1 | ModelExecutionMetadata |
| **错误信息** | 失败节点的错误详情、重试状态 | P0 | WS `graph_node_error` 事件 |
| **Pregel 步骤** | BSP 引擎的步骤级进度 | P2 | `graph_pregel_step` 事件 |

**可视化呈现**：
- **节点颜色状态**：运行中（脉冲动画）、完成（绿色勾）、失败（红色叉）、中断（黄色暂停）、等待（灰色）
- **连线动画**：正在执行的数据流连线显示流动动画
- **进度条**：Graph 整体进度（已执行节点数/总节点数）
- **时间线**：底部时间线展示节点执行顺序和耗时

### 4.5 运行时操作

| 操作类型 | 具体需求 | 优先级 | 触发方式 |
|----------|----------|--------|----------|
| **HITL 确认** | 中断节点弹出确认框，用户输入后恢复执行 | P0 | WS `checkpoint_interrupt` 事件 → 前端弹框 |
| **取消执行** | 取消正在运行的 Graph | P0 | CancelGraphExecution API |
| **重试失败节点** | 从失败节点重新执行 | P1 | ResumeGraph API（从失败检查点） |
| **修改状态** | 编辑当前 State 并继续执行 | P1 | TimeTravel EditState API |
| **时间旅行** | 回到历史检查点，查看/编辑状态 | P2 | TimeTravel API |
| **分支切换** | 在条件路由处选择不同分支重新执行 | P2 | EditState + Resume |

---

## 5. 维度四：设计辅助与资产复用

> 引入设计模式建议和资产复用是显著降低用户设计门槛、提升一致性的有效策略。同时，应提供完善的设计时校验功能。

### 5.1 设计时校验

**编译时校验**（框架已支持）：
- 入口点必须存在
- 所有节点可达（无孤立节点）
- 条件路由的目标节点必须存在
- `WithEndsMap` 声明的目标节点必须存在

**产品层校验**（需实现）：
- State Schema 完整性检查：所有节点读写的字段是否已定义
- Agent 引用校验：Agent 节点引用的 Agent 是否存在
- 中断点合理性检查：HITL 节点是否在关键决策点
- 循环检测：是否存在无退出条件的循环
- 并发安全检查：并行节点是否写入同一 State 字段（Reducer 冲突）

**需求**：

| 校验项 | 优先级 | 说明 |
|--------|--------|------|
| 基础拓扑校验 | P0 | 入口点、孤立节点、不可达节点 |
| Agent 引用校验 | P0 | Agent 节点引用的 Agent 存在 |
| State Schema 校验 | P1 | 节点读写字段已定义、Reducer 类型匹配 |
| 循环退出校验 | P1 | 循环路径有退出条件 |
| 并发安全校验 | P2 | 并行节点无 State 写冲突 |

### 5.2 设计模式建议

预置常见工作流模式，用户可基于模板快速创建：

| 模式名称 | 拓扑 | 适用场景 |
|----------|------|----------|
| **顺序流水线** | A → B → C → D | 数据处理管线、报告生成 |
| **审批流** | A → [审批] → B / C | 需要人工确认的业务流程 |
| **并行评审** | A → (B ∥ C ∥ D) → 汇总 | 多角度评审、多源数据采集 |
| **生成-评审循环** | A → B → [评分] → A / D | 迭代优化、质量提升 |
| **条件分发** | A → [路由] → B / C / D | 根据类型/优先级分发任务 |
| **子图嵌套** | A → [子工作流] → B | 复用通用流程片段 |

**需求**：

| 需求 | 优先级 | 说明 |
|------|--------|------|
| 模板库 | P1 | 内置 6 种常见模式模板 |
| 从模板创建 | P1 | 用户选择模板后自动生成 Graph 定义 |
| 自定义模板 | P2 | 用户可将已有 Graph 保存为模板 |
| 智能推荐 | P3 | 根据用户描述推荐合适的工作流模式 |

### 5.3 资产复用

| 资产类型 | 说明 | 优先级 |
|----------|------|--------|
| **Graph 模板** | 从已有 Graph 创建模板，快速复用 | P1 |
| **Agent 仓库** | 系统已有 Agent 目录，Graph 节点直接引用 | ✅ 已有 |
| **子图复用** | 将常用流程片段封装为子图，跨 Graph 复用 | P1 |
| **Graph 版本管理** | Graph 定义版本化，支持回滚 | P2 |
| **导入/导出** | Graph 定义 JSON 导入导出 | P2 |
| **分享协作** | Graph 定义可分享给其他用户 | P3 |
| **市场/商店** | 社区共享 Graph 模板 | P3 |

### 5.4 节点结果缓存与重试

**框架已支持**：
- `WithNodeCachePolicy` — 节点结果缓存，避免重复计算
- `WithCacheKeyFields` — 缓存键字段选择
- `WithRetryPolicy` — 节点执行重试（指数退避 + Jitter）
- `WithDefaultRetryPolicy` — Executor 级默认重试策略

**需求**：

| 需求 | 优先级 | 说明 |
|------|--------|------|
| 节点重试配置 | P1 | 节点属性面板支持配置重试策略 |
| 节点缓存配置 | P2 | 节点属性面板支持配置缓存策略 |
| Executor 默认重试 | P2 | Graph 级默认重试策略 |

---

## 6. 节点属性配置需求

每种节点类型有不同的属性配置需求：

### 6.1 Function 节点

| 属性 | 说明 | 必填 |
|------|------|------|
| 节点 ID | 唯一标识 | 是 |
| 描述 | 节点功能说明 | 否 |
| FuncRef | 注册的函数引用 | 是 |
| InterruptBefore | 执行前中断 | 否 |
| InterruptAfter | 执行后中断 | 否 |

### 6.2 LLM 节点

| 属性 | 说明 | 必填 |
|------|------|------|
| 节点 ID | 唯一标识 | 是 |
| 描述 | 节点功能说明 | 否 |
| Instruction | LLM 指令 | 是 |
| Model | 模型选择 | 是 |
| Tools | 绑定工具列表 | 否 |
| UserInputKey | State 中作为 user_input 的字段 | 否 |
| GenerationConfig | 生成参数配置 | 否 |
| InterruptBefore | 执行前中断 | 否 |
| InterruptAfter | 执行后中断 | 否 |

### 6.3 Tool 节点

| 属性 | 说明 | 必填 |
|------|------|------|
| 节点 ID | 唯一标识 | 是 |
| 描述 | 节点功能说明 | 否 |
| ToolNames | 工具名称列表 | 是 |
| EnableParallelTools | 并行执行多个工具调用 | 否 |
| ToolCallRetryPolicy | 工具调用重试策略 | 否 |
| InterruptBefore | 执行前中断 | 否 |
| InterruptAfter | 执行后中断 | 否 |

### 6.4 Agent 节点

| 属性 | 说明 | 必填 |
|------|------|------|
| 节点 ID | 唯一标识 | 是 |
| 描述 | 节点功能说明 | 否 |
| AgentName | 引用的系统 Agent 名称 | 是 |
| InputMapper | 输入映射（State → Agent 运行时状态） | 否 |
| OutputMapper | 输出映射（Agent 结果 → State 更新） | 否 |
| IsolatedMessages | 是否隔离会话历史 | 否 |
| InputFromLastResponse | 上游 last_response → 下游 user_input | 否 |
| EventScope | 子图事件作用域 | 否 |
| InterruptBefore | 执行前中断 | 否 |
| InterruptAfter | 执行后中断 | 否 |

### 6.5 Router 节点

| 属性 | 说明 | 必填 |
|------|------|------|
| 节点 ID | 唯一标识 | 是 |
| 描述 | 路由逻辑说明 | 否 |
| CondFuncRef | 条件函数引用 | 是 |
| PathMap | 分支路径映射 | 是 |
| Destinations | 声明可能的动态路由目标 | 否 |

---

## 7. 编辑器交互需求

### 7.1 编辑器布局

```
┌──────────┬───────────────────────────┬──────────────┐
│ 组件面板  │        画布区域           │  属性面板    │
│          │                           │              │
│ Function │   ┌───┐   ┌───┐          │ 节点ID       │
│ LLM      │   │ A │──▶│ B │          │ 类型         │
│ Tool     │   └───┘   └─┬─┘          │ 指令/引用    │
│ Agent    │             │             │ 模型/工具    │
│ Router   │         ┌───▼───┐         │ 中断设置     │
│ Join     │         │   C   │         │ I/O 映射     │
│          │         └───────┘         │              │
│ ──────── │                           │ ──────────── │
│ State    │                           │ State Schema │
│ Schema   │                           │ 字段列表     │
│ ──────── │                           │ ──────────── │
│ 模板库   │                           │ 校验结果     │
└──────────┴───────────────────────────┴──────────────┘
```

### 7.2 编辑原则

- **拖拽优先**：所有节点和连线通过拖拽操作完成
- **属性面板**：选中节点后右侧弹出属性编辑面板
- **实时预览**：编辑过程中实时预览 Graph 拓扑
- **智能提示**：连线时自动提示可连接的节点
- **校验反馈**：保存前校验 Graph 定义完整性

### 7.3 编辑流程

1. 从左侧组件面板拖拽节点到画布
2. 拖拽连线定义执行顺序
3. 双击节点编辑属性（Agent 引用、指令、工具等）
4. 定义 State Schema（共享数据结构）
5. 设置中断点（HITL）
6. 校验并保存
7. 测试执行

### 7.4 节点样式（对齐 trpc-agent-go visualize.go）

| NodeType | 形状 | 填充色 | 边框色 |
|----------|------|--------|--------|
| LLM | 矩形 | `#e3f2fd` | `#2196f3` |
| Tool | 矩形 | `#fff3e0` | `#ff9800` |
| Agent | 矩形 | `#e8f5e9` | `#4caf50` |
| Router | 菱形 | `#eeeeee` | `#757575` |
| Join | 菱形 | `#f3e5f5` | `#9c27b0` |
| Function | 矩形 | `#f3e5f5` | `#9c27b0` |
| Start | 椭圆 | `#e1f5e1` | `#4caf50` |
| End | 椭圆 | `#ffe1e1` | `#f44336` |

### 7.5 执行状态样式

| 状态 | 节点样式 | 说明 |
|------|----------|------|
| idle | 默认样式 | 未执行 |
| running | 脉冲动画 + 蓝色光晕 | 正在执行 |
| completed | 绿色勾标记 | 执行完成 |
| failed | 红色叉标记 + 红色边框 | 执行失败 |
| interrupted | 黄色暂停标记 | HITL 中断 |
| waiting | 灰色 | 等待执行 |

---

## 8. trpc-agent-go 框架参照

### 8.1 框架核心结构

```
pkg/trpc-agent-go/graph/
├── graph.go              # Graph 核心结构：AddNode/AddEdge/AddConditionalEdges/Compile
├── state_graph.go        # StateGraph 构建器：NewStateGraph/NewStateSchema/AddField/AddNode/AddLLMNode/AddToolNode
├── state.go              # State 管理：StateSchema/StateField/Reducer/Command
├── executor.go           # Executor：执行编译后的 Graph（BSP 引擎）
├── executor_dag.go       # DAG 执行引擎：并行节点调度
├── checkpoint.go         # Checkpoint：持久化执行状态（CheckpointTuple/PendingWrite/InterruptState）
├── interrupt.go          # HITL 中断：InterruptError/Interrupt()
├── resume.go             # 恢复执行：ResumeCommand/ResumeValue
├── time_travel.go        # 时间旅行：回放历史状态
├── cache.go              # 缓存：CachePolicy/CacheKey
├── retry.go              # 重试：RetryPolicy（指数退避 + Jitter）
├── stream.go             # 流式：事件流输出
├── events.go             # 事件：Graph 执行事件（ObjectType 常量 + Metadata 结构）
├── callbacks.go          # 回调：BeforeNode/AfterNode/OnNodeError/AgentEvent
├── visualize.go          # 可视化：生成 DOT 格式（NodeType 样式映射）
├── errors.go             # 错误：Graph 执行错误
├── message_ops.go        # 消息操作：状态消息处理
├── completion_control.go # 完成控制：Graph 终止条件
├── external_interrupt.go # 外部中断：运行时中断 Graph
├── static_interrupt.go   # 静态中断：编译时中断点
├── trace_task.go         # 追踪：执行追踪
├── surface_runtime.go    # Surface Runtime：运行时表面管理
├── execution_engine.go   # 执行引擎接口：BSP/DAG
├── call_options.go       # 调用选项：Per-call 配置
├── keys.go               # 状态键常量
└── utils.go              # 工具函数
```

### 8.2 框架事件类型（events.go）

| ObjectType 常量 | 说明 | 对应前端事件 |
|------------------|------|-------------|
| `graph.execution` | Graph 执行完成 | `graph_execution_done` |
| `graph.node.start` | 节点开始执行 | `graph_node_start` |
| `graph.node.complete` | 节点执行完成 | `graph_node_end` |
| `graph.node.error` | 节点执行错误 | `graph_node_error` |
| `graph.node.custom` | 节点自定义事件 | `graph_node_custom` |
| `graph.pregel.step` | Pregel 步骤事件 | `graph_step` |
| `graph.pregel.planning` | Pregel 规划阶段 | `graph_planning` |
| `graph.pregel.execution` | Pregel 执行阶段 | `graph_pregel_exec` |
| `graph.pregel.update` | Pregel 更新阶段 | `graph_pregel_update` |
| `graph.state.update` | 状态更新事件 | `state_delta` |
| `graph.checkpoint` | 检查点事件 | `checkpoint` |
| `graph.checkpoint.created` | 检查点创建事件 | `checkpoint_created` |
| `graph.checkpoint.committed` | 检查点提交事件 | `checkpoint_committed` |
| `graph.checkpoint.interrupt` | 检查点中断事件 | `checkpoint`（HITL） |
| `graph.channel.update` | Channel 更新事件 | `channel_update` |
| `graph.barrier` | Graph 级 Barrier 事件 | `graph_barrier` |
| `graph.node.barrier` | 节点级 Barrier 事件 | `node_barrier` |

### 8.3 框架 Metadata 结构

| Metadata 类型 | 说明 | 包含字段 |
|---------------|------|----------|
| `NodeExecutionMetadata` | 节点执行元数据 | NodeID, NodeType, Phase, StartTime, EndTime, Duration, InputKeys, OutputKeys, Error, StepNumber, Attempt |
| `ToolExecutionMetadata` | 工具执行元数据 | ToolName, ToolID, Phase, Duration, Input, Output, Error |
| `ModelExecutionMetadata` | 模型执行元数据 | ModelName, NodeID, Phase, Duration, Input, Output, Error, StepNumber |
| `PregelStepMetadata` | Pregel 步骤元数据 | StepNumber, Phase, TaskCount, ActiveNodes, Duration, CheckpointID, LineageID, InterruptKey |
| `StateUpdateMetadata` | 状态更新元数据 | UpdatedKeys, RemovedKeys, StateSize |
| `CompletionMetadata` | 完成元数据 | TotalSteps, TotalDuration, FinalStateKeys, FinalResponseID |
| `ChannelUpdateMetadata` | Channel 更新元数据 | ChannelName, ChannelType, ValueCount, TriggeredNodes |
| `NodeCustomEventMetadata` | 自定义事件元数据 | EventType, Category, NodeID |

---

## 9. 需求优先级总览

### P0 — 核心能力（必须实现）

| # | 需求 | 状态 | 说明 |
|---|------|------|------|
| 1 | State Schema + Reducer | ✅ 已实现 | GraphBuildConfig 支持 StateFields |
| 2 | 条件路由 | ✅ 已实现 | ConditionalEdgeDef + CondFuncRef |
| 3 | HITL 中断/恢复 | ✅ 已实现 | InterruptBefore/After + Resume API |
| 4 | 事件桥接到 WS | ✅ 已实现 | EventBridge + EventBus |
| 5 | 前端编辑器 | ✅ 已实现 | Vue Flow 画布 |
| 6 | 前端执行监控 | ✅ 已实现 | 节点状态高亮 |
| 7 | 基础拓扑校验 | ⚠️ 部分 | 框架编译时校验已有，产品层校验缺失 |
| 8 | Agent 引用校验 | ❌ 未实现 | 保存时校验 Agent 节点引用的 Agent 存在 |

### P1 — 增强能力（应该实现）

| # | 需求 | 状态 | 说明 |
|---|------|------|------|
| 9 | 检查点持久化（SQLite） | ⚠️ 部分 | InMemory 已实现，SQLite Saver 适配器已有但未注入 |
| 10 | 时间旅行调试 | ✅ 已实现 | TimeTravel API + 前端时间线 |
| 11 | 子图嵌套 | ✅ 已实现 | SubgraphDef + InputMapper/OutputMapper |
| 12 | DAG 执行引擎 | ✅ 已实现 | ExecutionEngine 配置 |
| 13 | 节点重试配置 | ❌ 未实现 | 节点属性面板支持配置 RetryPolicy |
| 14 | 设计模式模板 | ❌ 未实现 | 内置 6 种常见模式模板 |
| 15 | State Schema 校验 | ❌ 未实现 | 节点读写字段已定义、Reducer 类型匹配 |
| 16 | 循环退出校验 | ❌ 未实现 | 循环路径有退出条件 |
| 17 | LLM 节点配置 | ❌ 未实现 | Instruction + Model + Tools 配置面板 |
| 18 | Tool 节点配置 | ❌ 未实现 | ToolNames + ParallelTools 配置面板 |
| 19 | Agent 节点 I/O 映射 | ❌ 未实现 | InputMapper/OutputMapper 可视化配置 |
| 20 | 子图复用 | ❌ 未实现 | 常用流程片段封装为子图跨 Graph 复用 |
| 21 | Graph 模板 | ❌ 未实现 | 从已有 Graph 创建模板 |
| 22 | 执行摘要 | ❌ 未实现 | 最终状态、总耗时、总 Token |
| 23 | 步骤时间线 | ❌ 未实现 | 按时间顺序展示每个节点的执行结果 |

### P2 — 高级能力（可以实现）

| # | 需求 | 状态 | 说明 |
|---|------|------|------|
| 24 | 节点缓存配置 | ❌ 未实现 | CachePolicy + CacheKeyFields |
| 25 | Graph 版本管理 | ❌ 未实现 | 定义版本化，支持回滚 |
| 26 | 导入/导出 | ❌ 未实现 | Graph 定义 JSON 导入导出 |
| 27 | 结果导出 | ❌ 未实现 | 导出最终 State 为 JSON |
| 28 | 并发安全校验 | ❌ 未实现 | 并行节点无 State 写冲突 |
| 29 | 自定义模板保存 | ❌ 未实现 | 用户可将已有 Graph 保存为模板 |
| 30 | Redis Checkpoint | ❌ 未实现 | 分布式场景下的 Checkpoint 共享 |

### P3 — 远期能力

| # | 需求 | 状态 | 说明 |
|---|------|------|------|
| 31 | 智能推荐 | ❌ 未实现 | 根据用户描述推荐工作流模式 |
| 32 | 分享协作 | ❌ 未实现 | Graph 定义可分享给其他用户 |
| 33 | 市场/商店 | ❌ 未实现 | 社区共享 Graph 模板 |
| 34 | 自动优化 | ❌ 未实现 | 基于执行历史自动建议优化 |

---

## 10. 任务派工与执行规则

> 参考 Hermes Kanban 理念，引入明确的角色派工、安全原则和任务生命周期管理。

### 10.1 Agent 角色定义

系统应支持为每个 Agent 定义一组**角色（Roles）**或**能力标签（Capabilities）**：

- 角色可用于派工匹配，确保任务被具备相应能力的 Agent 执行
- Agent 可拥有多个角色标签
- 角色应支持层级结构（如 "senior-reviewer" 继承 "reviewer"）

### 10.2 基于角色的 Agent 选择

工作流节点可指定"所需角色"或"特定 Agent ID"作为执行者：

- **静态指派**：在设计时指定明确的 Agent（现有 `agent_name` 字段）
- **动态指派**：运行时按角色匹配，从 Team 中筛选符合条件的 Agent
- **指派策略**：若多人匹配则支持策略选择（如最少任务数、随机、手动选择）
- **安全原则**：若无可匹配 Agent，节点应进入**挂起状态（Pending Assignment）**并发送通知，而不是随机指派。遵循"宁愿停滞，不错派"原则

### 10.3 任务模型

工作流实例化后，每个待执行的 Agent 节点将生成一个**任务（Task）**：

| 字段 | 说明 |
|------|------|
| task_id | 任务唯一标识 |
| node_id | 关联的工作流节点 |
| execution_id | 关联的工作流执行实例 |
| assignee | 具体 Agent ID 或角色 |
| status | 任务状态（见 10.4） |
| context | 执行上下文 |
| input | 输入数据 |
| output | 结构化交付物 |
| summary | 任务摘要 |
| metadata | 元数据（执行耗时、模型调用次数等） |
| created_at / claimed_at / completed_at | 时间戳 |

### 10.4 标准化任务状态机

每个任务必须遵循严格的状态机：

```
pending → claimed → complete
                  → review_required → complete / claimed
                  → blocked → claimed
                  → failed → claimed（重试）/ cancelled
                  → timed_out → claimed（重试）/ failed
         → cancelled
```

| 状态 | 说明 |
|------|------|
| `pending` | 等待被领取或指派 |
| `claimed` | 已被 Agent 领取，执行中 |
| `complete` | 执行成功，产出符合规范的交付物 |
| `blocked` | 执行受阻，需外部输入或人工决策 |
| `review_required` | 产出需要审核 |
| `failed` | 执行失败（系统可自动重试或进入人工处理） |
| `timed_out` | 执行超时 |
| `cancelled` | 被取消 |

### 10.5 强制完成回调

Agent 完成任务时，必须调用标准的完成回调，提供：

- 任务摘要（summary）
- 元数据（执行耗时、模型调用次数等）
- 结构化交付物（output）

如果 Agent 因故障退出而未调用回调，系统应在检测到进程/会话失效后自动标记为 `crashed` 或 `gave_up`。

---

## 11. 审核与质量门禁

### 11.1 审核节点

- 提供**人工审核节点**（现有 HITL InterruptBefore/After）和**自动审核节点（Reviewer Agent）**
- 审核节点作用于上游 Agent 节点的输出，可配置审核规则
- 审核通过后，任务状态变为 `complete`；驳回则返回 `claimed` 或 `blocked`，并附审核意见

### 11.2 评论与反馈

- 审核节点应支持添加评论，评论附着在任务上，供 Agent 迭代参考
- 评论应包含：审核人、时间、内容、类型（approve/reject/suggestion）

---

## 12. 全链路可观测性

### 12.1 结构化日志

- 系统应为每个任务记录单独的 stdout 和 stderr 日志
- 工作流运行实例应提供统一的日志视图，可按节点、时间、日志级别过滤

### 12.2 运行历史

- 记录每次任务尝试（task_run），包含启动时间、结束时间、退出码、日志引用等
- 支持查看任务的所有历史运行记录

### 12.3 事件追踪

- 记录任务状态变更的完整事件流（task_events），如 `task_created`、`task_claimed`、`heartbeat`、`task_completed` 等
- 提供图形化的甘特图或时间线视图展示工作流执行轨迹

---

## 13. 智能超时与重试机制

### 13.1 进程存活感知的超时管理

- 系统不应仅依赖挂钟时间判定超时，应结合 Agent 的心跳上报
- 如果在执行期间 Agent 持续发送心跳，且未超出最大执行时间，允许延长任务租约

### 13.2 失败重试与熔断

- 可配置节点的自动重试次数和重试间隔（现有 `WithRetryPolicy`）
- 重试耗尽后，任务标记为 `failed`，可选触发人工干预或执行补偿节点
- 支持熔断策略：当连续失败达到阈值时，暂停整个工作流分支

---

## 14. 对外集成与扩展能力

### 14.1 自定义 Agent 执行器

- 提供插件机制（如 `spawn_fn`），允许开发者注册外部 CLI 程序、HTTP 服务或自定义代码作为可调度的 Worker
- 定义清晰的接口契约：输入上下文格式、退出码映射、输出解析规则

### 14.2 Webhook 与 API

- 工作流关键节点支持发送 Webhook 通知
- 外部系统可通过 API 触发工作流、查询状态、推送事件

### 14.3 Agent 交互 API

Agent 领取任务、心跳上报、提交结果的标准协议：

| API | 说明 |
|-----|------|
| ClaimTask | Agent 领取任务 |
| Heartbeat | Agent 心跳上报 |
| SubmitResult | Agent 提交任务结果 |
| ReportBlocked | Agent 报告执行受阻 |

---

## 15. 非功能需求

| 维度 | 要求 |
|------|------|
| **性能** | 图引擎应能处理上千节点规模的图编译与执行，单任务调度延迟低于 100ms |
| **可靠性** | 支持工作流执行的事务性保障，关键状态变更必须持久化 |
| **可扩展性** | 节点类型、Agent 执行器、工具等应支持热插拔式扩展 |
| **安全性** | 多租户隔离，敏感状态字段支持加密存储 |
| **易用性** | 可视化编辑器提供设计时校验、自动布局、撤销重做等功能 |

---

## 16. 数据需求

| 数据模型 | 说明 |
|----------|------|
| 工作流定义模型 | 包含节点集、边集、状态模式、全局配置 |
| 运行实例模型 | 关联工作流定义 ID，持有当前状态快照、检查点引用 |
| 任务模型 | 包含指派信息、状态、输入输出、日志引用、审核信息（见 10.3） |
| 事件模型 | 时间戳、事件类型、源节点、描述 |
| 日志模型 | 任务级 stdout/stderr 日志 |
| 运行历史模型 | 每次任务尝试的启动时间、结束时间、退出码、日志引用 |
