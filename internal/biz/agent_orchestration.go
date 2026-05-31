package biz

type OrchestrationMode string

const (
	OrchestrationModeSingle   OrchestrationMode = "single"
	OrchestrationModeChain    OrchestrationMode = "chain"
	OrchestrationModeCycle    OrchestrationMode = "cycle"
	OrchestrationModeParallel OrchestrationMode = "parallel"
)

type OrchestrationConfig struct {
	Mode           OrchestrationMode `json:"mode"`
	SubAgentKeys   []string          `json:"sub_agent_keys,omitempty"`
	MaxIterations  *int              `json:"max_iterations,omitempty"`
	EscalationRule string            `json:"escalation_rule,omitempty"`
}
