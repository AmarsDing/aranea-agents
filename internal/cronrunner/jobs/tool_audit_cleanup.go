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

// ToolAuditCleanup periodically purges tool_invocation_audit rows older than retention policy.
type ToolAuditCleanup struct {
	interval time.Duration
	tools    *biz.ToolUsecase
	log      *log.Helper
}

func NewToolAuditCleanup(interval time.Duration, tools *biz.ToolUsecase, logger log.Logger) *ToolAuditCleanup {
	if interval <= 0 {
		interval = 24 * time.Hour
	}
	return &ToolAuditCleanup{interval: interval, tools: tools, log: log.NewHelper(logger)}
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
			event.SysLogWarn("tool_audit.cleanup", "工具审计日志清理失败", event.P("error", err))
			if w.log != nil {
				w.log.Warnf("tool audit cleanup: %v", err)
			}
			return
		}
		if n > 0 && w.log != nil {
			w.log.Infof("tool audit cleanup: removed %d rows", n)
		}
	})
}

func ToolAuditCleanupDisabled() bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv("TOOL_AUDIT_CLEANUP_DISABLED")))
	return raw == "1" || raw == "true" || raw == "yes"
}
