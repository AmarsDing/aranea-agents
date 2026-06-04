## ADDED Requirements

### Requirement: Memory butler agent seed data
The system SHALL seed a `__memory__` agent with `Kind=system_builtin`, `ToolsProfile=system_memory`, `Model=gpt-4.1`, `DisplayName=记忆管家`, and `Description=基于学术原则的智能记忆管理者：选择性记忆、质量驱动遗忘、记忆蒸馏`.

#### Scenario: Memory butler agent exists after seed
- **WHEN** the system performs seed data initialization
- **THEN** an agent with `AgentKey="__memory__"` SHALL exist in the database

### Requirement: Memory butler system prompt
The system SHALL provide a system prompt file at `internal/scenario/system/prompts/memory.md` defining the memory butler's core principles (selective memory, quality-driven forgetting, error propagation prevention), workflow (analyze → dream_cycle if health < 0.6), and constraints (dry_run default, hybrid policy, 1000 fact limit).

#### Scenario: Memory butler uses system prompt
- **WHEN** the memory butler agent is loaded for a conversation
- **THEN** its system prompt SHALL include the core principles and workflow instructions

### Requirement: analyze_memory_quality tool
The system SHALL provide an `analyze_memory_quality` tool that calls `ExperienceAnalyticsUsecase.AnalyzeMemoryQuality` and returns a `MemoryQualityReport` with `hit_rate`, `miss_rate`, `redundancy_score`, `misaligned_count`, `inactive_count`, `predictable_count`, and `health_score`.

#### Scenario: Analyze memory quality for specific agent
- **WHEN** the tool is called with `agent_id` set to a valid agent ID
- **THEN** the tool SHALL return a quality report scoped to that agent's memories

#### Scenario: Analyze global memory quality
- **WHEN** the tool is called with `agent_id` empty
- **THEN** the tool SHALL return a quality report across all agents' memories

### Requirement: selective_remember tool (P0 semantic novelty rule)
The system SHALL provide a `selective_remember` tool that determines whether content is worth remembering using a semantic novelty rule: compute the content's embedding, search for the most similar existing memory, and skip if cosine similarity > 0.85 (redundant). If not redundant, write the memory via `MemoryAdminUsecase.UpsertFactRow`.

#### Scenario: Novel content is remembered
- **WHEN** the tool is called with content that has no similar existing memory (max cosine < 0.85)
- **THEN** the tool SHALL write the memory and return `remembered=true`

#### Scenario: Redundant content is skipped
- **WHEN** the tool is called with content that has an existing memory with cosine similarity > 0.85
- **THEN** the tool SHALL NOT write the memory and return `remembered=false` with reason "redundant"

#### Scenario: Embedding failure defaults to remembering
- **WHEN** the embedding computation fails
- **THEN** the tool SHALL write the memory anyway and return `remembered=true`

### Requirement: forget_low_quality tool
The system SHALL provide a `forget_low_quality` tool that detects misaligned memories (retrieved >= 3 times with > 50% negative feedback rate) and deletes them via `MemoryAdminUsecase.DeleteFactRowsByIDs`. When `dry_run=true`, the tool SHALL return the list of candidate fact IDs without deleting.

#### Scenario: Dry run returns candidates without deletion
- **WHEN** the tool is called with `dry_run=true`
- **THEN** the tool SHALL return `deleted_count=0` and `deleted_ids` listing the candidate fact IDs

#### Scenario: Execute deletion of misaligned memories
- **WHEN** the tool is called with `dry_run=false` and misaligned memories exist
- **THEN** the tool SHALL delete those memories and return the actual count and IDs

#### Scenario: No misaligned memories found
- **WHEN** no memories meet the misaligned criteria
- **THEN** the tool SHALL return `deleted_count=0` and empty `deleted_ids`

### Requirement: forget_inactive tool
The system SHALL provide a `forget_inactive` tool that identifies memories not retrieved within `inactive_threshold_days` (default 30) and deletes them via `MemoryAdminUsecase.DeleteFactRowsByIDs`. When `dry_run=true`, the tool SHALL return candidates without deleting.

#### Scenario: Inactive memories identified and deleted
- **WHEN** the tool is called with `dry_run=false` and inactive memories exist
- **THEN** the tool SHALL delete those memories and return the count and IDs

#### Scenario: Custom threshold days
- **WHEN** the tool is called with `inactive_threshold_days=60`
- **THEN** only memories not retrieved in 60+ days SHALL be considered inactive

### Requirement: deduplicate_memories tool
The system SHALL provide a `deduplicate_memories` tool that finds semantically duplicate memories (embedding cosine similarity > `sim_threshold`, default 0.95) and merges them by keeping the most recent fact and deleting the older ones via `MemoryAdminUsecase.DeleteFactRowsByIDs`.

#### Scenario: Duplicate memories merged
- **WHEN** two or more memories have cosine similarity > sim_threshold
- **THEN** the tool SHALL keep the most recent one, delete the others, and return `merged_count`

#### Scenario: No duplicates found
- **WHEN** no memories exceed the similarity threshold
- **THEN** the tool SHALL return `merged_count=0`

### Requirement: consolidate_episodes tool
The system SHALL provide a `consolidate_episodes` tool that distills fragmented episodic memories into structured semantic knowledge by: listing episodic facts for the agent, calling LLM with a distillation prompt, and writing the distilled facts via `MemoryAdminUsecase.UpsertFactRow`.

#### Scenario: Episodes distilled into semantic facts
- **WHEN** the tool is called for an agent with episodic memories
- **THEN** the tool SHALL produce distilled semantic facts and return `distilled_count`

#### Scenario: No episodic memories to consolidate
- **WHEN** the agent has no episodic memories
- **THEN** the tool SHALL return `distilled_count=0`

### Requirement: dream_cycle tool
The system SHALL provide a `dream_cycle` tool that orchestrates a composite memory maintenance operation: (1) analyze_memory_quality, (2) forget_low_quality, (3) forget_inactive, (4) deduplicate_memories, (5) consolidate_episodes. It SHALL return `quality_before`, `quality_after`, `actions_taken`, `deleted_count`, `merged_count`, `distilled_count`.

#### Scenario: Dream cycle improves memory health
- **WHEN** dream_cycle is executed with `dry_run=false`
- **THEN** `quality_after` SHALL be >= `quality_before`

#### Scenario: Dream cycle dry run
- **WHEN** dream_cycle is executed with `dry_run=true`
- **THEN** no memories SHALL be deleted or modified, and `actions_taken` SHALL list planned actions

### Requirement: Dream cycle snapshot and rollback
The system SHALL create a snapshot of facts to be deleted before dream_cycle execution, stored in `agent_runtime_settings.dream_snapshot_json`. The snapshot SHALL be retained for 7 days. Users SHALL be able to request "撤销上次整理" to restore deleted facts from the snapshot via `MemoryAdminUsecase.UpsertFactRow`.

#### Scenario: Snapshot created before deletion
- **WHEN** dream_cycle is about to delete facts
- **THEN** a snapshot of those facts SHALL be saved before deletion proceeds

#### Scenario: Rollback restores deleted facts
- **WHEN** a user requests rollback within 7 days
- **THEN** the facts from the snapshot SHALL be re-created via UpsertFactRow

### Requirement: Forget policy configuration
The system SHALL store forget policy configuration in `agent_runtime_settings.forget_policy_json` as a JSON field with default values: `policy=hybrid`, `max_memory_count=1000`, `max_memory_age_days=90`, `inactive_threshold_days=30`, `misaligned_input_sim_threshold=0.8`, `misaligned_output_sim_threshold=0.5`, `prediction_error_threshold=0.3`, `dedup_sim_threshold=0.95`.

#### Scenario: Default forget policy applied to memory butler
- **WHEN** the memory butler agent is seeded
- **THEN** its `agent_runtime_settings.forget_policy_json` SHALL contain the default configuration

### Requirement: Memory butler tools registration
The system SHALL register all memory butler tools via `memory_butler.RegisterAll(deps)` in `internal/service/system_builtin_tools.go`, injected only when `ag.AgentKey == "__memory__"`.

#### Scenario: Memory butler tools injected for memory agent
- **WHEN** the ChatOrchestrator assembles tools for an agent with `AgentKey="__memory__"`
- **THEN** all 7 memory butler tools SHALL be included

#### Scenario: No memory butler tools for other agents
- **WHEN** the ChatOrchestrator assembles tools for an agent with a different AgentKey
- **THEN** no memory butler tools SHALL be included

### Requirement: Dream cycle cron task
The system SHALL register a cron task with `TaskKey="dream_cycle"`, `Schedule="0 3 * * *"`, and `Message="请执行 dream_cycle，整理记忆系统"` targeting the `__memory__` agent.

#### Scenario: Dream cycle triggers at 3 AM daily
- **WHEN** the cron scheduler reaches 3:00 AM
- **THEN** a conversation SHALL be initiated with the memory butler agent to execute dream_cycle

### Requirement: Memory health threshold actions
The system SHALL define memory health threshold actions: HealthScore 0.8-1.0 = no action, 0.6-0.8 = suggest user to organize, 0.4-0.6 = auto-trigger dream_cycle (dry_run=true), < 0.4 = auto-trigger dream_cycle (dry_run=false) + alert notification.

#### Scenario: Critical health triggers automatic dream_cycle
- **WHEN** HealthScore < 0.4
- **THEN** dream_cycle SHALL be triggered with `dry_run=false` and an alert notification SHALL be sent
