package plugintrpc

import (
	"context"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
)

// AgentKeyResolver maps runtime agent_key to platform agent_id (optional).
type AgentKeyResolver func(ctx context.Context, agentKey string) string

// Manager aggregates plugin Runtime and hook rules for callback chain assembly.
type Manager struct {
	rt               *Runtime
	hooks            *biz.HookResolver
	resolveAgentID   AgentKeyResolver
}

// NewManager wires Runtime + HookResolver and loads hooks from the DB.
func NewManager(rt *Runtime, hooks *biz.HookResolver) *Manager {
	m := &Manager{rt: rt, hooks: hooks}
	if hooks != nil {
		_ = hooks.Reload(context.Background())
	}
	return m
}

// ConfirmationGuardConfigForAgent returns confirmation_guard config when enabled for the agent.
func (m *Manager) ConfirmationGuardConfigForAgent(agentID string) (ConfirmationGuardConfig, bool) {
	if m == nil || m.rt == nil {
		return ConfirmationGuardConfig{}, false
	}
	return m.rt.ConfirmationGuardConfigForAgent(agentID)
}

// CostGuardBudgetTrackerForAgent returns scope-aware cost_guard budget tracker.
func (m *Manager) CostGuardBudgetTrackerForAgent(agentID string) *CostGuardBudgetTracker {
	if m == nil || m.rt == nil {
		return NewCostGuardBudgetTracker()
	}
	return m.rt.CostGuardBudgetTrackerForAgent(agentID)
}

// CostGuardBudgetTracker returns the global budget tracker.
func (m *Manager) CostGuardBudgetTracker() *CostGuardBudgetTracker {
	if m == nil || m.rt == nil {
		return NewCostGuardBudgetTracker()
	}
	return m.rt.CostGuardBudgetTracker()
}

// SetAgentKeyResolver sets optional agent_key → agent_id lookup for hook on_event scoping.
func (m *Manager) SetAgentKeyResolver(fn AgentKeyResolver) {
	if m == nil {
		return
	}
	m.resolveAgentID = fn
	if m.rt != nil {
		m.rt.SetAgentKeyResolver(fn)
	}
}

func (m *Manager) platformAgentID(ctx context.Context, agentKey string) string {
	if m == nil || m.resolveAgentID == nil {
		return ""
	}
	return strings.TrimSpace(m.resolveAgentID(ctx, agentKey))
}

// ReloadHooks refreshes hook rules from the database.
func (m *Manager) ReloadHooks(ctx context.Context) error {
	if m == nil || m.hooks == nil {
		return nil
	}
	return m.hooks.Reload(ctx)
}

// MergeChain appends hook rules and chain-orchestrated plugins onto the product base chain.
func (m *Manager) MergeChain(ctx context.Context, agentID, agentKey string, base *callbacks.Chain) *callbacks.Chain {
	if m == nil {
		return base
	}
	_ = ctx
	var entries []callbacks.Callback

	if m.hooks != nil {
		resolved := m.hooks.Resolve(agentID, agentKey)
		var stats StatsRecorder
		var notifier *HookNotifier
		if m.rt != nil {
			stats = m.rt.stats
			notifier = m.rt.HookNotifier()
		}
		entries = append(entries, wrapResilientHooks(HookCallbacks(resolved, agentID, agentKey, stats, notifier))...)
	}

	if m.rt != nil {
		plugins, orders := m.rt.ChainPluginsForAgent(agentID)
		if len(plugins) > 0 {
			pluginEntries, err := ChainEntriesForPlugins(plugins, orders)
			if err != nil {
				hookLogger.Warn("plugin.chain_mirror failed", "agent_id", agentID, "error", err)
			} else {
				entries = append(entries, pluginEntries...)
			}
		}
	}

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

// OnEvent forwards events to scope-filtered DB plugins.
func (m *Manager) OnEvent(
	ctx context.Context,
	invocation *trpcagent.Invocation,
	e *trpcevent.Event,
) (*trpcevent.Event, error) {
	agentKey := agentKeyFromInvocation(invocation)
	agentID := m.platformAgentID(ctx, agentKey)
	plugins := m.PluginsForAgent(agentID)
	if len(plugins) == 0 {
		return e, nil
	}
	mgr, err := trpcplugin.NewManager(plugins...)
	if err != nil {
		return e, err
	}
	return mgr.OnEvent(ctx, invocation, e)
}
