package biz

// ToolWeightReport is the per-tool weight analysis result.
type ToolWeightReport struct {
	ToolKey        string  `json:"tool_key"`
	CallCount      int     `json:"call_count"`
	SuccessRate    float64 `json:"success_rate"`
	AvgDurationMS  float64 `json:"avg_duration_ms"`
	WeightScore    float64 `json:"weight_score"`
	Recommendation string  `json:"recommendation"` // "promote" | "demote" | "keep" | "disable"
}

// SkillHealth is the per-skill health analysis result.
type SkillHealth struct {
	SkillID        string  `json:"skill_id"`
	InvokeCount7d  int     `json:"invoke_count_7d"`
	SuccessRate    float64 `json:"success_rate"`
	AvgDurationMS  float64 `json:"avg_duration_ms"`
	Trend          string  `json:"trend"`           // "rising" | "stable" | "declining" | "dormant"
	HealthStatus   string  `json:"health_status"`   // "healthy" | "warning" | "critical" | "dormant"
	Recommendation string  `json:"recommendation"`  // "keep" | "evolve" | "retire" | "merge"
}

// OrchestrationQuality is the DQ score for orchestration analysis.
type OrchestrationQuality struct {
	Validity    float64 `json:"validity"`
	Specificity float64 `json:"specificity"`
	Correctness float64 `json:"correctness"`
	Efficiency  float64 `json:"efficiency"`
	DQScore     float64 `json:"dq_score"`
}

// OrchestrationModeReport is the per-mode orchestration analysis result.
type OrchestrationModeReport struct {
	Mode                string             `json:"mode"`
	SuccessRate         float64            `json:"success_rate"`
	AvgTokens           int                `json:"avg_tokens"`
	AvgDurationSec      int                `json:"avg_duration_sec"`
	MemberContributions map[string]float64 `json:"member_contributions"`
	DQScore             float64            `json:"dq_score"`
}

// MemoryQualityReport is the memory health analysis result.
type MemoryQualityReport struct {
	HitRate          float64 `json:"hit_rate"`
	MissRate         float64 `json:"miss_rate"`
	RedundancyScore  float64 `json:"redundancy_score"`
	MisalignedCount  int     `json:"misaligned_count"`
	InactiveCount    int     `json:"inactive_count"`
	PredictableCount int     `json:"predictable_count"`
	HealthScore      float64 `json:"health_score"`
	TotalFacts       int     `json:"total_facts"`
}

// AgentCapabilityProfile is the per-agent capability analysis result.
type AgentCapabilityProfile struct {
	AgentID                    string             `json:"agent_id"`
	ToolSuccessRates           map[string]float64 `json:"tool_success_rates"`
	SkillScores                map[string]float64 `json:"skill_scores"`
	OrchestrationContributions map[string]float64 `json:"orchestration_contributions"`
	CostEfficiency             float64            `json:"cost_efficiency"`
}

// ForgetConfig stores the memory butler's forget policy configuration.
type ForgetConfig struct {
	Policy                       string  `json:"policy"`                          // "hybrid" (default for P0)
	MaxMemoryCount               int     `json:"max_memory_count"`                // default 1000
	MaxMemoryAgeDays             int     `json:"max_memory_age_days"`             // default 90
	InactiveThresholdDays        int     `json:"inactive_threshold_days"`         // default 30
	MisalignedInputSimThreshold  float64 `json:"misaligned_input_sim_threshold"`  // default 0.8
	MisalignedOutputSimThreshold float64 `json:"misaligned_output_sim_threshold"` // default 0.5
	PredictionErrorThreshold     float64 `json:"prediction_error_threshold"`      // default 0.3
	DedupSimThreshold            float64 `json:"dedup_sim_threshold"`             // default 0.95
}

// DefaultForgetConfig returns the default forget policy configuration.
func DefaultForgetConfig() ForgetConfig {
	return ForgetConfig{
		Policy:                       "hybrid",
		MaxMemoryCount:               1000,
		MaxMemoryAgeDays:             90,
		InactiveThresholdDays:        30,
		MisalignedInputSimThreshold:  0.8,
		MisalignedOutputSimThreshold: 0.5,
		PredictionErrorThreshold:     0.3,
		DedupSimThreshold:            0.95,
	}
}

// DreamSnapshot stores a snapshot of facts before dream_cycle execution.
type DreamSnapshot struct {
	ExecutedAt   string         `json:"executed_at"`
	DeletedFacts []FactSnapshot `json:"deleted_facts"`
	MergedFacts  []FactSnapshot `json:"merged_facts"`
}

// FactSnapshot is a snapshot of a single fact for dream_cycle rollback.
type FactSnapshot struct {
	ID        string `json:"id"`
	Statement string `json:"statement"`
	ScopeType string `json:"scope_type"`
	ScopeID   string `json:"scope_id"`
	Kind      string `json:"kind"`
}
