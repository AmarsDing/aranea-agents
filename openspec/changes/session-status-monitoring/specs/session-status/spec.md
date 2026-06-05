# Session Status — Backend Spec

## MODIFIED Requirements

### Requirement: Session Status Enum

`sessions.status` SHALL be the single source of truth for session execution state. The status enum SHALL consist of exactly five values:

| Value | Meaning |
|-------|---------|
| `idle` | Session exists but no Runner is active |
| `running` | A Runner is actively executing |
| `completed` | Runner finished normally |
| `interrupted` | Runner was stopped before normal completion |
| `awaiting_confirmation` | Runner paused, waiting for user input (tool confirmation or agent reply) |

The `status` field default value SHALL be `"idle"`. Legacy values `active`, `archived`, `deleted` SHALL no longer be valid status values.

#### Scenario: New session defaults to idle

WHEN a new session is created
THEN `sessions.status` SHALL be `"idle"`

#### Scenario: Legacy active status migrated to idle

WHEN a session has `status = 'active'` before migration
THEN the migration script SHALL set `status = 'idle'`

#### Scenario: Invalid status value rejected

WHEN code attempts to set `sessions.status` to a value not in the enum
THEN the system SHALL reject the operation

---

### Requirement: Session Status Reason Enum

A `status_reason` field SHALL provide sub-type information for `interrupted` and `awaiting_confirmation` statuses.

**Interrupted sub-types:**

| Value | Meaning |
|-------|---------|
| `user_cancelled` | User explicitly cancelled execution |
| `timeout` | Execution exceeded time limit |
| `budget_escalated` | Hard budget reached, escalated to background |
| `error` | Runtime error occurred |
| `context_overflow` | Context window exceeded and compression failed |
| `server_shutdown` | Server graceful shutdown |
| `unexpected_shutdown` | Server crashed or was killed |
| `confirmation_timeout` | Confirmation wait exceeded timeout |

**Awaiting confirmation sub-types:**

| Value | Meaning |
|-------|---------|
| `tool_confirmation` | Tool execution requires user approval |
| `agent_awaiting_reply` | Agent is waiting for user reply |

**Manual override:**

| Value | Meaning |
|-------|---------|
| `manual_override` | Status was changed via admin ForceTransition RPC |

`status_reason` SHALL be empty string (`""`) when the status is `idle`, `running`, or `completed`. It SHALL be populated when the status is `interrupted` or `awaiting_confirmation`.

#### Scenario: Running session interrupted with reason

WHEN a running session is interrupted due to timeout
THEN `status` SHALL be `"interrupted"` and `status_reason` SHALL be `"timeout"`

#### Scenario: Idle session has empty reason

WHEN a session is in `idle` status
THEN `status_reason` SHALL be `""`

#### Scenario: Awaiting confirmation with tool reason

WHEN a running session pauses for tool confirmation
THEN `status` SHALL be `"awaiting_confirmation"` and `status_reason` SHALL be `"tool_confirmation"`

---

### Requirement: Status Changed At Timestamp

A `status_changed_at` field SHALL record the ISO 8601 timestamp of the last status transition. It SHALL be updated on every status change. Default value SHALL be empty string (`""`).

#### Scenario: Status change updates timestamp

WHEN a session transitions from `idle` to `running`
THEN `status_changed_at` SHALL be set to the current ISO 8601 timestamp

#### Scenario: New session has empty timestamp

WHEN a new session is created
THEN `status_changed_at` SHALL be `""`

#### Scenario: Consecutive transitions update timestamp

WHEN a session transitions from `running` to `interrupted` at time T1, then from `interrupted` to `running` at time T2
THEN `status_changed_at` SHALL be T2

---

### Requirement: Session Status State Machine

The `SessionStatusMachine` SHALL enforce legal status transitions. Any transition not in the legal transition table SHALL be rejected with `kerrors.FailedPrecondition`.

**Legal transition table:**

| From | To | Trigger | status_reason |
|------|----|---------|---------------|
| idle | running | User sends message | — |
| running | completed | Runner finishes normally | — |
| running | interrupted | User cancels | `user_cancelled` |
| running | interrupted | Timeout | `timeout` |
| running | interrupted | Hard budget reached | `budget_escalated` |
| running | interrupted | Runtime error | `error` |
| running | interrupted | Context overflow | `context_overflow` |
| running | interrupted | Server graceful shutdown | `server_shutdown` |
| running | interrupted | Server unexpected shutdown | `unexpected_shutdown` |
| running | awaiting_confirmation | Tool needs confirmation | `tool_confirmation` |
| running | awaiting_confirmation | Agent awaits reply | `agent_awaiting_reply` |
| awaiting_confirmation | running | User confirms/replies | — |
| awaiting_confirmation | interrupted | User cancels | `user_cancelled` |
| awaiting_confirmation | interrupted | Confirmation timeout | `confirmation_timeout` |
| completed | running | User sends new message | — |
| interrupted | running | User sends new message | — |

The `SessionStatusMachine` SHALL provide:
- `NewSessionStatusMachine(status, reason, changedAt)` — construct from current state
- `TransitionTo(target, reason)` — validate and apply transition
- `CanTransitionTo(target)` — check if transition is legal without applying
- `IsProtected()` — return true if status is `running` or `awaiting_confirmation`
- `Status()` — return current status
- `StatusReason()` — return current reason

#### Scenario: Legal transition idle to running

WHEN a session in `idle` status transitions to `running`
THEN the transition SHALL succeed and `status` SHALL be `"running"`

#### Scenario: Illegal transition idle to completed

WHEN a session in `idle` status attempts to transition to `completed`
THEN the transition SHALL be rejected with `FailedPrecondition`

#### Scenario: Illegal transition completed to interrupted

WHEN a session in `completed` status attempts to transition to `interrupted`
THEN the transition SHALL be rejected with `FailedPrecondition`

#### Scenario: Running to interrupted with reason

WHEN a session in `running` status transitions to `interrupted` with reason `timeout`
THEN the transition SHALL succeed, `status` SHALL be `"interrupted"`, and `status_reason` SHALL be `"timeout"`

#### Scenario: Awaiting confirmation back to running

WHEN a session in `awaiting_confirmation` status transitions to `running`
THEN the transition SHALL succeed and `status` SHALL be `"running"`

#### Scenario: Completed to running on new message

WHEN a session in `completed` status transitions to `running`
THEN the transition SHALL succeed and `status` SHALL be `"running"`

#### Scenario: IsProtected returns true for running

WHEN a session has status `running`
THEN `IsProtected()` SHALL return `true`

#### Scenario: IsProtected returns true for awaiting_confirmation

WHEN a session has status `awaiting_confirmation`
THEN `IsProtected()` SHALL return `true`

#### Scenario: IsProtected returns false for idle

WHEN a session has status `idle`
THEN `IsProtected()` SHALL return `false`

---

### Requirement: Delete and Archive Protection

Sessions with protected statuses (`running`, `awaiting_confirmation`) SHALL NOT be deleted or archived. The backend SHALL return `kerrors.FailedPrecondition` with message indicating the session is in a protected status and cannot be deleted/archived.

For batch operations (`BatchDelete`, `BatchArchive`), the system SHALL skip protected sessions and return a partial failure result indicating which sessions were skipped and why.

#### Scenario: Delete protected running session rejected

WHEN a delete request is made for a session with `status = 'running'`
THEN the system SHALL return `FailedPrecondition` and the session SHALL NOT be deleted

#### Scenario: Delete protected awaiting_confirmation session rejected

WHEN a delete request is made for a session with `status = 'awaiting_confirmation'`
THEN the system SHALL return `FailedPrecondition` and the session SHALL NOT be deleted

#### Scenario: Archive protected session rejected

WHEN an archive request is made for a session with `status = 'running'`
THEN the system SHALL return `FailedPrecondition` and the session SHALL NOT be archived

#### Scenario: Batch delete skips protected sessions

WHEN a batch delete request includes sessions with mixed statuses (some running, some idle)
THEN the system SHALL delete non-protected sessions and return partial failure for protected ones

#### Scenario: Delete idle session succeeds

WHEN a delete request is made for a session with `status = 'idle'`
THEN the system SHALL delete the session successfully

---

### Requirement: Lifecycle Determination by Timestamps

Session lifecycle (active/archived/deleted) SHALL be determined by `archived_at` and `deleted_at` timestamps, NOT by the `status` field.

| Lifecycle | Determination |
|-----------|--------------|
| Active | `deleted_at = '' AND archived_at = ''` |
| Archived | `archived_at != ''` |
| Deleted | `deleted_at != ''` |

The `status` field SHALL only represent execution state. Legacy values `active`, `archived`, `deleted` in the `status` field SHALL be eliminated.

#### Scenario: Active session determined by timestamps

WHEN a session has `deleted_at = ''` and `archived_at = ''`
THEN the session SHALL be considered active regardless of `status` value

#### Scenario: Archived session determined by timestamp

WHEN a session has `archived_at != ''`
THEN the session SHALL be considered archived regardless of `status` value

#### Scenario: Deleted session determined by timestamp

WHEN a session has `deleted_at != ''`
THEN the session SHALL be considered deleted regardless of `status` value

---

## ADDED Requirements

### Requirement: Ent Schema Changes

The `sessions` Ent schema SHALL be modified as follows:

1. `status` field: value domain changed to `idle / running / completed / interrupted / awaiting_confirmation`, default value `"idle"`
2. `status_reason` field: new `String` field, default `""`, stores interruption/await reason sub-type
3. `status_changed_at` field: new `String` field, default `""`, stores ISO 8601 timestamp of last status change

The `state_json` field's `runtime.status`, `runtime.error_message`, and `runtime.updated_at` keys SHALL be deprecated. Other `state_json` keys (`runtime.run_id`, `runtime.await_*`) SHALL be preserved as they represent runtime metadata, not execution state.

#### Scenario: New session schema fields

WHEN a new session is created through Ent
THEN `status` SHALL be `"idle"`, `status_reason` SHALL be `""`, and `status_changed_at` SHALL be `""`

#### Scenario: Status field accepts new enum values

WHEN `status` is set to any of `idle`, `running`, `completed`, `interrupted`, `awaiting_confirmation`
THEN Ent SHALL accept the value without error

#### Scenario: Runtime status in state_json deprecated

WHEN ChatOrchestrator updates session state
THEN it SHALL NOT write `runtime.status` to `state_json`; it SHALL write to `sessions.status` instead

---

### Requirement: Session Status Machine Implementation

A `SessionStatusMachine` struct SHALL be implemented in `internal/biz/session/status_machine.go`. It SHALL encapsulate the current status, reason, and changed_at, and enforce the legal transition table.

#### Scenario: Construct machine from existing state

WHEN `NewSessionStatusMachine("running", "timeout", "2026-05-31T12:00:00Z")` is called
THEN a machine SHALL be created with status `running`, reason `timeout`, and changedAt from the given timestamp

#### Scenario: TransitionTo applies valid transition

WHEN `TransitionTo("interrupted", "error")` is called on a machine with status `running`
THEN the machine's status SHALL become `interrupted` and reason SHALL become `error`

#### Scenario: TransitionTo rejects invalid transition

WHEN `TransitionTo("idle", "")` is called on a machine with status `running`
THEN the method SHALL return `kerrors.FailedPrecondition` and the machine state SHALL remain unchanged

#### Scenario: CanTransitionTo returns correct result

WHEN `CanTransitionTo("completed")` is called on a machine with status `running`
THEN it SHALL return `true`

WHEN `CanTransitionTo("idle")` is called on a machine with status `running`
THEN it SHALL return `false`

---

### Requirement: SessionUsecase TransitionStatus

`SessionUsecase` SHALL provide a `TransitionStatus(ctx, sessionID, targetStatus, reason)` method as the unified entry point for status transitions. This method SHALL:

1. Load the session from the repository
2. Construct a `SessionStatusMachine` from current state
3. Call `TransitionTo(target, reason)` to validate
4. Persist the new status, reason, and changed_at via `SessionMutator.TransitionSessionStatus`
5. Publish a `session.status_changed` WS event

#### Scenario: TransitionStatus performs full transition

WHEN `TransitionStatus(ctx, "session-1", "running", "")` is called on an idle session
THEN the session's status SHALL be updated to `running` in the database, `status_changed_at` SHALL be updated, and a `session.status_changed` WS event SHALL be published

#### Scenario: TransitionStatus rejects invalid transition

WHEN `TransitionStatus(ctx, "session-1", "completed", "")` is called on an idle session
THEN the method SHALL return `FailedPrecondition` and no database write or WS event SHALL occur

---

### Requirement: SessionUsecase BatchTransitionInterrupted

`SessionUsecase` SHALL provide a `BatchTransitionInterrupted(ctx, reason)` method that transitions all sessions with `status = 'running'` to `interrupted` with the given reason. This is used for graceful shutdown and startup recovery.

#### Scenario: Batch transition on graceful shutdown

WHEN `BatchTransitionInterrupted(ctx, "server_shutdown")` is called
THEN all sessions with `status = 'running'` SHALL be transitioned to `interrupted` with reason `server_shutdown`

#### Scenario: No running sessions to transition

WHEN `BatchTransitionInterrupted(ctx, "server_shutdown")` is called and no sessions have `status = 'running'`
THEN the method SHALL return without error

---

### Requirement: SessionUsecase RecoverOrphanedRunningSessions

`SessionUsecase` SHALL provide a `RecoverOrphanedRunningSessions(ctx)` method that finds all sessions with `status = 'running'` at startup and transitions them to `interrupted` with reason `unexpected_shutdown`.

#### Scenario: Orphaned running sessions recovered on startup

WHEN `RecoverOrphanedRunningSessions(ctx)` is called and sessions with `status = 'running'` exist
THEN those sessions SHALL be transitioned to `interrupted` with reason `unexpected_shutdown`

#### Scenario: No orphaned sessions

WHEN `RecoverOrphanedRunningSessions(ctx)` is called and no sessions have `status = 'running'`
THEN the method SHALL return without error

---

### Requirement: SessionMutator TransitionSessionStatus

The `SessionMutator` interface SHALL add a new method:

```go
TransitionSessionStatus(ctx context.Context, id string, status SessionStatus, reason SessionStatusReason) error
```

The data layer implementation SHALL update `status`, `status_reason`, and `status_changed_at` in a single database write.

#### Scenario: TransitionSessionStatus persists all fields

WHEN `TransitionSessionStatus(ctx, "session-1", "interrupted", "timeout")` is called
THEN the database row SHALL have `status = 'interrupted'`, `status_reason = 'timeout'`, and `status_changed_at` set to the current ISO 8601 timestamp

---

### Requirement: ChatOrchestrator Status Transition Triggers

`ChatOrchestrator` SHALL replace all `persistRunStatus` calls that write `runtime.status` to `state_json` with `uc.TransitionStatus` calls. The following trigger points SHALL be implemented:

| Trigger | Target Status | Reason |
|---------|--------------|--------|
| Runner starts | `running` | — |
| Runner finishes normally | `completed` | — |
| Runner cancelled by user | `interrupted` | `user_cancelled` |
| Runner timeout | `interrupted` | `timeout` |
| Runner error | `interrupted` | `error` |
| Budget escalation | `interrupted` | `budget_escalated` |
| Tool needs confirmation | `awaiting_confirmation` | `tool_confirmation` |
| Agent awaits reply | `awaiting_confirmation` | `agent_awaiting_reply` |
| User confirms/replies | `running` | — |

`persistRunStatus` SHALL be retained but simplified: it SHALL only write runtime metadata keys (`runtime.run_id`, etc.) to `state_json`, NOT `runtime.status`.

#### Scenario: Runner start triggers running status

WHEN a Runner starts executing for a session
THEN `uc.TransitionStatus(sessionID, "running", "")` SHALL be called

#### Scenario: Runner completion triggers completed status

WHEN a Runner finishes normally
THEN `uc.TransitionStatus(sessionID, "completed", "")` SHALL be called

#### Scenario: Runner cancellation triggers interrupted with reason

WHEN a Runner is cancelled by the user
THEN `uc.TransitionStatus(sessionID, "interrupted", "user_cancelled")` SHALL be called

#### Scenario: Tool confirmation triggers awaiting_confirmation

WHEN a tool execution requires user confirmation
THEN `uc.TransitionStatus(sessionID, "awaiting_confirmation", "tool_confirmation")` SHALL be called

#### Scenario: User confirmation resumes running

WHEN a user confirms a tool execution
THEN `uc.TransitionStatus(sessionID, "running", "")` SHALL be called

---

### Requirement: SessionStatusGuard Lifecycle Hook

A `SessionStatusGuard` SHALL be implemented in `internal/service/session_status_guard.go` and registered as a Kratos lifecycle hook.

**OnShutdown**: When the server shuts down gracefully:
1. Cancel all active Runners
2. Call `uc.BatchTransitionInterrupted(ctx, "server_shutdown")` to transition all running sessions to `interrupted`
3. Sessions in `awaiting_confirmation` SHALL remain unchanged (they are recoverable)

**OnStartup**: When the server starts:
1. Call `uc.RecoverOrphanedRunningSessions(ctx)` to transition orphaned running sessions to `interrupted` + `unexpected_shutdown`

The `SessionStatusGuard` SHALL cooperate with the existing `SessionRunDurableWorker.CleanupOrphanedRuns`: the Guard fixes `sessions.status` while the Durable Worker cleans `session_runs` table records.

#### Scenario: Graceful shutdown transitions running sessions

WHEN the server shuts down gracefully and sessions with `status = 'running'` exist
THEN those sessions SHALL be transitioned to `interrupted` with reason `server_shutdown`

#### Scenario: Graceful shutdown preserves awaiting_confirmation sessions

WHEN the server shuts down gracefully and sessions with `status = 'awaiting_confirmation'` exist
THEN those sessions SHALL remain in `awaiting_confirmation` status unchanged

#### Scenario: Startup recovery for orphaned running sessions

WHEN the server starts and sessions with `status = 'running'` exist (from a previous crash)
THEN those sessions SHALL be transitioned to `interrupted` with reason `unexpected_shutdown`

#### Scenario: Startup with no orphaned sessions

WHEN the server starts and no sessions have `status = 'running'`
THEN `RecoverOrphanedRunningSessions` SHALL complete without error

---

### Requirement: Confirmation Timeout Auto-Cleanup

`SessionStatusGuard` SHALL provide a `CleanupExpiredConfirmations(ctx)` method that finds all sessions with `status = 'awaiting_confirmation'` where `status_changed_at` is older than the confirmation timeout (default 24 hours) and transitions them to `interrupted` with reason `confirmation_timeout`.

This method SHALL be invoked by the existing Durable Worker polling mechanism.

#### Scenario: Confirmation timeout triggers interrupted

WHEN a session has `status = 'awaiting_confirmation'` and `status_changed_at` is older than 24 hours
THEN `CleanupExpiredConfirmations` SHALL transition it to `interrupted` with reason `confirmation_timeout`

#### Scenario: Recent awaiting_confirmation not cleaned up

WHEN a session has `status = 'awaiting_confirmation'` and `status_changed_at` is less than 24 hours ago
THEN `CleanupExpiredConfirmations` SHALL NOT transition it

#### Scenario: No expired confirmations

WHEN no sessions have `status = 'awaiting_confirmation'` with expired timeout
THEN `CleanupExpiredConfirmations` SHALL return without error

---

### Requirement: WS Envelope Session Status Changed

A new WebSocket envelope type `session.status_changed` SHALL be published whenever a session's status transitions. The envelope payload SHALL include:

```json
{
  "type": "session.status_changed",
  "session_id": "xxx",
  "status": "interrupted",
  "status_reason": "user_cancelled",
  "status_changed_at": "2026-05-31T12:00:00Z"
}
```

#### Scenario: Status transition publishes WS event

WHEN a session transitions from `running` to `interrupted` with reason `timeout`
THEN a `session.status_changed` WS envelope SHALL be published with the session ID, new status, reason, and timestamp

#### Scenario: WS event includes all fields

WHEN a `session.status_changed` event is published
THEN the payload SHALL include `session_id`, `status`, `status_reason`, and `status_changed_at`

---

### Requirement: Proto Changes

The `Session` protobuf message SHALL include the following fields:

```protobuf
message Session {
  // ...existing fields
  string status = X;             // idle / running / completed / interrupted / awaiting_confirmation
  string status_reason = Y;      // interruption/await reason sub-type
  string status_changed_at = Z;  // ISO 8601 timestamp
}
```

A new admin-only RPC SHALL be added:

```protobuf
rpc ForceTransitionSessionStatus(ForceTransitionSessionStatusRequest) returns (ForceTransitionSessionStatusResponse) {
  option (google.api.http) = { post: "/v1/sessions/{id}/force-status" body: "*" };
}

message ForceTransitionSessionStatusRequest {
  string id = 1;
  string status = 2;
  string status_reason = 3;  // defaults to "manual_override"
}
```

The `ForceTransitionSessionStatus` RPC SHALL bypass the state machine validation and directly set the target status. It SHALL only be accessible with admin privileges. If `status_reason` is empty, it SHALL default to `"manual_override"`.

#### Scenario: ForceTransition overrides status

WHEN `ForceTransitionSessionStatus` is called with `id = "session-1"`, `status = "idle"`, `status_reason = ""`
THEN the session's status SHALL be set to `idle` with reason `manual_override`

#### Scenario: ForceTransition with explicit reason

WHEN `ForceTransitionSessionStatus` is called with `status_reason = "manual_override"`
THEN the session's `status_reason` SHALL be set to the provided value

#### Scenario: ForceTransition requires admin privileges

WHEN `ForceTransitionSessionStatus` is called without admin privileges
THEN the request SHALL be rejected with permission denied error

---

### Requirement: Data Migration

A data migration SHALL be executed to convert existing session data:

1. All sessions with `status = 'active'` SHALL be updated to `status = 'idle'`
2. Sessions with `status = 'archived'` or `status = 'deleted'` SHALL NOT have their `status` field modified; their lifecycle SHALL henceforth be determined by `archived_at`/`deleted_at` timestamps
3. Sessions with `status = 'running'` SHALL remain as `running`; the `SessionStatusGuard.OnStartup` will handle orphaned running sessions after restart

#### Scenario: Active sessions migrated to idle

WHEN the migration runs on sessions with `status = 'active'`
THEN those sessions SHALL have `status = 'idle'`

#### Scenario: Archived sessions status unchanged

WHEN the migration runs on sessions with `status = 'archived'`
THEN their `status` field SHALL NOT be modified; lifecycle SHALL be determined by `archived_at`

#### Scenario: Running sessions preserved

WHEN the migration runs on sessions with `status = 'running'`
THEN their `status` SHALL remain `running`

---

### Requirement: Index Adjustment

An index SHALL be created on `sessions(status, deleted_at)` with a partial filter `WHERE deleted_at = ''` to optimize queries for sessions by execution status among active sessions.

#### Scenario: Index supports status queries

WHEN a query filters by `status` and `deleted_at = ''`
THEN the database SHALL use the `idx_sessions_status` index for efficient lookup
