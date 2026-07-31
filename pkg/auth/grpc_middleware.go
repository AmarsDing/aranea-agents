package auth

import (
	"context"
	"strings"

	"aranea-agents/pkg/loggateway"

	"github.com/go-kratos/kratos/v2/middleware"
	"google.golang.org/grpc/metadata"
)

// GRPCMiddleware is a Kratos middleware for gRPC server authentication.
//
// EP-SEC-04: gRPC was previously unauthenticated.  This middleware:
//  1. In dev/bypass mode → injects DevBypassPrincipal (same as HTTP bypass).
//  2. Bearer token present in gRPC metadata (key "authorization") → validates JWT.
//  3. No token → allows the request through with a warning (gRPC is internal-only;
//     enforce via network policy until M2 adds mTLS or service-account tokens).
//
// At M2 this function should be tightened to reject unauthenticated requests in
// production by checking DEPLOY_ENV.
func GRPCMiddleware(lg loggateway.Logger) middleware.Middleware {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			// Bypass mode: inject dev principal and continue.
			if HTTPAuthBypassEnabled() {
				lg.Warn("auth bypass active: injecting dev principal (gRPC)",
					loggateway.StepID("auth.bypass"))
				return handler(NewContext(ctx, DevBypassPrincipal()), req)
			}

			token := grpcBearerToken(ctx)
			if token == "" {
				lg.Info("gRPC request without credentials (internal-only)",
					loggateway.StepID("grpc.unauthenticated"))
				emitGRPCUnauthenticated(ctx)
				return handler(ctx, req)
			}

			auth, err := ParseToken(token, authSecretKey)
			if err != nil {
				// K2: 登录失败 Warn 需节流防爆破刷屏（与 HTTP 共享 10s 窗口）。
				if ok, suppressed := tokenInvalidThrottle.allow(); ok {
					fields := []loggateway.Field{
						loggateway.StepID("grpc.token_invalid"),
						loggateway.Err(err),
					}
					if suppressed > 0 {
						fields = append(fields, loggateway.Int("suppressed", suppressed))
					}
					lg.Warn("gRPC auth rejected: token parse failed", fields...)
				}
				return nil, ErrUnauthorized
			}
			return handler(NewContext(ctx, auth), req)
		}
	}
}

// grpcBearerToken extracts the Bearer token from the "authorization" gRPC metadata key.
func grpcBearerToken(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get("authorization")
	if len(vals) == 0 {
		return ""
	}
	v := strings.TrimSpace(vals[0])
	after, found := strings.CutPrefix(v, "Bearer ")
	if !found {
		after, found = strings.CutPrefix(v, "bearer ")
	}
	if !found {
		return ""
	}
	return strings.TrimSpace(after)
}
