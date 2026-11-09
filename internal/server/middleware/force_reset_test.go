package middleware_test

import (
	"context"
	"testing"

	servermw "aranea-agents/internal/server/middleware"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
)

func TestForceResetGate(t *testing.T) {
	mw := servermw.ForceResetGate(loggateway.NewNoop())
	run := func(ctx context.Context) (err error, called bool) {
		handler := func(context.Context, interface{}) (interface{}, error) {
			called = true
			return "ok", nil
		}
		_, err = mw(handler)(ctx, nil)
		return err, called
	}

	forceResetUser := &auth.Auth{UserID: 9, Access: "user", Roles: []string{"user"}, ForceReset: true}
	normalUser := &auth.Auth{UserID: 10, Access: "user", Roles: []string{"user"}}

	t.Run("无Auth放行_认证归auth中间件", func(t *testing.T) {
		err, called := run(grpcCtxWithOperation("/kratos.admin.v1.AdminService/ListAdmins"))
		if err != nil || !called {
			t.Fatalf("err=%v called=%v", err, called)
		}
	})

	t.Run("未置位放行", func(t *testing.T) {
		ctx := auth.NewContext(grpcCtxWithOperation("/kratos.admin.v1.AdminService/ListAdmins"), normalUser)
		err, called := run(ctx)
		if err != nil || !called {
			t.Fatalf("err=%v called=%v", err, called)
		}
	})

	t.Run("置位_豁免组rpc放行_gRPC键", func(t *testing.T) {
		for _, op := range []string{
			"/kratos.admin.v1.AdminService/Current",
			"/kratos.admin.v1.AdminService/Logout",
			"/kratos.admin.v1.AdminService/ChangePassword",
		} {
			ctx := auth.NewContext(grpcCtxWithOperation(op), forceResetUser)
			err, called := run(ctx)
			if err != nil || !called {
				t.Fatalf("%s: err=%v called=%v", op, err, called)
			}
		}
	})

	t.Run("置位_豁免组rpc放行_HTTP键", func(t *testing.T) {
		for _, tc := range [][2]string{
			{"GET", "/v1/admins/current"},
			{"POST", "/v1/admins/logout"},
			{"POST", "/v1/users/change-password"},
		} {
			ctx := auth.NewContext(httpCtxWithRoute(tc[0], tc[1]), forceResetUser)
			err, called := run(ctx)
			if err != nil || !called {
				t.Fatalf("%s %s: err=%v called=%v", tc[0], tc[1], err, called)
			}
		}
	})

	t.Run("置位_非豁免rpc一律403", func(t *testing.T) {
		for _, op := range []string{
			"/kratos.admin.v1.AdminService/ListAdmins",
			"/kratos.admin.v1.AdminService/GetMenus",
			"/kratos.ceoboard.v1.CeoboardService/GetOverview",
		} {
			ctx := auth.NewContext(grpcCtxWithOperation(op), forceResetUser)
			err, called := run(ctx)
			if called {
				t.Fatalf("%s: 不应触达 handler", op)
			}
			ae, ok := apierror.From(err)
			if !ok || ae.Code != apierror.CodeForbidden || ae.Domain != "AUTH" {
				t.Fatalf("%s: 期望 AUTH/FORBIDDEN,实际 %v", op, err)
			}
		}
	})

	t.Run("置位_HTTP非豁免一律403", func(t *testing.T) {
		ctx := auth.NewContext(httpCtxWithRoute("GET", "/v1/auth/menus"), forceResetUser)
		err, called := run(ctx)
		if called {
			t.Fatal("不应触达 handler")
		}
		ae, ok := apierror.From(err)
		if !ok || ae.Code != apierror.CodeForbidden {
			t.Fatalf("期望 FORBIDDEN,实际 %v", err)
		}
	})

	t.Run("置位_方法不同的同路径不算豁免", func(t *testing.T) {
		// GET /v1/admins/current 豁免;POST 同路径(若存在)不豁免。
		ctx := auth.NewContext(httpCtxWithRoute("POST", "/v1/admins/current"), forceResetUser)
		err, called := run(ctx)
		if called || err == nil {
			t.Fatalf("err=%v called=%v", err, called)
		}
	})
}
