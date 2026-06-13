package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/alias"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/pkg/loggateway"
)

// confirmCatalogEntry records whether a catalog tool requires confirmation
// and its registry (ToolSet) name for runtime name derivation.
type confirmCatalogEntry struct {
	requiresConfirm bool
	registryName    string // e.g., "file", "hostexec", "mcpbroker"
}

// catalogKeyRegistryNames maps catalog tool keys to their registry (ToolSet) name.
// Only ToolSet-backed tools need an entry; single-tool registrations have no prefix.
// Derived from internal/data/builtin_tools_seed.go registryName field.
var catalogKeyRegistryNames = map[string]string{
	// file ToolSet
	"read_file":           "file",
	"read_multiple_files": "file",
	"save_file":           "file",
	"list_file":           "file",
	"search_file":         "file",
	"search_content":      "file",
	"replace_content":     "file",
	"diff_edit":           "file",
	"patch_file":          "file",
	// hostexec ToolSet
	"shell_exec": "hostexec",
	// email ToolSet
	"send_email": "email",
	// todo
	"todo_write": "todo",
	// await_user_reply
	"await_user_reply": "await_user_reply",
	// claudecode ToolSet
	"claude_code": "claudecode",
	// workspace_exec
	"workspace_exec": "workspace_exec",
	// arxiv_search ToolSet
	"arxiv_search": "arxiv_search",
	// wikipedia ToolSet
	"wikipedia_search": "wikipedia",
	// google_search ToolSet
	"google_search": "google_search",
	// mcpbroker
	"mcp_broker": "mcpbroker",
	// read_document
	"read_document": "read_document",
	// read_spreadsheet
	"read_spreadsheet": "read_spreadsheet",
	// browser ToolSet
	"browser": "browser",
	// model_registry_sync
	"model_registry_sync": "model_registry_sync",
}

type toolConfirmGate struct {
	catalog   map[string]confirmCatalogEntry
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

func buildCatalogConfirmTools(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps) map[string]confirmCatalogEntry {
	if deps.ToolUC == nil || strings.TrimSpace(ag.ID) == "" {
		return nil
	}
	eff := loadEffectiveToolKeys(ctx, deps, ag.ID)
	if len(eff) == 0 {
		return nil
	}
	overrides, err := deps.ToolUC.ListToolAgentOverridesByAgent(ctx, ag.ID)
	if err != nil {
		// Fail-closed: when DB is unavailable, assume all enabled tools require
		// confirmation rather than silently skipping the security gate.
		lg := deps.Logger()
		lg.Warn("tool confirm gate: DB query for overrides failed, using fail-closed policy",
			loggateway.StepID("agent.tool_build"),
			loggateway.Str("agent_id", ag.ID),
			loggateway.Err(err))
		overrides = nil
		// Build a conservative catalog: all enabled tools require confirmation.
		out := make(map[string]confirmCatalogEntry, len(eff))
		for key, enabled := range eff {
			if enabled {
				out[key] = confirmCatalogEntry{
					requiresConfirm: true,
					registryName:    catalogKeyRegistryNames[key],
				}
			}
		}
		return out
	}
	overrideByKey := make(map[string]biz.ToolAgentOverride, len(overrides))
	for _, o := range overrides {
		overrideByKey[strings.TrimSpace(o.ToolKey)] = o
	}
	out := make(map[string]confirmCatalogEntry)
	for key, enabled := range eff {
		if !enabled {
			continue
		}
		tool, err := deps.ToolUC.GetTool(ctx, key)
		if err != nil {
			// Fail-closed: when DB is unavailable for a specific tool,
			// assume it requires confirmation to be safe.
			out[key] = confirmCatalogEntry{
				requiresConfirm: true,
				registryName:    catalogKeyRegistryNames[key],
			}
			continue
		}
		ov, hasOV := overrideByKey[key]
		if biz.ToolRequiresConfirmation(tool, ov, hasOV) {
			out[key] = confirmCatalogEntry{
				requiresConfirm: true,
				registryName:    catalogKeyRegistryNames[key],
			}
		}
	}
	return out
}

func catalogRequiresConfirm(catalog map[string]confirmCatalogEntry, toolName string) bool {
	if catalog == nil {
		return false
	}
	toolName = strings.TrimSpace(toolName)
	// Exact match first.
	if entry, ok := catalog[toolName]; ok {
		return entry.requiresConfirm
	}
	// Try deriving catalog key from runtime name using registry name prefix.
	// Runtime names follow the pattern: <registryName>_<catalogKey>
	// e.g., "file_save_file" → registry="file", catalogKey="save_file"
	for catalogKey, entry := range catalog {
		if entry.registryName == "" {
			continue
		}
		prefix := entry.registryName + "_"
		if strings.HasPrefix(toolName, prefix) {
			suffix := strings.TrimPrefix(toolName, prefix)
			if suffix == catalogKey {
				return entry.requiresConfirm
			}
		}
	}
	// Try reverse alias lookup: check if toolName is a canonical runtime name
	// that maps from a catalog key alias (e.g., "exec_command" ← "shell_exec").
	for aliasKey, canonical := range alias.RuntimeToolNameAliases {
		if canonical == toolName {
			if entry, ok := catalog[aliasKey]; ok {
				return entry.requiresConfirm
			}
		}
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
	for k, entry := range g.catalog {
		if entry.requiresConfirm {
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
