## ADDED Requirements

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
