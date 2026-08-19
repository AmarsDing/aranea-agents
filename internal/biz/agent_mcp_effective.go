package biz

import (
	"context"
	"strings"

	"aranea-agents/internal/biz/tool"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

// Tool key constants are now defined in the tool subpackage; re-exported here for backward compatibility.
const (
	ToolKeyMCPToolSet       = tool.ToolKeyMCPToolSet
	ToolKeyMCPBroker        = tool.ToolKeyMCPBroker
	ToolKeyKnowledgeSearch  = tool.ToolKeyKnowledgeSearch
	ToolKeyKnowledgeReflect = tool.ToolKeyKnowledgeReflect
	ToolKeyKnowledgeWrite   = tool.ToolKeyKnowledgeWrite
	ToolKeyWebResearch      = tool.ToolKeyWebResearch
	ToolKeyKanban           = tool.ToolKeyKanban
	ToolKeyCallAgent        = tool.ToolKeyCallAgent
)

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

// MCP returns the platform MCP usecase (may be nil).
func (t *AgentMCPTooling) MCP() *MCPServerUsecase {
	if t == nil {
		return nil
	}
	return t.mcp
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
	// P2-B 收紧：按调用方 workspace 过滤可见 MCP 服务器（共享 + 自有），
	// 与 ListMCPServers RPC 的租户可见性一致。此前 runtime 层不过滤，
	// 租户 Agent 可挂载其他租户的私有 MCP 服务器（跨租户泄漏）。
	// 系统调用方（cron/prewarm，WithSystemWorkspace）不过滤。
	q := MCPListQuery{}
	if !workspace.IsSystem(ctx) {
		q.WorkspaceID = workspace.IDFromContext(ctx)
	}
	rows, err := t.mcp.List(ctx, q)
	if err != nil {
		return nil, err
	}
	out := filterEnabledMCPServerRows(rows)
	out = FilterEffectiveMCPServers(out, MCPPolicyFromAgentEffectiveTools(eff))
	// C-05: decrypt at-rest secrets before runtime tool assembly.
	for i := range out {
		dec, err := t.mcp.PrepareConfigJSONForRuntime(ctx, out[i].ConfigJSON)
		if err != nil {
			return nil, err
		}
		out[i].ConfigJSON = dec
	}
	return out, nil
}

// AllEnabledServers returns every enabled, non-deleted, active MCP server row
// with secrets decrypted — independent of any single agent's tool policy.
// Used by the startup MCP connection pre-warm, which must see servers even
// before any agent turn resolves them.
func (t *AgentMCPTooling) AllEnabledServers(ctx context.Context) ([]EffectiveMCPServer, error) {
	if t == nil || t.mcp == nil {
		return nil, nil
	}
	rows, err := t.mcp.List(ctx, MCPListQuery{})
	if err != nil {
		return nil, err
	}
	out := filterEnabledMCPServerRows(rows)
	// C-05: decrypt at-rest secrets before runtime tool assembly.
	for i := range out {
		dec, err := t.mcp.PrepareConfigJSONForRuntime(ctx, out[i].ConfigJSON)
		if err != nil {
			return nil, err
		}
		out[i].ConfigJSON = dec
	}
	return out, nil
}

// filterEnabledMCPServerRows drops disabled, soft-deleted, and non-active
// rows. Pure function shared by EffectiveServersForAgent and AllEnabledServers.
func filterEnabledMCPServerRows(rows []MCPServer) []EffectiveMCPServer {
	out := make([]EffectiveMCPServer, 0, len(rows))
	for _, row := range rows {
		if !row.Enabled || strings.TrimSpace(row.DeletedAt) != "" {
			continue
		}
		if st := strings.ToLower(strings.TrimSpace(row.Status)); st != "" && st != string(AgentStatusActive) {
			continue
		}
		out = append(out, EffectiveMCPServer{
			ID:         row.ID,
			ServerKey:  strings.TrimSpace(row.Key),
			ConfigJSON: row.ConfigJSON,
		})
	}
	return out
}

// agentReferencesMCPServer reports whether an agent's effective tool policy
// exposes the given MCP server key via mcp_tool_set / mcp_broker — the exact
// inverse of FilterEffectiveMCPServers: deny wins; an empty mcp: allow list
// means "all visible servers" and therefore references every server. Key
// comparison is case-insensitive (same as FilterEffectiveMCPServers).
func agentReferencesMCPServer(eff AgentEffectiveTools, serverKey string) bool {
	key := strings.ToLower(strings.TrimSpace(serverKey))
	if key == "" || !effectiveToolsAllowsMCPServers(eff) {
		return false
	}
	pol := MCPPolicyFromAgentEffectiveTools(eff)
	for _, d := range pol.DenyServerKeys {
		if d == key {
			return false
		}
	}
	if len(pol.AllowServerKeys) == 0 {
		return true
	}
	for _, a := range pol.AllowServerKeys {
		if a == key {
			return true
		}
	}
	return false
}

// AgentIDsReferencingMCPServer returns the IDs of agents whose effective tool
// policy references the given MCP server key. P0-3: the MCP health runner
// calls this on the DOWN→UP recovery edge to invalidate stale cached builds.
//
// System-caller view: pages through ALL agents (cross-workspace). This is a
// low-frequency path (server recovery edge only), so the per-agent effective
// tools computation is acceptable. Approximation note: a tenant-private
// server of workspace A may match a workspace-B agent with an empty mcp:
// allow list; the false positive costs one extra rebuild and is safe.
func (u *AgentUsecase) AgentIDsReferencingMCPServer(ctx context.Context, serverKey string) ([]string, error) {
	if u == nil || u.reader == nil {
		return nil, nil
	}
	key := strings.ToLower(strings.TrimSpace(serverKey))
	if key == "" {
		return nil, nil
	}
	const pageSize = 100
	var out []string
	for offset := 0; ; offset += pageSize {
		page, err := u.reader.SearchAgents(ctx, AgentListQuery{Limit: pageSize, Offset: offset})
		if err != nil {
			return nil, err
		}
		for _, ag := range page.Items {
			eff, err := u.GetEffectiveTools(ctx, ag.ID)
			if err != nil {
				// One broken agent must not sink the whole reverse lookup.
				if u.lg != nil {
					u.lg.Warn("MCP 恢复反查跳过异常 agent", loggateway.StepID("agent.mcp_reverse_lookup"), loggateway.Str("agent_id", ag.ID), loggateway.Err(err))
				}
				continue
			}
			if agentReferencesMCPServer(eff, key) {
				out = append(out, ag.ID)
			}
		}
		if len(page.Items) < pageSize || offset+len(page.Items) >= page.Total {
			break
		}
	}
	return out, nil
}

// MCPUsageSummary is the aggregate MCP-adoption stat shown on the admin MCP page.
type MCPUsageSummary struct {
	// EnabledAgentCount 是有效工具策略开启了 MCP 门禁（mcp_tool_set / mcp_broker）的 agent 数。
	EnabledAgentCount int
	// TotalAgentCount 是全部 agent 数（任意状态），供前端展示「N / M」上下文。
	TotalAgentCount int
}

// GetMCPUsageSummary 以批量方式计算 MCP 采纳统计：
// 一次 ListAgentRuntimeSettings + 一次 catalog SearchTools + 一次 agent 计数，
// 然后对每条 settings 行做内存门禁评估。与 AgentIDsReferencingMCPServer
// （per-agent GetEffectiveTools，N+1）不同，本方法对高频的 MCP 列表页加载安全。
//
// 正确性说明：无 agent_runtime_settings 行的 agent 回退 DefaultAgentRuntimeSettings
// （profile "coding"），不含 mcp_tool_set / mcp_broker，门禁关闭——因此仅迭代已存
// settings 行即完备（profile 非默认值的 agent 必有 settings 行）。per-agent 工具覆盖
// （ListToolAgentOverridesByAgent）有意不应用：那是 per-agent 查询，会重引入 N+1；
// 针对 mcp 门禁的 override 属边缘场景，本统计是汇总指标，可接受。
func (u *AgentUsecase) GetMCPUsageSummary(ctx context.Context) (MCPUsageSummary, error) {
	var out MCPUsageSummary
	if u == nil || u.reader == nil || u.settings == nil || u.tools == nil {
		return out, nil
	}
	agents, err := u.reader.SearchAgents(ctx, AgentListQuery{Limit: 1, Offset: 0})
	if err != nil {
		return out, err
	}
	out.TotalAgentCount = agents.Total

	settingsMap, err := u.settings.ListAgentRuntimeSettings(ctx)
	if err != nil {
		return out, err
	}
	catalog, err := u.tools.SearchTools(ctx, ToolListQuery{Limit: searchToolsAllLimit, Offset: 0})
	if err != nil {
		return out, err
	}
	for _, s := range settingsMap {
		eff := buildAgentEffectiveTools(withSettingDefaults(s), catalog.Items, u.lg)
		if effectiveToolsAllowsMCPServers(eff) {
			out.EnabledAgentCount++
		}
	}
	return out, nil
}
