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
func GRPCMiddleware() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			// Bypass mode: inject dev principal and continue.
			if HTTPAuthBypassEnabled() {
				loggateway.Global().Warn("auth bypass active: injecting dev principal (gRPC)",
					loggateway.StepID("auth.bypass"))
				return handler(NewContext(ctx, DevBypassPrincipal()), req)
			}

			token := grpcBearerToken(ctx)
			if token == "" {
				loggateway.Global().Info("gRPC request without credentials (internal-only)",
					loggateway.StepID("grpc.unauthenticated"))
				return handler(ctx, req)
			}

			auth, err := ParseToken(token, authSecretKey)
			if err != nil {
				loggateway.Global().Warn("gRPC auth rejected: token parse failed",
					loggateway.StepID("grpc.token_invalid"), loggateway.Err(err))
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
