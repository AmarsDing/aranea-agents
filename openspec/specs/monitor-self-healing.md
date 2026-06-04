# Monitor Self Healing

## Diag Bundle

### REQ-DB-01: Auto-Heal Metadata in Bundle
DiagBundle SHALL include auto-heal metadata for each flow entry: `auto_healed`, `heal_strategy`, `heal_attempts`, `heal_success`. This allows diagnostic bundle consumers to understand which errors were already handled by the runtime.

### REQ-DB-02: Self-Heal Summary Section
DiagBundle manifest SHALL include a `self_heal_summary` section with: total errors, auto-healed count, auto-heal success count, auto-heal failure count, unhealed count.

### REQ-DB-03: Runtime Heal State in RootCauseResult
RootCauseResult SHALL include `RuntimeAutoHealed bool` and `RuntimeHealAttempts int` fields, populated from the trigger event's metadata. This allows API consumers to distinguish between "runtime already tried to fix this" and "runtime hasn't attempted to fix this".

## Root Cause Engine

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

## Runtime Auto Heal

### REQ-RH-01: AutoHealStrategy Interface
The runtime SHALL define an `AutoHealStrategy` interface that components implement to describe their self-healing behavior. Each strategy specifies: can-heal predicate, heal action, max attempts, and backoff duration.

### REQ-RH-02: LLM Call Auto-Heal
When an LLM call fails with a transient error (timeout, rate-limit 429, context exceeded), the runtime SHALL automatically retry with exponential backoff. Max attempts: timeout=2, rate-limit=3, context-exceeded=1 (with compression). The retry SHALL be transparent to the caller.

### REQ-RH-03: MCP Connection Auto-Reconnect
When an MCP server connection drops, the runtime SHALL automatically attempt reconnection up to 3 times with exponential backoff (3s base). The reconnection SHALL be triggered by the MCP broker health check.

### REQ-RH-04: Tool Execution Auto-Retry
When a tool execution fails with a transient error, the runtime SHALL automatically retry up to 1 time with 1s backoff. Non-transient errors (permission denied, invalid input) SHALL NOT be retried.

### REQ-RH-05: Self-Heal Event Reporting
When auto-heal is triggered (whether successful or not), the runtime SHALL emit a FlowLog event with the following metadata fields:
- `auto_healed`: boolean, always true for auto-heal events
- `heal_strategy`: string (e.g., "retry_with_backoff", "reconnect", "compress_and_retry")
- `heal_attempts`: integer, number of attempts made
- `heal_success`: boolean, whether the heal succeeded
- `heal_backoff_ms`: integer, backoff duration in milliseconds

### REQ-RH-06: Heal Strategy Configuration
Auto-heal behavior SHALL be configurable per Agent via `WithAutoHealConfig(AutoHealConfig)` RunOption. Default: enabled for all transient error types.

### REQ-RH-07: Heal Circuit Breaker
If auto-heal for the same error type fails 5 consecutive times within 10 minutes, the runtime SHALL stop attempting auto-heal for that error type and emit a `heal_circuit_open` FlowLog event. The circuit SHALL reset after 30 minutes.

## Self Heal Observer

### REQ-SO-01: FlowLog Event Observation
SelfHealObserver SHALL subscribe to FlowLog events via EventBus. For each event with `flow_phase=error`, it SHALL evaluate root causes and record the observation.

### REQ-SO-02: Auto-Heal Success Tracking
When a FlowLog event contains `auto_healed=true` and `heal_success=true`, the observer SHALL record a HealRecord with status `observed_healed`. No alert SHALL be fired.

### REQ-SO-03: Auto-Heal Failure Tracking
When a FlowLog event contains `auto_healed=true` and `heal_success=false`, the observer SHALL record a HealRecord with status `observed_failed`. If the same rule fails 3+ consecutive times, an alert SHALL be fired.

### REQ-SO-04: Unhealed Error Alerting
When a FlowLog event has `flow_phase=error` and `auto_healed=false` (runtime did not attempt heal), the observer SHALL run root cause analysis. If a rule matches with confidence >= 0.7, an alert SHALL be fired with the root cause and fix suggestion.

### REQ-SO-05: HealRecord Persistence
All HealRecords SHALL be persisted to SQLite via Ent ORM. Records SHALL NOT be lost on process restart. TTL: 30 days, auto-purged by cron job.

### REQ-SO-06: Heal Statistics API
The observer SHALL expose a `GetHealStats` API returning: total heals, success rate, top failing rules, recent heal history. This data SHALL be queryable via `GET /v1/monitor/heal-stats`.

### REQ-SO-07: ListHealRecords API
The observer SHALL expose a `ListHealRecords` API with pagination, filtering by rule_id/status/session_id, and time range. `GET /v1/monitor/heal-records`.

### REQ-SO-08: Cooldown Per Severity
Alert cooldown SHALL be severity-dependent: critical=30min, high=10min, medium=5min, low=2min. Same-rule alerts within cooldown period SHALL be suppressed.
