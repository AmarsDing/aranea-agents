# Graph 前端 Phase C — Checkpoint / TimeTravel / Task 看板

**日期**：2026-05-23  
**模块**：Graph (36) · 前端

## 摘要

按 `36-graph-development.md` Phase C 落地 Graph Run 页人机协同与任务态 UI：检查点时间线、TimeTravel/EditState、Task Kanban + 详情抽屉，并接线 WS `graph_task_status` 投影。

## Phase C — 人机协同与任务态

| 项 | 变更 |
|----|------|
| C-1 | 新增 `GraphCheckpointPanel.vue` + store `listCheckpoints` |
| C-2 | 新增 `useGraphTimeTravel.ts` + `GraphTimeTravelPanel.vue`（GetStateSnapshot / EditState / Resume） |
| C-3 | 新增 `GraphTaskKanban.vue` / `GraphTaskKanbanCard.vue` |
| C-4 | 新增 `GraphTaskDetailDrawer.vue`（评论/日志/运行记录 + claim/submit/review） |
| C-5 | `useGraphExecutionStream` 订阅 `graph_task_status`；`taskStreamProjection.ts` 归一化 biz 状态 |
| C-6 | 新增 `GraphRunInspector.vue`（监控 \| 检查点 \| 任务 三 Tab）整合 Run 侧栏 |

## 架构

- **SRP**：Task/Checkpoint 投影在 `features/graph/tasks/` 与 `runtime/`；组件仅 props/emits
- **WS**：`graph_task_status` metadata（`execution_id` / `node_id` / `task_id` / `task_status` / `assignee` / `summary`）
- **影响域**：`envelope.ts`、`stores/graph`、`useGraphRunPage`、`GraphRunPage.vue`

## 验证

```bash
cd web && pnpm test -- --run src/features/graph/runtime/graphExecutionProjection.spec.ts src/features/graph/tasks/taskStreamProjection.spec.ts
cd web && pnpm build
```
