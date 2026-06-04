# Monitor Selfcheck Repair

## Self Check Repair

### Requirement: SelfCheckRepairer interface
系统 SHALL 定义 `SelfCheckRepairer` 接口，包含 `CanRepair(checkName, status)` 和 `Repair(ctx, result) RepairOutcome` 方法。

#### Scenario: Repairer claims responsibility
- **WHEN** a self-check result has status "unhealthy" or "degraded"
- **THEN** the Repairer SHALL evaluate `CanRepair` to determine if it can handle the repair

#### Scenario: Repairer executes repair
- **WHEN** `CanRepair` returns true
- **THEN** the system SHALL call `Repair` and record the `RepairOutcome`

#### Scenario: Repair success
- **WHEN** repair action completes successfully
- **THEN** `RepairOutcome.Success` SHALL be true and `RepairOutcome.Action` SHALL describe the action taken

#### Scenario: Repair failure
- **WHEN** repair action fails
- **THEN** `RepairOutcome.Success` SHALL be false and `RepairOutcome.Message` SHALL describe the failure reason; the system SHALL NOT retry the repair automatically

### Requirement: Built-in repair actions
系统 SHALL 为以下检查器提供内置修复动作：

#### Scenario: Flow file repair - cleanup expired compressed files
- **WHEN** flow_file_checker returns "degraded" (low disk space)
- **THEN** the repairer SHALL delete expired compressed files (.jsonl.gz older than retention period) to free disk space

#### Scenario: Trace projector repair - trigger backfill
- **WHEN** trace_projector_checker returns "degraded" or "unhealthy"
- **THEN** the repairer SHALL trigger a TraceBackfill job to recover missing traces

#### Scenario: Alert eval repair - restart worker
- **WHEN** alert_eval_checker returns "unhealthy" (worker stalled)
- **THEN** the repairer SHALL restart the AlertEvalWorker goroutine

#### Scenario: EventBus repair - resubscribe
- **WHEN** eventbus_checker returns "degraded" or "unhealthy"
- **THEN** the repairer SHALL resubscribe the disconnected handler to EventBus

#### Scenario: DB health and WebSocket - no auto repair
- **WHEN** db_health_checker or websocket_checker returns "unhealthy"
- **THEN** the system SHALL NOT attempt auto repair; only log and alert

### Requirement: Repair idempotency
所有修复动作 SHALL 具有幂等性，重复执行不会产生副作用。

#### Scenario: Duplicate repair execution
- **WHEN** the same repair action is executed twice for the same issue
- **THEN** the second execution SHALL be a no-op and return success

### Requirement: Repair action logging
每次修复动作 SHALL 记录到自检报告的 `repair_actions` 字段，包含动作名称、结果、时间戳。

#### Scenario: Repair action recorded in report
- **WHEN** a repair action is executed
- **THEN** the SelfCheckReport SHALL include the repair action in its `repair_actions` array

### Requirement: Repair cooldown
同一检查器的修复动作 SHALL 有 5 分钟冷却期，防止修复动作频繁执行。

#### Scenario: Repair within cooldown
- **WHEN** a repair action was executed for checker X less than 5 minutes ago
- **THEN** the system SHALL skip the repair and log "repair skipped: cooldown active"

#### Scenario: Repair after cooldown
- **WHEN** a repair action was executed for checker X more than 5 minutes ago
- **THEN** the system SHALL execute the repair normally

### Requirement: Repair integration with HealRecord
修复动作 SHALL 同步写入 HealRecord（复用 monitor-self-healing 变更的持久化机制），实现统一修复历史追溯。

#### Scenario: Repair recorded as HealRecord
- **WHEN** a self-check repair action is executed
- **THEN** the system SHALL create a HealRecord with trigger_type="self_check", fix_action_type matching the repair action, and status based on outcome

### Requirement: RootCauseCondition self-check dimension
RootCauseCondition SHALL 增加 `SelfCheckStatus` 字段，允许根因规则基于自检结果匹配。

#### Scenario: Root cause rule matches self-check failure
- **WHEN** a RootCauseCondition has `SelfCheckStatus: "unhealthy"` and the self-check for that subsystem is unhealthy
- **THEN** the rule SHALL match and the RootCauseResult SHALL include self-check context

#### Scenario: Root cause rule ignores self-check when nil
- **WHEN** a RootCauseCondition has `SelfCheckStatus: nil`
- **THEN** the rule SHALL not consider self-check status in matching (backward compatible)

## Self Check Scheduler

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
