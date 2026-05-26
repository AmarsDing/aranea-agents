package plugintrpc

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
)

const (
	hookRetryPollInterval = 60 * time.Second
	hookRetryStaleAfter   = 5 * time.Minute
	hookRetryBatchSize    = 20
	hookRetryQueryTimeout = 30 * time.Second
)

// HookDeliveryRetryWorker polls for stale pending hook deliveries and retries
// them. It handles the case where the process crashed after persisting a
// delivery but before completing the in-process delivery attempt (OUT-02 / HK-01).
//
// Multi-instance safety: before retrying each delivery, TryClaimForRetry
// atomically increments attempt_count WHERE attempt_count = currentCount.
// Only the pod that wins the optimistic lock proceeds with delivery.
type HookDeliveryRetryWorker struct {
	repo     biz.HookDeliveryRepo
	notifier *HookNotifier
	stop     chan struct{}
}

// NewHookDeliveryRetryWorker creates a retry worker. notifier must be the same
// HookNotifier used for new deliveries so that retry logic is identical.
func NewHookDeliveryRetryWorker(repo biz.HookDeliveryRepo, notifier *HookNotifier) *HookDeliveryRetryWorker {
	return &HookDeliveryRetryWorker{
		repo:     repo,
		notifier: notifier,
		stop:     make(chan struct{}),
	}
}

// Start launches the background polling loop. Call Stop to shut it down cleanly.
func (w *HookDeliveryRetryWorker) Start() {
	safego.Go(context.Background(), "hook.delivery.retry_worker", w.loop)
}

// Stop signals the polling loop to exit.
func (w *HookDeliveryRetryWorker) Stop() {
	if w == nil {
		return
	}
	select {
	case w.stop <- struct{}{}:
	default:
	}
}

func (w *HookDeliveryRetryWorker) loop() {
	ticker := time.NewTicker(hookRetryPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-w.stop:
			return
		case <-ticker.C:
			w.retryStale()
		}
	}
}

func (w *HookDeliveryRetryWorker) retryStale() {
	ctx, cancel := context.WithTimeout(context.Background(), hookRetryQueryTimeout)
	defer cancel()

	stale, err := w.repo.ListStalePending(ctx, time.Now().UTC().Add(-hookRetryStaleAfter), hookRetryBatchSize)
	if err != nil {
		event.SysLogWarn("system.hook.reload_fail", "hook.delivery.retry_worker: ListStalePending failed", event.P("error", err.Error()))
		return
	}
	for _, d := range stale {
		w.retryOne(d)
	}
}

func (w *HookDeliveryRetryWorker) retryOne(d biz.HookDelivery) {
	if w.notifier == nil {
		return
	}

	// Optimistic claim: only proceed if we atomically advance attempt_count.
	// This prevents duplicate delivery when multiple pods race to retry the same record.
	ctx, cancel := context.WithTimeout(context.Background(), hookRetryQueryTimeout)
	defer cancel()

	claimed, err := w.repo.TryClaimForRetry(ctx, d.ID, d.AttemptCount)
	if err != nil {
		event.SysLogWarn("system.hook.reload_fail", "hook.delivery.retry_worker: TryClaimForRetry failed",
			event.P("id", d.ID), event.P("error", err.Error()))
		return
	}
	if !claimed {
		return // another instance already claimed this delivery
	}

	event.SysLogInfo("system.hook.reload_fail", "hook.delivery.retry_worker: retrying stale delivery",
		event.P("id", d.ID),
		event.P("hook_key", d.HookKey),
		event.P("attempt_count", d.AttemptCount+1),
		event.P("max_attempts", d.MaxAttempts),
	)

	// TryClaimForRetry already incremented attempt_count by 1, so the next
	// attempt to record is d.AttemptCount+2 (the first new HTTP attempt).
	w.notifier.processDeliveryFrom(context.Background(), d, 0, d.AttemptCount+2)
}
