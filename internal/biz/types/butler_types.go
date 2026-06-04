package types

// ButlerTier identifies the tier/role of a butler within the system.
// Used by the butler-unified-architecture to classify and route
// orchestration responsibilities.
type ButlerTier string

const (
	ButlerTierSpirit      ButlerTier = "spirit"      // Top-level orchestrator (plan_and_execute)
	ButlerTierOrchestrator ButlerTier = "orchestrator" // Task orchestration (check_progress, cancel)
	ButlerTierMemory      ButlerTier = "memory"      // Memory lifecycle management
	ButlerTierSkills      ButlerTier = "skills"      // Skill health & evolution
	ButlerTierMonitor     ButlerTier = "monitor"     // Self-check & auto-heal
)

// ButlerCapability enumerates the capabilities a butler tier can perform.
type ButlerCapability string

const (
	ButlerCapabilityPlan    ButlerCapability = "plan"    // Task planning & decomposition
	ButlerCapabilityExecute ButlerCapability = "execute" // Task execution & orchestration
	ButlerCapabilityMonitor ButlerCapability = "monitor" // Health monitoring & alerting
	ButlerCapabilityHeal    ButlerCapability = "heal"    // Auto-healing & recovery
	ButlerCapabilityEvolve  ButlerCapability = "evolve"  // Skill evolution & optimization
	ButlerCapabilityRecall  ButlerCapability = "recall"  // Memory recall & management
)

// OrchestrationStepRecord captures the name, input, output, and status of
// each internal step within a coarse-grained tool invocation (e.g., plan_and_execute).
// This is distinct from the existing biz.OrchestrationStep which is a persisted
// DB entity for timeline reconstruction.
type OrchestrationStepRecord struct {
	StepName  string `json:"step_name"`  // e.g., "classify_industry", "assemble_team"
	InputJSON string `json:"input_json,omitempty"`
	OutputJSON string `json:"output_json,omitempty"`
	Status    string `json:"status"`     // "pending", "running", "completed", "failed", "skipped"
	Error     string `json:"error,omitempty"`
	StartedAt string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
}
