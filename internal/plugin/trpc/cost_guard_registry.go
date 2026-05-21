package plugintrpc

import (
	"context"
	"strings"
	"sync"

	"aranea-agents/internal/biz"
)

// CostGuardBudgetRegistry tracks daily token usage per scope bucket.
type CostGuardBudgetRegistry struct {
	mu      sync.RWMutex
	repo    biz.PluginCostGuardUsageRepo
	byScope map[string]*CostGuardBudgetTracker
}

func NewCostGuardBudgetRegistry() *CostGuardBudgetRegistry {
	return &CostGuardBudgetRegistry{byScope: make(map[string]*CostGuardBudgetTracker)}
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

func normalizeCostGuardScopeKey(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" || strings.EqualFold(scope, "global") {
		return "global"
	}
	return scope
}

func (r *CostGuardBudgetRegistry) TrackerForScope(scope string) *CostGuardBudgetTracker {
	if r == nil {
		return NewCostGuardBudgetTracker()
	}
	key := normalizeCostGuardScopeKey(scope)
	r.mu.Lock()
	defer r.mu.Unlock()
	if t, ok := r.byScope[key]; ok && t != nil {
		return t
	}
	t := NewCostGuardBudgetTracker()
	t.scopeKey = key
	if r.repo != nil {
		t.SetUsageRepo(r.repo, key)
	}
	r.byScope[key] = t
	return t
}

// CostGuardScopeForAgent returns the budget bucket key for an agent turn.
func (rt *Runtime) CostGuardScopeForAgent(agentID string) string {
	agentID = strings.TrimSpace(agentID)
	if rt == nil {
		if agentID != "" {
			return agentID
		}
		return "global"
	}
	rt.mu.RLock()
	defer rt.mu.RUnlock()
	for _, e := range rt.active {
		if e.key != "cost_guard" || e.costGuard == nil {
			continue
		}
		if !PluginMatchesScope(e.scope, agentID) {
			continue
		}
		if s := strings.TrimSpace(e.scope); s != "" && !strings.EqualFold(s, "global") {
			return s
		}
		if agentID != "" {
			return agentID
		}
		return "global"
	}
	if agentID != "" {
		return agentID
	}
	return "global"
}

// BudgetTrackerForContext returns the scope-aware tracker for the current invocation agent.
func (rt *Runtime) BudgetTrackerForContext(ctx context.Context) *CostGuardBudgetTracker {
	if rt == nil || rt.budgets == nil {
		return NewCostGuardBudgetTracker()
	}
	return rt.budgets.TrackerForScope(rt.CostGuardScopeForAgent(rt.platformAgentIDFromContext(ctx)))
}

// ToolRequiresConfirmation reports whether a tool needs user approval (plugin rules or catalog).
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
	cfg, ok := rt.ConfirmationGuardConfigForAgent(rt.platformAgentIDFromContext(ctx))
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

// CostGuardBudgetTrackerForAgent returns the scope-aware tracker for cost_guard.
func (rt *Runtime) CostGuardBudgetTrackerForAgent(agentID string) *CostGuardBudgetTracker {
	if rt == nil || rt.budgets == nil {
		return NewCostGuardBudgetTracker()
	}
	return rt.budgets.TrackerForScope(rt.CostGuardScopeForAgent(agentID))
}
