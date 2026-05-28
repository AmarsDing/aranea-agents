package monitor

import (
	"context"
	"os"
	"strings"
	"time"

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
	ready    bool
}

func NewAlertEvalWorker(uc *Usecase, buffer *MetricRingBuffer) *AlertEvalWorker {
	if uc == nil {
		return nil
	}
	interval := evalIntervalFromEnv()
	return &AlertEvalWorker{
		usecase:  uc,
		interval: interval,
		buffer:   buffer,
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
		w.usecase.RebuildRingBuffer(ctx, w.buffer)
		w.ready = true
	})
}

func (w *AlertEvalWorker) evaluate(ctx context.Context) {
	if !w.ready {
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
	return w != nil && w.ready
}
