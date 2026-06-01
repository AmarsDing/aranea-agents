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

// ToolAuditCleanup periodically purges tool_invocation_audit rows older than retention policy.
type ToolAuditCleanup struct {
	interval time.Duration
	tools    *biz.ToolUsecase
	lg       loggateway.Logger
}

func NewToolAuditCleanup(interval time.Duration, tools *biz.ToolUsecase, lg loggateway.Logger) *ToolAuditCleanup {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	return &ToolAuditCleanup{interval: interval, tools: tools, lg: lg}
}

func (w *ToolAuditCleanup) Start(ctx context.Context) {
	if w == nil || w.tools == nil {
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

func (w *ToolAuditCleanup) runOnce(ctx context.Context) {
	safego.Go(ctx, "tool_audit.cleanup", func() {
		n, err := w.tools.PurgeOldInvocationAudits(context.Background())
		if err != nil {
			w.lg.Warn("tool audit cleanup failed", loggateway.Err(err))
			return
		}
		if n > 0 {
			w.lg.Info("tool audit cleanup", loggateway.Int("removed", int(n)))
		}
	})
}

func ToolAuditCleanupDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("TOOL_AUDIT_CLEANUP_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
