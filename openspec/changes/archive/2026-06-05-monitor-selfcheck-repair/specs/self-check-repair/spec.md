## ADDED Requirements

### Requirement: SelfCheckRepairer interface
系统 SHALL 定义 `SelfCheckRepairer` 接口，包含 `CanRepair(checkName, status)` 和 `Repair(ctx, result) RepairOutcome` 方法。

#### Scenario: Repairer claims responsibility
- **WHEN** a self-check result has status "failed" or "warning"
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

### Requirement: SelfCheckRepairDispatcher
系统 SHALL 提供 `SelfCheckRepairDispatcher`，负责将自检结果路由到合适的 Repairer，并执行冷却期检查。

#### Scenario: Dispatch to first matching repairer
- **WHEN** a self-check result needs repair
- **THEN** the dispatcher SHALL find the first registered Repairer where `CanRepair` returns true and execute its `Repair` method

#### Scenario: No repairer available
- **WHEN** no registered Repairer can handle the check result
- **THEN** the dispatcher SHALL return `RepairOutcome{Success: false, Action: "none", Message: "no repairer available"}`

### Requirement: Built-in repair actions

#### Scenario: Flow file repair - cleanup expired compressed files
- **WHEN** flow_file checker returns "warning" or "failed"
- **THEN** the FlowFileRepairer SHALL call `PurgeExpiredFiles()` and `CompressOldFiles()` to free disk space
- **AND** return `RepairOutcome{Success: true, Action: "purge_expired_and_compress"}`

**注意**：FlowFileRepairer 是当前唯一已实现的 Repairer。以下 Repairer 尚未实现：

#### Scenario: Trace projector repair - trigger backfill（未实现）
- **WHEN** trace_projector checker returns "warning" or "failed"
- **THEN** the repairer SHALL trigger a TraceBackfill job to recover missing traces

#### Scenario: Alert eval repair - restart worker（未实现）
- **WHEN** alert_eval checker returns "failed" (worker not ready)
- **THEN** the repairer SHALL restart the AlertEvalWorker goroutine

#### Scenario: EventBus repair - resubscribe（未实现）
- **WHEN** eventbus checker returns "warning" or "failed"
- **THEN** the repairer SHALL resubscribe the disconnected handler to EventBus

#### Scenario: DB health and WebSocket - no auto repair
- **WHEN** db_health or websocket checker returns "failed" or "warning"
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
同一检查器的修复动作 SHALL 有 5 分钟冷却期（300 秒），防止修复动作频繁执行。

#### Scenario: Repair within cooldown
- **WHEN** a repair action was executed for checker X less than 5 minutes ago (and succeeded)
- **THEN** the system SHALL skip the repair and return `RepairOutcome{Success: false, Action: "skipped_cooldown"}`

#### Scenario: Repair after cooldown
- **WHEN** a repair action was executed for checker X more than 5 minutes ago, or the previous repair failed (no cooldown set)
- **THEN** the system SHALL execute the repair normally

#### Scenario: Cooldown only set on success
- **WHEN** a repair action fails
- **THEN** the cooldown SHALL NOT be set, allowing immediate retry on next check cycle

### Requirement: Repair integration with HealRecord
修复动作 SHALL 同步写入 HealRecord（复用 monitor-self-healing 变更的持久化机制），实现统一修复历史追溯。

#### Scenario: Repair recorded as HealRecord
- **WHEN** a self-check repair action is executed
- **THEN** the system SHALL create a HealRecord with trigger_type="self_check", fix_action_type matching the repair action, and status based on outcome

**注意**：此功能尚未实现，代码中 SelfCheckRepairDispatcher 未调用 HealRecord 写入。

## MODIFIED Requirements

### Requirement: RootCauseCondition self-check dimension
RootCauseCondition SHALL 增加 `SelfCheckStatus` 字段，允许根因规则基于自检结果匹配。

#### Scenario: Root cause rule matches self-check failure
- **WHEN** a RootCauseCondition has `SelfCheckStatus: "failed"` and the self-check for that subsystem is failed
- **THEN** the rule SHALL match and the RootCauseResult SHALL include self-check context

#### Scenario: Root cause rule ignores self-check when nil
- **WHEN** a RootCauseCondition has `SelfCheckStatus: nil`
- **THEN** the rule SHALL not consider self-check status in matching (backward compatible)

**注意**：`SelfCheckStatusCondition` 类型已定义在 `internal/biz/types/monitor_condition.go`，Proto 中 `RootCauseCondition.self_check_status` 已定义，但 Go 层 `RootCauseCondition` struct 中尚未添加 SelfCheckStatus 字段，`rc-self-check-failure` 内置根因规则也未实现。

### Requirement: DiagBundle self-check snapshot
DiagBundle SHALL 增加自检快照数据，从 SelfCheckReportRepo 获取最近一次报告。

#### Scenario: Self-check snapshot in diagnostic bundle
- **WHEN** a diagnostic bundle is generated
- **THEN** the bundle SHALL include a `self_check_snapshot` field with the most recent SelfCheckReport

**注意**：此功能尚未实现，代码中 DiagBundle 和 DiagBundleGenerator 未包含 self_check_snapshot。
