package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	servermw "aranea-agents/internal/server/middleware"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/transport"
)

// fakeTransport 是测试用最小 gRPC transport.Transporter。
type fakeTransport struct{ operation string }

func (f fakeTransport) Kind() transport.Kind            { return transport.KindGRPC }
func (f fakeTransport) Endpoint() string                { return "grpc://test" }
func (f fakeTransport) Operation() string               { return f.operation }
func (f fakeTransport) RequestHeader() transport.Header { return emptyHeader{} }
func (f fakeTransport) ReplyHeader() transport.Header   { return emptyHeader{} }

// fakeHTTPTransport 额外满足 kratos http.Transporter（Request + PathTemplate），
// 让中间件能读到 HTTP 方法。
type fakeHTTPTransport struct {
	fakeTransport
	method string
}

func (f fakeHTTPTransport) Request() *http.Request {
	return httptest.NewRequest(f.method, "/"+f.operation, nil)
}
func (f fakeHTTPTransport) PathTemplate() string { return f.operation }

type emptyHeader struct{}

func (emptyHeader) Get(string) string      { return "" }
func (emptyHeader) Set(string, string)     {}
func (emptyHeader) Add(string, string)     {}
func (emptyHeader) Keys() []string         { return nil }
func (emptyHeader) Values(string) []string { return nil }

var (
	_ transport.Transporter = fakeTransport{}
	_ transport.Transporter = fakeHTTPTransport{}
)

// assertAuthForbidden 断言 err 为 apierror AUTH/FORBIDDEN 形状（评审
// 2026-11-09 A1：Permission 中间件与 ForceResetGate 共用同一 403 形状）。
func assertAuthForbidden(t *testing.T, err error) {
	t.Helper()
	ae, ok := apierror.From(err)
	if !ok || ae.Code != apierror.CodeForbidden || ae.Domain != "AUTH" {
		t.Fatalf("expect AUTH/FORBIDDEN apierror, got %v", err)
	}
}

// grpcCtxWithOperation 构造带 operation 的 gRPC 服务器上下文。
func grpcCtxWithOperation(op string) context.Context {
	return transport.NewServerContext(context.Background(), fakeTransport{operation: op})
}

// httpCtxWithRoute 构造带方法与 pathTemplate 的 HTTP 服务器上下文。
func httpCtxWithRoute(method, pathTemplate string) context.Context {
	return transport.NewServerContext(context.Background(),
		fakeHTTPTransport{fakeTransport{operation: pathTemplate}, method})
}

func TestPermissionMiddleware(t *testing.T) {
	registry := servermw.NewPermissionRegistry()
	registry.RequireRPC("/kratos.admin.v1.AdminService/UpdateAdmin", "PUT", "/v1/admins/update", "user.write")

	mw := servermw.Permission(registry, loggateway.NewNoop())
	run := func(ctx context.Context) (err error, called bool) {
		handler := func(context.Context, interface{}) (interface{}, error) {
			called = true
			return "ok", nil
		}
		_, err = mw(handler)(ctx, nil)
		return err, called
	}

	superadmin := &auth.Auth{UserID: 1, Access: "admin", Roles: []string{"superadmin"}, Clearance: 5}
	viewer := &auth.Auth{UserID: 2, Access: "user", Roles: []string{"viewer"}, Clearance: 1}
	userRole := &auth.Auth{UserID: 3, Access: "user", Roles: []string{"user"}, Clearance: 1}

	t.Run("未登记operation_放行", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "production")
		ctx := auth.NewContext(grpcCtxWithOperation("/kratos.admin.v1.AdminService/Login"), viewer)
		err, called := run(ctx)
		if err != nil || !called {
			t.Fatalf("unregistered op must pass: err=%v called=%v", err, called)
		}
	})

	t.Run("gRPC键_登记且授权_放行", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "production")
		ctx := auth.NewContext(grpcCtxWithOperation("/kratos.admin.v1.AdminService/UpdateAdmin"), superadmin)
		err, called := run(ctx)
		if err != nil || !called {
			t.Fatalf("authorized must pass: err=%v called=%v", err, called)
		}
	})

	t.Run("HTTP键_方法路径均匹配_放行", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "production")
		ctx := auth.NewContext(httpCtxWithRoute(http.MethodPut, "/v1/admins/update"), superadmin)
		err, called := run(ctx)
		if err != nil || !called {
			t.Fatalf("http method+path key must match: err=%v called=%v", err, called)
		}
	})

	t.Run("HTTP键_方法不符_落默认策略放行", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "production")
		// GET /v1/admins/update 未登记 → 默认 authenticated 放行。
		ctx := auth.NewContext(httpCtxWithRoute(http.MethodGet, "/v1/admins/update"), viewer)
		err, called := run(ctx)
		if err != nil || !called {
			t.Fatalf("unregistered method must fall back to default: err=%v called=%v", err, called)
		}
	})

	t.Run("gRPC键_登记但无权_403", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "production")
		ctx := auth.NewContext(grpcCtxWithOperation("/kratos.admin.v1.AdminService/UpdateAdmin"), userRole)
		err, called := run(ctx)
		if called {
			t.Fatal("denied must not call handler")
		}
		assertAuthForbidden(t, err)
	})

	t.Run("HTTP键_登记但无权_403", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "production")
		ctx := auth.NewContext(httpCtxWithRoute(http.MethodPut, "/v1/admins/update"), userRole)
		err, called := run(ctx)
		if called {
			t.Fatal("denied http must not call handler")
		}
		assertAuthForbidden(t, err)
	})

	// W1d 冒烟 S3b 暴露并钉死：kratos v2.9.2 下 proto 生成 HTTP 路由的
	// tr.Operation() 返回 gRPC 全方法（如 /kratos.admin.v1.AdminService/UpdateAdmin）
	// 而非 pathTemplate。中间件若只查 `<方法> <operation>` 复合键，所有
	// proto HTTP 请求落「未登记 → 放行」，权限硬判全局失效。以下两条用例
	// 钉住裸键查表路径。
	t.Run("HTTP面operation为gRPC全方法_无权_403", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "production")
		ctx := auth.NewContext(httpCtxWithRoute(http.MethodPut, "/kratos.admin.v1.AdminService/UpdateAdmin"), userRole)
		err, called := run(ctx)
		if called {
			t.Fatal("http op=grpc-method denied must not call handler")
		}
		assertAuthForbidden(t, err)
	})

	t.Run("HTTP面operation为gRPC全方法_授权_放行", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "production")
		ctx := auth.NewContext(httpCtxWithRoute(http.MethodPut, "/kratos.admin.v1.AdminService/UpdateAdmin"), superadmin)
		err, called := run(ctx)
		if err != nil || !called {
			t.Fatalf("http op=grpc-method authorized must pass: err=%v called=%v", err, called)
		}
	})

	t.Run("生产环境无主体_fail-closed", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "production")
		ctx := grpcCtxWithOperation("/kratos.admin.v1.AdminService/UpdateAdmin")
		err, called := run(ctx)
		if called {
			t.Fatal("no principal in prod must fail-closed without calling handler")
		}
		assertAuthForbidden(t, err)
	})

	t.Run("dev环境无主体_gRPC内网直通放行", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "dev")
		ctx := grpcCtxWithOperation("/kratos.admin.v1.AdminService/UpdateAdmin")
		err, called := run(ctx)
		if err != nil || !called {
			t.Fatalf("dev carve-out must pass: err=%v called=%v", err, called)
		}
	})

	t.Run("dev环境有主体但无权_仍403", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "dev")
		ctx := auth.NewContext(grpcCtxWithOperation("/kratos.admin.v1.AdminService/UpdateAdmin"), userRole)
		err, called := run(ctx)
		if called {
			t.Fatal("dev with principal still enforces code")
		}
		assertAuthForbidden(t, err)
	})

	t.Run("无transport上下文_放行", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "production")
		if err, called := run(context.Background()); err != nil || !called {
			t.Fatalf("no transport ctx must pass through: err=%v called=%v", err, called)
		}
	})
}

func TestPermissionRegistryConflictPanics(t *testing.T) {
	r := servermw.NewPermissionRegistry()
	r.Require("/op", "user.read")
	defer func() {
		if recover() == nil {
			t.Fatal("conflicting duplicate registration must panic")
		}
	}()
	r.Require("/op", "user.write")
}

func TestPermissionRegistryIdempotentSameCode(t *testing.T) {
	r := servermw.NewPermissionRegistry()
	r.Require("/op", "user.read")
	r.Require("/op", "user.read")
	if code, ok := r.CodeFor("/op"); !ok || code != "user.read" {
		t.Fatalf("got %q ok=%v", code, ok)
	}
}

func TestPermissionRegistryHTTPKeyWithoutMethodPanics(t *testing.T) {
	r := servermw.NewPermissionRegistry()
	defer func() {
		if recover() == nil {
			t.Fatal("http path without method must panic")
		}
	}()
	r.RequireRPC("/kratos.x.v1.S/M", "", "/v1/x", "user.read")
}

// TestPermissionDeniedShapeMatchesForceResetGate 评审 2026-11-09 A1：
// Permission 中间件与 ForceResetGate 的 403 必须同形状——同为 apierror
// AUTH/FORBIDDEN，经 APIToKratos 同一翻译路径（apierror.ToKratos）后
// reason 均为 AUTH_FORBIDDEN、HTTP 403。
func TestPermissionDeniedShapeMatchesForceResetGate(t *testing.T) {
	registry := servermw.NewPermissionRegistry()
	registry.RequireRPC("/kratos.admin.v1.AdminService/UpdateAdmin", "PUT", "/v1/admins/update", "user.write")
	permMW := servermw.Permission(registry, loggateway.NewNoop())
	frMW := servermw.ForceResetGate(loggateway.NewNoop())
	noop := func(context.Context, interface{}) (interface{}, error) { return "ok", nil }

	t.Setenv("DEPLOY_ENV", "production")
	viewer := &auth.Auth{UserID: 2, Access: "user", Roles: []string{"viewer"}, Clearance: 1}
	frUser := &auth.Auth{UserID: 9, Access: "user", Roles: []string{"user"}, ForceReset: true}

	_, permErr := permMW(noop)(auth.NewContext(grpcCtxWithOperation("/kratos.admin.v1.AdminService/UpdateAdmin"), viewer), nil)
	_, frErr := frMW(noop)(auth.NewContext(grpcCtxWithOperation("/kratos.admin.v1.AdminService/ListAdmins"), frUser), nil)

	for name, err := range map[string]error{"permission": permErr, "force_reset": frErr} {
		assertAuthForbidden(t, err)
		ke := kerrors.FromError(apierror.ToKratos(err))
		if ke.Code != int32(http.StatusForbidden) || ke.Reason != "AUTH_FORBIDDEN" {
			t.Fatalf("%s: translated = %d/%s, want 403/AUTH_FORBIDDEN", name, ke.Code, ke.Reason)
		}
	}
}
