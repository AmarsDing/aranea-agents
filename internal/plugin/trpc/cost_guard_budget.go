package plugintrpc

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// CostGuardBudgetTracker tracks daily token consumption for cost_guard.
type CostGuardBudgetTracker struct {
	mu       sync.Mutex
	day      string
	tokens   int
	repo     biz.PluginCostGuardUsageRepo
	scopeKey string
}

func NewCostGuardBudgetTracker() *CostGuardBudgetTracker {
	return &CostGuardBudgetTracker{scopeKey: "global"}
}

// SetUsageRepo enables cross-process persistence for daily totals.
func (t *CostGuardBudgetTracker) SetUsageRepo(repo biz.PluginCostGuardUsageRepo, scopeKey string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.repo = repo
	if sk := strings.TrimSpace(scopeKey); sk != "" {
		t.scopeKey = sk
	}
}

func (t *CostGuardBudgetTracker) WouldExceed(budget, add int) bool {
	if t == nil || budget <= 0 || add <= 0 {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ensureDayLocked()
	return t.tokens+add > budget
}

func (t *CostGuardBudgetTracker) TryConsume(budget, add int) bool {
	if t == nil || budget <= 0 {
		return true
	}
	if add <= 0 {
		add = 1
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.ensureDayLocked()
	if t.tokens+add > budget {
		return false
	}
	t.tokens += add
	if t.repo != nil {
		_ = t.repo.AddTokens(context.Background(), t.day, t.scopeKey, add)
	}
	return true
}

func (t *CostGuardBudgetTracker) ensureDayLocked() {
	day := time.Now().UTC().Format("2006-01-02")
	if t.day == day {
		return
	}
	t.day = day
	t.tokens = 0
	if t.repo != nil {
		if n, err := t.repo.GetTokens(context.Background(), day, t.scopeKey); err == nil {
			t.tokens = n
		}
	}
}

// ResolveCostGuardTarget returns fallback model when guard rules require routing away from baseMod.
func ResolveCostGuardTarget(baseMod string, cfg CostGuardConfig, estTokens int, tracker *CostGuardBudgetTracker) string {
	baseMod = strings.TrimSpace(baseMod)
	if need, _ := costGuardNeedsFallback(baseMod, cfg, estTokens, tracker); !need {
		return ""
	}
	fb := strings.TrimSpace(cfg.FallbackModel)
	if fb == "" || fb == baseMod || toolInList(fb, cfg.BlockedModels) {
		return ""
	}
	return fb
}

func costGuardNeedsFallback(baseMod string, cfg CostGuardConfig, estTokens int, tracker *CostGuardBudgetTracker) (bool, string) {
	if toolInList(baseMod, cfg.BlockedModels) {
		return true, "blocked_model"
	}
	if cfg.MaxPromptTokens > 0 && estTokens > cfg.MaxPromptTokens {
		return true, "max_prompt_tokens"
	}
	if cfg.DailyTokenBudget > 0 && tracker != nil && tracker.WouldExceed(cfg.DailyTokenBudget, estTokens) {
		return true, "daily_budget"
	}
	return false, ""
}

func costGuardShouldBlock(baseMod string, cfg CostGuardConfig, estTokens int, tracker *CostGuardBudgetTracker) (bool, string) {
	need, reason := costGuardNeedsFallback(baseMod, cfg, estTokens, tracker)
	if !need {
		return false, ""
	}
	if ResolveCostGuardTarget(baseMod, cfg, estTokens, tracker) != "" {
		return false, ""
	}
	return true, reason
}

// EstimateInvocationTokens heuristically estimates prompt tokens from an invocation.
func EstimateInvocationTokens(inv *trpcagent.Invocation) int {
	if inv == nil {
		return 1
	}
	n := len(invocationPromptText(inv)) / 4
	if inv.Session != nil {
		inv.Session.EventMu.RLock()
		for _, ev := range inv.Session.Events {
			if ev.Response != nil {
				for _, ch := range ev.Response.Choices {
					n += len(ch.Message.Content) / 4
				}
			}
		}
		inv.Session.EventMu.RUnlock()
	}
	if n <= 0 {
		return 1
	}
	return n
}

func invocationPromptText(inv *trpcagent.Invocation) string {
	if inv == nil {
		return ""
	}
	if c := strings.TrimSpace(inv.Message.Content); c != "" {
		return c
	}
	return ""
}

func estimateRequestTokens(req *trpcmodel.Request) int {
	return estimatePromptTokens(req)
}

func estimatePromptTokens(req *trpcmodel.Request) int {
	if req == nil {
		return 0
	}
	n := 0
	for _, m := range req.Messages {
		n += len(m.Content) / 4
	}
	if n <= 0 {
		return 1
	}
	return n
}
