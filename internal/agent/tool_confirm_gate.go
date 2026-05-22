package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
)

type toolConfirmGate struct {
	catalog   map[string]bool
	plugin    plugintrpc.ConfirmationGuardConfig
	hasPlugin bool
}

func buildToolConfirmGate(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) *toolConfirmGate {
	catalog := buildCatalogConfirmTools(ctx, ag, deps)
	var pluginCfg plugintrpc.ConfirmationGuardConfig
	hasPlugin := false
	if deps.PluginManager != nil {
		if cfg, ok := deps.PluginManager.ConfirmationGuardConfigForAgent(ag.ID); ok {
			pluginCfg = cfg
			hasPlugin = true
		}
	}
	if len(catalog) == 0 && !hasPlugin {
		return nil
	}
	if hasPlugin && len(pluginCfg.ConfirmTools) == 0 && len(pluginCfg.ConfirmPatterns) == 0 && len(catalog) == 0 {
		return nil
	}
	return &toolConfirmGate{catalog: catalog, plugin: pluginCfg, hasPlugin: hasPlugin}
}

func buildCatalogConfirmTools(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) map[string]bool {
	if deps.ToolUC == nil || strings.TrimSpace(ag.ID) == "" {
		return nil
	}
	eff := loadEffectiveToolKeys(ctx, deps, ag.ID)
	if len(eff) == 0 {
		return nil
	}
	overrides, err := deps.ToolUC.ListToolAgentOverridesByAgent(ctx, ag.ID)
	if err != nil {
		overrides = nil
	}
	overrideByKey := make(map[string]biz.ToolAgentOverride, len(overrides))
	for _, o := range overrides {
		overrideByKey[strings.TrimSpace(o.ToolKey)] = o
	}
	out := make(map[string]bool)
	for key, enabled := range eff {
		if !enabled {
			continue
		}
		tool, err := deps.ToolUC.GetTool(ctx, key)
		if err != nil {
			continue
		}
		ov, hasOV := overrideByKey[key]
		if biz.ToolRequiresConfirmation(tool, ov, hasOV) {
			out[key] = true
			for _, alias := range runtimeConfirmAliases(key) {
				out[alias] = true
			}
		}
	}
	return out
}

// runtimeConfirmAliases maps catalog tool_key to mounted runtime declaration names.
func runtimeConfirmAliases(catalogKey string) []string {
	switch strings.TrimSpace(catalogKey) {
	case "shell_exec":
		return []string{"exec_command"}
	default:
		return nil
	}
}

func catalogRequiresConfirm(catalog map[string]bool, toolName string) bool {
	if catalog == nil {
		return false
	}
	toolName = strings.TrimSpace(toolName)
	if catalog[toolName] {
		return true
	}
	if toolName == "exec_command" && catalog["shell_exec"] {
		return true
	}
	return false
}

func (g *toolConfirmGate) needsConfirm(toolName string, args []byte) bool {
	if g == nil {
		return false
	}
	toolName = strings.TrimSpace(toolName)
	if catalogRequiresConfirm(g.catalog, toolName) {
		return true
	}
	if g.hasPlugin && plugintrpc.MatchConfirmationGuard(g.plugin, toolName, args) {
		return true
	}
	return false
}

func (g *toolConfirmGate) pluginAllowWithoutChannel(toolName string, args []byte) bool {
	if g == nil || !g.hasPlugin || !plugintrpc.ConfirmationDefaultAllow(g.plugin) {
		return false
	}
	if g.catalog != nil && catalogRequiresConfirm(g.catalog, strings.TrimSpace(toolName)) {
		return false
	}
	return plugintrpc.MatchConfirmationGuard(g.plugin, toolName, args)
}

// confirmationMap returns tool keys to annotate on declarations (static keys only).
func (g *toolConfirmGate) confirmationMap() map[string]bool {
	if g == nil {
		return nil
	}
	out := make(map[string]bool)
	for k, v := range g.catalog {
		if v {
			out[k] = true
		}
	}
	if g.hasPlugin {
		for _, key := range g.plugin.ConfirmTools {
			key = strings.TrimSpace(key)
			if key != "" {
				out[key] = true
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func toolConfirmApproved(reply string) bool {
	if approved, structured := serviceawaitreply.ParseToolConfirmReply(reply); structured {
		return approved
	}
	reply = strings.TrimSpace(strings.ToLower(reply))
	switch reply {
	case "y", "yes", "approve", "approved", "allow", "ok", "true", "确认", "同意", "允许", "通过":
		return true
	default:
		return false
	}
}
