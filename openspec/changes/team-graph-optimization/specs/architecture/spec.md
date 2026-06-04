## MODIFIED Requirements

### Requirement: GraphBuildConfig field count
The `GraphBuildConfig` struct SHALL contain 12 fields. The `FailurePolicy *TeamFailurePolicy` and `ParallelBranchIDs []string` fields were never present on `GraphBuildConfig` (they exist on `team.Definition` and as compile-pipeline parameters). The `TaskMeta map[string]NodeTaskMeta` field has been added to carry task metadata through the compilation pipeline.

> **实现偏差（DEV-01/DEV-02）**：设计原定 `TaskMeta` 为 `CompiledTeam` 的独立字段，`GraphBuildConfig` 保持 11 字段纯图拓扑。实际 `TaskMeta` 放在了 `GraphBuildConfig` 上（第 12 字段），`CompiledTeam` 通过值嵌入继承。`FailurePolicy`/`ParallelBranchIDs` 从未在 `GraphBuildConfig` 上定义，"从 13 字段简化为 11 字段"的描述基于错误前提。

#### Scenario: GraphBuildConfig has no Team domain concepts
- **WHEN** `GraphBuildConfig` is defined in `internal/biz/graph.go`
- **THEN** it SHALL NOT contain any field that references Team domain types (`TeamFailurePolicy`, `ParallelBranchIDs`)

#### Scenario: Graph runtime consumes only universal NodeDef fields
- **WHEN** Graph runtime processes a node failure
- **THEN** it SHALL read `NodeDef.FailureAction`, `NodeDef.FallbackAgent`, `NodeDef.RetryMaxAttempts` — NOT `GraphBuildConfig.FailurePolicy`

#### Scenario: TaskMeta accessible via CompiledTeam
- **WHEN** Task coordination logic needs role/assignment information
- **THEN** it SHALL read from `CompiledTeam.TaskMeta[nodeID]` (inherited from `GraphBuildConfig` value embedding)

### Requirement: NodeDef field count
The `NodeDef` struct SHALL contain 20 fields (down from 28). The 8 Task metadata fields SHALL be moved to `NodeTaskMeta`.

#### Scenario: NodeDef contains only graph topology and universal failure fields
- **WHEN** `NodeDef` is defined in `internal/biz/graph.go`
- **THEN** it SHALL NOT contain: `RequiredRole`, `AssignmentMode`, `AssignmentStrategy`, `ReviewerAgent`, `ReviewRules`, `TimeoutSeconds`, `HeartbeatIntervalSeconds`, `EnableLeaseExtension`

#### Scenario: NodeDef retains failure recovery fields
- **WHEN** `NodeDef` is defined in `internal/biz/graph.go`
- **THEN** it SHALL contain: `RetryMaxAttempts`, `FailureAction`, `FallbackAgent` (expanded from FailurePolicy at compile time)
