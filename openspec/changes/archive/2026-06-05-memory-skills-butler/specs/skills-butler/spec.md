## ADDED Requirements

### Requirement: Skills butler agent definition
The system SHALL register a system builtin agent with `AgentKey="__skills__"`, `Kind="system_builtin"`, `Model="gpt-4.1"`, `ToolsProfile="system_skills"`, and system prompt at `internal/scenario/system/prompts/skills/skills.md`.

#### Scenario: Skills butler seed data
- **WHEN** the system seeds builtin agents
- **THEN** `SeedSkillsAgent()` SHALL insert the `__skills__` agent with `DisplayName="技能管家"` and `Description="基于使用数据的技能进化/消亡决策、工具权重优化、编排分析"`
- **AND** `Ownership` field SHALL be `"system_builtin"` for agent list filtering

#### Scenario: Skills butler prompt file seeding
- **WHEN** the system seeds prompt files
- **THEN** `SeedButlerPromptFiles()` SHALL load `prompts/skills/skills.md` into `agent_prompt_files` table for the `__skills__` agent

### Requirement: Skills butler tool injection via Port+Adapter pattern
The skills butler tools SHALL be injected using a Port+Adapter pattern. The `Deps` struct in `internal/tools/skills_butler/registry.go` SHALL define port interfaces, and `internal/service/skills_butler_adapter.go` SHALL bridge `*biz.ExperienceAnalyticsUsecase` to `skills_butler.AnalyticsPort`.

#### Scenario: Skills butler Deps structure
- **WHEN** `skills_butler.RegisterAll(deps)` is called
- **THEN** `deps` SHALL contain `Skills SkillUsecasePort`, `Evolution EvolutionUsecasePort`, `Queries SkillQueryReaderPort`, `Analytics AnalyticsPort`

#### Scenario: Conditional AnalyticsPort registration
- **WHEN** `Analytics` port is nil in `deps`
- **THEN** `RegisterAll` SHALL register only the non-analytics tools (Skills, Evolution, Queries ports)
- **AND** the 4 analytics-dependent tools (`analyze_skill_health`, `analyze_tool_weights`, `analyze_orchestration`, `optimize_orchestration`) SHALL NOT be registered

#### Scenario: AnalyticsPort adapter bridges ExperienceAnalyticsUsecase
- **WHEN** `skills_butler_adapter.go` constructs the `AnalyticsPort`
- **THEN** it SHALL wrap `*biz.ExperienceAnalyticsUsecase` methods to satisfy the `AnalyticsPort` interface
- **AND** the adapter SHALL be constructed in `cli_admin_tools.go` when `settings.EvolutionSkillEvolve` is enabled

#### Scenario: Tool injection in ChatOrchestrator
- **WHEN** `ChatOrchestrator` processes a turn for an agent with `AgentKey="__skills__"`
- **THEN** `skillsButlerTools()` in `cli_admin_tools.go` SHALL return all registered skills butler tools
- **AND** the tools SHALL be injected at the turn execution point in `chat_orchestrator_turn.go`

### Requirement: analyze_skill_health tool
The system SHALL provide `analyze_skill_health` tool that invokes `ExperienceAnalyticsUsecase.AnalyzeSkillHealth` and returns a list of `SkillHealth` reports.

#### Scenario: Analyze all skills
- **WHEN** `analyze_skill_health` is called with `skill_id=""`
- **THEN** the tool SHALL call `ExperienceAnalyticsUsecase.AnalyzeSkillHealth(ctx)`
- **AND** return `skills` array with `SkillHealth` for each skill containing `skill_id`, `invoke_count_7d`, `success_rate`, `avg_duration_ms`, `trend`, `health_status`, `recommendation`

#### Scenario: Analyze specific skill
- **WHEN** `analyze_skill_health` is called with a specific `skill_id`
- **THEN** the tool SHALL filter results to only return the health report for that skill

### Requirement: evolve_skill tool
The system SHALL provide `evolve_skill` tool that analyzes failure patterns and generates an optimized Skill version. The new version SHALL be created as disabled (pending review) and MUST be confirmed by the user before activation.

#### Scenario: Evolve skill failure analysis
- **WHEN** `evolve_skill` is called with `skill_id` and `failure_patterns`
- **THEN** the tool SHALL load the current Skill via `SkillUsecasePort`
- **AND** extract failure cases from `SkillQueryReaderPort.SearchSkillInvocations` with status="failure"
- **AND** call LLM via `provider.TRPCModelForProviderModel` with the failure analysis prompt template

#### Scenario: LLM call for skill optimization
- **WHEN** the evolve_skill tool calls LLM
- **THEN** it SHALL use `ProviderCatalog *biz.LlmProviderModelUsecase` and `RoundTrip *provider.RoundTrip` from the adapter
- **AND** the prompt SHALL include: Skill name, current body, description, and failure cases
- **AND** the LLM SHALL return JSON with `failure_analysis`, `optimized_body`, `changes`, `confidence`

#### Scenario: New version created as pending review
- **WHEN** the LLM returns an optimized body
- **THEN** the tool SHALL create a new Skill via `SkillUsecasePort.Create` with `SkillKey = existing_key + "_v2"` and `Name = existing_name + " (优化版)"`
- **AND** call `SkillUsecasePort.ToggleEnabled(newID, false)` to disable the new version
- **AND** return `new_version`, `diff_preview`, `status="pending_review"`

#### Scenario: Evolve skill requires user confirmation
- **WHEN** a new Skill version is created
- **THEN** it SHALL NOT be automatically enabled
- **AND** the user MUST explicitly confirm activation

### Requirement: retire_skill tool
The system SHALL provide `retire_skill` tool that marks a long-term underperforming Skill as disabled. The retirement functionality is covered by `optimize_skill` in the actual implementation, which handles critical/dormant health status by generating retirement suggestions.

#### Scenario: Retire skill health verification
- **WHEN** `retire_skill` is called with `skill_id` and `reason`
- **THEN** the tool SHALL first verify the Skill's health status via `analyze_skill_health`
- **AND** only proceed if `health_status` is "dormant" or "critical"

#### Scenario: Dependency check before retirement
- **WHEN** a Skill is confirmed for retirement
- **THEN** the tool SHALL check if any Agent's `tools_allow` references this Skill
- **AND** check if any orchestration template depends on this Skill
- **AND** return `dependent_agents` listing all affected agents

#### Scenario: Execute skill retirement
- **WHEN** dependency check passes
- **THEN** the tool SHALL call `SkillUsecasePort.ToggleEnabled(skillID, false)`
- **AND** append `[已退役] {reason}` to the Skill's description
- **AND** publish an `EnvelopeTypeAlertNotify` event with `alert_type="skill_retired"`, `skill_id`, `reason`, `severity="warning"`, and `message="Skill {name} 已退役：{reason}"`

#### Scenario: Notification via EnvelopeTypeAlertNotify
- **WHEN** a Skill is retired
- **THEN** the tool SHALL publish an event via `EventBus.Publish(ctx, event.Envelope{Type: event.EnvelopeTypeAlertNotify, Payload: ...})`
- **AND** the frontend `conversationEventDispatcher.ts` SHALL display a toast notification "Skill {name} 已退役：{reason}"

### Requirement: recommend_skills tool
The system SHALL provide `recommend_skills` tool that recommends Skill combinations based on task description using embedding similarity.

#### Scenario: Skill recommendation by task description
- **WHEN** `recommend_skills` is called with `task_description` and `top_k=5`
- **THEN** the tool SHALL call `SkillUsecasePort.ScoreByEmbedding(ctx, taskDescription, candidates)`
- **AND** return `recommendations` array with `skill_id`, `name`, `score` for the top K matches

#### Scenario: Default top_k value
- **WHEN** `recommend_skills` is called without `top_k`
- **THEN** `top_k` SHALL default to 5

### Requirement: analyze_tool_weights tool
The system SHALL provide `analyze_tool_weights` tool that invokes `ExperienceAnalyticsUsecase.AnalyzeToolWeights` and returns per-tool weight reports.

#### Scenario: Analyze tool weights for specific agent
- **WHEN** `analyze_tool_weights` is called with `agent_id`
- **THEN** the tool SHALL call `ExperienceAnalyticsUsecase.AnalyzeToolWeights(ctx)`
- **AND** return `tools` array with `ToolWeightReport` for each tool containing `tool_key`, `call_count`, `success_rate`, `avg_duration_ms`, `weight_score`, `recommendation`

#### Scenario: Analyze global tool weights
- **WHEN** `analyze_tool_weights` is called with `agent_id=""`
- **THEN** the tool SHALL return aggregated tool weights across all agents

### Requirement: analyze_orchestration tool
The system SHALL provide `analyze_orchestration` tool that invokes `ExperienceAnalyticsUsecase.AnalyzeOrchestration` and returns per-mode orchestration quality reports.

#### Scenario: Analyze orchestration with time range
- **WHEN** `analyze_orchestration` is called with `time_range="30d"` and `mode_filter=""`
- **THEN** the tool SHALL call `ExperienceAnalyticsUsecase.AnalyzeOrchestration(ctx, "30d", "")`
- **AND** return `modes` array with `OrchestrationModeReport` for each mode containing `mode`, `success_rate`, `avg_tokens`, `avg_duration_sec`, `member_contributions`, `dq_score`

#### Scenario: Filter by orchestration mode
- **WHEN** `analyze_orchestration` is called with `mode_filter="coordinator"`
- **THEN** the tool SHALL return only the report for the "coordinator" mode

#### Scenario: DQ Score interpretation in report
- **WHEN** a mode's `dq_score` is returned
- **THEN** the report SHALL include an interpretation: "excellent" if >= 0.7, "acceptable" if 0.5-0.7, "poor" if 0.3-0.5, "failing" if < 0.3

### Requirement: optimize_orchestration tool
The system SHALL provide `optimize_orchestration` tool that generates orchestration optimization suggestions based on analysis results. Suggestions SHALL NOT be automatically executed; user confirmation is required.

#### Scenario: Generate optimization suggestions
- **WHEN** `optimize_orchestration` is called with `time_range`
- **THEN** the tool SHALL first call `analyze_orchestration` internally
- **AND** based on DQ Score thresholds, generate `OrchestrationSuggestion` entries with `type` (e.g., "mode_switch", "agent_replace", "topology_cache"), `description`, `confidence`

#### Scenario: Suggestions require user confirmation
- **WHEN** optimization suggestions are generated
- **THEN** they SHALL NOT be automatically applied
- **AND** the user MUST explicitly confirm each suggestion before execution

#### Scenario: Topology caching for excellent modes
- **WHEN** a mode has `DQScore >= 0.7`
- **THEN** `optimize_orchestration` SHALL suggest caching the current orchestration topology for reuse

#### Scenario: Topology avoidance for failing modes
- **WHEN** a mode has `DQScore < 0.3`
- **THEN** `optimize_orchestration` SHALL suggest marking the topology as "avoid" and recommend alternative modes

### Requirement: SkillHealth model
The system SHALL define `SkillHealth` struct with fields: `SkillID string`, `InvokeCount7d int`, `SuccessRate float64`, `AvgDurationMS float64`, `Trend string` ("rising"|"stable"|"declining"|"dormant"), `HealthStatus string` ("healthy"|"warning"|"critical"|"dormant"), `Recommendation string` ("keep"|"evolve"|"retire"|"merge").

#### Scenario: SkillHealth status determination rules
- **WHEN** a Skill's metrics are evaluated
- **THEN** `HealthStatus` SHALL be:
  - "healthy" if `InvokeCount7d > 10` AND `SuccessRate > 0.8`
  - "warning" if `InvokeCount7d > 5` AND `0.6 <= SuccessRate <= 0.8`
  - "critical" if `InvokeCount7d < 2` OR `SuccessRate < 0.6`
  - "dormant" if 30 days with zero invocations

#### Scenario: SkillHealth recommendation mapping
- **WHEN** `HealthStatus` is determined
- **THEN** `Recommendation` SHALL be "keep" for healthy, "evolve" for warning, "evolve" for critical with `InvokeCount7d > 0`, "retire" for critical with `InvokeCount7d == 0` or dormant

### Requirement: ToolWeightReport model
The system SHALL define `ToolWeightReport` struct with fields: `ToolKey string`, `CallCount int`, `SuccessRate float64`, `AvgDurationMS float64`, `WeightScore float64`, `Recommendation string` ("promote"|"demote"|"keep"|"disable").

#### Scenario: ToolWeightReport weight calculation
- **WHEN** a `ToolWeightReport` is computed
- **THEN** `WeightScore` SHALL equal `normalize(success_rate) * 0.5 + normalize(call_count) * 0.3 + normalize(1/duration) * 0.2`

#### Scenario: ToolWeightReport recommendation rules
- **WHEN** `WeightScore` is computed
- **THEN** `Recommendation` SHALL be "promote" if >= 0.7, "keep" if 0.3-0.7, "demote" if 0.1-0.3, "disable" if < 0.1

#### Scenario: Tool weight stored in agent_runtime_settings
- **WHEN** `analyze_tool_weights` results are persisted
- **THEN** they SHALL be stored in `agent_runtime_settings.tool_weight_json` as a JSON map of `tool_key -> weight_score`
- **AND** `ChatOrchestrator` SHALL read this field when building `TRPCBuilderDeps` to filter disabled tools and inject prompt priority hints
- **AND** the system SHALL NOT modify `tools.Assemble()` for weight-based sorting

### Requirement: OrchestrationQuality model
The system SHALL define `OrchestrationQuality` struct with fields: `Validity float64`, `Specificity float64`, `Correctness float64`, `Efficiency float64`, `DQScore float64`.

#### Scenario: DQ Score formula
- **WHEN** an `OrchestrationQuality` is computed
- **THEN** `DQScore` SHALL equal `0.4 * Validity + 0.3 * Specificity + 0.3 * Correctness`
- **WHERE** `Validity = success_rate`, `Specificity = min(avg_output_length / 500, 1.0)`, `Correctness = 1 - negative_feedback_rate`

#### Scenario: DQ Score tiered thresholds
- **WHEN** `DQScore >= 0.7`
- **THEN** the orchestration SHALL be rated "excellent" and its topology cached
- **WHEN** `0.5 <= DQScore < 0.7`
- **THEN** the orchestration SHALL be rated "acceptable" with no action
- **WHEN** `0.3 <= DQScore < 0.5`
- **THEN** the orchestration SHALL be rated "poor" and the skills butler SHALL generate optimization suggestions
- **WHEN** `DQScore < 0.3`
- **THEN** the orchestration SHALL be rated "failing" and its topology SHALL be marked as "avoid"

### Requirement: skill_health_scan cron task
The system SHALL register a cron task for skill health scanning that triggers the skills butler agent weekly on Monday at 4:00 AM via `CronChatRunner`.

#### Scenario: Cron task seed data
- **WHEN** the system seeds cron tasks via `SeedCronTasks()`
- **THEN** a `CronTask` with `TaskKey="cron_skill_health_scan"`, `Schedule="0 4 * * 1"`, `AgentID` pointing to the `__skills__` agent SHALL be created
- **AND** `ConfigJSON` SHALL be `{"schedule":"0 4 * * 1","message":"请分析所有 Skill 的健康度","type":"agent"}`

#### Scenario: Cron execution via CronChatRunner
- **WHEN** the cron runner dispatches the `skill_health_scan` task
- **THEN** it SHALL create an Agent Session for the `__skills__` agent
- **AND** call `Chat.RunCronTurn(sessionID, "请分析所有 Skill 的健康度", "")`
- **AND** the skills butler agent SHALL respond by invoking the `analyze_skill_health` tool

### Requirement: Skills butler tool input/output structs
The system SHALL define typed input/output structs for each skills butler tool using `function.NewFunctionTool[I, O]` pattern.

#### Scenario: AnalyzeSkillHealth IO
- **WHEN** `analyze_skill_health` tool is invoked
- **THEN** input SHALL be `AnalyzeSkillHealthInput{SkillID string}` and output SHALL be `AnalyzeSkillHealthOutput{Skills []SkillHealth}`

#### Scenario: EvolveSkill IO
- **WHEN** `evolve_skill` tool is invoked
- **THEN** input SHALL be `EvolveSkillInput{SkillID string, FailurePatterns []string}` and output SHALL be `EvolveSkillOutput{NewVersion string, DiffPreview string, Status string}`

#### Scenario: RetireSkill IO
- **WHEN** `retire_skill` tool is invoked
- **THEN** input SHALL be `RetireSkillInput{SkillID string, Reason string}` and output SHALL be `RetireSkillOutput{Success bool, DependentAgents []string}`

#### Scenario: RecommendSkills IO
- **WHEN** `recommend_skills` tool is invoked
- **THEN** input SHALL be `RecommendSkillsInput{TaskDescription string, TopK int}` and output SHALL be `RecommendSkillsOutput{Recommendations []SkillRecommendation}` where `SkillRecommendation` has `SkillID string`, `Name string`, `Score float64`

#### Scenario: AnalyzeToolWeights IO
- **WHEN** `analyze_tool_weights` tool is invoked
- **THEN** input SHALL be `AnalyzeToolWeightsInput{AgentID string}` and output SHALL be `AnalyzeToolWeightsOutput{Tools []ToolWeightReport}`

#### Scenario: AnalyzeOrchestration IO
- **WHEN** `analyze_orchestration` tool is invoked
- **THEN** input SHALL be `AnalyzeOrchestrationInput{TimeRange string, ModeFilter string}` and output SHALL be `AnalyzeOrchestrationOutput{Modes []OrchestrationModeReport}` where `OrchestrationModeReport` has `Mode string`, `SuccessRate float64`, `AvgTokens int`, `AvgDurationSec int`, `MemberContributions map[string]float64`, `DQScore float64`

#### Scenario: OptimizeOrchestration IO
- **WHEN** `optimize_orchestration` tool is invoked
- **THEN** input SHALL be `OptimizeOrchestrationInput{TimeRange string}` and output SHALL be `OptimizeOrchestrationOutput{Suggestions []OrchestrationSuggestion}` where `OrchestrationSuggestion` has `Type string`, `Description string`, `Confidence float64`

### Requirement: Skills butler additional tools
The system SHALL provide `skills_butler_optimize_skill` and `skills_butler_analyze_skill_usage` tools in addition to the core 7 tools, as discovered in the actual implementation.

#### Scenario: optimize_skill tool
- **WHEN** `optimize_skill` tool is invoked for a Skill with critical health status
- **THEN** it SHALL generate optimization or retirement suggestions based on usage statistics
- **AND** this tool covers the functionality originally planned for the standalone `retire_skill` tool

#### Scenario: analyze_skill_usage tool
- **WHEN** `analyze_skill_usage` tool is invoked
- **THEN** it SHALL analyze Skill invocation frequency, success rate, and trends
- **AND** return usage statistics that complement `analyze_skill_health`

### Requirement: Skills butler Deps include LLM provider for evolve_skill
The skills butler adapter SHALL inject LLM provider dependencies needed by `evolve_skill` tool for calling LLM to analyze failure patterns and generate optimized Skill bodies.

#### Scenario: LLM provider in adapter
- **WHEN** `skillsButlerTools()` constructs the skills butler Deps
- **THEN** the adapter SHALL include `ProviderCatalog *biz.LlmProviderModelUsecase`, `RoundTrip *provider.RoundTrip`, `ProviderCode string`, `ModelAPIID string`
- **AND** `evolve_skill` SHALL call `provider.TRPCModelForProviderModel(ctx, catalog, roundTrip, providerCode, modelAPIID)` to obtain the LLM model

#### Scenario: LLM failure analysis prompt
- **WHEN** `evolve_skill` calls LLM for failure analysis
- **THEN** the prompt SHALL include Skill name, current body, description, and failure cases
- **AND** request JSON output with `failure_analysis`, `optimized_body`, `changes`, `confidence` fields
