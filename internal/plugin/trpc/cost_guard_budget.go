package plugintrpc

import (
	"context"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

type costGuardPersistEntry struct {
	day   string
	scope string
	delta int
}

type CostGuardBudgetTracker struct {
	mu       sync.Mutex
	day      string
	tokens   int
	repo     biz.PluginCostGuardUsageRepo
	scopeKey string

	persistCh   chan costGuardPersistEntry
	persistDone chan struct{}
	persistWg   sync.WaitGroup
}

type CostGuardBudgetOption func(*CostGuardBudgetTracker)

func WithUsageRepo(repo biz.PluginCostGuardUsageRepo) CostGuardBudgetOption {
	return func(t *CostGuardBudgetTracker) {
		t.repo = repo
	}
}

func WithScopeKey(key string) CostGuardBudgetOption {
	return func(t *CostGuardBudgetTracker) {
		if sk := strings.TrimSpace(key); sk != "" {
			t.scopeKey = sk
		}
	}
}

const (
	costGuardPersistChanSize = 256
	costGuardPersistFlushMs  = 500
	costGuardPersistBatch    = 32
)

func NewCostGuardBudgetTracker(opts ...CostGuardBudgetOption) *CostGuardBudgetTracker {
	t := &CostGuardBudgetTracker{
		scopeKey:    "global",
		persistCh:   make(chan costGuardPersistEntry, costGuardPersistChanSize),
		persistDone: make(chan struct{}),
	}
	for _, opt := range opts {
		opt(t)
	}
	t.persistWg.Add(1)
	safego.Go(context.Background(), "cost_guard_budget.persist_worker", t.persistWorker)
	return t
}

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
	var day, scope string
	var repo biz.PluginCostGuardUsageRepo
	consumed := false
	t.mu.Lock()
	t.ensureDayLocked()
	if t.tokens+add <= budget {
		t.tokens += add
		day = t.day
		scope = t.scopeKey
		repo = t.repo
		consumed = true
	}
	t.mu.Unlock()
	if consumed && repo != nil {
		t.persistAdd(day, scope, add)
	}
	return consumed
}

func (t *CostGuardBudgetTracker) AddOverBudget(add int) {
	if t == nil || add <= 0 {
		return
	}
	t.mu.Lock()
	t.ensureDayLocked()
	t.tokens += add
	day := t.day
	scope := t.scopeKey
	repo := t.repo
	t.mu.Unlock()
	if repo != nil {
		t.persistAdd(day, scope, add)
	}
}

func (t *CostGuardBudgetTracker) persistAdd(day, scope string, delta int) {
	select {
	case t.persistCh <- costGuardPersistEntry{day: day, scope: scope, delta: delta}:
	default:
		event.SysLogWarn("system.plugin.cost_guard_persist_drop", "cost_guard persist channel full, entry dropped",
			event.P("scope", scope), event.P("day", day), event.P("delta", delta))
	}
}

func (t *CostGuardBudgetTracker) Close() {
	if t == nil {
		return
	}
	close(t.persistDone)
	t.persistWg.Wait()
}

func (t *CostGuardBudgetTracker) persistWorker() {
	defer t.persistWg.Done()
	ticker := time.NewTicker(time.Duration(costGuardPersistFlushMs) * time.Millisecond)
	defer ticker.Stop()
	buf := make([]costGuardPersistEntry, 0, costGuardPersistBatch)
	for {
		select {
		case entry, ok := <-t.persistCh:
			if !ok {
				t.flushPersist(buf)
				return
			}
			buf = append(buf, entry)
			if len(buf) >= costGuardPersistBatch {
				t.flushPersist(buf)
				buf = buf[:0]
			}
		case <-ticker.C:
			if len(buf) > 0 {
				t.flushPersist(buf)
				buf = buf[:0]
			}
		case <-t.persistDone:
			t.drainPersist(buf)
			return
		}
	}
}

func (t *CostGuardBudgetTracker) drainPersist(buf []costGuardPersistEntry) {
	for {
		select {
		case entry, ok := <-t.persistCh:
			if !ok {
				t.flushPersist(buf)
				return
			}
			buf = append(buf, entry)
		default:
			t.flushPersist(buf)
			return
		}
	}
}

func (t *CostGuardBudgetTracker) flushPersist(batch []costGuardPersistEntry) {
	if len(batch) == 0 {
		return
	}
	aggr := t.aggregatePersist(batch)
	bg := context.Background()
	for key, delta := range aggr {
		if err := t.repo.AddTokens(bg, key.day, key.scope, delta); err != nil {
			event.SysLogWarn("system.plugin.cost_guard_persist_fail", "cost_guard 写库失败，本地累加与远端可能漂移",
				event.P("scope", key.scope), event.P("day", key.day), event.P("delta", delta), event.P("error", err.Error()))
		}
	}
}

type persistAggrKey struct {
	day   string
	scope string
}

func (t *CostGuardBudgetTracker) aggregatePersist(batch []costGuardPersistEntry) map[persistAggrKey]int {
	m := make(map[persistAggrKey]int, len(batch))
	for _, e := range batch {
		k := persistAggrKey{day: e.day, scope: e.scope}
		m[k] += e.delta
	}
	return m
}

func (t *CostGuardBudgetTracker) ensureDayLocked() {
	day := time.Now().UTC().Format("2006-01-02")
	if t.day == day {
		return
	}
	repo := t.repo
	scope := t.scopeKey
	t.day = day
	t.tokens = 0
	t.mu.Unlock()
	var loaded int
	if repo != nil {
		if n, err := repo.GetTokens(context.Background(), day, scope); err == nil {
			loaded = n
		} else {
			event.SysLogWarn("system.plugin.cost_guard_load_fail", "cost_guard 读取日用量失败，从 0 开始计数",
				event.P("scope", scope), event.P("day", day), event.P("error", err.Error()))
		}
	}
	t.mu.Lock()
	if t.day == day {
		t.tokens = loaded
	}
}

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

// TECH-DEBT: framework-internal-access — directly accesses inv.Session.EventMu/Events.
// Should use a framework public API for session token stats when available.
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
