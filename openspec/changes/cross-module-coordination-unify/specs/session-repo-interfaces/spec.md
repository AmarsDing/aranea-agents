## MODIFIED Requirements

### Requirement: Session three-table schema is final
The sessions table SHALL be split into three tables: `sessions` (core fields), `session_metrics` (token usage, cost, latency), and `session_runtime` (status, runtime state). This three-table structure is the final schema. All modules MUST use SessionMetricsRepo and SessionRuntimeRepo for accessing metrics and runtime data respectively.

#### Scenario: Module reads session metrics
- **WHEN** a module needs session token usage or cost data
- **THEN** it queries SessionMetricsRepo, not the sessions table directly

#### Scenario: Module updates session status
- **WHEN** a module needs to update session status
- **THEN** it uses SessionRuntimeRepo.TransitionSessionStatus(), not a direct sessions table update

### Requirement: SessionMetricsDTO decoupled from toProtoSession
The `toProtoSession` function in service layer SHALL query SessionMetricsRepo independently for metrics fields, instead of joining from the sessions table. The SessionMetricsDTO SHALL be a separate struct used only for proto conversion.

#### Scenario: toProtoSession fetches metrics independently
- **WHEN** toProtoSession is called for a session
- **THEN** it fetches metrics from SessionMetricsRepo by session_id, not from the sessions table

### Requirement: SessionRuntime patch migration
The `UpdateSession` function SHALL route runtime field updates (status, status_reason, finished_at) through SessionRuntimeRepo, not through a direct sessions table update.

#### Scenario: UpdateSession changes status
- **WHEN** UpdateSession is called with a status change
- **THEN** the status update is routed through SessionRuntimeRepo.TransitionSessionStatus()

### Requirement: SessionStatus enum in biz/types
The `SessionStatus` enum and `StatusReason` type SHALL be defined in `internal/biz/types/session_types.go` as the single source of truth. All modules MUST import these types from `biz/types`.

#### Scenario: SessionStatus type source
- **WHEN** a module needs to reference session status values
- **THEN** it imports `types.SessionStatus` from `internal/biz/types/session_types.go`
