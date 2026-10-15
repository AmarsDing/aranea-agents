package biz

import (
	"aranea-agents/pkg/loggateway"
	"strings"
)

func buildAgentEffectiveTools(settings AgentRuntimeSettings, catalog []Tool, lg loggateway.Logger) AgentEffectiveTools {
	allow := jsonStringList(settings.ToolsAllowJSON, lg)
	deny := jsonStringList(settings.ToolsDenyJSON, lg)

	prof := strings.TrimSpace(settings.ToolsProfile)
	allowedSet := computePolicyAllowedSet(prof, allow, catalog)
	denySet := computePolicyDenySet(deny, catalog)
	applyRegistryAdminDenials(catalog, denySet)
	applySpiritReservedDenials(prof, allowedSet, denySet)

	profileSet := profileAllowSet(prof, catalog)
	PropagateAllowAliases(profileSet)

	registryKeys := make(map[string]bool, len(catalog))
	for _, tool := range catalog {
		registryKeys[tool.Key] = true
	}

	items := make([]EffectiveAgentTool, 0, len(catalog)+3)
	for _, tool := range catalog {
		st, rsn, en := computeEffectiveToolState(settings, tool, prof, allowedSet, denySet)
		items = append(items, EffectiveAgentTool{
			ToolKey:        tool.Key,
			DisplayName:    tool.DisplayName,
			Category:       tool.Category,
			Source:         tool.Source,
			Enabled:        en,
			EffectiveState: st,
			Reason:         rsn,
			Origin:         grantOrigin(st, tool.Key, profileSet),
		})
	}

	const shellExecKey = "shell_exec"
	if !registryKeys[shellExecKey] && allowedSet[shellExecKey] {
		syn := syntheticShellExecTool()
		st, rsn, en := computeEffectiveToolState(settings, syn, prof, allowedSet, denySet)
		items = append(items, EffectiveAgentTool{
			ToolKey:        shellExecKey,
			DisplayName:    syn.DisplayName,
			Category:       syn.Category,
			Source:         syn.Source,
			Enabled:        en,
			EffectiveState: st,
			Reason:         rsn,
			Origin:         grantOrigin(st, shellExecKey, profileSet),
		})
	}

	const webSearchKey = "duckduckgo_search"
	if !registryKeys[webSearchKey] && allowedSet[webSearchKey] {
		syn := syntheticWebSearchTool()
		st, rsn, en := computeEffectiveToolState(settings, syn, prof, allowedSet, denySet)
		items = append(items, EffectiveAgentTool{
			ToolKey:        webSearchKey,
			DisplayName:    syn.DisplayName,
			Category:       syn.Category,
			Source:         syn.Source,
			Enabled:        en,
			EffectiveState: st,
			Reason:         rsn,
			Origin:         grantOrigin(st, webSearchKey, profileSet),
		})
	}
	const webFetchKey = "web_fetch"
	if !registryKeys[webFetchKey] && allowedSet[webFetchKey] {
		syn := syntheticWebFetchTool()
		st, rsn, en := computeEffectiveToolState(settings, syn, prof, allowedSet, denySet)
		items = append(items, EffectiveAgentTool{
			ToolKey:        webFetchKey,
			DisplayName:    syn.DisplayName,
			Category:       syn.Category,
			Source:         syn.Source,
			Enabled:        en,
			EffectiveState: st,
			Reason:         rsn,
			Origin:         grantOrigin(st, webFetchKey, profileSet),
		})
	}

	return AgentEffectiveTools{
		ToolsEnabled: settings.ToolsEnabled,
		Profile:      CanonicalToolProfile(settings.ToolsProfile),
		Allow:        allow,
		Deny:         deny,
		Items:        items,
	}
}
