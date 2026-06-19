package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/session"
	"aranea-agents/internal/memory"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

const (
	memorySleepTimeDefaultInterval = 1 * time.Hour
)

// memorySleepTimeRetryBackoff is the 3-step backoff schedule for the
// sleep-time consolidation job: 1s, 2s, 4s. This is much shorter than the
// default (30s/2m/10m) because consolidation jobs are queue-driven and
// failures are typically transient (DB busy, vector store timeout).
// The circuit breaker prevents retry storms when the backend is down.
var memorySleepTimeRetryBackoff = []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

// SleepTimeTargetLister enumerates (agent, user) pairs whose memories should
// be consolidated by the Sleep-time Agent. Implementations typically scan
// agents with memory enabled and their active users.
type SleepTimeTargetLister interface {
	ListConsolidationTargets(ctx context.Context) ([]trpcmemory.UserKey, error)
}

// MemorySleepTimeWorker runs the Sleep-time Agent periodically. It starts the
// SleepTimeService queue worker (consumer) and, when a target lister is
// provided, periodically enqueues consolidation jobs for each target (producer).
//
// When no target lister is wired, the worker only drains the queue —
// consolidation jobs are then expected to be enqueued by other parts of the
// system (e.g. after a session ends).
type MemorySleepTimeWorker struct {
	interval time.Duration
	service  *memory.SleepTimeService
	lister   SleepTimeTargetLister
	runner   *JobRunner // unified Job framework (retry + dead letter)
	lg       loggateway.Logger
}

// NewMemorySleepTimeWorker creates a MemorySleepTimeWorker.
//
// Parameters:
//   - interval: tick interval. Falls back to 1h when <= 0.
//   - service:  the SleepTimeService whose queue is drained and whose
//     Consolidate method is used.
//   - lister:   optional target lister. When nil, the worker only drains the
//     queue.
//   - lg:       logger.
func NewMemorySleepTimeWorker(interval time.Duration, service *memory.SleepTimeService, lister SleepTimeTargetLister, lg loggateway.Logger) *MemorySleepTimeWorker {
	if interval <= 0 {
		interval = memorySleepTimeDefaultInterval
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &MemorySleepTimeWorker{
		interval: interval,
		service:  service,
		lister:   lister,
		runner:   NewJobRunner(lg),
		lg:       lg,
	}
}

// WithCircuitBreaker attaches a circuit breaker to the worker's job runner.
// When the circuit is open, consolidation jobs are rejected immediately
// instead of being retried. Returns the receiver for chaining.
func (w *MemorySleepTimeWorker) WithCircuitBreaker(cb *CircuitBreaker) *MemorySleepTimeWorker {
	if w != nil && w.runner != nil {
		w.runner.WithCircuitBreaker(cb)
	}
	return w
}

// WithDeadLetter attaches a dead-letter writer to the worker's job runner.
// When a consolidation job exhausts all retries, it is written to the
// dead-letter sink for later replay. Returns the receiver for chaining.
func (w *MemorySleepTimeWorker) WithDeadLetter(dl DeadLetterWriter) *MemorySleepTimeWorker {
	if w != nil && w.runner != nil {
		w.runner.WithDeadLetter(dl)
	}
	return w
}

// Start blocks until ctx is cancelled. It launches the queue worker goroutine
// and ticks the enqueue loop at the configured interval.
//
// T3.1: The queue worker drains the consolidation queue via
// SleepTimeService.QueueChan() and wraps each job in a JobRunner for retry
// (3 attempts with 30s/2m/10m backoff), panic recovery, and dead-letter
// metrics. Only read failures are retried — mutation failures are treated
// as graceful degradation inside Consolidate (non-idempotent operations
// must not be retried).
func (w *MemorySleepTimeWorker) Start(ctx context.Context) {
	if w == nil || w.service == nil {
		return
	}

	// Launch the queue worker (consumer) in a managed goroutine.
	// T3.1: drain the queue ourselves (instead of calling service.Start) so
	// each job can be wrapped in a JobRunner for retry + dead-letter metrics.
	safego.Go(ctx, "memory.sleep_time.worker", func() {
		ch := w.service.QueueChan()
		if ch == nil {
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case req := <-ch:
				w.processJobWithRetry(ctx, req)
			}
		}
	})

	// If no lister is wired, the worker only drains the queue.
	if w.lister == nil {
		w.lg.Info("sleep-time worker started (queue-only mode, no target lister)")
		<-ctx.Done()
		return
	}

	w.lg.Info("sleep-time worker started", loggateway.Any("interval", w.interval.String()))

	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	// Run an initial tick immediately so consolidation starts without waiting
	// for the first interval.
	w.runOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.runOnce(ctx)
		}
	}
}

// runOnce lists consolidation targets and enqueues a job for each. The actual
// consolidation happens asynchronously in the queue worker goroutine.
func (w *MemorySleepTimeWorker) runOnce(ctx context.Context) {
	safego.Go(ctx, "memory.sleep_time.enqueue", func() {
		targets, err := w.lister.ListConsolidationTargets(ctx)
		if err != nil {
			w.lg.Warn("sleep-time target list failed", loggateway.Err(err))
			return
		}
		if len(targets) == 0 {
			return
		}
		enqueued := 0
		for _, uk := range targets {
			if uk.AppName == "" || uk.UserID == "" {
				continue
			}
			if err := w.service.EnqueueConsolidationJob(ctx, uk); err != nil {
				w.lg.Warn("sleep-time enqueue failed",
					loggateway.Str("app", uk.AppName),
					loggateway.Str("user", uk.UserID),
					loggateway.Err(err))
				continue
			}
			enqueued++
		}
		if enqueued > 0 {
			w.lg.Info("sleep-time enqueued consolidation jobs",
				loggateway.Int("count", enqueued),
				loggateway.Int("targets", len(targets)))
		}
	})
}

// processJobWithRetry wraps a single consolidation job in the unified JobRunner
// for retry (read failures only), panic recovery, and dead-letter metrics.
//
// Retry safety: Consolidate returns an error ONLY on memory read failure
// (idempotent — safe to retry). Mutation failures are treated as graceful
// degradation inside Consolidate (return nil) because merge/reflect/update_core
// operations are NOT idempotent — retrying them could add duplicate memories.
//
// Backoff: uses memorySleepTimeRetryBackoff (1s/2s/4s) instead of the default
// (30s/2m/10m) because queue-driven jobs benefit from fast retries. The
// circuit breaker (when attached) prevents retry storms when the backend is
// down.
func (w *MemorySleepTimeWorker) processJobWithRetry(ctx context.Context, req memory.ConsolidationJobRequest) {
	// JobRunner.Run handles all error logging and dead-letter metrics internally
	// (warn on each retry, error on exhausted retries, jobDeadTotal counter).
	// The returned error is therefore intentionally ignored — there is nothing
	// the caller can do beyond what JobRunner already does.
	_ = w.runner.Run(ctx, JobConfig{
		JobID:      "memory_sleep_time",
		MaxRetries: DefaultJobMaxRetries,
		Backoff:    memorySleepTimeRetryBackoff,
	}, func(ctx context.Context) error {
		return w.service.Consolidate(ctx, req.UserKey)
	})
}

// MemorySleepTimeDisabled reports whether the Sleep-time worker is disabled via
// the MEMORY_SLEEP_TIME_DISABLED environment variable.
func MemorySleepTimeDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("MEMORY_SLEEP_TIME_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}

// AgentUserKeyLister adapts biz.AgentUsecase to the SleepTimeTargetLister
// interface. It lists agents with L3 facts enabled and pairs each agent with
// the provided user IDs. When no user IDs are configured, it returns an empty
// list (the caller is expected to wire a richer lister in production).
type AgentUserKeyLister struct {
	agents  *biz.AgentUsecase
	userIDs []string
}

// NewAgentUserKeyLister creates an AgentUserKeyLister.
func NewAgentUserKeyLister(agents *biz.AgentUsecase, userIDs []string) *AgentUserKeyLister {
	return &AgentUserKeyLister{agents: agents, userIDs: userIDs}
}

// ListConsolidationTargets returns (agent, user) pairs for agents with L3
// facts enabled, cross-joined with the configured user IDs.
func (l *AgentUserKeyLister) ListConsolidationTargets(ctx context.Context) ([]trpcmemory.UserKey, error) {
	if l == nil || l.agents == nil || len(l.userIDs) == 0 {
		return nil, nil
	}
	targets, err := l.agents.ListMemoryMaintenanceTargets(ctx)
	if err != nil {
		return nil, err
	}
	var out []trpcmemory.UserKey
	for _, t := range targets {
		if !t.WriteL3Facts {
			continue
		}
		for _, uid := range l.userIDs {
			uid = strings.TrimSpace(uid)
			if uid == "" {
				continue
			}
			out = append(out, trpcmemory.UserKey{AppName: t.AgentID, UserID: uid})
		}
	}
	return out, nil
}

// memorySleepTimeSessionLookbackDays is the lookback window (in days) for
// deriving consolidation targets from session activity. Sessions that had no
// activity (last_message_at or last_run_at) within this window are excluded.
const memorySleepTimeSessionLookbackDays = 7

// AgentUserKeyListerFromSession derives Sleep-time consolidation targets from
// real session activity instead of env-var configuration. It enumerates
// distinct (agent_id, user_id) pairs from sessions active within the last
// memorySleepTimeSessionLookbackDays days, then filters to agents with L3
// facts enabled.
//
// This replaces the env-var-driven AgentUserKeyLister in production: instead
// of requiring operators to set MEMORY_SLEEP_TIME_USER_IDS, the worker
// automatically discovers which users need consolidation based on who has
// been chatting recently.
type AgentUserKeyListerFromSession struct {
	agents   *biz.AgentUsecase
	sessions session.SessionReader
}

// NewAgentUserKeyListerFromSession creates an AgentUserKeyListerFromSession.
// The sessions reader is used to enumerate active (agent, user) pairs; the
// agents usecase filters them to L3-enabled agents.
func NewAgentUserKeyListerFromSession(agents *biz.AgentUsecase, sessions session.SessionReader) *AgentUserKeyListerFromSession {
	return &AgentUserKeyListerFromSession{agents: agents, sessions: sessions}
}

// Compile-time interface checks.
var (
	_ SleepTimeTargetLister = (*AgentUserKeyLister)(nil)
	_ SleepTimeTargetLister = (*AgentUserKeyListerFromSession)(nil)
)

// ListConsolidationTargets returns (agent, user) pairs derived from recent
// session activity, filtered to agents with L3 facts enabled.
func (l *AgentUserKeyListerFromSession) ListConsolidationTargets(ctx context.Context) ([]trpcmemory.UserKey, error) {
	if l == nil || l.agents == nil || l.sessions == nil {
		return nil, nil
	}
	// 1. Enumerate agents with L3 facts enabled → build a set for O(1) lookup.
	targets, err := l.agents.ListMemoryMaintenanceTargets(ctx)
	if err != nil {
		return nil, err
	}
	l3Agents := make(map[string]struct{}, len(targets))
	for _, t := range targets {
		if t.WriteL3Facts && t.AgentID != "" {
			l3Agents[t.AgentID] = struct{}{}
		}
	}
	if len(l3Agents) == 0 {
		return nil, nil
	}
	// 2. Derive distinct (agent, user) pairs from sessions active in the last
	//    7 days.
	pairs, err := l.sessions.ListActiveAgentUserKeys(ctx, memorySleepTimeSessionLookbackDays)
	if err != nil {
		return nil, err
	}
	// 3. Filter to L3-enabled agents and de-duplicate (the SQL DISTINCT already
	//    de-duplicates, but we guard against future changes).
	seen := make(map[string]struct{}, len(pairs))
	var out []trpcmemory.UserKey
	for _, p := range pairs {
		if p.AgentID == "" || p.UserID == "" {
			continue
		}
		if _, ok := l3Agents[p.AgentID]; !ok {
			continue
		}
		key := p.AgentID + "\x00" + p.UserID
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trpcmemory.UserKey{AppName: p.AgentID, UserID: p.UserID})
	}
	return out, nil
}
