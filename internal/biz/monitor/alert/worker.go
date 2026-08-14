package alert

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

// AlertEvalWorker periodically evaluates alert rules via the Engine.
// DEV-05: the worker depends on *Engine (not the root monitor.Usecase),
// which eliminates the historical circular dependency and the
// Usecase.SetEvalWorker setter injection.
type AlertEvalWorker struct {
	engine   *Engine
	interval time.Duration
	buffer   *MetricRingBuffer
	sf       singleflight.Group
	ready    atomic.Bool
	lg       loggateway.Logger
}

func NewAlertEvalWorker(engine *Engine, buffer *MetricRingBuffer, lg loggateway.Logger) *AlertEvalWorker {
	if engine == nil {
		return nil
	}
	interval := evalIntervalFromEnv()
	return &AlertEvalWorker{
		engine:   engine,
		interval: interval,
		buffer:   buffer,
		lg:       lg,
	}
}

func (w *AlertEvalWorker) Start(ctx context.Context) {
	if w == nil || w.engine == nil {
		return
	}
	w.rebuildFromDB(ctx)
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.evaluate(ctx)
		}
	}
}

func (w *AlertEvalWorker) rebuildFromDB(ctx context.Context) {
	safego.Go(ctx, "monitor.alert-eval-rebuild", func() {
		rebuilt := w.engine.RebuildRingBuffer(ctx, w.buffer)
		if rebuilt > 0 {
			w.ready.Store(true)
			w.lg.Info("AlertEvalWorker: ring buffer rebuilt from DB",
				loggateway.StepID("monitor.alert_eval_rebuild_ok"),
				loggateway.Int("buckets", rebuilt))
		} else {
			// 0 buckets rebuilt — DB may be empty on first run.
			// Mark ready anyway so incremental updates can flow in;
			// the evaluator will simply see zero counts until data arrives.
			w.ready.Store(true)
			w.lg.Warn("AlertEvalWorker: RebuildRingBuffer rebuilt 0 buckets, will rely on incremental updates",
				loggateway.StepID("monitor.alert_eval_rebuild_empty"))
		}
	})
}

func (w *AlertEvalWorker) evaluate(ctx context.Context) {
	if !w.ready.Load() {
		return
	}
	_, _, _ = w.sf.Do("eval", func() (any, error) {
		w.engine.EvaluateAlerts(ctx)
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

// RestartEvalWorker triggers a rebuild from DB to recover the worker from a stalled state.
func (w *AlertEvalWorker) RestartEvalWorker(ctx context.Context) {
	if w == nil {
		return
	}
	w.ready.Store(false)
	w.rebuildFromDB(ctx)
}
