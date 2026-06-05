## ADDED Requirements

### Requirement: CompiledTeam structure
The system SHALL define a `CompiledTeam` struct in `internal/biz/compiled_team.go` that embeds `GraphBuildConfig` (value embedding), contains `RoleManifest map[string]RoleInfo`, `OriginalPolicy *TeamFailurePolicy`, and `CompiledAt time.Time`.

> **实现偏差（DEV-01）**：`TaskMeta map[string]NodeTaskMeta` 实际位于 `GraphBuildConfig` 上（第 12 字段），而非 `CompiledTeam` 的独立字段。由于 `CompiledTeam` 值嵌入 `GraphBuildConfig`，`CompiledTeam.TaskMeta` 仍可访问，功能等价。

#### Scenario: CompiledTeam contains pure graph topology
- **WHEN** a Team Definition is compiled via `CompileToCompiledTeam`
- **THEN** the resulting `CompiledTeam.GraphBuildConfig` SHALL NOT contain `FailurePolicy` or `ParallelBranchIDs` fields (these were never on `GraphBuildConfig`; they exist on `team.Definition`)

#### Scenario: CompiledTeam preserves original policy
- **WHEN** a Team Definition with FailurePolicy is compiled
- **THEN** `CompiledTeam.OriginalPolicy` SHALL retain the original `TeamFailurePolicy` object for query and debugging purposes

#### Scenario: CompiledTeam records compilation timestamp
- **WHEN** a Team Definition is compiled via `CompileToCompiledTeam`
- **THEN** `CompiledTeam.CompiledAt` SHALL be set to the compilation time

#### Scenario: CompiledTeam provides node-level accessors
- **WHEN** runtime code needs task metadata or role info for a specific node
- **THEN** `CompiledTeam.TaskMetaForNode(nodeID)` SHALL return the `NodeTaskMeta` for that node
- **AND** `CompiledTeam.RoleForNode(nodeID)` SHALL return the `RoleInfo` for that node

### Requirement: NodeTaskMeta separation
The system SHALL define `NodeTaskMeta` struct containing 8 fields: `RequiredRole`, `AssignmentMode`, `AssignmentStrategy`, `ReviewerAgent`, `ReviewRules`, `TimeoutSeconds`, `HeartbeatIntervalSeconds`, `EnableLeaseExtension`. These fields SHALL be removed from `NodeDef`.

#### Scenario: NodeDef no longer contains Task fields
- **WHEN** `CompiledTeam` is produced by compilation
- **THEN** `NodeDef` SHALL have 20 fields (down from 28), with Task metadata accessible via `CompiledTeam.TaskMeta[nodeID]`

#### Scenario: Task coordination logic consumes NodeTaskMeta
- **WHEN** Task coordination logic needs role/assignment information
- **THEN** it SHALL read from `CompiledTeam.TaskMeta` instead of `NodeDef`

### Requirement: FailurePolicy compile-time expansion
The system SHALL expand `FailurePolicy` into `NodeDef` universal fields during compilation. After expansion, `GraphBuildConfig` SHALL NOT contain `FailurePolicy` or `ParallelBranchIDs` (these fields were never on `GraphBuildConfig`; they exist on `team.Definition`).

> **实现偏差（DEV-03）**：`graph/trpc/node_wiring.go` 的 `nodeOptions` 仍接收 `*biz.TeamFailurePolicy` 参数（用于 CircuitBreaker），Goal 4（"Graph 运行时零 Team 类型依赖"）未完全达成。

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
- **WHEN** a Team has parallel branches with `parallel_fail=continue`
- **THEN** parallel branch nodes SHALL have `FailureAction = "skip_on_failure"` via `ApplyParallelFailContinue`

#### Scenario: CircuitBreaker expansion (partial)
- **WHEN** a Team has `FailurePolicy.CircuitBreaker` with `FailureThreshold > 0`
- **THEN** `ApplyCircuitBreakerPolicy` SHALL set `RetryMaxAttempts` on agent/llm/tool nodes
- **BUT** `graph/trpc/node_wiring.go` SHALL still receive `*biz.TeamFailurePolicy` for runtime circuit breaker callback registration (not yet fully decoupled)

### Requirement: CompileToCompiledTeam pipeline
The system SHALL provide `CompileToCompiledTeam` that produces a `CompiledTeam` instead of `GraphBuildConfig`. The legacy `CompileToGraphRuntimeConfig` SHALL be retained as a delegate wrapper for backward compatibility.

> **实现偏差（DEV-08/DEV-15）**：`CompileToGraphRuntimeConfig` 保留为委托包装（调用 `CompileToCompiledTeam` 后取 `.GraphBuildConfig`），`CompileToGraphRuntimeConfigFromJSON` 也委托给 `CompileToCompiledTeam`，但返回类型为 `*biz.CompiledTeam`（而非 `biz.GraphBuildConfig`），函数名与返回类型不一致。调用方 `runner_team_compiler.go` 直接使用 `CompiledTeam` 返回值。

#### Scenario: Compilation produces CompiledTeam
- **WHEN** `CompileToCompiledTeam` is called with a Team Definition
- **THEN** it SHALL return a `CompiledTeam` containing `GraphBuildConfig` (pure topology), `TaskMeta`, `RoleManifest`, `OriginalPolicy`, and `CompiledAt`

#### Scenario: Legacy wrapper delegates to CompileToCompiledTeam
- **WHEN** `CompileToGraphRuntimeConfig` is called
- **THEN** it SHALL delegate to `CompileToCompiledTeam` and return `ct.GraphBuildConfig`

#### Scenario: Linked graph compilation
- **WHEN** a Team Definition has a `linked_graph_id` and a `GraphBuildConfigLoader` is provided
- **THEN** `CompileToCompiledTeam` SHALL load the persisted graph asset and apply failure policy on top

#### Scenario: Adaptive mode compilation
- **WHEN** a Team Definition has mode "swarm" or "adaptive"
- **THEN** `CompileToCompiledTeam` SHALL apply `applyAdaptiveAgentDestinations` to move transfer edges into node Destinations

### Requirement: CompiledTeam persistence
The system SHALL persist `CompiledTeam` to SQLite via a new `CompiledTeamRepo`, replacing the in-memory `teamBuildConfigs` cache as the primary recovery mechanism. The in-memory cache SHALL be retained as a hot-path optimization.

> **实现偏差（DEV-04）**：同时存在 Ent Schema（`internal/data/ent/schema/compiled_team.go`，用于文档/DDL 追踪）和手写 SQL DDL（`compiled_team_schema.go` 中的 `EnsureCompiledTeamSchema`），实际 CRUD 使用手写 SQL。表结构包含 `id`/`team_id`/`graph_id`/`session_id`/`config_json`/`created_at`/`updated_at`，比设计多了 `session_id` 和 `id`（复合主键）字段。`LoadForSession` 方法额外校验 session 活跃状态，这是设计文档未提及的安全增强。DDL 迁移通过 `ddl_migration_registry.go` 注册（版本 20260705 初始建表 + 版本 20260714 新增 `session_id` 字段）。

#### Scenario: Team graph survives process restart
- **WHEN** a Team Graph is compiled and persisted
- **THEN** after process restart, `buildConfigForExecution` SHALL successfully load the `CompiledTeam` from DB

#### Scenario: GC eviction no longer breaks resume
- **WHEN** a `team:` prefixed graph is evicted by GC
- **THEN** resume SHALL succeed by loading `CompiledTeam` from persistent storage

#### Scenario: CompiledTeam uses biz-layer types for persistence
- **WHEN** `CompiledTeam` is serialized to JSON for persistence
- **THEN** it SHALL use biz-layer `GraphBuildConfig` (with `FuncRef string` references, not function pointers)

#### Scenario: Dual-write to memory and DB
- **WHEN** `RegisterTeamGraphExecution` is called
- **THEN** the `CompiledTeam` SHALL be written to both the in-memory `teamBuildConfigs` map and the persistent `CompiledTeamRepo`

#### Scenario: LoadForSession validates session liveness
- **WHEN** `CompiledTeamRepo.LoadForSession` is called with a sessionID
- **THEN** it SHALL verify the session is still active via `SessionRuntimeReader` before returning the compiled team

#### Scenario: buildConfigForExecution fallback chain
- **WHEN** `buildConfigForExecution` is called for a team graph
- **THEN** it SHALL first check in-memory cache, then DB (via `compiledTeamRepo.LoadForSession`), then fall back to recompiling from persisted graph definition

### Requirement: CompileToGraphBuildConfig pure topology compilation
The system SHALL provide `CompileToGraphBuildConfig` and `CompileToGraphBuildConfigFromJSON` that produce `biz.GraphBuildConfig` directly (without `CompiledTeam` wrapping), for scenarios that only need graph topology (visualization, template generation). These functions SHALL NOT apply `finalizeRuntimeGraphConfig` (no FailurePolicy expansion, no RoleManifest generation).

> **实现偏差（DEV-16）**：设计文档未记录这些函数，但它们作为"纯图拓扑编译"入口存在于 `graph_compile.go`，与 `CompileToCompiledTeam`（完整编译）互补。

#### Scenario: Pure topology compilation for visualization
- **WHEN** `CompileToGraphBuildConfig` is called with a Team Definition
- **THEN** it SHALL return `biz.GraphBuildConfig` without FailurePolicy expansion or RoleManifest

#### Scenario: Shared compilation logic
- **WHEN** `CompileToGraphBuildConfig` compiles a definition
- **THEN** it SHALL use the same `compileToGraphBuildConfigWithLoader` as `CompileToCompiledTeam`
