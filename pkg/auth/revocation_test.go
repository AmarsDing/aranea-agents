package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"

	"google.golang.org/grpc/metadata"
)

// grpcIncomingCtxWithBearer 构造带 authorization 元数据的 gRPC 入站上下文。
func grpcIncomingCtxWithBearer(authz string) context.Context {
	md := metadata.New(map[string]string{"authorization": authz})
	return metadata.NewIncomingContext(context.Background(), md)
}

// fakeTVChecker 是测试用 TokenVersionChecker。
type fakeTVChecker struct {
	versions map[int64]int64
	err      error
}

func (f fakeTVChecker) CurrentVersion(_ context.Context, userID int64) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	return f.versions[userID], nil
}

// issueRevocationTestToken 以测试密钥签发带 P1 claims 的 token。
func issueRevocationTestToken(t *testing.T, secret string, userID int64, tv int64) string {
	t.Helper()
	tok, err := GenerateTokenForPrincipal(Principal{
		UserID:       userID,
		Access:       "user",
		Roles:        []string{"user"},
		Clearance:    1,
		TokenVersion: tv,
	}, secret, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func serveWithAuthMiddleware(t *testing.T, secret string, tv int64) *httptest.ResponseRecorder {
	t.Helper()
	tok := issueRevocationTestToken(t, secret, 7, tv)
	req := httptest.NewRequest(http.MethodGet, "/v1/some/protected", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	handlerCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})
	Middleware(loggateway.NewNoop())(next).ServeHTTP(rec, req)
	if handlerCalled {
		// 便于 table 测试读取
		rec.Header().Set("X-Handler-Called", "1")
	}
	return rec
}

// TestHTTPTokenVersionEnforcement pins P1-3 验收：token_version 递增后旧 tv
// 立即失效（HTTP 面）；checker 出错 fail-closed；未接线保持旧行为。
func TestHTTPTokenVersionEnforcement(t *testing.T) {
	secret := "test-secret-key-32bytes-minimum!!"
	prev := SetSecretForTesting(secret)
	defer SetSecretForTesting(prev)
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "")
	t.Setenv("DEPLOY_ENV", "production")

	cases := []struct {
		name       string
		checker    TokenVersionChecker
		tokenTV    int64
		wantStatus int
	}{
		{name: "未接线checker_旧行为放行", checker: nil, tokenTV: 0, wantStatus: http.StatusOK},
		{name: "tv一致_放行", checker: fakeTVChecker{versions: map[int64]int64{7: 3}}, tokenTV: 3, wantStatus: http.StatusOK},
		{name: "旧tv0_未bump用户_放行", checker: fakeTVChecker{versions: map[int64]int64{7: 0}}, tokenTV: 0, wantStatus: http.StatusOK},
		{name: "bump后旧tv立即失效", checker: fakeTVChecker{versions: map[int64]int64{7: 4}}, tokenTV: 3, wantStatus: http.StatusUnauthorized},
		{name: "checker出错_fail-closed", checker: fakeTVChecker{err: errors.New("redis+db down")}, tokenTV: 0, wantStatus: http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			SetTokenVersionChecker(tc.checker)
			defer SetTokenVersionChecker(nil)
			rec := serveWithAuthMiddleware(t, secret, tc.tokenTV)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}

// TestGRPCTokenVersionEnforcement：gRPC 面同样的吊销语义。
func TestGRPCTokenVersionEnforcement(t *testing.T) {
	secret := "test-secret-key-32bytes-minimum!!"
	prev := SetSecretForTesting(secret)
	defer SetSecretForTesting(prev)
	t.Setenv("KRATOS_HTTP_AUTH_DISABLED", "")
	t.Setenv("DEPLOY_ENV", "production")

	mw := GRPCMiddleware(loggateway.NewNoop())

	run := func(tv int64) error {
		tok := issueRevocationTestToken(t, secret, 9, tv)
		ctx := grpcIncomingCtxWithBearer("Bearer " + tok)
		ran := false
		_, err := mw(grpcTestHandler(&ran))(ctx, nil)
		if err == nil && !ran {
			t.Fatal("handler not run but no error")
		}
		return err
	}

	SetTokenVersionChecker(fakeTVChecker{versions: map[int64]int64{9: 2}})
	defer SetTokenVersionChecker(nil)
	if err := run(2); err != nil {
		t.Fatalf("matching tv should pass, got %v", err)
	}
	if err := run(1); err != ErrUnauthorized {
		t.Fatalf("stale tv should be rejected with ErrUnauthorized, got %v", err)
	}

	SetTokenVersionChecker(fakeTVChecker{err: errors.New("store down")})
	if err := run(2); err != ErrUnauthorized {
		t.Fatalf("checker error must fail-closed, got %v", err)
	}
}
