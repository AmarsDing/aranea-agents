# M54: Hermes Kanban 适配 — 技术设计

> **版本**：2026-05-23  
> **需求**：[54-hermes-kanban.md](./54-hermes-kanban.md)  
> **开发计划**：[54-hermes-kanban-development.md](./54-hermes-kanban-development.md)

---

## 1. 架构总览

```text
Graph Run (ExecuteGraph)
    │
    ├─ graph_node_start ──► GraphTaskCoordinator.OnNodeStart
    │                           ├─ TaskUsecase.CreateTask
    │                           └─ PublishGraphTaskStatus (WS)
    │
    ├─ TaskDispatcher (background tick)
    │       └─ pending ──► ClaimTask + enqueue worker hint
    │
    ├─ Agent kanban_* tools ──► TaskUsecase (biz)
    │
    └─ task complete/review ──► GraphTaskCoordinator.OnTaskComplete
                                    └─ GraphUsecase.ResumeExecution
```

**分层**：
- `internal/biz`：`TaskUsecase`、`GraphTaskCoordinator` 接口、`TaskDispatcher` 扫描逻辑
- `internal/service`：`GraphTaskRuntimeHook` 实现协调器 + WS 投影
- `internal/tools/kanban`：Agent CallableTool → 注入 `KanbanToolBridge`
- `web/`：`GraphTaskKanban` + `GraphTaskDetailDrawer` + Observatory 扩展

---

## 2. 状态映射

| Hermes | Aranea TaskStatus | GraphTaskKanban 列 |
|--------|-------------------|-------------------|
| triage / todo | pending | 待处理 |
| ready | pending / pending_assignment | 待处理 |
| running | claimed | 执行中 |
| blocked | blocked | 异常 |
| review | review_required | 待审核 |
| done | complete | 已完成 |
| failed / archived | failed / timed_out / cancelled / crashed | 异常 |

---

## 3. GraphTaskCoordinator

```go
type GraphTaskCoordinator interface {
    OnGraphNodeStart(ctx context.Context, exec *GraphExecution, node *NodeDef, inputPreview string) error
    OnTaskCompleted(ctx context.Context, task *GraphTask) error
    PublishTaskStatus(ctx context.Context, task *GraphTask, extra map[string]any)
}
```

- **OnGraphNodeStart**：对 `agent` / `llm` 节点（可配置）创建 Task；幂等：同 execution+node 仅一条 active task
- **OnTaskCompleted**：若 execution 为 `waiting_human` 或节点挂起，调用 `ResumeExecution`
- 实现在 `internal/service/graph_task_runtime.go`

---

## 4. TaskDispatcher

文件：`internal/biz/task_dispatcher.go`

| 步骤 | 行为 |
|------|------|
| Tick | 每 `dispatch_interval_seconds`（默认 30） |
| Scan | `ListTasksByExecution` 或全局 pending / pending_assignment |
| Assign | static → `agent_name`；dynamic → `assignment_strategy` |
| Claim | 自动 ClaimTask + 记录 TaskRun |
| Notify | `PublishTaskStatus` |

超时：`CheckTimeouts` 将无 heartbeat 的 claimed → timed_out；可选 reclaim → pending（Hermes reclaim 语义）。

Cron：挂接 `internal/runner/task_timeout_runner.go` 或 Cron 模块 periodic job。

---

## 5. 数据模型扩展

### 5.1 graph_task_links（P2）

```sql
CREATE TABLE graph_task_links (
  id TEXT PRIMARY KEY,
  parent_task_id TEXT NOT NULL,
  child_task_id TEXT NOT NULL,
  execution_id TEXT NOT NULL,
  created_at TIMESTAMP
);
```

- `LinkTasks(parent, child)` / `UnlinkTasks`
- 父 `complete` → 检查所有 parent links 的父均 complete → 子 `pending`

### 5.2 Activity 历史（P2）

`AgentNodeState` 扩展：

```go
ActivityHistory []ActivitySnapshot `json:"activity_history,omitempty"`
```

StatusProjector append 而非 replace `CurrentActivity`（cap 20）。

---

## 6. Proto / RPC 扩展

| RPC | HTTP | 说明 |
|-----|------|------|
| UnblockTask | POST `/v1/graph/tasks/{task_id}/unblock` | blocked → pending |
| LinkTasks | POST `/v1/graph/tasks/link` | parent_id + child_id |
| UnlinkTasks | DELETE `/v1/graph/tasks/link` | 移除依赖 |
| CreateTask | POST `/v1/graph/executions/{execution_id}/tasks` | orchestrator 显式创建 |

---

## 7. kanban_* Tools

包：`internal/tools/kanban`

| Tool | Biz 调用 |
|------|----------|
| kanban_show | GetTask + ListComments + ListEvents |
| kanban_list | ListTasks |
| kanban_complete | SubmitTaskResult |
| kanban_block | ReportBlocked |
| kanban_unblock | UnblockTask |
| kanban_heartbeat | Heartbeat |
| kanban_comment | AddTaskComment |
| kanban_create | CreateTask |
| kanban_link | LinkTasks |

**上下文**：Runner 注入 `ARANEA_TASK_ID`、`ARANEA_EXECUTION_ID`；Tool bridge 从 context 读取。

注册：`internal/tools/trpc/toolsets.go` 增加 `kanban` toolset；`builtin_tools_seed.go` 种子行。

---

## 8. 前端组件

```
web/src/components/workflow/WorkflowKanbanBoard.vue   # 列壳
web/src/components/graph/GraphTaskKanban.vue          # 任务列
web/src/components/graph/GraphTaskDetailDrawer.vue    # 详情/unblock/events
web/src/components/orchestration/OrchestrationKanban.vue  # Agent 行（M53）
web/src/features/graph/tasks/taskStreamProjection.ts  # WS 映射
web/src/features/kanban/useGraphTaskKanbanFocus.ts    # Kanban ↔ 画布 focus
```

Observatory Phase 3：`TeamRunObservatoryPage` 增加「任务」Tab → `GraphTaskKanban`。

---

## 9. Hermes UI 技术栈对照

| Hermes | Aranea |
|--------|--------|
| React dashboard plugin | Vue 3 + Quasar Admin |
| FastAPI REST | Kratos HTTP + graph.proto |
| SQLite kanban.db | Ent + SQLite graph_tasks |
| Python kanban_db | Go TaskRepo |
| HTML5 drag-and-drop | vuedraggable（P3） |
| localStorage board switch | Router execution scope |

---

## 10. 测试

| 层 | 文件 |
|----|------|
| Biz | `task_dispatcher_test.go`, `task_links_test.go` |
| Service | `graph_task_runtime_test.go` |
| Tools | `kanban/tool_test.go` |
| Frontend | `taskStreamProjection.test.ts`, `useGraphTaskKanbanFocus.test.ts` |
