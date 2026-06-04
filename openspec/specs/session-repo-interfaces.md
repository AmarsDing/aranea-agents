# Session Repo Interfaces

## Session Repo Interfaces

### Requirement: Session repo interfaces for split tables
The `biz.SessionRepo` composite interface SHALL be updated to include new sub-interfaces for `session_metrics` and `session_runtime` tables. The existing `SessionReader`, `SessionWriter`, `ContextUpdater` interfaces SHALL be modified to reflect the table split.

#### Scenario: SessionMetricsReader interface
- **WHEN** a usecase needs to read session metrics
- **THEN** it SHALL depend on `biz.SessionMetricsReader` interface with methods: `GetSessionMetrics`, `BatchGetSessionMetrics`

#### Scenario: SessionMetricsWriter interface
- **WHEN** the delta flush mechanism writes metrics
- **THEN** it SHALL depend on `biz.SessionMetricsWriter` interface with method: `ApplyMetricsDelta`

#### Scenario: SessionRuntimeReader interface
- **WHEN** a usecase needs to read session runtime state
- **THEN** it SHALL depend on `biz.SessionRuntimeReader` interface with methods: `GetSessionRuntime`, `GetSessionRevision`

#### Scenario: SessionRuntimeWriter interface
- **WHEN** runtime state changes during a turn
- **THEN** it SHALL depend on `biz.SessionRuntimeWriter` interface with methods: `PatchSessionState`, `UpdateRunnerSnapshot`, `BumpSessionRevision`

#### Scenario: SessionRepo composite includes new sub-interfaces
- **WHEN** `biz.SessionRepo` is used for Wire binding
- **THEN** it SHALL embed `SessionMetricsReader` + `SessionMetricsWriter` + `SessionRuntimeReader` + `SessionRuntimeWriter` in addition to existing sub-interfaces
