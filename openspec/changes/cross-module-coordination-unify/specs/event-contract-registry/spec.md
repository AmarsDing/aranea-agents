## ADDED Requirements

### Requirement: Centralized EnvelopeType registration
All cross-module EnvelopeType constants SHALL be defined in `internal/event/envelope.go`. No other package SHALL define EnvelopeType constants. New event types MUST follow the naming convention `{Domain}_{Action}` (e.g., `EnvelopeTypeButlerOrchestrationStarted`).

#### Scenario: New event type registration
- **WHEN** a module needs a new cross-module event type
- **THEN** it adds the constant to `internal/event/envelope.go` with the `{Domain}_{Action}` naming convention

#### Scenario: Grep verification of event type uniqueness
- **WHEN** a developer searches for `EnvelopeType` constant definitions
- **THEN** all definitions are found only in `internal/event/envelope.go`

### Requirement: Butler orchestration event types
The system SHALL define the following EnvelopeType constants in `internal/event/envelope.go`:
- `EnvelopeTypeButlerOrchestrationStarted`
- `EnvelopeTypeButlerOrchestrationCompleted`
- `EnvelopeTypeButlerOrchestrationFailed`

#### Scenario: plan_and_execute emits orchestration started event
- **WHEN** `plan_and_execute` begins execution
- **THEN** an event with `EnvelopeTypeButlerOrchestrationStarted` is published to EventBus

#### Scenario: plan_and_execute emits orchestration completed event
- **WHEN** `plan_and_execute` completes successfully
- **THEN** an event with `EnvelopeTypeButlerOrchestrationCompleted` is published to EventBus

### Requirement: Skill evolution event types
The system SHALL define the following EnvelopeType constants in `internal/event/envelope.go`:
- `EnvelopeTypeSkillHealthChanged`
- `EnvelopeTypeSkillEvolutionProposed`

#### Scenario: Skill health drops below threshold
- **WHEN** a skill's health score drops below 0.5
- **THEN** an event with `EnvelopeTypeSkillHealthChanged` is published to EventBus

#### Scenario: New skill evolution is proposed
- **WHEN** the auto-creator detects a repeat pattern and proposes a new skill
- **THEN** an event with `EnvelopeTypeSkillEvolutionProposed` is published to EventBus

### Requirement: Monitor event types
The system SHALL define the following EnvelopeType constants in `internal/event/envelope.go`:
- `EnvelopeTypeMonitorAutoHealed`
- `EnvelopeTypeMonitorSelfCheckCompleted`

#### Scenario: Runtime auto-heal succeeds
- **WHEN** a runtime auto-heal strategy successfully recovers from an error
- **THEN** an event with `EnvelopeTypeMonitorAutoHealed` is published to EventBus

#### Scenario: Self-check cycle completes
- **WHEN** a self-check cycle finishes all checkers
- **THEN** an event with `EnvelopeTypeMonitorSelfCheckCompleted` is published to EventBus
