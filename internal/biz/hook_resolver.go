package biz

import (
	"context"
	"strings"
	"sync"
)

// ResolvedHook is one enabled hook rule that matches an agent scope.
type ResolvedHook struct {
	Hook Hook
	Rule HookConfig
}

// HookResolver loads hooks from the DB and matches them to agents at chain-build time.
type HookResolver struct {
	uc *HookUsecase

	mu    sync.RWMutex
	cache []Hook
}

// NewHookResolver creates a resolver backed by HookUsecase.
func NewHookResolver(uc *HookUsecase) *HookResolver {
	return &HookResolver{uc: uc}
}

// Reload refreshes the in-memory hook snapshot (enabled + active only).
func (r *HookResolver) Reload(ctx context.Context) error {
	if r == nil || r.uc == nil {
		return nil
	}
	all, err := r.uc.List(ctx)
	if err != nil {
		return err
	}
	enabled := make([]Hook, 0, len(all))
	for _, h := range all {
		if !hookRuleActive(h) {
			continue
		}
		cfg, err := ParseHookConfig(h.ConfigJSON)
		if err != nil || cfg.CallbackPoint == "" {
			continue
		}
		enabled = append(enabled, h)
	}
	r.mu.Lock()
	r.cache = enabled
	r.mu.Unlock()
	return nil
}

func hookRuleActive(h Hook) bool {
	if !h.Enabled {
		return false
	}
	if strings.TrimSpace(h.DeletedAt) != "" {
		return false
	}
	st := strings.ToLower(strings.TrimSpace(h.Status))
	return st == "" || st == "active"
}

// Resolve returns hooks whose config matches the agent (tool_name checked at invoke time).
func (r *HookResolver) Resolve(agentID, agentKey string) []ResolvedHook {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	snapshot := append([]Hook(nil), r.cache...)
	r.mu.RUnlock()

	out := make([]ResolvedHook, 0, len(snapshot))
	for _, h := range snapshot {
		cfg, err := ParseHookConfig(h.ConfigJSON)
		if err != nil || cfg.CallbackPoint == "" {
			continue
		}
		if !HookAppliesToAgent(cfg.Condition, agentID, agentKey) {
			continue
		}
		out = append(out, ResolvedHook{Hook: h, Rule: cfg})
	}
	return out
}
