## ADDED Requirements

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
