# M53: Team × Graph 编排融合 — 产品需求

> **版本**：2026-06-06
> **读者**：产品、全栈开发、运维
> **关联**：[11-multi-agent.md](./11-multi-agent.md) · [36-graph-workflow.md](./36-graph-workflow.md) · [51-message-mechanism.md](./51-message-mechanism.md) · [0-system-diagram.md](./0-system-diagram.md)
> **技术设计**：[53-team-graph-orchestration.design.md](./53-team-graph-orchestration.design.md)
> **开发计划**：[53-team-graph-orchestration.development.md](./53-team-graph-orchestration.development.md)

---

## 1. 背景与问题

当前 **Team（M11）** 与 **Graph（M36）** 是两条平行轨道：

| 现象 | 用户影响 |
|------|----------|
| Team 编辑器用表单 + 静态拓扑预览，Graph 用 Vue Flow 真画布 | 同一协作关系需在两处理解，认知成本高 |
| Team 编排拓扑仅前端生成，不参与后端运行 | 用户以为改了联动关系，运行时仍按 mode 硬编码 |
| Team 观测靠 TeamRunStep 列表，Graph 靠节点高亮 | 无法统一回答「每个 Agent 收到什么、做了什么、交出了什么」 |
| Graph 能力（Checkpoint、HITL、Retry、Task）与 Team 运行时未打通 | 复杂协作无法容错与人工介入 |

**目标**：以 **OrchestrationSpec（编排规格）** 为统一真相源，Team 提供模式化简视图，Graph 提供自由拓扑视图；运行态统一观测 Agent 状态与 Kanban 工作阶段。**终态**：GraphAgent 为唯一执行引擎，Team 是 OrchestrationSpec 的编辑视图。

> 详见 [53-team-graph-orchestration.development.md §2 现状评估](./53-team-graph-orchestration.development.md#2-现状评估2026-06-06)

---

## 2. 用户故事

### US-01 创建 Team 后自动关联 Agent 联动

**作为** 团队管理员  
**我希望** 选择编排模式（sequential / parallel / coordinator 等）后，系统自动生成 Agent 节点与边  
**以便** 我不必从零画拓扑，且成员与连线一一对应  

**验收**：

- 选模式并添加成员后，编排画布出现对应节点与边（与 mode 语义一致）
- 用户可拖拽改边、增删条件路由（advanced）
- 保存后编排画布拓扑持久化，与成员列表一致

### US-02 运行中禁止修改、仅观测

**作为** 运维/管理员  
**我希望** Team Run 进行中时编排定义只读  
**以便** 观测与回放对应同一份拓扑快照  

**验收**：

- 存在运行中的 TeamRun 时，Team 编辑与编排画布编辑禁用
- Run 开始时冻结编排规格快照
- Run 结束后可编辑（新版本），历史 Run 仍按快照回放

### US-03 Graph 与 Kanban 中看见 Agent 当前状态

**作为** 观察者  
**我希望** 在 Graph 画布与 Kanban 看板上看到每个 Agent 的实时状态  
**以便** 知道谁在跑、谁在等、谁失败了  

**验收**：

- Graph 节点显示聚合状态 badge + 细态 subtitle（如「工具执行中」）
- Kanban 每 Agent 一行，行首 status chip 与 Graph 一致
- 点击 Kanban 行 ↔ Graph 节点联动 focus
- 状态从 WS 增量更新，延迟可感知 < 1s（局域网）

### US-04 收 / 做 / 交 工作看板

**作为** 观察者
**我希望** 每个 Agent 有三个工作阶段列：收到、进行中、已交付
**以便** 追踪输入、活动与产出

**验收**：

- 「收到」展示 input_preview / 上游 state 摘要
- 「进行中」展示 Activity 时间线（tool / skill / mcp / subagent）
- 「已交付」展示 output_preview / artifact 引用
- 工作阶段与生命周期状态正交（状态用 chip，阶段用列）
- Graph Run 任务看板（M54 `GraphTaskKanban`）与 Agent 工作看板共用任务状态投影

### US-05 Agent 失败时的任务保障

**作为** 管理员  
**我希望** 某 Agent 失败时有 retry、备用 Agent、跳过或人工介入策略  
**以便** 整体任务仍可完成  

**验收**：

- 失败节点在 Graph 红色高亮，Kanban 显示 failed / retrying
- 配置 FailurePolicy 后按策略自动 retry 或 fallback
- HITL / Task `review_required` 时全局 banner + 恢复入口
- Graph 中可看到 skip / fallback 边与状态

### US-06 Graph 能力全量接入（长期）

**作为** 高级用户
**我希望** Team 复杂场景可升级为 Graph 执行（Checkpoint、子图、Router）
**以便** 不离开 Team 资产即可使用 Graph 引擎

**验收**：

- Checkpoint：长时间 Team Run 进程重启后可恢复
- HITL：节点配置中断点后运行时暂停，Observatory 可审核恢复
- 子图：Team 嵌套子图 / Router 节点，编译期循环检测
- Task / review 节点进入 Team 编译

### US-07 Activity 时间线与历史

**作为** 观察者
**我希望** 在 Kanban 和 Observatory 中看到每个 Agent 的完整 Activity 历史（而非仅当前活动）
**以便** 追踪工具调用链和执行细节

**验收**：

- StatusProjector 维护 Activity 历史记录（上限 20 条），前端 Kanban 「进行中」列展示时间轴
- Observatory Timeline Tab 展示持久化的 Activity 记录，支持按节点过滤
- Activity 持久化到数据库，Run 结束后可从 DB 重建

### US-08 运行时引擎选择与收敛

**作为** 管理员
**我希望** 可选择 Team 运行时引擎（Graph / Native），且新建 Team 默认走 Graph
**以便** 在生产验证后逐步收敛到统一执行路径

**验收**：

- 前端编排面板可切换 `runtime_engine`（graph / native），保存不丢字段
- 新 Team 默认 `runtime_engine=graph`；旧 Team 不受影响
- `ARANEA_TEAM_GRAPH_RUNTIME` 默认开启；关闭后回退到 Native 路径
- Native vs Graph parity E2E 对比通过，差异文档化

### US-09 容错完整化

**作为** 管理员
**我希望** Agent 失败时有完整的容错策略（retry / skip / fallback / 熔断 / 人工接管）
**以便** 关键任务不因单点失败而中断

**验收**：

- FailurePolicy 支持 retry（含 backoff）、skip、fallback_agent、parallel_fail continue
- 节点失败时前端展示失败 banner，提供重试 / 切 fallback / 审核 / 终止操作
- Circuit Breaker：连续失败达阈值后节点冻结 + WS alert
- 死信：失败超过 retry 且 policy=halt 的任务进入失败队列，可在后台任务面板查看

### US-10 编排规格产品化

**作为** 团队管理员
**我希望** 前端完整暴露 OrchestrationSpec 的所有字段（runtime_engine / failure_policy / embedded graph / linked_graph_id）
**以便** 不需要手动编辑 JSON 即可配置高级选项

**验收**：

- 前端解析 Team 定义时保留未知字段，不丢 `runtime_engine` / `failure_policy`
- 编排面板可编辑 failure_policy、runtime_engine
- Graph 编辑器属性面板支持 RetryPolicy / Destinations / Mapper 编辑
- OrchestrationSpec v2 类型前后端对齐

---

## 3. 功能规格

### 3.1 编排来源（Orchestration Source）

| 值 | 含义 |
|----|------|
| `preset` | 由 Team mode + members 编译生成 graph，用户可微调 |
| `linked_graph` | 引用独立 Graph 资产 |
| `custom` | 用户改动超出 preset 表达能力 |

### 3.2 Agent 节点状态（生命周期）

四类、十六细态（UI 聚合为七色块）：

| 类 | 细态 |
|----|------|
| 调度 | `idle`, `queued`, `scheduled` |
| 执行 | `running`, `thinking`, `tool_running`, `transferring`, `retrying` |
| 等待 | `waiting_input`, `waiting_review`, `waiting_assign`, `blocked` |
| 终态 | `success`, `failed`, `skipped`, `cancelled`, `timed_out` |

### 3.3 工作阶段（Kanban 列）

| 列 | 含义 |
|----|------|
| `received` | 节点输入 / 上游交付 |
| `doing` | 当前 Activity 链 |
| `delivered` | 节点输出 / 交付物 |

### 3.4 运行锁定

- TeamRun 进行中时编排定义只读
- GraphExecution 进行中时关联 Graph 定义只读
- Run 开始时冻结编排规格快照，结束后可编辑新版本

> 数据模型与字段定义详见 [53-team-graph-orchestration.design.md §2.3 TeamRun 扩展](./53-team-graph-orchestration.design.md#23-teamrun-扩展)

### 3.5 观测通道

- 实时：WS 推送 Agent 节点状态增量
- 首屏：REST 查询 Observatory（含节点状态、拓扑、Activity 时间线）
- 与现有 Team / Graph / Channel 观测并存，由统一投影器输出

> Envelope 协议与 WS 通道设计详见 [53-team-graph-orchestration.design.md §四 StatusProjector](./53-team-graph-orchestration.design.md#四statusprojector)

---

## 4. 非功能需求

| 项 | 要求 |
|----|------|
| 性能 | Observatory 首屏 30 节点 < 500ms |
| 兼容 | OrchestrationSpec v1/v2 双协议并存；Graph 执行为默认路径 |
| 前端 | 遵循 `aranea-frontend-guide` SKILL §6 token；复用 `GraphFlowNode` |
| 执行单链 | GraphAgent 为唯一执行引擎；Native 路径已移除 |

> 架构分层与依赖红线详见 [53-team-graph-orchestration.design.md §1.2 分层与依赖](./53-team-graph-orchestration.design.md#12-分层与依赖)

---

## 5. 模块边界

| 模块 | 本需求中的职责 |
|------|----------------|
| Team (11) | 模式化编排、成员、TeamRun |
| Graph (36) | 自由拓扑、GraphAgent 执行、Checkpoint |
| Message (51) | Envelope 传输 |
| Monitor (18) | 可选订阅 orchestration 事件 |
| Channel (17) | 异步 Graph 路径与统一 Graph 编译链对齐 |

**不在范围**：Agent 目录 CRUD、单 Agent Chat 非 Team 路径的 Kanban（后续可复用投影器）。

---

## 6. 验收标准索引

> 验收标准索引（含 Phase 阶段与实施状态）已迁移至开发计划文档。
> 详见 [53-team-graph-orchestration.development.md §4 任务板](./53-team-graph-orchestration.development.md#4-任务板当前冲刺)
