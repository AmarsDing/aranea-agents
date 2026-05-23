# 2026-05-23 Hermes Kanban M54 优化重构

**影响**：🟡 中 | **模块**：Biz / Service / Web (M54)

## 目标

在不改变对外行为的前提下，收敛 M54 任务板内核的职责边界、消除重复逻辑与静默失败，降低回归风险。

## 后端

### 接口分层

- `TaskStatusPublisher` 从 `GraphTaskCoordinator` 拆出；`TaskUsecase` 仅依赖发布接口，避免与 Graph 生命周期 hook 耦合。
- `ShouldCreateTaskForNode` 导出为公共函数，Graph 执行与 Runtime 共用同一门控。

### 派工与依赖

- 新增 `task_dispatch.go`：`allParentTasksComplete` / `isTaskReadyForDispatch` / `resolveDispatchAssignee` / `publishTaskStatus`。
- `TaskDispatcher`：tick 带超时；claim/dispatch 失败写 Warn 日志；依赖门控与 `promoteReadyChildren` 共用同一 helper。
- `promoteReadyChildren`：移除无字段变更的 `UpdateTask` 空写；仅发 `task_ready` 事件 + WS。

### 健壮性

- `CreateTaskWithParents`：修复无 parent 时重复 `afterTaskMutation`（双 WS 推送）。
- `CheckTimeouts`：统一走 `publishTaskStatus`；`UpdateTask` 错误不再静默丢弃。
- `graph_execution`：`OnGraphNodeStart` 失败写 Warn 日志。
- `GraphTaskRuntime`：Resume / Publish 失败写日志；dispatch RunID 加 uuid 后缀避免冲突。
- 删除未使用的 `graph_task_status_publish.go`（发布路径已收敛到 `GraphTaskRuntime`）。

### 测试

- `task_dispatch_test.go`：parent 依赖门控单测。

## 前端

- 提取 `features/graph/tasks/kanbanColumns.ts`：列定义、空板文案、`kanbanAdminActionForDrop`。
- `GraphTaskKanban.vue` 消费共享常量，拖拽规则单点维护。

## 回归风险

| 区域 | 风险 | 缓解 |
|------|------|------|
| WS 推送频率 | `CreateTaskWithParents` 少一次重复推送 | 预期行为；E2E 看板仍应更新 |
| Dispatcher 日志 | 失败时多 Warn 行 | 仅运维可见，不改变状态机 |
| promote | 去掉空 UpdateTask | 子任务 status 仍为 pending，依赖 dispatcher 门控 |

## 验收

```bash
go test ./internal/biz/ -run "TestAllParent|TestIsTask" -count=1
go build ./cmd/admin/...
cd web && pnpm build
```

## 待办（未变）

- `spawn_fn`（G14）、triage/decompose、RunGateway 真 spawn。
