## MODIFIED Requirements

### Requirement: Spirit tools are coarse-grained entry points only
The 3 Spirit tools (`plan_and_execute`, `check_progress`, `cancel_orchestration`) SHALL be the only tools exposed to Agents. The 8 fine-grained orchestration functions (classify_industry, search_positions, find_agents_by_position, instantiate_agent_from_position, estimate_task, assemble_team, report_task_result, query_agent_status) SHALL be converted from tool definitions to regular Go functions in `internal/tools/spirit/orchestration_steps.go`.

#### Scenario: Fine-grained functions are not registered as tools
- **WHEN** a Spirit Agent's tool list is inspected
- **THEN** only 3 tools are visible: plan_and_execute, check_progress, cancel_orchestration

#### Scenario: Fine-grained functions are callable internally
- **WHEN** plan_and_execute needs to classify an industry
- **THEN** it calls `classifyIndustry()` function from `orchestration_steps.go` directly

### Requirement: Spirit tools emit unified events
The Spirit tools SHALL emit events using the centralized EnvelopeType constants from `internal/event/envelope.go` (EnvelopeTypeButlerOrchestrationStarted/Completed/Failed), not private event type definitions.

#### Scenario: plan_and_execute emits centralized event
- **WHEN** plan_and_execute starts an orchestration
- **THEN** it emits an event with `EnvelopeTypeButlerOrchestrationStarted` from `internal/event`
