## MODIFIED Requirements

### Requirement: TeamUsecase dependency injection
TeamUsecase SHALL receive its repository dependencies as individual sub-interfaces (TeamReader, TeamWriter, TeamRunReader, TeamRunWriter, OrchestrationStepRepo, TaskDeadLetterRepo) instead of the aggregated TeamRepository interface.

#### Scenario: Constructor receives sub-interfaces
- **WHEN** `NewTeamUsecase` is called
- **THEN** it accepts 6 sub-interface parameters plus AgentIDExistenceChecker, and stores each in a separate field

#### Scenario: Wire binds sub-interfaces
- **WHEN** Wire resolves TeamUsecase dependencies
- **THEN** all 6 sub-interfaces are bound to the same TeamRepository concrete type via `wire.Bind`

#### Scenario: Usecase methods use narrow fields
- **WHEN** a TeamUsecase method needs to read teams
- **THEN** it uses `uc.reader` (TeamReader) instead of `uc.repo` (TeamRepository)
