package plugintrpc

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

const (
	// R-2：分桶数软上限与 idle 淘汰 TTL。byScope 以 "{workspace}:{agent}"
	// 为键，多租户 + 多 agent 下会无界增长；超过软上限时淘汰 idle 超过
	// TTL 的分桶（Close 会先冲刷未持久化计数，新桶建时 ensureDayLocked
	// 会从 DB 重新加载当日用量，语义不丢）。
	costGuardMaxScopes    = 1024
	costGuardScopeIdleTTL = 48 * time.Hour
)

type CostGuardBudgetRegistry struct {
	mu      sync.RWMutex
	repo    biz.PluginCostGuardUsageRepo
	byScope map[string]*CostGuardBudgetTracker
	lg      loggateway.Logger
}

func NewCostGuardBudgetRegistry(lg loggateway.Logger) *CostGuardBudgetRegistry {
	return &CostGuardBudgetRegistry{byScope: make(map[string]*CostGuardBudgetTracker), lg: lg}
}

func (r *CostGuardBudgetRegistry) SetUsageRepo(repo biz.PluginCostGuardUsageRepo) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.repo = repo
	for key, tracker := range r.byScope {
		if tracker != nil {
			tracker.SetUsageRepo(repo, key)
		}
	}
	r.mu.Unlock()
}

func (r *CostGuardBudgetRegistry) Reset() {
	if r == nil {
		return
	}
	r.mu.Lock()
	for _, tracker := range r.byScope {
		if tracker != nil {
			tracker.Close()
		}
	}
	r.byScope = make(map[string]*CostGuardBudgetTracker)
	r.mu.Unlock()
}

func (r *CostGuardBudgetRegistry) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	for _, tracker := range r.byScope {
		if tracker != nil {
			tracker.Close()
		}
	}
	r.mu.Unlock()
}

func normalizeCostGuardScopeKey(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" || strings.EqualFold(scope, "global") {
		return "global"
	}
	return scope
}

func (r *CostGuardBudgetRegistry) TrackerForScope(scope string) *CostGuardBudgetTracker {
	if r == nil {
		return nil // R-1：nil 即 no-op，不再构造泄漏的临时 tracker
	}
	key := normalizeCostGuardScopeKey(scope)
	r.mu.RLock()
	if t, ok := r.byScope[key]; ok && t != nil {
		t.lastUsedUnix.Store(time.Now().Unix())
		r.mu.RUnlock()
		return t
	}
	r.mu.RUnlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.byScope[key]; ok && t != nil {
		t.lastUsedUnix.Store(time.Now().Unix())
		return t
	}
	r.evictIdleLocked(time.Now())
	var opts []CostGuardBudgetOption
	opts = append(opts, WithScopeKey(key))
	if r.repo != nil {
		opts = append(opts, WithUsageRepo(r.repo))
	}
	t := NewCostGuardBudgetTracker(r.lg, opts...)
	t.lastUsedUnix.Store(time.Now().Unix())
	r.byScope[key] = t
	return t
}

// evictIdleLocked removes trackers idle longer than costGuardScopeIdleTTL
// when the bucket count exceeds the soft cap. Must be called with r.mu held.
// Evicted trackers are Closed so pending persists flush before drop.
func (r *CostGuardBudgetRegistry) evictIdleLocked(now time.Time) {
	if len(r.byScope) <= costGuardMaxScopes {
		return
	}
	cutoff := now.Add(-costGuardScopeIdleTTL).Unix()
	for key, t := range r.byScope {
		if t == nil || t.lastUsedUnix.Load() < cutoff {
			if t != nil {
				t.Close()
			}
			delete(r.byScope, key)
			r.lg.Info("cost_guard 分桶 idle 淘汰",
				loggateway.StepID("plugin.cost_guard.scope_evict"),
				loggateway.Str("scope", key))
		}
	}
}

// CostGuardScopeForAgent returns the budget bucket key for an agent turn.
// E2E-P1-11: include workspace so multi-tenant shared agents do not share one
// budget bucket. Format: "{workspace}:{agentID}" (or "global" fallbacks).
func (rt *Runtime) CostGuardScopeForAgent(agentID string) string {
	return rt.CostGuardScopeForAgentInWorkspace("", agentID)
}

// CostGuardScopeForAgentInWorkspace returns the budget bucket for workspace+agent.
func (rt *Runtime) CostGuardScopeForAgentInWorkspace(workspaceID, agentID string) string {
	agentID = strings.TrimSpace(agentID)
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" || workspaceID == workspace.SystemWorkspaceID {
		workspaceID = workspace.DefaultWorkspaceID
	}
	base := func(id string) string {
		if id == "" {
			return workspaceID + ":global"
		}
		return workspaceID + ":" + id
	}
	if rt == nil {
		return base(agentID)
	}
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	for _, e := range rt.entriesVisibleTo(workspaceID) {
		if e.key != "cost_guard" || e.costGuard == nil {
			continue
		}
		if !PluginMatchesScope(e.scope, agentID) {
			continue
		}
		if s := strings.TrimSpace(e.scope); s != "" && !strings.EqualFold(s, "global") {
			return base(s)
		}
		return base(agentID)
	}
	return base(agentID)
}

// BudgetTrackerForContext returns the scope-aware tracker for the current invocation agent.
func (rt *Runtime) BudgetTrackerForContext(ctx context.Context) *CostGuardBudgetTracker {
	if rt == nil || rt.budgets == nil {
		return nil // R-1：nil 即 no-op
	}
	ws := workspace.IDFromContext(ctx)
	return rt.budgets.TrackerForScope(rt.CostGuardScopeForAgentInWorkspace(ws, rt.platformAgentIDFromContext(ctx)))
}

func (rt *Runtime) ToolRequiresConfirmation(ctx context.Context, toolName string, args []byte) bool {
	if rt == nil {
		return false
	}
	if rt.ToolMatchesConfirmationGuard(ctx, toolName, args) {
		return true
	}
	rt.mu.RLock()
	checker := rt.catalogConfirm
	rt.mu.RUnlock()
	if checker == nil {
		return false
	}
	return checker(ctx, rt.platformAgentIDFromContext(ctx), strings.TrimSpace(toolName))
}

// ToolMatchesConfirmationGuard reports whether confirmation_guard would require approval for the tool.
func (rt *Runtime) ToolMatchesConfirmationGuard(ctx context.Context, toolName string, args []byte) bool {
	if rt == nil {
		return false
	}
	// N-B1：按调用方工作区过滤，杜绝跨租户 guard 配置泄漏。
	cfg, ok := rt.ConfirmationGuardConfigForAgent(rt.platformAgentIDFromContext(ctx), workspace.IDFromContext(ctx))
	if !ok {
		return false
	}
	return MatchConfirmationGuard(cfg, toolName, args)
}

func (rt *Runtime) platformAgentIDFromContext(ctx context.Context) string {
	_, agentKey := sessionAgentKey(ctx, nil)
	agentID := strings.TrimSpace(agentKey)
	rt.mu.RLock()
	resolve := rt.resolveAgent
	rt.mu.RUnlock()
	if resolve != nil {
		if id := strings.TrimSpace(resolve(ctx, agentKey)); id != "" {
			agentID = id
		}
	}
	return agentID
}
