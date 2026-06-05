## ADDED Requirements

### Requirement: Memory butler agent definition
The system SHALL register a system builtin agent with `AgentKey="__memory__"`, `Kind="system_builtin"`, `Model="gpt-4.1"`, `ToolsProfile="system_memory"`, and system prompt at `internal/scenario/system/prompts/memory/memory.md`.

#### Scenario: Memory butler seed data
- **WHEN** the system seeds builtin agents
- **THEN** `SeedMemoryAgent()` SHALL insert the `__memory__` agent with `DisplayName="记忆管家"` and `Description="基于学术原则的智能记忆管理者：选择性记忆、质量驱动遗忘、记忆蒸馏"`
- **AND** `Ownership` field SHALL be `"system_builtin"` for agent list filtering

#### Scenario: Memory butler prompt file seeding
- **WHEN** the system seeds prompt files
- **THEN** `SeedButlerPromptFiles()` SHALL load `prompts/memory/memory.md` into `agent_prompt_files` table for the `__memory__` agent

### Requirement: Memory butler tool injection via Port+Adapter
The memory butler tools SHALL be injected using a Port+Adapter pattern. The `Deps` struct in `internal/tools/memory_butler/registry.go` SHALL define port interfaces, and `internal/service/cli_admin_tools.go` SHALL construct the adapter.

#### Scenario: Memory butler Deps structure
- **WHEN** `memory_butler.RegisterAll(deps)` is called
- **THEN** `deps` SHALL contain `Analytics *biz.ExperienceAnalyticsUsecase`, `MemoryAdmin *biz.MemoryAdminUsecase`, `Embedder biz.SkillEmbedder`, `ProviderCatalog *biz.LlmProviderModelUsecase`, `RoundTrip *provider.RoundTrip`, `ProviderCode string`, `ModelAPIID string`, `EventBus event.Bus`

#### Scenario: Tool injection in ChatOrchestrator
- **WHEN** `ChatOrchestrator` processes a turn for an agent with `AgentKey="__memory__"`
- **THEN** `memoryButlerTools()` in `cli_admin_tools.go` SHALL return all registered memory butler tools
- **AND** the tools SHALL be injected at the turn execution point in `chat_orchestrator_turn.go`

### Requirement: analyze_memory_quality tool
The system SHALL provide `analyze_memory_quality` tool that invokes `ExperienceAnalyticsUsecase.AnalyzeMemoryQuality` and returns a `MemoryQualityReport`.

#### Scenario: Analyze memory quality for specific agent
- **WHEN** the memory butler calls `analyze_memory_quality` with `agent_id="agent_x"`
- **THEN** the tool SHALL call `ExperienceAnalyticsUsecase.AnalyzeMemoryQuality(ctx, "agent_x")`
- **AND** return `hit_rate`, `miss_rate`, `redundancy_score`, `misaligned_count`, `inactive_count`, `predictable_count`, `health_score`

#### Scenario: Analyze global memory quality
- **WHEN** the memory butler calls `analyze_memory_quality` with `agent_id=""`
- **THEN** the tool SHALL call `ExperienceAnalyticsUsecase.AnalyzeMemoryQuality(ctx, "")` for global aggregation

### Requirement: selective_remember tool
The system SHALL provide `selective_remember` tool that implements prediction-error-based selective memory writing. P0 stage SHALL use semantic novelty rules (embedding similarity) instead of LLM prediction to save tokens.

#### Scenario: P0 semantic novelty check - redundant content
- **WHEN** `selective_remember` is called with `content` and `agent_id`
- **THEN** the tool SHALL compute the embedding of `content`
- **AND** search for the most similar existing memory via `MemoryRepo.FindSimilar(ctx, agentID, emb, 5)`
- **AND** if the top result has `cosine_similarity > 0.85`, return `remembered=false` with `reason="redundant with existing memory"`

#### Scenario: P0 semantic novelty check - novel content
- **WHEN** the most similar existing memory has `cosine_similarity <= 0.85`
- **THEN** the tool SHALL write the memory via `MemoryAdminUsecase.UpsertFactRow`
- **AND** return `remembered=true` with `reason="novel content worth remembering"`

#### Scenario: Embedding failure fallback
- **WHEN** embedding computation fails
- **THEN** the tool SHALL default to `remembered=true` (fail-open) and log the error

#### Scenario: P1 prediction error distillation (future)
- **WHEN** P1 stage is enabled
- **THEN** `selective_remember` SHALL first run the P0 semantic novelty check
- **AND** if P0 passes, call LLM to predict content given context, compute `prediction_error = 1 - cosine_similarity(prediction_embedding, actual_embedding)`
- **AND** only remember if `prediction_error > PredictionErrorThreshold`

### Requirement: forget_low_quality tool
The system SHALL provide `forget_low_quality` tool that detects and deletes misaligned experiences (high input similarity but low output similarity / high negative feedback rate). P0 stage SHALL use retrieval + negative feedback rate as a proxy for misaligned detection.

#### Scenario: Misaligned detection via negative feedback rate
- **WHEN** `forget_low_quality` is called with `agent_id`
- **THEN** the tool SHALL list all facts for the agent via `MemoryAdminUsecase.ListFactRows(ctx, "agent", agentID, "", "", "", 1000, 0)`
- **AND** for each fact, compute `retrieval_count` from `memory_search` tool invocation records
- **AND** compute `negative_feedback_count` from feedback records after retrieval
- **AND** identify facts where `retrieval_count >= 3` AND `negative_feedback_count / retrieval_count > 0.5` as misaligned

#### Scenario: Dry run mode
- **WHEN** `forget_low_quality` is called with `dry_run=true`
- **THEN** the tool SHALL return `deleted_count=0` and `deleted_ids` listing the IDs of misaligned facts WITHOUT actually deleting them

#### Scenario: Execute deletion
- **WHEN** `forget_low_quality` is called with `dry_run=false`
- **THEN** the tool SHALL delete misaligned facts via `MemoryAdminUsecase.DeleteFactRowsByIDs(ctx, factIDs)`
- **AND** return `deleted_count` and `deleted_ids`

### Requirement: forget_inactive tool
The system SHALL provide `forget_inactive` tool that attenuates or forgets memories not retrieved within a configurable threshold period.

#### Scenario: Inactive memory detection
- **WHEN** `forget_inactive` is called with `agent_id` and `inactive_threshold_days`
- **THEN** the tool SHALL identify all facts for the agent not retrieved (via `memory_search`/`memory_load` records) within `inactive_threshold_days`
- **AND** return the list of inactive fact IDs

#### Scenario: Dry run mode for inactive memories
- **WHEN** `forget_inactive` is called with `dry_run=true`
- **THEN** the tool SHALL return `forgotten_count=0` and `forgotten_ids` listing inactive fact IDs WITHOUT deleting

#### Scenario: Execute inactive memory deletion
- **WHEN** `forget_inactive` is called with `dry_run=false`
- **THEN** the tool SHALL delete inactive facts via `MemoryAdminUsecase.DeleteFactRowsByIDs`
- **AND** return `forgotten_count` and `forgotten_ids`

### Requirement: deduplicate_memories tool
The system SHALL provide `deduplicate_memories` tool that merges semantically highly similar memories. P0 stage SHALL use trigram similarity instead of embedding cosine similarity.

#### Scenario: P0 trigram-based deduplication
- **WHEN** `deduplicate_memories` is called with `agent_id` and `sim_threshold`
- **THEN** the tool SHALL compute trigram similarity between all fact pairs for the agent
- **AND** identify pairs with similarity above `sim_threshold`
- **AND** merge duplicate facts by keeping the more recent one and deleting the older one via `MemoryAdminUsecase.DeleteFactRowsByIDs`
- **AND** return `merged_count`

#### Scenario: P1 embedding-based deduplication (future)
- **WHEN** P1 stage is enabled
- **THEN** `deduplicate_memories` SHALL use embedding cosine similarity > `dedup_sim_threshold` (default 0.95) instead of trigram similarity

### Requirement: consolidate_episodes tool
The system SHALL provide `consolidate_episodes` tool that distills fragmented episodic memories into structured semantic knowledge. P0 stage SHALL use deduplication + concatenation instead of LLM distillation.

#### Scenario: P0 simple consolidation
- **WHEN** `consolidate_episodes` is called with `agent_id`
- **THEN** the tool SHALL list all `episode`-type facts for the agent via `MemoryAdminUsecase.ListFactRows`
- **AND** deduplicate and concatenate episode facts
- **AND** write the result as a new `semantic`-type fact via `MemoryAdminUsecase.UpsertFactRow`
- **AND** return `distilled_count`

#### Scenario: P1 LLM distillation (future)
- **WHEN** P1 stage is enabled
- **THEN** `consolidate_episodes` SHALL call LLM with the distillation prompt template to extract structured knowledge from episodes
- **AND** write each distilled fact as a separate `semantic`-type fact

### Requirement: dream_cycle composite operation
The system SHALL provide `dream_cycle` tool as a composite operation that orchestrates multiple memory maintenance tools in sequence. The actual implementation SHALL execute 8 steps.

#### Scenario: dream_cycle 8-step execution flow
- **WHEN** `dream_cycle` is called with `agent_id` and `dry_run`
- **THEN** it SHALL execute the following steps in order:
  1. `analyze_memory_quality` - obtain current memory health report (record `quality_before`)
  2. Save pre-operation snapshot to `dream_snapshot_json` (for rollback)
  3. `forget_low_quality` - delete misaligned memories
  4. `forget_inactive` - attenuate/forget long-unretrieved memories
  5. `deduplicate_memories` - merge semantically duplicate memories
  6. `consolidate_episodes` - distill episodic memories to semantic knowledge
  7. `analyze_memory_quality` again (record `quality_after`)
  8. Generate `dream_report` with `quality_before`, `quality_after`, `actions_taken`, `deleted_count`, `merged_count`, `distilled_count`

#### Scenario: dream_cycle dry run mode
- **WHEN** `dream_cycle` is called with `dry_run=true`
- **THEN** steps 3-6 SHALL be executed in dry_run mode (no actual deletion/merge)
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
- **THEN** it SHALL contain: `Policy string` ("fifo"|"lru"|"priority_decay"|"reflection_summary"|"random_drop"|"hybrid"), `MaxMemoryCount int`, `MaxMemoryAgeDays int`, `InactiveThresholdDays int`, `MisalignedInputSimThreshold float64`, `MisalignedOutputSimThreshold float64`, `PredictionErrorThreshold float64`, `DedupSimThreshold float64`

#### Scenario: Default policy is hybrid
- **WHEN** `ForgetConfig.Policy` is not explicitly set
- **THEN** it SHALL default to `"hybrid"` (combining priority_decay + lru + reflection_summary)

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

#### Scenario: Deletion checks observation references
- **WHEN** a fact is about to be deleted
- **THEN** the system SHALL check if any observations reference this fact
- **AND** if references exist, the deletion SHALL be blocked with an error indicating dependent observations

### Requirement: dream_cycle cron task
The system SHALL register a cron task for `dream_cycle` that triggers the memory butler agent daily at 3:00 AM via `CronChatRunner`.

#### Scenario: Cron task seed data
- **WHEN** the system seeds cron tasks via `SeedCronTasks()`
- **THEN** a `CronTask` with `TaskKey="cron_dream_cycle"`, `Schedule="0 3 * * *"`, `AgentID` pointing to the `__memory__` agent SHALL be created
- **AND** `ConfigJSON` SHALL be `{"schedule":"0 3 * * *","message":"请执行 dream_cycle，整理记忆系统","type":"agent"}`

#### Scenario: Cron execution via CronChatRunner
- **WHEN** the cron runner dispatches the `dream_cycle` task
- **THEN** it SHALL create an Agent Session for the `__memory__` agent
- **AND** call `Chat.RunCronTurn(sessionID, "请执行 dream_cycle，整理记忆系统", "")`
- **AND** the memory butler agent SHALL respond by invoking the `dream_cycle` tool

### Requirement: Dream snapshot for rollback
The system SHALL save a pre-operation snapshot before dream_cycle executes any destructive operations. The snapshot SHALL be stored in `agent_runtime_settings.dream_snapshot_json` and retained for 7 days.

#### Scenario: Snapshot saved before deletion
- **WHEN** dream_cycle step 2 executes
- **THEN** all facts targeted for deletion/merge SHALL be serialized into a `DreamSnapshot` struct containing `ExecutedAt string`, `DeletedFacts []FactSnapshot`, `MergedFacts []FactSnapshot`
- **AND** each `FactSnapshot` SHALL contain `ID`, `Statement`, `ScopeType`, `ScopeID`, `Kind`

#### Scenario: Rollback within 7-day window
- **WHEN** a user requests "撤销上次整理" within 7 days of a dream_cycle execution
- **THEN** the memory butler SHALL restore deleted facts from the snapshot via `MemoryAdminUsecase.UpsertFactRow`

#### Scenario: Snapshot expired after 7 days
- **WHEN** a dream_snapshot is older than 7 days
- **THEN** the snapshot SHALL be considered expired and no rollback SHALL be possible

### Requirement: HealthScore auto-trigger dream_cycle
The system SHALL automatically trigger dream_cycle when `MemoryQualityReport.HealthScore` drops below 0.6.

#### Scenario: Suboptimal health triggers suggestion
- **WHEN** `HealthScore` is between 0.6 and 0.8
- **THEN** the memory butler SHALL suggest "建议整理记忆" to the user without auto-triggering

#### Scenario: Unhealthy health triggers dry_run dream_cycle
- **WHEN** `HealthScore` is between 0.4 and 0.6
- **THEN** the memory butler SHALL auto-trigger `dream_cycle` with `dry_run=true`

#### Scenario: Critical health triggers full dream_cycle with alert
- **WHEN** `HealthScore` is below 0.4
- **THEN** the memory butler SHALL auto-trigger `dream_cycle` with `dry_run=false`
- **AND** publish an alert via `EventBus` with `EnvelopeTypeAlertNotify` and `alert_type="memory_critical"`

### Requirement: Memory butler tool input/output structs
The system SHALL define typed input/output structs for each memory butler tool using `function.NewFunctionTool[I, O]` pattern.

#### Scenario: AnalyzeMemoryQuality IO
- **WHEN** `analyze_memory_quality` tool is invoked
- **THEN** input SHALL be `AnalyzeMemoryQualityInput{AgentID string}` and output SHALL be `AnalyzeMemoryQualityOutput{HitRate, MissRate, RedundancyScore, MisalignedCount, InactiveCount, PredictableCount, HealthScore float64/int}`

#### Scenario: SelectiveRemember IO
- **WHEN** `selective_remember` tool is invoked
- **THEN** input SHALL be `SelectiveRememberInput{Content string, Context string, AgentID string}` and output SHALL be `SelectiveRememberOutput{Remembered bool, Reason string}`

#### Scenario: ForgetLowQuality IO
- **WHEN** `forget_low_quality` tool is invoked
- **THEN** input SHALL be `ForgetLowQualityInput{AgentID string, DryRun bool}` and output SHALL be `ForgetLowQualityOutput{DeletedCount int, DeletedIDs []string}`

#### Scenario: ForgetInactive IO
- **WHEN** `forget_inactive` tool is invoked
- **THEN** input SHALL be `ForgetInactiveInput{AgentID string, InactiveThresholdDays int, DryRun bool}` and output SHALL be `ForgetInactiveOutput{ForgottenCount int, ForgottenIDs []string}`

#### Scenario: DeduplicateMemories IO
- **WHEN** `deduplicate_memories` tool is invoked
- **THEN** input SHALL be `DeduplicateMemoriesInput{AgentID string, SimThreshold float64}` and output SHALL be `DeduplicateMemoriesOutput{MergedCount int}`

#### Scenario: ConsolidateEpisodes IO
- **WHEN** `consolidate_episodes` tool is invoked
- **THEN** input SHALL be `ConsolidateEpisodesInput{AgentID string}` and output SHALL be `ConsolidateEpisodesOutput{DistilledCount int}`

#### Scenario: DreamCycle IO
- **WHEN** `dream_cycle` tool is invoked
- **THEN** input SHALL be `DreamCycleInput{AgentID string, DryRun bool}` and output SHALL be `DreamCycleOutput{QualityBefore float64, QualityAfter float64, ActionsTaken []string, DeletedCount int, MergedCount int, DistilledCount int}`
