package plugintrpc

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

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
	retryWorker    *HookDeliveryRetryWorker
	bus            event.Bus
	budgets        *CostGuardBudgetRegistry
	resolveAgent   AgentKeyResolver
	catalogConfirm CatalogConfirmChecker
	lg             loggateway.Logger
}

func NewRuntime(stats StatsRecorder, lg loggateway.Logger) *Runtime {
	return &Runtime{
		stats:   stats,
		budgets: NewCostGuardBudgetRegistry(lg),
		lg:      lg,
	}
}

// SetHookDeliveryRepo enables durable Hook notify delivery with retries and
// starts the background retry worker for crash-recovery (OUT-02 / HK-01).
func (rt *Runtime) SetHookDeliveryRepo(repo biz.HookDeliveryRepo) {
	if rt == nil {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.notifier = NewHookNotifier(nil, repo, rt.lg)
	if repo != nil {
		rt.retryWorker = NewHookDeliveryRetryWorker(nil, repo, rt.notifier, rt.lg)
	}
}

func (rt *Runtime) StartBackgroundWorkers() {
	if rt == nil {
		return
	}
	rt.mu.RLock()
	w := rt.retryWorker
	rt.mu.RUnlock()
	if w != nil {
		w.Start()
	}
}

// Close stops background workers started by this Runtime (e.g. hook retry worker, stats worker).
func (rt *Runtime) Close() {
	if rt == nil {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.retryWorker != nil {
		rt.retryWorker.Stop()
	}
	if c, ok := rt.stats.(interface{ Close() }); ok {
		c.Close()
	}
	if rt.budgets != nil {
		rt.budgets.Reset()
	}
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
		return NewCostGuardBudgetTracker(rt.lg)
	}
	return rt.budgets.TrackerForScope("global")
}

func (rt *Runtime) SetBus(bus event.Bus) {
	rt.mu.Lock()
	rt.bus = bus
	rt.mu.Unlock()
	InitHookLogger(bus, rt.lg)
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
		ap := adapt(p, stats, bus, rt, rt.lg)
		if ap == nil {
			continue
		}
		e := runtimeEntry{
			plugin:        ap.plugin,
			scope:         strings.TrimSpace(p.Scope),
			key:           p.Key,
			enabled:       true,
			sortOrder:     p.SortOrder,
			orchestration: ResolvePluginOrchestration(p),
			modelRouter:         ap.modelRouter,
			costGuard:           ap.costGuard,
			confirmationGuard:   ap.confirmationGuard,
		}
		built = append(built, e)
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

// PluginsForAgent returns active plugins for the agent.
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
