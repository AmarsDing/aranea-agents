package plugintrpc

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

type costGuardPersistEntry struct {
	day   string
	scope string
	delta int
}

// TECH-DEBT(BR1): CostGuardBudgetTracker 在 persistWorker 中直接调用 repo 写库，
// 未经过 EventBus 统一管道。当前已通过 channel+worker 批量异步化，
// 但应迁移到 EventBus + consumer 模式以保持架构一致性。
//
// P1-11 (fail-closed persistence): previously, when the persist channel was
// saturated or a DB write errored, the entry was silently dropped with only
// a warning log. This caused the in-memory counter (reservation) to drift
// away from the durable DB ledger — on process restart the day's counter
// would be loaded from DB missing the dropped deltas, allowing subsequent
// calls to exceed the configured daily budget (fail-open).
//
// The current model treats the in-memory counter as the optimistic
// reservation and the DB as the authoritative ledger. To keep them
// consistent:
//  1. persistAdd escalates to synchronous write when the async channel
//     is full (no silent drop).
//  2. TryConsume rolls back the in-memory reservation if both async
//     queueing and synchronous write fail (fail-closed).
//  3. flushPersist re-queues failed DB writes to a bounded retry channel
//     drained by retryWorker with backoff.
//  4. Close drains both persistCh and retryCh so shutdown does not lose
//     pending entries.
type CostGuardBudgetTracker struct {
	mu       sync.Mutex
	day      string
	tokens   int
	repo     biz.PluginCostGuardUsageRepo
	scopeKey string
	lg       loggateway.Logger

	persistCh   chan costGuardPersistEntry
	persistDone chan struct{}
	persistWg   sync.WaitGroup

	// P1-11: retry buffer for persist failures. retryWorker drains this
	// channel with backoff, re-attempting DB writes until success or
	// Close. The buffer is bounded to prevent unbounded growth under
	// persistent DB outages; overflow is logged as a last resort.
	retryCh   chan costGuardPersistEntry
	retryDone chan struct{}
	retryWg   sync.WaitGroup

	// lastUsedUnix 记录 registry 最近一次命中时间（R-2：idle 分桶淘汰依据）。
	lastUsedUnix atomic.Int64
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

	// P1-11: retry channel sizing & timing. The retry buffer is sized to
	// absorb a burst of transient DB write failures without dropping. The
	// retry interval is the polling cadence for the retry worker; the
	// sync-write timeout bounds how long TryConsume will block when
	// escalating from async to sync persistence.
	costGuardRetryChanSize   = 512
	costGuardRetryIntervalMs = 1000
	costGuardPersistSyncTO   = 3 * time.Second
)

func NewCostGuardBudgetTracker(lg loggateway.Logger, opts ...CostGuardBudgetOption) *CostGuardBudgetTracker {
	t := &CostGuardBudgetTracker{
		scopeKey:    "global",
		lg:          lg,
		persistCh:   make(chan costGuardPersistEntry, costGuardPersistChanSize),
		persistDone: make(chan struct{}),
		retryCh:     make(chan costGuardPersistEntry, costGuardRetryChanSize),
		retryDone:   make(chan struct{}),
	}
	for _, opt := range opts {
		opt(t)
	}
	t.persistWg.Add(1)
	safego.Go(appctx.Ctx(), "cost_guard_budget.persist_worker", t.persistWorker)
	t.retryWg.Add(1)
	safego.Go(appctx.Ctx(), "cost_guard_budget.retry_worker", t.retryWorker)
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
	t.mu.Lock()
	t.ensureDayLocked()
	if t.tokens+add > budget {
		t.mu.Unlock()
		return false
	}
	t.tokens += add
	day := t.day
	scope := t.scopeKey
	repo := t.repo
	t.mu.Unlock()
	if repo == nil {
		return true
	}
	// P1-11: route to async persist; on channel saturation, escalate to
	// synchronous write. If sync write also fails, roll back the in-memory
	// reservation (fail-closed) so the budget counter stays consistent with
	// the durable ledger. Without this rollback, a saturated channel plus a
	// failing DB would let subsequent calls read an inflated counter and
	// exceed the configured daily budget.
	entry := costGuardPersistEntry{day: day, scope: scope, delta: add}
	if t.tryQueuePersist(entry) {
		return true
	}
	if t.persistSync(entry) {
		return true
	}
	t.rollbackReservation(day, add)
	t.lg.Error("cost_guard TryConsume fail-closed: persist path exhausted, reservation rolled back",
		loggateway.StepID("plugin.cost_guard.try_consume_fail_closed"),
		loggateway.Str("scope", scope),
		loggateway.Str("day", day),
		loggateway.Int("delta", add))
	return false
}

// rollbackReservation undoes an in-memory TryConsume reservation after the
// persist path failed. R-4：仅在仍是同一天时回滚。若在 persist 尝试期间已
// 跨日，另一 goroutine 的 ensureDayLocked 已把 t.tokens 重置为新日计数，
// 此时回滚会误减新日额度（旧日的内存计数随日切已作废，无需回滚）。
func (t *CostGuardBudgetTracker) rollbackReservation(day string, add int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.day != day {
		return
	}
	t.tokens -= add
	if t.tokens < 0 {
		t.tokens = 0 // safety guard against concurrent rollbacks
	}
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
	if repo == nil {
		return
	}
	// AddOverBudget is the fallback-bypass path (call already allowed via
	// fallback model). Persistence here is best-effort with retry: we still
	// route through the same persist path so the ledger stays consistent,
	// but we do NOT roll back the in-memory counter on persist failure
	// because the LLM call has already been authorized.
	entry := costGuardPersistEntry{day: day, scope: scope, delta: add}
	if !t.tryQueuePersist(entry) {
		t.persistSyncOrRetry(entry)
	}
}

// tryQueuePersist attempts to enqueue entry on the async persist channel.
// Returns true on success, false when the channel is saturated (caller
// must escalate — never silently drop).
func (t *CostGuardBudgetTracker) tryQueuePersist(entry costGuardPersistEntry) bool {
	select {
	case t.persistCh <- entry:
		return true
	default:
		return false
	}
}

// persistSyncOrRetry attempts a synchronous write; on failure, queues the
// entry on the retry channel. Used by AddOverBudget (best-effort path)
// where rolling back the in-memory counter is not desired.
func (t *CostGuardBudgetTracker) persistSyncOrRetry(entry costGuardPersistEntry) {
	if t.persistSync(entry) {
		return
	}
	t.queueForRetry(entry, "persist_sync_or_retry")
}

// persistSync writes a single entry directly to the durable ledger with a
// bounded timeout. Returns true on success, false on error or timeout.
func (t *CostGuardBudgetTracker) persistSync(entry costGuardPersistEntry) bool {
	repo := t.repo
	if repo == nil {
		return true // nothing to persist; treat as success
	}
	ctx, cancel := context.WithTimeout(context.Background(), costGuardPersistSyncTO)
	defer cancel()
	if err := repo.AddTokens(ctx, entry.day, entry.scope, entry.delta); err != nil {
		t.lg.Warn("cost_guard sync persist failed",
			loggateway.StepID("plugin.cost_guard.persist_sync_fail"),
			loggateway.Str("scope", entry.scope),
			loggateway.Str("day", entry.day),
			loggateway.Int("delta", entry.delta),
			loggateway.Err(err))
		return false
	}
	return true
}

// queueForRetry enqueues entry on the retry channel. When the retry channel
// is also saturated (persistent DB outage), the entry is logged as a
// last-resort drop — this is the only remaining data-loss vector after
// P1-11 and is reserved for catastrophic DB unavailability.
func (t *CostGuardBudgetTracker) queueForRetry(entry costGuardPersistEntry, source string) {
	select {
	case t.retryCh <- entry:
	default:
		// Retry buffer saturated — log loudly. This indicates a sustained
		// DB outage that has exhausted both the persist (256) and retry
		// (512) buffers; operators should treat this as a P1 alert.
		t.lg.Error("cost_guard retry buffer saturated, entry dropped",
			loggateway.StepID("plugin.cost_guard.retry_drop"),
			loggateway.Str("source", source),
			loggateway.Str("scope", entry.scope),
			loggateway.Str("day", entry.day),
			loggateway.Int("delta", entry.delta))
	}
}

func (t *CostGuardBudgetTracker) Close() {
	if t == nil {
		return
	}
	close(t.persistDone)
	t.persistWg.Wait()
	close(t.retryDone)
	t.retryWg.Wait()
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
	repo := t.repo
	if repo == nil {
		return
	}
	aggr := t.aggregatePersist(batch)
	bg := context.Background()
	for key, delta := range aggr {
		if err := repo.AddTokens(bg, key.day, key.scope, delta); err != nil {
			// P1-11: do not silently drop. Re-queue on retry channel so
			// retryWorker can re-attempt with backoff. This keeps the
			// durable ledger consistent with the in-memory counter
			// across transient DB outages.
			t.lg.Warn("cost_guard persist write failed, queueing for retry",
				loggateway.StepID("plugin.cost_guard.persist_fail"),
				loggateway.Str("scope", key.scope),
				loggateway.Str("day", key.day),
				loggateway.Int("delta", delta),
				loggateway.Err(err))
			t.queueForRetry(costGuardPersistEntry{day: key.day, scope: key.scope, delta: delta}, "flush_persist")
		}
	}
}

// retryWorker drains retryCh with bounded backoff. Entries that fail again
// are re-queued; entries that succeed are dropped. On Close (retryDone
// closed), the worker drains the retry buffer synchronously before
// returning so shutdown does not lose pending entries.
func (t *CostGuardBudgetTracker) retryWorker() {
	defer t.retryWg.Done()
	ticker := time.NewTicker(time.Duration(costGuardRetryIntervalMs) * time.Millisecond)
	defer ticker.Stop()
	buf := make([]costGuardPersistEntry, 0, costGuardPersistBatch)
	for {
		select {
		case entry, ok := <-t.retryCh:
			if !ok {
				t.flushRetry(buf)
				return
			}
			buf = append(buf, entry)
			if len(buf) >= costGuardPersistBatch {
				t.flushRetry(buf)
				buf = buf[:0]
			}
		case <-ticker.C:
			if len(buf) > 0 {
				t.flushRetry(buf)
				buf = buf[:0]
			}
		case <-t.retryDone:
			t.drainRetry(buf)
			return
		}
	}
}

// drainRetry exhausts retryCh on Close, performing one final flush of any
// remaining entries. Failures during drain are logged but not re-queued
// (the worker is shutting down; re-queuing would risk an infinite loop).
func (t *CostGuardBudgetTracker) drainRetry(buf []costGuardPersistEntry) {
	for {
		select {
		case entry, ok := <-t.retryCh:
			if !ok {
				t.flushRetryFinal(buf)
				return
			}
			buf = append(buf, entry)
		default:
			t.flushRetryFinal(buf)
			return
		}
	}
}

// flushRetry attempts to persist a batch of previously-failed entries.
// Successful writes are dropped; failures are re-queued for another cycle.
func (t *CostGuardBudgetTracker) flushRetry(batch []costGuardPersistEntry) {
	if len(batch) == 0 {
		return
	}
	repo := t.repo
	if repo == nil {
		return
	}
	aggr := t.aggregatePersist(batch)
	ctx, cancel := context.WithTimeout(context.Background(), costGuardPersistSyncTO)
	defer cancel()
	for key, delta := range aggr {
		if err := repo.AddTokens(ctx, key.day, key.scope, delta); err != nil {
			t.lg.Warn("cost_guard retry write failed, will retry again",
				loggateway.StepID("plugin.cost_guard.retry_fail"),
				loggateway.Str("scope", key.scope),
				loggateway.Str("day", key.day),
				loggateway.Int("delta", delta),
				loggateway.Err(err))
			// Re-queue for next retry cycle. Non-blocking send; if the
			// retry buffer is full (sustained DB outage), the entry is
			// dropped via queueForRetry's last-resort logging path.
			t.queueForRetry(costGuardPersistEntry{day: key.day, scope: key.scope, delta: delta}, "flush_retry_requeue")
		}
	}
}

// flushRetryFinal is the shutdown-path variant: it does not re-queue on
// failure (worker is closing). Failed entries are logged with error level
// so operators can reconcile the durable ledger manually if needed.
func (t *CostGuardBudgetTracker) flushRetryFinal(batch []costGuardPersistEntry) {
	if len(batch) == 0 {
		return
	}
	repo := t.repo
	if repo == nil {
		return
	}
	aggr := t.aggregatePersist(batch)
	ctx, cancel := context.WithTimeout(context.Background(), costGuardPersistSyncTO)
	defer cancel()
	for key, delta := range aggr {
		if err := repo.AddTokens(ctx, key.day, key.scope, delta); err != nil {
			t.lg.Error("cost_guard final retry write failed on Close, ledger may drift",
				loggateway.StepID("plugin.cost_guard.retry_final_fail"),
				loggateway.Str("scope", key.scope),
				loggateway.Str("day", key.day),
				loggateway.Int("delta", delta),
				loggateway.Err(err))
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

// ensureDayLocked must be called while holding t.mu. It detects day rollover
// and loads the persisted daily usage from the repo. The DB read happens
// in-lock: this is intentional because (a) it only occurs once per day per
// scope, and (b) releasing the lock mid-load would let concurrent TryConsume
// callers see t.tokens=0 and bypass the daily budget (CVE-style race).
func (t *CostGuardBudgetTracker) ensureDayLocked() {
	day := time.Now().UTC().Format("2006-01-02")
	if t.day == day {
		return
	}
	repo := t.repo
	scope := t.scopeKey
	t.day = day
	t.tokens = 0
	if repo != nil {
		if n, err := repo.GetTokens(context.Background(), day, scope); err == nil {
			t.tokens = n
		} else {
			t.lg.Warn("cost_guard daily usage load failed, starting from zero",
				loggateway.StepID("plugin.cost_guard.load_fail"),
				loggateway.Str("scope", scope),
				loggateway.Str("day", day),
				loggateway.Err(err))
		}
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

// TECH-DEBT(BR14): framework-internal-access — directly accesses inv.Session.EventMu/Events.
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
