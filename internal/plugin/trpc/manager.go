package plugintrpc

import (
	"context"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

// Manager aggregates plugin Runtime and hook rules for callback chain assembly.
type Manager struct {
	rt    *Runtime
	hooks *biz.HookResolver
}

// NewManager wires Runtime + HookResolver and loads hooks from the DB.
func NewManager(rt *Runtime, hooks *biz.HookResolver) *Manager {
	m := &Manager{rt: rt, hooks: hooks}
	if hooks != nil {
		_ = hooks.Reload(context.Background())
	}
	return m
}

// ReloadHooks refreshes hook rules from the database.
func (m *Manager) ReloadHooks(ctx context.Context) error {
	if m == nil || m.hooks == nil {
		return nil
	}
	return m.hooks.Reload(ctx)
}

// MergeChain appends hook-derived callbacks onto the product base chain.
func (m *Manager) MergeChain(ctx context.Context, agentID, agentKey string, base *callbacks.Chain) *callbacks.Chain {
	if m == nil || m.hooks == nil {
		return base
	}
	_ = ctx
	resolved := m.hooks.Resolve(agentID, agentKey)
	entries := wrapResilientHooks(HookCallbacks(resolved, agentID, agentKey))
	if len(entries) == 0 {
		return base
	}
	if base == nil {
		return callbacks.NewChain(entries...)
	}
	return base.Append(entries...)
}

// ModelRouterConfigForAgent returns model_router config when enabled for the agent.
func (m *Manager) ModelRouterConfigForAgent(agentID string) (ModelRouterConfig, bool) {
	if m == nil || m.rt == nil {
		return ModelRouterConfig{}, false
	}
	return m.rt.ModelRouterConfigForAgent(agentID)
}

// CostGuardConfigForAgent returns cost_guard config when enabled for the agent.
func (m *Manager) CostGuardConfigForAgent(agentID string) (CostGuardConfig, bool) {
	if m == nil || m.rt == nil {
		return CostGuardConfig{}, false
	}
	return m.rt.CostGuardConfigForAgent(agentID)
}

// Plugins returns DB-backed runner plugins (without the event bridge).
func (m *Manager) Plugins() []trpcplugin.Plugin {
	if m == nil || m.rt == nil {
		return nil
	}
	return m.rt.Plugins()
}

// PluginsForAgent returns scope-filtered runner plugins.
func (m *Manager) PluginsForAgent(agentID string) []trpcplugin.Plugin {
	if m == nil || m.rt == nil {
		return nil
	}
	return m.rt.PluginsForAgent(agentID)
}

// RunnerPlugins returns plugins for trpcrunner.WithPlugins, including the OnEvent bridge.
func (m *Manager) RunnerPlugins() []trpcplugin.Plugin {
	return m.RunnerPluginsForAgent("")
}

// RunnerPluginsForAgent returns scope-filtered plugins plus the OnEvent bridge.
func (m *Manager) RunnerPluginsForAgent(agentID string) []trpcplugin.Plugin {
	if m == nil {
		return nil
	}
	plugins := m.PluginsForAgent(agentID)
	return append(plugins, &productEventPlugin{mgr: m})
}

// OnEvent forwards events to registered trpc plugins (DB plugins only).
func (m *Manager) OnEvent(
	ctx context.Context,
	invocation *trpcagent.Invocation,
	e *trpcevent.Event,
) (*trpcevent.Event, error) {
	plugins := m.Plugins()
	if len(plugins) == 0 {
		return e, nil
	}
	mgr, err := trpcplugin.NewManager(plugins...)
	if err != nil {
		return e, err
	}
	return mgr.OnEvent(ctx, invocation, e)
}
