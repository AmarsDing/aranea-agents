## Requirements

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
