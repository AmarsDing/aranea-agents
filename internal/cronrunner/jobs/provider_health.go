package jobs

import (
	"context"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// ProviderHealthScanner periodically checks LLM provider endpoint reachability.
type ProviderHealthScanner struct {
	interval time.Duration
	uc       HealthCheckRunner
	lg       loggateway.Logger
	flowLog  biz.FlowLogWriter
}

// NewProviderHealthScanner creates a scanner. Pass interval ≤0 for 5 minutes default.
func NewProviderHealthScanner(interval time.Duration, uc HealthCheckRunner, lg loggateway.Logger, flowLog biz.FlowLogWriter) *ProviderHealthScanner {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &ProviderHealthScanner{interval: interval, uc: uc, lg: lg, flowLog: flowLog}
}

// Start blocks until ctx is cancelled.
func (w *ProviderHealthScanner) Start(ctx context.Context) {
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

func (w *ProviderHealthScanner) runOnce(ctx context.Context) {
	safego.Go(ctx, "provider.health", func() {
		w.processOnce(ctx)
	})
}

func (w *ProviderHealthScanner) processOnce(ctx context.Context) {
	if err := w.uc.RunHealthChecks(ctx); err != nil {
		w.lg.Warn("provider health check failed",
			loggateway.StepID("system.provider_health.failed"),
			loggateway.Err(err))
		if w.flowLog != nil {
			w.flowLog.LogFlowError(context.Background(), "", "system.provider_health.failed",
				"模型供应商健康检查失败", biz.LogPair{Key: "error", Value: err.Error()})
		}
	}
}
