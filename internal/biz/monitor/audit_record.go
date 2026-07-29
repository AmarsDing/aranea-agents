package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// Audit 动作动词（规范：action = "<verb>.<resource>"，动词在前，见 18-monitor.md §2）。
const (
	AuditVerbCreate      = "create"
	AuditVerbUpdate      = "update"
	AuditVerbDelete      = "delete"
	AuditVerbToggle      = "toggle"
	AuditVerbArchive     = "archive"
	AuditVerbUnarchive   = "unarchive"
	AuditVerbPin         = "pin"
	AuditVerbUnpin       = "unpin"
	AuditVerbSync        = "sync"
	AuditVerbCredentials = "credentials"
)

// Audit 严重级别。
const (
	AuditSeverityInfo    = "info"
	AuditSeverityWarning = "warning"
)

// AdminAuditEntry 是 RecordAdminAudit 的结构化入参。
type AdminAuditEntry struct {
	Action     string         // 规范形式 "<verb>.<resource>"，用 AuditAction 构造
	Resource   string         // 实体类型：agent/team/channel/provider/config/session/tool/mcp_server/skill
	ResourceID string         // 实体 ID（批量操作可为空，数量放 Summary）
	Summary    string         // 人类可读摘要，如 "key=my-agent count=3"
	Before     map[string]any // 可选：变更前关键字段
	After      map[string]any // 可选：变更后关键字段
	Actor      string         // 可选：默认取 ctx 中的 auth 主体
	IP         string         // 由 service 层从 HTTP 头注入
	UserAgent  string         // 由 service 层从 HTTP 头注入
	RequestID  string         // 由 service 层从 HTTP 头注入
	Severity   string         // 可选：为空时按 Action 自动分级（AuditSeverity）
}

// auditDetail 是 audit_logs.detail 的 JSON 契约。
type auditDetail struct {
	Summary string         `json:"summary"`
	Before  map[string]any `json:"before,omitempty"`
	After   map[string]any `json:"after,omitempty"`
}

// AuditAction 构造规范 action 字符串（"<verb>.<resource>"）。
func AuditAction(verb, resource string) string {
	return strings.TrimSpace(verb) + "." + strings.TrimSpace(resource)
}

// AuditSeverity 按 action 自动分级：delete.*/credentials.* 为 warning，其余为 info。
func AuditSeverity(action string) string {
	a := strings.TrimSpace(action)
	if strings.HasPrefix(a, AuditVerbDelete+".") || strings.HasPrefix(a, AuditVerbCredentials+".") {
		return AuditSeverityWarning
	}
	return AuditSeverityInfo
}

// RecordAdminAudit 记录一条管理审计（best-effort，失败仅告警不阻断主流程）。
func (u *Usecase) RecordAdminAudit(ctx context.Context, e AdminAuditEntry) {
	if u == nil {
		return
	}
	actor := strings.TrimSpace(e.Actor)
	if actor == "" {
		if p, ok := auth.FromContext(ctx); ok && p != nil {
			actor = fmt.Sprintf("%d", p.UserID)
		}
	}
	severity := strings.TrimSpace(e.Severity)
	if severity == "" {
		severity = AuditSeverity(e.Action)
	}
	if err := u.RecordAuditLog(ctx, AuditLog{
		Action:     strings.TrimSpace(e.Action),
		Resource:   strings.TrimSpace(e.Resource),
		ResourceID: strings.TrimSpace(e.ResourceID),
		Detail:     marshalAuditDetail(e),
		Actor:      actor,
		IP:         strings.TrimSpace(e.IP),
		UserAgent:  strings.TrimSpace(e.UserAgent),
		RequestID:  strings.TrimSpace(e.RequestID),
		Severity:   severity,
	}); err != nil {
		u.lg.Warn("RecordAdminAudit failed", loggateway.StepID("monitor.audit_log_fail"), loggateway.Str("action", e.Action), loggateway.Err(err))
	}
}

// marshalAuditDetail 将 AdminAuditEntry 序列化为 detail JSON 契约（{"summary":..., "before":..., "after":...}）。
func marshalAuditDetail(e AdminAuditEntry) string {
	d := auditDetail{Summary: strings.TrimSpace(e.Summary), Before: e.Before, After: e.After}
	raw, err := json.Marshal(d)
	if err != nil {
		// map 中含不可序列化值时降级为纯 summary 对象
		fallback, _ := json.Marshal(auditDetail{Summary: d.Summary})
		return string(fallback)
	}
	return string(raw)
}
