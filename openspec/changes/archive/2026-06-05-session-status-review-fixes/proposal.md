## Why

Session Status Monitoring 变更已归档，但代码审查发现 5 个阻断级问题和 9 个建议级问题：错误码语义错误（BadRequest 应为 Conflict）、超时场景 reason 分类错误、遗留非法 status 值、并发冲突静默吞错、前端 REST API 映射缺失关键字段。这些问题会导致前端 tooltip 永远为空、状态转换错误码误导客户端、并发场景数据不一致。

## What Changes

- **修复错误码语义**：`SessionStatusMachine.TransitionTo` 和 `SessionUsecase.Archive/Delete` 中 `kerrors.BadRequest` 改为 `kerrors.Conflict`
- **修复超时 reason 分类**：首字节超时和 Turn 超时场景使用 `StatusReasonTimeout` 替代 `StatusReasonError`
- **移除非法 status 值**：`DeleteSessionsByAgentID` 不再设置 `status = "deleted"`，仅依赖 `deleted_at` 时间戳
- **修复并发冲突处理**：`TransitionSessionStatus` 在 `n == 0` 时返回错误而非静默 nil
- **修复前端映射缺失**：`kratosSessionToLegacy` 补全 `status_reason` 和 `status_changed_at` 字段映射
- **补全批量 WS 推送**：`BatchTransitionInterrupted` 在状态转换后发布 `session.status_changed` WS 事件
- **Admin Store 监听 WS 事件**：Admin Session Store 注册 `status_changed` 事件监听器
- **清理死代码**：移除 `sessionUi.ts` 中未使用的 `statusBadgeColor` 函数

## Capabilities

### New Capabilities

（无新增能力）

### Modified Capabilities

- `session-status`: 修复错误码语义、超时 reason 分类、并发冲突处理、批量 WS 推送、非法 status 值
- `session-status-frontend`: 修复 REST API 映射缺失、Admin Store WS 监听、清理死代码

## Impact

- **后端 biz 层**：`internal/biz/session/status_machine.go`（错误码）、`internal/biz/session/usecase.go`（错误码）、`internal/biz/session/recovery.go`（WS 推送）
- **后端 data 层**：`internal/data/session_repo.go`（移除非法 status）、`internal/data/session_runtime_repo.go`（并发冲突处理）
- **后端 service 层**：`internal/service/chat_orchestrator_turn.go`（超时 reason）
- **前端 api 层**：`web/src/features/session/api.ts`（映射补全）
- **前端 store 层**：`web/src/stores/session/index.ts`（WS 监听）
- **前端组件层**：`web/src/components/sessions/sessionUi.ts`（死代码清理）
