# Session 状态监控 — 任务清单

> **状态**: ✅ 一期已完成 / ⏳ 二期未实现
> **完成日期**: 2026-06-02（一期） / 2026-06-05（review-fixes）

---

## 实施总结

### 已完成的任务

| Task | 描述 | 状态 |
|------|------|------|
| Task 1 | 定义 SessionStatus / SessionStatusReason 常量 (`internal/biz/session/status.go`) | ✅ |
| Task 2 | 实现 SessionStatusMachine + 单测 (`status_machine.go` + `status_machine_test.go`) | ✅ |
| Task 3 | Ent Schema 变更：status 默认值 idle + 新增 status_reason / status_changed_at | ✅ |
| Task 4 | SessionRuntimeWriter 新增 TransitionSessionStatus + Data 层实现 + Archive/Delete 保护 | ✅ |
| Task 5 | 批量操作 WHERE 条件更新 (`session_repo_batch.go`) | ✅ |
| Task 6 | SessionUsecase 新增 TransitionStatus + 保护逻辑 + BatchTransitionInterrupted + RecoverOrphanedRunning | ✅ |
| Task 7 | 搜索/查询中的 status 过滤迁移 (active→archived_at/deleted_at) | ✅ |
| Task 8 | ChatOrchestrator 状态转换触发点改造 | ✅ (含 Team Turn / Pending Queue 触发点) |
| Task 9 | SessionStatusGuard 优雅退出 + 异常恢复 + Team 恢复 + 编排恢复 + 测试 | ✅ |
| Task 10 | Proto 变更 + make api (status_reason=48, status_changed_at=49) | ✅ |
| Task 11 | SessionService Proto 映射变更 (toProtoSession) | ✅ |
| Task 12 | WS Envelope session.status_changed + 前端 sessionSync + patchSessionStatus | ✅ |
| Task 13 | 前端类型 + SessionStatusBadge + 删除保护 UI | ✅ |
| Task 14 | Wire 装配更新 (WireSessionStatusPublisher + 生命周期注册) | ✅ |
| Task 15 | 数据迁移脚本 (active→idle + NULL defaults) | ✅ |
| Task 16 | 全量验证 | ✅ |

### Review-Fixes 修复（2026-06-05）

| Task | 描述 | 状态 |
|------|------|------|
| R1 | 错误码修复：`kerrors.BadRequest` → `kerrors.Conflict`（status_machine + usecase） | ✅ |
| R2 | 超时 reason 分类修复：首字节超时/Turn 超时使用 `StatusReasonTimeout` | ✅ |
| R3 | 非法 status 值修复：`DeleteSessionsByAgentID` 移除 `SetStatus("deleted")` | ✅ |
| R4 | 并发冲突处理修复：`TransitionSessionStatus` 在 `n==0` 时返回 `kerrors.Conflict` | ✅ |
| R5 | 批量 WS 推送补全：`BatchTransitionInterrupted` 发布 `session.status_changed` 事件 | ✅ |
| R6 | 前端映射修复：`kratosSessionToLegacy` 补全 `status_reason` / `status_changed_at` | ✅ |
| R7 | Admin Store WS 监听：注册 `status_changed` 事件 + `patchSessionStatus` 方法 | ✅ |
| R7b | Chat Store WS 监听：注册 `status_changed` 事件 + `patchSessionStatus` 方法 | ✅ |
| R8 | 死代码清理：移除 `sessionUi.ts` 中 `statusBadgeColor` 函数 | ✅ |

### 代码中实际实现但原始任务未记录的内容

| 内容 | 代码位置 | 说明 |
|------|----------|------|
| Team Turn 触发点 | `internal/service/team_turn_hooks.go` | Team 会话的 running/interrupted/completed 状态转换 |
| Pending Queue 触发点 | `internal/service/chat_orchestrator_turn.go:1073-1087` | Pending queue 中 Team 会话的状态转换 |
| SessionStatusGuard Team 恢复 | `internal/service/session_status_guard.go:50-109` | 启动时恢复孤儿 running teams + team runs |
| SessionStatusGuard 编排恢复 | `internal/service/session_status_guard.go:112-117` | 启动时恢复中断的编排任务 |
| SessionStatusGuard 构造函数扩展 | `internal/service/session_status_guard.go:18` | 新增 `teamUC` 和 `orchestrator` 参数 |
| SessionRuntimeWriter 接口 | `internal/biz/session/metrics_repo.go:23-26` | `TransitionSessionStatus` 定义在此接口而非 `SessionMutator` |
| `IsProtectedStatus` 独立函数 | `internal/biz/session/status.go:31-33` | 独立于 Machine 的保护状态判断函数 |
| Chat Store WS 监听 | `web/src/stores/chat/sessionStore.ts:95-96` | Chat 页面也监听 `status_changed` 事件 |
| Spirit Team 错误触发点 | `internal/service/spirit_team.go:101` | Spirit team 执行出错时转为 interrupted |

### 未实现的二期任务

| Task | 描述 | 状态 |
|------|------|------|
| 2.1 | 确认超时自动清理（CleanupExpiredConfirmations） | ❌ 未实现 |
| 2.2 | Admin 强制状态覆盖 RPC（ForceTransitionSessionStatus） | ❌ 未实现 |
| 2.3 | 中断会话恢复引导 UI（「继续对话」按钮） | ❌ 未实现 |
| 2.4 | 工具确认弹窗 UI | ❌ 未实现 |
| 2.5 | Agent 等待回复提示 UI | ❌ 未实现 |
| 2.6 | 独立 status+deleted_at 索引（idx_sessions_status） | ❌ 未实现（当前仅有 deleted_at+status 复合索引） |

### 本次新增/修改的文件

| 文件 | 操作 | 说明 |
|------|------|------|
| `internal/service/chat_orchestrator_turn.go` | 修改 | 补充 4 处缺失的 transitionSessionStatus 调用 |
| `internal/service/session_status_guard_test.go` | 新增 | SessionStatusGuard 单元测试 (3 个用例) |

### ChatOrchestrator 触发点补全详情

| 场景 | 位置 | 新增调用 |
|------|------|----------|
| 首字节超时 | chat_orchestrator_turn.go:910 | `transitionSessionStatus(interrupted, timeout)` |
| 流式错误 | chat_orchestrator_turn.go:917 | `transitionSessionStatus(interrupted, error)` |
| Turn 开始运行 | chat_orchestrator_turn.go:670 | `transitionSessionStatus(running)` |
| Resume await 失败 | chat_orchestrator_turn.go:411 | `transitionSessionStatus(interrupted, error)` |

### Team Turn 触发点（代码中实现，原始文档未记录）

| 场景 | 位置 | 调用 |
|------|------|------|
| Team Turn 开始 | team_turn_hooks.go:53 | `transitionSessionStatus(running)` |
| Team Turn 失败 | team_turn_hooks.go:65 | `transitionSessionStatus(interrupted, error)` |
| Team Turn 完成 | team_turn_hooks.go:74 | `transitionSessionStatus(completed)` |

### Pending Queue 触发点（代码中实现，原始文档未记录）

| 场景 | 位置 | 调用 |
|------|------|------|
| Pending Queue 执行 | chat_orchestrator_turn.go:1076 | `transitionSessionStatus(running)` |
| Pending Queue 失败 | chat_orchestrator_turn.go:1082 | `transitionSessionStatus(interrupted, error)` |
| Pending Queue 完成 | chat_orchestrator_turn.go:1087 | `transitionSessionStatus(completed)` |

### 已有的触发点（确认无需修改）

| 场景 | 位置 | 状态 |
|------|------|------|
| Turn 超时 | chat_orchestrator_turn.go:935 | ✅ 已有（reason=timeout） |
| 空回复 | chat_orchestrator_turn.go:963 | ✅ 已有（reason=error） |
| Turn 完成 | chat_orchestrator_turn.go:1033 | ✅ 已有 |
| 工具需确认 | chat_orchestrator_turn.go:366 | ✅ 已有 |
| Agent 等待回复 | chat_orchestrator_turn.go:368 | ✅ 已有 |
| 确认后恢复运行 | chat_orchestrator_turn.go:375 | ✅ 已有 |
| 预算升级 | chat_orchestrator_session_run.go:236 | ✅ 已有 |
| 用户取消 | chat_orchestrator.go:399 | ✅ 已有 |
