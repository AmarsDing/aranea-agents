# M54: Hermes Kanban 适配 — 开发计划

> **版本**：2026-05-24 | **状态**：✅ Phase 0–3 已落地；Phase 4–5 进行中  
> **需求**：[54-hermes-kanban.md](./54-hermes-kanban.md) · **设计**：[54-hermes-kanban.design.md](./54-hermes-kanban.design.md)  
> **进度**：[execution-plan.md](../guides/execution-plan.md) EP-HK-01  
> **Hermes 参考**：`hermes-agent/plugins/kanban/` · `hermes_cli/kanban_db.py` · [Kanban 文档](https://hermes-agent.nousresearch.com/docs/user-guide/features/kanban)

---

## 1. 代码锚点

| 层级 | 路径 |
|------|------|
| Task 领域 | `internal/biz/task.go`, `task_dispatch.go`, `task_dispatcher.go`, `task_links.go`, `graph_task_coordinator.go` |
| Graph 挂钩 | `internal/biz/graph_execution.go`, `internal/service/graph_task_runtime.go` |
| WS / Webhook | `internal/service/graph_task_status.go` |
| Tools | `internal/tools/kanban/tools.go`, `bridge.go`, `env.go` |
| Service bridge | `internal/service/kanban_bridge.go` |
| 前端看板 | `web/src/components/graph/GraphTaskKanban*.vue` |
| 前端列/投影 | `web/src/features/graph/tasks/kanbanColumns.ts`, `taskStreamProjection.ts` |
| 前端编排 | `web/src/features/graph/useGraphRunTasks.ts` |
| Agent 看板 | `web/src/components/orchestration/OrchestrationKanban*.vue` |
| 壳组件 | `web/src/components/workflow/WorkflowKanbanBoard.vue` |
| 详情抽屉 | `web/src/components/graph/GraphTaskDetailDrawer.vue` |
| 观测入口 | `web/src/pages/TeamRunObservatoryPage.vue`, `GraphRunInspector.vue` |
| Proto | `api/kratos/graph/v1/graph.proto` |

**Hermes 对照文件**（UI/行为移植时阅读）：

| 主题 | Hermes |
|------|--------|
| Dashboard UI | `plugins/kanban/dashboard/dist/index.js` |
| REST API | `plugins/kanban/dashboard/plugin_api.py` |
| DB + Dispatcher | `hermes_cli/kanban_db.py` |
| CLI | `hermes_cli/kanban.py` |
| Agent tools | `tools/kanban_tools.py` |
| Gateway tick | `gateway/run.py` `_kanban_dispatcher_watcher` |

---

## 2. Phase 0 — 文档 ✅

- M54 需求/设计/开发计划
- 交叉引用 M36/M53/frontend-pages/execution-plan

---

## 3. Phase 1 — 运行时 + Tools（P1）✅

| ID | 任务 | 状态 |
|----|------|------|
| HK-RT-01 | GraphTaskCoordinator + node start CreateTask | ✅ |
| HK-RT-02 | TaskDispatcher background tick | ✅ |
| HK-RT-03 | OnTaskCompleted → ResumeExecution | ✅ |
| HK-RT-04 | UnblockTask RPC | ✅ |
| HK-RT-05 | CheckTimeouts runner | ✅ |
| HK-TOOLS-01 | kanban toolset（9 工具） | ✅ |
| HK-TOOLS-02 | builtin seed + toolset prune | ✅ |
| HK-FE-01 | Drawer unblock + 空板引导 | ✅ |

**验收**：Graph Run → 任务自动出现 → `kanban_complete` → 看板更新 + Graph 继续。

---

## 4. Phase 2 — 依赖 + 观测（P2）✅

| ID | 任务 | 状态 |
|----|------|------|
| HK-DEP-01 | graph_task_links Ent + repo | ✅ |
| HK-DEP-02 | promote on parent complete | ✅ |
| HK-TOOLS-03 | kanban_create/link | ✅ |
| HK-OBS-01 | ActivityHistory in StatusProjector | ✅ |
| HK-FE-02 | Events Tab + Kanban↔canvas focus | ✅ |

---

## 5. Phase 3 — UX + 集成（P2/P3）✅

| ID | 任务 | 状态 |
|----|------|------|
| HK-FE-03 | vuedraggable 列拖拽（unblock/approve） | ✅ |
| HK-FE-04 | Observatory 任务 Tab | ✅ |
| HK-INT-01 | Task status Webhook `graph.task.status` | ✅ |
| HK-INT-02 | spawn_fn worker lane | 📋 → Phase 4 |
| HK-ORCH-01 | triage/decompose（可选） | 📋 → Phase 5 |

---

## 6. Phase 4 — Worker 真派工（P1）📋

> **目标**：对齐 Hermes `_default_spawn`；Dispatcher claim 后自动启动 Agent Run。

| ID | 任务 | 包 / 文件 | 状态 | 验收 |
|----|------|-----------|------|------|
| HK-INT-02a | `DispatchTask` 调 RunGateway | `graph_task_runtime.go` · `task_dispatch.go` | ⏳ | Claim 后 30s 内 Agent Run 启动 |
| HK-INT-02b | Session 绑定 task metadata | `channel_ingress_session` 模式或 graph session helper | ⏳ | Session metadata 含 task_id |
| HK-INT-02c | env 注入 ARANEA_TASK_ID | `internal/agent` run spec | ⏳ | kanban_show 可读当前 task |
| HK-INT-02d | TaskRun.log_ref = run_id | `task.go` | ⏳ | ListTaskRuns 可跳转 Run |
| HK-INT-02e | 单测 + FlowLog `graph.task.dispatch` | service test | ⏳ | CI 绿 |

**依赖**：RunGateway 稳定（M40）、Graph 节点 assignee 配置。  
**不影响**：Web Chat 手动 Kanban 操作。

---

## 7. Phase 5 — UI 对标 Hermes Dashboard（P1–P3）📋

> **目标**：在 Graph/Observatory 嵌入场景下补齐 Hermes `/kanban` 核心交互，不做独立 kanban.db。

### 7.1 P1 — 详情与依赖

| ID | 任务 | 文件 | 状态 | Hermes 参照 |
|----|------|------|------|-------------|
| HK-FE-05 | Drawer「依赖」Tab：link/unlink UI | `GraphTaskDetailDrawer.vue`, `features/graph/tasks/useTaskLinks.ts` | ⏳ | DependencyEditor |
| HK-FE-05b | ListTaskLinks API 或 GetTask 扩展 | `graph.proto` · biz | ⏳ | GET task parents/children |

### 7.2 P2 — 看板工具栏与创建

| ID | 任务 | 文件 | 状态 | Hermes 参照 |
|----|------|------|------|-------------|
| HK-FE-06 | 搜索 + assignee 筛选 Toolbar | `GraphTaskKanbanToolbar.vue` | ⏳ | BoardToolbar |
| HK-FE-07 | 多选 + 批量 unblock | `GraphTaskKanban.vue` | ⏳ | BulkActionBar |
| HK-FE-08 | 列内 Inline Create | `GraphTaskKanban.vue` + CreateTask | ⏳ | InlineCreate |
| HK-FE-09 | Attention 诊断条 | `GraphTaskKanbanAttentionStrip.vue` | ⏳ | AttentionStrip |
| HK-FE-10 | 拖拽 reassign assignee | `kanbanColumns.ts` + PATCH API | ⏳ | bulk reassign |

### 7.3 P2 — 卡片信息密度

| ID | 任务 | 文件 | 状态 |
|----|------|------|------|
| HK-FE-11 | 卡片：priority · 评论数 · 阻塞图标 | `GraphTaskKanbanCard.vue` | ⏳ |
| HK-FE-12 | 子任务进度（link 完成数/总数） | 同上 + biz aggregate | ⏳ |

### 7.4 P3 — 可选增强

| ID | 任务 | 状态 | Hermes 参照 |
|----|------|------|-------------|
| HK-FE-13 | Graph 列表「打开任务板」快捷入口 | ⏳ | `/kanban` 一级 Tab |
| HK-FE-14 | Running 列 Agent 泳道 | ⏳ | Lanes by profile |
| HK-ORCH-01 | Admin「分解任务」LLM 按钮 | ⏳ | decompose API |
| HK-FE-15 | `kanban/tool_test.go` + column 单测 | ⏳ | — |

---

## 8. Phase 6 — Team 收敛（跟随 M53）

| ID | 任务 | 关联 |
|----|------|------|
| HK-ORCH-02 | Observatory 双 Kanban 统一布局（分屏） | M53 Phase 5–7 |
| HK-ORCH-03 | Task 状态 ↔ OrchestrationKanban 行高亮 | US-HK-06 增强 |

---

## 9. M36 差距映射（G18–G22）

| ID | 差距 | M54 状态 |
|----|------|----------|
| G18 | Graph 运行时 CreateTask | ✅ HK-RT-01 |
| G19 | TaskDispatcher | ✅ HK-RT-02 |
| G20 | kanban_* tools | ✅ HK-TOOLS-01 |
| G21 | Unblock + reclaim | ✅ HK-RT-04/05 |
| G22 | 任务依赖图 | ✅ HK-DEP-01；UI ⏳ HK-FE-05 |
| G14 | spawn_fn 真 Worker | ⏳ HK-INT-02 |

---

## 10. 推荐迭代顺序

```text
迭代 HK-a（P1，约 1.5 周）— 闭环派工
  HK-INT-02a–e → E2E：Graph Run → auto spawn → kanban_complete → resume

迭代 HK-b（P1，约 1 周）— 依赖可视化
  HK-FE-05 + HK-FE-05b → Drawer 依赖 Tab 可用

迭代 HK-c（P2，约 1.5 周）— Dashboard 交互
  HK-FE-06 → HK-FE-09 → HK-FE-08 → HK-FE-07

迭代 HK-d（P3，按需）
  HK-FE-13/14 · HK-ORCH-01 · M53 双 Kanban 收敛
```

---

## 11. 验证命令

```bash
make wire && make api && make build
go test ./internal/biz/... -run 'Task|Dispatcher|Link' -count=1
go test ./internal/service/... -run 'GraphTask|Kanban' -count=1
go test ./internal/tools/kanban/... -count=1
cd web && pnpm test -- taskStreamProjection kanbanColumns
```

**手工验收**（Graph Run 页）：

1. 含 Agent 节点的 Graph 执行 → Inspector「任务」出现卡片  
2. WS「实时」badge 亮 → 拖拽 blocked → 待处理 → unblock  
3. `kanban_complete` 后列迁移 + Graph 继续  
4. Observatory 双 Tab 切换任务/Agent 看板  

---

## 12. 文档修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| 1.0 | 2026-05-23 | Phase 0–3 任务板 |
| 1.1 | 2026-05-24 | Hermes UI 对照锚点；Phase 4 G14 spawn；Phase 5 UI 对标；Phase 6 M53 收敛 |
