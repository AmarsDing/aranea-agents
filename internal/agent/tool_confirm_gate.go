package agent

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/alias"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/pkg/loggateway"
)

// confirmCatalogEntry records whether a catalog tool requires confirmation.
type confirmCatalogEntry struct {
	requiresConfirm bool
}

// toolsetRegistryNames returns the set of ToolSet names from the runtime
// tool registry. These names serve as prefixes in mounted tool declaration
// names (e.g., registryName "file" → mounted name "file_save_file").
// Derived dynamically from tools.Registry() so it stays in sync automatically.
func toolsetRegistryNames() map[string]bool {
	names := make(map[string]bool)
	for _, reg := range tools.Registry() {
		if reg.ToolSetFactory != nil {
			names[reg.Name] = true
		}
	}
	return names
}

type toolConfirmGate struct {
	catalog   map[string]confirmCatalogEntry
	plugin    plugintrpc.ConfirmationGuardConfig
	hasPlugin bool
	// sessionGrants holds session-scoped "always allow for this session"
	// grants. It is a process-wide store shared across agent rebuilds;
	// entries are keyed by (sessionID, agentID, toolKey) so a grant never
	// leaks across sessions.
	sessionGrants *toolGrantStore
	// persistedGrant queries the DB-backed "always allow" grant tier.
	// A nil function disables the tier (treated as no grant).
	persistedGrant func(ctx context.Context, agentID, toolKey string) bool
}

// Confirmation decision reasons, recorded in logs for audit (Grok's
// decision_reason counterpart).
const (
	// confirmReasonDefaultAllow: the tool does not require confirmation.
	confirmReasonDefaultAllow = "default_allow"
	// confirmReasonGrantSession: allowed by a session-scoped grant.
	confirmReasonGrantSession = "grant_session"
	// confirmReasonGrantPersisted: allowed by a persisted grant.
	confirmReasonGrantPersisted = "grant_persisted"
	// confirmReasonPolicyCatalog: confirmation required by catalog policy.
	confirmReasonPolicyCatalog = "policy_catalog"
	// confirmReasonPolicyPlugin: confirmation required by plugin guard.
	confirmReasonPolicyPlugin = "policy_plugin"
)

// confirmDecision is the outcome of the confirmation decision chain.
type confirmDecision struct {
	needsConfirm bool
	reason       string
}

// defaultToolGrantStore is the process-wide session-grant store. Session
// grants are lost on process restart (matching Grok's session grant
// semantics); the TTL bounds memory growth for long-running processes.
var defaultToolGrantStore = newToolGrantStore(time.Now)

// decide runs the confirmation decision chain:
//
//	policy (catalog/plugin) → persisted grant → session grant → prompt
//
// Grants are only consulted for tools that actually require confirmation;
// other tools short-circuit as default_allow.
func (g *toolConfirmGate) decide(ctx context.Context, sessionID, agentID, toolName string, args []byte) confirmDecision {
	toolName = strings.TrimSpace(toolName)
	needsByCatalog := catalogRequiresConfirm(g.catalog, toolName)
	needsByPlugin := g.hasPlugin && plugintrpc.MatchConfirmationGuard(g.plugin, toolName, args)
	if !needsByCatalog && !needsByPlugin {
		return confirmDecision{needsConfirm: false, reason: confirmReasonDefaultAllow}
	}
	if g.persistedGrant != nil && g.persistedGrant(ctx, agentID, toolName) {
		return confirmDecision{needsConfirm: false, reason: confirmReasonGrantPersisted}
	}
	if g.sessionGrants != nil && g.sessionGrants.HasSession(sessionID, agentID, toolName) {
		return confirmDecision{needsConfirm: false, reason: confirmReasonGrantSession}
	}
	if needsByCatalog {
		return confirmDecision{needsConfirm: true, reason: confirmReasonPolicyCatalog}
	}
	return confirmDecision{needsConfirm: true, reason: confirmReasonPolicyPlugin}
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
	gate := &toolConfirmGate{
		catalog:       catalog,
		plugin:        pluginCfg,
		hasPlugin:     hasPlugin,
		sessionGrants: defaultToolGrantStore,
	}
	if deps.ToolUC != nil {
		gate.persistedGrant = deps.ToolUC.HasToolGrant
	}
	return gate
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
			}
			continue
		}
		ov, hasOV := overrideByKey[key]
		if biz.ToolRequiresConfirmation(tool, ov, hasOV) {
			out[key] = confirmCatalogEntry{
				requiresConfirm: true,
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
	// Try deriving catalog key from runtime name using ToolSet prefix.
	// Runtime names follow the pattern: <toolsetName>_<catalogKey>
	// e.g., "file_save_file" → toolset="file", catalogKey="save_file"
	// ToolSet names are derived dynamically from tools.Registry().
	for toolsetName := range toolsetRegistryNames() {
		prefix := toolsetName + "_"
		if strings.HasPrefix(toolName, prefix) {
			suffix := strings.TrimPrefix(toolName, prefix)
			if entry, ok := catalog[suffix]; ok {
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
