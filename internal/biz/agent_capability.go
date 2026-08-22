package biz

// AgentCapability represents an agent's capabilities for matching
type AgentCapability struct {
	AgentKey    string
	DisplayName string
	Description string
	Roles       []string
	Domains     []string
	Tools       []string
	Skills      []string
	// Mission 是 Agent 使命文本（agents.mission_statement，空时回退 Description）。
	Mission string
	// DomainPath 是 Agent 的归一化领域路径（agents.domain_path）。
	DomainPath string
	// TODO(debt): DEV-03 — AgentCapability.Capacity field unused. No conflict detection
	// or load balancing logic implemented yet.
	// See: https://github.com/aranea-agents/aranea-agents/issues/DEV-03
	Capacity int // max concurrent tasks

	// Organization placement (M78). Empty when the agent has no position or
	// the org reader was not wired — matching then skips org prune.
	PositionID     string
	PositionKey    string
	DepartmentID   string
	DepartmentName string
	CompanyID      string
	CompanyName    string
	AgentVariant   string
}

// IsHeuristicAssignable reports whether this capability may be selected by
// Allocate (not AllocateExplicit). System keys and department leads are out.
func (c AgentCapability) IsHeuristicAssignable() bool {
	if IsSystemAgentKey(c.AgentKey) {
		return false
	}
	return !IsDeptLeadAgent(Agent{AgentKey: c.AgentKey, AgentVariant: c.AgentVariant})
}
