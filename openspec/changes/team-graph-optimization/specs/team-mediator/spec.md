## ADDED Requirements

### Requirement: TeamRunMediator structure
The system SHALL define a `TeamRunMediator` struct in `internal/team/runner_mediator.go` that mediates between `Runner` and `TeamGraphRunCoordinator`, eliminating their structural bidirectional dependency.

> **实现偏差（DEV-05）**：`TeamRunMediator` 结构体已定义并实现 `TeamGraphCoordAccess` + `TeamGraphRunFinisher` 双接口，但 Runner 仍直接持有 `teamGraphCoord *TeamGraphRunCoordinator`，Mediator 尚未集成到 Runner 中。Runner 仍有 5 个 Setter 方法（`SetTeamGraphRunCoordinator`/`SetAwaitHookProvider`/`SetRuns`/`SetStreamOptsFactory`/`SetAgentHelper`），DoD（"仅保留 2 个 Setter"）未满足。

#### Scenario: Runner starts graph execution via Mediator
- **WHEN** Runner needs to start a Graph execution
- **THEN** it SHALL call `Mediator.StartGraphRun(ctx, teamRun, compiledTeam)` instead of directly calling `TeamGraphRunCoordinator`

#### Scenario: Coordinator persists results via Mediator
- **WHEN** `TeamGraphRunCoordinator` needs to persist execution results back to Runner
- **THEN** it SHALL call `Mediator.OnGraphRunComplete(ctx, result)` instead of directly calling Runner methods

#### Scenario: Mediator implements dual interfaces
- **WHEN** `TeamRunMediator` is constructed
- **THEN** it SHALL implement both `TeamGraphCoordAccess` (for Runner to call) and `TeamGraphRunFinisher` (for Coordinator to call)

#### Scenario: Mediator uses post-construction wiring
- **WHEN** `NewTeamRunMediator()` creates a mediator
- **THEN** `SetCoordinator` and `SetFinisher` SHALL be called after construction to wire the concrete instances

### Requirement: RunnerConfig replaces non-circular Setters
The system SHALL define a `RunnerConfig` struct in `internal/team/runner_config.go` that consolidates non-circular dependency fields, replacing their individual Setter methods.

> **实现偏差**：`RunnerConfig` 已定义，包含 `GraphLoader`/`TeamGraphTasks`/`AwaitHookProvider`/`Knowledge *KnowledgeFacade`/`StreamOptsFactory`/`AgentHelper`/`Runs`/`GraphRoot`/`PluginRT`/`PluginManager`。但 `AwaitHookProvider`/`Runs`/`StreamOptsFactory`/`AgentHelper` 仍有独立 Setter 方法（它们修改 `RunnerConfig` 字段），尚未完全消除。

#### Scenario: Runner constructed with RunnerConfig
- **WHEN** Wire constructs a Runner
- **THEN** Runner SHALL receive `RunnerConfig` as a constructor parameter containing: `GraphLoader`, `TeamGraphTasks`, `AwaitHookProvider`, `Knowledge`, `StreamOptsFactory`, `AgentHelper`, `Runs`, `GraphRoot`, `PluginRT`, `PluginManager`

#### Scenario: Remaining Setters modify RunnerConfig fields
- **WHEN** `SetAwaitHookProvider`/`SetRuns`/`SetStreamOptsFactory`/`SetAgentHelper` are called
- **THEN** they SHALL modify the corresponding fields in `Runner.cfg` (the embedded `RunnerConfig`)

#### Scenario: Only circular-dependency Setters remain (target)
- **WHEN** Runner is fully refactored with RunnerConfig and Mediator
- **THEN** only `SetTeamGraphRunCoordinator` SHALL remain as a Setter method (circular dependency); all other Setters SHALL be removed

### Requirement: KnowledgeFacade encapsulation
The system SHALL encapsulate 4 Knowledge fields (`KnowledgeRetriever`, `KnowledgeRouter`, `KnowledgeFederatedRetriever`, `KnowledgeEvaluator`) into a `KnowledgeFacade` struct.

#### Scenario: Runner holds single KnowledgeFacade
- **WHEN** Runner is refactored
- **THEN** it SHALL hold a single `cfg.Knowledge *KnowledgeFacade` field instead of 4 individual knowledge fields
