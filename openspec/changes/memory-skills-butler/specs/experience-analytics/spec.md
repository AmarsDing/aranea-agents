## ADDED Requirements

### Requirement: ExperienceAnalyticsUsecase shared infrastructure
The system SHALL define `ExperienceAnalyticsUsecase` in `internal/biz/experience_analytics.go` as a biz-layer Usecase (NOT an Agent) that composes existing repository interfaces. It SHALL serve as shared analytics infrastructure for both the memory butler and skills butler.

#### Scenario: Usecase composes existing repos without new interfaces
- **WHEN** `NewExperienceAnalyticsUsecase` is constructed
- **THEN** it SHALL depend on `biz.EvolutionMetricsRepo`, `biz.SkillQueryReader`, `biz.TeamRepository`, `usage.AnalyticsRepo`, `*biz.MemoryAdminUsecase`, `biz.SessionReader`, and `biz.ToolInvocationReader`
- **AND** it SHALL NOT define any new repository interface

#### Scenario: Wire registration in biz ProviderSet
- **WHEN** the application starts via Wire
- **THEN** `NewExperienceAnalyticsUsecase` SHALL be included in `internal/biz/biz.go` ProviderSet
- **AND** `biz.ToolInvocationReader` SHALL be bound to `*ToolInvocationData` in `internal/data/data.go` ProviderSet

#### Scenario: ChatServiceDeps receives ExperienceAnalyticsUsecase
- **WHEN** `provideChatServiceDeps` is called in `cmd/admin/wire.go`
- **THEN** it SHALL include `*biz.ExperienceAnalyticsUsecase` as a parameter

### Requirement: AnalyzeToolWeights
`ExperienceAnalyticsUsecase` SHALL provide `AnalyzeToolWeights(ctx context.Context) ([]ToolWeightReport, error)` that computes per-tool weight scores based on historical invocation data.

#### Scenario: Tool weight calculation
- **WHEN** `AnalyzeToolWeights` is called
- **THEN** it SHALL query `biz.ToolInvocationReader.ListToolInvocations` for the last 30 days of data
- **AND** compute `WeightScore = normalize(success_rate) * 0.5 + normalize(call_count) * 0.3 + normalize(1/duration) * 0.2` per tool
- **AND** return a `ToolWeightReport` for each tool with `ToolKey`, `CallCount`, `SuccessRate`, `AvgDurationMS`, `WeightScore`, `Recommendation`

#### Scenario: Tool weight recommendation
- **WHEN** a tool's `WeightScore` is computed
- **THEN** the `Recommendation` SHALL be "promote" if `WeightScore >= 0.7`, "keep" if `0.3 <= WeightScore < 0.7`, "demote" if `0.1 <= WeightScore < 0.3`, "disable" if `WeightScore < 0.1`

#### Scenario: Insufficient data fallback
- **WHEN** fewer than 10 tool invocation records exist
- **THEN** `AnalyzeToolWeights` SHALL return an error indicating insufficient data rather than unreliable results

### Requirement: AnalyzeSkillHealth
`ExperienceAnalyticsUsecase` SHALL provide `AnalyzeSkillHealth(ctx context.Context) ([]SkillHealth, error)` that evaluates each Skill's health based on 7-day invocation data.

#### Scenario: Skill health data source
- **WHEN** `AnalyzeSkillHealth` is called
- **THEN** it SHALL query `biz.SkillQueryReader.SearchSkillInvocations` with `Since` set to 7 days ago and `Limit` 1000
- **AND** aggregate per-skill `InvokeCount7d`, `SuccessRate`, `AvgDurationMS`

#### Scenario: Skill health status determination
- **WHEN** a Skill's aggregated metrics are computed
- **THEN** `HealthStatus` SHALL be "healthy" if `InvokeCount7d > 10` AND `SuccessRate > 0.8`
- **AND** "warning" if `InvokeCount7d > 5` AND `0.6 <= SuccessRate <= 0.8`
- **AND** "critical" if `InvokeCount7d < 2` OR `SuccessRate < 0.6`
- **AND** "dormant" if 30 days have no invocations

#### Scenario: Skill health recommendation
- **WHEN** a Skill's `HealthStatus` is determined
- **THEN** `Recommendation` SHALL be "keep" for "healthy", "evolve" for "warning", "evolve" or "retire" for "critical" (based on whether `InvokeCount7d > 0`), "retire" for "dormant"

#### Scenario: Skill trend detection
- **WHEN** a Skill has invocation data across multiple 7-day windows
- **THEN** `Trend` SHALL be "rising" if recent 7-day count > previous 7-day count by >20%, "declining" if < -20%, "stable" otherwise, "dormant" if zero invocations

### Requirement: AnalyzeOrchestration
`ExperienceAnalyticsUsecase` SHALL provide `AnalyzeOrchestration(ctx context.Context, timeRange string, modeFilter string) ([]OrchestrationModeReport, error)` that evaluates orchestration efficiency per mode.

#### Scenario: Orchestration data aggregation
- **WHEN** `AnalyzeOrchestration` is called with `timeRange="30d"` and `modeFilter=""`
- **THEN** it SHALL query `biz.TeamRepository` for team_runs and team_run_steps within the time range
- **AND** group results by orchestration mode (sequential/parallel/coordinator/swarm/adaptive)

#### Scenario: DQ Score computation per mode
- **WHEN** orchestration data is grouped by mode
- **THEN** each mode's `DQScore` SHALL be computed as `0.4 * Validity + 0.3 * Specificity + 0.3 * Correctness`
- **WHERE** `Validity = success_rate`, `Specificity = min(avg_output_length / 500, 1.0)`, `Correctness = 1 - negative_feedback_rate`

#### Scenario: Member contribution calculation
- **WHEN** a mode has team_run_steps data
- **THEN** `MemberContributions` SHALL be computed as `map[agent_id]float64` where each value is `agent_success_count / agent_total_count`

#### Scenario: Mode filter applied
- **WHEN** `AnalyzeOrchestration` is called with `modeFilter="coordinator"`
- **THEN** it SHALL only return the `OrchestrationModeReport` for the "coordinator" mode

#### Scenario: Minimum data requirement
- **WHEN** a mode has fewer than 10 team_run records
- **THEN** that mode's report SHALL include a warning that results may be unreliable

### Requirement: AnalyzeMemoryQuality
`ExperienceAnalyticsUsecase` SHALL provide `AnalyzeMemoryQuality(ctx context.Context, agentID string) (*MemoryQualityReport, error)` that evaluates memory system health.

#### Scenario: Memory quality metrics computation
- **WHEN** `AnalyzeMemoryQuality` is called with an `agentID`
- **THEN** it SHALL compute `HitRate` from memory_search tool invocation success rate
- **AND** `MissRate = 1 - HitRate`
- **AND** `RedundancyScore` based on embedding cosine similarity > 0.95 pairs
- **AND** `MisalignedCount` as the count of facts with high retrieval but high negative feedback rate
- **AND** `InactiveCount` as facts not retrieved within `inactive_threshold_days`
- **AND** `PredictableCount` as facts with prediction error below threshold

#### Scenario: HealthScore calculation
- **WHEN** memory quality metrics are computed
- **THEN** `HealthScore` SHALL be calculated as:
  `0.3 * hit_rate + 0.2 * (1 - redundancy_score) + 0.2 * (1 - misaligned_count/max(total_facts,1)) + 0.15 * (1 - inactive_count/max(total_facts,1)) + 0.15 * (1 - predictable_count/max(total_facts,1))`
- **AND** the result SHALL be in range [0, 1]

#### Scenario: Global memory quality analysis
- **WHEN** `AnalyzeMemoryQuality` is called with empty `agentID`
- **THEN** it SHALL aggregate metrics across all agents

### Requirement: AnalyzeAgentCapability
`ExperienceAnalyticsUsecase` SHALL provide `AnalyzeAgentCapability(ctx context.Context) (map[string]AgentCapabilityProfile, error)` that builds capability profiles for each agent based on historical performance.

#### Scenario: Agent capability profile construction
- **WHEN** `AnalyzeAgentCapability` is called
- **THEN** for each agent it SHALL compute `ToolSuccessRates` from `ToolInvocationReader`, `SkillScores` from `SkillQueryReader`, `OrchestrationContributions` from `TeamRepository`, and `CostEfficiency` from `usage.AnalyticsRepo`

#### Scenario: Capability profile used by skills butler
- **WHEN** the skills butler calls `AnalyzeAgentCapability`
- **THEN** the result SHALL inform `recommend_skills` tool decisions about which agents benefit from which skills

### Requirement: ToolWeightReport model
The system SHALL define `ToolWeightReport` struct in `internal/biz/experience_analytics_types.go` with fields: `ToolKey string`, `CallCount int`, `SuccessRate float64`, `AvgDurationMS float64`, `WeightScore float64`, `Recommendation string` ("promote"|"demote"|"keep"|"disable").

#### Scenario: ToolWeightReport serialization
- **WHEN** a `ToolWeightReport` is returned from `AnalyzeToolWeights`
- **THEN** it SHALL be JSON-serializable with all fields populated

### Requirement: SkillHealth model
The system SHALL define `SkillHealth` struct in `internal/biz/experience_analytics_types.go` with fields: `SkillID string`, `InvokeCount7d int`, `SuccessRate float64`, `AvgDurationMS float64`, `Trend string` ("rising"|"stable"|"declining"|"dormant"), `HealthStatus string` ("healthy"|"warning"|"critical"|"dormant"), `Recommendation string` ("keep"|"evolve"|"retire"|"merge").

#### Scenario: SkillHealth JSON output
- **WHEN** a `SkillHealth` is returned from `AnalyzeSkillHealth`
- **THEN** it SHALL be JSON-serializable and consumable by the skills butler's `analyze_skill_health` tool

### Requirement: OrchestrationQuality model
The system SHALL define `OrchestrationQuality` struct in `internal/biz/experience_analytics_types.go` with fields: `Validity float64`, `Specificity float64`, `Correctness float64`, `Efficiency float64`, `DQScore float64`.

#### Scenario: DQ Score formula
- **WHEN** an `OrchestrationQuality` is computed
- **THEN** `DQScore` SHALL equal `0.4 * Validity + 0.3 * Specificity + 0.3 * Correctness`

#### Scenario: DQ Score thresholds
- **WHEN** `DQScore >= 0.7`
- **THEN** the orchestration mode SHALL be rated "excellent" and its topology cached
- **WHEN** `0.5 <= DQScore < 0.7`
- **THEN** the mode SHALL be rated "acceptable" with no action needed
- **WHEN** `0.3 <= DQScore < 0.5`
- **THEN** the mode SHALL be rated "poor" and the skills butler SHALL generate optimization suggestions
- **WHEN** `DQScore < 0.3`
- **THEN** the mode SHALL be rated "failing" and its topology SHALL be marked as "avoid"

### Requirement: MemoryQualityReport model
The system SHALL define `MemoryQualityReport` struct in `internal/biz/experience_analytics_types.go` with fields: `HitRate float64`, `MissRate float64`, `RedundancyScore float64`, `MisalignedCount int`, `InactiveCount int`, `PredictableCount int`, `HealthScore float64`.

#### Scenario: HealthScore threshold triggers dream_cycle
- **WHEN** `HealthScore < 0.6`
- **THEN** the memory butler SHALL trigger `dream_cycle`

#### Scenario: HealthScore tiered thresholds
- **WHEN** `0.8 <= HealthScore <= 1.0`
- **THEN** memory system status SHALL be "healthy" with no action needed
- **WHEN** `0.6 <= HealthScore < 0.8`
- **THEN** memory system status SHALL be "suboptimal" and the butler SHALL suggest memory cleanup
- **WHEN** `0.4 <= HealthScore < 0.6`
- **THEN** memory system status SHALL be "unhealthy" and `dream_cycle` SHALL auto-trigger with `dry_run=true`
- **WHEN** `HealthScore < 0.4`
- **THEN** memory system status SHALL be "critical" and `dream_cycle` SHALL auto-trigger with `dry_run=false` plus an alert notification

### Requirement: ToolInvocationReader interface
The system SHALL use the existing `biz.ToolInvocationReader` interface (defined in `internal/biz/tool/tool.go`) for tool invocation detail queries. No new interface SHALL be created.

#### Scenario: ExperienceAnalyticsUsecase uses existing ToolInvocationReader
- **WHEN** `ExperienceAnalyticsUsecase` needs tool invocation details
- **THEN** it SHALL call `biz.ToolInvocationReader.SearchToolInvocations` and `biz.ToolInvocationReader.GetToolInvocationParams`
- **AND** it SHALL NOT define a new `ToolInvocationReader` in `internal/biz/evolution.go`

### Requirement: OrchestrationModeReport model
The system SHALL define `OrchestrationModeReport` struct in `internal/biz/experience_analytics_types.go` with fields: `Mode string`, `SuccessRate float64`, `AvgTokens int`, `AvgDurationSec int`, `MemberContributions map[string]float64`, `DQScore float64`.

#### Scenario: OrchestrationModeReport per mode
- **WHEN** `AnalyzeOrchestration` returns results
- **THEN** each `OrchestrationModeReport` SHALL contain aggregated metrics for one orchestration mode
