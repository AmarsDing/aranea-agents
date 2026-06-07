# M53: Team × Graph 编排融合 — 产品需求

> **版本**：2026-06-06
> **读者**：产品、全栈开发、运维
> **关联**：[11 multi-agent.md](./11%20multi-agent.md) · [36 graph-workflow.md](./36%20graph-workflow.md) · [51 消息机制.md](./51%20消息机制.md) · [0 系统框图.md](./0%20系统框图.md)
> **技术设计**：[53 team-graph-orchestration.design.md](./53%20team-graph-orchestration.design.md)
> **开发计划**：[53-team-graph-orchestration.development.md](./53-team-graph-orchestration-development.md)

---

## 1. 背景与问题

当前 **Team（M11）** 与 **Graph（M36）** 是两条平行轨道：

| 现象 | 用户影响 |
|------|----------|
| Team 编辑器用表单 + 静态拓扑预览，Graph 用 Vue Flow 真画布 | 同一协作关系需在两处理解，认知成本高 |
| Team `definition.graph` 仅前端生成，不参与后端运行 | 用户以为改了联动关系，运行时仍按 mode 硬编码 |
| Team 观测靠 `TeamRunStep` 列表，Graph 靠节点高亮 | 无法统一回答「每个 Agent 收到什么、做了什么、交出了什么」 |
| Graph 能力（Checkpoint、HITL、Retry、Task）与 Team 运行时未打通 | 复杂协作无法容错与人工介入 |

**目标**：以 **OrchestrationSpec（编排规格）** 为统一真相源，Team 提供模式化简视图，Graph 提供自由拓扑视图；运行态统一观测 Agent 状态与 Kanban 工作阶段。**终态**：GraphAgent 为唯一执行引擎，Team 是 OrchestrationSpec 的编辑视图。

**当前进度**：Phase 0.5–4 ✅ | Phase 5 大部分 ✅ | Phase 6–7 ✅ | Phase 8.1–8.8 ✅ | Phase 8.9 待实施

---

## 2. 用户故事

### US-01 创建 Team 后自动关联 Agent 联动

**作为** 团队管理员  
**我希望** 选择编排模式（sequential / parallel / coordinator 等）后，系统自动生成 Agent 节点与边  
**以便** 我不必从零画拓扑，且成员与连线一一对应  

**验收**：

- 选模式并添加成员后，编排画布出现对应节点与边（与 mode 语义一致）
- 用户可拖拽改边、增删条件路由（advanced）
- 保存后 `definition.graph` 持久化，与成员列表一致

### US-02 运行中禁止修改、仅观测

**作为** 运维/管理员  
**我希望** Team Run 进行中时编排定义只读  
**以便** 观测与回放对应同一份拓扑快照  

**验收**：

- 存在 `running` 的 TeamRun 时，Team PATCH 与编排画布编辑禁用
- Run 开始时冻结 `definition_snapshot_json`
- Run 结束后可编辑（新版本），历史 Run 仍按快照回放

### US-03 Graph 与 Kanban 中看见 Agent 当前状态

**作为** 观察者  
**我希望** 在 Graph 画布与 Kanban 看板上看到每个 Agent 的实时状态  
**以便** 知道谁在跑、谁在等、谁失败了  

**验收**：

- Graph 节点显示聚合状态 badge + 细态 subtitle（如「工具执行中」）
- Kanban 每 Agent 一行，行首 status chip 与 Graph 一致
- 点击 Kanban 行 ↔ Graph 节点联动 focus
- 状态从 WS `orchestration_agent_status` 增量更新，延迟可感知 < 1s（局域网）

### US-04 收 / 做 / 交 工作看板

**作为** 观察者  
**我希望** 每个 Agent 有三个工作阶段列：收到、进行中、已交付  
**以便** 追踪输入、活动与产出  

**验收**：

- 「收到」展示 input_preview / 上游 state 摘要
- 「进行中」展示 Activity 时间线（tool / skill / mcp / subagent）
- 「已交付」展示 output_preview / artifact 引用
- 工作阶段与生命周期状态正交（状态用 chip，阶段用列）
- Graph Run **任务看板**（M54 `GraphTaskKanban`）与 Agent 工作看板共用 `graph_task_status` 投影

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

- Checkpoint：长时间 Team Run 进程重启后可恢复（Phase 6 ✅）
- HITL：节点配置 `interrupt_before/after` 后运行时暂停，Observatory 可审核恢复（Phase 6 ✅）
- 子图：Team 嵌套子图 / Router 节点，编译期循环检测（Phase 7 ✅）
- Task / review 节点进入 Team 编译（Phase 7 ✅）

### US-07 Activity 时间线与历史

**作为** 观察者
**我希望** 在 Kanban 和 Observatory 中看到每个 Agent 的完整 Activity 历史（而非仅当前活动）
**以便** 追踪工具调用链和执行细节

**验收**：

- StatusProjector 维护 `activity_history[]`（上限 20 条），前端 Kanban 「进行中」列展示时间轴
- Observatory Timeline Tab 展示持久化的 Activity 记录，支持按节点过滤
- Activity 持久化到 `orchestration_steps` 表，Run 结束后可从 DB 重建

### US-08 运行时引擎选择与收敛

**作为** 管理员
**我希望** 可选择 Team 运行时引擎（Graph / Native），且新建 Team 默认走 Graph
**以便** 在生产验证后逐步收敛到统一执行路径

**验收**：

- 前端编排面板可切换 `runtime_engine`（graph / native），保存不丢字段
- 新 Team 默认 `runtime_engine=graph`；旧 Team 不受影响
- `ARANEA_TEAM_GRAPH_RUNTIME` 默认开启；`ARANEA_TEAM_NATIVE=1` 仅作紧急熔断
- Native vs Graph parity E2E 对比通过，差异文档化

### US-09 容错完整化

**作为** 管理员
**我希望** Agent 失败时有完整的容错策略（retry / skip / fallback / 熔断 / 人工接管）
**以便** 关键任务不因单点失败而中断

**验收**：

- FailurePolicy 支持 retry（含 backoff）、skip、fallback_agent、parallel_fail continue（Phase 4 ✅）
- 节点失败时前端展示失败 banner，提供重试 / 切 fallback / 审核 / 终止操作
- Circuit Breaker：连续失败达阈值后节点冻结 + WS alert（类型预留，实现待 Phase 8.9+）
- 死信：失败超过 retry 且 policy=halt 的 Job 写入 `task_dead_letters`（待实施）

### US-10 编排规格产品化

**作为** 团队管理员
**我希望** 前端完整暴露 OrchestrationSpec 的所有字段（runtime_engine / failure_policy / embedded graph / linked_graph_id）
**以便** 不需要手动编辑 JSON 即可配置高级选项

**验收**：

- 前端 `parseDefinition` 保留未知字段（raw merge），不丢 `runtime_engine` / `failure_policy`
- 编排面板可编辑 failure_policy、runtime_engine
- Graph 编辑器属性面板支持 RetryPolicy / Destinations / Mapper 编辑
- OrchestrationSpec v2 类型前后端对齐

---

## 3. 功能规格

### 3.1 编排来源（Orchestration Source）

| 值 | 含义 |
|----|------|
| `preset` | 由 Team mode + members 编译生成 graph，用户可微调 |
| `linked_graph` | 引用 `graphs` 表独立资产 |
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

- TeamRun `status=running` → 编排只读
- GraphExecution 进行中 → 关联 Graph 定义只读
- 快照字段：`TeamRun.definition_snapshot_json`

### 3.5 观测通道

- WS：`orchestration_agent_status`（节点状态增量）
- REST（规划）：`GET /v1/team-runs/{id}/observatory`
- 与现有 `member_*`、`graph_node_*`、`team_summary` 并存，由 StatusProjector 统一投影

---

## 4. 非功能需求

| 项 | 要求 |
|----|------|
| 架构 | `internal/biz` 不 import trpc；状态归约在 biz，投影在 team/service |
| 性能 | Observatory 首屏 30 节点 < 500ms |
| 兼容 | Phase 0.5–4 已完成；Phase 5+ 扩大 Graph 执行占比，Native 保留至 Phase 8 为安全网 |
| 前端 | 遵循 `aranea-frontend-guide` SKILL §6 token；复用 `GraphFlowNode` |
| 执行单链 | GraphAgent 为唯一执行引擎；`ARANEA_TEAM_NATIVE=1` 仅紧急熔断 |

---

## 5. 模块边界

| 模块 | 本需求中的职责 |
|------|----------------|
| Team (11) | 模式化编排、成员、TeamRun |
| Graph (36) | 自由拓扑、GraphAgent 执行、Checkpoint |
| Message (51) | Envelope 传输 |
| Monitor (18) | 可选订阅 orchestration 事件 |
| Channel (17) | `async_graph_id` 与统一 Graph 路径对齐（Phase 4） |

**不在范围**：Agent 目录 CRUD、单 Agent Chat 非 Team 路径的 Kanban（后续可复用投影器）。

---

## 6. 验收标准索引

| ID | 摘要 | 阶段 | 状态 |
|----|------|------|------|
| OBS-01 | Graph Agent 节点聚合 + 细态 badge | Phase 1 | ✅ |
| OBS-02 | Kanban 三列 + status chip | Phase 1 | ✅ |
| OBS-03 | Kanban ↔ Graph focus 联动 | Phase 1 | ✅ |
| OBS-04 | WS 实时状态切换 | Phase 0.5 | ✅ |
| OBS-05 | retry / skipped 视觉 | Phase 2 | ✅ |
| OBS-06 | Run 结束状态冻结可回放 | Phase 1 | ✅ |
| TG-01 | mode → graph 编译器 | Phase 2 | ✅ |
| TG-02 | TeamRun 绑定 graph_execution | Phase 3 | ✅ |
| FP-01 | FailurePolicy | Phase 4 | ✅ |
| RT-01 | Native vs Graph parity E2E | Phase 5 | ✅ |
| RT-02 | runtime_engine 前端编辑 + raw merge | Phase 5 | ✅ |
| RT-03 | GraphEditorCanvas readonly | Phase 5 | ✅ |
| RT-04 | 运行时指标 + Canary | Phase 5 | ✅ |
| HIST-01 | Activity 时间线 + 持久化 | Phase 5 | ⏳ |
| CMP-V2 | OrchestrationSpec v2 类型对齐 | Phase 6 | ⏳ |
| CHK-01 | Team Graph Run Checkpoint | Phase 6 | ⏳ |
| HITL-01 | Team Graph Run InterruptBefore/After | Phase 6 | ⏳ |
| FP-02 | Circuit Breaker | Phase 6b | 📋 |
| FP-03 | HITL 错误接管 | Phase 6b | 📋 |
| FP-04 | 死信表 | Phase 6b | 📋 |
| G-RETRY | RetryPolicy 属性面板 | Phase 6c | ✅ |
| G-GOTO | Destinations 属性面板 | Phase 6c | ✅ |
| G-AGENT-MAP | Mapper 属性面板 | Phase 6c | ✅ |
| RETIRE | 移除 Native 主路径 | Phase 7 | ✅ |
| TASK-01 | Task / review 节点编译 | Phase 7 | ✅ |
| SUB-01 | 子图 / Router 节点 | Phase 7 | ✅ |
| BL-01~07 | 架构优化（状态机/协议化/单轨化/模板/配置化/错误规范化） | Phase 8 | ✅ |
| BL-05 | Step 持久化事件驱动统一 | Phase 8.9 | 📋 |
| BL-09 | Observer 单订阅化 | Phase 8.9 | 📋 |

完整任务拆分见 [开发计划](./53-team-graph-orchestration-development.md)。
