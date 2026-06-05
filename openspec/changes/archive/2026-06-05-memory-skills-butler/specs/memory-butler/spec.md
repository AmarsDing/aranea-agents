## ADDED Requirements

### Requirement: Memory butler agent definition
The system SHALL register a system builtin agent with `AgentKey="__memory__"`, `Kind="system_builtin"`, `Model="gpt-4.1"`, `ToolsProfile="system_memory"`, and system prompt at `internal/scenario/system/prompts/memory/memory.md`.

#### Scenario: Memory butler seed data
- **WHEN** the system seeds builtin agents
- **THEN** `SeedMemoryAgent()` SHALL insert the `__memory__` agent with `DisplayName="记忆管家"` and `Description="基于学术原则的智能记忆管理者：选择性记忆、质量驱动遗忘、记忆蒸馏"`
- **AND** `Kind` field SHALL be `"system_builtin"` for agent classification

#### Scenario: Memory butler prompt file seeding
- **WHEN** the system seeds prompt files
- **THEN** `SeedButlerPromptFiles()` SHALL load `prompts/memory/memory.md` into `agent_prompt_files` table for the `__memory__` agent

### Requirement: Memory butler tool injection
The memory butler tools SHALL be injected when the agent's `AgentKey` is `"__memory__"`. The `Deps` struct in `internal/tools/memory_butler/registry.go` SHALL define the dependencies.

#### Scenario: Memory butler Deps structure
- **WHEN** `memory_butler.RegisterAll(deps)` is called
- **THEN** `deps` SHALL contain `Analytics *biz.ExperienceAnalyticsUsecase`, `MemoryAdmin *biz.MemoryAdminUsecase`, `Embedder skill.SkillEmbedder`, `EventBus contract.Bus`, `Agents biz.AgentRuntimeSettingsRepo`

#### Scenario: Tool injection in ChatOrchestrator
- **WHEN** `ChatOrchestrator` processes a turn for an agent with `AgentKey="__memory__"`
- **THEN** `memoryButlerTools()` in `cli_admin_tools.go` SHALL return all registered memory butler tools
- **AND** the tools SHALL be injected at the turn execution point in `chat_orchestrator_turn.go`

### Requirement: analyze_memory_quality tool
The system SHALL provide `memory_butler_analyze_quality` tool that invokes `ExperienceAnalyticsUsecase.AnalyzeMemoryQuality` and returns a quality report.

#### Scenario: Analyze memory quality for specific agent
- **WHEN** the memory butler calls `memory_butler_analyze_quality` with `agent_id="agent_x"`
- **THEN** the tool SHALL call `ExperienceAnalyticsUsecase.AnalyzeMemoryQuality(ctx, "agent_x", since)`
- **AND** return `hit_rate` (mapped from `RetrievalQuality`), `miss_rate` (computed as `1 - hit_rate`), `redundancy_score` (0 in P0), `misaligned_count` (mapped from `NegativeFeedback`), `inactive_count` (0 in P0), `predictable_count` (0 in P0), `health_score`

#### Scenario: P0 simplified metrics
- **WHEN** the tool returns results in P0 stage
- **THEN** `redundancy_score`, `inactive_count`, and `predictable_count` SHALL be 0
- **AND** these fields SHALL be populated in P1 when the underlying data becomes available

### Requirement: selective_remember tool
The system SHALL provide `memory_butler_selective_remember` tool that implements selective memory writing. P0 stage SHALL use substring matching instead of LLM prediction or embedding similarity to save tokens.

#### Scenario: P0 substring-based redundancy check - redundant content
- **WHEN** `memory_butler_selective_remember` is called with `content` and `agent_id`
- **THEN** the tool SHALL list existing facts via `MemoryAdminUsecase.ListFactRows`
- **AND** if the new content exactly matches or is a substring of an existing fact (or vice versa, for strings >20 chars), return `remembered=false` with `reason="redundant with existing memory"`

#### Scenario: P0 substring-based redundancy check - novel content
- **WHEN** the content is not found to be redundant
- **THEN** the tool SHALL write the memory via `MemoryAdminUsecase.UpsertFactRow` with `FactKind="semantic"` and `SourceKind="selective_remember"`
- **AND** return `remembered=true` with `reason="novel content worth remembering"`

#### Scenario: P1 embedding-based check (future)
- **WHEN** P1 stage is enabled
- **THEN** `selective_remember` SHALL use embedding cosine similarity (threshold 0.85) instead of substring matching
- **AND** further use LLM prediction error distillation for higher quality filtering

### Requirement: forget_low_quality tool
The system SHALL provide `memory_butler_forget_low_quality` tool that detects and deletes misaligned experiences. P0 stage SHALL use hit_count + negative_feedback_count from fact rows as a proxy for misaligned detection.

#### Scenario: Misaligned detection via negative feedback rate
- **WHEN** `memory_butler_forget_low_quality` is called with `agent_id`
- **THEN** the tool SHALL list all facts for the agent via `MemoryAdminUsecase.ListFactRows(ctx, "agent", agentID, "", "", "", 500, 0)`
- **AND** for each fact, read `hit_count` and `negative_feedback_count` from the JSON row
- **AND** identify facts where `hit_count >= 3` AND `negative_feedback_count > 0` AND `negative_feedback_count / hit_count > 0.5` as misaligned

#### Scenario: Dry run mode
- **WHEN** `memory_butler_forget_low_quality` is called with `dry_run=true`
- **THEN** the tool SHALL return `deleted_count` (number of candidates) and `deleted_ids` listing the IDs of misaligned facts WITHOUT actually deleting them

#### Scenario: Execute deletion
- **WHEN** `memory_butler_forget_low_quality` is called with `dry_run=false`
- **THEN** the tool SHALL delete misaligned facts via `MemoryAdminUsecase.DeleteFactRowsByIDs(ctx, factIDs)`
- **AND** return `deleted_count` and `deleted_ids`

### Requirement: forget_inactive tool
The system SHALL provide `memory_butler_forget_inactive` tool that forgets memories not updated within a configurable threshold period.

#### Scenario: Inactive memory detection
- **WHEN** `memory_butler_forget_inactive` is called with `agent_id` and `inactive_threshold_days`
- **THEN** the tool SHALL identify all facts for the agent with `updated_at` before the cutoff date
- **AND** default `inactive_threshold_days` to 30 if not provided or <= 0

#### Scenario: Dry run mode for inactive memories
- **WHEN** `memory_butler_forget_inactive` is called with `dry_run=true`
- **THEN** the tool SHALL return `forgotten_count` (number of candidates) and `forgotten_ids` listing inactive fact IDs WITHOUT deleting

#### Scenario: Execute inactive memory deletion
- **WHEN** `memory_butler_forget_inactive` is called with `dry_run=false`
- **THEN** the tool SHALL delete inactive facts via `MemoryAdminUsecase.DeleteFactRowsByIDs`
- **AND** return `forgotten_count` and `forgotten_ids`

### Requirement: deduplicate_memories tool
The system SHALL provide `memory_butler_deduplicate_memories` tool that merges highly similar memories. P0 stage SHALL use trigram similarity instead of embedding cosine similarity.

#### Scenario: P0 trigram-based deduplication
- **WHEN** `memory_butler_deduplicate_memories` is called with `agent_id` and `sim_threshold`
- **THEN** the tool SHALL compute trigram similarity between all fact pairs for the agent
- **AND** identify pairs with similarity above `sim_threshold` (default 0.8)
- **AND** keep the newer fact and delete the older one via `MemoryAdminUsecase.DeleteFactRowsByIDs`
- **AND** return `merged_count`

#### Scenario: P1 embedding-based deduplication (future)
- **WHEN** P1 stage is enabled
- **THEN** `deduplicate_memories` SHALL use embedding cosine similarity > `dedup_sim_threshold` (default 0.95) instead of trigram similarity

### Requirement: consolidate_episodes tool
The system SHALL provide `memory_butler_consolidate_episodes` tool that distills fragmented episodic memories into structured semantic knowledge. P0 stage SHALL use deduplication + concatenation instead of LLM distillation.

#### Scenario: P0 simple consolidation
- **WHEN** `memory_butler_consolidate_episodes` is called with `agent_id`
- **THEN** the tool SHALL list all `episode`-type facts for the agent via `MemoryAdminUsecase.ListFactRows(ctx, "agent", agentID, "episode", "", "", 500, 0)`
- **AND** deduplicate episode statements (case-insensitive)
- **AND** concatenate unique statements with `"[Distilled from episodes] "` prefix
- **AND** write the result as a new `semantic`-type fact via `MemoryAdminUsecase.UpsertFactRow` with `SourceKind="consolidate_episodes"`
- **AND** return `distilled_count`

#### Scenario: P1 LLM distillation (future)
- **WHEN** P1 stage is enabled
- **THEN** `consolidate_episodes` SHALL call LLM with the distillation prompt template to extract structured knowledge from episodes

### Requirement: dream_cycle composite operation
The system SHALL provide `memory_butler_dream_cycle` tool as a composite operation that orchestrates multiple memory maintenance tools in sequence. The actual implementation SHALL execute 8 steps.

#### Scenario: dream_cycle 8-step execution flow
- **WHEN** `memory_butler_dream_cycle` is called with `agent_id` and `dry_run`
- **THEN** it SHALL execute the following steps in order:
  1. `analyze_memory_quality` - obtain current memory health report (record `quality_before`)
  2. Save pre-operation snapshot to `DreamSnapshot` (for rollback)
  3. `forget_low_quality` - delete misaligned memories
  4. `forget_inactive` - attenuate/forget long-unretrieved memories
  5. `deduplicate_memories` - merge semantically duplicate memories
  6. `consolidate_episodes` - distill episodic memories to semantic knowledge
  7. Save dream snapshot to `agent_runtime_settings.dream_snapshot_json` via `Agents.UpsertAgentRuntimeSettings`
  8. `analyze_memory_quality` again (record `quality_after`)

#### Scenario: dream_cycle dry run mode
- **WHEN** `memory_butler_dream_cycle` is called with `dry_run=true`
- **THEN** only step 1 SHALL execute (quality measurement)
- **AND** the report SHALL show what WOULD be done without making changes

#### Scenario: dream_cycle returns quality delta
- **WHEN** dream_cycle completes
- **THEN** `DreamCycleOutput` SHALL contain `quality_before` and `quality_after` showing the health score change
- **AND** `actions_taken` SHALL list each step executed with its outcome

### Requirement: ForgetConfig storage and defaults
The system SHALL store `ForgetConfig` as JSON in `agent_runtime_settings.forget_policy_json` field. The Ent Schema SHALL include this field with default value `"{}"`.

#### Scenario: ForgetConfig default values in seed data
- **WHEN** the `__memory__` agent is seeded
- **THEN** `ForgetConfigJSON` SHALL be set to the default: `{"policy":"hybrid","max_memory_count":1000,"max_memory_age_days":90,"inactive_threshold_days":30,"misaligned_input_sim_threshold":0.8,"misaligned_output_sim_threshold":0.5,"prediction_error_threshold":0.3,"dedup_sim_threshold":0.95}`

#### Scenario: ForgetConfig fields
- **WHEN** `ForgetConfig` is deserialized from `forget_policy_json`
- **THEN** it SHALL contain: `Policy string` ("hybrid" for P0), `MaxMemoryCount int`, `MaxMemoryAgeDays int`, `InactiveThresholdDays int`, `MisalignedInputSimThreshold float64`, `MisalignedOutputSimThreshold float64`, `PredictionErrorThreshold float64`, `DedupSimThreshold float64`

### Requirement: L3FactWriter sub-interface for memory deletion
The system SHALL define `L3FactWriter` as a sub-interface of the memory admin store, providing `DeleteFactRow(ctx context.Context, factID string) error` and `DeleteFactRowsByIDs(ctx context.Context, factIDs []string) (int, error)`.

#### Scenario: L3FactWriter injected into MemoryAdminUsecase
- **WHEN** `NewMemoryAdminUsecase` is constructed
- **THEN** it SHALL receive a `factWriter L3FactWriter` parameter
- **AND** `MemoryAdminUsecase.DeleteFactRow` SHALL delegate to `factWriter.DeleteFactRow`
- **AND** `MemoryAdminUsecase.DeleteFactRowsByIDs` SHALL delegate to `factWriter.DeleteFactRowsByIDs`

#### Scenario: Deletion syncs vector index
- **WHEN** `L3FactWriter.DeleteFactRow` is called
- **THEN** the data layer implementation SHALL delete the `memory_facts` table row via Ent ORM
- **AND** call `MemoryFactIndexSyncer` to clean up the vector index entry

### Requirement: dream_cycle cron task
The system SHALL register a cron task for `dream_cycle` that triggers the memory butler agent daily at 3:00 AM.

#### Scenario: Cron task seed data
- **WHEN** the system seeds cron tasks via `SeedCronTasks()`
- **THEN** a cron task with `TaskKey="dream_cycle"`, `Schedule="0 3 * * *"`, `AgentID` pointing to the `__memory__` agent SHALL be created
- **AND** `ConfigJSON` SHALL be `{"schedule":"0 3 * * *","dry_run":true}`

### Requirement: Dream snapshot for rollback
The system SHALL save a pre-operation snapshot before dream_cycle executes any destructive operations. The snapshot SHALL be stored in `agent_runtime_settings.dream_snapshot_json`.

#### Scenario: Snapshot saved before deletion
- **WHEN** dream_cycle step 2 executes
- **THEN** all facts SHALL be serialized into a `DreamSnapshot` struct containing `ExecutedAt string`, `DeletedFacts []FactSnapshot`, `MergedFacts []FactSnapshot`
- **AND** each `FactSnapshot` SHALL contain `ID`, `Statement`, `ScopeType`, `ScopeID`, `Kind`

#### Scenario: Snapshot saved to agent runtime settings
- **WHEN** dream_cycle step 7 executes
- **THEN** the snapshot JSON SHALL be saved via `Agents.GetAgentRuntimeSettings` + `Agents.UpsertAgentRuntimeSettings` with `DreamSnapshotJSON` field

### Requirement: Memory butler tool input/output structs
The system SHALL define typed input/output structs for each memory butler tool using `function.NewFunctionTool[I, O]` pattern. All tool names SHALL use the `memory_butler_` prefix.

#### Scenario: AnalyzeMemoryQuality IO
- **WHEN** `memory_butler_analyze_quality` tool is invoked
- **THEN** input SHALL be `analyzeMemoryQualityInput{AgentID string}` and output SHALL be `analyzeMemoryQualityOutput{HitRate, MissRate, RedundancyScore, MisalignedCount, InactiveCount, PredictableCount, HealthScore float64/int}`

#### Scenario: SelectiveRemember IO
- **WHEN** `memory_butler_selective_remember` tool is invoked
- **THEN** input SHALL be `selectiveRememberInput{Content string, Context string, AgentID string}` and output SHALL be `selectiveRememberOutput{Remembered bool, Reason string}`

#### Scenario: ForgetLowQuality IO
- **WHEN** `memory_butler_forget_low_quality` tool is invoked
- **THEN** input SHALL be `forgetLowQualityInput{AgentID string, DryRun bool}` and output SHALL be `forgetLowQualityOutput{DeletedCount int, DeletedIDs []string}`

#### Scenario: ForgetInactive IO
- **WHEN** `memory_butler_forget_inactive` tool is invoked
- **THEN** input SHALL be `forgetInactiveInput{AgentID string, InactiveThresholdDays int, DryRun bool}` and output SHALL be `forgetInactiveOutput{ForgottenCount int, ForgottenIDs []string}`

#### Scenario: DeduplicateMemories IO
- **WHEN** `memory_butler_deduplicate_memories` tool is invoked
- **THEN** input SHALL be `deduplicateMemoriesInput{AgentID string, SimThreshold float64}` and output SHALL be `deduplicateMemoriesOutput{MergedCount int}`

#### Scenario: ConsolidateEpisodes IO
- **WHEN** `memory_butler_consolidate_episodes` tool is invoked
- **THEN** input SHALL be `consolidateEpisodesInput{AgentID string}` and output SHALL be `consolidateEpisodesOutput{DistilledCount int}`

#### Scenario: DreamCycle IO
- **WHEN** `memory_butler_dream_cycle` tool is invoked
- **THEN** input SHALL be `dreamCycleInput{AgentID string, DryRun bool}` and output SHALL be `dreamCycleOutput{QualityBefore float64, QualityAfter float64, ActionsTaken []string, DeletedCount int, MergedCount int, DistilledCount int}`
