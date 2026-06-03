## ADDED Requirements

### Requirement: Bus.logDrop uses stderr directly
Bus.logDrop() SHALL write to os.Stderr directly using fmt.Fprintf, NOT through loggateway.Logger. This structurally prevents the feedback loop: Bus.logDrop → Gateway.Warn → Pipeline → EventBusSink → Bus.Publish.

#### Scenario: Message drop notification does not enter Pipeline
- **WHEN** Bus.deliverToSubscriber() drops a message due to buffer overflow
- **AND** Bus.logDrop() is called
- **THEN** the drop notification SHALL be written to os.Stderr
- **AND** loggateway.Logger.Warn() SHALL NOT be called
- **AND** the notification SHALL NOT enter logpipeline.Pipeline

#### Scenario: Drop notification format
- **WHEN** Bus.logDrop() writes a drop notification
- **THEN** the format SHALL include: timestamp, envelope type, subscriber ID, reason
- **AND** the format SHALL be parseable by standard log analysis tools

### Requirement: Bus droppedCount counter
Bus SHALL maintain an atomic droppedCount counter that increments on every message drop. This counter SHALL be exposed via Pipeline.Stats() or a metrics endpoint, providing visibility into drop events without producing new log entries.

#### Scenario: droppedCount increments on drop
- **WHEN** Bus.deliverToSubscriber() drops a message
- **THEN** Bus.droppedCount SHALL increment by 1
- **AND** no new log entry SHALL be produced

#### Scenario: droppedCount visible in stats
- **WHEN** Pipeline.Stats() is called
- **THEN** the result SHALL include Bus.droppedCount

### Requirement: EventBusSink circuit breaker with half-open probing
EventBusSink SHALL implement a circuit breaker that pauses publishing after consecutive timeout failures. The half-open state SHALL allow 3 probe attempts before re-opening, preventing premature re-closing.

#### Scenario: Circuit breaker opens after consecutive failures
- **WHEN** EventBusSink.Publish() times out 5 consecutive times
- **THEN** the circuit breaker SHALL open
- **AND** subsequent Publish calls SHALL be skipped for 10 seconds
- **AND** skipped entries SHALL increment a circuit_breaker_skipped counter

#### Scenario: Circuit breaker enters half-open state after cooldown
- **WHEN** the circuit breaker has been open for 10 seconds
- **THEN** the circuit breaker SHALL enter half-open state
- **AND** the next Publish call SHALL be attempted
- **AND** halfOpenAttempts SHALL be set to 1

#### Scenario: Half-open probe succeeds
- **WHEN** a half-open Publish call succeeds
- **THEN** the circuit breaker SHALL close
- **AND** halfOpenAttempts SHALL be reset to 0
- **AND** failures SHALL be reset to 0

#### Scenario: Half-open probe fails, but attempts < 3
- **WHEN** a half-open Publish call times out
- **AND** halfOpenAttempts < 3
- **THEN** halfOpenAttempts SHALL increment by 1
- **AND** the circuit breaker SHALL remain in half-open state
- **AND** the next Publish call SHALL still be attempted

#### Scenario: Half-open probe fails 3 times
- **WHEN** half-open Publish calls time out 3 consecutive times
- **THEN** the circuit breaker SHALL re-open for another 10 seconds
- **AND** halfOpenAttempts SHALL be reset to 0

#### Scenario: Circuit breaker does not affect other Sinks
- **WHEN** EventBusSink's circuit breaker is open
- **THEN** FileSink and StdoutSink SHALL continue processing entries normally
- **AND** no entries SHALL be dropped from FileSink/StdoutSink due to EventBusSink's circuit breaker

### Requirement: EventBusSink circuit breaker metrics
EventBusSink SHALL expose circuit breaker state via Pipeline.Stats() for monitoring.

#### Scenario: Stats include circuit breaker state
- **WHEN** Pipeline.Stats() is called
- **THEN** the result SHALL include EventBusSink circuit_breaker_open (bool), circuit_breaker_skipped (uint64), and half_open_attempts (int64)
