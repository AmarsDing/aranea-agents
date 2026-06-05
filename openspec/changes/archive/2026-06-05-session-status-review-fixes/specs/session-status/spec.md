## ADDED Requirements

### Requirement: Session Status State Machine

The `SessionStatusMachine` SHALL enforce legal status transitions. Any transition not in the legal transition table SHALL be rejected with `kerrors.Conflict` (NOT `kerrors.BadRequest`).

#### Scenario: Illegal transition returns FailedPrecondition

WHEN a session in `idle` status attempts to transition to `completed`
THEN the method SHALL return `kerrors.Conflict` (HTTP 409 / gRPC ABORTED)

#### Scenario: Legal transition succeeds

WHEN a session in `idle` status transitions to `running`
THEN the transition SHALL succeed without error

---

### Requirement: Delete and Archive Protection

Sessions with protected statuses (`running`, `awaiting_confirmation`) SHALL NOT be deleted or archived. The backend SHALL return `kerrors.Conflict` (NOT `kerrors.BadRequest`) with message indicating the session is in a protected status.

#### Scenario: Delete protected running session returns FailedPrecondition

WHEN a delete request is made for a session with `status = 'running'`
THEN the system SHALL return `Conflict` and the session SHALL NOT be deleted

#### Scenario: Archive protected session returns FailedPrecondition

WHEN an archive request is made for a session with `status = 'running'`
THEN the system SHALL return `Conflict` and the session SHALL NOT be archived

---

### Requirement: ChatOrchestrator Status Transition Triggers

`ChatOrchestrator` SHALL use the correct `status_reason` for each trigger point. Timeout scenarios (first-byte timeout, turn timeout) SHALL use `StatusReasonTimeout`, NOT `StatusReasonError`.

Updated trigger points:

| Trigger | Target Status | Reason |
|---------|--------------|--------|
| First-byte timeout | `interrupted` | `timeout` |
| Turn timeout | `interrupted` | `timeout` |
| Runner error | `interrupted` | `error` |
| Empty reply | `interrupted` | `error` |
| Stream error | `interrupted` | `error` |
| Spirit Team error | `interrupted` | `error` |

> Note: Spirit Team error trigger is implemented in `internal/service/spirit_team.go:101`.

#### Scenario: First-byte timeout uses timeout reason

WHEN a first-byte timeout occurs during Runner execution
THEN `uc.TransitionStatus(sessionID, "interrupted", "timeout")` SHALL be called

#### Scenario: Turn timeout uses timeout reason

WHEN a turn timeout occurs
THEN `uc.TransitionStatus(sessionID, "interrupted", "timeout")` SHALL be called

#### Scenario: Runner error uses error reason

WHEN a runtime error occurs during Runner execution
THEN `uc.TransitionStatus(sessionID, "interrupted", "error")` SHALL be called

---

### Requirement: SessionMutator DeleteSessionsByAgentID

`DeleteSessionsByAgentID` SHALL NOT set the `status` field to `"deleted"`. It SHALL only set `deleted_at` to the current timestamp. The `deleted` value is no longer a valid status value; lifecycle SHALL be determined by `deleted_at` timestamp.

#### Scenario: DeleteSessionsByAgentID does not set status to deleted

WHEN `DeleteSessionsByAgentID` is called for an agent
THEN the sessions SHALL have `deleted_at` set to the current timestamp and `status` SHALL NOT be modified to `"deleted"`

---

### Requirement: TransitionSessionStatus Concurrent Conflict Handling

`TransitionSessionStatus` SHALL return an error when zero rows are affected (indicating the current status has been concurrently modified). It SHALL NOT silently return nil in this case.

#### Scenario: Concurrent status conflict returns error

WHEN `TransitionSessionStatus` is called and the WHERE condition `status = currentStatus` matches zero rows
THEN the method SHALL return `kerrors.Conflict` indicating the status has been concurrently modified

#### Scenario: Normal transition succeeds

WHEN `TransitionSessionStatus` is called and the current status matches
THEN the method SHALL update the status and return nil

---

### Requirement: BatchTransitionInterrupted WS Event Publishing

`BatchTransitionInterrupted` SHALL publish a `session.status_changed` WS event for each session that is successfully transitioned. This ensures the frontend is notified of status changes during server startup and shutdown.

#### Scenario: Batch transition publishes WS events

WHEN `BatchTransitionInterrupted` transitions sessions from `running` to `interrupted`
THEN a `session.status_changed` WS event SHALL be published for each successfully transitioned session

#### Scenario: Batch transition with failed sessions still publishes for successful ones

WHEN `BatchTransitionInterrupted` processes 3 sessions and 1 fails
THEN WS events SHALL be published for the 2 successfully transitioned sessions
