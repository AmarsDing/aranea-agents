## MODIFIED Requirements

### Requirement: GraphBuildConfig field count
The `GraphBuildConfig` struct in `internal/biz/graph.go` SHALL contain 12 fields. The `FailurePolicy *TeamFailurePolicy` and `ParallelBranchIDs []string` fields were never present on `GraphBuildConfig` (they exist on `team.Definition` and as compile-pipeline parameters). The `TaskMeta map[string]NodeTaskMeta` field has been added to carry task metadata through the compilation pipeline.

The `GraphBuildConfig` struct in `internal/graph/trpc/builder.go` SHALL contain 11 fields (no `TaskMeta`). It is a separate struct from biz-layer `GraphBuildConfig` and requires `bizCfgToTrpc`/`trpcCfgToBiz` conversion functions.

> **实现偏差（DEV-01/DEV-02/DEV-13）**：设计原定 `TaskMeta` 为 `CompiledTeam` 的独立字段，`GraphBuildConfig` 保持 11 字段纯图拓扑。实际 `TaskMeta` 放在了 `GraphBuildConfig` 上（第 12 字段），`CompiledTeam` 通过值嵌入继承。`FailurePolicy`/`ParallelBranchIDs` 从未在 `GraphBuildConfig` 上定义，"从 13 字段简化为 11 字段"的描述基于错误前提。trpc 层 `GraphBuildConfig` 仍为独立结构体（11 字段），与 biz 层不同，这是双源定义未消除的根本原因。

> **DDL 迁移机制**：`compiled_teams` 表通过 `ddl_migration_registry.go` 管理版本化 DDL 迁移（版本 20260705 初始建表 + 版本 20260714 新增 `session_id` 字段）。同时存在 Ent Schema（`internal/data/ent/schema/compiled_team.go`）用于文档/DDL 追踪，实际 CRUD 使用手写 SQL。

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
