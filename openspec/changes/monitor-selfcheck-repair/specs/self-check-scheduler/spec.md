## ADDED Requirements

### Requirement: SelfChecker interface
系统 SHALL 定义 `SelfChecker` 接口，包含 `Name()` 和 `Check(ctx) SelfCheckResult` 方法。每个子系统健康检查 SHALL 实现此接口。

#### Scenario: Checker returns healthy result
- **WHEN** a subsystem is operating normally
- **THEN** the Checker SHALL return `SelfCheckResult{Status: "healthy", Checker: "<name>", Message: "<description>"}`

#### Scenario: Checker returns degraded result
- **WHEN** a subsystem is partially functional (e.g., high latency but still working)
- **THEN** the Checker SHALL return `SelfCheckResult{Status: "degraded", Checker: "<name>", Message: "<description>"}`

#### Scenario: Checker returns unhealthy result
- **WHEN** a subsystem is not functioning (e.g., connection lost, worker stopped)
- **THEN** the Checker SHALL return `SelfCheckResult{Status: "unhealthy", Checker: "<name>", Message: "<description>"}`

#### Scenario: Checker timeout
- **WHEN** a Checker does not return within 10 seconds
- **THEN** the Scheduler SHALL cancel the check via context and record `SelfCheckResult{Status: "unhealthy", Message: "check timed out"}`

### Requirement: SelfCheckScheduler periodic execution
系统 SHALL 提供 `SelfCheckScheduler`，按配置间隔（默认 5 分钟）自动执行所有已注册的 SelfChecker。

#### Scenario: Periodic self-check runs on schedule
- **WHEN** the configured interval elapses
- **THEN** the Scheduler SHALL execute all registered Checkers sequentially and aggregate results into a `SelfCheckReport`

#### Scenario: Concurrent self-check prevention
- **WHEN** a self-check is already in progress and another is triggered (timer or manual)
- **THEN** the Scheduler SHALL skip the new trigger and log a warning

#### Scenario: Overall status aggregation
- **WHEN** all checkers return "healthy"
- **THEN** the report overall_status SHALL be "healthy"
- **WHEN** any checker returns "degraded" and none returns "unhealthy"
- **THEN** the report overall_status SHALL be "degraded"
- **WHEN** any checker returns "unhealthy"
- **THEN** the report overall_status SHALL be "unhealthy"

### Requirement: Manual self-check trigger API
系统 SHALL 提供 `POST /v1/monitor/self-check` API，允许手动触发一次自检。

#### Scenario: Manual trigger succeeds
- **WHEN** user calls the API and no self-check is in progress
- **THEN** the system SHALL execute all Checkers immediately and return the report

#### Scenario: Manual trigger while check in progress
- **WHEN** user calls the API while a self-check is already running
- **THEN** the system SHALL return 409 Conflict with a message indicating a check is in progress

### Requirement: Self-check report persistence
系统 SHALL 将每次自检报告持久化到 SQLite，保留 30 天。

#### Scenario: Report saved after each check
- **WHEN** a self-check completes
- **THEN** the system SHALL save a `SelfCheckReport` record with all check results, overall status, repair actions, and timing

#### Scenario: Old reports cleanup
- **WHEN** a cleanup cron job runs
- **THEN** the system SHALL delete reports older than 30 days

### Requirement: List self-check reports API
系统 SHALL 提供 `GET /v1/monitor/self-check-reports` API，支持分页查询自检报告历史。

#### Scenario: List reports with pagination
- **WHEN** user requests reports with page_size and page_token
- **THEN** the system SHALL return reports ordered by started_at descending with pagination support

### Requirement: Built-in checkers
系统 SHALL 提供 6 个内置 SelfChecker：db_health_checker、flow_file_checker、trace_projector_checker、alert_eval_checker、eventbus_checker、websocket_checker。

#### Scenario: DB health checker
- **WHEN** SQLite connection is available and schema is intact
- **THEN** db_health_checker SHALL return "healthy"
- **WHEN** SQLite connection fails or schema is corrupted
- **THEN** db_health_checker SHALL return "unhealthy"

#### Scenario: Flow file checker
- **WHEN** disk space is sufficient (>100MB free) and write test passes
- **THEN** flow_file_checker SHALL return "healthy"
- **WHEN** disk space is low (<100MB) but write still works
- **THEN** flow_file_checker SHALL return "degraded"
- **WHEN** write test fails
- **THEN** flow_file_checker SHALL return "unhealthy"

#### Scenario: Trace projector checker
- **WHEN** TraceProjector has processed events within the last 5 minutes
- **THEN** trace_projector_checker SHALL return "healthy"
- **WHEN** no new traces projected for >5 minutes but EventBus is subscribed
- **THEN** trace_projector_checker SHALL return "degraded"
- **WHEN** EventBus subscription is disconnected
- **THEN** trace_projector_checker SHALL return "unhealthy"

#### Scenario: Alert eval checker
- **WHEN** AlertEvalWorker has completed an evaluation within the last 60 seconds
- **THEN** alert_eval_checker SHALL return "healthy"
- **WHEN** last evaluation was >60 seconds ago
- **THEN** alert_eval_checker SHALL return "unhealthy"

#### Scenario: EventBus checker
- **WHEN** all expected subscriptions are active and no significant lag
- **THEN** eventbus_checker SHALL return "healthy"
- **WHEN** a subscription is missing but others are active
- **THEN** eventbus_checker SHALL return "degraded"
- **WHEN** all subscriptions are disconnected
- **THEN** eventbus_checker SHALL return "unhealthy"

#### Scenario: WebSocket checker
- **WHEN** at least one WS connection is active and recent push within 30 seconds
- **THEN** websocket_checker SHALL return "healthy"
- **WHEN** no active WS connections
- **THEN** websocket_checker SHALL return "degraded" (WS is client-initiated, cannot auto-repair)

### Requirement: Self-check metric integration
系统 SHALL 将自检结果注册为告警指标 `monitor.selfcheck_unhealthy_count`，值等于最近一次自检中 unhealthy 的检查器数量。

#### Scenario: Metric updated after each check
- **WHEN** a self-check completes with 2 unhealthy checkers
- **THEN** `monitor.selfcheck_unhealthy_count` SHALL be set to 2

#### Scenario: Alert rule triggers on unhealthy count
- **WHEN** an alert rule is configured for `monitor.selfcheck_unhealthy_count > 0`
- **THEN** the existing alert evaluation workflow SHALL fire the alert
