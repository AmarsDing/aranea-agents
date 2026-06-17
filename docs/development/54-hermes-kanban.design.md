# M54: Hermes Kanban 适配 — 技术设计

> **版本**：2026-05-24  
> **需求**：[54-hermes-kanban.md](./54-hermes-kanban.md)  
> **开发计划**：[54-hermes-kanban-development.md](./54-hermes-kanban-development.md)

---

## 1. 架构总览

### 1.1 Aranea 运行时（已实现）

```text
Graph Run (ExecuteGraph)
    │
    ├─ graph_node_start ──► GraphTaskCoordinator.OnNodeStart
    │                           ├─ TaskUsecase.CreateTask
    │                           └─ PublishGraphTaskStatus (WS)
    │
    ├─ TaskDispatcher (background tick, default 30s)
    │       ├─ ListTasks pending / pending_assignment
    │       ├─ ClaimTask + TaskRun record
    │       ├─ CheckTimeouts (heartbeat → timed_out → reclaim)
    │       └─ [G14] RunGateway.SpawnAgentRun ──► 待实现
    │
    ├─ Agent kanban_* tools ──► KanbanToolBridge ──► TaskUsecase
    │
    └─ task complete/review ──► GraphTaskCoordinator.OnTaskComplete
                                    └─ GraphUsecase.ResumeExecution
```

**分层**：

| 包 | 职责 |
|----|------|
| `internal/biz` | `TaskUsecase`、`TaskDispatcher`、`task_links`、`GraphTaskCoordinator` 接口 |
| `internal/service` | `GraphTaskRuntime`、`graph_task_status` WS、`kanban_bridge.go` |
| `internal/tools/kanban` | CallableTool → `KanbanToolBridge` |
| `web/` | `GraphTaskKanban`、`GraphTaskDetailDrawer`、Observatory 集成 |

### 1.2 Hermes 参考架构（对照）

```text
Gateway _kanban_dispatcher_watcher (60s)
    └─ kanban_db.dispatch_once() per board
           ├─ release_stale_claims / detect_stale_running
           ├─ recompute_ready (parent done → child ready)
           └─ claim_task + _default_spawn(profile, task_id)

Surfaces:
  kanban_db.py  ◄──  tools/kanban_tools.py
                ◄──  hermes_cli/kanban.py
                ◄──  plugins/kanban/dashboard/plugin_api.py
                ◄──  React KanbanPage (dist/index.js)
```

**关键差异**：Hermes **单 SQLite 内核** + **真 spawn 子进程 worker**；Aranea **Ent graph_tasks** + **Graph 作用域** + spawn 待 G14。

---

## 2. 状态映射

| Hermes status | Aranea TaskStatus | GraphTaskKanban 列 |
|---------------|-------------------|-------------------|
| triage / todo | TASK_PENDING | 待处理 |
| scheduled | TASK_PENDING | 待处理（无独立列） |
| ready / pending_assignment | TASK_PENDING_ASSIGNMENT | 待处理 |
| running | TASK_CLAIMED | 执行中 |
| blocked | TASK_BLOCKED | 异常 |
| review | TASK_REVIEW_REQUIRED | 待审核 |
| done | TASK_COMPLETE | 已完成 |
| failed / archived | TASK_FAILED / TASK_CANCELLED / TASK_TIMED_OUT / TASK_CRASHED | 异常 |

---

## 3. GraphTaskCoordinator

```go
type TaskStatusPublisher interface {
    PublishTaskStatus(ctx context.Context, task *GraphTask, extra map[string]any)
}

type GraphTaskCoordinator interface {
    TaskStatusPublisher
    OnGraphNodeStart(ctx context.Context, exec *GraphExecution, node *NodeDef, meta NodeTaskMeta, inputPreview string) error
    OnTaskCompleted(ctx context.Context, task *GraphTask) error
}
```

- **OnGraphNodeStart**：对 `agent` / `llm` 节点 CreateTask；幂等 execution+node；`meta NodeTaskMeta` 携带 Team/Graph 节点上下文（定义于 `internal/biz/compiled_team.go`）
- **OnTaskCompleted**：`waiting_human` / 挂起 → `ResumeExecution`
- 实现：`internal/service/graph_task_runtime.go`（`GraphTaskRuntime` 同时实现 `TaskStatusPublisher` + `GraphTaskCoordinator` + `TaskDispatchAgentRunner`）

---

## 4. TaskDispatcher

文件：`internal/biz/task_dispatcher.go`

| 步骤 | 行为 | Hermes 对照 |
|------|------|-------------|
| Tick | `dispatch_interval_seconds`（默认 30） | `dispatch_once` 60s |
| Scan | pending / pending_assignment | `recompute_ready` + ready queue |
| Assign | static `agent_name` / dynamic strategy | assignee profile |
| Claim | ClaimTask + TaskRun | `claim_task` |
| Spawn | **LogRef only** 📋 | `_default_spawn` hermes chat |
| Timeout | heartbeat → timed_out → pending | `detect_stale_running` |

**G14 目标实现**（`internal/service/graph_task_runtime.go` 或 `task_dispatch.go`）：

```go
// DispatchTask — 从仅写 TaskRun 扩展为：
// 1. Resolve assignee → Agent ID
// 2. RunGateway.EnqueueAgentRun(ctx, AgentRunSpec{
//      AgentID, SessionID?, Prompt: fmt.Sprintf("Work graph task %s", task.ID),
//      Env: map[string]string{"ARANEA_TASK_ID": task.ID, "ARANEA_EXECUTION_ID": exec.ID},
//      Toolsets: []string{"kanban"},
//    })
// 3. TaskRun.LogRef = run_id
```

环境变量对照 Hermes `HERMES_KANBAN_TASK` → Aranea `ARANEA_TASK_ID`（`internal/tools/kanban/env.go`）。

---

## 5. 数据模型

### 5.1 graph_tasks（Ent，已有）

核心字段：id, execution_id, node_id, status, assignee, input, output, summary, blocked_reason, preview 等。

### 5.2 graph_task_links（P2 ✅）

```sql
-- internal/data/ent/schema/graph_task_link.go
parent_task_id, child_task_id, execution_id
```

- `LinkTasks` / `UnlinkTasks` RPC
- 父 `TASK_COMPLETE` → `promoteReadyChildren`

### 5.3 Activity 历史（P2 ✅）

`AgentNodeState.ActivityHistory` — StatusProjector append（cap 20）。

---

## 6. Proto / RPC（已实现）

| RPC | HTTP | Hermes 对照 |
|-----|------|-------------|
| ListTasks | GET `/v1/graph/executions/{execution_id}/tasks` | `kanban list/show` |
| GetTask | GET `/v1/graph/tasks/{task_id}` | `kanban show` |
| ClaimTask | POST `/v1/graph/tasks/{task_id}/claim` | dispatcher claim |
| SubmitTaskResult | POST `/v1/graph/tasks/{task_id}/submit` | `kanban_complete` |
| Heartbeat | POST `/v1/graph/tasks/{task_id}/heartbeat` | `kanban_heartbeat` |
| ReportBlocked | POST `/v1/graph/tasks/{task_id}/blocked` | block |
| UnblockTask | POST `/v1/graph/tasks/{task_id}/unblock` | unblock |
| CreateTask | POST `/v1/graph/executions/{execution_id}/tasks` | `kanban_create` |
| LinkTasks | POST `/v1/graph/tasks/link` | `kanban_link` |
| UnlinkTasks | POST `/v1/graph/tasks/unlink` | unlink |
| ReviewTask | POST `/v1/graph/tasks/{task_id}/review` | review column claim |
| ListTaskComments | GET `/v1/graph/tasks/{task_id}/comments` | `kanban_comment` list |
| AddTaskComment | POST `/v1/graph/tasks/{task_id}/comments` | `kanban_comment` add |
| ListTaskLogs | GET `/v1/graph/tasks/{task_id}/logs` | drawer logs |
| ListTaskRuns | GET `/v1/graph/tasks/{task_id}/runs` | drawer runs |
| ListTaskEvents | GET `/v1/graph/executions/{execution_id}/task-events` | drawer events |

定义：`api/kratos/graph/v1/graph.proto`（TaskService，行 916–993）

> **未实现 RPC**：`ListTaskLinks`（依赖 Tab 需要时通过 `GetTask` 扩展 links 或新增 RPC，见 development §7.1 HK-FE-05b）、`ReleaseTask`（拖拽 reassign 场景，见 §8.4 HK-FE-10）。

---

## 7. kanban_* Tools

包：`internal/tools/kanban`

| Tool | Biz | Orchestrator only |
|------|-----|-------------------|
| kanban_show | GetTask + comments + events | — |
| kanban_list | ListTasks | ✅ |
| kanban_complete | SubmitTaskResult | — |
| kanban_block | ReportBlocked | — |
| kanban_unblock | UnblockTask | ✅ |
| kanban_heartbeat | Heartbeat | — |
| kanban_comment | AddTaskComment | — |
| kanban_create | CreateTask | — |
| kanban_link | LinkTasks | — |

注册：`internal/tools/trpc/toolsets.go`（`ToolsetConfig.Kanban` 开关，行 199–201）  
Seed：`internal/data/builtin_tools_seed.go`（key=`kanban`，readonly seed，行 83）  
启用判定：`internal/agent/tool_assembly.go`（`kanbanpkg.Enabled()` 检查，行 73–78）  
Bridge：`internal/service/kanban_bridge.go`  
启用条件：`ARANEA_TASK_ID` 非空 或 `ARANEA_KANBAN_TOOLS=1`（`internal/tools/kanban/tools.go` `Enabled()`，行 263–264）

---

## 8. 前端架构

### 8.1 组件树

```text
GraphRunPage / TeamRunObservatoryPage
  └─ GraphRunInspector | q-tab「任务看板」
        └─ GraphTaskKanban (WorkflowKanbanBoard shell)
              ├─ draggable columns (vuedraggable)
              └─ GraphTaskKanbanCard
        └─ GraphTaskDetailDrawer (maximized glass dialog)
  └─ q-tab「Agent 工作看板」
        └─ OrchestrationKanban
              └─ OrchestrationKanbanCard (收/做/交 zones)
```

### 8.2 数据流

```text
WS Envelope graph_task_status
  → taskStreamProjection.ts
  → useGraphRunTasks.ts (merge tasks, focusTaskForNode)
  → GraphTaskKanban props.tasks

User drag / drawer action
  → GraphTaskKanban.vue emit('adminAction', {taskId, action})
  → features/graph/api.ts (Claim/Submit/Unblock/Review)
  → store refresh + WS echo
```

> `adminAction` 是 `GraphTaskKanban.vue` 的 emit 事件（`'unblock' | 'approve'`），由父组件消费调用对应 RPC。

### 8.3 Hermes UI → Aranea 组件映射

| Hermes | Aranea 文件 | 说明 |
|--------|-------------|------|
| KanbanPage | `GraphRunInspector` + Observatory tab | 嵌入非独立页 |
| BoardSwitcher | Router `execution_id` | 隐式；无多 board UI |
| BoardToolbar | — | 待补 HK-FE-06 |
| BulkActionBar | — | 待补 HK-FE-07 |
| BoardColumns | `WorkflowKanbanBoard` + `kanbanColumns.ts` | 5 列 |
| TaskCard | `GraphTaskKanbanCard.vue` | 字段较少 |
| TaskDrawer | `GraphTaskDetailDrawer.vue` | 缺依赖 Tab |
| DependencyEditor | — | 待补 HK-FE-05 |
| InlineCreate | — | 待补 HK-FE-08 |
| TrashDropZone | — | 不实现；用 Graph 取消 execution |
| DiagnosticsSection | — | 待补 HK-FE-09 |
| WS /events | `graph_task_status` | 已实现 |
| Lanes by profile | OrchestrationKanban 列表 | 非泳道布局 |

> 各组件的实施进度（✅/⏳/📋）详见 [开发计划](./54-hermes-kanban-development.md)。

### 8.4 拖拽 Admin 语义

`kanbanColumns.ts` → `kanbanAdminActionForDrop`:

| 拖放 | 动作 | RPC |
|------|------|-----|
| 异常 → 待处理 | unblock | `UnblockTask` |
| 待审核 → 已完成 | approve | `ReviewTask` (approve) |
| 其他 | 忽略 | Hermes 允许更广 PATCH |

**扩展 HK-FE-10**：pending ← claimed 触发 `ReleaseTask`（若新增 RPC）或 reassign。

### 8.5 Kanban ↔ Graph 画布联动

- `useGraphRunTasks.focusTaskForNode(tasks, nodeId)` → `GraphEditorCanvas` `:focus-selected-node` prop（`GraphRunPage.vue` 行 67）
- 选卡片 → 高亮对应节点；选节点 → scroll card into view（`GraphTaskKanban` card refs）

---

## 9. Phase 5 实现设计（UI + G14）

### 9.1 HK-INT-02 Worker Spawn（P1）

**触发**：`TaskDispatcher` claim 成功后  
**依赖**：`RunGateway`、`AgentFactory`（已有 Chat/Team 路径）  
**步骤**：

1. 从 Task.assignee / node Agent 解析 `agent_id`
2. 创建或复用 Session（metadata `graph_task_id`）
3. `ChatService` 或专用 `RunNativeTurnUnary` 带 system hint「执行 Graph 任务 {id}」
4. 注入 env：`ARANEA_TASK_ID`、`ARANEA_EXECUTION_ID`
5. TaskRun 记录 `run_id`；完成时 Coordinator 回调

**验收**：Claim 后 Agent Run 自动启动；`kanban_complete` 结束 Run。

### 9.2 HK-FE-05 依赖 Tab（P1）

**UI**：`GraphTaskDetailDrawer` 新增 Tab「依赖」  
**API**：`ListTaskLinks`（若无则 GetTask 扩展 links）· 已有 Link/Unlink RPC  
**交互**：parent/child chips；添加 parent_id；unlink 按钮；环检测错误 toast

### 9.3 HK-FE-06 Toolbar（P2）

**组件**：`GraphTaskKanbanToolbar.vue`  
**能力**：client-side filter（nodeId, assignee, status）；assignee 下拉（从 tasks 聚合）  
**位置**：`GraphTaskKanban` header 行

### 9.4 HK-FE-08 Inline Create（P2）

列头 `+` → 小表单 → `CreateTask` RPC（execution_id, node_id 选手动或默认 orchestrator 节点）

### 9.5 HK-FE-09 Diagnostics（P2）

**规则**（client-side，免新 API）：

- claimed + `last_heartbeat` 超 `heartbeat_timeout_sec` → ⚠ stale
- pending_assignment + 无 assignee → needs assignee
- blocked + 长 duration → attention

**组件**：`GraphTaskKanbanAttentionStrip.vue`

### 9.6 HK-ORCH-01 Triage/Decompose（P3，可选）

Hermes：`kanban_specify.py` / `kanban_decompose.py` 调 auxiliary LLM。  
Aranea：Graph 节点 `llm` + `kanban_create/link` 工具链替代；或 Admin「分解任务」按钮调 `ChatService` 一次性 fan-out。

---

## 10. Hermes vs Aranea 技术栈

| Hermes | Aranea |
|--------|--------|
| React dashboard plugin (IIFE) | Vue 3 + Quasar Admin |
| FastAPI `/api/plugins/kanban` | Kratos HTTP graph.proto |
| SQLite kanban.db | Ent graph_tasks + graph_task_links |
| Python kanban_db | Go TaskRepo + TaskDispatcher |
| HTML5 drag-and-drop | vuedraggable |
| localStorage board switch | Router execution scope |
| Gateway dispatcher 60s | TaskDispatcher 30s + Cron timeout runner |

---

## 11. 测试

| 层 | 文件 |
|----|------|
| Biz | `internal/biz/task_dispatch_test.go`（含 `TestAllParentTasksComplete`、`TestIsTaskReadyForDispatch`） |
| Biz | `internal/biz/graph_node_task_test.go`（`TestShouldCreateTaskForNode`） |
| Biz | `internal/biz/graph_task_input_test.go` |
| Tools | `internal/tools/kanban/tools_test.go`（`TestNewToolset_*`、`TestEnabled_*`） |
| Tools | `internal/tools/kanban/bridge_test.go`（Bridge 接口契约 + env 读取） |
| Tools | `internal/tools/kanban/bridge_more_test.go`（9 工具 Call 路径覆盖） |
| Frontend | `web/src/features/graph/tasks/taskStreamProjection.spec.ts` |
| E2E | Graph Run → task appear → complete → resume（手工 LT） |

> 各测试的通过状态与待补清单（如 `kanbanColumns.spec.ts`、service 层 `graph_task_runtime_test.go`）详见 [开发计划 §7.4 HK-FE-15](./54-hermes-kanban-development.md#74-p3--可选增强)。

---

## 12. 文档修订记录

| 版本 | 日期 | 说明 |
|------|------|------|
| 1.0 | 2026-05-23 | Coordinator、Dispatcher、Tools、前端映射 |
| 1.1 | 2026-05-24 | Hermes 参考架构；UI 组件对照表；Phase 5/G14 实现设计 |
| 1.2 | 2026-06-17 | 三件套内容边界整理 + 代码核对：修正 `GraphTaskCoordinator` 接口签名（补 `NodeTaskMeta`）；Proto HTTP 方法全量列示（修正 `UnlinkTasks`/`ListTaskComments`/`AddTaskComment`）；修正 `builtin_tools_seed.go` 路径为 `internal/data/`；修正 `adminAction` 归属（`GraphTaskKanban.vue` emit）；测试文件清单与代码对齐（`tools_test.go` 非 `tool_test.go`，无独立 `task_links_test.go`/`graph_task_runtime_test.go`）；§8.3/§11 移除状态标记（迁移至 development） |
