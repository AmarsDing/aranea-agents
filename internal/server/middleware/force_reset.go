package middleware

import (
	"context"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"

	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	kratoshttp "github.com/go-kratos/kratos/v2/transport/http"
)

// 本文件实现 P1 补漏·force_reset 强制改密门（三项目整合 §4.1 / R3）：
// wb 迁移账号（一次性口令）与管理员重置的账号带 force_reset 标记，登录后
// 必须立即改密。置位期间后端只放行豁免组 rpc（改密/登出/查自身），其余
// 一律 403；前端路由守卫同步把用户钉在强制改密页。
//
// claim 快照可信性：fr claim 是签发时刻 admins.force_reset 的快照。两条标记
// 转换路径都先递增 token_version 吊销全部会话再翻转标记（ChangePassword 清
// 标记；service UpdateAdmin 管理台置位），故：
//   - 清标记后，带 fr=true 的旧 token 已全部失效，无"误拦已清用户"窗口；
//   - 置位后，带 fr=false 的旧 token 已全部失效，无"绕过门继续用"窗口。
// 迁移/建号置位发生在账号无存活会话时，同样安全。
//
// 中间件必须排在认证与 Permission 之后：只处理已认证且 fr 置位的主体。

// ForceResetMessage 是拒绝响应的稳定文案（前端可按 AUTH_FORBIDDEN + 本文案识别）。
const ForceResetMessage = "password reset required"

// forceResetExemptOperations 是置位期间仍放行的 operation 键集合
// （gRPC 全方法名 + HTTP `<大写方法> <pathTemplate>`，与 Permission 中间件同键空间）。
var forceResetExemptOperations = map[string]struct{}{
	"/kratos.admin.v1.AdminService/Logout":         {},
	"/kratos.admin.v1.AdminService/Current":        {},
	"/kratos.admin.v1.AdminService/ChangePassword": {},
	HTTPKey("POST", "/v1/admins/logout"):           {},
	HTTPKey("GET", "/v1/admins/current"):           {},
	HTTPKey("POST", "/v1/users/change-password"):   {},
}

// IsForceResetExempt 报告 operation 键是否在豁免组（测试与中间件共用）。
func IsForceResetExempt(operation string) bool {
	_, ok := forceResetExemptOperations[operation]
	return ok
}

// ForceResetGate 返回强制改密门 kratos 中间件。
//
// 判定顺序：
//  1. ctx 无 Auth（未认证/dev 直通）→ 放行（认证问题由 auth 中间件负责）；
//  2. fr 未置位 → 放行；
//  3. operation 在豁免组 → 放行；
//  4. 其余 → 403 AUTH_FORBIDDEN / password reset required。
func ForceResetGate(lg loggateway.Logger) middleware.Middleware {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			a, ok := auth.FromContext(ctx)
			if !ok || a == nil || !a.ForceReset {
				return handler(ctx, req)
			}
			key := ""
			if tr, tok := transport.FromServerContext(ctx); tok {
				key = tr.Operation()
				if hreq, hok := kratoshttp.RequestFromServerContext(ctx); hok && hreq != nil && hreq.Method != "" {
					key = HTTPKey(hreq.Method, tr.Operation())
				}
			}
			if IsForceResetExempt(key) {
				return handler(ctx, req)
			}
			if ok, suppressed := permissionDeniedThrottle.allow(); ok {
				fields := []loggateway.Field{
					loggateway.StepID("auth.force_reset_blocked"),
					loggateway.Str("operation", key),
					loggateway.Int64("user_id", a.UserID),
				}
				if suppressed > 0 {
					fields = append(fields, loggateway.Int("suppressed", suppressed))
				}
				lg.Warn("force_reset principal blocked outside exemption group", fields...)
			}
			return nil, apierror.Forbidden("AUTH", ForceResetMessage)
		}
	}
}
