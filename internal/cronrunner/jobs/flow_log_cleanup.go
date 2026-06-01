package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// FlowLogCleanup periodically purges old flow_log_events rows.
type FlowLogCleanup struct {
	interval time.Duration
	flowLogs *biz.FlowLogUsecase
	lg       loggateway.Logger
}

func NewFlowLogCleanup(interval time.Duration, flowLogs *biz.FlowLogUsecase, lg loggateway.Logger) *FlowLogCleanup {
	if interval <= 0 {
		interval = time.Hour
	}
	return &FlowLogCleanup{
		interval: interval,
		flowLogs: flowLogs,
		lg:       lg,
	}
}

func (w *FlowLogCleanup) Start(ctx context.Context) {
	if w == nil || w.flowLogs == nil {
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

func (w *FlowLogCleanup) runOnce(ctx context.Context) {
	safego.Go(ctx, "flow_log.cleanup", func() {
		n, err := w.flowLogs.PurgeExpired(ctx)
		if err != nil {
			w.lg.Warn("flow log cleanup failed", loggateway.Err(err))
			return
		}
		if n > 0 {
			w.lg.Info("flow log cleanup removed rows", loggateway.Int("count", int(n)))
		}
	})
}

func FlowLogCleanupDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("FLOW_LOG_CLEANUP_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
