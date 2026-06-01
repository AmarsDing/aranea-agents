package monitor

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

func (u *Usecase) RecordAdminAudit(ctx context.Context, action, resource, resourceID, detail string) {
	if u == nil {
		return
	}
	actor := ""
	if p, ok := auth.FromContext(ctx); ok && p != nil {
		actor = fmt.Sprintf("%d", p.UserID)
	}
	if err := u.RecordAuditLog(ctx, AuditLog{
		Action:     strings.TrimSpace(action),
		Resource:   strings.TrimSpace(resource),
		ResourceID: strings.TrimSpace(resourceID),
		Detail:     detail,
		Actor:      actor,
		Severity:   "info",
	}); err != nil {
		u.lg.Warn("RecordAdminAudit failed", loggateway.StepID("monitor.audit_log_fail"), loggateway.Str("action", action), loggateway.Err(err))
	}
}
