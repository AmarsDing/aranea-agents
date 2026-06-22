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
	// TODO(debt): DEV-03 — AgentCapability.Capacity field unused. No conflict detection
	// or load balancing logic implemented yet.
	// See: https://github.com/aranea-agents/aranea-agents/issues/DEV-03
	Capacity int // max concurrent tasks
}
