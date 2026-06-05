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
	Capacity    int // max concurrent tasks
}
