package jobs

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// HealthCheckRunner abstracts the periodic health-check entry point so the
// scanners can be tested without a concrete usecase.
// *biz.ChannelUsecase and *biz.LlmProviderModelUsecase both satisfy it.
type HealthCheckRunner interface {
	RunHealthChecks(ctx context.Context) error
}

// ChannelHealthScanner periodically re-evaluates enabled channel configuration.
type ChannelHealthScanner struct {
	interval time.Duration
	uc       HealthCheckRunner
	lg       loggateway.Logger
	flowLog  biz.FlowLogWriter
}

// NewChannelHealthScanner creates a scanner. Pass interval ≤0 for 10 minutes default.
func NewChannelHealthScanner(interval time.Duration, uc HealthCheckRunner, lg loggateway.Logger, flowLog biz.FlowLogWriter) *ChannelHealthScanner {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	return &ChannelHealthScanner{interval: interval, uc: uc, lg: lg, flowLog: flowLog}
}

// Start blocks until ctx is cancelled.
func (w *ChannelHealthScanner) Start(ctx context.Context) {
	if w == nil || w.uc == nil {
		return
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
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

func (w *ChannelHealthScanner) runOnce(ctx context.Context) {
	safego.Go(ctx, "channel.health", func() {
		w.processOnce(ctx)
	})
}

func (w *ChannelHealthScanner) processOnce(ctx context.Context) {
	if err := w.uc.RunHealthChecks(ctx); err != nil {
		w.lg.Warn("channel health check failed",
			loggateway.StepID("system.channel_health.failed"),
			loggateway.Err(err))
		if w.flowLog != nil {
			w.flowLog.LogFlowError(context.Background(), "", "system.channel_health.failed",
				"渠道健康检查失败", biz.LogPair{Key: "error", Value: err.Error()})
		}
	}
}
