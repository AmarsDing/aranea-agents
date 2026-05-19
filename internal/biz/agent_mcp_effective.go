package biz

import (
	"context"
	"strings"
)

// ToolKeyMCPToolSet is the platform tool_key for mounting external MCP servers at runtime.
// It appears in effective-tools when the agent profile allows it (see tools table seeds).
const ToolKeyMCPToolSet = "mcp_tool_set"

const ToolKeyMCPBroker = "mcp_broker"

const ToolKeyKnowledgeSearch = "knowledge_search"

const ToolKeyCallAgent = "call_agent"

// EffectiveMCPServer is a biz DTO for one enabled MCP server row — no ADK imports.
type EffectiveMCPServer struct {
	ID         string
	ServerKey  string
	ConfigJSON string
}

// mcpServerAllowPrefix in ToolsAllowJSON / ToolsDenyJSON lists restricts MCP exposure by platform server key, e.g. mcp:my_server.
const mcpServerAllowPrefix = "mcp:"

// EffectiveMCPPolicy is derived from agent tool allow/deny JSON (entries prefixed with mcp:).
type EffectiveMCPPolicy struct {
	AllowServerKeys []string
	DenyServerKeys  []string
}

// MCPPolicyFromAgentEffectiveTools extracts mcp:<server_key> entries from raw allow/deny lists.
func MCPPolicyFromAgentEffectiveTools(eff AgentEffectiveTools) EffectiveMCPPolicy {
	var allow, deny []string
	for _, s := range eff.Allow {
		sTrim := strings.TrimSpace(s)
		if after, ok := strings.CutPrefix(sTrim, mcpServerAllowPrefix); ok {
			if k := strings.TrimSpace(after); k != "" {
				allow = append(allow, k)
			}
		}
	}
	for _, s := range eff.Deny {
		sTrim := strings.TrimSpace(s)
		if after, ok := strings.CutPrefix(sTrim, mcpServerAllowPrefix); ok {
			if k := strings.TrimSpace(after); k != "" {
				deny = append(deny, k)
			}
		}
	}
	return EffectiveMCPPolicy{
		AllowServerKeys: uniqueLowerKeys(allow),
		DenyServerKeys:  uniqueLowerKeys(deny),
	}
}

func uniqueLowerKeys(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		k := strings.ToLower(strings.TrimSpace(s))
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, k)
	}
	return out
}

// FilterEffectiveMCPServers applies allow (if any mcp: entries were present) and deny to platform server rows.
func FilterEffectiveMCPServers(servers []EffectiveMCPServer, pol EffectiveMCPPolicy) []EffectiveMCPServer {
	deny := map[string]bool{}
	for _, k := range pol.DenyServerKeys {
		deny[k] = true
	}
	var out []EffectiveMCPServer
	for _, s := range servers {
		sk := strings.TrimSpace(s.ServerKey)
		if sk == "" {
			sk = strings.TrimSpace(s.ID)
		}
		low := strings.ToLower(sk)
		if deny[low] {
			continue
		}
		if len(pol.AllowServerKeys) > 0 {
			allowed := false
			for _, a := range pol.AllowServerKeys {
				if a == low {
					allowed = true
					break
				}
			}
			if !allowed {
				continue
			}
		}
		out = append(out, s)
	}
	return out
}

// AgentMCPTooling resolves which platform MCP servers apply to an agent turn (Kratos biz layer).
type AgentMCPTooling struct {
	agents *AgentUsecase
	mcp    *MCPServerUsecase
}

// NewAgentMCPTooling wires effective MCP resolution; either arg may be nil (returns empty servers).
func NewAgentMCPTooling(agents *AgentUsecase, mcp *MCPServerUsecase) *AgentMCPTooling {
	return &AgentMCPTooling{agents: agents, mcp: mcp}
}

func effectiveToolsAllowsMCP(eff AgentEffectiveTools) bool {
	return effectiveToolsAllowsMCPServers(eff)
}

func effectiveToolsAllowsMCPServers(eff AgentEffectiveTools) bool {
	if !eff.ToolsEnabled {
		return false
	}
	for _, it := range eff.Items {
		if !it.Enabled {
			continue
		}
		k := strings.TrimSpace(strings.ToLower(it.ToolKey))
		if k == strings.ToLower(ToolKeyMCPToolSet) || k == strings.ToLower(ToolKeyMCPBroker) {
			return true
		}
	}
	return false
}

// EffectiveServersForAgent returns MCP server rows when mcp_tool_set or mcp_broker is enabled.
func (t *AgentMCPTooling) EffectiveServersForAgent(ctx context.Context, agentID string) ([]EffectiveMCPServer, error) {
	if t == nil || t.agents == nil || t.mcp == nil {
		return nil, nil
	}
	id := strings.TrimSpace(agentID)
	if id == "" {
		return nil, nil
	}
	eff, err := t.agents.GetEffectiveTools(ctx, id)
	if err != nil {
		return nil, err
	}
	if !effectiveToolsAllowsMCPServers(eff) {
		return nil, nil
	}
	rows, err := t.mcp.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]EffectiveMCPServer, 0, len(rows))
	for _, row := range rows {
		if !row.Enabled || strings.TrimSpace(row.DeletedAt) != "" {
			continue
		}
		if st := strings.ToLower(strings.TrimSpace(row.Status)); st != "" && st != "active" {
			continue
		}
		out = append(out, EffectiveMCPServer{
			ID:         row.ID,
			ServerKey:  strings.TrimSpace(row.Key),
			ConfigJSON: row.ConfigJSON,
		})
	}
	out = FilterEffectiveMCPServers(out, MCPPolicyFromAgentEffectiveTools(eff))
	return out, nil
}
