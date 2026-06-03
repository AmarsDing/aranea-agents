## MODIFIED Requirements

### Requirement: GraphBuildConfig field count
The `GraphBuildConfig` struct SHALL contain 11 fields (down from 13). The `FailurePolicy *TeamFailurePolicy` and `ParallelBranchIDs []string` fields SHALL be removed.

#### Scenario: GraphBuildConfig has no Team domain concepts
- **WHEN** `GraphBuildConfig` is defined in `internal/biz/graph.go`
- **THEN** it SHALL NOT contain any field that references Team domain types (`TeamFailurePolicy`, `ParallelBranchIDs`)

#### Scenario: Graph runtime consumes only universal NodeDef fields
- **WHEN** Graph runtime processes a node failure
- **THEN** it SHALL read `NodeDef.FailureAction`, `NodeDef.FallbackAgent`, `NodeDef.RetryMaxAttempts` — NOT `GraphBuildConfig.FailurePolicy`

### Requirement: NodeDef field count
The `NodeDef` struct SHALL contain 20 fields (down from 28). The 8 Task metadata fields SHALL be moved to `NodeTaskMeta`.

#### Scenario: NodeDef contains only graph topology and universal failure fields
- **WHEN** `NodeDef` is defined in `internal/biz/graph.go`
- **THEN** it SHALL NOT contain: `RequiredRole`, `AssignmentMode`, `AssignmentStrategy`, `ReviewerAgent`, `ReviewRules`, `TimeoutSeconds`, `HeartbeatIntervalSeconds`, `EnableLeaseExtension`
