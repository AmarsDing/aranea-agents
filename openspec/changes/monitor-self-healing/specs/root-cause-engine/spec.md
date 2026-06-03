## Requirements

### REQ-RC-01: AutoHealed Condition Dimension
RootCauseCondition SHALL support an `AutoHealed *bool` field. When set to `true`, only match errors that the runtime has already attempted to auto-heal. When set to `false`, only match errors that the runtime has NOT attempted to auto-heal. When `nil`, match regardless of auto-heal status.

### REQ-RC-02: HealAttempts Condition Dimension
RootCauseCondition SHALL support a `HealAttempts int` field. When > 0, only match errors where the runtime has made at least this many auto-heal attempts.

### REQ-RC-03: Repeated Failure Rule
A built-in rule `rc-repeated-auto-heal-failure` SHALL match when `AutoHealed=true` and `HealAttempts>=3`. This rule has severity `critical` and action `log_only` (alert only, no auto-fix).

### REQ-RC-04: Unhealed Error Rules
Existing rules (rc-provider-timeout, rc-mcp-connection-failure, etc.) SHALL be updated with `AutoHealed=false` to only match errors that the runtime has NOT already handled. This prevents duplicate alerts for errors already auto-healed by the runtime.

### REQ-RC-05: Severity-Based Cooldown
The cooldown period SHALL be determined by the rule's severity level: critical=30min, high=10min, medium=5min, low=2min. This replaces the current global 5-minute cooldown.

### REQ-RC-06: AddRules Validation
`AddRules` SHALL validate that new rules have non-empty ID and Name. Rules with invalid conditions (e.g., unparseable regex) SHALL be rejected with an error, not silently skipped.
