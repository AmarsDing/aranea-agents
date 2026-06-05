## ADDED Requirements

### Requirement: Session table cold-hot split
The system SHALL split the `sessions` table into three tables: `sessions` (cold metadata), `session_metrics` (hot aggregates), and `session_runtime` (runtime state). Each table SHALL have `session_id` as primary key with `session_runtime.session_id` and `session_metrics.session_id` as foreign keys referencing `sessions.id`.

#### Scenario: New session creation writes to all three tables
- **WHEN** a new session is created
- **THEN** the system SHALL INSERT a row into `sessions`, INSERT a row into `session_metrics` with zeroed counters, and INSERT a row into `session_runtime` with initial state

#### Scenario: Session metrics are written asynchronously
- **WHEN** a chat turn completes and metrics delta is flushed
- **THEN** the system SHALL UPDATE `session_metrics` asynchronously via the existing `SessionMetricsDelta` mechanism, without blocking the synchronous write path

#### Scenario: Session runtime state is written synchronously
- **WHEN** runtime state changes (status, state_json, revision, runner_snapshot)
- **THEN** the system SHALL UPDATE `session_runtime` synchronously, merging multiple patches into minimal writes

### Requirement: Session list query with metrics JOIN
The system SHALL support `SearchSessions` queries that LEFT JOIN `session_metrics` to return complete session data in a single query.

#### Scenario: List sessions with metrics
- **WHEN** `SearchSessions` is called
- **THEN** the system SHALL return sessions with metrics fields populated from `session_metrics` table via LEFT JOIN

#### Scenario: Metrics cache hit
- **WHEN** `SearchSessions` is called and session metrics are in the LRU cache
- **THEN** the system SHALL return cached metrics without querying the `session_metrics` table

### Requirement: Session metrics cache
The system SHALL maintain an in-process LRU cache (capacity 500, TTL 30s) for `session_metrics` rows. Cache SHALL be invalidated when metrics are flushed.

#### Scenario: Cache miss triggers DB read
- **WHEN** a session's metrics are not in cache
- **THEN** the system SHALL query `session_metrics` from DB and populate the cache

#### Scenario: Metrics flush invalidates cache
- **WHEN** `ApplyMetricsDelta` writes to `session_metrics`
- **THEN** the system SHALL remove the affected session_id from cache

### Requirement: MetricsUpdated WebSocket event
The system SHALL publish an `EnvelopeTypeMetricsUpdated` event via EventBus when session metrics are flushed, so the frontend can update in real-time.

#### Scenario: Metrics updated event published
- **WHEN** `ApplyMetricsDelta` completes
- **THEN** the system SHALL publish `EnvelopeTypeMetricsUpdated` with session_id and updated metrics fields

### Requirement: Feature flag controlled migration
The session table split SHALL be controlled by a feature flag with three states: `legacy` (write to old sessions columns), `dual_write` (write to both old and new tables), `new_table` (write only to new tables).

#### Scenario: Legacy mode
- **WHEN** feature flag is `legacy`
- **THEN** the system SHALL write metrics/runtime fields to the `sessions` table as before

#### Scenario: Dual write mode
- **WHEN** feature flag is `dual_write`
- **THEN** the system SHALL write to both old `sessions` columns and new tables, reading from new tables

#### Scenario: New table mode
- **WHEN** feature flag is `new_table`
- **THEN** the system SHALL write only to `session_metrics` and `session_runtime`, ignoring old columns in `sessions`
