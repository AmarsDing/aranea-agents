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
	plugin            trpcplugin.Plugin
	scope             string
	key               string
	enabled           bool
	sortOrder         int
	orchestration     PluginOrchestrationPath
	modelRouter       *ModelRouterConfig
	costGuard         *CostGuardConfig
	confirmationGuard *ConfirmationGuardConfig
}

type Runtime struct {
	mu             sync.RWMutex
	active         []runtimeEntry
	stats          StatsRecorder
	notifier       *HookNotifier
	bus            event.Bus
	budgets        *CostGuardBudgetRegistry
	resolveAgent   AgentKeyResolver
	catalogConfirm CatalogConfirmChecker
}

func NewRuntime(stats StatsRecorder) *Runtime {
	return &Runtime{
		stats:   stats,
		budgets: NewCostGuardBudgetRegistry(),
	}
}

// SetHookDeliveryRepo enables durable Hook notify delivery with retries.
func (rt *Runtime) SetHookDeliveryRepo(repo biz.HookDeliveryRepo) {
	if rt == nil {
		return
	}
	rt.mu.Lock()
	rt.notifier = NewHookNotifier(repo)
	rt.mu.Unlock()
}

// HookNotifier returns the configured Hook notify worker.
func (rt *Runtime) HookNotifier() *HookNotifier {
	if rt == nil {
		return nil
	}
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.notifier
}

// SetAgentKeyResolver enables agent_key → agent_id lookup for scoped plugins.
func (rt *Runtime) SetAgentKeyResolver(fn AgentKeyResolver) {
	rt.mu.Lock()
	rt.resolveAgent = fn
	rt.mu.Unlock()
}

// SetCatalogConfirmChecker wires catalog requires_confirmation lookup for plugins.
func (rt *Runtime) SetCatalogConfirmChecker(fn CatalogConfirmChecker) {
	rt.mu.Lock()
	rt.catalogConfirm = fn
	rt.mu.Unlock()
}

// SetToolUsecase wires the tool usecase for catalog confirmation checks.
// This encapsulates the biz.ToolUsecase → CatalogConfirmChecker adapter
// inside the plugin/trpc package instead of requiring a closure in wire.go.
func (rt *Runtime) SetToolUsecase(tools *biz.ToolUsecase) {
	if rt == nil || tools == nil {
		return
	}
	rt.SetCatalogConfirmChecker(func(ctx context.Context, agentID, toolName string) bool {
		return tools.RequiresConfirmationForAgent(ctx, agentID, toolName)
	})
}

// CostGuardBudgetTracker returns the global budget tracker (legacy; prefer CostGuardBudgetTrackerForAgent).
func (rt *Runtime) CostGuardBudgetTracker() *CostGuardBudgetTracker {
	if rt == nil || rt.budgets == nil {
		return NewCostGuardBudgetTracker()
	}
	return rt.budgets.TrackerForScope("global")
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
		if tp := adapt(p, stats, bus, rt); tp != nil {
			ValidatePluginCallbackPoints(p)
			e := runtimeEntry{
				plugin:        tp,
				scope:         strings.TrimSpace(p.Scope),
				key:           p.Key,
				enabled:       true,
				sortOrder:     p.SortOrder,
				orchestration: ResolvePluginOrchestration(p),
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
			if p.Key == "confirmation_guard" {
				var cfg ConfirmationGuardConfig
				parsePluginConfig(p.ConfigJSON, p.DefaultConfigJSON, &cfg)
				e.confirmationGuard = &cfg
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

// PluginsForAgent returns Runner-orchestrated plugins for the agent (excludes chain-only).
func (rt *Runtime) PluginsForAgent(agentID string) []trpcplugin.Plugin {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	out := make([]trpcplugin.Plugin, 0, len(rt.active))
	for _, e := range rt.active {
		if e.orchestration == OrchestrationChain {
			continue
		}
		if PluginMatchesScope(e.scope, agentID) {
			out = append(out, e.plugin)
		}
	}
	return out
}

// ChainPluginsForAgent returns plugins mirrored into LLMAgent Callback Chain for the agent.
func (rt *Runtime) ChainPluginsForAgent(agentID string) ([]trpcplugin.Plugin, []int) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	plugins := make([]trpcplugin.Plugin, 0)
	orders := make([]int, 0)
	for _, e := range rt.active {
		if e.orchestration != OrchestrationChain {
			continue
		}
		if PluginMatchesScope(e.scope, agentID) {
			plugins = append(plugins, e.plugin)
			orders = append(orders, e.sortOrder)
		}
	}
	return plugins, orders
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

// SetCostGuardUsageRepo enables cross-process daily token persistence.
func (rt *Runtime) SetCostGuardUsageRepo(repo biz.PluginCostGuardUsageRepo) {
	if rt == nil {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.budgets != nil {
		rt.budgets.SetUsageRepo(repo)
	}
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

// ConfirmationGuardConfigForAgent returns confirmation_guard config when enabled for the agent.
func (rt *Runtime) ConfirmationGuardConfigForAgent(agentID string) (ConfirmationGuardConfig, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	for _, e := range rt.active {
		if e.key != "confirmation_guard" || e.confirmationGuard == nil {
			continue
		}
		if PluginMatchesScope(e.scope, agentID) {
			return *e.confirmationGuard, true
		}
	}
	return ConfirmationGuardConfig{}, false
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
