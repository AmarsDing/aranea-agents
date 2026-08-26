package configgraph

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/skill"
)

// effectiveToolsConcurrency 是 design §4.1 step 4 规定的 GetEffectiveTools
// 并发度（单 agent 失败记 broken 继续）。
const effectiveToolsConcurrency = 8

// agentExtractor: agents 节点 + bound_position（position_id 直读列）+
// bound_position_key（position_key → organizations.org_key 双解，目标缺失即
// broken，ORPHAN 锚点）+ granted_tool / enables_mcp（GetEffectiveTools 净集）
// + allows_skill（skill_runtime_json.allowed_slugs）+ tool_override
// （tool_agent_overrides deleted_at=” 直读表）。
type agentExtractor struct {
	provider EffectiveToolsProvider // nil = 跳过 granted_tool/enables_mcp（仅测试）
}

func (agentExtractor) NodeType() string { return NodeTypeAgent }

func (agentExtractor) ExtractNodes(ctx context.Context, src SourceRepo) ([]Node, error) {
	rows, err := src.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	nodes := make([]Node, 0, len(rows))
	for _, r := range rows {
		if r.ID == "" {
			continue
		}
		nodes = append(nodes, newNode(NodeTypeAgent, r.ID, r.AgentKey, r.DisplayName, r.WorkspaceID,
			statusFromDeletedAt(r.DeletedAt),
			map[string]any{
				"kind":          r.Kind,
				"agent_variant": r.AgentVariant,
				"status":        r.Status,
			}))
	}
	return nodes, nil
}

func (x agentExtractor) ExtractEdges(ctx context.Context, src SourceRepo) ([]Edge, error) {
	agents, err := src.ListAgents(ctx)
	if err != nil {
		return nil, err
	}
	policies, err := src.ListAgentToolPolicies(ctx)
	if err != nil {
		return nil, err
	}
	policyByAgent := make(map[string]AgentToolPolicyRow, len(policies))
	for _, p := range policies {
		if p.AgentID != "" {
			policyByAgent[p.AgentID] = p
		}
	}

	var active []AgentRow
	for _, r := range agents {
		if r.ID != "" && statusFromDeletedAt(r.DeletedAt) != NodeStatusDeleted {
			active = append(active, r)
		}
	}

	var edges []Edge
	for _, r := range active {
		if pos := strings.TrimSpace(r.PositionID); pos != "" {
			edges = append(edges, Edge{
				SrcType: NodeTypeAgent, SrcRef: r.ID,
				DstType: NodeTypeOrganization, DstRef: pos,
				Type:        EdgeTypeBoundPosition,
				Evidence:    evidence("agents", "position_id", "position_id"),
				WorkspaceID: r.WorkspaceID,
			})
		}
		if posKey := strings.TrimSpace(r.PositionKey); posKey != "" {
			edges = append(edges, Edge{
				SrcType: NodeTypeAgent, SrcRef: r.ID,
				DstType: NodeTypeOrganization, DstKey: posKey,
				Type:        EdgeTypeBoundPositionKey,
				Evidence:    evidence("agents", "position_key", "position_key"),
				WorkspaceID: r.WorkspaceID,
			})
		}
		if pol, ok := policyByAgent[r.ID]; ok {
			rp := skill.ParseRuntimePolicy(pol.SkillRuntimeJSON)
			for _, slug := range rp.AllowedSlugs {
				slug = strings.TrimSpace(slug)
				if slug == "" {
					continue
				}
				edges = append(edges, Edge{
					SrcType: NodeTypeAgent, SrcRef: r.ID,
					DstType: NodeTypeSkill, DstRef: slug, DstKey: slug,
					Type:        EdgeTypeAllowsSkill,
					Evidence:    evidence("agent_runtime_settings", "skill_runtime_json", "skill_runtime_json.allowed_slugs"),
					WorkspaceID: r.WorkspaceID,
				})
			}
		}
	}

	if x.provider != nil {
		edges = append(edges, x.effectiveToolEdges(ctx, active)...)
	}

	overrides, err := src.ListToolOverrides(ctx)
	if err != nil {
		return nil, err
	}
	for _, o := range overrides {
		if o.AgentID == "" || (o.ToolID == "" && o.ToolKey == "") {
			continue
		}
		edges = append(edges, Edge{
			SrcType: NodeTypeAgent, SrcRef: o.AgentID,
			DstType: NodeTypeTool, DstRef: o.ToolID, DstKey: o.ToolKey,
			Type:     EdgeTypeToolOverride,
			Evidence: withExtra(evidence("tool_agent_overrides", "mode", "mode"), "mode", o.Mode),
		})
	}
	return edges, nil
}

// effectiveToolEdges 并发拉取各 agent 的 GetEffectiveTools 净集（design §4.1
// 并发 8），结果按 agent 顺序合并保证重建产出确定。
func (x agentExtractor) effectiveToolEdges(ctx context.Context, active []AgentRow) []Edge {
	perAgent := make([][]Edge, len(active))
	var wg sync.WaitGroup
	sem := make(chan struct{}, effectiveToolsConcurrency)
	for i, ag := range active {
		wg.Add(1)
		go func(i int, ag AgentRow) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			perAgent[i] = x.effectiveEdgesFor(ctx, ag)
		}(i, ag)
	}
	wg.Wait()
	var out []Edge
	for _, list := range perAgent {
		out = append(out, list...)
	}
	return out
}

// effectiveEdgesFor 产出单 agent 的 granted_tool + enables_mcp 边。
func (x agentExtractor) effectiveEdgesFor(ctx context.Context, ag AgentRow) []Edge {
	eff, err := x.provider.GetEffectiveTools(ctx, ag.ID)
	if err != nil {
		return []Edge{extractErrorEdge(NodeTypeAgent, ag.ID, EdgeTypeGrantedTool, ag.WorkspaceID,
			"agents", "id", "GetEffectiveTools", err)}
	}
	var edges []Edge
	for _, it := range eff.Items {
		if !it.Enabled || it.EffectiveState != "allowed" {
			continue
		}
		key := strings.TrimSpace(it.ToolKey)
		if key == "" {
			continue
		}
		origin := strings.TrimSpace(it.Origin)
		if origin == "" {
			origin = GrantOriginAllow // 防御：allowed 行必带 origin
		}
		ev := evidence("agent_runtime_settings", "tools", "GetEffectiveTools")
		ev[EvidenceKeyGrantOrigin] = origin
		edges = append(edges, Edge{
			SrcType: NodeTypeAgent, SrcRef: ag.ID,
			DstType: NodeTypeTool, DstRef: key, DstKey: key,
			Type:        EdgeTypeGrantedTool,
			Evidence:    ev,
			WorkspaceID: ag.WorkspaceID,
		})
	}
	// enables_mcp 与运行时同门径：mcp_tool_set/mcp_broker 开启时 allow 里的
	// mcp:<server_key> 条目才生效（deny 条目是排除而非引用，不成边）。
	if mcpGateOpen(eff) {
		for _, sk := range biz.MCPServerKeysFromPolicyEntries(eff.Allow) {
			edges = append(edges, Edge{
				SrcType: NodeTypeAgent, SrcRef: ag.ID,
				DstType: NodeTypeMCPServer, DstRef: sk, DstKey: sk,
				Type:        EdgeTypeEnablesMCP,
				Evidence:    withExtra(evidence("agent_runtime_settings", "tools_allow_json", "tools_allow_json"), "policy", "allow"),
				WorkspaceID: ag.WorkspaceID,
			})
		}
	}
	return edges
}

// mcpGateOpen 复刻 biz.effectiveToolsAllowsMCPServers 门径（未导出，同构）。
func mcpGateOpen(eff biz.AgentEffectiveTools) bool {
	if !eff.ToolsEnabled {
		return false
	}
	for _, it := range eff.Items {
		if !it.Enabled || it.EffectiveState != "allowed" {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(it.ToolKey))
		if k == strings.ToLower(biz.ToolKeyMCPToolSet) || k == strings.ToLower(biz.ToolKeyMCPBroker) {
			return true
		}
	}
	return false
}
