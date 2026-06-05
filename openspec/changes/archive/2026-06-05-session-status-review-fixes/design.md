## Context

Session Status Monitoring 变更（归档于 `2026-06-05-session-status-monitoring`）已上线，代码审查发现 5 个阻断级和 9 个建议级问题。这些问题集中在：错误码语义、超时 reason 分类、遗留非法 status 值、并发冲突静默处理、前端映射缺失。本变更仅做修复，不引入新功能。

## Goals / Non-Goals

**Goals:**

- 修复所有阻断级问题，确保状态转换错误码语义正确
- 修复前端 REST API 映射缺失，使 `status_reason` / `status_changed_at` 在所有数据路径可用
- 补全批量操作 WS 推送，确保服务启停时前端能感知状态变更
- 清理遗留非法 status 值和死代码

**Non-Goals:**

- 不实现 `ForceTransitionSessionStatus` RPC（属于原二期任务）
- 不修改 Ent Schema 或 Proto
- 不修改 ChatPage 侧边栏（属于二期增强）
- 不修改 Spirit 组件的类型断言（需 Spirit 模块配合）

## Decisions

### D1: 错误码统一为 FailedPrecondition

**决策**：`SessionStatusMachine.TransitionTo` 和 `SessionUsecase.Archive/Delete` 中的非法状态转换/保护状态操作，统一返回 `kerrors.Conflict`。

**理由**：`BadRequest` 语义是"请求格式错误"，而非法状态转换是"前置条件不满足"。`Conflict`（HTTP 409 / gRPC ABORTED）比 `BadRequest` 更准确，客户端可据此做重试或状态刷新。实际代码中使用 `kerrors.Conflict` 而非 `kerrors.FailedPrecondition`，因为 Kratos 的 `Conflict` 映射到 HTTP 409，语义更贴合"状态冲突"。

**替代方案**：保持 `BadRequest` — 被否决，语义不匹配。

### D2: 超时场景使用 StatusReasonTimeout

**决策**：首字节超时和 Turn 超时场景的 `transitionSessionStatus` 调用，reason 从 `StatusReasonError` 改为 `StatusReasonTimeout`。

**理由**：设计规格明确区分 `timeout`（可重试）和 `error`（运行时错误），两者对用户的意义不同。前端 tooltip 显示"执行超时"比"执行出错"更准确。

**影响范围**：`chat_orchestrator_turn.go` 中 3 处调用（L904 首字节超时、L929 Turn 超时、L957 空回复超时场景保持 `error` 因为不是超时）。

### D3: TransitionSessionStatus 并发冲突返回错误

**决策**：`sessionRuntimeRepo.TransitionSessionStatus` 在 `n == 0` 时返回 `kerrors.Conflict`，而非静默返回 nil。

**理由**：`n == 0` 意味着 WHERE 条件中的 `currentStatus` 已被并发修改，转换未生效。调用方需要知道这一点以决策是否重试。静默 nil 会导致数据不一致。

**替代方案**：返回 sentinel error `ErrStatusConflict` — 可行但需额外定义，`kerrors.Conflict` 已足够表达语义。

### D4: DeleteSessionsByAgentID 移除 SetStatus("deleted")

**决策**：`DeleteSessionsByAgentID` 不再设置 `status = "deleted"`，仅设置 `deleted_at` 时间戳。

**理由**：新设计中 `deleted` 不再是合法 status 值，生命周期由 `deleted_at` 判断。设置非法 status 值违反单一真相源原则。

### D5: BatchTransitionInterrupted 补发 WS 事件

**决策**：`BatchTransitionInterrupted` 在每个 session 状态转换成功后，调用 `statusPublisher.PublishSessionStatusChanged` 发布 WS 事件。

**理由**：服务启停时前端需要感知 session 状态变更。当前 `BatchTransitionInterrupted` 直接写 DB 不发 WS，导致前端状态滞后。

**风险**：启动时 WS 连接可能尚未建立，事件可能丢失。但这是可接受的——前端刷新后会从 REST API 拉取最新状态。

### D6: 前端 kratosSessionToLegacy 补全映射

**决策**：在 `kratosSessionToLegacy` 函数中添加 `status_reason` 和 `status_changed_at` 字段映射。

**理由**：这是阻断级前端问题。当前 REST API 返回的 Session 对象中这两个字段永远为空，导致 Badge tooltip 功能失效。

## Risks / Trade-offs

- **[D3 并发冲突错误]** → 调用方需处理 `kerrors.Conflict` 错误。当前 `transitionSessionStatus`（ChatOrchestrator）已做 warn 日志，不影响主流程。`BatchTransitionInterrupted` 中已有 failedCount 计数逻辑。
- **[D5 启动时 WS 事件丢失]** → 可接受。前端有刷新机制兜底。
- **[D2 reason 变更]** → 已中断 session 的 `status_reason` 可能从 `error` 变为 `timeout`。这是正确的语义修正，不影响功能。
