## ADDED Requirements

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
