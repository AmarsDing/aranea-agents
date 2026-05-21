package auth

import (
	"context"
	"fmt"
	"os"
	"strings"

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
				return handler(NewContext(ctx, DevBypassPrincipal()), req)
			}

			// Try to read Bearer token from gRPC metadata.
			token := grpcBearerToken(ctx)
			if token == "" {
				// No auth credentials – allow with a debug log.
				// Note: gRPC port should be internal-only (not exposed to internet).
				fmt.Fprintln(os.Stderr, "[flow][system] system.grpc.unauthenticated: gRPC request without credentials (internal-only until M2)")
				_ = os.Stderr.Sync()
				return handler(ctx, req)
			}

			auth, err := ParseToken(token, authSecretKey)
			if err != nil {
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
