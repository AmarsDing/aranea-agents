## ADDED Requirements

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
