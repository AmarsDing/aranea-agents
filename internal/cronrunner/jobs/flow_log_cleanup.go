package jobs

import (
	"context"
	"os"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"

	"github.com/go-kratos/kratos/v2/log"
)

// FlowLogCleanup periodically purges old flow_log_events rows.
type FlowLogCleanup struct {
	interval time.Duration
	flowLogs *biz.FlowLogUsecase
	log      *log.Helper
}

func NewFlowLogCleanup(interval time.Duration, flowLogs *biz.FlowLogUsecase, logger log.Logger) *FlowLogCleanup {
	if interval <= 0 {
		interval = time.Hour
	}
	return &FlowLogCleanup{
		interval: interval,
		flowLogs: flowLogs,
		log:      log.NewHelper(logger),
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
			event.SysLogWarn("flow_log.cleanup", "流程日志 TTL 清理失败", event.P("error", err))
			if w.log != nil {
				w.log.Warnf("flow log cleanup: %v", err)
			}
			return
		}
		if n > 0 && w.log != nil {
			w.log.Infof("flow log cleanup: removed %d rows", n)
		}
	})
}

func FlowLogCleanupDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("FLOW_LOG_CLEANUP_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
