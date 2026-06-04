## ADDED Requirements

### Requirement: SkillHealth type in biz/types
The system SHALL define `SkillHealth` struct in `internal/biz/types/skill_health.go` with fields: `SkillSlug string`, `HealthScore float64`, `SuccessRate float64`, `AvgLatencyMs float64`, `InvocationCount int64`, `FailureCount int64`, `LastInvokedAt time.Time`. All modules (skill-butler-tools, skill-intelligence, memory-skills-butler) MUST import this type from `biz/types` instead of defining their own.

#### Scenario: skill-butler-tools uses unified SkillHealth
- **WHEN** `analyze_skill_usage` tool computes skill health
- **THEN** it returns `types.SkillHealth` from `internal/biz/types` package

#### Scenario: skill-intelligence uses unified SkillHealth
- **WHEN** SkillIntelligenceWorker generates experience reports
- **THEN** it uses `types.SkillHealth` from `internal/biz/types` package

### Requirement: ToolWeightReport type in biz/types
The system SHALL define `ToolWeightReport` struct in `internal/biz/types/skill_health.go` with fields: `ToolName string`, `CurrentWeight float64`, `SuggestedWeight float64`, `UsageFrequency float64`, `SuccessContribution float64`. All modules MUST import this type from `biz/types`.

#### Scenario: optimize_skill tool uses unified ToolWeightReport
- **WHEN** `optimize_skill` tool generates weight suggestions
- **THEN** it returns `types.ToolWeightReport` from `internal/biz/types` package

### Requirement: ExperienceReport type in biz/types
The system SHALL define `ExperienceReport` struct in `internal/biz/types/skill_health.go` with fields: `SkillSlug string`, `Period time.Duration`, `Health SkillHealth`, `ToolWeights []ToolWeightReport`, `FailurePatterns []string`, `OptimizationSuggestions []string`. All modules MUST import this type from `biz/types`.

#### Scenario: skill-intelligence generates ExperienceReport
- **WHEN** SkillIntelligenceUsecase generates a diagnostic report
- **THEN** it returns `types.ExperienceReport` from `internal/biz/types` package

### Requirement: No duplicate type definitions across modules
The system MUST NOT contain duplicate definitions of SkillHealth, ToolWeightReport, or ExperienceReport in any package other than `internal/biz/types/`. Existing duplicate definitions in `internal/tools/skills_butler/` or `internal/biz/skill_intelligence.go` MUST be replaced with imports from `biz/types`.

#### Scenario: Grep verification of type uniqueness
- **WHEN** a developer searches for `type SkillHealth struct` across the codebase
- **THEN** exactly one definition exists in `internal/biz/types/skill_health.go`
