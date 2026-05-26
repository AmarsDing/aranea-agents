package biz

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

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
		slog.Warn("RecordAdminAudit failed", "action", action, "error", err)
	}
}
