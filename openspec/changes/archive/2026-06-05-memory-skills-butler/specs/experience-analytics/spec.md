## ADDED Requirements

### Requirement: ExperienceAnalyticsUsecase shared infrastructure
The system SHALL define `ExperienceAnalyticsUsecase` in `internal/biz/experience_analytics.go` as a biz-layer Usecase (NOT an Agent) that composes existing repository interfaces. It SHALL serve as shared analytics infrastructure for both the memory butler and skills butler.

#### Scenario: Usecase composes existing repos without new interfaces
- **WHEN** `NewExperienceAnalyticsUsecase` is constructed
- **THEN** it SHALL depend on `biz.EvolutionMetricsRepo`, `biz.SkillQueryReader`, `biz.TeamRepository`, `biz.UsageAnalyticsRepo`, `*biz.MemoryAdminUsecase`, `biz.SessionReader`, `biz.ToolInvocationReader`, and `loggateway.Logger`
- **AND** it SHALL NOT define any new repository interface

#### Scenario: Wire registration in biz ProviderSet
- **WHEN** the application starts via Wire
- **THEN** `NewExperienceAnalyticsUsecase` SHALL be included in `internal/biz/biz.go` ProviderSet
- **AND** `biz.ToolInvocationReader` SHALL be bound to the data layer implementation in `internal/data/data.go` ProviderSet

#### Scenario: ChatServiceDeps receives ExperienceAnalyticsUsecase
- **WHEN** `provideChatServiceDeps` is called in `cmd/admin/wire.go`
- **THEN** it SHALL include `*biz.ExperienceAnalyticsUsecase` as a parameter

### Requirement: AnalyzeToolWeights
`ExperienceAnalyticsUsecase` SHALL provide `AnalyzeToolWeights(ctx context.Context, agentID string, since time.Time) (ToolWeightAnalysis, error)` that computes per-tool weight scores based on historical invocation data.

#### Scenario: Tool weight calculation
- **WHEN** `AnalyzeToolWeights` is called with `agentID` and `since`
- **THEN** it SHALL query `biz.ToolInvocationReader.SearchToolInvocations` for data since the given time
- **AND** compute `WeightScore = normalize(success_rate) * 0.5 + normalize(call_count) * 0.3 + normalize(1/duration) * 0.2` per tool
- **AND** return a `ToolWeightAnalysis` containing `AgentID` and `Items []ToolWeightItem` with `ToolKey`, `CallCount`, `SuccessCount`, `SuccessRate`, `AvgDurationMS`, `WeightScore`, `Recommendation`

#### Scenario: Tool weight recommendation
- **WHEN** a tool's `WeightScore` is computed
- **THEN** the `Recommendation` SHALL be "keep" if `WeightScore >= 0.7 AND SuccessRate >= 0.9`, "monitor" if `WeightScore >= 0.5`, "disable" if `SuccessRate < 0.5`, "review" otherwise

#### Scenario: AgentID required
- **WHEN** `AnalyzeToolWeights` is called with empty `agentID`
- **THEN** it SHALL return a BadRequest error

### Requirement: AnalyzeSkillHealth
`ExperienceAnalyticsUsecase` SHALL provide `AnalyzeSkillHealth(ctx context.Context, agentID string, since time.Time) (SkillHealthAnalysis, error)` that evaluates each Skill's health based on invocation data.

#### Scenario: Skill health data source
- **WHEN** `AnalyzeSkillHealth` is called with `agentID` and `since`
- **THEN** it SHALL query `biz.SkillQueryReader.SearchSkillInvocations` with the given agentID and time range
- **AND** aggregate per-skill `InvokeCount`, `SuccessCount`, `FailureCount`, `SuccessRate`, `AvgDurationMS`

#### Scenario: Skill health status determination
- **WHEN** a Skill's aggregated metrics are computed
- **THEN** `HealthStatus` SHALL be "unused" if `InvokeCount == 0`
- **AND** "healthy" if `SuccessRate >= 0.9`
- **AND** "degraded" if `0.7 <= SuccessRate < 0.9`
- **AND** "unstable" if `0.5 <= SuccessRate < 0.7`
- **AND** "critical" if `SuccessRate < 0.5`

#### Scenario: Skill health recommendation
- **WHEN** a Skill's `HealthStatus` is determined
- **THEN** `Recommendation` SHALL be "consider_removing" for "unused", "keep" for "healthy", "review_errors" for "degraded", "investigate_failures" for "unstable", "disable_or_rewrite" for "critical"

#### Scenario: Trend computed by adapter
- **WHEN** the skills butler adapter converts `SkillHealthItem` to `biz.SkillHealth`
- **THEN** `Trend` SHALL be "dormant" if `InvokeCount == 0`, "declining" if `SuccessRate < 0.5`, "rising" if `SuccessRate >= 0.9`, "stable" otherwise

### Requirement: AnalyzeOrchestration
`ExperienceAnalyticsUsecase` SHALL provide `AnalyzeOrchestration(ctx context.Context, agentID string, since time.Time) (OrchestrationAnalysis, error)` that evaluates orchestration efficiency per mode.

#### Scenario: Orchestration data aggregation
- **WHEN** `AnalyzeOrchestration` is called with `agentID` and `since`
- **THEN** it SHALL query `biz.TeamRepository` for team_runs within the time range
- **AND** group results by orchestration mode

#### Scenario: DQ Score computation per mode
- **WHEN** orchestration data is grouped by mode
- **THEN** each mode's `DQScore` SHALL be computed as `0.4 * Validity + 0.3 * Specificity + 0.3 * Correctness`
- **WHERE** `Validity = success_rate`, `Specificity = (RunCount - ErrorCount) / RunCount`, `Correctness = SuccessCount / RunCount`

#### Scenario: Mode filter applied by adapter
- **WHEN** the skills butler adapter calls `AnalyzeOrchestration` with `modeFilter`
- **THEN** the adapter SHALL filter results by mode after receiving the analysis

#### Scenario: OrchestrationModeItem fields
- **WHEN** `AnalyzeOrchestration` returns results
- **THEN** each `OrchestrationModeItem` SHALL contain `Mode`, `RunCount`, `SuccessCount`, `SuccessRate`, `DQScore`, `Validity`, `Specificity`, `Correctness`
- **AND** `AvgTokens`, `AvgDurationSec`, `MemberContributions` are NOT implemented in P0

### Requirement: AnalyzeMemoryQuality
`ExperienceAnalyticsUsecase` SHALL provide `AnalyzeMemoryQuality(ctx context.Context, agentID string, since time.Time) (MemoryQualityAnalysis, error)` that evaluates memory system health.

#### Scenario: Memory quality metrics computation
- **WHEN** `AnalyzeMemoryQuality` is called with `agentID` and `since`
- **THEN** it SHALL compute `FactCount` from `MemoryAdminUsecase.ListFactRows`
- **AND** `RetrievalQuality` from `EvolutionMetricsRepo.GetRetrievalQuality`
- **AND** `NegativeFeedback` from `EvolutionMetricsRepo.GetNegativeFeedbackCount`

#### Scenario: HealthScore calculation
- **WHEN** memory quality metrics are computed
- **THEN** `HealthScore` SHALL be calculated as:
  `0.4 * coverageScore + 0.4 * retrievalQuality + 0.2 * (1 - penalty)`
- **WHERE** `coverageScore = min(FactCount / 100.0, 1.0)`, `penalty = min(NegativeFeedback / 10.0, 1.0)`
- **AND** the result SHALL be in range [0, 1]

#### Scenario: Memory quality recommendation
- **WHEN** `HealthScore >= 0.8`
- **THEN** `Recommendation` SHALL be "healthy"
- **WHEN** `0.6 <= HealthScore < 0.8`
- **THEN** `Recommendation` SHALL be "review_facts"
- **WHEN** `FactCount == 0`
- **THEN** `Recommendation` SHALL be "seed_memory"
- **WHEN** otherwise
- **THEN** `Recommendation` SHALL be "prune_and_enrich"

### Requirement: AnalyzeAgentCapability
`ExperienceAnalyticsUsecase` SHALL provide `AnalyzeAgentCapability(ctx context.Context, agentID string, timeRange string) (AgentCapabilityAnalysis, error)` that builds a combined capability report for an agent.

#### Scenario: Agent capability analysis composition
- **WHEN** `AnalyzeAgentCapability` is called with `agentID` and `timeRange`
- **THEN** it SHALL call `AnalyzeToolWeights`, `AnalyzeSkillHealth`, `AnalyzeOrchestration`, `AnalyzeMemoryQuality` and `computeCostSummary`
- **AND** return `AgentCapabilityAnalysis` containing `ToolWeights`, `SkillHealth`, `Orchestration`, `MemoryQuality`, `CostSummary`

#### Scenario: CostSummary computation
- **WHEN** `computeCostSummary` is called
- **THEN** it SHALL query `UsageAnalyticsRepo.GetModelUsageSummary` for the agent's cost data
- **AND** return `CostSummary` with `TotalCostMicroUSD`, `TotalTokens`, `CallCount`

### Requirement: ToolWeightItem model
The system SHALL define `ToolWeightItem` struct in `internal/biz/experience_analytics.go` with fields: `ToolKey string`, `CallCount int`, `SuccessCount int`, `SuccessRate float64`, `AvgDurationMS float64`, `WeightScore float64`, `Recommendation string` ("keep"|"monitor"|"review"|"disable").

#### Scenario: ToolWeightItem wrapped in ToolWeightAnalysis
- **WHEN** `AnalyzeToolWeights` returns results
- **THEN** items SHALL be wrapped in `ToolWeightAnalysis{AgentID string, Items []ToolWeightItem}`

### Requirement: SkillHealthItem model
The system SHALL define `SkillHealthItem` struct in `internal/biz/experience_analytics.go` with fields: `SkillID string`, `SkillName string`, `InvokeCount int`, `SuccessCount int`, `FailureCount int`, `SuccessRate float64`, `AvgDurationMS float64`, `HealthStatus string` ("healthy"|"degraded"|"unstable"|"critical"|"unused"), `Recommendation string` ("keep"|"review_errors"|"investigate_failures"|"disable_or_rewrite"|"consider_removing").

#### Scenario: SkillHealthItem wrapped in SkillHealthAnalysis
- **WHEN** `AnalyzeSkillHealth` returns results
- **THEN** items SHALL be wrapped in `SkillHealthAnalysis{AgentID string, Items []SkillHealthItem}`

### Requirement: OrchestrationModeItem model
The system SHALL define `OrchestrationModeItem` struct in `internal/biz/experience_analytics.go` with fields: `Mode string`, `RunCount int`, `SuccessCount int`, `SuccessRate float64`, `DQScore float64`, `Validity float64`, `Specificity float64`, `Correctness float64`.

#### Scenario: OrchestrationModeItem wrapped in OrchestrationAnalysis
- **WHEN** `AnalyzeOrchestration` returns results
- **THEN** items SHALL be wrapped in `OrchestrationAnalysis{AgentID string, Items []OrchestrationModeItem}`

### Requirement: MemoryQualityAnalysis model
The system SHALL define `MemoryQualityAnalysis` struct in `internal/biz/experience_analytics.go` with fields: `AgentID string`, `FactCount int`, `RetrievalQuality float64`, `NegativeFeedback int`, `HealthScore float64`, `Recommendation string`.

#### Scenario: MemoryQualityAnalysis not MemoryQualityReport
- **WHEN** code references memory quality results
- **THEN** it SHALL use `MemoryQualityAnalysis` (not `MemoryQualityReport` which is a legacy type in `experience_analytics_types.go`)

### Requirement: AgentCapabilityAnalysis model
The system SHALL define `AgentCapabilityAnalysis` struct in `internal/biz/experience_analytics.go` with fields: `AgentID string`, `ToolWeights ToolWeightAnalysis`, `SkillHealth SkillHealthAnalysis`, `Orchestration OrchestrationAnalysis`, `MemoryQuality MemoryQualityAnalysis`, `CostSummary CostSummary`.

### Requirement: CostSummary model
The system SHALL define `CostSummary` struct in `internal/biz/experience_analytics.go` with fields: `TotalCostMicroUSD int64`, `TotalTokens int`, `CallCount int`.

### Requirement: ToolInvocationReader interface
The system SHALL use the existing `biz.ToolInvocationReader` interface (defined in `internal/biz/tool/tool.go`) for tool invocation detail queries. No new interface SHALL be created.

#### Scenario: ExperienceAnalyticsUsecase uses existing ToolInvocationReader
- **WHEN** `ExperienceAnalyticsUsecase` needs tool invocation details
- **THEN** it SHALL call `biz.ToolInvocationReader.SearchToolInvocations`
- **AND** it SHALL NOT define a new `ToolInvocationReader` in `internal/biz/evolution.go`

### Requirement: SkillInvocationStatsReader interface
The system SHALL define `SkillInvocationStatsReader` in `internal/biz/skill_invocation_stats.go` for skill invocation statistics queries, separate from `SkillQueryReader`.

#### Scenario: Skills butler uses SkillInvocationStatsReader
- **WHEN** skills butler tools need skill invocation statistics
- **THEN** they SHALL use `SkillInvocationStatsReader.GetSkillInvocationStats(ctx, agentID, since)`
- **AND** the adapter `skillsButlerQueryAdapter` SHALL bridge `biz.SkillInvocationStatsReader` to `skills_butler.SkillQueryReaderPort`

### Requirement: DQ Score thresholds
The system SHALL define DQ Score tiered thresholds for orchestration quality interpretation.

#### Scenario: DQ Score interpretation
- **WHEN** `DQScore >= 0.7`
- **THEN** the orchestration SHALL be rated "excellent" and its topology suggested for caching
- **WHEN** `0.5 <= DQScore < 0.7`
- **THEN** the mode SHALL be rated "acceptable" with no action needed
- **WHEN** `0.3 <= DQScore < 0.5`
- **THEN** the mode SHALL be rated "poor" and the skills butler SHALL generate optimization suggestions
- **WHEN** `DQScore < 0.3`
- **THEN** the mode SHALL be rated "failing" and its topology SHALL be suggested as "avoid"

### Requirement: Legacy types in experience_analytics_types.go
The system SHALL maintain legacy types `ToolWeightReport`, `SkillHealth`, `OrchestrationQuality`, `OrchestrationModeReport`, `MemoryQualityReport`, `AgentCapabilityProfile`, `ForgetConfig`, `DreamSnapshot`, `FactSnapshot` in `internal/biz/experience_analytics_types.go` for spec compatibility and adapter bridging. These types are used by the adapter layer to convert biz-layer analysis results to the format expected by butler tools.

#### Scenario: Adapter bridges ToolWeightAnalysis to ToolWeightReport
- **WHEN** `skillsButlerAnalyticsAdapter.AnalyzeToolWeights` converts results
- **THEN** it SHALL map `ToolWeightItem` fields to `ToolWeightReport` fields

#### Scenario: Adapter bridges SkillHealthAnalysis to SkillHealth
- **WHEN** `skillsButlerAnalyticsAdapter.AnalyzeSkillHealth` converts results
- **THEN** it SHALL map `SkillHealthItem` fields to `SkillHealth` fields
- **AND** compute `Trend` based on `InvokeCount` and `SuccessRate`

#### Scenario: Adapter bridges OrchestrationAnalysis to OrchestrationModeReport
- **WHEN** `skillsButlerAnalyticsAdapter.AnalyzeOrchestration` converts results
- **THEN** it SHALL map `OrchestrationModeItem` fields to `OrchestrationModeReport` fields
- **AND** apply `modeFilter` to filter results
