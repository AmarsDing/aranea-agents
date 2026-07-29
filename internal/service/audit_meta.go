package service

import (
	"context"
	"net"
	"net/http"
	"strings"

	"aranea-agents/internal/biz"

	khttp "github.com/go-kratos/kratos/v2/transport/http"
)

// recordAudit 填充 HTTP 请求元数据（IP / UserAgent / RequestID）后记录管理审计。
func recordAudit(ctx context.Context, mon *biz.MonitorUsecase, e biz.AdminAuditEntry) {
	if r, ok := khttp.RequestFromServerContext(ctx); ok && r != nil {
		e.IP, e.UserAgent, e.RequestID = auditMetaFromRequest(r)
	}
	mon.RecordAdminAudit(ctx, e)
}

// auditMetaFromRequest 从 HTTP 请求提取审计元数据：客户端 IP、User-Agent、X-Request-Id。
func auditMetaFromRequest(r *http.Request) (ip, userAgent, requestID string) {
	return clientIP(r), r.UserAgent(), r.Header.Get("X-Request-Id")
}

// clientIP 依次取 X-Forwarded-For 首跳、X-Real-IP、RemoteAddr 主机部分。
func clientIP(r *http.Request) string {
	if xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return xff
	}
	if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
		return xrip
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
