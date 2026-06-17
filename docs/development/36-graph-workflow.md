# M4: Graph 工作流 — 详细需求（v3）

> **编排融合（M53）**：[53 team-graph-orchestration.md](./53%20team-graph-orchestration.md) — Team mode 编译为 Graph、统一运行观测。

> 对标 `pkg/trpc-agent-go/graph` 包，构建"LangGraph for Go"级别的确定性工作流引擎。
> 本文档从四个核心维度梳理 Graph 的产品定位、需求层级和演进路线。
> 架构设计、Proto 契约、数据模型详见 [36-graph-workflow.design.md](./36-graph-workflow.design.md)；代码锚点、现状评估、任务清单详见 [36-graph-workflow.development.md](./36-graph-workflow.development.md)。

---

## 0. Graph 的存在意义

### 0.1 Team vs Graph 的本质差异

| 维度 | Team | Graph |
|------|------|-------|
| 编排哲学 | 模式化协作——选择预定义模式，配置成员 | 自由编排——画布定义任意拓扑，精确控制每一步 |
| 控制权 | 框架决定执行顺序，Agent 自主推理 | 用户决定流程骨架，Agent 在节点内自主推理 |
| 条件分支 | 无（模式固定） | 支持（基于状态的条件路由、Command.GoTo） |
| 人工介入 | 无 | HITL 中断/恢复（审批节点） |
| 状态管理 | 隐式（消息传递） | 显式 State Schema + Reducer |
| 可回溯性 | TeamRun 步骤列表 | Checkpoint + TimeTravel 任意状态回放 |
| 并行控制 | 模式级（parallel 整体并行） | 节点级（DAG 引擎自动并行无依赖节点） |
| 适用场景 | 简单协作（多人讨论、流水线） | 复杂业务流程（审批流、数据处理管线、多阶段决策） |

**核心洞察**：Team 解决"多 Agent 如何协作"，Graph 解决"复杂流程如何确定性地执行"。二者互补——Graph 中的节点本身可以是 Team。

### 0.2 Graph 的三大核心价值

1. **确定性流程控制**：Agent 节点内部拥有 LLM 自主推理能力，但节点间流转由确定性图规则控制
2. **运行态透明性**：每个节点执行到哪、输入输出是什么、卡在哪里，一目了然
3. **流程可干预可回溯**：内置人工审批、检查点恢复、全链路运行轨迹，保障流程可控可回溯

---

## 1. 四维需求架构

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

> 现状对齐（已实现/待实现能力清单）详见 [36-graph-workflow.development.md §2 现状评估](./36-graph-workflow.development.md#2-现状评估)。

---

## 2. 维度一：图结构与混合控制

### 2.1 统一节点类型体系（P1）

**用户故事**：作为工作流设计者，我希望在画布上拖拽不同类型的节点（Function/LLM/Tool/Agent/Router/Join），每种节点有对应的属性配置面板，以便灵活构建复杂工作流。

**功能规格**：

| 节点类型 | 用途 | 视觉样式 | 必填属性 |
|----------|------|----------|----------|
| Function | 纯逻辑处理 | 紫色矩形 | FuncRef |
| LLM | 轻量级 LLM 调用 | 蓝色矩形 | Instruction + Model |
| Tool | 直接调用工具 | 橙色矩形 | ToolNames |
| Agent | 引用系统 Agent | 绿色矩形 | AgentName |
| Router | 条件路由 | 灰色菱形 | CondFuncRef + PathMap |
| Join | 汇聚并行分支 | 紫色菱形 | 自动处理 |

**验收标准**：
- [x] LLM 节点可在画布上创建，属性面板支持配置 Instruction/Model/Tools/UserInputKey/GenerationConfig
- [x] Tool 节点可在画布上创建，属性面板支持配置 ToolNames/EnableParallelTools
- [x] Agent 节点支持配置 InputMapper/OutputMapper/IsolatedMessages/InputFromLastResponse
- [x] 各节点类型支持 InterruptBefore/After 配置
- [x] 各节点类型支持 RetryPolicy/CachePolicy 配置

### 2.2 Agent 节点的混合控制（P1）

**用户故事**：作为工作流设计者，我希望 Agent 节点内部自主推理但节点间流转由图规则控制，以便兼顾灵活性和确定性。

**功能规格**：
- Agent 节点内部：拥有完整 LLM 推理能力，可自主决策、调用工具
- Agent 节点外部：输入来自 Graph State（InputMapper），输出写回 Graph State（OutputMapper）
- 流转控制：Agent 执行完毕后，下一个节点由图的边规则决定

**验收标准**：
- [x] Agent 节点 InputMapper 将 Graph State 投影为 Agent 运行时状态
- [x] Agent 节点 OutputMapper 将 Agent 输出写回 Graph State
- [x] `WithSubgraphIsolatedMessages` 可隔离 Agent 会话历史
- [x] `WithSubgraphInputFromLastResponse` 支持上游 last_response → 下游 user_input

### 2.3 边类型与流转规则（P1）

**用户故事**：作为工作流设计者，我希望定义确定性流转、条件路由和动态路由三种边类型，以便精确控制流程走向。

**功能规格**：

| 边类型 | 说明 | 视觉表示 |
|--------|------|----------|
| Runtime Edge | 确定性流转，A 完成后必定到 B | 实线箭头 |
| Conditional Edge | 条件路由，根据状态选择分支 | 虚线箭头 + 条件标签 |
| Command Edge | 节点内部通过 Command.GoTo 动态路由 | 动态，运行时决定 |

**验收标准**：
- [x] Runtime Edge 确定性流转正常工作
- [x] Conditional Edge 根据 CondFunc 返回值选择正确分支
- [x] Command.GoTo 动态路由事件通过 WS 推送到前端
- [x] 前端执行监控中动态高亮实际执行路径

### 2.4 执行引擎选择（P2）

**用户故事**：作为工作流设计者，我希望选择 BSP 或 DAG 执行引擎，以便在不同场景下获得最优性能。

**功能规格**：

| 引擎 | 适用场景 |
|------|----------|
| BSP（默认） | 需要确定性、可复现的场景 |
| DAG | 高吞吐、节点间无复杂状态交互的场景 |

**验收标准**：
- [x] Graph 定义中可选择执行引擎
- [x] BSP 引擎按 Pregel 步骤执行，每步同步
- [x] DAG 引擎分析节点依赖，无依赖节点并行执行

### 2.5 子图嵌套（P2）

**用户故事**：作为工作流设计者，我希望将常用流程片段封装为子图节点，以便跨 Graph 复用。

**功能规格**：
- 子图编译后作为 Agent 节点注册
- 子图状态通过 InputMapper/OutputMapper 与父图映射
- 子图支持独立 Checkpoint 和 TimeTravel
- 子图事件通过 `WithSubgraphEventScope` 限定作用域

**验收标准**：
- [ ] 子图节点可在画布上创建，引用另一个 Graph 定义
- [ ] 子图 InputMapper/OutputMapper 正确映射状态
- [ ] 子图执行事件不污染父图事件流

---

## 3. 维度二：动态拓扑与状态共享

### 3.1 条件路由（✅ 已实现）

**用户故事**：作为工作流设计者，我希望基于状态动态选择执行路径，以便构建分支逻辑。

**验收标准**：
- [x] Router 节点支持选择已注册条件函数（CondFuncRef）
- [x] Router 节点支持编辑路径映射表（PathMap）
- [x] 前端条件分支以虚线箭头 + 标签展示

### 3.2 动态节点生成（Command.GoTo）（P1）

**用户故事**：作为工作流设计者，我希望节点执行时动态决定下一步，以便处理运行时才能确定的流程走向。

**功能规格**：
- 节点属性面板支持配置 `Destinations`（声明可能的动态路由目标）
- 运行时 Command.GoTo 事件通过 WS 推送到前端
- 前端执行监控中动态高亮实际执行路径

**验收标准**：
- [x] 节点 `Destinations` 字段在属性面板可编辑
- [x] `WithEndsMap` 在 builder 中正确接线
- [x] Command.GoTo 事件通过 WS `graph_node_custom` 推送

### 3.3 动态任务节点插入（BabyAGI 启发，P2）

> 来源：BabyAGI Task Creation Agent（GitHub 22k+ stars），竞品分析差距 #8
> 对应需求：`docs/competitive-gap-requirements-2026-05-31.md` P2-7

**用户故事**：作为工作流设计者，我希望 Agent 节点执行完后能根据结果动态插入新任务节点到 DAG，以便处理执行时才能发现的子任务。

**背景**：当前 Graph 工作流的 DAG 是预定义的，Agent 节点只能在预定义路径中选择。BabyAGI 的 Task Creation Agent 展示了一种更自主的模式——Agent 执行完一个任务后，根据结果动态生成新任务并插入队列。将此能力引入 Graph，可实现"预定义骨架 + 运行时动态扩展"的混合编排。

**功能规格**：
- Agent 节点执行完后可 emit `DynamicNodeInsert` 事件，携带新节点定义（AgentKey/Input/Dependencies）
- Graph Executor 接收事件后动态插入新节点到 DAG，自动建立边连接
- 动态插入的节点在 `TaskStatus` 中标记为 `dynamic_spawn`
- 前端 Graph 执行监控中动态渲染新增节点（虚线边 + `dynamic_spawn` 标签）
- 动态节点受 `SessionRun` 预算控制——预算不足时拒绝插入

**验收标准**：
- [ ] Agent 节点可通过事件动态插入新节点
- [ ] 动态节点在 Graph 可视化中标注为 `dynamic_spawn`
- [ ] 动态节点受预算控制
- [ ] 动态节点执行结果自动沉淀为记忆（L2 语义召回）

### 3.4 全局共享工作流状态（✅ 已实现）

**用户故事**：作为工作流设计者，我希望所有节点共享同一个 Graph State，以便无感传递上下文。

**验收标准**：
- [x] 编辑器提供 State Schema 编辑面板
- [x] 支持 4 种 Reducer（Default/Append/Cover/Merge）
- [x] 节点属性面板显示该节点读写的 State 字段

### 3.5 State Schema 校验（✅ 已实现）

**用户故事**：作为工作流设计者，我希望保存 Graph 时自动校验 State Schema 完整性，以便避免运行时错误。

**验收标准**：
- [x] 校验 State Schema 字段完整性
- [x] 校验 Reducer 类型匹配
- [x] 校验 Agent 引用存在性
- [x] 校验基础拓扑（入口点、孤立节点、不可达节点）

---

## 4. 维度三：人机协同与可观测性

### 4.1 人工审批节点（✅ 已实现）

**用户故事**：作为工作流设计者，我希望在关键决策点插入人工审批，以便保障流程可控。

**验收标准**：
- [x] 节点属性面板支持勾选 InterruptBefore/After
- [x] WS `checkpoint` 事件触发前端确认对话框
- [x] `ResumeGraph` API 接受用户输入并恢复执行

### 4.2 状态检查点与恢复（✅ 已实现）

**用户故事**：作为工作流运维者，我希望查看和恢复历史检查点，以便回溯和调试执行过程。

**验收标准**：
- [x] Checkpoint 自动保存
- [x] `ListCheckpoints` / `GetStateSnapshot` / `EditState` API
- [x] SQLite Checkpoint Saver 适配器

### 4.3 时间旅行调试（✅ 已实现）

**用户故事**：作为工作流运维者，我希望回放到任意历史步骤并编辑状态重新执行，以便调试和探索不同路径。

**验收标准**：
- [x] TimeTravel API 查询任意步骤状态
- [x] EditState + Resume 从编辑点重新执行

### 4.4 全链路运行轨迹（P1 增强）

**用户故事**：作为工作流运维者，我希望实时查看每个节点的执行状态、耗时和资源消耗，以便快速定位问题。

**功能规格**：

| 观测维度 | 优先级 | 说明 |
|----------|--------|------|
| 执行进度 | P0 | 当前执行到哪个节点、已执行/总节点数 |
| 节点状态 | P0 | 每个节点的运行/等待/完成/失败/中断状态 |
| 数据流 | P0 | 每个节点的输入/输出 State 快照 |
| 时间信息 | P1 | 每个节点的开始/结束时间、耗时 |
| 资源消耗 | P1 | 每个节点的 Token 用量、成本 |
| 错误信息 | P0 | 失败节点的错误详情、重试状态 |

**验收标准**：
- [x] WS `graph_node_start/end/error` 事件实时推送节点状态
- [x] 前端节点颜色状态：运行中（脉冲动画）、完成（绿色勾）、失败（红色叉）、中断（黄色暂停）
- [x] Graph 执行完成后推送 `graph_execution_done` 事件，包含 ExecutionSummary
- [x] ExecutionSummary 包含总步骤、总耗时、各节点执行详情

### 4.5 运行时操作（✅ 已实现）

| 操作 | 状态 | 说明 |
|------|------|------|
| HITL 确认 | ✅ | `ResumeGraph` API |
| 取消执行 | ✅ | `CancelGraphExecution` API |
| 重试失败节点 | ✅ | 从失败检查点 Resume + RetryPolicy 自动重试 |
| 修改状态 | ✅ | `EditState` + `ResumeGraph` |
| 时间旅行 | ✅ | `GetStateSnapshot` API |

---

## 5. 维度四：设计辅助与资产复用

### 5.1 设计时校验（✅ 已实现）

**用户故事**：作为工作流设计者，我希望保存 Graph 时自动校验定义的正确性，以便在设计阶段就发现错误。

**验收标准**：
- [x] 基础拓扑校验（入口点、孤立节点、不可达节点）
- [x] Agent 引用校验
- [x] State Schema 校验（字段完整性、Reducer 类型匹配）
- [x] 循环退出校验
- [x] FuncRef/CondFuncRef 注册校验

### 5.2 设计模式模板（✅ 已实现）

**用户故事**：作为工作流设计者，我希望从预置模板快速创建工作流，以便降低设计门槛。

**验收标准**：
- [x] 内置 6 种模板（顺序流水线/审批流/并行评审/生成-评审循环/条件分发/子图嵌套）
- [x] `ListGraphTemplates` / `CreateGraphFromTemplate` API
- [x] 前端模板选择面板

### 5.3 资产复用（P2）

**用户故事**：作为工作流设计者，我希望将已有 Graph 保存为模板、导入导出定义、管理版本，以便团队协作和资产积累。

**功能规格**：

| 资产类型 | 优先级 | 说明 |
|----------|--------|------|
| 用户自定义模板 | P2 | 将已有 Graph 保存为模板 |
| Graph 版本管理 | P2 | 定义版本化，支持回滚 |
| 导入/导出 | P2 | Graph 定义 JSON 导入导出 |
| 子图复用 | P2 | 将常用流程片段封装为子图，跨 Graph 复用 |

**验收标准**：
- [x] 用户可将已有 Graph 保存为自定义模板
- [x] Graph 定义支持版本化存储和回滚
- [x] Graph 定义可导出为 JSON 并从 JSON 导入

### 5.4 节点结果缓存与重试（P2）

**用户故事**：作为工作流设计者，我希望为节点配置重试策略和缓存策略，以便提高执行可靠性和效率。

**功能规格**：
- 节点属性面板支持配置 RetryPolicy（最大重试次数、退避策略）
- 节点属性面板支持配置 CachePolicy（缓存键字段、TTL）
- Graph 级默认重试策略

**验收标准**：
- [x] 节点属性面板支持配置重试策略
- [x] 节点属性面板支持配置缓存策略
- [x] 失败节点按重试策略自动重试

---

## 6. 任务派工与执行规则

> **Hermes Kanban 闭环（运行时 + Dispatcher + Tools）**：见 [54-hermes-kanban.md](./54-hermes-kanban.md)。本节 RPC/Ent 已实现；Graph 节点自动建任务与 Dispatcher 见 M54 Phase 1。

### 6.1 任务模型与状态机（✅ 已实现）

**用户故事**：作为工作流运维者，我希望每个节点执行都作为独立任务管理，以便跟踪状态、指派 Agent、记录结果。

**验收标准**：
- [x] 任务状态机：pending → claimed → complete/blocked/review_required/failed/timed_out/cancelled
- [x] `CreateTask` / `ClaimTask` / `SubmitTaskResult` / `ReportBlocked` API
- [x] 任务持久化（Ent Schema: graph_task）

### 6.2 Agent 角色与动态派工（✅ 已实现）

**用户故事**：作为工作流设计者，我希望节点可按角色动态指派 Agent，以便实现灵活的人力/Agent 调度。

**验收标准**：
- [x] 节点支持 `required_role` + `assignment_mode`（static/dynamic）
- [x] 动态派工按 `assignment_strategy`（least_tasks/random/manual）选择 Agent
- [x] 无匹配 Agent 时任务状态为 `pending_assignment`

### 6.3 审核与质量门禁（✅ 已实现）

**用户故事**：作为工作流设计者，我希望在关键节点设置审核门禁，以便保障输出质量。

**验收标准**：
- [x] 人工审核：复用 HITL InterruptBefore/After
- [x] 自动审核：Reviewer Agent 对上游输出审核
- [x] 审核评论：`AddTaskComment` / `ListTaskComments` API
- [x] 审核通过 → complete，驳回 → claimed

### 6.4 智能超时与重试（✅ 已实现）

**用户故事**：作为工作流运维者，我希望系统结合心跳感知超时，而非仅依赖挂钟时间，以便避免误判。

**验收标准**：
- [x] `Heartbeat` API 支持心跳上报
- [x] 心跳感知超时：持续心跳时延长租约
- [x] 超时后任务标记为 `timed_out`

### 6.5 全链路可观测性（✅ 已实现）

**验收标准**：
- [x] 结构化日志：`ListTaskLogs` API
- [x] 运行历史：`ListTaskRuns` API
- [x] 事件追踪：`ListTaskEvents` API

### 6.6 对外集成（✅ 部分实现）

| API | 状态 | 说明 |
|-----|------|------|
| ClaimTask | ✅ | Agent 领取任务 |
| Heartbeat | ✅ | Agent 心跳上报 |
| SubmitTaskResult | ✅ | Agent 提交结果 |
| ReportBlocked | ✅ | Agent 报告阻塞 |
| Webhook 通知 | ✅ | `GraphTaskRuntime` → `graph.task.status` Webhook |
| 熔断策略 | 🟡 | Proto `CircuitBreakerPolicy` 已定义，未接入 NodeDef |

---

## 7. 编辑器交互需求

### 7.1 编辑器布局

- 左侧：组件面板（Function/LLM/Tool/Agent/Router/Join + State Schema + 模板）
- 中间：Vue Flow 画布区域
- 右侧：属性面板（节点属性 + State Schema 字段列表 + 校验结果）

### 7.2 节点样式

| NodeType | 形状 | 填充色 | 边框色 |
|----------|------|--------|--------|
| LLM | 矩形 | `#e3f2fd` | `#2196f3` |
| Tool | 矩形 | `#fff3e0` | `#ff9800` |
| Agent | 矩形 | `#e8f5e9` | `#4caf50` |
| Router | 菱形 | `#eeeeee` | `#757575` |
| Join | 菱形 | `#f3e5f5` | `#9c27b0` |
| Function | 矩形 | `#f3e5f5` | `#9c27b0` |

### 7.3 执行状态样式

| 状态 | 节点样式 |
|------|----------|
| idle | 默认样式 |
| running | 脉冲动画 + 蓝色光晕 |
| completed | 绿色勾标记 |
| failed | 红色叉标记 + 红色边框 |
| interrupted | 黄色暂停标记 |
| waiting | 灰色 |

---

## 8. 非功能需求

| 维度 | 要求 |
|------|------|
| **性能** | 图引擎应能处理上千节点规模的图编译与执行，单任务调度延迟低于 100ms |
| **可靠性** | 支持工作流执行的事务性保障，关键状态变更必须持久化 |
| **可扩展性** | 节点类型、Agent 执行器、工具等应支持热插拔式扩展 |
| **安全性** | 多租户隔离，敏感状态字段支持加密存储 |
| **易用性** | 可视化编辑器提供设计时校验、自动布局、撤销重做等功能 |

---

## 9. 数据需求

> 数据模型详细设计（Ent Schema、字段定义、表关系）详见 [36-graph-workflow.design.md §8 数据模型](./36-graph-workflow.design.md#八数据模型)。

| 数据模型 | 状态 | 说明 |
|----------|------|------|
| 工作流定义模型 | ✅ | 节点集、边集、状态模式、全局配置 |
| 运行实例模型 | ✅ | 关联定义 ID、当前状态、检查点引用 |
| 任务模型 | ✅ | 指派信息、状态、输入输出 |
| 事件模型 | ✅ | 事件类型、源节点、描述 |
| 日志模型 | ✅ | 任务级 stdout/stderr 日志 |
| 运行历史模型 | ✅ | 启动时间、结束时间、退出码 |
| 评论模型 | ✅ | 审核人、内容、类型 |
| 任务依赖模型 | ✅ | 任务间依赖关系 |
