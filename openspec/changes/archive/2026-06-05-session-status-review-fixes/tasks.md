## 1. 后端错误码修复

- [x] 1.1 `internal/biz/session/status_machine.go`: `TransitionTo` 方法中 `kerrors.BadRequest` 改为 `kerrors.Conflict`，更新对应单测验证返回 409
- [x] 1.2 `internal/biz/session/usecase.go`: `Archive` 和 `Delete` 方法中保护状态返回的 `kerrors.BadRequest` 改为 `kerrors.Conflict`
- [x] 1.3 运行 `go test ./internal/biz/session/... -count=1` 验证错误码修复

## 2. 后端超时 reason 分类修复

- [x] 2.1 `internal/service/chat_orchestrator_turn.go`: 首字节超时场景（~L904）`StatusReasonError` 改为 `StatusReasonTimeout`
- [x] 2.2 `internal/service/chat_orchestrator_turn.go`: Turn 超时场景（~L929）`StatusReasonError` 改为 `StatusReasonTimeout`
- [x] 2.3 确认空回复场景（~L957）和流式错误场景（~L911）保持 `StatusReasonError` 不变
- [x] 2.4 运行 `go build` 验证编译通过

## 3. 后端非法 status 值修复

- [x] 3.1 `internal/data/session_repo.go`: `DeleteSessionsByAgentID` 方法移除 `SetStatus("deleted")` 和 `SetStatusReason("manual_override")` 和 `SetStatusChangedAt(now)`，仅保留 `SetDeletedAt(now)`
- [x] 3.2 `internal/data/session_repo.go`: `CreateSession` 中 `in.Status == ""` 的默认值从硬编码 `"idle"` 改为 `string(session.SessionStatusIdle)`
- [x] 3.3 运行 `go build` 验证编译通过

## 4. 后端并发冲突处理修复

- [x] 4.1 `internal/data/session_runtime_repo.go`: `TransitionSessionStatus` 在 `n == 0` 时返回 `kerrors.Conflict("SESSION", "session status was concurrently modified")` 而非 nil
- [x] 4.2 确认 `ChatOrchestrator.transitionSessionStatus` 已有 warn 日志处理错误，无需额外修改
- [x] 4.3 确认 `BatchTransitionInterrupted` 已有 failedCount 计数逻辑，无需额外修改
- [x] 4.4 运行 `go build` 验证编译通过

## 5. 后端批量 WS 推送补全

- [x] 5.1 `internal/biz/session/recovery.go`: `BatchTransitionInterrupted` 在每个 session 成功转换后调用 `uc.statusPublisher.PublishSessionStatusChanged(sessionID, interrupted, reasonStr, changedAt)`
- [x] 5.2 添加 nil 检查：`if uc.statusPublisher != nil` 后再调用
- [x] 5.3 运行 `go test ./internal/biz/session/... -count=1` 验证

## 6. 前端映射修复

- [x] 6.1 `web/src/features/session/api.ts`: `kratosSessionToLegacy` 函数添加 `status_reason: (s.statusReason ?? '') as SessionStatusReason` 和 `status_changed_at: s.statusChangedAt ?? ''`
- [x] 6.2 验证 `web/src/features/session/types.ts` 中 `Session` 接口已包含 `statusReason` 和 `statusChangedAt` 字段
- [x] 6.3 运行 `pnpm build` 验证编译通过

## 7. 前端 Admin Store WS 监听

- [x] 7.1 `web/src/stores/session/index.ts`: 注册 `onSessionMutation` 监听器，处理 `status_changed` 事件
- [x] 7.2 添加 `patchSessionStatus` 方法到 Admin Store
- [x] 7.3 运行 `pnpm build` 验证编译通过

## 8. 前端死代码清理

- [x] 8.1 `web/src/components/sessions/sessionUi.ts`: 删除 `statusBadgeColor` 函数
- [x] 8.2 全局搜索确认无 `statusBadgeColor` 引用
- [x] 8.3 运行 `pnpm build` 验证编译通过

## 9. 全量验证

- [x] 9.1 后端：`go test ./internal/biz/session/... -count=1` 通过
- [x] 9.2 后端：`go vet ./internal/biz/session/` 通过
- [x] 9.3 前端：`pnpm build` 通过
