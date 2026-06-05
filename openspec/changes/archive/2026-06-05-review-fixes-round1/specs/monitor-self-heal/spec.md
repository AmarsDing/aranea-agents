## ADDED Requirements

### Requirement: SelfHealObserver constructor nil handling
NewSelfHealObserver SHALL return (*SelfHealObserver, error) instead of *SelfHealObserver. When repo or engine is nil, it SHALL return (nil, error) with a descriptive kerrors message.

#### Scenario: Nil repo returns error
- **WHEN** NewSelfHealObserver is called with nil repo
- **THEN** it returns (nil, kerrors.InternalServer("MONITOR", "HealRecordRepo is required"))

#### Scenario: Nil engine returns error
- **WHEN** NewSelfHealObserver is called with nil engine
- **THEN** it returns (nil, kerrors.InternalServer("MONITOR", "RootCauseEngine is required"))

#### Scenario: Valid arguments return observer
- **WHEN** NewSelfHealObserver is called with valid repo, engine, notifier, and logger
- **THEN** it returns a non-nil *SelfHealObserver and nil error

### Requirement: SelfHealObserver lock granularity optimization
ObserveFlowLogEvent SHALL use a single lock interval for both the success branch (delete failCounts) and the failure branch (increment failCounts), instead of two separate lock/unlock cycles.

#### Scenario: Success branch single lock
- **WHEN** ObserveFlowLogEvent processes a healed event
- **THEN** it acquires the mutex once, deletes the failCounts entry, and releases the mutex once

#### Scenario: Failure branch single lock
- **WHEN** ObserveFlowLogEvent processes a failed heal event
- **THEN** it acquires the mutex once, increments failCounts, reads the count, and releases the mutex once

### Requirement: SeverityCooldown read-only access
SeverityCooldown SHALL be an unexported variable. External code SHALL access cooldown durations through the exported function GetSeverityCooldown(severity string) time.Duration.

#### Scenario: Access cooldown via function
- **WHEN** external code needs the cooldown duration for a severity level
- **THEN** it calls GetSeverityCooldown("critical") and receives the corresponding duration

#### Scenario: Unknown severity returns default
- **WHEN** GetSeverityCooldown is called with an unknown severity
- **THEN** it returns the "medium" cooldown duration as default
