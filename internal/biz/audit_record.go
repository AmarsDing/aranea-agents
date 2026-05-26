package biz

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/auth"
)

// RecordAdminAudit writes a best-effort audit_logs row for admin mutations.
func RecordAdminAudit(ctx context.Context, mon *MonitorUsecase, action, resource, resourceID, detail string) {
	if mon == nil {
		return
	}
	actor := ""
	if p, ok := auth.FromContext(ctx); ok && p != nil {
		actor = fmt.Sprintf("%d", p.UserID)
	}
	if err := mon.RecordAuditLog(ctx, AuditLog{
		Action:     strings.TrimSpace(action),
		Resource:   strings.TrimSpace(resource),
		ResourceID: strings.TrimSpace(resourceID),
		Detail:     detail,
		Actor:      actor,
		Severity:   "info",
	}); err != nil {
		event.SysLogWarn("system.monitor.audit_log_fail", "RecordAdminAudit failed", event.P("action", action), event.P("error", err.Error()))
	}
}
