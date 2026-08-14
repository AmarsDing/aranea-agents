package plugintrpc

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
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
	hookConf conf.RuntimeHookConfig
	stop     chan struct{}
	done     chan struct{} // loop 退出时关闭（N-B3：Stop 后可等待，回归测试可观测）
	// N-B3: once 守卫 + close(stop) 广播语义。此前 Stop 用无缓冲 channel
	// 非阻塞发送，loop 正在 retryStale（可达 RetryQueryTimeout）时信号必丢，
	// worker 永久存活。close 无丢失窗口且天然幂等。
	startOnce sync.Once
	stopOnce  sync.Once
	started   atomic.Bool
}

// NewHookDeliveryRetryWorker creates a retry worker. notifier must be the same
// HookNotifier used for new deliveries so that retry logic is identical.
// // WIRE: needs *conf.Runtime
func NewHookDeliveryRetryWorker(runtimeConf *conf.Runtime, repo biz.HookDeliveryRepo, notifier *HookNotifier, lg loggateway.Logger) *HookDeliveryRetryWorker {
	return &HookDeliveryRetryWorker{
		repo:     repo,
		notifier: notifier,
		lg:       lg,
		hookConf: runtimeConf.HookConfig(),
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start launches the background polling loop. Call Stop to shut it down cleanly.
func (w *HookDeliveryRetryWorker) Start() {
	w.startOnce.Do(func() {
		w.started.Store(true)
		safego.Go(appctx.Ctx(), "hook.delivery.retry_worker", w.loop)
	})
}

// Stop signals the polling loop to exit. Safe to call multiple times; the
// signal is never lost even while the loop is inside retryStale.
func (w *HookDeliveryRetryWorker) Stop() {
	if w == nil {
		return
	}
	w.stopOnce.Do(func() {
		close(w.stop)
	})
}

// Wait blocks until the loop exits or timeout elapses. Returns true when the
// loop has exited (or was never started). Used by Runtime.Close for bounded
// graceful shutdown.
func (w *HookDeliveryRetryWorker) Wait(timeout time.Duration) bool {
	if w == nil || !w.started.Load() {
		return true
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-w.done:
		return true
	case <-timer.C:
		return false
	}
}

func (w *HookDeliveryRetryWorker) loop() {
	defer close(w.done)
	ticker := time.NewTicker(w.hookConf.RetryPollInterval)
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
	ctx, cancel := context.WithTimeout(context.Background(), w.hookConf.RetryQueryTimeout)
	defer cancel()

	stale, err := w.repo.ListStalePending(ctx, time.Now().UTC().Add(-w.hookConf.RetryStaleAfter), int(w.hookConf.RetryBatchSize))
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
	ctx, cancel := context.WithTimeout(context.Background(), w.hookConf.RetryQueryTimeout)
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
