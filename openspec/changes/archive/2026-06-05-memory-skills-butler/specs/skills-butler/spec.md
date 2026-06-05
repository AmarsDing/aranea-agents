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
The skills butler tools SHALL be injected using a Port+Adapter pattern. The `Deps` struct in `internal/tools/skills_butler/registry.go` SHALL define port interfaces, and `internal/service/skills_butler_adapter.go` SHALL bridge biz usecases to skills_butler port interfaces.

#### Scenario: Skills butler Deps structure
- **WHEN** `skills_butler.RegisterAll(deps)` is called
- **THEN** `deps` SHALL contain `Skills SkillUsecasePort`, `Evolution EvolutionUsecasePort`, `Queries SkillQueryReaderPort`, `Analytics AnalyticsPort`
- **AND** `SkillUsecasePort` SHALL define methods: `ListProposals`, `ApproveProposal`, `RejectProposal`, `RegisterApproved`, `CreateProposal`
- **AND** `EvolutionUsecasePort` SHALL define method: `GetEvolutionMetrics`
- **AND** `SkillQueryReaderPort` SHALL define method: `GetSkillInvocationStats`
- **AND** `AnalyticsPort` SHALL define methods: `AnalyzeToolWeights`, `AnalyzeSkillHealth`, `AnalyzeOrchestration`

#### Scenario: Conditional AnalyticsPort registration
- **WHEN** `Analytics` port is nil in `deps`
- **THEN** `RegisterAll` SHALL register only the non-analytics tools (`analyze_skill_usage`, `recommend_skills`, `evolve_skill`, `optimize_skill`)
- **AND** the 4 analytics-dependent tools (`skills_butler_analyze_skill_health`, `skills_butler_analyze_tool_weights`, `skills_butler_analyze_orchestration`, `skills_butler_optimize_orchestration`) SHALL NOT be registered

#### Scenario: Adapter architecture
- **WHEN** `skills_butler_adapter.go` constructs the port adapters
- **THEN** it SHALL provide 4 adapter structs:
  - `skillsButlerSkillUsecaseAdapter` wrapping `*biz.SkillEvolutionUsecase` → `SkillUsecasePort`
  - `skillsButlerEvolutionAdapter` wrapping `*biz.EvolutionUsecase` → `EvolutionUsecasePort`
  - `skillsButlerQueryAdapter` wrapping `biz.SkillInvocationStatsReader` → `SkillQueryReaderPort`
  - `skillsButlerAnalyticsAdapter` wrapping `*biz.ExperienceAnalyticsUsecase` + `agentID` → `AnalyticsPort`
- **AND** `skillsButlerAnalyticsAdapter` SHALL inject `agentID` into each `ExperienceAnalyticsUsecase` call (e.g., `AnalyzeSkillHealth(ctx, a.agentID, since)`)
- **AND** `skillsButlerAnalyticsAdapter.AnalyzeOrchestration` SHALL filter results by `modeFilter` and only pass `Mode`, `SuccessRate`, `DQScore` to `OrchestrationModeReport` (not `AvgTokens`, `AvgDurationSec`, `MemberContributions`)
- **AND** `skillsButlerAnalyticsAdapter.AnalyzeSkillHealth` SHALL compute `Trend` field in the adapter: "dormant" if `InvokeCount==0`, "declining" if `SuccessRate<0.5`, "rising" if `SuccessRate>=0.9`, "stable" otherwise

#### Scenario: Tool injection condition via EvolutionSkillEvolve
- **WHEN** `ChatOrchestrator.skillsButlerTools()` is called for an agent
- **THEN** it SHALL check `settings.EvolutionSkillEvolve` from `GetAgentRuntimeSettings(ctx, ag.ID)`
- **AND** only return tools if `EvolutionSkillEvolve` is enabled
- **AND** the tools SHALL be injected at the turn execution point in `chat_orchestrator_turn.go`

### Requirement: analyze_skill_usage tool
The system SHALL provide `skills_butler_analyze_skill_usage` tool that queries `SkillQueryReaderPort.GetSkillInvocationStats` and returns per-skill usage statistics with health assessment.

#### Scenario: Analyze skill usage for agent
- **WHEN** `skills_butler_analyze_skill_usage` is called with `agent_id` and optional `time_range` (default "30d")
- **THEN** the tool SHALL call `deps.Queries.GetSkillInvocationStats(ctx, agentID, since)`
- **AND** compute health for each skill using `assessHealth(stat, weeks)` function
- **AND** return `skills` array with `skillUsageItem` for each skill containing `skill_name`, `count`, `success_rate`, `avg_duration_ms`, `health`

#### Scenario: assessHealth function
- **WHEN** `assessHealth(stat, weeks)` is called
- **THEN** it SHALL compute `callsPerWeek = float64(count) / weeks`
- **AND** return "healthy" if `callsPerWeek > 5` AND `successRate >= 0.8`
- **AND** return "warning" if `callsPerWeek > 5` AND `successRate >= 0.6`
- **AND** return "critical" if `callsPerWeek < 2` OR `successRate < 0.6`
- **AND** return "warning" as default fallback

#### Scenario: timeRangeToSince helper
- **WHEN** `timeRangeToSince(tr)` is called
- **THEN** it SHALL return `now - 7d` for "7d", `now - 30d` for "30d", `now - 90d` for "90d", default `now - 30d`

#### Scenario: weeksInRange helper
- **WHEN** `weeksInRange(tr)` is called
- **THEN** it SHALL return 1.0 for "7d", 4.29 for "30d", 12.86 for "90d", default 4.29

### Requirement: evolve_skill tool
The system SHALL provide `skills_butler_evolve_skill` tool that creates a Skill evolution proposal (SkillProposal) for the specified agent. The proposal SHALL be created with "pending" status and MUST be approved before the Skill is registered.

#### Scenario: Create evolution proposal
- **WHEN** `skills_butler_evolve_skill` is called with `agent_id`, `skill_name`, and `improvement_description`
- **THEN** the tool SHALL construct `patternDesc = "{skill_name}: {improvement_description}"`
- **AND** compute `patternHash = sha256(patternDesc)[:8]` as hex string
- **AND** create a `biz.SkillProposal` with `AgentID`, `PatternHash`, `PatternDesc`, `SkillName`, `SkillMD=""`, `Status=SkillProposalStatusPending`, `CreatedAt=now.UTC()`
- **AND** call `deps.Skills.CreateProposal(ctx, proposal)` to persist the proposal
- **AND** return `proposal_id`, `skill_name`, `status`, `pattern_desc`, `created_at`

#### Scenario: Evolve skill requires all input fields
- **WHEN** `skills_butler_evolve_skill` is called with missing `agent_id`, `skill_name`, or `improvement_description`
- **THEN** the tool SHALL return a `BadRequest` error with code "SKILLS_BUTLER"

#### Scenario: Proposal requires approval before activation
- **WHEN** a new SkillProposal is created
- **THEN** it SHALL have `Status="pending"` and SHALL NOT be automatically enabled
- **AND** the user MUST explicitly approve the proposal via `ApproveProposal` and `RegisterApproved` before the Skill is registered

### Requirement: recommend_skills tool
The system SHALL provide `skills_butler_recommend_skills` tool that recommends Skill actions based on pending proposals and usage statistics (not embedding similarity).

#### Scenario: Recommend skills based on pending proposals
- **WHEN** `skills_butler_recommend_skills` is called with `agent_id` and optional `context_description`
- **THEN** the tool SHALL call `deps.Skills.ListProposals(ctx, agentID, "pending")`
- **AND** for each pending proposal, add a recommendation with `skill_name`, `reason="检测到重复工具调用模式：{pattern_desc}"`, `source="pending_proposal"`

#### Scenario: Recommend skills based on usage statistics
- **WHEN** usage statistics are available
- **THEN** the tool SHALL call `deps.Queries.GetSkillInvocationStats(ctx, agentID, since30d)`
- **AND** for each skill with `assessHealth == "warning"`, add recommendation with `reason="成功率偏低({success_rate}%)，建议优化或替换"`, `source="usage_warning"`
- **AND** for each skill with `assessHealth == "critical"`, add recommendation with `reason="使用率极低或成功率不足({success_rate}%)，建议移除或重构"`, `source="usage_critical"`

#### Scenario: Recommend skills output
- **WHEN** recommendations are compiled
- **THEN** the tool SHALL return `agent_id` and `recommendations` array with each item containing `skill_name`, `reason`, `source`

### Requirement: optimize_skill tool
The system SHALL provide `skills_butler_optimize_skill` tool that generates optimization suggestions for a specific Skill based on its usage statistics. This tool replaces the originally planned standalone `retire_skill` tool.

#### Scenario: Optimize skill with usage data
- **WHEN** `skills_butler_optimize_skill` is called with `agent_id` and `skill_name`
- **THEN** the tool SHALL call `deps.Queries.GetSkillInvocationStats(ctx, agentID, since30d)`
- **AND** find the matching skill by `skill_name`
- **AND** compute health using `assessHealth(stat, 4.29)`
- **AND** generate optimization suggestions based on:
  - **reliability** (high priority): if `successRate < 0.6`, suggest checking Skill logic
  - **reliability** (medium priority): if `0.6 <= successRate < 0.8`, suggest adding error handling
  - **performance** (medium priority): if `avgDurationMs > 5000`, suggest optimizing execution path
  - **performance** (low priority): if `2000 < avgDurationMs <= 5000`, suggest async optimization
  - **usage** (medium priority): if `callsPerWeek < 2`, suggest evaluating whether to keep
  - **general** (low priority): if no issues found, report good health

#### Scenario: Optimize skill with no usage data
- **WHEN** the target skill has no invocation records in the past 30 days
- **THEN** the tool SHALL return `health="unknown"` with a single "usage" suggestion (high priority): "该 Skill 在近 30 天内无调用记录，建议评估是否仍需要此 Skill"

#### Scenario: Optimize skill output
- **WHEN** optimization analysis is complete
- **THEN** the tool SHALL return `agent_id`, `skill_name`, `health`, and `suggestions` array with each item containing `category`, `description`, `priority`

### Requirement: analyze_skill_health tool
The system SHALL provide `skills_butler_analyze_skill_health` tool that invokes `AnalyticsPort.AnalyzeSkillHealth` and returns a list of skill health reports.

#### Scenario: Analyze all skills
- **WHEN** `skills_butler_analyze_skill_health` is called with `skill_id=""`
- **THEN** the tool SHALL call `deps.Analytics.AnalyzeSkillHealth(ctx)`
- **AND** return `skills` array with `skillHealthItem` for each skill containing `skill_id`, `invoke_count_7d`, `success_rate`, `avg_duration_ms`, `trend`, `health_status`, `recommendation`

#### Scenario: Analyze specific skill
- **WHEN** `skills_butler_analyze_skill_health` is called with a specific `skill_id`
- **THEN** the tool SHALL filter results to only return the health report for that skill

#### Scenario: HealthStatus values from ExperienceAnalyticsUsecase
- **WHEN** `ExperienceAnalyticsUsecase.AnalyzeSkillHealth` computes health status
- **THEN** `HealthStatus` SHALL be determined by `skillHealthStatus(successRate, invokeCount)`:
  - "unused" if `invokeCount == 0`, with recommendation "consider_removing"
  - "healthy" if `successRate >= 0.9`, with recommendation "keep"
  - "degraded" if `0.7 <= successRate < 0.9`, with recommendation "review_errors"
  - "unstable" if `0.5 <= successRate < 0.7`, with recommendation "investigate_failures"
  - "critical" if `successRate < 0.5`, with recommendation "disable_or_rewrite"

### Requirement: analyze_tool_weights tool
The system SHALL provide `skills_butler_analyze_tool_weights` tool that invokes `AnalyticsPort.AnalyzeToolWeights` and returns per-tool weight reports.

#### Scenario: Analyze tool weights
- **WHEN** `skills_butler_analyze_tool_weights` is called with `agent_id`
- **THEN** the tool SHALL call `deps.Analytics.AnalyzeToolWeights(ctx)`
- **AND** return `tools` array with `toolWeightItem` for each tool containing `tool_key`, `call_count`, `success_rate`, `avg_duration_ms`, `weight_score`, `recommendation`

### Requirement: analyze_orchestration tool
The system SHALL provide `skills_butler_analyze_orchestration` tool that invokes `AnalyticsPort.AnalyzeOrchestration` and returns per-mode orchestration quality reports.

#### Scenario: Analyze orchestration with time range
- **WHEN** `skills_butler_analyze_orchestration` is called with `time_range` (default "30d") and `mode_filter`
- **THEN** the tool SHALL call `deps.Analytics.AnalyzeOrchestration(ctx, timeRange, modeFilter)`
- **AND** return `modes` array with `orchestrationModeItem` for each mode containing `mode`, `success_rate`, `avg_tokens`, `avg_duration_sec`, `member_contributions`, `dq_score`

#### Scenario: Filter by orchestration mode
- **WHEN** `analyze_orchestration` is called with `mode_filter` set
- **THEN** the adapter SHALL filter results to only return the report for the matching mode

### Requirement: optimize_orchestration tool
The system SHALL provide `skills_butler_optimize_orchestration` tool that generates orchestration optimization suggestions based on DQ Score analysis. Suggestions SHALL NOT be automatically executed.

#### Scenario: Generate optimization suggestions
- **WHEN** `skills_butler_optimize_orchestration` is called with `time_range`
- **THEN** the tool SHALL first call `deps.Analytics.AnalyzeOrchestration(ctx, timeRange, "")` internally
- **AND** find the best mode (highest DQScore)
- **AND** generate suggestions based on DQ Score thresholds:
  - `avoid_topology` (confidence 0.9): if `DQScore < 0.3`, suggest marking topology as avoid
  - `switch_mode` (confidence 0.8): if `DQScore < 0.5`, suggest switching to best mode
  - `cache_topology` (confidence 0.85): if `DQScore > 0.7`, suggest caching the topology

#### Scenario: Suggestions require user confirmation
- **WHEN** optimization suggestions are generated
- **THEN** they SHALL NOT be automatically applied
- **AND** the user MUST explicitly confirm each suggestion before execution

### Requirement: SkillHealth model (legacy type in biz)
The system SHALL define `SkillHealth` struct in `internal/biz/experience_analytics_types.go` with fields: `SkillID string`, `InvokeCount7d int`, `SuccessRate float64`, `AvgDurationMS float64`, `Trend string` ("rising"|"stable"|"declining"|"dormant"), `HealthStatus string` ("healthy"|"warning"|"critical"|"dormant"), `Recommendation string` ("keep"|"evolve"|"retire"|"merge").

Note: The actual `HealthStatus` values computed by `ExperienceAnalyticsUsecase.skillHealthStatus()` are "unused"/"healthy"/"degraded"/"unstable"/"critical" with recommendations "consider_removing"/"keep"/"review_errors"/"investigate_failures"/"disable_or_rewrite". The legacy `SkillHealth` struct comments still reference the original design values.

#### Scenario: SkillHealth adapter Trend computation
- **WHEN** `skillsButlerAnalyticsAdapter.AnalyzeSkillHealth` maps `SkillHealthItem` to `SkillHealth`
- **THEN** `Trend` SHALL be computed in the adapter: "dormant" if `InvokeCount==0`, "declining" if `SuccessRate<0.5`, "rising" if `SuccessRate>=0.9`, "stable" otherwise

### Requirement: ToolWeightReport model
The system SHALL define `ToolWeightReport` struct in `internal/biz/experience_analytics_types.go` with fields: `ToolKey string`, `CallCount int`, `SuccessRate float64`, `AvgDurationMS float64`, `WeightScore float64`, `Recommendation string` ("promote"|"demote"|"keep"|"disable").

### Requirement: OrchestrationModeReport model
The system SHALL define `OrchestrationModeReport` struct in `internal/biz/experience_analytics_types.go` with fields: `Mode string`, `SuccessRate float64`, `AvgTokens int`, `AvgDurationSec int`, `MemberContributions map[string]float64`, `DQScore float64`.

Note: The actual `OrchestrationModeItem` in `ExperienceAnalyticsUsecase` only contains `Mode`, `RunCount`, `SuccessCount`, `SuccessRate`, `DQScore`, `Validity`, `Specificity`, `Correctness`. The adapter only maps `Mode`, `SuccessRate`, `DQScore` to `OrchestrationModeReport`.

### Requirement: skill_health_scan cron task
The system SHALL register a cron task for skill health scanning that triggers the skills butler agent weekly on Monday at 4:00 AM via `CronChatRunner`.

#### Scenario: Cron task seed data
- **WHEN** the system seeds cron tasks via `SeedCronTasks()`
- **THEN** a `CronTask` with `id="cron_skill_health_scan"`, `taskKey="skill_health_scan"`, `name="技能健康扫描"`, `description="每周一凌晨4点触发技能管家执行 Skill 健康度分析"`, `agentID="agent___skills__"` SHALL be created
- **AND** `ConfigJSON` SHALL be `{"schedule":"0 4 * * 1"}`

### Requirement: Skills butler tool input/output structs
The system SHALL define typed input/output structs for each skills butler tool using `function.NewFunctionTool[I, O]` pattern. All tool names SHALL use the `skills_butler_` prefix.

#### Scenario: AnalyzeSkillUsage IO
- **WHEN** `skills_butler_analyze_skill_usage` tool is invoked
- **THEN** input SHALL be `analyzeSkillUsageInput{AgentID string, TimeRange string}` and output SHALL be `analyzeSkillUsageOutput{AgentID string, TimeRange string, Skills []skillUsageItem}` where `skillUsageItem` has `SkillName string`, `Count int`, `SuccessRate float64`, `AvgDurationMs int64`, `Health string`

#### Scenario: EvolveSkill IO
- **WHEN** `skills_butler_evolve_skill` tool is invoked
- **THEN** input SHALL be `evolveSkillInput{AgentID string, SkillName string, ImprovementDescription string}` and output SHALL be `evolveSkillOutput{ProposalID string, SkillName string, Status string, PatternDesc string, CreatedAt string}`

#### Scenario: OptimizeSkill IO
- **WHEN** `skills_butler_optimize_skill` tool is invoked
- **THEN** input SHALL be `optimizeSkillInput{AgentID string, SkillName string}` and output SHALL be `optimizeSkillOutput{AgentID string, SkillName string, Health string, Suggestions []optimizationSuggestion}` where `optimizationSuggestion` has `Category string`, `Description string`, `Priority string`

#### Scenario: RecommendSkills IO
- **WHEN** `skills_butler_recommend_skills` tool is invoked
- **THEN** input SHALL be `recommendSkillsInput{AgentID string, ContextDescription string}` and output SHALL be `recommendSkillsOutput{AgentID string, Recommendations []skillRecommendation}` where `skillRecommendation` has `SkillName string`, `Reason string`, `Source string`

#### Scenario: AnalyzeSkillHealth IO
- **WHEN** `skills_butler_analyze_skill_health` tool is invoked
- **THEN** input SHALL be `analyzeSkillHealthInput{SkillID string}` and output SHALL be `analyzeSkillHealthOutput{Skills []skillHealthItem}` where `skillHealthItem` has `SkillID string`, `InvokeCount7d int`, `SuccessRate float64`, `AvgDurationMs float64`, `Trend string`, `HealthStatus string`, `Recommendation string`

#### Scenario: AnalyzeToolWeights IO
- **WHEN** `skills_butler_analyze_tool_weights` tool is invoked
- **THEN** input SHALL be `analyzeToolWeightsInput{AgentID string}` and output SHALL be `analyzeToolWeightsOutput{Tools []toolWeightItem}` where `toolWeightItem` has `ToolKey string`, `CallCount int`, `SuccessRate float64`, `AvgDurationMs float64`, `WeightScore float64`, `Recommendation string`

#### Scenario: AnalyzeOrchestration IO
- **WHEN** `skills_butler_analyze_orchestration` tool is invoked
- **THEN** input SHALL be `analyzeOrchestrationInput{TimeRange string, ModeFilter string}` and output SHALL be `analyzeOrchestrationOutput{Modes []orchestrationModeItem}` where `orchestrationModeItem` has `Mode string`, `SuccessRate float64`, `AvgTokens int`, `AvgDurationSec int`, `MemberContributions map[string]float64`, `DQScore float64`

#### Scenario: OptimizeOrchestration IO
- **WHEN** `skills_butler_optimize_orchestration` tool is invoked
- **THEN** input SHALL be `optimizeOrchestrationInput{TimeRange string}` and output SHALL be `optimizeOrchestrationOutput{Suggestions []orchestrationSuggestionItem}` where `orchestrationSuggestionItem` has `Type string`, `Description string`, `Confidence float64`

### Requirement: Skills butler tool registration order
The system SHALL register skills butler tools in a specific order, with non-analytics tools always registered and analytics-dependent tools conditionally registered.

#### Scenario: Non-analytics tools (always registered)
- **WHEN** `RegisterAll(deps)` is called
- **THEN** the following tools SHALL always be registered (in order):
  1. `skills_butler_analyze_skill_usage`
  2. `skills_butler_recommend_skills`
  3. `skills_butler_evolve_skill`
  4. `skills_butler_optimize_skill`

#### Scenario: Analytics-dependent tools (conditional)
- **WHEN** `deps.Analytics != nil`
- **THEN** the following tools SHALL additionally be registered (in order):
  5. `skills_butler_analyze_skill_health`
  6. `skills_butler_analyze_tool_weights`
  7. `skills_butler_analyze_orchestration`
  8. `skills_butler_optimize_orchestration`

### Requirement: SkillInvocationStat model
The system SHALL define `SkillInvocationStat` struct in `internal/tools/skills_butler/registry.go` with fields: `SkillName string`, `Count int`, `SuccessRate float64`, `AvgDurationMs int64`.

#### Scenario: SkillInvocationStat adapter mapping
- **WHEN** `skillsButlerQueryAdapter.GetSkillInvocationStats` is called
- **THEN** it SHALL call `biz.SkillInvocationStatsReader.GetSkillInvocationStats(ctx, agentID, since)`
- **AND** map each `biz` stat to `skills_butler.SkillInvocationStat` with fields `SkillName`, `Count`, `SuccessRate`, `AvgDurationMs`

### Requirement: Skills butler error codes
The system SHALL define error variables in `internal/tools/skills_butler/errors.go` using `kerrors.BadRequest`.

#### Scenario: Error definitions
- **WHEN** input validation fails
- **THEN** the tool SHALL return `errAgentIDRequired` (code "SKILLS_BUTLER", message "agent_id is required")
- **OR** `errSkillNameRequired` (code "SKILLS_BUTLER", message "skill_name is required")
- **OR** `errImprovementDescRequired` (code "SKILLS_BUTLER", message "improvement_description is required")
- **OR** `errTimeRangeRequired` (code "SKILLS_BUTLER", message "time_range is required")
