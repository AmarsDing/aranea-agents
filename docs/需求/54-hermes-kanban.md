# M54: Hermes Kanban 适配 — 产品需求

> **版本**：2026-05-23  
> **读者**：产品、全栈开发、运维  
> **外部参考**：[Hermes Agent Kanban](https://hermes-agent.nousresearch.com/docs/user-guide/features/kanban)  
> **关联**：[36 graph-workflow.md](./36%20graph-workflow.md) · [53 team-graph-orchestration.md](./53%20team-graph-orchestration.md) · [frontend-pages.md](./frontend-pages.md)  
> **技术设计**：[54-hermes-kanban.design.md](./54-hermes-kanban.design.md)  
> **开发计划**：[54-hermes-kanban-development.md](./54-hermes-kanban-development.md)  
> **历史**：[devlog/2026-05-16-Graph.md](../devlog/2026-05-16-Graph.md)（Hermes 理念首次引入 M36）

---

## 1. 背景

Hermes Kanban 是 **持久化多 Agent 工作队列**，与 `delegate_task`（fork-join RPC）相对：任务跨进程、跨重启、可人工介入、可审计。

Aranea 已在 **M36 Graph Task** 实现任务模型、RPC、Ent 持久化与 **GraphTaskKanban** UI；在 **M53** 实现 **OrchestrationKanban**（Agent 收/做/交）。缺口在于 **运行时自动建任务、Dispatcher 派工、kanban_* Agent 工具、依赖图与交互式看板**。

**本模块目标**：在 Aranea 架构内（GraphExecution 作用域、Kratos + tRPC Agent）实现 Hermes 同等 **运维与协作效果**，不克隆 Hermes 独立 `kanban.db` 产品。

---

## 2. 核心映射

| Hermes | Aranea |
|--------|--------|
| Board（kanban.db） | **GraphExecution**（一次 Run 一块板） |
| `ready → running` | `pending → claimed` |
| Dispatcher spawn worker | **TaskDispatcher** + RunGateway / Agent Run |
| `kanban_*` tools | **`internal/tools/kanban`** → Task RPC |
| 收/做/交 Agent 行 | **OrchestrationKanban**（M53） |
| 任务状态列 | **GraphTaskKanban**（M36） |

---

## 3. 用户故事

### US-HK-01 运行 Graph 时自动出现任务卡片

**作为** 运维者  
**我希望** Graph Run 中 Agent 节点激活时自动创建任务并出现在任务看板  
**以便** 无需手工登记即可跟踪节点级工作项  

**验收**：`graph_node_start` 后 WS `graph_task_status` 推送；Inspector「任务」Tab 可见卡片。

### US-HK-02 Worker 通过 Tool 完成/阻塞任务

**作为** 执行 Agent  
**我希望** 使用 `kanban_show` / `kanban_complete` / `kanban_block` 等工具驱动任务生命周期  
**以便** 与 Hermes worker 协议一致，无需直接调 HTTP  

**验收**：Tool 调用后任务状态与看板列同步迁移。

### US-HK-03 任务完成驱动 Graph 继续

**作为** 工作流设计者  
**我希望** 任务 `complete`（含审核通过）后 Graph 从等待态恢复  
**以便** 人机协同节点与 Task 门禁串联  

**验收**：Submit/Review 通过后 `ResumeGraph` 或节点继续执行。

### US-HK-04 阻塞与解除阻塞

**作为** 管理员  
**我希望** Agent 可 `kanban_block`，我可在 Drawer 或 Tool 中 `unblock`  
**以便** 人工介入后任务重新进入待领取队列  

**验收**：`UnblockTask` RPC + Drawer 按钮 + `kanban_unblock` tool。

### US-HK-05 编排者分解任务图（P2）

**作为** Orchestrator Agent  
**我希望** `kanban_create` + `kanban_link` 建立依赖，父任务完成后子任务自动 ready  
**以便** 实现 Hermes 式 fan-out / join  

**验收**：`graph_task_links` + promote 逻辑 + E2E 两并行 + 一汇总。

### US-HK-06 双 Kanban 一致观测

**作为** 观察者  
**我希望** Task 看板与 Agent 工作看板状态一致  
**以便** 同时回答「有哪些任务」与「每个 Agent 在做什么」  

**验收**：`graph_task_status` 投影到 OrchestrationKanban；Activity 时间线（P2）。

---

## 4. UI 规格（Hermes Dashboard 对齐要点）

| 区域 | Hermes | Aranea 落地 |
|------|--------|-------------|
| 列布局 | triage/todo/ready/running/blocked/done | GraphTaskKanban 五列（待处理/执行中/待审核/已完成/异常） |
| 实时 | task_events WebSocket | `graph_task_status` WS |
| 卡片 | 标题、assignee、priority | nodeId、assignee、status chip |
| 侧栏 | 详情/评论/事件/依赖 | GraphTaskDetailDrawer 多 Tab |
| 拖拽 | 列间 PATCH status | Phase 3 `vuedraggable` + Unblock/Reassign |
| Agent 行 | Lanes by profile | OrchestrationKanban 三列收/做/交 |

技术栈：Vue 3 + Quasar + Pinia；样式见 [UX.md](../frontend/UX.md) 与 `WorkflowKanbanBoard`。

---

## 5. 模块边界

| 在范围 | 不在范围（v1） |
|--------|----------------|
| Graph Execution 任务板 | 全局独立 `~/.aranea/kanban.db` |
| kanban_* 工具 + Task RPC | Hermes CLI `/kanban` 命令 |
| TaskDispatcher + 心跳超时 | 完整 triage/decompose LLM（P3 可选） |
| M53 Orchestration 联动 | 跨 Execution 任务 link |

---

## 6. 验收索引

| ID | 摘要 | 阶段 |
|----|------|------|
| HK-RT-01 | 节点进入 CreateTask + WS | P1 |
| HK-RT-02 | TaskDispatcher | P1 |
| HK-RT-03 | complete → ResumeGraph | P1 |
| HK-TOOLS-01 | kanban_* 工具集 | P1 |
| HK-DEP-01 | 任务依赖 link | P2 |
| HK-OBS-01 | Activity 时间线 | P2 |
| HK-FE-03 | 列拖拽 | P3 |

完整任务板见 [开发计划](./54-hermes-kanban-development.md)。
