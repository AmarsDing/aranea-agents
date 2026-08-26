package service

import (
	"encoding/json"
	"net/http"

	"aranea-agents/internal/biz/diagnostics"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

// DiagnosticsService 是 79-runtime-governance R8 运行时体检 API
// （GET /api/v1/admin/diagnostics，design §9.2）。
//
// 走 custom route 而非 proto：/api/v1/* 子树被 twinOpenAPI 前缀处理器接管
// （mux 按注册次序匹配，proto 服务注册在 registerCustomRoutes 之后会被遮蔽），
// 与 /api/v1/admin/ecosystem/preset/*、/api/v1/config-graph/* 同先例。
// 响应契约 {items:[{key,status,summary,detail_ref}]} 由 biz/diagnostics.Report
// 直接序列化；单项检查失败只翻转该项 status，整体恒 200（降级不 500）。
type DiagnosticsService struct {
	uc *diagnostics.Usecase
	lg loggateway.Logger
}

// NewDiagnosticsService 构造服务。uc 为 nil 时返回 nil，路由侧判空跳过注册。
func NewDiagnosticsService(uc *diagnostics.Usecase, lg loggateway.Logger) *DiagnosticsService {
	if uc == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &DiagnosticsService{uc: uc, lg: lg}
}

func writeDiagnosticsJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ServeDiagnostics GET /api/v1/admin/diagnostics — 全量体检报告。admin 鉴权
// 与 config-graph 同标准（JWT + HasAdminAccess）。
func (s *DiagnosticsService) ServeDiagnostics(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	writeDiagnosticsJSON(w, http.StatusOK, s.uc.Run(r.Context()))
}

// ServeToolAssemblyReconcile GET /api/v1/admin/tool-assembly/reconcile —
// 工具装配对账全量明细（audit.py 复算下线后的服务侧单源，design §9.1 ADR C2）。
// 对账源未装配/失败返回 500（该端点是审计取数口，调用方需要明确失败而非降级）。
func (s *DiagnosticsService) ServeToolAssemblyReconcile(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	report, err := s.uc.ToolAssemblyReport(r.Context())
	if err != nil {
		writeDiagnosticsJSON(w, http.StatusInternalServerError, map[string]any{
			"error": map[string]string{"code": "DIAGNOSTICS.RECONCILE_FAILED", "message": err.Error()},
		})
		return
	}
	writeDiagnosticsJSON(w, http.StatusOK, report)
}

func (s *DiagnosticsService) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	a, ok := auth.FromContext(r.Context())
	if !ok || a == nil {
		writeDiagnosticsJSON(w, http.StatusUnauthorized, map[string]any{
			"error": map[string]string{"code": "DIAGNOSTICS.UNAUTHORIZED", "message": "authentication required"},
		})
		return false
	}
	if !a.HasAdminAccess() {
		writeDiagnosticsJSON(w, http.StatusForbidden, map[string]any{
			"error": map[string]string{"code": "DIAGNOSTICS.FORBIDDEN", "message": "admin access required"},
		})
		return false
	}
	return true
}
