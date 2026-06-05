# Session 状态监控 — 任务清单

> **状态**: ✅ 已完成
> **完成日期**: 2026-06-02
> **复查日期**: 2026-06-05（文档与代码对齐）

---

## 实施总结

### 已完成的任务

| Task | 描述 | 状态 |
|------|------|------|
| Task 1 | 定义 SessionStatus / SessionStatusReason 常量 + `IsProtectedStatus()` 独立函数 (`internal/biz/session/status.go`) | ✅ |
| Task 2 | 实现 SessionStatusMachine + 单测 (`status_machine.go` + `status_machine_test.go`)，`ChangedAt()` 返回 ISO 8601 字符串 | ✅ |
| Task 3 | Ent Schema 变更：status 默认值 idle + 新增 status_reason / status_changed_at | ✅ |
| Task 4 | `SessionRuntimeWriter.TransitionSessionStatus` 子接口 + Data 层实现（含 TOCTOU WHERE 条件 + 并发冲突返回 `kerrors.Conflict`）+ Archive/Delete 保护 | ✅ |
| Task 5 | 批量操作 WHERE 条件更新 (`session_repo_batch.go`) | ✅ |
| Task 6 | SessionUsecase 新增 TransitionStatus + 保护逻辑 + BatchTransitionInterrupted（含 WS 推送）+ RecoverOrphanedRunning | ✅ |
| Task 7 | 搜索/查询中的 status 过滤迁移 (active→archived_at/deleted_at) | ✅ |
| Task 8 | ChatOrchestrator 状态转换触发点改造（含 Team Turn hooks + Spirit Team + Pending Queue） | ✅ |
| Task 9 | SessionStatusGuard 优雅退出 + 异常恢复（含 Team 恢复 + 编排恢复）+ 测试 | ✅ |
| Task 10 | Proto 变更 + make api (status_reason=48, status_changed_at=49) | ✅ |
| Task 11 | SessionService Proto 映射变更 (toProtoSession 映射 StatusReason + StatusChangedAt) | ✅ |
| Task 12 | WS Envelope `session.status_changed` + Admin Store + Chat Store patchSessionStatus | ✅ |
| Task 13 | 前端类型 + SessionStatusBadge (`components/sessions/`) + 删除保护 UI | ✅ |
| Task 14 | Wire 装配更新 (ProvideSessionStatusPublisher + 生命周期注册) | ✅ |
| Task 15 | 数据迁移脚本 (active→idle + NULL defaults) | ✅ |
| Task 16 | 全量验证 | ✅ |

### 代码实际实现与原始设计的差异

| 差异点 | 原始设计 | 实际实现 | 原因 |
|--------|----------|----------|------|
| `IsProtected` 位置 | `SessionStatusMachine.IsProtected()` 方法 | `IsProtectedStatus()` 独立函数（`status.go`） | 无需构造 Machine 即可判断保护状态，更轻量 |
| `TransitionSessionStatus` 接口 | `SessionMutator` 子接口 | `SessionRuntimeWriter` 子接口（`metrics_repo.go`） | 该方法使用 WHERE currentStatus 做 TOCTOU 防护，属于运行时管理域 |
| `changedAt` 类型 | `time.Time` | `string`（ISO 8601） | 与 DB 字段和 Proto 字段一致，避免反复序列化 |
| `ChangedAt()` 方法 | 未提及 | `SessionStatusMachine.ChangedAt() string` | TransitionTo 后需获取新的时间戳用于 DB 写入和 WS 推送 |
| SessionStatusGuard 构造参数 | 仅 `*biz.SessionUsecase` | `*biz.SessionUsecase` + `*biz.TeamUsecase` + `biz.TaskOrchestratorPort` + `loggateway.Logger` | 需恢复 Team 和编排任务，需注入 Logger |
| OnStartup 恢复范围 | 仅 Session | Session + Team + 编排任务 | 完整恢复需要覆盖所有运行时实体 |
| BatchTransitionInterrupted | 不发 WS 事件 | 每个成功转换后发布 WS 事件 | 前端需感知服务启停时的状态变更 |
| 前端 Badge 路径 | `components/session/` | `components/sessions/`（复数） | 与现有 sessions 组件目录一致 |
| 前端 Badge awaiting_confirmation 颜色 | 蓝色/信息色 | accent（与 running 一致） | Quasar 主题下 accent 更醒目 |
| Chat Store WS 监听 | 未提及 | `stores/chat/sessionStore.ts` 也监听 `status_changed` | 聊天页面需实时更新 session 状态 |

### ChatOrchestrator 触发点完整清单

| 场景 | 位置 | reason |
|------|------|--------|
| Turn 开始运行 | chat_orchestrator_turn.go:670 | — |
| Turn 超时 | chat_orchestrator_turn.go:935 | `timeout` |
| 首字节超时 | chat_orchestrator_turn.go:910 | `timeout` |
| 流式错误 | chat_orchestrator_turn.go:917 | `error` |
| 空回复 | chat_orchestrator_turn.go:963 | `error` |
| Resume await 失败 | chat_orchestrator_turn.go:411 | `error` |
| Turn 完成 | chat_orchestrator_turn.go:1033 | — |
| 工具需确认 | chat_orchestrator_turn.go:366 | `tool_confirmation` |
| Agent 等待回复 | chat_orchestrator_turn.go:368 | `agent_awaiting_reply` |
| 确认后恢复运行 | chat_orchestrator_turn.go:375 | — |
| 预算升级 | chat_orchestrator_session_run.go:236 | `budget_escalated` |
| 用户取消 | chat_orchestrator.go:400 | `user_cancelled` |
| Pending Queue 执行开始 | chat_orchestrator_turn.go:1076 | — |
| Pending Queue 执行失败 | chat_orchestrator_turn.go:1082 | `error` |
| Pending Queue 执行完成 | chat_orchestrator_turn.go:1087 | — |
| Team Turn 开始 | team_turn_hooks.go:53 | — |
| Team Turn 失败 | team_turn_hooks.go:65 | `error` |
| Team Turn 完成 | team_turn_hooks.go:74 | — |
| Spirit Team 错误 | spirit_team.go:101 | `error` |
