# M54: Hermes Kanban 适配 — 开发计划

> **版本**：2026-05-23 | **状态**：✅ Phase 0–3 已落地；Phase 4 跟随 M53  
> **需求**：[54-hermes-kanban.md](./54-hermes-kanban.md) · **设计**：[54-hermes-kanban.design.md](./54-hermes-kanban.design.md)  
> **进度**：[execution-plan.md](../guides/execution-plan.md) EP-HK-01

---

## 1. 代码锚点

| 层级 | 路径 |
|------|------|
| Task 领域 | `internal/biz/task.go`, `task_dispatch.go`, `task_dispatcher.go`, `task_links.go` |
| Graph 挂钩 | `internal/biz/graph_execution.go`, `internal/service/graph_task_runtime.go` |
| WS | `internal/service/graph_task_status.go` |
| Tools | `internal/tools/kanban/` |
| 前端 | `web/src/components/graph/GraphTaskKanban*.vue`, `features/graph/tasks/` |
| Proto | `api/kratos/graph/v1/graph.proto` |

---

## 2. Phase 0 — 文档 ✅

- M54 需求/设计/开发计划
- 交叉引用 M36/M53/frontend-pages/execution-plan/README/devlog

---

## 3. Phase 1 — 运行时 + Tools（P1）

| ID | 任务 | 状态 |
|----|------|------|
| HK-RT-01 | GraphTaskCoordinator + node start CreateTask | ✅ |
| HK-RT-02 | TaskDispatcher background tick | ✅ |
| HK-RT-03 | OnTaskCompleted → ResumeExecution | ✅ |
| HK-RT-04 | UnblockTask RPC | ✅ |
| HK-RT-05 | CheckTimeouts runner | ✅ |
| HK-TOOLS-01 | kanban toolset | ✅ |
| HK-TOOLS-02 | builtin seed + toolset prune | ✅ |
| HK-FE-01 | Drawer unblock + 空板引导 | ✅ |

**验收**：Graph Run → 任务自动出现 → kanban_complete → 看板更新 + Graph 继续。

---

## 4. Phase 2 — 依赖 + 观测（P2）

| ID | 任务 | 状态 |
|----|------|------|
| HK-DEP-01 | graph_task_links Ent + repo | ✅ |
| HK-DEP-02 | promote on parent complete | ✅ |
| HK-TOOLS-03 | kanban_create/link | ✅ |
| HK-OBS-01 | ActivityHistory in StatusProjector | ✅ |
| HK-FE-02 | Events Tab + Kanban↔canvas focus | ✅ |

---

## 5. Phase 3 — UX + 集成（P2/P3）

| ID | 任务 | 状态 |
|----|------|------|
| HK-FE-03 | vuedraggable 列拖拽 | ✅ |
| HK-FE-04 | Observatory 任务 Tab | ✅ |
| HK-INT-01 | Task status Webhook（G13） | ✅ |
| HK-INT-02 | spawn_fn worker lane | 📋 G14 |
| HK-ORCH-01 | triage/decompose（可选） | 📋 |

---

## 6. Phase 4 — Team 收敛

跟随 M53 Phase 5–7：TG-RT-TASK、Observatory 双 Kanban 统一。

---

## 7. M36 差距补充（G18–G22）

| ID | 差距 | 关联 |
|----|------|------|
| G18 | Graph 运行时 CreateTask | HK-RT-01 |
| G19 | TaskDispatcher | HK-RT-02 |
| G20 | kanban_* tools | HK-TOOLS-01 |
| G21 | Unblock + reclaim | HK-RT-04/05 |
| G22 | 任务依赖图 | HK-DEP-01 |
