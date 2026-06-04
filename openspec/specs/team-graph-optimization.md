# Team Graph Optimization

## Compiled Team

### Requirement: CompiledTeam structure
The system SHALL define a `CompiledTeam` struct in `internal/biz/compiled_team.go` that embeds `GraphBuildConfig` (value embedding), contains `TaskMeta map[string]NodeTaskMeta`, `RoleManifest map[string]RoleInfo`, and `OriginalPolicy *TeamFailurePolicy`.

#### Scenario: CompiledTeam contains pure graph topology
- **WHEN** a Team Definition is compiled via `CompileToCompiledTeam`
- **THEN** the resulting `CompiledTeam.GraphBuildConfig` SHALL NOT contain `FailurePolicy` or `ParallelBranchIDs` fields

#### Scenario: CompiledTeam preserves original policy
- **WHEN** a Team Definition with FailurePolicy is compiled
- **THEN** `CompiledTeam.OriginalPolicy` SHALL retain the original `TeamFailurePolicy` object for query and debugging purposes

### Requirement: NodeTaskMeta separation
The system SHALL define `NodeTaskMeta` struct containing 8 fields: `RequiredRole`, `AssignmentMode`, `AssignmentStrategy`, `ReviewerAgent`, `ReviewRules`, `TimeoutSeconds`, `HeartbeatIntervalSeconds`, `EnableLeaseExtension`. These fields SHALL be removed from `NodeDef`.

#### Scenario: NodeDef no longer contains Task fields
- **WHEN** `CompiledTeam` is produced by compilation
- **THEN** `NodeDef` SHALL have 20 fields (down from 28), with Task metadata accessible via `CompiledTeam.TaskMeta[nodeID]`

#### Scenario: Task coordination logic consumes NodeTaskMeta
- **WHEN** Task coordination logic needs role/assignment information
- **THEN** it SHALL read from `CompiledTeam.TaskMeta` instead of `NodeDef`

### Requirement: FailurePolicy compile-time expansion
The system SHALL expand `FailurePolicy` into `NodeDef` universal fields during compilation. After expansion, `GraphBuildConfig` SHALL NOT contain `FailurePolicy` or `ParallelBranchIDs`.

#### Scenario: Retry policy expansion
- **WHEN** a Team has `FailurePolicy.Retry` with `MaxRetries=3`
- **THEN** each affected `NodeDef.RetryMaxAttempts` SHALL be set to 3

#### Scenario: Failover policy expansion
- **WHEN** a Team has `FailurePolicy.Failover` with `FallbackAgentID="backup-agent"`
- **THEN** each affected `NodeDef.FailureAction` SHALL be "failover" and `NodeDef.FallbackAgent` SHALL be "backup-agent"

#### Scenario: Skip policy expansion
- **WHEN** a Team has `FailurePolicy.Skip`
- **THEN** each affected `NodeDef.FailureAction` SHALL be "skip_on_failure"

#### Scenario: ParallelBranchIDs expansion
- **WHEN** a Team has parallel branches
- **THEN** parallel branch nodes SHALL have `FailureAction = "skip_on_failure"` and `ParallelBranchIDs` SHALL be removed from `GraphBuildConfig`

### Requirement: CompileToCompiledTeam pipeline
The system SHALL replace `CompileToGraphRuntimeConfig` with `CompileToCompiledTeam` that produces a `CompiledTeam` instead of `GraphBuildConfig`.

#### Scenario: Compilation produces CompiledTeam
- **WHEN** `CompileToCompiledTeam` is called with a Team Definition
- **THEN** it SHALL return a `CompiledTeam` containing `GraphBuildConfig` (pure topology), `TaskMeta`, `RoleManifest`, and `OriginalPolicy`

### Requirement: CompiledTeam persistence
The system SHALL persist `CompiledTeam` to SQLite via a new `CompiledTeamRepo`, replacing the in-memory `teamBuildConfigs` cache.

#### Scenario: Team graph survives process restart
- **WHEN** a Team Graph is compiled and persisted
- **THEN** after process restart, `buildConfigForExecution` SHALL successfully load the `CompiledTeam` from DB

#### Scenario: GC eviction no longer breaks resume
- **WHEN** a `team:` prefixed graph is evicted by GC
- **THEN** resume SHALL succeed by loading `CompiledTeam` from persistent storage

#### Scenario: CompiledTeam uses biz-layer types for persistence
- **WHEN** `CompiledTeam` is serialized to JSON for persistence
- **THEN** it SHALL use biz-layer `GraphBuildConfig` (with `FuncRef string` references, not function pointers)

## Role Manifest

### Requirement: RoleManifest generation
The system SHALL generate a `RoleManifest map[string]RoleInfo` during Team compilation, mapping each nodeID to role information.

#### Scenario: Role info populated from catalog
- **WHEN** `CompileToCompiledTeam` processes a Team Definition
- **THEN** for each node with an `AgentName`, `RoleManifest[nodeID]` SHALL contain `AgentID`, `AgentKey`, `DisplayName`, and `Role` fields populated from the agent catalog

### Requirement: RoleInfo structure
The system SHALL define `RoleInfo` struct with fields: `AgentID string`, `AgentKey string`, `DisplayName string`, `Role string`, `Capabilities []string`.

#### Scenario: AgentKey matches runtime value
- **WHEN** a node is compiled with agent "reviewer"
- **THEN** `RoleInfo.AgentKey` SHALL match the runtime `ag.AgentKey` value, NOT a hardcoded "key-" prefix

### Requirement: Role semantic queryability
The system SHALL make `RoleManifest` queryable at runtime for frontend display and logging.

#### Scenario: Frontend displays role name
- **WHEN** frontend requests execution status for a Team Graph
- **THEN** the system SHALL return role information from `CompiledTeam.RoleManifest` so that "which role is executing" can be displayed

#### Scenario: Logging traces role decisions
- **WHEN** a graph node executes and produces a log entry
- **THEN** the log SHALL be correlatable to `RoleManifest[nodeID].Role` for decision path tracing

### Requirement: Capabilities field reservation
The `RoleInfo.Capabilities` field SHALL be reserved for future runtime dynamic scheduling. The system SHALL NOT implement dynamic scheduling in this change.

#### Scenario: Capabilities field is empty
- **WHEN** `RoleManifest` is generated during compilation
- **THEN** `Capabilities` SHALL be an empty slice `[]string{}`

## Team Mediator

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
