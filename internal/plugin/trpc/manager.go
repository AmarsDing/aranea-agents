package plugintrpc

// Callback orchestration boundaries:
//
// 1. Runner WithPlugins — DB-backed built-in plugins + framework plugins (identity, guardrail,
//    toolcallid, messagemerger). Order: plugins.sort_order ASC at Runtime.Apply, then framework
//    plugins appended by Manager.RunnerPluginsForAgent.
//
// 2. LLMAgent Callback Chain — product metrics, tool timing/recorder + hook rules.
//    Order: fixed product priorities (timing=5, recorder=50) + hooks at 300+sort_order.
//
// 3. ModelSelector — model_router / cost_guard catalog swaps only (no duplicate BeforeModel routing).
//
// 4. Hook rules — user-defined Chain entries; on_event via productEventPlugin bridge.
//
// confirmation_guard Runner plugin blocks directly via BeforeTool CustomResult.
// permission_guard denies deny_tools via BeforeTool CustomResult.

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcguardrail "trpc.group/trpc-go/trpc-agent-go/plugin/guardrail"
	trpcidentity "trpc.group/trpc-go/trpc-agent-go/plugin/identity"
	trpcmessagemerger "trpc.group/trpc-go/trpc-agent-go/plugin/messagemerger"
	trpctoolcallid "trpc.group/trpc-go/trpc-agent-go/plugin/toolcallid"
)

// AgentKeyResolver maps runtime agent_key to platform agent_id (optional).
type AgentKeyResolver func(ctx context.Context, agentKey string) string

// Manager aggregates plugin Runtime and hook rules for callback chain assembly.
type Manager struct {
	rt             *Runtime
	hooks          *biz.HookResolver
	resolveAgentID AgentKeyResolver
	resolveMu      sync.RWMutex
	lg             loggateway.Logger

	guardrailOnce sync.Once
	guardrail     *trpcguardrail.Plugin
	guardrailErr  error

	identityOnce sync.Once
	identity     *trpcidentity.Plugin
}

func NewManager(rt *Runtime, hooks *biz.HookResolver, lg loggateway.Logger) *Manager {
	m := &Manager{rt: rt, hooks: hooks, lg: lg}
	if hooks != nil {
		if err := hooks.Reload(context.Background()); err != nil {
			lg.Warn("Hook rules load failed, hook notifications unavailable",
				loggateway.StepID("plugin.manager.hook_reload_fail"),
				loggateway.Err(err))
		}
	}
	return m
}

// ConfirmationGuardConfigForAgent returns confirmation_guard config when
// enabled for the agent within workspace visibility (N-B1).
func (m *Manager) ConfirmationGuardConfigForAgent(agentID, workspaceID string) (ConfirmationGuardConfig, bool) {
	if m == nil || m.rt == nil {
		return ConfirmationGuardConfig{}, false
	}
	return m.rt.ConfirmationGuardConfigForAgent(agentID, workspaceID)
}

// BudgetTrackerForContext returns the cost_guard budget tracker resolved from
// the invocation context (workspace + agent) — the single bucket the runtime
// cost_guard plugin also consumes (N-B2). nil Manager/Runtime 返回 nil，
// tracker 全部方法对 nil 接收器为 no-op（R-1：不再构造泄漏的临时 tracker）。
func (m *Manager) BudgetTrackerForContext(ctx context.Context) *CostGuardBudgetTracker {
	if m == nil || m.rt == nil {
		return nil
	}
	return m.rt.BudgetTrackerForContext(ctx)
}

// SetAgentKeyResolver sets optional agent_key → agent_id lookup for hook on_event scoping.
func (m *Manager) SetAgentKeyResolver(fn AgentKeyResolver) {
	if m == nil {
		return
	}
	m.resolveMu.Lock()
	m.resolveAgentID = fn
	m.resolveMu.Unlock()
	if m.rt != nil {
		m.rt.SetAgentKeyResolver(fn)
	}
}

func (m *Manager) platformAgentID(ctx context.Context, agentKey string) string {
	if m == nil {
		return ""
	}
	m.resolveMu.RLock()
	fn := m.resolveAgentID
	m.resolveMu.RUnlock()
	if fn == nil {
		return ""
	}
	return strings.TrimSpace(fn(ctx, agentKey))
}

// ReloadHooks refreshes hook rules from the database.
func (m *Manager) ReloadHooks(ctx context.Context) error {
	if m == nil || m.hooks == nil {
		return nil
	}
	return m.hooks.Reload(ctx)
}

// MergeChain appends hook rules onto the product base chain.
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
		entries = append(entries, wrapResilientHooks(HookCallbacks(resolved, agentID, agentKey, stats, notifier, m.lg))...)
	}

	if len(entries) == 0 {
		return base
	}
	if base == nil {
		return callbacks.NewChain(entries...)
	}
	return base.Append(entries...)
}

// ModelRouterConfigForAgent returns model_router config when enabled for the
// agent within workspace visibility (N-B1).
func (m *Manager) ModelRouterConfigForAgent(agentID, workspaceID string) (ModelRouterConfig, bool) {
	if m == nil || m.rt == nil {
		return ModelRouterConfig{}, false
	}
	return m.rt.ModelRouterConfigForAgent(agentID, workspaceID)
}

// CostGuardConfigForAgent returns cost_guard config when enabled for the
// agent within workspace visibility (N-B1).
func (m *Manager) CostGuardConfigForAgent(agentID, workspaceID string) (CostGuardConfig, bool) {
	if m == nil || m.rt == nil {
		return CostGuardConfig{}, false
	}
	return m.rt.CostGuardConfigForAgent(agentID, workspaceID)
}

// Plugins returns DB-backed runner plugins (without the event bridge).
func (m *Manager) Plugins() []trpcplugin.Plugin {
	if m == nil || m.rt == nil {
		return nil
	}
	return m.rt.Plugins()
}

// PluginsForAgent returns scope-filtered runner plugins visible to workspaceID (C-06).
func (m *Manager) PluginsForAgent(agentID, workspaceID string) []trpcplugin.Plugin {
	if m == nil || m.rt == nil {
		return nil
	}
	return m.rt.PluginsForAgent(agentID, workspaceID)
}

// RunnerPlugins returns plugins for trpcrunner.WithPlugins, including the OnEvent bridge.
func (m *Manager) RunnerPlugins() []trpcplugin.Plugin {
	return m.RunnerPluginsForAgent("", "")
}

// RunnerPluginsForAgent returns scope-filtered plugins plus the OnEvent bridge and framework plugins.
func (m *Manager) RunnerPluginsForAgent(agentID, workspaceID string) []trpcplugin.Plugin {
	if m == nil {
		return nil
	}
	plugins := m.PluginsForAgent(agentID, workspaceID)
	plugins = append(plugins, &productEventPlugin{mgr: m, name: "aranea_event_bridge"})
	m.resolveMu.RLock()
	resolver := m.resolveAgentID
	m.resolveMu.RUnlock()
	if resolver != nil {
		m.identityOnce.Do(func() {
			m.identity = BuildIdentityPlugin(resolver)
		})
		if m.identity != nil {
			plugins = append(plugins, m.identity)
		}
	}
	m.guardrailOnce.Do(func() {
		m.guardrail, m.guardrailErr = BuildGuardrailPlugin()
	})
	if m.guardrailErr == nil && m.guardrail != nil {
		plugins = append(plugins, m.guardrail)
	}
	plugins = append(plugins,
		trpctoolcallid.New(),
		trpcmessagemerger.New(),
	)
	return plugins
}
