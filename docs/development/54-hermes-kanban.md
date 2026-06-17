# M54: Hermes Kanban 适配 — 产品需求

> **版本**：2026-05-24  
> **读者**：产品、全栈开发、运维  
> **外部参考**：[Hermes Agent Kanban](https://hermes-agent.nousresearch.com/docs/user-guide/features/kanban) · `gateway` + `hermes_cli/kanban_db.py` + `plugins/kanban/dashboard/`  
> **关联**：[36 graph-workflow.md](./36%20graph-workflow.md) · [53 team-graph-orchestration.md](./53%20team-graph-orchestration.md) · [frontend-pages.md](./frontend-pages.md)  
> **技术设计**：[54-hermes-kanban.design.md](./54-hermes-kanban.design.md)  
> **开发计划**：[54-hermes-kanban-development.md](./54-hermes-kanban-development.md)  
> **历史**：[devlog/2026-05-16-Graph.md](../devlog/2026-05-16-Graph.md)（Hermes 理念首次引入 M36）

---

## 1. 背景

### 1.1 Hermes Kanban 是什么

Hermes Kanban 是 **持久化多 Agent 工作队列**，与 `delegate_task`（临时 fork-join）相对：

- 任务跨进程、跨重启、可人工介入、可审计
- 独立 SQLite `kanban.db`（可多 Board）
- **三套入口**：`kanban_*` Agent 工具 · `hermes kanban` CLI · Dashboard `/kanban` 插件页
- **Dispatcher** 嵌入 Gateway（默认 60s tick）：reclaim → promote → claim → **spawn worker**（`hermes -p <profile> chat -q "work kanban task <id>"`）

### 1.2 Aranea 为何适配而非克隆

Aranea 已在 **M36 Graph Task** 实现任务模型、RPC、Ent 持久化与 **GraphTaskKanban** UI；在 **M53** 实现 **OrchestrationKanban**（Agent 收/做/交）。**Phase 0–3（2026-05-23）已落地**运行时闭环与基础 UI。

**本模块目标**：在 Aranea 架构内（**GraphExecution 作用域**、Kratos + tRPC Agent）实现 Hermes 同等 **运维与协作效果**，不克隆独立 `kanban.db` 产品与 Hermes CLI。

---

## 2. 核心映射

| Hermes 概念 | Aranea 落地 | 说明 |
|-------------|-------------|------|
| Board（kanban.db） | **GraphExecution** | 一次 Graph Run = 一块板；无全局 Board 切换 |
| 9 列状态 | **5 列 Task 看板** | 合并 triage/todo/ready → 待处理 |
| `ready → running` | `pending → claimed` | Dispatcher + ClaimTask |
| Dispatcher spawn worker | **TaskDispatcher** + RunGateway（G14 待补） | 当前仅记录 TaskRun，未真 spawn（见 [development §6 Phase 4](./54-hermes-kanban-development.md#6-phase-4--worker-真派工p1)） |
| `kanban_*` tools | `internal/tools/kanban` | 9 工具已注册 |
| Worker Skill | Agent toolset `kanban` + Graph 节点 Agent | 非独立 profile spawn |
| 收/做/交 Agent 行 | **OrchestrationKanban**（M53） | 卡片内三 zone，非独立列 |
| Dashboard `/kanban` | Graph Run Inspector / Observatory **任务 Tab** | 无侧栏一级菜单 |
| 多 Board | 多个 Graph 定义 + 多次 Execute | Router `execution_id` 作用域 |
| task_links 依赖 | `graph_task_links` | 后端已实现；Drawer 依赖 Tab 待补（见 [development §7.1 HK-FE-05](./54-hermes-kanban-development.md#71-p1--详情与依赖)） |

---

## 3. Hermes Dashboard UI 规格（对照源）

> 源码：`hermes-agent/plugins/kanban/dashboard/dist/index.js` + `plugin_api.py`

### 3.1 页面结构

| 区域 | Hermes 组件 | 能力 |
|------|-------------|------|
| 顶栏 | `BoardSwitcher` | 多 Board 下拉；localStorage 记忆；`+ New board` |
| 工具栏 | `BoardToolbar` | 搜索；tenant/assignee 筛选；显示 archived；Running 列 **Lanes by profile**；Nudge dispatcher；Auto/Manual orchestration |
| 批量 | `BulkActionBar` | 多选 → 批量 status / archive / reassign / priority |
| 看板 | `BoardColumns` × 9 列 | triage · todo · scheduled · ready · running · blocked · review · done（+ archived） |
| 卡片 | `TaskCard` | id · priority · tenant · 子任务进度 · assignee · 评论/链接数 · 诊断徽章 · staleness |
| 列内创建 | `InlineCreate` | 列头 `+` → POST 创建 |
| 拖拽 | HTML5 DnD + touch | 跨列 PATCH status；**不可拖入 running**（须 Dispatcher claim） |
| 详情 | `TaskDrawer` | 标题/assignee/priority/body 内联编辑；依赖编辑器；评论；事件；runs；Specify/Decompose |
| 诊断 | `AttentionStrip` / `DiagnosticsSection` | 僵尸 worker · stale claim · 循环依赖 |
| 实时 | WebSocket `/events?board=` | task_events → 250ms debounce reload |

### 3.2 Hermes 列与状态（完整）

`triage | todo | scheduled | ready | running | blocked | review | done | archived`

典型流转：`create → triage|todo → (parents done) → ready → (dispatcher claim) → running → complete → done`；可选 `review` 列走 SDLC review agent。

### 3.3 Hermes 卡片操作

| 操作 | 入口 | 约束 |
|------|------|------|
| 认领 running | 仅 Dispatcher | 人工不可拖入 running |
| 完成 | Worker `kanban_complete` / Drawer | running/ready/blocked → done |
| 阻塞 | `kanban_block` | → blocked |
| 解阻塞 | Orchestrator `kanban_unblock` / Drawer | → ready/todo |
| 依赖 | `kanban_link` / DependencyEditor | parent complete → child promote |
| 评论 | `kanban_comment` | 持久化协作协议 |
| 心跳 | `kanban_heartbeat` | 延长 claim TTL |
| 删除 | TrashDropZone | DELETE task |

---

## 4. Aranea UI 规格（当前 + 目标）

> **组件树、文件路径、拖拽 Admin 语义、Kanban↔Graph 画布联动等设计细节**详见 [54-hermes-kanban.design.md §8](./54-hermes-kanban.design.md#8-前端架构)。

### 4.1 入口与路由

| 入口 | 路由 / 位置 |
|------|-------------|
| Graph Run 观测 | `/graphs/:id/run/:execId` → Inspector「任务」 |
| Team Observatory | `/teams/:teamId/runs/:runId/observatory` →「任务看板」 |
| Agent 工作看板 | 同页「Agent 工作看板」Tab（M53） |
| **独立 Kanban 页** | 无（Hermes `/kanban`；Phase 5 可选） |

### 4.2 任务看板布局（用户视角）

| 区域 | Aranea 体验 | Hermes 对照 |
|------|-------------|-------------|
| 列 | **5 列**：待处理 / 执行中 / 待审核 / 已完成 / 异常 | 9 列；scheduled/review 独立 |
| 实时 | 「实时」badge 提示 WS 推送 | task_events WS |
| 卡片字段 | nodeId · role · status chip · assignee · summary/input 预览 | id · priority · tenant · 诊断 · 子任务进度 |
| 空状态 | 引导「Agent 节点激活自动建任务」 | Inline create + 教程 |
| 拖拽 | 列间拖拽（受限语义） | 全列 DnD |
| 拖拽语义 | blocked→待处理 **unblock**；待审核→已完成 **approve** | 任意列 PATCH（除 running） |
| 刷新 | 手动 refresh 按钮 | WS 自动 + debounce reload |

### 4.3 任务详情抽屉（用户视角）

| Tab | Aranea 体验 | Hermes TaskDrawer |
|-----|--------|-------------------|
| 详情 | claim / submit / block / unblock / review；Agent Key；输入只读 | 全字段 inline edit + StatusActions |
| 评论 | 列表 + 添加 | 同等 |
| 事件 | 任务事件时间线 | task_events 时间线 |
| 日志 | 任务日志 tail | WorkerLogSection tail |
| 运行 | 任务运行历史 | RunHistorySection |
| **依赖** | 未实现（Phase 5 HK-FE-05） | DependencyEditor parent/child chips |
| Specify/Decompose | 未实现（Phase 5 HK-ORCH-01） | triage → LLM 规格化 / 分解 |

### 4.4 Agent 工作看板（用户视角）

| 维度 | Aranea 体验 | Hermes |
|------|--------|--------|
| 布局 | 纵向列表 + 状态筛选 | Running 列 **Lanes by profile** |
| 卡片 | 收到 / 进行中 / 已交付 三 zone | 按 profile 泳道 |
| 联动 | 选 Agent 行 → Graph 节点 focus | 无 Graph 画布 |
| 数据源 | Orchestration WS | task board 独立 |

### 4.5 视觉规范

> 玻璃材质 Dialog、列壳复用、与 Graph Run 画布同页等 UX 规范详见 [54-hermes-kanban.design.md §8](./54-hermes-kanban.design.md#8-前端架构)。

---

## 5. 用户故事

### 已实现（Phase 0–3）

| ID | 故事 | 验收 |
|----|------|------|
| US-HK-01 | Graph Run 中 Agent 节点激活自动建任务 | `graph_task_status` WS；Inspector 任务 Tab |
| US-HK-02 | Worker 通过 `kanban_*` 驱动生命周期 | Tool → 看板列同步 |
| US-HK-03 | 任务 complete 驱动 Graph 继续 | Submit/Review → ResumeGraph |
| US-HK-04 | block / unblock | RPC + Drawer + `kanban_unblock` |
| US-HK-05 | `kanban_create` + `kanban_link` 依赖 | promote on parent complete |
| US-HK-06 | 双 Kanban 观测 | Observatory 任务 + Agent 两 Tab |

### 待实现（Phase 4–5）

| ID | 故事 | 优先级 | Hermes 参照 |
|----|------|--------|-------------|
| US-HK-07 | Dispatcher **真 spawn** Worker Agent Run | P1 | `_default_spawn` + Gateway watcher |
| US-HK-08 | Drawer **依赖 Tab**（link/unlink 可视化） | P1 | DependencyEditor |
| US-HK-09 | 看板 **Toolbar**：搜索 / assignee 筛选 / 批量选择 | P2 | BoardToolbar |
| US-HK-10 | 列内 **Inline Create** 任务 | P2 | InlineCreate |
| US-HK-11 | 拖拽 **Reassign**（改 assignee） | P2 | Bulk reassign |
| US-HK-12 | **Diagnostics** 条（超时/僵尸/无 assignee） | P2 | AttentionStrip |
| US-HK-13 | Orchestrator **triage/decompose**（可选 LLM） | P3 | specify/decompose API |
| US-HK-14 | 独立 **/kanban** 或 Graph 列表快捷入口 | P3 | Dashboard 一级 Tab |
| US-HK-15 | Running 列 **Agent 泳道** 分组 | P3 | Lanes by profile |

---

## 6. 模块边界

| 在范围 | 不在范围（v1） |
|--------|----------------|
| Graph Execution 任务板 + 双 Kanban 观测 | 全局独立 `~/.aranea/kanban.db` |
| kanban_* 工具 + Task RPC + Dispatcher | Hermes CLI `hermes kanban` |
| TaskDispatcher + 心跳超时 + G14 spawn | 完整 Hermes 9 列产品（合并为 5 列） |
| M53 Orchestration 联动 | 跨 Execution 任务 link |
| Webhook `graph.task.status` | Hermes multi-board localStorage |

---

## 7. 验收索引

| ID | 摘要 | 阶段 |
|----|------|------|
| HK-RT-01 | 节点 CreateTask + WS | P1 |
| HK-RT-02 | TaskDispatcher tick | P1 |
| HK-RT-03 | complete → ResumeGraph | P1 |
| HK-TOOLS-01 | kanban_* 工具集 | P1 |
| HK-DEP-01 | graph_task_links + promote | P2 |
| HK-OBS-01 | Activity 时间线 | P2 |
| HK-FE-03 | 列拖拽 unblock/approve | P3 |
| HK-INT-01 | Task status Webhook | P2 |
| HK-INT-02 | spawn_fn worker lane | P2 |
| HK-FE-05 | Drawer 依赖 Tab | P1 |
| HK-FE-06 | Toolbar 搜索/筛选 | P2 |
| HK-ORCH-01 | triage/decompose | P3 |

> 各任务的实施状态（✅/⏳/📋）详见 [开发计划](./54-hermes-kanban-development.md)。

---

## 8. 文档修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| 1.0 | 2026-05-23 | 初版：映射、6 用户故事、UI 要点 |
| 1.1 | 2026-05-24 | Hermes Dashboard UI 全量对照；Aranea 入口/组件规格；Phase 4–5 用户故事 |
| 1.2 | 2026-06-17 | 三件套内容边界整理：§4 移除组件名/文件路径（迁移至 design §8）；§5/§7 移除状态标记（迁移至 development）；保留用户视角交互规格 |
