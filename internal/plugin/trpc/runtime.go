package plugintrpc

import (
	"context"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
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
	// decisions 是 M80 决策记录入口（P1-④，2026-08-30）：output_policy 等
	// 插件的阻断事件需写 system_guard 决策记录。经 SetDecisionCollector
	// 后置注入（插件 Apply 可能早于 app BeforeStart，故插件侧经
	// decisionCollector() 每次现取而非构造期快照）。
	decisions decision.Collector

	// R-3：Apply 纪元。并发热重载（系统全量 vs 租户增量、慢 DB List vs
	// 快 List）可能乱序提交；提交时若已有更新纪元落盘则丢弃陈旧快照。
	applySeq   atomic.Int64
	appliedSeq atomic.Int64
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

// SetDecisionCollector 注入 M80 决策记录 collector（P1-④）。由 newApp
// BeforeStart 接线；插件在事件发射点经 decisionCollector() 现取，对
// Apply/注入的先后序免疫。
func (rt *Runtime) SetDecisionCollector(c decision.Collector) {
	if rt == nil {
		return
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	rt.decisions = c
}

// decisionCollector 返回当前注入的决策 collector（nil = 决策记录降级）。
func (rt *Runtime) decisionCollector() decision.Collector {
	if rt == nil {
		return nil
	}
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	return rt.decisions
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
		// N-B3：Stop 信号现在可靠，Close 有界等待 loop 退出，避免 in-flight
		// 重试被硬切（超时仍继续关闭，不阻塞进程退出）。
		rt.retryWorker.Wait(3 * time.Second)
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
	// R-3：领取纪元（入口处，快照生成前）；提交时若已有更新纪元落盘则丢弃。
	seq := rt.applySeq.Add(1)
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
	if seq < rt.appliedSeq.Load() {
		rt.lg.Warn("丢弃过期的插件热重载快照",
			loggateway.StepID("plugin.runtime.stale_apply"),
			loggateway.Int64("seq", seq),
			loggateway.Int64("applied_seq", rt.appliedSeq.Load()))
		return
	}
	rt.appliedSeq.Store(seq)
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
// R-7：同 key 去重——租户自有插件覆盖 shared 同 key 插件，避免重复注册
// （同一 BeforeModel/BeforeTool 被挂两次）。
func (rt *Runtime) PluginsForAgent(agentID, workspaceID string) []trpcplugin.Plugin {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	workspaceID = strings.TrimSpace(workspaceID)
	entries := rt.entriesVisibleTo(workspaceID)
	var ownKeys map[string]bool
	if workspaceID != "" && workspaceID != workspace.SystemWorkspaceID {
		for _, e := range rt.activeByWS[workspaceID] {
			if ownKeys == nil {
				ownKeys = make(map[string]bool)
			}
			ownKeys[e.key] = true
		}
	}
	out := make([]trpcplugin.Plugin, 0, len(entries))
	for _, e := range entries {
		if e.workspaceID == "" && ownKeys[e.key] {
			continue // 租户同 key 插件覆盖 shared
		}
		if PluginMatchesScope(e.scope, agentID) {
			out = append(out, e.plugin)
		}
	}
	return out
}

// configEntriesFor returns entries visible to workspaceID in deterministic
// priority order (N-B1): the caller's own workspace partition first (tenant
// config overrides shared on key conflict), then shared ("") entries.
// System / empty workspaceID (admin / legacy callers) sees shared first, then
// every other partition sorted by workspace ID — map iteration order must
// never decide which tenant's config wins.
func (rt *Runtime) configEntriesFor(workspaceID string) []runtimeEntry {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID != "" && workspaceID != workspace.SystemWorkspaceID {
		own := rt.activeByWS[workspaceID]
		shared := rt.activeByWS[""]
		out := make([]runtimeEntry, 0, len(own)+len(shared))
		out = append(out, own...)
		out = append(out, shared...)
		return out
	}
	shared := rt.activeByWS[""]
	rest := make([]string, 0, len(rt.activeByWS))
	for wsID := range rt.activeByWS {
		if wsID != "" {
			rest = append(rest, wsID)
		}
	}
	sort.Strings(rest)
	out := make([]runtimeEntry, 0, len(shared))
	out = append(out, shared...)
	for _, wsID := range rest {
		out = append(out, rt.activeByWS[wsID]...)
	}
	return out
}

// ModelRouterConfigForAgent returns model_router config when the plugin is
// enabled for the agent within workspace visibility (N-B1). The caller's own
// workspace plugin overrides a shared plugin of the same key.
func (rt *Runtime) ModelRouterConfigForAgent(agentID, workspaceID string) (ModelRouterConfig, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	for _, e := range rt.configEntriesFor(workspaceID) {
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

// CostGuardConfigForAgent returns cost_guard config when the plugin is
// enabled for the agent within workspace visibility (N-B1).
func (rt *Runtime) CostGuardConfigForAgent(agentID, workspaceID string) (CostGuardConfig, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	for _, e := range rt.configEntriesFor(workspaceID) {
		if e.key != "cost_guard" || e.costGuard == nil {
			continue
		}
		if PluginMatchesScope(e.scope, agentID) {
			return *e.costGuard, true
		}
	}
	return CostGuardConfig{}, false
}

// ConfirmationGuardConfigForAgent returns confirmation_guard config when
// enabled for the agent within workspace visibility (N-B1).
func (rt *Runtime) ConfirmationGuardConfigForAgent(agentID, workspaceID string) (ConfirmationGuardConfig, bool) {
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	for _, e := range rt.configEntriesFor(workspaceID) {
		if e.key != "confirmation_guard" || e.confirmationGuard == nil {
			continue
		}
		if PluginMatchesScope(e.scope, agentID) {
			return *e.confirmationGuard, true
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
