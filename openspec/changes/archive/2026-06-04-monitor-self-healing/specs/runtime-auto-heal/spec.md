## Requirements

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
