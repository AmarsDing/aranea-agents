package biz

import (
	"context"
	"strings"
)

// 目录页的「多少 Agent 能用」：一次读出全部 Agent 的工具策略（Agent + 运行设置 + 工具目录各一次查询），
// 在内存里逐个判定，避免按 Agent 或按条目反复查库。判定规则与单个工具的 GetToolAgentBindings、
// 单个 MCP 服务器的 AgentIDsReferencingMCPServer、运行时技能可见性过滤保持一致（单测对照）。

// agentPolicySnap 是一个 Agent 的工具策略快照。
type agentPolicySnap struct {
	agent    Agent
	settings AgentRuntimeSettings
	profile  string
	allowed  map[string]bool
	denied   map[string]bool
	allowRaw []string
	denyRaw  []string
}

// policySnapshots 读出可见范围内全部 Agent 的策略快照与完整工具目录。
func (u *AgentUsecase) policySnapshots(ctx context.Context, callerWS string) ([]agentPolicySnap, []Tool, error) {
	agents, err := u.listAllAgentsForBindings(ctx, callerWS)
	if err != nil || len(agents) == 0 {
		return nil, nil, err
	}
	settingsByAgent, err := u.settings.ListAgentRuntimeSettings(ctx)
	if err != nil {
		return nil, nil, err
	}
	// 完整目录：profile 分组（group:filesystem 等）要按它展开，管理员禁用也作用于每一行
	catalog, err := u.tools.SearchTools(ctx, ToolListQuery{Limit: searchToolsAllLimit, Offset: 0})
	if err != nil {
		return nil, nil, err
	}
	out := make([]agentPolicySnap, 0, len(agents))
	for i := range agents {
		ag := agents[i]
		settings, ok := settingsByAgent[ag.ID]
		if !ok {
			settings = DefaultAgentRuntimeSettings()
			settings.AgentID = ag.ID
		}
		settings = withSettingDefaults(settings)
		allow := jsonStringList(settings.ToolsAllowJSON, u.lg)
		deny := jsonStringList(settings.ToolsDenyJSON, u.lg)
		prof := strings.TrimSpace(settings.ToolsProfile)
		allowedSet := computePolicyAllowedSet(prof, allow, catalog.Items)
		denySet := computePolicyDenySet(deny, catalog.Items)
		applyRegistryAdminDenials(catalog.Items, denySet)
		out = append(out, agentPolicySnap{
			agent: ag, settings: settings, profile: prof,
			allowed: allowedSet, denied: denySet, allowRaw: allow, denyRaw: deny,
		})
	}
	return out, catalog.Items, nil
}

// toolStateFor 判定某个工具对某个 Agent 是否生效（含单 Agent 覆盖与 web_research 就绪闸）。
func (u *AgentUsecase) toolStateFor(s agentPolicySnap, tool Tool, ov *ToolAgentOverride, platform *WebResearchSetting) (EffectiveAgentTool, string) {
	state, reason, enabled := computeEffectiveToolState(s.settings, tool, s.profile, s.allowed, s.denied)
	item := EffectiveAgentTool{ToolKey: tool.Key, Enabled: enabled, EffectiveState: state, Reason: reason}
	overrideMode := ""
	if ov != nil {
		overrideMode = strings.ToLower(strings.TrimSpace(ov.Mode))
		applyOverrideToEffectiveItem(&item, *ov, s.settings.ToolsEnabled)
	}
	// Web research readiness gate: mirror applyWebResearchEffectiveGate (allowed → denied when API key is missing).
	if tool.Key == ToolKeyWebResearch && item.EffectiveState == "allowed" && u.webResearchChecker != nil && platform != nil {
		// fallback 垫底、用户配置优先（BUG-2：此前参数顺序颠倒，默认值覆盖用户配置）。
		cfgMap := MergeToolConfigMaps(tool.DefaultConfigJSON, tool.ConfigJSON)
		if ov != nil && strings.TrimSpace(ov.ConfigOverrideJSON) != "" {
			MergeJSONMapInto(cfgMap, ov.ConfigOverrideJSON)
		}
		if !u.webResearchChecker.ResolveReady(cfgMap, webResearchPlatformFieldsPtr(platform)) {
			item.Enabled = false
			item.EffectiveState = "denied"
			item.Reason = "missing_api_key"
		}
	}
	return item, overrideMode
}

// overridesByAgent 读一个工具的单 Agent 覆盖；目录行上覆盖数为 0 的工具不查库。
func (u *AgentUsecase) overridesByAgent(ctx context.Context, tool Tool) (map[string]ToolAgentOverride, error) {
	out := map[string]ToolAgentOverride{}
	if tool.AgentOverrideCount == 0 {
		return out, nil
	}
	rows, err := u.tools.ListToolAgentOverrides(ctx, tool.Key)
	if err != nil {
		return nil, err
	}
	for _, o := range rows {
		out[strings.TrimSpace(o.AgentID)] = o
	}
	return out, nil
}

// CountAgentsAllowedByTool 返回每个工具 key 对应「当前能用它的 Agent 数」。
func (u *AgentUsecase) CountAgentsAllowedByTool(ctx context.Context, toolKeys []string, callerWS string) (map[string]int, error) {
	out := make(map[string]int, len(toolKeys))
	if len(toolKeys) == 0 {
		return out, nil
	}
	snaps, catalog, err := u.policySnapshots(ctx, callerWS)
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, k := range toolKeys {
		want[k] = true
		out[k] = 0
	}
	platform := loadWebResearchPlatformFromSys(ctx, u.sys, u.lg)
	for _, tool := range catalog {
		if !want[tool.Key] {
			continue
		}
		ovs, err := u.overridesByAgent(ctx, tool)
		if err != nil {
			return nil, err
		}
		n := 0
		for _, s := range snaps {
			var ov *ToolAgentOverride
			if o, ok := ovs[s.agent.ID]; ok {
				ov = &o
			}
			if item, _ := u.toolStateFor(s, tool, ov, platform); item.EffectiveState == "allowed" {
				n++
			}
		}
		out[tool.Key] = n
	}
	return out, nil
}

// CountAgentsByMCPServer 返回每个 MCP 服务器 key 对应「会挂载它的 Agent 数」：
// Agent 开了 mcp_tool_set 或 mcp_broker，且 mcp:<key> 没被拒、（有 mcp: 白名单时）在白名单里。
func (u *AgentUsecase) CountAgentsByMCPServer(ctx context.Context, serverKeys []string, callerWS string) (map[string]int, error) {
	out := make(map[string]int, len(serverKeys))
	if len(serverKeys) == 0 {
		return out, nil
	}
	snaps, catalog, err := u.policySnapshots(ctx, callerWS)
	if err != nil {
		return nil, err
	}
	var gates []Tool
	for _, tool := range catalog {
		if tool.Key == ToolKeyMCPToolSet || tool.Key == ToolKeyMCPBroker {
			gates = append(gates, tool)
		}
	}
	gateOverrides := make([]map[string]ToolAgentOverride, len(gates))
	for i, g := range gates {
		if gateOverrides[i], err = u.overridesByAgent(ctx, g); err != nil {
			return nil, err
		}
	}
	for _, k := range serverKeys {
		out[k] = 0
	}
	for _, s := range snaps {
		if !s.settings.ToolsEnabled || !snapEnablesAny(u, s, gates, gateOverrides) {
			continue
		}
		pol := EffectiveMCPPolicy{
			AllowServerKeys: MCPServerKeysFromPolicyEntries(s.allowRaw),
			DenyServerKeys:  MCPServerKeysFromPolicyEntries(s.denyRaw),
		}
		for _, k := range serverKeys {
			if mcpPolicyAdmits(pol, k) {
				out[k]++
			}
		}
	}
	return out, nil
}

func snapEnablesAny(u *AgentUsecase, s agentPolicySnap, gates []Tool, overrides []map[string]ToolAgentOverride) bool {
	for i, g := range gates {
		var ov *ToolAgentOverride
		if o, ok := overrides[i][s.agent.ID]; ok {
			ov = &o
		}
		if item, _ := u.toolStateFor(s, g, ov, nil); item.Enabled {
			return true
		}
	}
	return false
}

// mcpPolicyAdmits 与 agentReferencesMCPServer 同一判定：拒绝优先；有白名单时必须在白名单里。
func mcpPolicyAdmits(pol EffectiveMCPPolicy, serverKey string) bool {
	key := strings.ToLower(strings.TrimSpace(serverKey))
	if key == "" {
		return false
	}
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

// CountAgentsSeeingSkill 返回每个技能 slug 对应「能看见它的 Agent 数」，规则同运行时
// skillruntime.NewAgentVisibilityFilter：被拒的看不见；配了白名单时只看得见白名单里的。
func (u *AgentUsecase) CountAgentsSeeingSkill(ctx context.Context, slugs []string, callerWS string) (map[string]int, error) {
	out := make(map[string]int, len(slugs))
	if len(slugs) == 0 {
		return out, nil
	}
	agents, err := u.listAllAgentsForBindings(ctx, callerWS)
	if err != nil {
		return nil, err
	}
	settingsByAgent, err := u.settings.ListAgentRuntimeSettings(ctx)
	if err != nil {
		return nil, err
	}
	for _, slug := range slugs {
		out[slug] = 0
	}
	for _, ag := range agents {
		policy := ParseSkillRuntimePolicy(settingsByAgent[ag.ID].SkillRuntimeJSON)
		allowed := toSet(policy.AllowedSlugs)
		denied := toSet(policy.DeniedSlugs)
		for _, slug := range slugs {
			key := strings.ToLower(strings.TrimSpace(slug))
			if key == "" || denied[key] || (len(allowed) > 0 && !allowed[key]) {
				continue
			}
			out[slug]++
		}
	}
	return out, nil
}

func toSet(in []string) map[string]bool {
	out := make(map[string]bool, len(in))
	for _, s := range in {
		out[s] = true
	}
	return out
}
