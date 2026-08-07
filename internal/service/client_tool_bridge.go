package service

// client_tool_bridge.go — client tool bridge 装配适配（design 74 §6）。
//
// Bridge 本体在 internal/tools/clientbridge（依赖倒置：自带窄端口
// AuditRecorder / Router / FlowLogWriter alias）。本文件把 service 层
// 可用的协作者（monitor 审计仓、流程日志 writer）适配进 Bridge Deps。

import (
	"context"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/clientbridge"
	"aranea-agents/pkg/loggateway"
)

// ProvideClientToolBridge constructs the process-wide client tool bridge.
// audit nil → 审计跳过（best-effort）；flow nil → 流程日志跳过。
func ProvideClientToolBridge(audit biz.MonitorAuditRepo, flow biz.FlowLogWriter, lg loggateway.Logger) *clientbridge.Bridge {
	return clientbridge.NewBridge(clientbridge.Deps{
		Audit: ProvideClientToolAuditRecorder(audit, lg),
		Flow:  flow,
		LG:    lg,
	})
}

// ProvideClientToolAuditRecorder adapts the monitor audit repo to the
// clientbridge.AuditRecorder port. Nil repo → nil（Bridge 跳过审计）。
func ProvideClientToolAuditRecorder(repo biz.MonitorAuditRepo, lg loggateway.Logger) clientbridge.AuditRecorder {
	if repo == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &clientToolAuditRecorder{repo: repo, lg: lg}
}

// clientToolAuditRecorder writes one audit_logs row per invocation lifecycle
// event (invoke/result/offline/timeout), reusing the monitor audit domain.
type clientToolAuditRecorder struct {
	repo biz.MonitorAuditRepo
	lg   loggateway.Logger
}

var _ clientbridge.AuditRecorder = (*clientToolAuditRecorder)(nil)

func (r *clientToolAuditRecorder) RecordClientToolAudit(ctx context.Context, e clientbridge.AuditEntry) {
	detail, _ := json.Marshal(map[string]any{
		"tool":       e.Tool,
		"session_id": e.SessionID,
		"detail":     e.Detail,
	})
	entry := biz.AuditLog{
		Action:     "client_tool." + e.Action,
		Resource:   "client_tool",
		ResourceID: e.InvocationID,
		Detail:     string(detail),
		Actor:      e.UserID,
	}
	if !e.CreatedAt.IsZero() {
		entry.CreatedAt = e.CreatedAt.UTC().Format(time.RFC3339)
	}
	// InsertAuditLog 自身补齐 ID/CreatedAt 缺省值；失败仅告警降级（不阻断调用链）。
	if err := r.repo.InsertAuditLog(ctx, entry); err != nil {
		r.lg.Warn("client tool audit persist failed",
			loggateway.StepID("client_tool.audit_fail"),
			loggateway.Str("action", entry.Action),
			loggateway.Str("invocation_id", e.InvocationID),
			loggateway.Err(err))
	}
}
