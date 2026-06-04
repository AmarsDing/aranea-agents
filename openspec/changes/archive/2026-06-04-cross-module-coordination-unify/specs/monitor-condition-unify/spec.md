## ADDED Requirements

### Requirement: RootCauseCondition proto oneof extension
The `RootCauseCondition` message in `api/kratos/monitor/v1/monitor.proto` SHALL use a `oneof condition` field to support extensible condition types. Each monitoring module (self-healing, selfcheck-repair) SHALL add its own condition type under the oneof.

#### Scenario: Self-healing adds AutoHealedCondition
- **WHEN** monitor-self-healing is implemented
- **THEN** `AutoHealedCondition` is added as a oneof option in `RootCauseCondition.condition`

#### Scenario: Selfcheck-repair adds SelfCheckStatusCondition
- **WHEN** monitor-selfcheck-repair is implemented
- **THEN** `SelfCheckStatusCondition` is added as a oneof option in `RootCauseCondition.condition`

#### Scenario: Both conditions coexist without conflict
- **WHEN** both monitor modules are implemented
- **THEN** `RootCauseCondition.condition` oneof contains both `auto_healed` and `self_check_status` options

### Requirement: Unified HealRecord and SelfCheckResult types in biz/types
The system SHALL define `HealRecord` and `SelfCheckResult` structs in `internal/biz/types/monitor_condition.go`. Both monitor-self-healing and monitor-selfcheck-repair MUST import these types from `biz/types`.

#### Scenario: HealRecord type uniqueness
- **WHEN** a developer searches for `type HealRecord struct`
- **THEN** exactly one definition exists in `internal/biz/types/monitor_condition.go`

#### Scenario: SelfCheckResult type uniqueness
- **WHEN** a developer searches for `type SelfCheckResult struct`
- **THEN** exactly one definition exists in `internal/biz/types/monitor_condition.go`

### Requirement: Alert level coordination
Both monitor modules SHALL register alerts through the same `AlertMetricRegistry` with coordinated severity levels: `critical` (system-down), `warning` (degraded), `info` (self-healed). No module SHALL define its own alert severity enum.

#### Scenario: Self-healing registers info-level alert
- **WHEN** runtime auto-heal succeeds
- **THEN** an info-level alert is registered via `AlertMetricRegistry`

#### Scenario: Selfcheck-repair registers warning-level alert
- **WHEN** a self-check detects a degraded component
- **THEN** a warning-level alert is registered via `AlertMetricRegistry`
