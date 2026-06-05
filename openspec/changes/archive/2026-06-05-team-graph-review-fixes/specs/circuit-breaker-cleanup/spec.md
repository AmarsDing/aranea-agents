## ADDED Requirements

### Requirement: Remove CircuitBreakerState and cbState passing chain
The system SHALL remove `CircuitBreakerState`, `NewCircuitBreakerState`, and all `cbState` parameter passing from `BuildStateGraphWithAgents`/`buildFromResolved`/`wireNode`/`nodeOptions`. The `circuit_breaker.go` file SHALL be deleted.

#### Scenario: BuildStateGraphWithAgents no longer creates cbState
- **WHEN** `BuildStateGraphWithAgents` is called
- **THEN** it SHALL NOT create a `CircuitBreakerState` instance
- **AND** it SHALL NOT pass `cbState` to `buildFromResolved` or `wireNode`

#### Scenario: wireNode no longer receives cbState parameter
- **WHEN** `wireNode` is called
- **THEN** its signature SHALL NOT include a `cbState` parameter

#### Scenario: nodeOptions no longer receives cbState parameter
- **WHEN** `nodeOptions` is called
- **THEN** its signature SHALL NOT include a `cbState` parameter

#### Scenario: NewGraphAgent variants no longer accept cbState
- **WHEN** `NewGraphAgent`/`NewGraphAgentWithSubAgents`/`NewGraphAgentWithSaver`/`NewGraphAgentWithEngine` are called
- **THEN** their signatures SHALL NOT include a `cbState` parameter

### Requirement: CircuitBreaker compile-time RetryMaxAttempts expansion remains functional
After removing `CircuitBreakerState`, the compile-time `ApplyCircuitBreakerPolicy` SHALL continue to set `RetryMaxAttempts` on affected nodes.

#### Scenario: CircuitBreaker policy sets RetryMaxAttempts at compile time
- **WHEN** a Team has `FailurePolicy.CircuitBreaker` with `MaxRetries=2`
- **THEN** `ApplyCircuitBreakerPolicy` SHALL set `NodeDef.RetryMaxAttempts = 2` on agent/llm/tool nodes
- **AND** this SHALL function identically to before the CircuitBreakerState removal
