# Hermes Kanban Phase 1–3 落地

**日期**：2026-05-23  
**模块**：M54 / M36 / M53

## 摘要

完成 Hermes 式 Graph Task 看板运行时闭环、Agent `kanban_*` 工具、依赖图、观测增强与前端交互。

## 后端

- Graph 节点 `node_start` → `CreateTask` + WS `graph_task_status`
- `TaskDispatcher`：pending → claim → Agent Run spawn
- `SubmitTask` complete → `ResumeGraph`
- `UnblockTask` / `LinkTasks` / `UnlinkTasks` RPC + biz
- `CheckTimeouts` Cron 挂接 + heartbeat reclaim
- `internal/tools/kanban` 工具集 + `ARANEA_TASK_ID` 自动挂载
- `graph_task_links` + 父任务完成 promote 子任务
- `ActivityHistory` 投影到 orchestration WS（`activity_history`）
- Team Observatory 响应增加 `graph_execution_id`
- `graph_task_status` 元数据增加 `webhook_topic`（G13 集成锚点）

## 前端

- `GraphTaskDetailDrawer`：解除阻塞、事件时间线 Tab
- `GraphTaskKanban`：空板引导、Kanban↔画布 focus、管理员跨列拖拽（unblock/approve）
- `OrchestrationKanbanCard`：Activity 时间线（TG-OBS-HIST）
- `TeamRunObservatoryPage`：Agent / 任务双 Tab

## 刻意未做（v1）

- 独立全局 `kanban.db`
- `spawn_fn` 外部 worker lane（对齐 G14，待后续）
- Orchestrator triage/decompose auxiliary LLM

## 验证

```bash
make api && make wire && go build ./cmd/admin && go test ./internal/biz/... ./internal/service/...
cd web && pnpm build
```
