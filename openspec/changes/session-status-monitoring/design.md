# Session 状态监控设计

> 日期：2026-05-31
> 状态：Draft
> 范围：Session 模块执行状态监控，类似 Trae 的执行中/完成/中断/需要确认等状态指示

***

## 一、背景与目标

### 1.1 现状问题

当前 Session 模块存在三层分散的状态体系：

| 层级        | 字段                                       | 值域                                                                                   | 问题                           |
| --------- | ---------------------------------------- | ------------------------------------------------------------------------------------ | ---------------------------- |
| 业务状态      | `sessions.status`                        | active / archived / deleted / running                                                | 混合了生命周期与执行状态                 |
| 运行时状态     | `sessions.state_json` → `runtime.status` | running / completed / failed / cancelled / idle / interactive / escalating / durable | 与 sessions.status 不同步，无单一真相源 |
| Run Phase | `session_runs.phase`                     | interactive / escalating / durable / completed / failed                              | 仅记录 Run 级别，不反映 Session 整体状态  |

**核心问题**：

1. `sessions.status` 的 `running` 与 `state_json.runtime.status` 是两套独立状态，同步不保证
2. 缺少「中断」和「需要确认」等状态的明确建模
3. 中断场景无原因区分，用户不知道为什么中断
4. 前端没有醒目的状态指示器

### 1.2 目标

1. 统一 `sessions.status` 为执行状态的单一真相源
2. 明确建模：idle / running / completed / interrupted / awaiting\_confirmation
3. 中断状态区分原因，告知用户为什么中断
4. 执行中和等待确认的会话禁止删除/归档
5. 前端提供 Trae 风格的醒目状态指示器

***

## 二、方案选型

### 2.1 三种方案对比

| 方案              | 核心思路                                | 优点         | 缺点           |
| --------------- | ----------------------------------- | ---------- | ------------ |
| **A：统一 status** | `sessions.status` 只管执行状态，生命周期改用时间戳  | 单一真相源，查询简单 | 需数据迁移        |
| B：双字段分离         | 保持 `status` + 新增 `execution_status` | 向后兼容       | 永久双字段复杂度     |
| C：复合状态值         | `status` 用点分值如 `active.running`     | 单字段        | 查询不便，Ent 不友好 |

### 2.2 选择：方案 A

理由：

* 用户明确要求「统一到 sessions.status」

* `archived`/`deleted` 本来就有 `archived_at`/`deleted_at` 时间戳，`status` 存它们是冗余的

* 单一真相源意味着前端、WS 推送、删除保护都只看一个字段，逻辑最清晰

* 数据迁移虽有一笔成本，但一次性完成，后续维护成本最低

***

## 三、状态枚举与状态机

### 3.1 执行状态枚举

```go
type SessionStatus string

const (
    SessionStatusIdle                 SessionStatus = "idle"
    SessionStatusRunning              SessionStatus = "running"
    SessionStatusCompleted            SessionStatus = "completed"
    SessionStatusInterrupted          SessionStatus = "interrupted"
    SessionStatusAwaitingConfirmation SessionStatus = "awaiting_confirmation"
)
```

### 3.2 状态原因枚举

```go
type SessionStatusReason string

const (
    // interrupted 子类型
    StatusReasonUserCancelled      SessionStatusReason = "user_cancelled"
    StatusReasonTimeout            SessionStatusReason = "timeout"
    StatusReasonBudgetEscalated    SessionStatusReason = "budget_escalated"
    StatusReasonError              SessionStatusReason = "error"
    StatusReasonContextOverflow    SessionStatusReason = "context_overflow"
    StatusReasonServerShutdown     SessionStatusReason = "server_shutdown"
    StatusReasonUnexpectedShutdown SessionStatusReason = "unexpected_shutdown"
    StatusReasonConfirmationTimeout SessionStatusReason = "confirmation_timeout"

    // awaiting_confirmation 子类型
    StatusReasonToolConfirmation   SessionStatusReason = "tool_confirmation"
    StatusReasonAgentAwaitingReply SessionStatusReason = "agent_awaiting_reply"

    // manual
    StatusReasonManualOverride     SessionStatusReason = "manual_override"
)
```

### 3.3 状态机转换规则

```
                    ┌─────────────────────────────────────────┐
                    │                                         │
                    ▼                                         │
  ┌──────┐  发消息  ┌─────────┐  正常完成  ┌───────────┐      │
  │ idle │───────→│ running │──────────→│ completed │      │
  └──────┘        └────┬────┘           └─────┬─────┘      │
     ▲                 │                      │             │
     │                 │ 取消/超时/错误         │ 发新消息     │
     │                 ▼                      ▼             │
     │          ┌─────────────┐        ┌─────────┐         │
     │          │ interrupted │        │ running │←────────┘
     │          └──────┬──────┘        └────┬────┘
     │                 │                    │
     │                 │ 发新消息            │ 工具需确认/Agent等待
     │                 ▼                    ▼
     │           ┌─────────┐      ┌──────────────────────┐
     │           │ running │      │ awaiting_confirmation │
     │           └─────────┘      └──────┬───────────────┘
     │                                   │
     │           确认/回复 ───────────────┤→ running
     │           取消 ───────────────────┤→ interrupted
     │                                   │
     └───────────────────────────────────┘
```

**合法转换表**：

| 从                      | 到                      | 触发条件        | status\_reason         |
| ---------------------- | ---------------------- | ----------- | ---------------------- |
| idle                   | running                | 用户发消息       | —                      |
| running                | completed              | Runner 正常完成 | —                      |
| running                | interrupted            | 用户取消        | `user_cancelled`       |
| running                | interrupted            | 超时          | `timeout`              |
| running                | interrupted            | 硬预算到达       | `budget_escalated`     |
| running                | interrupted            | 运行时错误       | `error`                |
| running                | interrupted            | 上下文溢出       | `context_overflow`     |
| running                | interrupted            | 服务正常关闭      | `server_shutdown`      |
| running                | interrupted            | 服务异常退出      | `unexpected_shutdown`  |
| running                | awaiting\_confirmation | 工具需确认       | `tool_confirmation`    |
| running                | awaiting\_confirmation | Agent等待回复   | `agent_awaiting_reply` |
| awaiting\_confirmation | running                | 用户确认/回复     | —                      |
| awaiting\_confirmation | interrupted            | 用户取消        | `user_cancelled`       |
| awaiting\_confirmation | interrupted            | 确认超时        | `confirmation_timeout` |
| completed              | running                | 用户发新消息      | —                      |
| interrupted            | running                | 用户发新消息      | —                      |

**非法转换**：任何不在上表中的转换都是非法的，Usecase 层做防御性校验，返回 `kerrors.FailedPrecondition`。

### 3.4 删除/归档保护

```go
var ProtectedStatuses = map[SessionStatus]bool{
    SessionStatusRunning:              true,
    SessionStatusAwaitingConfirmation: true,
}
```

* `status ∈ {running, awaiting_confirmation}` 时，禁止删除、归档、批量删除、批量归档

* 前端在 UI 上禁用删除/归档按钮，并显示原因提示

* 后端返回 `kerrors.FailedPrecondition("SESSION", "session is %s, cannot delete/archive", status)`

***

## 四、数据模型变更

### 4.1 Ent Schema 变更

**`sessions`** **表**：

| 变更 | 字段                  | 类型     | 默认值      | 说明                                                                     |
| -- | ------------------- | ------ | -------- | ---------------------------------------------------------------------- |
| 修改 | `status`            | String | `"idle"` | 值域改为：idle / running / completed / interrupted / awaiting\_confirmation |
| 新增 | `status_reason`     | String | `""`     | 中断原因/等待原因子类型                                                           |
| 新增 | `status_changed_at` | String | `""`     | 状态变更时间（ISO 8601）                                                       |

**废弃**：`state_json` 中的 `runtime.status`、`runtime.error_message`、`runtime.updated_at` 等运行时状态 key，改为写入 `sessions.status` / `sessions.status_reason` / `sessions.status_changed_at`。

**保留不变**：`state_json` 中的其他 key（`runtime.run_id`、`runtime.await_*` 等）继续使用，因为它们不是「状态」，而是「运行时元数据」。

### 4.2 生命周期判断迁移

| 旧判断方式                 | 新判断方式                                  |
| --------------------- | -------------------------------------- |
| `status = 'archived'` | `archived_at != ''`                    |
| `status = 'deleted'`  | `deleted_at != ''`                     |
| `status = 'active'`   | `deleted_at = '' AND archived_at = ''` |
| `status = 'running'`  | `status = 'running'`（不变）               |

**查询示例**：

```sql
-- 活跃会话（未删除未归档）
SELECT * FROM sessions WHERE deleted_at = '' AND archived_at = ''

-- 执行中的会话
SELECT * FROM sessions WHERE status = 'running' AND deleted_at = ''

-- 可删除的会话（非保护状态）
SELECT * FROM sessions
WHERE status NOT IN ('running', 'awaiting_confirmation')
  AND deleted_at = ''
```

### 4.3 索引调整

现有索引 `deleted_at + status` 需要适配新值域。新增索引：

```sql
CREATE INDEX idx_sessions_status ON sessions(status, deleted_at)
  WHERE deleted_at = ''
```

用于快速查询特定执行状态的活跃会话。

### 4.4 Proto 变更

```protobuf
message Session {
  // ...existing fields
  string status = X;             // idle / running / completed / interrupted / awaiting_confirmation
  string status_reason = Y;      // 中断/等待原因子类型
  string status_changed_at = Z;  // 状态变更时间（ISO 8601）
}
```

### 4.5 数据迁移

```sql
-- Step 1: 将 active 改为 idle
UPDATE sessions SET status = 'idle' WHERE status = 'active';

-- Step 2: archived/deleted 的 session 不改 status
-- 后续查询改用 archived_at/deleted_at 时间戳判断生命周期
-- 这些 session 的 status 值不再有意义

-- Step 3: 已有 running 状态的 session 保持不变
-- 进程重启后 SessionStatusGuard 会将 running 标记为 interrupted + unexpected_shutdown
```

***

## 五、后端实现

### 5.1 状态机

**新增** **`internal/biz/session/status_machine.go`**：

```go
type SessionStatusMachine struct {
    status       SessionStatus
    statusReason SessionStatusReason
    changedAt    time.Time
}

func NewSessionStatusMachine(status SessionStatus, reason SessionStatusReason, changedAt string) *SessionStatusMachine
func (m *SessionStatusMachine) TransitionTo(target SessionStatus, reason SessionStatusReason) error
func (m *SessionStatusMachine) CanTransitionTo(target SessionStatus) bool
func (m *SessionStatusMachine) IsProtected() bool
func (m *SessionStatusMachine) Status() SessionStatus
func (m *SessionStatusMachine) StatusReason() SessionStatusReason
```

* `TransitionTo` 校验合法转换表，非法转换返回 `kerrors.FailedPrecondition`

* `IsProtected` 返回 `status ∈ {running, awaiting_confirmation}`

### 5.2 SessionUsecase 变更

| 方法                               | 变更                                                                   |
| -------------------------------- | -------------------------------------------------------------------- |
| `Delete`                         | 调用 `statusMachine.IsProtected()`，保护中返回 `FailedPrecondition`          |
| `Archive`                        | 同上                                                                   |
| `BatchDelete`                    | 过滤掉 protected 状态的 session，返回部分失败结果                                   |
| `BatchArchive`                   | 同上                                                                   |
| `TransitionStatus`               | **新增**，统一的状态转换入口，内部调用 `statusMachine.TransitionTo` + 写 DB + 发布 WS 事件 |
| `BatchTransitionInterrupted`     | **新增**，批量将 running 转为 interrupted（用于优雅退出/异常恢复）                       |
| `RecoverOrphanedRunningSessions` | **新增**，启动时恢复孤儿 running session                                       |

**`SessionMutator`** **子接口变更**：

```go
type SessionMutator interface {
    ArchiveSession(ctx, id) error
    DeleteSession(ctx, id) error
    DeleteSessionsByAgentID(ctx, agentID) error
    PinSession(ctx, id) error
    UnpinSession(ctx, id) error
    TransitionSessionStatus(ctx, id, status, reason) error  // 新增
}
```

### 5.3 ChatOrchestrator 变更

**状态转换触发点**：

| 时机          | 当前行为                                         | 新行为                                                                             |
| ----------- | -------------------------------------------- | ------------------------------------------------------------------------------- |
| Runner 启动   | `persistRunStatus(running)` 写 state\_json    | `uc.TransitionStatus(sessionID, running, "")`                                   |
| Runner 正常完成 | `persistRunStatus(completed)` 写 state\_json  | `uc.TransitionStatus(sessionID, completed, "")`                                 |
| Runner 被取消  | `persistRunStatus(cancelled)` 写 state\_json  | `uc.TransitionStatus(sessionID, interrupted, "user_cancelled")`                 |
| Runner 超时   | `persistRunStatus(failed)` 写 state\_json     | `uc.TransitionStatus(sessionID, interrupted, "timeout")`                        |
| Runner 错误   | `persistRunStatus(failed)` 写 state\_json     | `uc.TransitionStatus(sessionID, interrupted, "error")`                          |
| 预算升级        | `persistRunStatus(durable)` 写 state\_json    | `uc.TransitionStatus(sessionID, interrupted, "budget_escalated")`               |
| 工具需确认       | `setRunStatusWithAwait(tool)` 写 state\_json  | `uc.TransitionStatus(sessionID, awaiting_confirmation, "tool_confirmation")`    |
| Agent等待回复   | `setRunStatusWithAwait(human)` 写 state\_json | `uc.TransitionStatus(sessionID, awaiting_confirmation, "agent_awaiting_reply")` |
| 用户确认/回复     | `setRunStatus(running)` 写 state\_json        | `uc.TransitionStatus(sessionID, running, "")`                                   |

**`persistRunStatus`** **保留但简化**：只写 `runtime.run_id` 等元数据 key，不再写 `runtime.status`。

### 5.4 SessionService 变更

* `DeleteSession` / `ArchiveSession`：调用 `uc.Delete` / `uc.Archive` 前无需额外检查（Usecase 内部已做保护）

* `BatchDeleteSessions` / `BatchArchiveSessions`：同上

* **新增 RPC** `ForceTransitionSessionStatus`（仅 Admin 权限）：

```protobuf
rpc ForceTransitionSessionStatus(ForceTransitionSessionStatusRequest) returns (ForceTransitionSessionStatusResponse) {
  option (google.api.http) = { post: "/v1/sessions/{id}/force-status" body: "*" };
}

message ForceTransitionSessionStatusRequest {
  string id = 1;
  string status = 2;
  string status_reason = 3;  // 默认 "manual_override"
}
```

### 5.5 WS 推送变更

**新增 Envelope 类型**：`session.status_changed`

```json
{
  "type": "session.status_changed",
  "session_id": "xxx",
  "status": "interrupted",
  "status_reason": "user_cancelled",
  "status_changed_at": "2026-05-31T12:00:00Z"
}
```

前端收到后直接更新本地 session 状态，无需重新拉取列表。

### 5.6 优雅退出与异常恢复

**新增** **`SessionStatusGuard`**（`internal/service/session_status_guard.go`）：

注册为 Kratos 生命周期钩子：

```go
type SessionStatusGuard struct {
    uc *biz.SessionUsecase
}

// OnShutdown: 程序正常退出时
func (g *SessionStatusGuard) OnShutdown(ctx context.Context) error {
    // 1. 取消所有活跃 Runner
    // 2. 批量将 status = running 的 session 转为 interrupted + "server_shutdown"
    // 3. awaiting_confirmation 的 session 保持不变（可恢复）
    return g.uc.BatchTransitionInterrupted(ctx, "server_shutdown")
}

// OnStartup: 程序启动时
func (g *SessionStatusGuard) OnStartup(ctx context.Context) error {
    // 查找所有 status = running 的 session
    // 批量转为 interrupted + "unexpected_shutdown"
    return g.uc.RecoverOrphanedRunningSessions(ctx)
}
```

**与现有 Durable Worker 的关系**：现有 `SessionRunDurableWorker.CleanupOrphanedRuns` 清理 `session_runs` 表的孤儿记录。`SessionStatusGuard.OnStartup` 在此基础上额外修复 `sessions.status`，两者配合执行。

### 5.7 确认超时自动清理

复用现有 Durable Worker 的轮询机制，新增清理逻辑：

```go
func (g *SessionStatusGuard) CleanupExpiredConfirmations(ctx context.Context) error {
    // 查找 status = awaiting_confirmation
    //   AND status_changed_at < now - confirmationTimeout(默认24h)
    // 批量转为 interrupted + confirmation_timeout
}
```

***

## 六、边界场景

| 场景             | 处理方式                                                         | 状态转换                                                             |
| -------------- | ------------------------------------------------------------ | ---------------------------------------------------------------- |
| 优雅退出           | `SessionStatusGuard.OnShutdown` 批量中断 running session         | `running` → `interrupted` + `server_shutdown`                    |
| 异常退出恢复         | `SessionStatusGuard.OnStartup` 修复孤儿 running session          | `running` → `interrupted` + `unexpected_shutdown`                |
| 确认超时           | 定时清理超过 24h 的 awaiting\_confirmation                          | `awaiting_confirmation` → `interrupted` + `confirmation_timeout` |
| Agent Transfer | Agent 间转移时 session 保持 running，不触发状态转换                        | 无转换                                                              |
| Team 多 Agent   | Team session 的 status 反映整体执行状态，任一子 Agent 在执行即为 running       | 与单 Agent 一致                                                      |
| LLM 限流         | 限流属于瞬时错误，Runner 内部重试，不改变 session status                      | 无转换                                                              |
| 上下文溢出          | 压缩失败且上下文超限时                                                  | `running` → `interrupted` + `context_overflow`                   |
| 手动状态修复         | Admin API 提供强制状态覆盖（仅用于运维排障）                                  | `任意` → `目标` + `manual_override`                                  |
| 长时间空闲          | idle 超过保留天数后的自动归档（现有逻辑）                                      | 由 `archived_at` 判断，status 不变                                     |
| 会话恢复引导         | interrupted / awaiting\_confirmation 的 session 在前端显示「继续对话」入口 | 用户点击 → 发消息 → `running`                                           |
| 并发 Run 防护      | 同一 session 不允许并发 Run（现有 `session_lock.go` 已保证）               | 无需额外处理                                                           |

***

## 七、前端实现

### 7.1 类型定义变更

**`features/session/types.ts`**：

```typescript
export type SessionStatus =
  | 'idle'
  | 'running'
  | 'completed'
  | 'interrupted'
  | 'awaiting_confirmation'

export type SessionStatusReason =
  | 'user_cancelled'
  | 'timeout'
  | 'budget_escalated'
  | 'error'
  | 'context_overflow'
  | 'server_shutdown'
  | 'unexpected_shutdown'
  | 'confirmation_timeout'
  | 'tool_confirmation'
  | 'agent_awaiting_reply'
  | 'manual_override'

export interface Session {
  // ...existing fields
  status: SessionStatus
  statusReason: SessionStatusReason
  statusChangedAt: string
}
```

### 7.2 状态指示器组件

**新增** **`components/session/SessionStatusBadge.vue`**：

| 状态                      | 图标   | 颜色                    | 文案   |
| ----------------------- | ---- | --------------------- | ---- |
| `idle`                  | ○    | 灰色                    | 空闲   |
| `running`               | ⟳ 旋转 | 强调色（`--color-accent`） | 执行中  |
| `completed`             | ✓    | 绿色                    | 已完成  |
| `interrupted`           | ✕    | 橙色/警告色                | 已中断  |
| `awaiting_confirmation` | ⏸    | 蓝色/信息色                | 等待确认 |

**中断原因文案映射**：

| status\_reason         | 显示文案         |
| ---------------------- | ------------ |
| `user_cancelled`       | 用户取消         |
| `timeout`              | 执行超时         |
| `budget_escalated`     | 预算超限，已升级后台执行 |
| `error`                | 执行出错         |
| `context_overflow`     | 上下文溢出        |
| `server_shutdown`      | 服务关闭         |
| `unexpected_shutdown`  | 服务异常退出       |
| `confirmation_timeout` | 确认超时         |

**悬停提示**：Badge 悬停时显示完整信息，如「已中断 · 执行超时 · 3 分钟前」。

### 7.3 删除保护 UI

**SessionsPage 变更**：

1. **单选删除**：`status ∈ {running, awaiting_confirmation}` 时，删除按钮 disabled + tooltip 提示「会话正在执行中，无法删除」
2. **批量删除**：勾选 protected 状态的 session 时，批量操作栏显示「N 个会话正在执行中，无法删除」
3. **删除确认弹窗**：对非 protected 的 session 也增加状态提示

**ChatPage 变更**：

1. 侧边栏 session 列表中，running/awaiting\_confirmation 的 session 显示状态 Badge
2. 右键菜单中，protected 状态的 session 禁用删除选项

### 7.4 WS 推送处理

**`sessionSync.ts`** **变更**：

```typescript
onSessionMutation('session.status_changed', (payload) => {
  const { session_id, status, status_reason, status_changed_at } = payload
  sessionStore.patchSessionStatus(session_id, status, status_reason, status_changed_at)
})
```

### 7.5 中断/等待恢复引导

**`interrupted`** **状态**：session 列表项显示「继续对话」按钮，点击后进入聊天页面并发消息。

**`awaiting_confirmation`** **状态**：

* `tool_confirmation`：在聊天页面显示工具确认弹窗（确认/拒绝）

* `agent_awaiting_reply`：在聊天页面显示输入框提示「Agent 正在等待你的回复」

***

## 八、分阶段落地

### 一期（核心）

| #    | 任务                                                                                                   | 涉及模块                                                             |
| ---- | ---------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| 1.1  | Ent Schema 新增 `status_reason`、`status_changed_at` 字段，修改 `status` 默认值                                 | `internal/data/ent/schema/session.go`                            |
| 1.2  | 新增 `SessionStatusMachine`                                                                            | `internal/biz/session/status_machine.go`                         |
| 1.3  | `SessionUsecase` 新增 `TransitionStatus`、`BatchTransitionInterrupted`、`RecoverOrphanedRunningSessions` | `internal/biz/session/usecase.go`                                |
| 1.4  | `SessionMutator` 新增 `TransitionSessionStatus`                                                        | `internal/biz/session/repo.go` + `internal/data/session_repo.go` |
| 1.5  | `ChatOrchestrator` 状态转换触发点改造                                                                         | `internal/service/chat_orchestrator*.go`                         |
| 1.6  | 删除/归档保护逻辑                                                                                            | `internal/biz/session/usecase.go`                                |
| 1.7  | `SessionStatusGuard` 优雅退出 + 异常恢复                                                                     | `internal/service/session_status_guard.go`                       |
| 1.8  | Proto 变更 + `make api`                                                                                | `api/kratos/session/v1/`                                         |
| 1.9  | WS Envelope `session.status_changed`                                                                 | `internal/service/` + `web/src/`                                 |
| 1.10 | 数据迁移脚本                                                                                               | `internal/data/migration/`                                       |
| 1.11 | 前端类型 + SessionStatusBadge + 删除保护 UI                                                                  | `web/src/`                                                       |
| 1.12 | Wire 装配更新                                                                                            | `cmd/admin/wire.go`                                              |

### 二期（增强）

| #   | 任务                | 涉及模块                                       |
| --- | ----------------- | ------------------------------------------ |
| 2.1 | 确认超时自动清理（CronJob） | `internal/service/session_status_guard.go` |
| 2.2 | Admin 强制状态覆盖 RPC  | `api/` + `internal/service/session.go`     |
| 2.3 | 中断会话恢复引导 UI       | `web/src/`                                 |
| 2.4 | 工具确认弹窗 UI         | `web/src/`                                 |
| 2.5 | Agent 等待回复提示 UI   | `web/src/`                                 |

***

## 九、变更影响速查

| 修改内容                      | 影响范围                                              | 需同步更新                                                                                                 |
| ------------------------- | ------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| `sessions.status` 值域变更    | Data 层查询、Service 层映射、前端类型                         | `internal/data/session_repo.go` + `internal/service/session.go` + `web/src/features/session/types.ts` |
| 新增 `status_reason` 字段     | Ent Schema → Data → Biz → Service → Proto → 前端    | 全栈                                                                                                    |
| 新增 `status_changed_at` 字段 | 同上                                                | 全栈                                                                                                    |
| `SessionMutator` 新增方法     | Data 层实现                                          | `internal/data/session_repo.go`                                                                       |
| `ChatOrchestrator` 状态转换改造 | Runner 生命周期管理                                     | `internal/service/chat_orchestrator*.go`                                                              |
| WS Envelope 新增类型          | 前端 dispatcher + Store                             | `web/src/stores/sessionSync.ts`                                                                       |
| 生命周期判断迁移                  | 所有使用 `status = 'active'/'archived'/'deleted'` 的查询 | `internal/data/session_repo.go` + 前端过滤逻辑                                                              |

***

## 十、验证计划

| 验证项     | 方法                                                                                                    |
| ------- | ----------------------------------------------------------------------------------------------------- |
| 状态机合法转换 | 单测 `TestSessionStatusMachine_TransitionTo`                                                            |
| 非法转换拒绝  | 单测 `TestSessionStatusMachine_InvalidTransition`                                                       |
| 删除保护    | 单测 `TestSessionUsecase_DeleteProtected`                                                               |
| 优雅退出    | 集成测试：启动 → 发消息 → 优雅退出 → 验证 status = interrupted + server\_shutdown                                     |
| 异常恢复    | 集成测试：插入 running session → 重启 → 验证 status = interrupted + unexpected\_shutdown                         |
| 确认超时    | 单测 `TestSessionStatusGuard_CleanupExpiredConfirmations`                                               |
| WS 推送   | 前端手动验证：发消息 → 观察 status badge 变化                                                                       |
| 数据迁移    | 集成测试：旧数据 → 执行迁移 → 验证新值域                                                                               |
| 前端删除保护  | 手动验证：running session 删除按钮 disabled                                                                    |
| 全量      | `make api && make wire && make build && make test && make lint` + `cd web && pnpm lint && pnpm build` |

