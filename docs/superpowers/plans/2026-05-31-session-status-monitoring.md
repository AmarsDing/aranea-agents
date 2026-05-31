# Session 状态监控 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 统一 `sessions.status` 为执行状态单一真相源，增加 idle/running/completed/interrupted/awaiting_confirmation 状态枚举、状态机、删除保护、优雅退出/异常恢复、前端状态指示器。

**Architecture:** `sessions.status` 列从混合生命周期+执行状态改为纯执行状态枚举，生命周期改用 `archived_at`/`deleted_at` 时间戳判断。新增 `status_reason` + `status_changed_at` 字段。`SessionStatusMachine` 校验合法转换。`SessionStatusGuard` 处理优雅退出和异常恢复。前端 `SessionStatusBadge` 组件显示状态。

**Tech Stack:** Go + Ent ORM + Kratos v2 + Wire DI + Vue 3 + Quasar + Pinia + TypeScript

---

## File Structure

| 操作 | 文件 | 职责 |
|------|------|------|
| Create | `internal/biz/session/status.go` | SessionStatus / SessionStatusReason 常量定义 |
| Create | `internal/biz/session/status_machine.go` | SessionStatusMachine 状态机 |
| Create | `internal/biz/session/status_machine_test.go` | 状态机单测 |
| Create | `internal/service/session_status_guard.go` | 优雅退出 + 异常恢复 |
| Create | `internal/service/session_status_guard_test.go` | Guard 单测 |
| Create | `web/src/components/session/SessionStatusBadge.vue` | 前端状态指示器 |
| Modify | `internal/data/ent/schema/session.go` | 新增字段 + 修改默认值 |
| Modify | `internal/biz/session/usecase.go` | 新增 TransitionStatus + 保护逻辑 |
| Modify | `internal/biz/session/repo.go` | 新增 SessionStatusTransitioner 子接口 |
| Modify | `internal/data/session_repo.go` | 实现 TransitionSessionStatus + 修改 Archive/Delete |
| Modify | `internal/data/session_repo_batch.go` | 修改批量操作 WHERE 条件 |
| Modify | `internal/service/chat_orchestrator.go` | 状态转换触发点 |
| Modify | `internal/service/chat_orchestrator_turn.go` | 状态转换触发点 |
| Modify | `internal/service/run_status_store.go` | 简化 persistRunStatus |
| Modify | `internal/service/session.go` | Proto 映射变更 |
| Modify | `api/kratos/session/v1/session.proto` | 新增字段 |
| Modify | `web/src/features/session/types.ts` | 前端类型定义 |
| Modify | `web/src/stores/chat/sessionStore.ts` | patchSessionStatus 方法 |
| Modify | `web/src/stores/sessionSync.ts` | 新增 status_changed 事件 |
| Modify | `cmd/admin/wire.go` | Wire 装配更新 |

---

### Task 1: 定义状态常量与类型

**Files:**
- Create: `internal/biz/session/status.go`

- [ ] **Step 1: 创建 status.go，定义 SessionStatus 和 SessionStatusReason 常量**

```go
package session

type SessionStatus string

const (
	SessionStatusIdle                 SessionStatus = "idle"
	SessionStatusRunning              SessionStatus = "running"
	SessionStatusCompleted            SessionStatus = "completed"
	SessionStatusInterrupted          SessionStatus = "interrupted"
	SessionStatusAwaitingConfirmation SessionStatus = "awaiting_confirmation"
)

type SessionStatusReason string

const (
	StatusReasonUserCancelled       SessionStatusReason = "user_cancelled"
	StatusReasonTimeout             SessionStatusReason = "timeout"
	StatusReasonBudgetEscalated     SessionStatusReason = "budget_escalated"
	StatusReasonError               SessionStatusReason = "error"
	StatusReasonContextOverflow     SessionStatusReason = "context_overflow"
	StatusReasonServerShutdown      SessionStatusReason = "server_shutdown"
	StatusReasonUnexpectedShutdown  SessionStatusReason = "unexpected_shutdown"
	StatusReasonConfirmationTimeout SessionStatusReason = "confirmation_timeout"

	StatusReasonToolConfirmation   SessionStatusReason = "tool_confirmation"
	StatusReasonAgentAwaitingReply SessionStatusReason = "agent_awaiting_reply"

	StatusReasonManualOverride SessionStatusReason = "manual_override"
)

func IsProtectedStatus(s SessionStatus) bool {
	return s == SessionStatusRunning || s == SessionStatusAwaitingConfirmation
}
```

- [ ] **Step 2: 验证编译通过**

Run: `go build ./internal/biz/session/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/biz/session/status.go
git commit -m "feat(session): define SessionStatus and SessionStatusReason constants"
```

---

### Task 2: 实现 SessionStatusMachine

**Files:**
- Create: `internal/biz/session/status_machine.go`
- Create: `internal/biz/session/status_machine_test.go`

- [ ] **Step 1: 编写状态机单测**

```go
package session

import (
	"testing"
)

func TestSessionStatusMachine_TransitionTo_ValidTransitions(t *testing.T) {
	tests := []struct {
		from   SessionStatus
		to     SessionStatus
		reason SessionStatusReason
	}{
		{SessionStatusIdle, SessionStatusRunning, ""},
		{SessionStatusRunning, SessionStatusCompleted, ""},
		{SessionStatusRunning, SessionStatusInterrupted, StatusReasonUserCancelled},
		{SessionStatusRunning, SessionStatusInterrupted, StatusReasonTimeout},
		{SessionStatusRunning, SessionStatusInterrupted, StatusReasonError},
		{SessionStatusRunning, SessionStatusInterrupted, StatusReasonBudgetEscalated},
		{SessionStatusRunning, SessionStatusInterrupted, StatusReasonContextOverflow},
		{SessionStatusRunning, SessionStatusInterrupted, StatusReasonServerShutdown},
		{SessionStatusRunning, SessionStatusInterrupted, StatusReasonUnexpectedShutdown},
		{SessionStatusRunning, SessionStatusAwaitingConfirmation, StatusReasonToolConfirmation},
		{SessionStatusRunning, SessionStatusAwaitingConfirmation, StatusReasonAgentAwaitingReply},
		{SessionStatusAwaitingConfirmation, SessionStatusRunning, ""},
		{SessionStatusAwaitingConfirmation, SessionStatusInterrupted, StatusReasonUserCancelled},
		{SessionStatusAwaitingConfirmation, SessionStatusInterrupted, StatusReasonConfirmationTimeout},
		{SessionStatusCompleted, SessionStatusRunning, ""},
		{SessionStatusInterrupted, SessionStatusRunning, ""},
	}
	for _, tt := range tests {
		m := NewSessionStatusMachine(tt.from, "", "")
		if err := m.TransitionTo(tt.to, tt.reason); err != nil {
			t.Errorf("TransitionTo(%s→%s) should succeed, got error: %v", tt.from, tt.to, err)
		}
		if m.Status() != tt.to {
			t.Errorf("after transition, status = %s, want %s", m.Status(), tt.to)
		}
		if tt.reason != "" && m.StatusReason() != tt.reason {
			t.Errorf("after transition, reason = %s, want %s", m.StatusReason(), tt.reason)
		}
	}
}

func TestSessionStatusMachine_TransitionTo_InvalidTransitions(t *testing.T) {
	tests := []struct {
		from SessionStatus
		to   SessionStatus
	}{
		{SessionStatusIdle, SessionStatusCompleted},
		{SessionStatusIdle, SessionStatusInterrupted},
		{SessionStatusIdle, SessionStatusAwaitingConfirmation},
		{SessionStatusCompleted, SessionStatusCompleted},
		{SessionStatusCompleted, SessionStatusInterrupted},
		{SessionStatusInterrupted, SessionStatusCompleted},
		{SessionStatusInterrupted, SessionStatusInterrupted},
		{SessionStatusRunning, SessionStatusIdle},
		{SessionStatusRunning, SessionStatusRunning},
	}
	for _, tt := range tests {
		m := NewSessionStatusMachine(tt.from, "", "")
		if err := m.TransitionTo(tt.to, ""); err == nil {
			t.Errorf("TransitionTo(%s→%s) should fail, but succeeded", tt.from, tt.to)
		}
	}
}

func TestSessionStatusMachine_IsProtected(t *testing.T) {
	if IsProtectedStatus(SessionStatusIdle) {
		t.Error("idle should not be protected")
	}
	if !IsProtectedStatus(SessionStatusRunning) {
		t.Error("running should be protected")
	}
	if !IsProtectedStatus(SessionStatusAwaitingConfirmation) {
		t.Error("awaiting_confirmation should be protected")
	}
	if IsProtectedStatus(SessionStatusCompleted) {
		t.Error("completed should not be protected")
	}
	if IsProtectedStatus(SessionStatusInterrupted) {
		t.Error("interrupted should not be protected")
	}
}

func TestSessionStatusMachine_CanTransitionTo(t *testing.T) {
	m := NewSessionStatusMachine(SessionStatusIdle, "", "")
	if !m.CanTransitionTo(SessionStatusRunning) {
		t.Error("idle should be able to transition to running")
	}
	if m.CanTransitionTo(SessionStatusCompleted) {
		t.Error("idle should not be able to transition to completed")
	}
}
```

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/biz/session/ -run TestSessionStatusMachine -count=1`
Expected: FAIL（NewSessionStatusMachine 未定义）

- [ ] **Step 3: 实现状态机**

```go
package session

import (
	"fmt"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

var validTransitions = map[SessionStatus][]SessionStatus{
	SessionStatusIdle:                 {SessionStatusRunning},
	SessionStatusRunning:              {SessionStatusCompleted, SessionStatusInterrupted, SessionStatusAwaitingConfirmation},
	SessionStatusCompleted:            {SessionStatusRunning},
	SessionStatusInterrupted:          {SessionStatusRunning},
	SessionStatusAwaitingConfirmation: {SessionStatusRunning, SessionStatusInterrupted},
}

type SessionStatusMachine struct {
	status       SessionStatus
	statusReason SessionStatusReason
	changedAt    string
}

func NewSessionStatusMachine(status SessionStatus, reason SessionStatusReason, changedAt string) *SessionStatusMachine {
	return &SessionStatusMachine{
		status:       status,
		statusReason: reason,
		changedAt:    changedAt,
	}
}

func (m *SessionStatusMachine) TransitionTo(target SessionStatus, reason SessionStatusReason) error {
	if !m.CanTransitionTo(target) {
		return kerrors.FailedPrecondition("SESSION", fmt.Sprintf("cannot transition session status from %s to %s", m.status, target))
	}
	m.status = target
	m.statusReason = reason
	return nil
}

func (m *SessionStatusMachine) CanTransitionTo(target SessionStatus) bool {
	allowed, ok := validTransitions[m.status]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == target {
			return true
		}
	}
	return false
}

func (m *SessionStatusMachine) Status() SessionStatus        { return m.status }
func (m *SessionStatusMachine) StatusReason() SessionStatusReason { return m.statusReason }
func (m *SessionStatusMachine) ChangedAt() string            { return m.changedAt }
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/biz/session/ -run TestSessionStatusMachine -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/biz/session/status_machine.go internal/biz/session/status_machine_test.go
git commit -m "feat(session): implement SessionStatusMachine with validation"
```

---

### Task 3: Ent Schema 变更

**Files:**
- Modify: `internal/data/ent/schema/session.go`

- [ ] **Step 1: 修改 session.go schema**

在 `status` 字段行，将默认值从 `"active"` 改为 `"idle"`：

```go
field.String("status").Default("idle"),
```

在 `status` 字段后新增两个字段：

```go
field.String("status_reason").Default(""),
field.String("status_changed_at").Default(""),
```

- [ ] **Step 2: 运行 go generate 重新生成 Ent 代码**

Run: `cd f:\aranea-agents && go generate ./internal/data/ent/...`
Expected: PASS，生成新的 Ent 代码包含 `StatusReason` 和 `StatusChangedAt` 字段

- [ ] **Step 3: 验证编译通过**

Run: `go build ./internal/data/...`
Expected: PASS（可能有其他文件引用旧字段需要适配，先记录错误）

- [ ] **Step 4: Commit**

```bash
git add internal/data/ent/
git commit -m "feat(session): add status_reason, status_changed_at fields and change status default to idle"
```

---

### Task 4: 新增 SessionStatusTransitioner 子接口 + Data 层实现

**Files:**
- Modify: `internal/biz/session/usecase.go`（repo 接口定义部分）
- Modify: `internal/data/session_repo.go`

> 注意：`SessionMutator` 已有 5 个方法（红线 #15），不新增方法。创建独立的 `SessionStatusTransitioner` 子接口。

- [ ] **Step 1: 在 usecase.go 的接口定义区域新增 SessionStatusTransitioner**

在 `SessionMutator` 接口后新增：

```go
type SessionStatusTransitioner interface {
	TransitionSessionStatus(ctx context.Context, id string, status string, reason string) error
}
```

在 `SessionRepo` 聚合接口中新增嵌入：

```go
type SessionRepo interface {
	SessionReader
	SessionWriter
	SessionMutator
	SessionBatchMutator
	SessionStatusTransitioner
	// ... 其余不变
}
```

在 `SessionUsecase` struct 中新增字段：

```go
type SessionUsecase struct {
	// ... 现有字段不变
	statusTransitioner SessionStatusTransitioner
}
```

修改 `NewSessionUsecase` 构造函数，从 `sessions SessionRepo` 中提取 `statusTransitioner`（因为 SessionRepo 已嵌入该接口，直接赋值 `sessions` 即可）：

```go
func NewSessionUsecase(sessions SessionRepo, ...) *SessionUsecase {
	uc := &SessionUsecase{
		// ... 现有赋值不变
		statusTransitioner: sessions,
	}
	// ...
}
```

- [ ] **Step 2: 在 session_repo.go 中实现 TransitionSessionStatus**

```go
func (r *sessionRepo) TransitionSessionStatus(ctx context.Context, id string, status string, reason string) error {
	c := r.txClient(ctx)
	now := nowRFC3339()
	_, err := c.Session.Update().
		Where(entsession.IDEQ(id), entsession.DeletedAtEQ("")).
		SetStatus(status).
		SetStatusReason(reason).
		SetStatusChangedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	return err
}
```

- [ ] **Step 3: 修改 ArchiveSession 和 DeleteSession 的 WHERE 条件**

将 `entsession.StatusNEQ("running")` 改为排除所有保护状态：

```go
func (r *sessionRepo) ArchiveSession(ctx context.Context, id string) (int, error) {
	c := r.txClient(ctx)
	now := nowRFC3339()
	n, err := c.Session.Update().
		Where(
			entsession.IDEQ(id),
			entsession.DeletedAtEQ(""),
			entsession.StatusNEQ(string(session.SessionStatusRunning)),
			entsession.StatusNEQ(string(session.SessionStatusAwaitingConfirmation)),
			entsession.ArchivedAtEQ(""),
		).
		SetArchivedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	return n, err
}
```

```go
func (r *sessionRepo) DeleteSession(ctx context.Context, id string) (int, error) {
	c := r.txClient(ctx)
	now := nowRFC3339()
	n, err := c.Session.Update().
		Where(
			entsession.IDEQ(id),
			entsession.DeletedAtEQ(""),
			entsession.StatusNEQ(string(session.SessionStatusRunning)),
			entsession.StatusNEQ(string(session.SessionStatusAwaitingConfirmation)),
		).
		SetDeletedAt(now).
		SetUpdatedAt(now).
		Save(ctx)
	if n > 0 {
		_, _ = NewChannelPeerSessionRepo(r.data).DeleteBySessionID(ctx, id)
	}
	return n, nil
}
```

> 注意：ArchiveSession 不再 `SetStatus("archived")`，DeleteSession 不再 `SetStatus("deleted")`。生命周期由 `archived_at`/`deleted_at` 时间戳判断。

- [ ] **Step 4: 修改 RestoreSession**

```go
func (r *sessionRepo) RestoreSession(ctx context.Context, id string) (biz.Session, error) {
	c := r.txClient(ctx)
	now := nowRFC3339()
	_, err := c.Session.Update().
		Where(entsession.IDEQ(id)).
		SetStatus(string(session.SessionStatusIdle)).
		SetStatusReason("").
		SetArchivedAt("").
		SetDeletedAt("").
		SetUpdatedAt(now).
		Save(ctx)
	// ... 后续读取逻辑不变
}
```

- [ ] **Step 5: 验证编译通过**

Run: `go build ./internal/data/... ./internal/biz/session/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/biz/session/usecase.go internal/data/session_repo.go
git commit -m "feat(session): add SessionStatusTransitioner interface and data layer impl"
```

---

### Task 5: 修改批量操作 WHERE 条件

**Files:**
- Modify: `internal/data/session_repo_batch.go`

- [ ] **Step 1: 修改 batchUpdateWheres 函数**

```go
func batchUpdateWheres(mode string, chunk []string) []predicate.Session {
	wheres := []predicate.Session{
		entsession.IDIn(chunk...),
		entsession.DeletedAtEQ(""),
		entsession.StatusNEQ(string(session.SessionStatusRunning)),
		entsession.StatusNEQ(string(session.SessionStatusAwaitingConfirmation)),
	}
	if mode == "archive" {
		wheres = append(wheres, entsession.ArchivedAtEQ(""))
	}
	return wheres
}
```

- [ ] **Step 2: 修改 ArchiveSessionsByIDs 和 DeleteSessionsByIDs**

ArchiveSessionsByIDs 不再 `SetStatus("archived")`：

```go
func (r *sessionRepo) ArchiveSessionsByIDs(ctx context.Context, ids []string) (int, []string, error) {
	return r.batchUpdateSessions(ctx, ids, "archive", func(upd *ent.SessionUpdate, now string) *ent.SessionUpdate {
		return upd.SetArchivedAt(now).SetUpdatedAt(now)
	})
}
```

DeleteSessionsByIDs 不再 `SetStatus("deleted")`：

```go
func (r *sessionRepo) DeleteSessionsByIDs(ctx context.Context, ids []string) (int, []string, error) {
	return r.batchUpdateSessions(ctx, ids, "delete", func(upd *ent.SessionUpdate, now string) *ent.SessionUpdate {
		return upd.SetDeletedAt(now).SetUpdatedAt(now)
	})
}
```

- [ ] **Step 3: 验证编译通过**

Run: `go build ./internal/data/...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/data/session_repo_batch.go
git commit -m "feat(session): update batch operations to use new status protection"
```

---

### Task 6: SessionUsecase 新增 TransitionStatus + 保护逻辑

**Files:**
- Modify: `internal/biz/session/usecase.go`

- [ ] **Step 1: 新增 TransitionStatus 方法**

```go
func (uc *SessionUsecase) TransitionStatus(ctx context.Context, id string, targetStatus session.SessionStatus, reason session.SessionStatusReason) error {
	sess, err := uc.sessionReader.GetSessionByID(ctx, id)
	if err != nil {
		return err
	}
	m := session.NewSessionStatusMachine(session.SessionStatus(sess.Status), session.SessionStatusReason(sess.StatusReason), sess.StatusChangedAt)
	if err := m.TransitionTo(targetStatus, reason); err != nil {
		return err
	}
	return uc.statusTransitioner.TransitionSessionStatus(ctx, id, string(targetStatus), string(reason))
}
```

- [ ] **Step 2: 修改 Archive 方法，使用 IsProtectedStatus**

```go
func (uc *SessionUsecase) Archive(ctx context.Context, id string) error {
	sess, err := uc.sessionReader.GetSessionByID(ctx, id)
	if err != nil {
		return err
	}
	if session.IsProtectedStatus(session.SessionStatus(sess.Status)) {
		return kerrors.FailedPrecondition("SESSION", fmt.Sprintf("session is %s, cannot archive", sess.Status))
	}
	n, err := uc.sessionMutator.ArchiveSession(ctx, id)
	if n == 0 {
		return kerrors.NotFound("SESSION", id)
	}
	return err
}
```

- [ ] **Step 3: 修改 Delete 方法，使用 IsProtectedStatus**

```go
func (uc *SessionUsecase) Delete(ctx context.Context, id string) error {
	sess, err := uc.sessionReader.GetSessionByID(ctx, id)
	if err != nil {
		return err
	}
	if session.IsProtectedStatus(session.SessionStatus(sess.Status)) {
		return kerrors.FailedPrecondition("SESSION", fmt.Sprintf("session is %s, cannot delete", sess.Status))
	}
	n, err := uc.sessionMutator.DeleteSession(ctx, id)
	if n == 0 {
		return kerrors.NotFound("SESSION", id)
	}
	return err
}
```

- [ ] **Step 4: 新增 BatchTransitionInterrupted 方法**

```go
func (uc *SessionUsecase) BatchTransitionInterrupted(ctx context.Context, reason session.SessionStatusReason) error {
	sessions, err := uc.sessionReader.ListSessionsForBatch(ctx, biz.SessionBatchFilter{
		Status: string(session.SessionStatusRunning),
	})
	if err != nil {
		return err
	}
	for _, s := range sessions {
		_ = uc.TransitionStatus(ctx, s.ID, session.SessionStatusInterrupted, reason)
	}
	return nil
}
```

- [ ] **Step 5: 新增 RecoverOrphanedRunningSessions 方法**

```go
func (uc *SessionUsecase) RecoverOrphanedRunningSessions(ctx context.Context) error {
	return uc.BatchTransitionInterrupted(ctx, session.StatusReasonUnexpectedShutdown)
}
```

- [ ] **Step 6: 修改 CreateSession 默认 status**

找到 `Create` 方法中设置 `Status` 的地方，将 `"active"` 改为 `string(session.SessionStatusIdle)`。

- [ ] **Step 7: 验证编译通过**

Run: `go build ./internal/biz/...`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add internal/biz/session/usecase.go
git commit -m "feat(session): add TransitionStatus, protection logic, and recovery methods"
```

---

### Task 7: 修改搜索/查询中的 status 过滤

**Files:**
- Modify: `internal/data/session_repo_batch.go`（sessionSearchWheres 函数）
- Modify: `internal/biz/session/usecase.go`（Search 方法中的 status 过滤逻辑）

- [ ] **Step 1: 修改 sessionSearchWheres 中的 status 过滤**

找到 `sessionSearchWheres` 函数中对 `status` 的过滤逻辑。将 `status = 'active'` 的判断改为 `archived_at = '' AND deleted_at = ''`，将 `status = 'archived'` 改为 `archived_at != ''`，将 `status = 'deleted'` 改为 `deleted_at != ''`。

具体修改取决于当前代码结构，需要读取 `sessionSearchWheres` 函数的完整实现后精确修改。

- [ ] **Step 2: 验证编译通过**

Run: `go build ./internal/data/...`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/data/session_repo_batch.go
git commit -m "refactor(session): update search filters to use timestamps for lifecycle"
```

---

### Task 8: ChatOrchestrator 状态转换触发点改造

**Files:**
- Modify: `internal/service/chat_orchestrator.go`
- Modify: `internal/service/chat_orchestrator_turn.go`
- Modify: `internal/service/run_status_store.go`

> 这是改动面最大的 Task。核心思路：每次 `persistRunStatus` 或 `setRunStatus` 调用时，额外调用 `uc.TransitionStatus`。`persistRunStatus` 保留但只写元数据 key。

- [ ] **Step 1: 在 ChatOrchestrator 中注入 SessionUsecase**

在 `ChatOrchestrator` struct 中新增字段：

```go
sessionUC *biz.SessionUsecase
```

在构造函数中注入。需要修改 `ChatOrchestratorDeps` 和 `NewChatOrchestrator`。

- [ ] **Step 2: 修改 persistRunStatus 调用点**

在每个 `persistRunStatus(ctx, sessionID, runID, status, errMsg)` 调用后，根据 status 值调用 `uc.TransitionStatus`：

| persistRunStatus 的 status | TransitionStatus 调用 |
|---|---|
| `"running"` | `TransitionStatus(ctx, sessionID, SessionStatusRunning, "")` |
| `"completed"` | `TransitionStatus(ctx, sessionID, SessionStatusCompleted, "")` |
| `"failed"` | `TransitionStatus(ctx, sessionID, SessionStatusInterrupted, StatusReasonError)` |
| `"cancelled"` | `TransitionStatus(ctx, sessionID, SessionStatusInterrupted, StatusReasonUserCancelled)` |

- [ ] **Step 3: 修改 setRunStatusWithAwait 调用点**

在 `setRunStatusWithAwait` 调用后，根据 awaitKind 调用 `TransitionStatus`：

| awaitKind | TransitionStatus 调用 |
|---|---|
| `"tool"` | `TransitionStatus(ctx, sessionID, SessionStatusAwaitingConfirmation, StatusReasonToolConfirmation)` |
| `"human"` | `TransitionStatus(ctx, sessionID, SessionStatusAwaitingConfirmation, StatusReasonAgentAwaitingReply)` |

- [ ] **Step 4: 修改预算升级触发点**

在 `escalateSessionRunToDurable` 中，取消 Runner 后调用：

```go
o.sessionUC.TransitionStatus(ctx, sessionID, session.SessionStatusInterrupted, session.StatusReasonBudgetEscalated)
```

- [ ] **Step 5: 简化 persistRunStatusToSession**

从 `persistRunStatusToSession` 中移除 `runtime.status`、`runtime.error_message`、`runtime.updated_at` 的写入，只保留 `runtime.run_id` 等元数据 key。

- [ ] **Step 6: 验证编译通过**

Run: `go build ./internal/service/...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/service/chat_orchestrator*.go internal/service/run_status_store.go
git commit -m "feat(session): wire ChatOrchestrator status transitions to SessionUsecase"
```

---

### Task 9: SessionStatusGuard — 优雅退出 + 异常恢复

**Files:**
- Create: `internal/service/session_status_guard.go`
- Create: `internal/service/session_status_guard_test.go`

- [ ] **Step 1: 编写 Guard 单测**

```go
package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/session"
)

type mockSessionUsecase struct {
	mock.Mock
}

func (m *mockSessionUsecase) TransitionStatus(ctx context.Context, id string, status session.SessionStatus, reason session.SessionStatusReason) error {
	args := m.Called(ctx, id, status, reason)
	return args.Error(0)
}

func (m *mockSessionUsecase) RecoverOrphanedRunningSessions(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func (m *mockSessionUsecase) BatchTransitionInterrupted(ctx context.Context, reason session.SessionStatusReason) error {
	args := m.Called(ctx, reason)
	return args.Error(0)
}

func TestSessionStatusGuard_OnStartup(t *testing.T) {
	uc := new(mockSessionUsecase)
	uc.On("RecoverOrphanedRunningSessions", mock.Anything).Return(nil)
	g := &SessionStatusGuard{uc: uc}
	err := g.OnStartup(context.Background())
	assert.NoError(t, err)
	uc.AssertExpectations(t)
}

func TestSessionStatusGuard_OnShutdown(t *testing.T) {
	uc := new(mockSessionUsecase)
	uc.On("BatchTransitionInterrupted", mock.Anything, session.StatusReasonServerShutdown).Return(nil)
	g := &SessionStatusGuard{uc: uc}
	err := g.OnShutdown(context.Background())
	assert.NoError(t, err)
	uc.AssertExpectations(t)
}
```

> 注意：mock 需要根据 SessionUsecase 的实际方法签名调整，可能需要 mock 更多方法。上面的 mock 仅包含核心方法，实际实现时需要补全。

- [ ] **Step 2: 运行测试确认失败**

Run: `go test ./internal/service/ -run TestSessionStatusGuard -count=1`
Expected: FAIL

- [ ] **Step 3: 实现 SessionStatusGuard**

```go
package service

import (
	"context"

	"aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"
)

type SessionStatusGuard struct {
	uc *biz.SessionUsecase
	lg loggateway.Logger
}

func NewSessionStatusGuard(uc *biz.SessionUsecase, lg loggateway.Logger) *SessionStatusGuard {
	return &SessionStatusGuard{uc: uc, lg: lg}
}

func (g *SessionStatusGuard) OnStartup(ctx context.Context) error {
	g.lg.Info("session status guard: recovering orphaned running sessions")
	if err := g.uc.RecoverOrphanedRunningSessions(ctx); err != nil {
		g.lg.Error("session status guard: failed to recover orphaned sessions", "error", err)
		return err
	}
	return nil
}

func (g *SessionStatusGuard) OnShutdown(ctx context.Context) error {
	g.lg.Info("session status guard: transitioning running sessions to interrupted on shutdown")
	if err := g.uc.BatchTransitionInterrupted(ctx, session.StatusReasonServerShutdown); err != nil {
		g.lg.Error("session status guard: failed to transition sessions on shutdown", "error", err)
		return err
	}
	return nil
}
```

- [ ] **Step 4: 运行测试确认通过**

Run: `go test ./internal/service/ -run TestSessionStatusGuard -count=1`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/session_status_guard.go internal/service/session_status_guard_test.go
git commit -m "feat(session): add SessionStatusGuard for graceful shutdown and startup recovery"
```

---

### Task 10: Proto 变更 + make api

**Files:**
- Modify: `api/kratos/session/v1/session.proto`

- [ ] **Step 1: 在 Session message 中新增字段**

在 `Session` message 的 `status` 字段后新增：

```protobuf
string status_reason = N;      // 下一个可用字段号
string status_changed_at = M;   // 下一个可用字段号
```

> 需要读取当前 proto 文件确定下一个可用字段号。

- [ ] **Step 2: 运行 make api 重新生成 proto 代码**

Run: `make api`
Expected: PASS

- [ ] **Step 3: 验证编译通过**

Run: `go build ./api/... ./internal/service/...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add api/ internal/service/
git commit -m "feat(session): add status_reason and status_changed_at to proto"
```

---

### Task 11: SessionService Proto 映射变更

**Files:**
- Modify: `internal/service/session.go`

- [ ] **Step 1: 修改 toProtoSession 映射函数**

在 `toProtoSession` 中新增 `StatusReason` 和 `StatusChangedAt` 字段映射：

```go
StatusReason:     s.StatusReason,
StatusChangedAt:  s.StatusChangedAt,
```

- [ ] **Step 2: 修改 fromProtoCreateSession / fromProtoUpdateSession**

确保创建/更新时不直接设置 `status`（status 由状态机管理，不允许用户直接设置）。

- [ ] **Step 3: 验证编译通过**

Run: `go build ./internal/service/...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/service/session.go
git commit -m "feat(session): update SessionService proto mapping for new status fields"
```

---

### Task 12: WS Envelope session.status_changed

**Files:**
- Modify: `internal/service/chat_orchestrator.go`（或 WS 发布相关文件）
- Modify: `web/src/stores/sessionSync.ts`
- Modify: `web/src/stores/chat/sessionStore.ts`

- [ ] **Step 1: 后端发布 session.status_changed Envelope**

在 `TransitionStatus` 成功后，发布 WS Envelope。具体实现取决于当前 WS 发布机制（`PublishRunStatusFull` 或类似函数）。

在 `SessionUsecase.TransitionStatus` 中，状态转换成功后发布：

```go
// 发布 WS 事件
// envelope type: "session.status_changed"
// payload: { session_id, status, status_reason, status_changed_at }
```

> 需要读取当前 WS 发布代码确定具体 API。

- [ ] **Step 2: 前端 sessionSync.ts 新增 status_changed 事件类型**

在 `SessionMutation` 类型中新增：

```typescript
| { type: 'status_changed'; id: string; status: SessionStatus; statusReason: SessionStatusReason; statusChangedAt: string }
```

- [ ] **Step 3: 前端 sessionStore.ts 新增 patchSessionStatus 方法**

```typescript
function patchSessionStatus(id: string, status: SessionStatus, statusReason: SessionStatusReason, statusChangedAt: string) {
  const idx = sessions.value.findIndex(s => s.id === id)
  if (idx !== -1) {
    sessions.value[idx] = { ...sessions.value[idx], status, statusReason, statusChangedAt }
  }
  if (selectedSession.value?.id === id) {
    selectedSession.value = { ...selectedSession.value, status, statusReason, statusChangedAt }
  }
}
```

- [ ] **Step 4: 在 sessionSync handler 中处理 status_changed**

```typescript
case 'status_changed':
  patchSessionStatus(mutation.id, mutation.status, mutation.statusReason, mutation.statusChangedAt)
  break
```

- [ ] **Step 5: 验证前端编译通过**

Run: `cd web && pnpm build`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/service/ web/src/stores/ web/src/features/session/
git commit -m "feat(session): add session.status_changed WS envelope and frontend handling"
```

---

### Task 13: 前端类型 + SessionStatusBadge + 删除保护 UI

**Files:**
- Modify: `web/src/features/session/types.ts`
- Create: `web/src/components/session/SessionStatusBadge.vue`
- Modify: `web/src/features/session/useSessionsPage.ts`（或相关 composable）
- Modify: `web/src/components/session/SessionsTableSection.vue`（或相关组件）

- [ ] **Step 1: 更新 types.ts**

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
  | ''

export interface Session {
  // ...existing fields
  status: SessionStatus
  status_reason: SessionStatusReason
  status_changed_at: string
}
```

- [ ] **Step 2: 创建 SessionStatusBadge.vue**

组件接收 `status` 和 `statusReason` props，根据状态显示不同图标、颜色和文案。悬停时显示完整信息。

> 具体实现遵循项目 Quasar 组件风格，使用 `QBadge` 或自定义样式。参考现有 `components/session/` 目录下的组件模式。

- [ ] **Step 3: 在 SessionsTableSection 中集成 SessionStatusBadge**

在表格的 status 列中使用 `SessionStatusBadge` 替代纯文本。

- [ ] **Step 4: 修改删除保护逻辑**

在删除按钮的 `disable` 条件中，将 `status === 'running'` 改为：

```typescript
const isProtected = status === 'running' || status === 'awaiting_confirmation'
```

- [ ] **Step 5: 修改归档按钮的禁用逻辑**

同样将归档按钮的禁用条件扩展到包含 `awaiting_confirmation`。

- [ ] **Step 6: 修改生命周期判断**

将前端所有 `status === 'archived'` 改为 `!!session.archived_at`，`status === 'deleted'` 改为 `!!session.deleted_at`，`status === 'active'` 改为 `!session.archived_at && !session.deleted_at`。

- [ ] **Step 7: 验证前端编译通过**

Run: `cd web && pnpm lint && pnpm build`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add web/src/
git commit -m "feat(session): add SessionStatusBadge component and delete protection UI"
```

---

### Task 14: Wire 装配更新

**Files:**
- Modify: `internal/service/service.go`（ProviderSet）
- Modify: `cmd/admin/wire.go`

- [ ] **Step 1: 在 service.ProviderSet 中注册 SessionStatusGuard**

```go
var ProviderSet = wire.NewSet(
	// ...existing
	NewSessionStatusGuard,
)
```

- [ ] **Step 2: 在 wire.go 中注入 SessionStatusGuard 到 Kratos 生命周期**

在 `newApp` 或 `wireApp` 中注册 `SessionStatusGuard` 的 `OnStartup` / `OnShutdown` 为生命周期钩子。

- [ ] **Step 3: 运行 make wire**

Run: `make wire`
Expected: PASS

- [ ] **Step 4: 验证全量编译通过**

Run: `go build ./cmd/admin`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/service/service.go cmd/admin/wire.go cmd/admin/wire_gen.go
git commit -m "feat(session): register SessionStatusGuard in Wire and lifecycle"
```

---

### Task 15: 数据迁移脚本

**Files:**
- Create: `internal/data/migration/session_status.go`

- [ ] **Step 1: 编写迁移函数**

```go
package migration

import (
	"database/sql"
)

func MigrateSessionStatus(db *sql.DB) error {
	_, err := db.Exec(`UPDATE sessions SET status = 'idle' WHERE status = 'active'`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE sessions SET status_reason = '' WHERE status_reason IS NULL`)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE sessions SET status_changed_at = '' WHERE status_changed_at IS NULL`)
	return err
}
```

> 具体迁移方式取决于项目现有的迁移框架。如果项目使用 Ent 的 auto-migration，则 Ent 会自动添加新列，只需处理 `active → idle` 的数据迁移。

- [ ] **Step 2: 在应用启动时调用迁移**

在 `internal/data/data.go` 的 `NewData` 或初始化函数中调用迁移。

- [ ] **Step 3: 验证迁移**

启动应用，检查数据库中 `sessions` 表的 `status` 值是否从 `active` 变为 `idle`。

- [ ] **Step 4: Commit**

```bash
git add internal/data/migration/
git commit -m "feat(session): add data migration for session status"
```

---

### Task 16: 全量验证

- [ ] **Step 1: 后端全量验证**

Run: `make api && make wire && make build && make test && make lint`
Expected: ALL PASS

- [ ] **Step 2: 前端全量验证**

Run: `cd web && pnpm lint && pnpm test && pnpm build`
Expected: ALL PASS

- [ ] **Step 3: 手动集成测试**

1. 启动应用
2. 创建新 session → 验证 `status = idle`
3. 发消息 → 验证 `status = running`
4. 正常完成 → 验证 `status = completed`
5. 尝试删除 running session → 验证返回错误
6. 优雅退出 → 验证 running session 变为 `interrupted + server_shutdown`
7. 重启 → 验证孤儿 running session 变为 `interrupted + unexpected_shutdown`

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "feat(session): complete session status monitoring implementation"
```
