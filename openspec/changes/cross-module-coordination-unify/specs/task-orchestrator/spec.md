## MODIFIED Requirements

### Requirement: TaskOrchestrator uses coarse-grained butler tools
The `TaskOrchestrator` SHALL invoke `plan_and_execute` as the single entry point for orchestration, instead of directly calling fine-grained tools. Internal orchestration steps (classify_industry, assemble_team, etc.) are handled within `plan_and_execute` and are not visible to TaskOrchestrator.

#### Scenario: TaskOrchestrator triggers orchestration
- **WHEN** TaskOrchestrator receives a complex task
- **THEN** it invokes `plan_and_execute` tool with the task description and receives an OrchestrationHandle

#### Scenario: TaskOrchestrator checks progress
- **WHEN** TaskOrchestrator needs to check orchestration progress
- **THEN** it invokes `check_progress` tool with the orchestration_id

#### Scenario: TaskOrchestrator cancels orchestration
- **WHEN** TaskOrchestrator needs to cancel an orchestration
- **THEN** it invokes `cancel_orchestration` tool with the orchestration_id

### Requirement: TaskOrchestrator uses biz/types for shared types
The `TaskOrchestrator` SHALL import `TaskPlan`, `AllocationPlan`, and `OrchestrationHandle` types from `internal/biz/types/` if they are shared across modules, instead of defining them locally.

#### Scenario: TaskPlan type source
- **WHEN** TaskOrchestrator creates a TaskPlan
- **THEN** it uses the `TaskPlan` type from `internal/biz/types/butler_types.go`
