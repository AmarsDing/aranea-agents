package plugintrpc

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/internal/biz"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

type runtimeEntry struct {
	plugin trpcplugin.Plugin
	scope  string
	key    string
}

// Runtime manages the set of active trpc-agent-go plugin.Plugin instances
// derived from enabled biz.Plugin DB rows.  It is safe for concurrent use.
//
// Call Apply whenever the enabled state of any plugin changes; the next runner
// creation will pick up the updated plugin slice automatically.
type Runtime struct {
	mu      sync.RWMutex
	active  []runtimeEntry
	stats   StatsRecorder
}

// NewRuntime creates an empty Runtime (no active plugins).
func NewRuntime(stats StatsRecorder) *Runtime {
	return &Runtime{stats: stats}
}

// Apply replaces the active plugin set from the supplied DB snapshot.
// Only enabled plugins with a known built-in key are instantiated.
func (rt *Runtime) Apply(_ context.Context, plugins []biz.Plugin) {
	built := make([]runtimeEntry, 0, len(plugins))
	for _, p := range plugins {
		if tp := adapt(p, rt.stats); tp != nil {
			built = append(built, runtimeEntry{
				plugin: tp,
				scope:  strings.TrimSpace(p.Scope),
				key:    p.Key,
			})
		}
	}
	rt.mu.Lock()
	rt.active = built
	rt.mu.Unlock()
}

// PluginMatchesScope reports whether a plugin scope applies to the given agent ID.
// scope "global" or empty matches all agents; otherwise scope must equal agentID.
func PluginMatchesScope(scope, agentID string) bool {
	scope = strings.TrimSpace(scope)
	agentID = strings.TrimSpace(agentID)
	if scope == "" || strings.EqualFold(scope, "global") {
		return true
	}
	if agentID == "" {
		return false
	}
	return scope == agentID
}

// PluginsForAgent returns plugins whose scope is global or matches agentID.
func (rt *Runtime) PluginsForAgent(agentID string) []trpcplugin.Plugin {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	out := make([]trpcplugin.Plugin, 0, len(rt.active))
	for _, e := range rt.active {
		if PluginMatchesScope(e.scope, agentID) {
			out = append(out, e.plugin)
		}
	}
	return out
}

// Plugins returns all active plugins (no scope filter). Prefer PluginsForAgent at turn time.
func (rt *Runtime) Plugins() []trpcplugin.Plugin {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	out := make([]trpcplugin.Plugin, len(rt.active))
	for i, e := range rt.active {
		out[i] = e.plugin
	}
	return out
}
