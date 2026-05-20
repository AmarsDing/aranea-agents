package plugintrpc

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
)

type runtimeEntry struct {
	plugin      trpcplugin.Plugin
	scope       string
	key         string
	enabled     bool
	modelRouter *ModelRouterConfig
	costGuard   *CostGuardConfig
}

type Runtime struct {
	mu      sync.RWMutex
	active  []runtimeEntry
	stats   StatsRecorder
	bus     event.Bus
}

func NewRuntime(stats StatsRecorder) *Runtime {
	return &Runtime{stats: stats}
}

func (rt *Runtime) SetBus(bus event.Bus) {
	rt.mu.Lock()
	rt.bus = bus
	rt.mu.Unlock()
	InitHookLogger(bus)
}

func (rt *Runtime) Apply(_ context.Context, plugins []biz.Plugin) {
	rt.mu.RLock()
	bus := rt.bus
	stats := rt.stats
	rt.mu.RUnlock()
	built := make([]runtimeEntry, 0, len(plugins))
	for _, p := range plugins {
		if !p.Enabled {
			continue
		}
		if tp := adapt(p, stats, bus); tp != nil {
			e := runtimeEntry{
				plugin:  tp,
				scope:   strings.TrimSpace(p.Scope),
				key:     p.Key,
				enabled: true,
			}
			if p.Key == "model_router" {
				var cfg ModelRouterConfig
				parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
				e.modelRouter = &cfg
			}
			if p.Key == "cost_guard" {
				var cfg CostGuardConfig
				parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
				e.costGuard = &cfg
			}
			built = append(built, e)
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

// ModelRouterConfigForAgent returns model_router config when the plugin is enabled for the agent.
func (rt *Runtime) ModelRouterConfigForAgent(agentID string) (ModelRouterConfig, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	for _, e := range rt.active {
		if e.key != "model_router" || e.modelRouter == nil {
			continue
		}
		if PluginMatchesScope(e.scope, agentID) {
			return *e.modelRouter, true
		}
	}
	return ModelRouterConfig{}, false
}

// CostGuardConfigForAgent returns cost_guard config when the plugin is enabled for the agent.
func (rt *Runtime) CostGuardConfigForAgent(agentID string) (CostGuardConfig, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	for _, e := range rt.active {
		if e.key != "cost_guard" || e.costGuard == nil {
			continue
		}
		if PluginMatchesScope(e.scope, agentID) {
			return *e.costGuard, true
		}
	}
	return CostGuardConfig{}, false
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
