## MODIFIED Requirements

### Requirement: SessionUsecase constructor completeness
SessionUsecase SHALL receive all dependencies through its constructor function, including SessionStatusPublisher, MetricsUpdatedPublisher, and SessionRuntimeWriter. The setter methods `SetStatusPublisher`, `SetMetricsUpdatedPublisher`, and `SetRuntimeWriter` SHALL be removed.

#### Scenario: Constructor receives all dependencies
- **WHEN** `NewSessionUsecase` is called
- **THEN** it accepts statusPublisher, metricsPublisher, and runtimeWriter as parameters

#### Scenario: Setter methods removed
- **WHEN** code attempts to call `SetStatusPublisher`, `SetMetricsUpdatedPublisher`, or `SetRuntimeWriter`
- **THEN** compilation fails because these methods no longer exist

#### Scenario: Wire provides all dependencies
- **WHEN** Wire resolves SessionUsecase
- **THEN** it injects statusPublisher, metricsPublisher, and runtimeWriter through the constructor
