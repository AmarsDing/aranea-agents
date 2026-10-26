package auth

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"

	"github.com/go-kratos/kratos/v2/middleware"
)

// grpcTestHandler records whether the wrapped handler ran.
func grpcTestHandler(ran *bool) middleware.Handler {
	return func(ctx context.Context, req interface{}) (interface{}, error) {
		*ran = true
		return "ok", nil
	}
}

// TestGRPCMiddlewareRejectsUnauthenticatedOutsideDev pins S-2: with
// DEPLOY_ENV set to a non-dev value, a credential-less gRPC request must be
// rejected with ErrUnauthorized instead of being passed through with the
// default_user context that disables the HTTP-side ownership gates.
func TestGRPCMiddlewareRejectsUnauthenticatedOutsideDev(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "")
	t.Setenv("CI", "")
	t.Setenv("DEPLOY_ENV", "production")

	mw := GRPCMiddleware(loggateway.NewNoop())
	ran := false
	_, err := mw(grpcTestHandler(&ran))(context.Background(), nil)
	if err == nil || err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
	if ran {
		t.Fatal("handler must not run for unauthenticated prod gRPC request")
	}
}

// TestGRPCMiddlewareAllowsUnauthenticatedInDev pins the dev/test/CI carve-out
// so local and CI traffic keeps working without service tokens.
func TestGRPCMiddlewareAllowsUnauthenticatedInDev(t *testing.T) {
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "")
	t.Setenv("CI", "")

	for _, env := range []string{"dev", "development", "test"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv("DEPLOY_ENV", env)
			mw := GRPCMiddleware(loggateway.NewNoop())
			ran := false
			got, err := mw(grpcTestHandler(&ran))(context.Background(), nil)
			if err != nil {
				t.Fatalf("dev env should pass through, got %v", err)
			}
			if !ran || got != "ok" {
				t.Fatalf("handler should run in dev, ran=%v got=%v", ran, got)
			}
		})
	}
}
