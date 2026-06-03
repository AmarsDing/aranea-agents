## Requirements

### REQ-DB-01: Auto-Heal Metadata in Bundle
DiagBundle SHALL include auto-heal metadata for each flow entry: `auto_healed`, `heal_strategy`, `heal_attempts`, `heal_success`. This allows diagnostic bundle consumers to understand which errors were already handled by the runtime.

### REQ-DB-02: Self-Heal Summary Section
DiagBundle manifest SHALL include a `self_heal_summary` section with: total errors, auto-healed count, auto-heal success count, auto-heal failure count, unhealed count.

### REQ-DB-03: Runtime Heal State in RootCauseResult
RootCauseResult SHALL include `RuntimeAutoHealed bool` and `RuntimeHealAttempts int` fields, populated from the trigger event's metadata. This allows API consumers to distinguish between "runtime already tried to fix this" and "runtime hasn't attempted to fix this".
