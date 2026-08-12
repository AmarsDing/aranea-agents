package agent

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizcu "aranea-agents/internal/biz/computeruse"
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
	lg        loggateway.Logger
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
	// confirmReasonPolicyDanger: confirmation forced by computer-use
	// danger-word content inspection (75 A5). Grants never bypass it —
	// a danger-word target prompts every single time.
	confirmReasonPolicyDanger = "policy_danger"
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
	// Computer-use danger-word floor (75 A5): a danger hit on act/launch
	// forces per-invocation confirmation regardless of catalog or grants.
	if computerUseDangerHit(toolName, args) {
		return confirmDecision{needsConfirm: true, reason: confirmReasonPolicyDanger}
	}
	needsByCatalog := g.catalogCheck(toolName)
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

// buildToolConfirmGate assembles the confirmation gate from the pre-loaded
// catalog snapshot (nil ⇒ no catalog policy) and the plugin guard. Loading
// happens once per build in BuildTRPCLLMAgent (loadToolBuildCatalog).
func buildToolConfirmGate(ctx context.Context, ag biz.Agent, deps TRPCBuilderDeps, catalog map[string]confirmCatalogEntry) *toolConfirmGate {
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
		lg:            deps.Logger(),
		sessionGrants: defaultToolGrantStore,
	}
	if deps.ToolUC != nil {
		gate.persistedGrant = deps.ToolUC.HasToolGrant
	}
	return gate
}

// Catalog match strategies, recorded in logs to explain gating decisions.
const (
	catalogMatchExact         = "exact"
	catalogMatchToolsetPrefix = "toolset_prefix"
	catalogMatchToolsetAlias  = "toolset_prefix_alias"
	catalogMatchReverseAlias  = "reverse_alias"
	catalogMatchSegment       = "segment"
)

// catalogConfirmMatch describes how the catalog resolved a runtime tool name.
type catalogConfirmMatch struct {
	requiresConfirm bool
	// via is one of the catalogMatch* strategies; "" when nothing matched.
	via string
	// catalogKey is the catalog key that decided the outcome.
	catalogKey string
}

// catalogRequiresConfirm reports whether the catalog gates toolName. Kept as
// a thin wrapper for unit tests; production call sites use
// (*toolConfirmGate).catalogCheck which logs indirect resolutions.
func catalogRequiresConfirm(catalog map[string]confirmCatalogEntry, toolName string) bool {
	return lookupCatalogConfirm(catalog, toolName).requiresConfirm
}

func lookupCatalogConfirm(catalog map[string]confirmCatalogEntry, toolName string) catalogConfirmMatch {
	if catalog == nil {
		return catalogConfirmMatch{}
	}
	toolName = strings.TrimSpace(toolName)
	// Exact match first.
	if entry, ok := catalog[toolName]; ok {
		return catalogConfirmMatch{requiresConfirm: entry.requiresConfirm, via: catalogMatchExact, catalogKey: toolName}
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
				return catalogConfirmMatch{requiresConfirm: entry.requiresConfirm, via: catalogMatchToolsetPrefix, catalogKey: suffix}
			}
			// The suffix may itself be a canonical runtime name whose catalog
			// key is an alias source (e.g. "hostexec_exec_command" → suffix
			// "exec_command" ← catalog key "shell_exec"). Resolve via reverse
			// alias lookup so mounted toolset names inherit the catalog policy;
			// an absent catalog entry (e.g. admin disabled confirmation) stays
			// ungated, preserving per-agent overrides.
			for aliasKey, canonical := range alias.RuntimeToolNameAliases {
				if canonical == suffix {
					if entry, ok := catalog[aliasKey]; ok {
						return catalogConfirmMatch{requiresConfirm: entry.requiresConfirm, via: catalogMatchToolsetAlias, catalogKey: aliasKey}
					}
				}
			}
		}
	}
	// Try reverse alias lookup: check if toolName is a canonical runtime name
	// that maps from a catalog key alias (e.g., "exec_command" ← "shell_exec").
	for aliasKey, canonical := range alias.RuntimeToolNameAliases {
		if canonical == toolName {
			if entry, ok := catalog[aliasKey]; ok {
				return catalogConfirmMatch{requiresConfirm: entry.requiresConfirm, via: catalogMatchReverseAlias, catalogKey: aliasKey}
			}
		}
	}
	// Try segment match: MCP-mounted toolsets expose sub-tools whose runtime
	// names derive from the catalog key (catalog "browser" → "browser_navigate",
	// optionally with an MCP ToolPrefix → "playwright_browser_navigate"). Match
	// when the catalog key appears as a whole underscore-delimited segment of
	// the runtime name. Consistent with the suffix-based matching used by
	// internal/tools/decorator.go and browser/guarded_toolset.go for the same
	// MCP ToolPrefix problem. Over-matching only causes extra confirmation
	// prompts (fail-safe direction for a security gate).
	for key, entry := range catalog {
		if !entry.requiresConfirm {
			continue
		}
		if strings.HasPrefix(toolName, key+"_") ||
			strings.HasSuffix(toolName, "_"+key) ||
			strings.Contains(toolName, "_"+key+"_") {
			return catalogConfirmMatch{requiresConfirm: true, via: catalogMatchSegment, catalogKey: key}
		}
	}
	return catalogConfirmMatch{}
}

// logger returns the gate logger, falling back to a noop for zero-value gates
// (unit tests construct gates via struct literals without a logger).
func (g *toolConfirmGate) logger() loggateway.Logger {
	if g != nil && g.lg != nil {
		return g.lg
	}
	return loggateway.NewNoop()
}

// catalogCheck resolves the catalog policy for a runtime tool name. Indirect
// resolutions (toolset prefix / alias / segment) are logged at Info level so
// unexpected gating decisions — e.g. a mounted name like
// "hostexec_exec_command" inheriting the "shell_exec" policy — can be traced
// without a debugger. Exact matches are the common path and stay silent.
func (g *toolConfirmGate) catalogCheck(toolName string) bool {
	m := lookupCatalogConfirm(g.catalog, toolName)
	if m.via != "" && m.via != catalogMatchExact {
		g.logger().Info("tool confirm gate: catalog matched via indirect rule",
			loggateway.StepID("agent.tool_confirm_gate"),
			loggateway.Str("tool_name", toolName),
			loggateway.Str("catalog_key", m.catalogKey),
			loggateway.Str("match_via", m.via),
			loggateway.Bool("requires_confirm", m.requiresConfirm),
		)
	}
	return m.requiresConfirm
}

// computerUseDangerTools are the computer-use tools whose arguments are
// content-inspected for danger words (75 A5). Only action-injecting tools
// are listed; observe/screenshot/session are read-only and never gated here.
var computerUseDangerTools = map[string]bool{
	"computer_use_act":    true,
	"computer_use_launch": true,
}

// computerUseDangerHit reports whether a computer-use act/launch invocation
// carries a danger-word target or payload text. Unparseable args fall
// through to the normal chain (the tool itself will reject them).
func computerUseDangerHit(toolName string, args []byte) bool {
	if !computerUseDangerTools[toolName] || len(args) == 0 {
		return false
	}
	var parsed map[string]any
	if err := json.Unmarshal(args, &parsed); err != nil {
		return false
	}
	target, _ := parsed["target"].(string)
	return bizcu.Policy{}.IsDanger(target, parsed)
}

func (g *toolConfirmGate) needsConfirm(toolName string, args []byte) bool {
	if g == nil {
		return false
	}
	toolName = strings.TrimSpace(toolName)
	if computerUseDangerHit(toolName, args) {
		return true
	}
	if g.catalogCheck(toolName) {
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
	if g.catalog != nil && g.catalogCheck(strings.TrimSpace(toolName)) {
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
