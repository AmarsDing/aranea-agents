package biz

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
)

// ToolAgentBinding is one agent's effective state for a specific tool,
// computed server-side in bulk. It replaces the N+1 pattern where the
// frontend calls GetEffectiveTools once per agent.
type ToolAgentBinding struct {
	AgentID      string
	AgentKey     string
	AgentName    string
	AgentStatus  string
	ToolsEnabled bool
	Profile      string
	State        string // allowed | denied
	Reason       string
	OverrideMode string // allow | deny | inherit when an override row exists, "" otherwise
}

// listAllAgentsForBindings pages through the agent catalog (data layer caps a
// single page at 500) so large deployments are not silently truncated.
func (u *AgentUsecase) listAllAgentsForBindings(ctx context.Context, callerWS string) ([]Agent, error) {
	const pageSize = 500
	out := make([]Agent, 0, pageSize)
	for offset := 0; ; offset += pageSize {
		page, err := u.reader.SearchAgents(ctx, AgentListQuery{WorkspaceID: callerWS, Limit: pageSize, Offset: offset})
		if err != nil {
			return nil, err
		}
		out = append(out, page.Items...)
		if len(page.Items) < pageSize || len(out) >= page.Total {
			return out, nil
		}
	}
}

// GetToolAgentBindings computes the effective state of one tool across every
// visible agent in a constant number of queries (tool + agents + settings +
// overrides + catalog), then resolves states purely in memory.
func (u *AgentUsecase) GetToolAgentBindings(ctx context.Context, toolID, callerWS string) ([]ToolAgentBinding, error) {
	toolID = strings.TrimSpace(toolID)
	if toolID == "" {
		return nil, apierror.BadRequest("TOOL", "tool id is required")
	}
	tool, err := u.tools.GetTool(ctx, toolID)
	if err != nil {
		return nil, err
	}
	agents, err := u.listAllAgentsForBindings(ctx, callerWS)
	if err != nil {
		return nil, err
	}
	if len(agents) == 0 {
		return []ToolAgentBinding{}, nil
	}
	settingsByAgent, err := u.settings.ListAgentRuntimeSettings(ctx)
	if err != nil {
		return nil, err
	}
	overrides, err := u.tools.ListToolAgentOverrides(ctx, tool.Key)
	if err != nil {
		return nil, err
	}
	overrideByAgent := make(map[string]ToolAgentOverride, len(overrides))
	for _, o := range overrides {
		overrideByAgent[strings.TrimSpace(o.AgentID)] = o
	}
	// Full catalog is required because profile groups (group:filesystem, ...)
	// expand against it and admin denials apply to every catalog row.
	catalog, err := u.tools.SearchTools(ctx, ToolListQuery{Limit: searchToolsAllLimit, Offset: 0})
	if err != nil {
		return nil, err
	}
	platform := loadWebResearchPlatformFromSys(ctx, u.sys)

	out := make([]ToolAgentBinding, 0, len(agents))
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

		state, reason, enabled := computeEffectiveToolState(settings, tool, prof, allowedSet, denySet)
		item := EffectiveAgentTool{ToolKey: tool.Key, Enabled: enabled, EffectiveState: state, Reason: reason}

		overrideMode := ""
		if ov, ok := overrideByAgent[ag.ID]; ok {
			overrideMode = strings.ToLower(strings.TrimSpace(ov.Mode))
			applyOverrideToEffectiveItem(&item, ov, settings.ToolsEnabled)
		}
		// Web research readiness gate: mirror applyWebResearchEffectiveGate for
		// the single-tool case (allowed → denied when API key is missing).
		if tool.Key == ToolKeyWebResearch && item.EffectiveState == "allowed" && u.webResearchChecker != nil && platform != nil {
			// fallback 垫底、用户配置优先（BUG-2：此前参数顺序颠倒，默认值覆盖用户配置）。
			cfgMap := MergeToolConfigMaps(tool.DefaultConfigJSON, tool.ConfigJSON)
			if ov, ok := overrideByAgent[ag.ID]; ok && strings.TrimSpace(ov.ConfigOverrideJSON) != "" {
				MergeJSONMapInto(cfgMap, ov.ConfigOverrideJSON)
			}
			if !u.webResearchChecker.ResolveReady(cfgMap, webResearchPlatformFieldsPtr(platform)) {
				item.Enabled = false
				item.EffectiveState = "denied"
				item.Reason = "missing_api_key"
			}
		}

		out = append(out, ToolAgentBinding{
			AgentID:      ag.ID,
			AgentKey:     ag.AgentKey,
			AgentName:    ag.DisplayName,
			AgentStatus:  ag.Status,
			ToolsEnabled: settings.ToolsEnabled,
			Profile:      canonicalToolProfile(settings.ToolsProfile),
			State:        item.EffectiveState,
			Reason:       item.Reason,
			OverrideMode: overrideMode,
		})
	}
	return out, nil
}
