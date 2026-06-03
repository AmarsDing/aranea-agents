## ADDED Requirements

### Requirement: Unified error translation function
The system SHALL provide an `entErrToBizErr(err error, domain, msg string) error` function in `internal/data/errors.go` that translates Ent errors to kerrors. All Repo methods SHALL use this function instead of ad-hoc error handling.

#### Scenario: NotFound translation
- **WHEN** an Ent query returns `ent.IsNotFound(err) == true`
- **THEN** `entErrToBizErr` SHALL return `kerrors.NotFound(domain, msg)`

#### Scenario: ConstraintError translation
- **WHEN** an Ent operation returns `ent.IsConstraintError(err) == true`
- **THEN** `entErrToBizErr` SHALL return `kerrors.Conflict(domain, msg)`

#### Scenario: NotLoaded translation
- **WHEN** an Ent query returns `ent.IsNotLoaded(err) == true`
- **THEN** `entErrToBizErr` SHALL return `kerrors.BadRequest(domain, msg)`

#### Scenario: Unknown error translation
- **WHEN** an Ent operation returns an unrecognized error
- **THEN** `entErrToBizErr` SHALL return `kerrors.InternalServer(domain, msg)`

### Requirement: DB query latency metrics
The system SHALL expose Prometheus histogram metrics for database query latency in `internal/data/`. The metric name SHALL be `aranea_db_query_duration_seconds` with labels `repo`, `operation`, and `status`.

#### Scenario: Query latency recorded
- **WHEN** a Repo method executes a database query
- **THEN** the system SHALL record the query duration in the histogram metric

### Requirement: Slow query logging
The system SHALL log warnings for database queries that exceed 100ms. The log entry SHALL include repo name, operation, duration, and query context.

#### Scenario: Slow query detected
- **WHEN** a database query takes longer than 100ms
- **THEN** the system SHALL log a warning with `loggateway.StepID("data.slow_query")`, repo name, operation, and duration

### Requirement: Connection pool metrics
The system SHALL expose Prometheus gauge metrics for SQLite and Postgres connection pool stats: `aranea_db_pool_open_connections`, `aranea_db_pool_in_use`, `aranea_db_pool_idle`, `aranea_db_pool_wait_count`.

#### Scenario: Pool stats collected
- **WHEN** metrics are scraped
- **THEN** the system SHALL report connection pool statistics for both write and read SQLite pools, and the Postgres pool
