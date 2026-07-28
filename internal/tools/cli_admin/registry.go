// Package cli_admin provides the cli_admin_* tools for the __system_admin__ agent.
//
// Tools in this package are injected at service startup via RegisterAll(deps).
// They must not import pkg/trpc-agent-go tool implementations directly in the
// registry — that is handled by the individual tool files.
//
// Architectural contract:
//   - This package defines Deps interfaces; concrete implementations are injected
//     from internal/service to avoid circular imports.
//   - Tools only run in the context of the __system_admin__ agent (enforced by
//     IsCLIAdminAllowed).
package cli_admin

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Deps carries all external dependencies the cli_admin tools need.
// Implementations are injected by internal/service when building the system admin agent.
type Deps struct {
	SkillRepo  SkillRepository
	AgentRepo  AgentRepository
	APIBaseURL string
	APIToken   string
	// AllowedPrivateHosts lists Git hosts exempt from the outboundguard
	// public-host check (e.g. intranet GitLab). Exact host match only.
	AllowedPrivateHosts []string
}

func (d Deps) String() string {
	token := d.APIToken
	if token != "" {
		token = "***"
	}
	return fmt.Sprintf("Deps{APIBaseURL:%q, APIToken:%q}", d.APIBaseURL, token)
}

// SkillRepository defines the skill read interface for cli_admin tools.
type SkillRepository interface {
	ListSkills(ctx context.Context, keyword string, limit, offset int32) ([]SkillItem, int32, error)
	GetSkill(ctx context.Context, id string) (*SkillItem, error)
}

// AgentRepository defines the agent read interface for cli_admin tools.
type AgentRepository interface {
	ListAgents(ctx context.Context, keyword string, limit, offset int32) ([]AgentItem, int32, error)
	GetAgent(ctx context.Context, id string) (*AgentItem, error)
	GetAgentByAgentKey(ctx context.Context, agentKey string) (*AgentItem, error)
}

// SkillItem is a lightweight skill representation returned by tools.
type SkillItem struct {
	ID          string `json:"id"`
	SkillKey    string `json:"skill_key"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status"`
	Version     string `json:"version"`
	// Enabled is exposed so install verification (system-admin prompt: confirm
	// enabled=true) checks the same fact the tool_assertion gate asserts.
	Enabled bool `json:"enabled"`
}

// AgentItem is a lightweight agent representation returned by tools.
type AgentItem struct {
	ID          string `json:"id"`
	AgentKey    string `json:"agent_key"`
	DisplayName string `json:"display_name"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Status      string `json:"status"`
}

// IsCLIAdminAllowed returns true only for the system admin agent.
func IsCLIAdminAllowed(agentKey string) bool {
	return agentKey == biz.SystemAdminAgentKey
}

// RegisterAll returns all cli_admin tools instantiated with the given deps.
// Call this from internal/service when building the __system_admin__ agent runner.
func RegisterAll(deps Deps) []trpctool.Tool {
	return []trpctool.Tool{
		newSkillListTool(deps),
		newSkillGetTool(deps),
		newSkillInstallFromURLTool(deps),
		newAgentListTool(deps),
		newAgentGetTool(deps),
		newPkgInstallFromURLTool(deps),
	}
}
