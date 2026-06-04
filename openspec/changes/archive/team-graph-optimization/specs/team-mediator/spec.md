## ADDED Requirements

### Requirement: TeamRunMediator structure
The system SHALL define a `TeamRunMediator` struct in `internal/team/runner_mediator.go` that mediates between `Runner` and `TeamGraphRunCoordinator`, eliminating their structural bidirectional dependency.

#### Scenario: Runner starts graph execution via Mediator
- **WHEN** Runner needs to start a Graph execution
- **THEN** it SHALL call `Mediator.StartGraphRun(ctx, teamRun, compiledTeam)` instead of directly calling `TeamGraphRunCoordinator`

#### Scenario: Coordinator persists results via Mediator
- **WHEN** `TeamGraphRunCoordinator` needs to persist execution results back to Runner
- **THEN** it SHALL call `Mediator.OnGraphRunComplete(ctx, result)` instead of directly calling Runner methods

### Requirement: RunnerConfig replaces non-circular Setters
The system SHALL define a `RunnerConfig` struct in `internal/team/runner_config.go` that consolidates 10 non-circular dependency fields, replacing their individual Setter methods.

#### Scenario: Runner constructed with RunnerConfig
- **WHEN** Wire constructs a Runner
- **THEN** Runner SHALL receive `RunnerConfig` as a constructor parameter containing: `GraphBuildConfigLoader`, `TeamGraphTaskCreator`, `AwaitHookProvider`, `KnowledgeRetriever`, `KnowledgeRouter`, `KnowledgeFederatedRetriever`, `KnowledgeEvaluator`, `StreamOptsFactory`, `AgentHelper`, `RunRegistry`, `GraphRootBuilder`

#### Scenario: Only 2 Setters remain
- **WHEN** Runner is refactored with RunnerConfig and Mediator
- **THEN** only `SetTeamGraphRunCoordinator` and `SetGraphRootBuilder` (if still circular) SHALL remain as Setter methods; all other Setters SHALL be removed

### Requirement: KnowledgeFacade encapsulation
The system SHALL encapsulate 4 Knowledge fields (`KnowledgeRetriever`, `KnowledgeRouter`, `KnowledgeFederatedRetriever`, `KnowledgeEvaluator`) into a `KnowledgeFacade` struct.

#### Scenario: Runner holds single KnowledgeFacade
- **WHEN** Runner is refactored
- **THEN** it SHALL hold a single `knowledgeFacade *KnowledgeFacade` field instead of 4 individual knowledge fields
