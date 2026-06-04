## ADDED Requirements

### Requirement: Butler tool hierarchy with coarse-grained entry points
The system SHALL expose exactly 3 coarse-grained butler orchestration tools to Agents: `plan_and_execute`, `check_progress`, and `cancel_orchestration`. The 8 fine-grained tools from system-builtin-agents (classify_industry, search_positions, find_agents_by_position, instantiate_agent_from_position, estimate_task, assemble_team, report_task_result, query_agent_status) SHALL be internal implementation functions invoked by the 3 coarse-grained tools.

#### Scenario: Agent invokes plan_and_execute for a complex task
- **WHEN** an Agent calls `plan_and_execute` with a task description
- **THEN** the system internally executes classify_industry → search_positions → find_agents → estimate_task → assemble_team as sequential steps, and returns the orchestration handle

#### Scenario: Agent invokes check_progress for a running orchestration
- **WHEN** an Agent calls `check_progress` with an orchestration_id
- **THEN** the system internally invokes query_agent_status and returns the aggregated progress

#### Scenario: Agent invokes cancel_orchestration
- **WHEN** an Agent calls `cancel_orchestration` with an orchestration_id
- **THEN** the system internally invokes report_task_result with cancellation status and terminates the orchestration

### Requirement: Unified tool injection path
All butler tools (Spirit 3 tools + Skills Butler 4 tools + Memory Butler tools) SHALL be injected through a single `systemBuiltinTools()` function in `internal/tools/system_builtin_tools.go`. The injection order MUST be deterministic: Spirit tools first, then Skills Butler tools, then Memory Butler tools.

#### Scenario: Spirit agent receives all butler tools
- **WHEN** a `system_builtin` kind Agent is constructed
- **THEN** all tools from `systemBuiltinTools()` are registered in deterministic order

#### Scenario: Non-spirit agent does not receive butler tools
- **WHEN** a regular Agent is constructed
- **THEN** no butler tools are injected

### Requirement: OrchestrationStep type for internal step tracking
The system SHALL define an `OrchestrationStep` type in `internal/biz/types/butler_types.go` that captures the name, input, output, and status of each internal step within a coarse-grained tool invocation.

#### Scenario: plan_and_execute records internal steps
- **WHEN** plan_and_execute completes
- **THEN** each internal step (classify_industry, assemble_team, etc.) is recorded as an OrchestrationStep with its input/output/status

### Requirement: ButlerTier and ButlerCapability type definitions
The system SHALL define `ButlerTier` (spirit / orchestrator / memory / skills / monitor) and `ButlerCapability` (plan / execute / monitor / heal / evolve / recall) enums in `internal/biz/types/butler_types.go`.

#### Scenario: Butler tier lookup
- **WHEN** code needs to identify a butler's tier
- **THEN** it uses `ButlerTier` enum from `biz/types` package
