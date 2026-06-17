package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/memory"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpcmemory "trpc.group/trpc-go/trpc-agent-go/memory"
)

const (
	memorySleepTimeDefaultInterval = 1 * time.Hour
)

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
		lg:       lg,
	}
}

// Start blocks until ctx is cancelled. It launches the queue worker goroutine
// and ticks the enqueue loop at the configured interval.
func (w *MemorySleepTimeWorker) Start(ctx context.Context) {
	if w == nil || w.service == nil {
		return
	}

	// Launch the queue worker (consumer) in a managed goroutine.
	safego.Go(ctx, "memory.sleep_time.worker", func() {
		w.service.Start(ctx)
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
