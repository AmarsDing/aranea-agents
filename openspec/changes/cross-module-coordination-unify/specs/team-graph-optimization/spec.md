## MODIFIED Requirements

### Requirement: CompiledTeam persistence uses three-table session schema
The `CompiledTeam` Ent schema and its repository SHALL use the three-table session structure (sessions / session_metrics / session_runtime) for any session-related queries. CompiledTeam records SHALL reference `session_id` and use SessionRuntimeRepo for status checks.

#### Scenario: CompiledTeam repo queries session status
- **WHEN** CompiledTeamRepo needs to check if a session is still running
- **THEN** it queries SessionRuntimeRepo for the session status, not the sessions table directly

### Requirement: GraphBuilderFactory split aligned with TaskOrchestrator
The `GraphBuilderFactory` SHALL be split into 4 narrow interfaces (DefinitionFactory, ExecutionFactory, CacheManager, TeamMediator) that align with TaskOrchestrator's DAGToGraphCompiler. The `DAGToGraphCompiler` SHALL use `DefinitionFactory` for graph compilation.

#### Scenario: DAGToGraphCompiler uses DefinitionFactory
- **WHEN** TaskOrchestrator compiles a DAG to a Graph
- **THEN** it uses `DefinitionFactory.BuildDefinition()` from the split GraphBuilderFactory

### Requirement: Team state machine aligned with SessionStatusMachine
The Team state machine (pending / running / completed / failed / cancelled / interrupted) SHALL be aligned with `SessionStatusMachine`. Team status transitions MUST trigger corresponding session status transitions via SessionRuntimeRepo.

#### Scenario: Team transitions to running
- **WHEN** a Team transitions to `running` state
- **THEN** the corresponding session status transitions to `running` via SessionRuntimeRepo

#### Scenario: Team transitions to failed
- **WHEN** a Team transitions to `failed` state
- **THEN** the corresponding session status transitions to `interrupted` via SessionRuntimeRepo

### Requirement: NodeDef field reduction preserved
The `NodeDef` struct SHALL maintain its reduced field count (20 fields, down from 28). No new fields SHALL be added without a corresponding removal of an existing field.

#### Scenario: NodeDef field count check
- **WHEN** a developer adds a field to NodeDef
- **THEN** the total field count MUST NOT exceed 20

### Requirement: FailurePolicy compile-time expansion
The `FailurePolicy` SHALL be expanded at compile time (when building CompiledTeam), not at runtime. The compiled Graph SHALL contain the expanded failure handling logic, not the abstract FailurePolicy enum.

#### Scenario: FailurePolicy expanded in CompiledTeam
- **WHEN** a Team with FailurePolicy=Retry is compiled
- **THEN** the resulting Graph contains explicit retry nodes, not a FailurePolicy enum value
