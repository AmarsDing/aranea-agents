## ADDED Requirements

### Requirement: SelfChecker interface
系统 SHALL 定义 `SelfChecker` 接口，包含 `Name()` 和 `Check(ctx) SelfCheckResult` 方法。每个子系统健康检查 SHALL 实现此接口。

#### Scenario: Checker returns passed result
- **WHEN** a subsystem is operating normally
- **THEN** the Checker SHALL return `SelfCheckResult{Status: "passed", Checker: "<name>", Message: "<description>"}`

#### Scenario: Checker returns warning result
- **WHEN** a subsystem is partially functional (e.g., no active traces but projector available)
- **THEN** the Checker SHALL return `SelfCheckResult{Status: "warning", Checker: "<name>", Message: "<description>"}`

#### Scenario: Checker returns failed result
- **WHEN** a subsystem is not functioning (e.g., connection lost, worker not ready)
- **THEN** the Checker SHALL return `SelfCheckResult{Status: "failed", Checker: "<name>", Message: "<description>"}`

#### Scenario: Checker timeout
- **WHEN** a Checker does not return within 10 seconds
- **THEN** the Scheduler SHALL cancel the check via context and record `SelfCheckResult{Status: "failed", Message: "checker timed out"}`

#### Scenario: Checker panic
- **WHEN** a Checker panics during execution
- **THEN** the Scheduler SHALL recover the panic and record `SelfCheckResult{Status: "failed", Message: "checker panicked"}`

### Requirement: SelfCheckScheduler periodic execution
系统 SHALL 提供 `SelfCheckScheduler`，按配置间隔（默认 5 分钟，可通过 `SELF_CHECK_INTERVAL` 环境变量配置，最小 1 分钟）自动执行所有已注册的 SelfChecker。

#### Scenario: Periodic self-check runs on schedule
- **WHEN** the configured interval elapses
- **THEN** the Scheduler SHALL execute all registered Checkers sequentially and aggregate results into a `SelfCheckReport`

#### Scenario: Concurrent self-check prevention
- **WHEN** a self-check is already in progress and another is triggered (timer or manual)
- **THEN** the Scheduler SHALL skip the new trigger and log a warning

#### Scenario: Overall status aggregation
- **WHEN** all checkers return "passed"
- **THEN** the report overall_status SHALL be "passed"
- **WHEN** any checker returns "warning" and none returns "failed"
- **THEN** the report overall_status SHALL be "warning"
- **WHEN** any checker returns "failed"
- **THEN** the report overall_status SHALL be "failed"

#### Scenario: Start runs immediately
- **WHEN** the Scheduler starts
- **THEN** it SHALL execute RunOnce immediately before waiting for the first ticker interval

### Requirement: Manual self-check trigger API
系统 SHALL 提供 `POST /v1/monitor/self-check` API，允许手动触发一次自检。

#### Scenario: Manual trigger succeeds
- **WHEN** user calls the API and no self-check is in progress
- **THEN** the system SHALL execute all Checkers immediately and return the report

#### Scenario: Manual trigger while check in progress
- **WHEN** user calls the API while a self-check is already running
- **THEN** the system SHALL return 409 Conflict with a message indicating a check is in progress

### Requirement: Self-check report persistence
系统 SHALL 将每次自检报告持久化到 SQLite `self_check_reports` 表，保留 30 天。

#### Scenario: Report saved after each check
- **WHEN** a self-check completes
- **THEN** the system SHALL save a `SelfCheckReport` record with all check results, overall status, repair actions, and timing. Persistence failure SHALL be logged but not block the self-check cycle.

#### Scenario: Old reports cleanup
- **WHEN** a cleanup cron job runs
- **THEN** the system SHALL delete reports older than 30 days

### Requirement: List self-check reports API
系统 SHALL 提供 `GET /v1/monitor/self-check-reports` API，支持分页查询自检报告历史。

#### Scenario: List reports with pagination
- **WHEN** user requests reports with limit and offset
- **THEN** the system SHALL return reports ordered by created_at descending with pagination support (default limit 20, max 100)

### Requirement: Built-in checkers
系统 SHALL 提供 6 个内置 SelfChecker：db_health、flow_file、trace_projector、alert_eval、eventbus、websocket。

#### Scenario: DB health checker
- **WHEN** SQLite connection is available and schema is intact (monitor_events table exists)
- **THEN** db_health SHALL return "passed"
- **WHEN** SQLite connection fails or schema is corrupted
- **THEN** db_health SHALL return "failed"

#### Scenario: Flow file checker
- **WHEN** write test passes (temp file creation succeeds)
- **THEN** flow_file SHALL return "passed"
- **WHEN** write test fails
- **THEN** flow_file SHALL return "failed"
- **WHEN** appender is nil
- **THEN** flow_file SHALL return "warning"

**注意**：代码当前未实现磁盘空间检查（>100MB=passed, <100MB=warning），仅做了写入测试。

#### Scenario: Trace projector checker
- **WHEN** TraceProjector has active traces (TraceCount > 0)
- **THEN** trace_projector SHALL return "passed"
- **WHEN** no active traces (TraceCount == 0)
- **THEN** trace_projector SHALL return "warning"
- **WHEN** projector is nil
- **THEN** trace_projector SHALL return "warning"

**注意**：代码当前检查的是 `TraceCount()` 而非"最近 5 分钟是否有新 Trace 投影"。

#### Scenario: Alert eval checker
- **WHEN** AlertEvalWorker is Ready
- **THEN** alert_eval SHALL return "passed"
- **WHEN** AlertEvalWorker is not Ready
- **THEN** alert_eval SHALL return "failed"
- **WHEN** worker is nil
- **THEN** alert_eval SHALL return "failed"

**注意**：代码当前检查的是 `Ready()` 而非"最近评估时间"。

#### Scenario: EventBus checker
- **WHEN** all expected subscriptions are active and healthy (IsHealthy returns true)
- **THEN** eventbus SHALL return "passed"
- **WHEN** some subscribers are unhealthy but count > 0
- **THEN** eventbus SHALL return "warning"
- **WHEN** all subscribers disconnected (count == 0 and IsHealthy == false)
- **THEN** eventbus SHALL return "failed"
- **WHEN** bus is nil
- **THEN** eventbus SHALL return "warning"

#### Scenario: WebSocket checker
- **WHEN** at least one WS connection is active (CountGlobalMonitorConns > 0)
- **THEN** websocket SHALL return "passed"
- **WHEN** no active WS connections
- **THEN** websocket SHALL return "warning"
- **WHEN** counter is nil
- **THEN** websocket SHALL return "warning"

**注意**：代码当前未检查"最近推送时间"，仅检查连接数。

### Requirement: Self-check metric integration
系统 SHALL 将自检结果注册为告警指标 `monitor.selfcheck_unhealthy_count`，通过 `SelfCheckUnhealthyCountMetric` 实现 `AlertMetric` 接口。

#### Scenario: Metric evaluates current unhealthy count
- **WHEN** the metric's Evaluate method is called
- **THEN** it SHALL execute all registered Checkers and return the count of non-passed results

#### Scenario: Alert rule triggers on unhealthy count
- **WHEN** an alert rule is configured for `monitor.selfcheck_unhealthy_count > 0`
- **THEN** the existing alert evaluation workflow SHALL fire the alert

### Requirement: SelfCheckResult extended fields
SelfCheckResult SHALL 包含 `CheckID`（UUID 唯一标识）和 `Conditions`（SelfCheckStatusCondition 数组，用于 RootCauseCondition 扩展）字段。

#### Scenario: CheckID assigned automatically
- **WHEN** a Checker returns a SelfCheckResult
- **THEN** the CheckID SHALL be a UUID generated by the checker or scheduler

#### Scenario: Conditions for root cause integration
- **WHEN** a self-check result is used in root cause analysis
- **THEN** the Conditions field SHALL carry SelfCheckStatusCondition data for RootCauseCondition matching
