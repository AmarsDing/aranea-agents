package plugintrpc

import (
	"strings"

	"aranea-agents/internal/biz"
)

// PluginOrchestrationPath decides whether a DB plugin runs on Runner, Chain, or both.
type PluginOrchestrationPath string

const (
	// OrchestrationRunner is the default: plugin hooks via Runner WithPlugins only.
	OrchestrationRunner PluginOrchestrationPath = "runner"
	// OrchestrationChain mirrors lifecycle hooks into LLMAgent Chain and excludes Runner (no double trigger).
	OrchestrationChain PluginOrchestrationPath = "chain"
)

// chainAllowlistBuiltinKeys are built-ins allowed to mirror into LLMAgent Chain when
// config_json sets callback_orchestration:"chain". All other built-ins stay Runner-only.
var chainAllowlistBuiltinKeys = map[string]struct{}{
	"skill_usage_tracker": {},
}

// runnerExclusiveKeys are built-ins that must never move to Chain-only (split concerns / OnEvent).
var runnerExclusiveKeys = map[string]struct{}{
	"audit_log":           {},
	"audit-log":           {},
	"auditlog":            {},
	"runtime_audit":       {},
	"model_router":        {},
	"cost_guard":          {},
	"confirmation_guard":  {},
	"output_policy":       {},
}

// ParsePluginOrchestrationPath reads callback_orchestration from plugin config JSON.
func ParsePluginOrchestrationPath(configJSON, defaultJSON string) PluginOrchestrationPath {
	merged := map[string]any{}
	parsePluginConfig(configJSON, defaultJSON, &merged)
	if merged == nil {
		return OrchestrationRunner
	}
	raw, _ := merged["callback_orchestration"].(string)
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "chain", "chain_only", "llm_chain":
		return OrchestrationChain
	default:
		return OrchestrationRunner
	}
}

// ResolvePluginOrchestration returns the effective path for a plugin row.
func ResolvePluginOrchestration(p biz.Plugin) PluginOrchestrationPath {
	key := strings.ToLower(strings.TrimSpace(p.Key))
	if _, exclusive := runnerExclusiveKeys[key]; exclusive {
		return OrchestrationRunner
	}
	path := ParsePluginOrchestrationPath(p.ConfigJSON, p.DefaultConfigJSON)
	if path != OrchestrationChain {
		return OrchestrationRunner
	}
	if pluginDeclaresOnEvent(p) {
		hookLogger.Warn("plugin: chain orchestration incompatible with on_event; forcing runner",
			"plugin", p.Key,
		)
		return OrchestrationRunner
	}
	if len(BuiltinCallbackPoints(p.Key)) > 0 && !chainAllowlistBuiltin(p.Key) {
		hookLogger.Warn("plugin: builtin not on chain allowlist; forcing runner",
			"plugin", p.Key,
		)
		return OrchestrationRunner
	}
	return OrchestrationChain
}

func chainAllowlistBuiltin(pluginKey string) bool {
	_, ok := chainAllowlistBuiltinKeys[strings.ToLower(strings.TrimSpace(pluginKey))]
	return ok
}

func pluginDeclaresOnEvent(p biz.Plugin) bool {
	for _, pt := range p.CallbackPoints {
		if strings.EqualFold(strings.TrimSpace(pt), "on_event") {
			return true
		}
	}
	pts := BuiltinCallbackPoints(p.Key)
	for _, pt := range pts {
		if pt == "on_event" {
			return true
		}
	}
	return false
}

// UsesChainOrchestration reports whether lifecycle hooks should mirror into LLMAgent Chain.
func UsesChainOrchestration(p biz.Plugin) bool {
	return ResolvePluginOrchestration(p) == OrchestrationChain
}

// UsesRunnerOrchestration reports whether the plugin stays on Runner WithPlugins.
func UsesRunnerOrchestration(p biz.Plugin) bool {
	return ResolvePluginOrchestration(p) != OrchestrationChain
}
