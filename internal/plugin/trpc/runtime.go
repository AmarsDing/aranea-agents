package plugintrpc

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/workspace"
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
	workspaceID       string
}

type Runtime struct {
	mu sync.RWMutex
	// activeByWS partitions enabled plugins by workspace ID.
	// Key "" = shared/legacy plugins visible to every workspace.
	activeByWS     map[string][]runtimeEntry
	stats          StatsRecorder
	notifier       *HookNotifier
	retryWorker    *HookDeliveryRetryWorker
	monitorBus     contract.MonitorBus
	budgets        *CostGuardBudgetRegistry
	resolveAgent   AgentKeyResolver
	catalogConfirm CatalogConfirmChecker
	lg             loggateway.Logger
}

func NewRuntime(stats StatsRecorder, lg loggateway.Logger) *Runtime {
	return &Runtime{
		activeByWS: make(map[string][]runtimeEntry),
		stats:      stats,
		budgets:    NewCostGuardBudgetRegistry(lg),
		lg:         lg,
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
	rt.notifier = NewHookNotifier(nil, repo, rt.lg, rt.monitorBus)
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

func (rt *Runtime) SetMonitorBus(monitorBus contract.MonitorBus) {
	rt.mu.Lock()
	rt.monitorBus = monitorBus
	if rt.notifier != nil {
		rt.notifier.SetMonitorBus(monitorBus)
	}
	rt.mu.Unlock()
	InitHookLogger(monitorBus, rt.lg)
}

// Apply hot-reloads enabled plugins into workspace-partitioned storage (C-06).
//
// System workspace context replaces the entire partition map (full snapshot).
// Tenant context merges by plugin.WorkspaceID and only touches partitions present
// in the batch (plus clears the caller's partition when empty), so two workspaces
// cannot overwrite each other.
func (rt *Runtime) Apply(ctx context.Context, plugins []biz.Plugin) {
	rt.mu.RLock()
	monitorBus := rt.monitorBus
	stats := rt.stats
	rt.mu.RUnlock()
	byWS := make(map[string][]runtimeEntry)
	for _, p := range plugins {
		if !p.Enabled {
			continue
		}
		ap := adapt(p, stats, monitorBus, rt, rt.lg)
		if ap == nil {
			continue
		}
		wsID := strings.TrimSpace(p.WorkspaceID)
		e := runtimeEntry{
			plugin:            ap.plugin,
			scope:             strings.TrimSpace(p.Scope),
			key:               p.Key,
			enabled:           true,
			sortOrder:         p.SortOrder,
			orchestration:     ResolvePluginOrchestration(p),
			modelRouter:       ap.modelRouter,
			costGuard:         ap.costGuard,
			confirmationGuard: ap.confirmationGuard,
			workspaceID:       wsID,
		}
		byWS[wsID] = append(byWS[wsID], e)
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.activeByWS == nil {
		rt.activeByWS = make(map[string][]runtimeEntry)
	}
	if workspace.IsSystem(ctx) {
		rt.activeByWS = byWS
		if rt.activeByWS == nil {
			rt.activeByWS = make(map[string][]runtimeEntry)
		}
		return
	}
	if ws, ok := workspace.FromContext(ctx); ok && ws != workspace.SystemWorkspaceID {
		// Tenant reload (List = shared + own): refresh those two partitions only.
		rt.activeByWS[""] = byWS[""]
		rt.activeByWS[ws] = byWS[ws]
		return
	}
	// No workspace on ctx: merge partitions present in the batch (tests / legacy).
	for wsID, entries := range byWS {
		rt.activeByWS[wsID] = entries
	}
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

// entriesVisibleTo returns shared ("") entries plus the caller's workspace entries.
// System / empty workspaceID returns every partition (admin / legacy callers).
func (rt *Runtime) entriesVisibleTo(workspaceID string) []runtimeEntry {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" || workspaceID == workspace.SystemWorkspaceID {
		var all []runtimeEntry
		for _, entries := range rt.activeByWS {
			all = append(all, entries...)
		}
		return all
	}
	shared := rt.activeByWS[""]
	own := rt.activeByWS[workspaceID]
	out := make([]runtimeEntry, 0, len(shared)+len(own))
	out = append(out, shared...)
	out = append(out, own...)
	return out
}

// PluginsForAgent returns active plugins for the agent within workspace visibility (C-06).
// Shared plugins (workspace="") are always included; tenant plugins only when workspaceID matches.
func (rt *Runtime) PluginsForAgent(agentID, workspaceID string) []trpcplugin.Plugin {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	entries := rt.entriesVisibleTo(workspaceID)
	out := make([]trpcplugin.Plugin, 0, len(entries))
	for _, e := range entries {
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
	for _, entries := range rt.activeByWS {
		for _, e := range entries {
			if e.key != "model_router" || e.modelRouter == nil {
				continue
			}
			if PluginMatchesScope(e.scope, agentID) {
				return *e.modelRouter, true
			}
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
	for _, entries := range rt.activeByWS {
		for _, e := range entries {
			if e.key != "cost_guard" || e.costGuard == nil {
				continue
			}
			if PluginMatchesScope(e.scope, agentID) {
				return *e.costGuard, true
			}
		}
	}
	return CostGuardConfig{}, false
}

// ConfirmationGuardConfigForAgent returns confirmation_guard config when enabled for the agent.
func (rt *Runtime) ConfirmationGuardConfigForAgent(agentID string) (ConfirmationGuardConfig, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	for _, entries := range rt.activeByWS {
		for _, e := range entries {
			if e.key != "confirmation_guard" || e.confirmationGuard == nil {
				continue
			}
			if PluginMatchesScope(e.scope, agentID) {
				return *e.confirmationGuard, true
			}
		}
	}
	return ConfirmationGuardConfig{}, false
}

// Plugins returns all active plugins across all workspaces (no scope filter).
// Prefer PluginsForAgent at turn time.
func (rt *Runtime) Plugins() []trpcplugin.Plugin {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	var out []trpcplugin.Plugin
	for _, entries := range rt.activeByWS {
		for _, e := range entries {
			out = append(out, e.plugin)
		}
	}
	return out
}
