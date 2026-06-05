package plugintrpc

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
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
	lg       loggateway.Logger
	stop     chan struct{}
}

// NewHookDeliveryRetryWorker creates a retry worker. notifier must be the same
// HookNotifier used for new deliveries so that retry logic is identical.
func NewHookDeliveryRetryWorker(repo biz.HookDeliveryRepo, notifier *HookNotifier, lg loggateway.Logger) *HookDeliveryRetryWorker {
	return &HookDeliveryRetryWorker{
		repo:     repo,
		notifier: notifier,
		lg:       lg,
		stop:     make(chan struct{}),
	}
}

// Start launches the background polling loop. Call Stop to shut it down cleanly.
func (w *HookDeliveryRetryWorker) Start() {
	safego.Go(appctx.Ctx(), "hook.delivery.retry_worker", w.loop)
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
		if ctx.Err() != nil {
			w.lg.Warn("hook.delivery.retry_worker: ListStalePending context timeout",
				loggateway.StepID("plugin.hook.delivery_retry"),
				loggateway.Err(ctx.Err()))
			return
		}
		w.lg.Warn("hook.delivery.retry_worker: ListStalePending failed", loggateway.StepID("plugin.hook.delivery_retry"), loggateway.Err(err))
		return
	}
	if len(stale) == 0 {
		return
	}
	w.lg.Info("hook.delivery.retry_worker: found stale deliveries",
		loggateway.StepID("plugin.hook.delivery_retry"),
		loggateway.Int("stale_count", len(stale)))
	retried := 0
	for _, d := range stale {
		select {
		case <-w.stop:
			w.lg.Info("hook.delivery.retry_worker: stopping after stop signal",
				loggateway.StepID("plugin.hook.delivery_retry"),
				loggateway.Int("retried_count", retried),
				loggateway.Int("remaining_count", len(stale)-retried))
			return
		default:
		}
		w.retryOne(d)
		retried++
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
		w.lg.Warn("hook.delivery.retry_worker: TryClaimForRetry failed",
			loggateway.StepID("plugin.hook.delivery_retry"),
			loggateway.Str("id", d.ID),
			loggateway.Err(err),
		)
		return
	}
	if !claimed {
		return // another instance already claimed this delivery
	}

	w.lg.Info("hook.delivery.retry_worker: retrying stale delivery",
		loggateway.StepID("plugin.hook.delivery_retry"),
		loggateway.Str("id", d.ID),
		loggateway.Str("hook_key", d.HookKey),
		loggateway.Int("attempt_count", d.AttemptCount+1),
		loggateway.Int("max_attempts", d.MaxAttempts),
	)

	// TryClaimForRetry already incremented attempt_count by 1, so the next
	// attempt to record is d.AttemptCount+2 (the first new HTTP attempt).
	w.notifier.processDeliveryFrom(context.Background(), d, 0, d.AttemptCount+2)
}
