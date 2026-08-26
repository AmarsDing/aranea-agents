package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/internal/biz/diagnostics"
	"aranea-agents/pkg/auth"
)

// newDiagRequest 直接打 ServeDiagnostics（绕过 kratos mux），聚焦鉴权与
// 响应契约；usecase 聚合逻辑由 biz/diagnostics 包测试覆盖。
func newDiagRequest(t *testing.T, access string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostics", nil)
	if access != "" {
		r = r.WithContext(auth.NewContext(r.Context(), &auth.Auth{UserID: 1, Access: access}))
	}
	return httptest.NewRecorder(), r
}

func TestDiagnosticsService_Unauthorized(t *testing.T) {
	svc := NewDiagnosticsService(diagnostics.NewUsecase(diagnostics.UsecaseDeps{}), nil)
	w, r := newDiagRequest(t, "")
	svc.ServeDiagnostics(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestDiagnosticsService_AdminForbidden(t *testing.T) {
	svc := NewDiagnosticsService(diagnostics.NewUsecase(diagnostics.UsecaseDeps{}), nil)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/diagnostics", nil)
	r = r.WithContext(auth.NewContext(r.Context(), &auth.Auth{UserID: 1, Access: "user"}))
	w := httptest.NewRecorder()
	svc.ServeDiagnostics(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestDiagnosticsService_OK(t *testing.T) {
	svc := NewDiagnosticsService(diagnostics.NewUsecase(diagnostics.UsecaseDeps{}), nil)
	w, r := newDiagRequest(t, "admin")
	svc.ServeDiagnostics(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", w.Code, w.Body.String())
	}
	var resp diagnostics.Report
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	// 无依赖装配时五个核心项仍出席（config_graph 缺席）。
	if len(resp.Items) != 5 {
		t.Fatalf("want 5 items, got %d: %+v", len(resp.Items), resp.Items)
	}
	// 契约字段完整：每项 key/status/summary/detail_ref 非空（detail_ref 前端
	// 跳转锚点，缺失会让面板无法落地——diagnostics.go 各检查项均已固化）。
	for _, it := range resp.Items {
		if it.Key == "" || it.Status == "" || it.DetailRef == "" {
			t.Fatalf("incomplete item: %+v", it)
		}
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Fatalf("content-type: %q", ct)
	}
}

// uc 为 nil 时构造函数返回 nil（路由侧判空跳过注册）。
func TestNewDiagnosticsService_NilUsecase(t *testing.T) {
	if NewDiagnosticsService(nil, nil) != nil {
		t.Fatal("nil usecase must yield nil service")
	}
}

// 对账明细端点：对账源未装配时返回 500 明确失败（审计取数口不降级）。
func TestDiagnosticsService_ReconcileSourceMissing(t *testing.T) {
	svc := NewDiagnosticsService(diagnostics.NewUsecase(diagnostics.UsecaseDeps{}), nil)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tool-assembly/reconcile", nil)
	r = r.WithContext(auth.NewContext(r.Context(), &auth.Auth{UserID: 1, Access: "admin"}))
	w := httptest.NewRecorder()
	svc.ServeToolAssemblyReconcile(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d body=%s", w.Code, w.Body.String())
	}
}

// 对账明细端点：未认证 401（与 diagnostics 同鉴权标准）。
func TestDiagnosticsService_ReconcileUnauthorized(t *testing.T) {
	svc := NewDiagnosticsService(diagnostics.NewUsecase(diagnostics.UsecaseDeps{}), nil)
	r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/tool-assembly/reconcile", nil)
	w := httptest.NewRecorder()
	svc.ServeToolAssemblyReconcile(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d body=%s", w.Code, w.Body.String())
	}
}
