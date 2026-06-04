# Memory Skills Butler

## Experience Analytics

### Requirement: ExperienceAnalyticsUsecase construction and Wire registration
The system SHALL provide `ExperienceAnalyticsUsecase` as a biz-layer Usecase composed of existing repository interfaces: `biz.EvolutionMetricsRepo`, `biz.SkillQueryReader`, `biz.TeamRepository`, `usage.AnalyticsRepo`, `*biz.MemoryAdminUsecase`, `biz.SessionReader`, and `biz.ToolInvocationReader`. It SHALL be registered via Wire in `internal/biz/biz.go` ProviderSet.

#### Scenario: Wire resolves all dependencies
- **WHEN** the application starts and Wire injects `ExperienceAnalyticsUsecase`
- **THEN** all 7 dependencies SHALL be resolved from existing Wire providers without error

#### Scenario: Constructor returns non-nil instance
- **WHEN** `NewExperienceAnalyticsUsecase` is called with valid dependencies
- **THEN** the returned struct SHALL have all fields populated and be non-nil

### Requirement: AnalyzeToolWeights analysis
The system SHALL analyze tool invocation data to produce per-tool weight reports. Each report SHALL include `tool_key`, `call_count`, `success_rate`, `avg_duration_ms`, `weight_score`, and `recommendation` (promote/demote/keep/disable). The weight_score SHALL be computed as `normalize(success_rate)*0.5 + normalize(call_count)*0.3 + normalize(1/duration)*0.2`.

#### Scenario: Tools with high success rate and high call count are promoted
- **WHEN** a tool has success_rate > 0.9 and call_count in top 20% of all tools
- **THEN** the recommendation SHALL be "promote" and weight_score SHALL be > 0.7

#### Scenario: Tools with low success rate are disabled
- **WHEN** a tool has success_rate < 0.3
- **THEN** the recommendation SHALL be "disable" and weight_score SHALL be < 0.3

#### Scenario: No tool invocation data available
- **WHEN** there are zero tool invocations in the queried time range
- **THEN** the method SHALL return an empty slice without error

### Requirement: AnalyzeSkillHealth analysis
The system SHALL analyze Skill invocation data to produce per-Skill health reports. Each report SHALL include `skill_id`, `invoke_count_7d`, `success_rate`, `avg_duration_ms`, `trend` (rising/stable/declining/dormant), `health_status` (healthy/warning/critical/dormant), and `recommendation` (keep/evolve/retire/merge).

#### Scenario: Healthy skill with high usage
- **WHEN** a Skill has invoke_count_7d > 10 and success_rate > 0.8
- **THEN** health_status SHALL be "healthy" and recommendation SHALL be "keep"

#### Scenario: Warning skill with moderate usage
- **WHEN** a Skill has invoke_count_7d > 5 and success_rate between 0.6 and 0.8
- **THEN** health_status SHALL be "warning" and recommendation SHALL be "evolve"

#### Scenario: Critical skill with low usage or success rate
- **WHEN** a Skill has invoke_count_7d < 2 or success_rate < 0.6
- **THEN** health_status SHALL be "critical" and recommendation SHALL be "evolve" or "retire"

#### Scenario: Dormant skill with no recent invocations
- **WHEN** a Skill has zero invocations in the last 30 days
- **THEN** health_status SHALL be "dormant" and recommendation SHALL be "retire"

### Requirement: AnalyzeOrchestration analysis
The system SHALL analyze team run data to produce per-mode orchestration reports. Each report SHALL include `mode`, `success_rate`, `avg_tokens`, `avg_duration_sec`, `member_contributions`, and `dq_score`. The DQ score SHALL be computed as `0.4*Validity + 0.3*Specificity + 0.3*Correctness` where Validity=success_rate, Specificity=min(avg_output_length/500, 1.0), Correctness=1-negative_feedback_rate.

#### Scenario: Orchestration analysis with sufficient data
- **WHEN** there are >= 10 team run records in the queried time range
- **THEN** the method SHALL return per-mode reports with DQ scores

#### Scenario: Insufficient orchestration data
- **WHEN** there are < 10 team run records in the queried time range
- **THEN** the method SHALL return an error indicating insufficient data

### Requirement: AnalyzeMemoryQuality analysis
The system SHALL analyze memory retrieval data to produce a memory quality report including `hit_rate`, `miss_rate`, `redundancy_score`, `misaligned_count`, `inactive_count`, `predictable_count`, and `health_score`. The health_score SHALL be computed as `0.3*hit_rate + 0.2*(1-redundancy_score) + 0.2*(1-misaligned_count/max(total_facts,1)) + 0.15*(1-inactive_count/max(total_facts,1)) + 0.15*(1-predictable_count/max(total_facts,1))`.

#### Scenario: Healthy memory system
- **WHEN** hit_rate > 0.7, redundancy_score < 0.1, and misaligned_count = 0
- **THEN** health_score SHALL be > 0.8

#### Scenario: Degraded memory system triggers alert
- **WHEN** health_score < 0.6
- **THEN** the report SHALL indicate that dream_cycle is recommended

### Requirement: AnalyzeAgentCapability analysis
The system SHALL analyze per-Agent capability profiles including tool success rates, skill scores, orchestration contributions, and cost efficiency.

#### Scenario: Agent capability profile generated
- **WHEN** an agent has tool invocations, skill invocations, and team run participation
- **THEN** the method SHALL return a comprehensive capability profile with all dimensions populated

### Requirement: ToolInvocationReader interface binding
The system SHALL bind the existing `biz.ToolInvocationReader` interface (from `internal/biz/tool/tool.go`) to its data layer implementation via Wire in `internal/data/data.go`. No new interface SHALL be created.

#### Scenario: Wire resolves ToolInvocationReader
- **WHEN** the application starts
- **THEN** `biz.ToolInvocationReader` SHALL be resolved to the existing data layer implementation

### Requirement: L3FactWriter interface for memory deletion
The system SHALL define a `L3FactWriter` sub-interface in `internal/biz/memory_admin_store.go` extending `L3FactAdminStore` with `DeleteFactRow(ctx, factID) error` and `DeleteFactRowsByIDs(ctx, factIDs) (int, error)` methods. `MemoryAdminUsecase` SHALL accept `L3FactWriter` as a constructor parameter and expose `DeleteFactRow` and `DeleteFactRowsByIDs` as proxy methods.

#### Scenario: Delete single fact
- **WHEN** `MemoryAdminUsecase.DeleteFactRow` is called with a valid fact ID
- **THEN** the fact SHALL be removed from the database and its vector index entry SHALL be cleaned up

#### Scenario: Delete multiple facts
- **WHEN** `MemoryAdminUsecase.DeleteFactRowsByIDs` is called with a list of fact IDs
- **THEN** all specified facts SHALL be removed and the returned count SHALL match the number of deleted facts

#### Scenario: Delete non-existent fact
- **WHEN** `DeleteFactRow` is called with a non-existent fact ID
- **THEN** the method SHALL return nil without error (idempotent)

## Memory Butler

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

## Skills Butler

### Requirement: Skills butler agent seed data
The system SHALL seed a `__skills__` agent with `Kind=system_builtin`, `ToolsProfile=system_skills`, `Model=gpt-4.1`, `DisplayName=技能管家`, and `Description=基于使用数据的技能进化/消亡决策、工具权重优化、编排分析`.

#### Scenario: Skills butler agent exists after seed
- **WHEN** the system performs seed data initialization
- **THEN** an agent with `AgentKey="__skills__"` SHALL exist in the database

### Requirement: Skills butler system prompt
The system SHALL provide a system prompt file at `internal/scenario/system/prompts/skills.md` defining the skills butler's core principles (data-driven, orchestration evolution, cost-aware), workflow (analyze health → analyze weights → analyze orchestration → evolve/retire/optimize), and constraints (evolve requires user confirmation, retire checks dependencies, orchestration suggestions require confirmation).

#### Scenario: Skills butler uses system prompt
- **WHEN** the skills butler agent is loaded for a conversation
- **THEN** its system prompt SHALL include the core principles and workflow instructions

### Requirement: analyze_skill_health tool
The system SHALL provide an `analyze_skill_health` tool that calls `ExperienceAnalyticsUsecase.AnalyzeSkillHealth` and returns a list of `SkillHealth` reports with `skill_id`, `invoke_count_7d`, `success_rate`, `avg_duration_ms`, `trend`, `health_status`, and `recommendation`.

#### Scenario: Analyze all skills health
- **WHEN** the tool is called with `skill_id` empty
- **THEN** the tool SHALL return health reports for all skills

#### Scenario: Analyze specific skill health
- **WHEN** the tool is called with a valid `skill_id`
- **THEN** the tool SHALL return a health report for that specific skill

### Requirement: evolve_skill tool
The system SHALL provide an `evolve_skill` tool that: (1) loads the current skill, (2) retrieves failure cases from skill invocations, (3) calls LLM to analyze failures and generate optimized body, (4) creates a new skill version via `SkillUsecase.Create` with `SkillKey + "_v2"` suffix, (5) disables the new version via `SkillUsecase.ToggleEnabled(id, false)` pending user review.

#### Scenario: Skill evolved with new version
- **WHEN** the tool is called with a valid `skill_id` and `failure_patterns`
- **THEN** a new disabled skill version SHALL be created and `status` SHALL be "pending_review"

#### Scenario: No failure cases found
- **WHEN** there are no failure invocations for the specified skill
- **THEN** the tool SHALL return an error indicating no data to evolve from

#### Scenario: LLM call fails
- **WHEN** the LLM call for failure analysis fails
- **THEN** the tool SHALL return an error without creating a new version

### Requirement: retire_skill tool
The system SHALL provide a `retire_skill` tool that: (1) verifies the skill health is dormant/critical via `analyze_skill_health`, (2) checks for dependent agents (agents whose `tools_allow` references this skill), (3) disables the skill via `SkillUsecase.ToggleEnabled(id, false)`, (4) publishes an `EnvelopeTypeAlertNotify` event with `alert_type="skill_retired"`, `severity="warning"`, skill_id, and reason.

#### Scenario: Skill retired with dependents notified
- **WHEN** a dormant skill is retired and dependent agents exist
- **THEN** the skill SHALL be disabled and `dependent_agents` SHALL list the affected agent IDs

#### Scenario: Skill retirement rejected for healthy skill
- **WHEN** the tool is called for a skill with health_status "healthy"
- **THEN** the tool SHALL return `success=false` with reason "skill is healthy, retirement not recommended"

#### Scenario: Skill retired without dependents
- **WHEN** a dormant skill is retired and no dependent agents exist
- **THEN** the skill SHALL be disabled and `dependent_agents` SHALL be empty

### Requirement: recommend_skills tool
The system SHALL provide a `recommend_skills` tool that calls `SkillUsecase.ScoreByEmbedding` to find skills matching a task description, returning top-K recommendations with `skill_id`, `name`, and `score`.

#### Scenario: Skills recommended for task
- **WHEN** the tool is called with a `task_description` and `top_k=5`
- **THEN** the tool SHALL return up to 5 skills ranked by embedding similarity score

#### Scenario: No matching skills
- **WHEN** no skills match the task description
- **THEN** the tool SHALL return an empty recommendations list

### Requirement: analyze_tool_weights tool
The system SHALL provide an `analyze_tool_weights` tool that calls `ExperienceAnalyticsUsecase.AnalyzeToolWeights` and returns a list of `ToolWeightReport` with `tool_key`, `call_count`, `success_rate`, `avg_duration_ms`, `weight_score`, and `recommendation`.

#### Scenario: Tool weights analyzed for agent
- **WHEN** the tool is called with a valid `agent_id`
- **THEN** the tool SHALL return weight reports scoped to that agent's tool usage

#### Scenario: Global tool weights analyzed
- **WHEN** the tool is called with empty `agent_id`
- **THEN** the tool SHALL return weight reports across all agents

### Requirement: analyze_orchestration tool
The system SHALL provide an `analyze_orchestration` tool that calls `ExperienceAnalyticsUsecase.AnalyzeOrchestration` and returns per-mode reports with `mode`, `success_rate`, `avg_tokens`, `avg_duration_sec`, `member_contributions`, and `dq_score`.

#### Scenario: Orchestration analysis with time range
- **WHEN** the tool is called with `time_range="30d"`
- **THEN** the tool SHALL return orchestration reports for the last 30 days

#### Scenario: Orchestration analysis filtered by mode
- **WHEN** the tool is called with `mode_filter="sequential"`
- **THEN** the tool SHALL return only the sequential mode report

### Requirement: optimize_orchestration tool
The system SHALL provide an `optimize_orchestration` tool that generates optimization suggestions based on orchestration analysis results. Suggestions SHALL NOT be auto-executed and SHALL require user confirmation. Each suggestion SHALL include `type`, `description`, and `confidence`.

#### Scenario: Optimization suggestions generated
- **WHEN** the tool is called and DQ scores indicate room for improvement
- **THEN** the tool SHALL return suggestions with confidence scores

#### Scenario: No optimization needed
- **WHEN** all orchestration modes have DQ scores > 0.7
- **THEN** the tool SHALL return an empty suggestions list

### Requirement: Skills butler tools registration
The system SHALL register all skills butler tools via `skills_butler.RegisterAll(deps)` in `internal/service/system_builtin_tools.go`, injected only when `ag.AgentKey == "__skills__"`.

#### Scenario: Skills butler tools injected for skills agent
- **WHEN** the ChatOrchestrator assembles tools for an agent with `AgentKey="__skills__"`
- **THEN** all 7 skills butler tools SHALL be included

#### Scenario: No skills butler tools for other agents
- **WHEN** the ChatOrchestrator assembles tools for an agent with a different AgentKey
- **THEN** no skills butler tools SHALL be included

### Requirement: Skill health scan cron task
The system SHALL register a cron task with `TaskKey="skill_health_scan"`, `Schedule="0 4 * * 1"` (every Monday 4 AM), and `Message="请分析所有 Skill 的健康度"` targeting the `__skills__` agent.

#### Scenario: Skill health scan triggers weekly
- **WHEN** the cron scheduler reaches Monday 4:00 AM
- **THEN** a conversation SHALL be initiated with the skills butler agent to analyze skill health

### Requirement: Tool weight storage in agent_runtime_settings
The system SHALL store tool weight analysis results in `agent_runtime_settings.tool_weight_json` as a JSON field mapping `tool_key` to `weight_score`. The `ChatOrchestrator` SHALL read this configuration when building `TRPCBuilderDeps` to filter disabled tools and inject prompt priority hints. The `tools.Assemble()` method SHALL NOT be modified.

#### Scenario: Tool weights stored after analysis
- **WHEN** `analyze_tool_weights` completes
- **THEN** the results SHALL be persisted in `agent_runtime_settings.tool_weight_json`

#### Scenario: Low-weight tools filtered in prompt
- **WHEN** a tool has weight_score < 0.3
- **THEN** the ChatOrchestrator SHALL mark it as `SkipSummarization` to reduce prompt usage

### Requirement: DQ score threshold actions
The system SHALL define DQ score threshold actions: DQ 0.7-1.0 = cache current orchestration topology, 0.5-0.7 = record but no action, 0.3-0.5 = generate optimization suggestions, < 0.3 = mark topology as "avoid".

#### Scenario: Excellent DQ score caches topology
- **WHEN** DQ score > 0.7
- **THEN** the orchestration topology SHALL be cached for reuse

#### Scenario: Poor DQ score marks topology as avoid
- **WHEN** DQ score < 0.3
- **THEN** the orchestration topology SHALL be marked as "avoid"

### Requirement: Skills butler LLM integration
The system SHALL use `provider.TRPCModelForProviderModel` (package-level function from `internal/provider/trpc_llm.go`) for LLM calls in `evolve_skill` and `consolidate_episodes` tools, using the agent's own Provider/Model configuration injected via `ProviderCatalog`, `RoundTrip`, `ProviderCode`, and `ModelAPIID` in the tool Deps.

#### Scenario: Evolve skill calls LLM with agent's model
- **WHEN** `evolve_skill` needs LLM analysis
- **THEN** it SHALL use `provider.TRPCModelForProviderModel` with the agent's configured provider and model
