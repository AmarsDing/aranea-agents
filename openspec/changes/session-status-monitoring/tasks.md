# Session 状态监控 — 任务清单

> **状态**: ✅ 已完成
> **完成日期**: 2026-06-02

---

## 实施总结

### 已完成的任务

| Task | 描述 | 状态 |
|------|------|------|
| Task 1 | 定义 SessionStatus / SessionStatusReason 常量 (`internal/biz/session/status.go`) | ✅ |
| Task 2 | 实现 SessionStatusMachine + 单测 (`status_machine.go` + `status_machine_test.go`) | ✅ |
| Task 3 | Ent Schema 变更：status 默认值 idle + 新增 status_reason / status_changed_at | ✅ |
| Task 4 | SessionStatusTransitioner 子接口 + Data 层实现 + Archive/Delete 保护 | ✅ |
| Task 5 | 批量操作 WHERE 条件更新 (`session_repo_batch.go`) | ✅ |
| Task 6 | SessionUsecase 新增 TransitionStatus + 保护逻辑 + BatchTransitionInterrupted + RecoverOrphanedRunning | ✅ |
| Task 7 | 搜索/查询中的 status 过滤迁移 (active→archived_at/deleted_at) | ✅ |
| Task 8 | ChatOrchestrator 状态转换触发点改造 | ✅ (本次补充 4 处缺失) |
| Task 9 | SessionStatusGuard 优雅退出 + 异常恢复 + 测试 | ✅ (本次新增测试) |
| Task 10 | Proto 变更 + make api (status_reason=48, status_changed_at=49) | ✅ |
| Task 11 | SessionService Proto 映射变更 (toProtoSession) | ✅ |
| Task 12 | WS Envelope session.status_changed + 前端 sessionSync + patchSessionStatus | ✅ |
| Task 13 | 前端类型 + SessionStatusBadge + 删除保护 UI | ✅ |
| Task 14 | Wire 装配更新 (WireSessionStatusPublisher + 生命周期注册) | ✅ |
| Task 15 | 数据迁移脚本 (active→idle + NULL defaults) | ✅ |
| Task 16 | 全量验证 | ✅ |

### 本次新增/修改的文件

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/service/chat_orchestrator_turn.go` | 修改 | 补充 4 处缺失的 transitionSessionStatus 调用 |
| `internal/service/session_status_guard_test.go` | 新增 | SessionStatusGuard 单元测试 (3 个用例) |

### ChatOrchestrator 触发点补全详情

| 场景 | 位置 | 新增调用 |
|------|------|----------|
| 首字节超时 | chat_orchestrator_turn.go:861 | `transitionSessionStatus(interrupted, error)` |
| 流式错误 | chat_orchestrator_turn.go:867 | `transitionSessionStatus(interrupted, error)` |
| Turn 开始运行 | chat_orchestrator_turn.go:655 | `transitionSessionStatus(running)` |
| Resume await 失败 | chat_orchestrator_turn.go:401 | `transitionSessionStatus(interrupted, error)` |

### 已有的触发点（确认无需修改）

| 场景 | 位置 | 状态 |
|------|------|------|
| Turn 超时 | chat_orchestrator_turn.go:885 | ✅ 已有 |
| 空回复 | chat_orchestrator_turn.go:913 | ✅ 已有 |
| Turn 完成 | chat_orchestrator_turn.go:983 | ✅ 已有 |
| 工具需确认 | chat_orchestrator_turn.go:357 | ✅ 已有 |
| Agent 等待回复 | chat_orchestrator_turn.go:359 | ✅ 已有 |
| 确认后恢复运行 | chat_orchestrator_turn.go:366 | ✅ 已有 |
| 预算升级 | chat_orchestrator_session_run.go:236 | ✅ 已有 |
| 用户取消 | chat_orchestrator.go:387 | ✅ 已有 |
| Pending queue | chat_orchestrator_turn.go:1026-1037 | ✅ 已有 |
