## ADDED Requirements

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

## MODIFIED Requirements

### Requirement: RootCauseCondition self-check dimension
RootCauseCondition SHALL 增加 `SelfCheckStatus` 字段，允许根因规则基于自检结果匹配。

#### Scenario: Root cause rule matches self-check failure
- **WHEN** a RootCauseCondition has `SelfCheckStatus: "unhealthy"` and the self-check for that subsystem is unhealthy
- **THEN** the rule SHALL match and the RootCauseResult SHALL include self-check context

#### Scenario: Root cause rule ignores self-check when nil
- **WHEN** a RootCauseCondition has `SelfCheckStatus: nil`
- **THEN** the rule SHALL not consider self-check status in matching (backward compatible)
