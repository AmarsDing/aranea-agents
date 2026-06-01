package monitor

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"golang.org/x/sync/singleflight"
)

const defaultEvalInterval = 30 * time.Second

func evalIntervalFromEnv() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("MONITOR_ALERT_EVAL_INTERVAL")); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d >= 5*time.Second {
			return d
		}
	}
	return defaultEvalInterval
}

type AlertEvalWorker struct {
	usecase  *Usecase
	interval time.Duration
	buffer   *MetricRingBuffer
	sf       singleflight.Group
	ready    atomic.Bool
	lg       loggateway.Logger
}

func NewAlertEvalWorker(uc *Usecase, buffer *MetricRingBuffer, lg loggateway.Logger) *AlertEvalWorker {
	if uc == nil {
		return nil
	}
	interval := evalIntervalFromEnv()
	return &AlertEvalWorker{
		usecase:  uc,
		interval: interval,
		buffer:   buffer,
		lg:       lg,
	}
}

func (w *AlertEvalWorker) Start(ctx context.Context) {
	if w == nil || w.usecase == nil {
		return
	}
	w.rebuildFromDB(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	cleanupTicker := time.NewTicker(time.Hour)
	defer cleanupTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.evaluate(ctx)
		case <-cleanupTicker.C:
			w.usecase.CleanupStaleLastFired(time.Now(), 24*time.Hour)
		}
	}
}

func (w *AlertEvalWorker) rebuildFromDB(ctx context.Context) {
	safego.Go(ctx, "monitor.alert-eval-rebuild", func() {
		rebuilt := w.usecase.RebuildRingBuffer(ctx, w.buffer)
		if rebuilt > 0 {
			w.ready.Store(true)
		} else {
			w.lg.Warn("AlertEvalWorker: RebuildRingBuffer rebuilt 0 buckets, will retry on next tick", loggateway.StepID("monitor.alert_eval_rebuild_fail"))
			w.ready.Store(true)
		}
	})
}

func (w *AlertEvalWorker) evaluate(ctx context.Context) {
	if !w.ready.Load() {
		return
	}
	_, _, _ = w.sf.Do("eval", func() (interface{}, error) {
		w.usecase.EvaluateAlerts(ctx)
		return nil, nil
	})
}

func (w *AlertEvalWorker) OnCompletion(status string, durationMs int64) {
	if w == nil || w.buffer == nil {
		return
	}
	w.buffer.RecordCompletion(status, durationMs)
}

func (w *AlertEvalWorker) Ready() bool {
	return w != nil && w.ready.Load()
}
