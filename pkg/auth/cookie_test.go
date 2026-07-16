package auth

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/transport"
)

func TestSessionCookieHttpOnly(t *testing.T) {
	c := newSessionCookie("jwt-value", time.Now().Add(time.Hour))
	raw := c.String()
	if !strings.Contains(raw, "HttpOnly") {
		t.Fatalf("cookie should be HttpOnly: %q", raw)
	}
	if !strings.Contains(raw, "SameSite=Lax") {
		t.Fatalf("cookie should be SameSite=Lax: %q", raw)
	}
}

func TestTokenFromHTTPRequestBearer(t *testing.T) {
	r, _ := http.NewRequest(http.MethodGet, "/v1/test", nil)
	r.Header.Set("Authorization", "Bearer my.jwt.token")
	if got := TokenFromHTTPRequest(r); got != "my.jwt.token" {
		t.Fatalf("got %q", got)
	}
}

// stubHeader is a minimal transport.Header implementation backed by http.Header.
type stubHeader struct {
	h http.Header
}

func (s *stubHeader) Get(key string) string        { return s.h.Get(key) }
func (s *stubHeader) Add(key, value string)        { s.h.Add(key, value) }
func (s *stubHeader) Set(key, value string)        { s.h.Set(key, value) }
func (s *stubHeader) Values(key string) []string   { return s.h.Values(key) }
func (s *stubHeader) Keys() []string {
	out := make([]string, 0, len(s.h))
	for k := range s.h {
		out = append(out, k)
	}
	return out
}

// stubTransporter is a minimal transport.Transporter implementation for
// testing cookie-setting functions without spinning up a kratos server.
type stubTransporter struct {
	reply *stubHeader
}

func newStubTransporter() *stubTransporter {
	return &stubTransporter{reply: &stubHeader{h: http.Header{}}}
}

func (s *stubTransporter) Kind() transport.Kind            { return transport.KindHTTP }
func (s *stubTransporter) Endpoint() string                { return "http://test" }
func (s *stubTransporter) Operation() string               { return "/test op" }
func (s *stubTransporter) RequestHeader() transport.Header { return s.reply }
func (s *stubTransporter) ReplyHeader() transport.Header   { return s.reply }

// TestSetCookieForWorkspace_NoTransport verifies the function errors when
// the context carries no kratos server transport (mirrors SetCookie behavior).
func TestSetCookieForWorkspace_NoTransport(t *testing.T) {
	err := SetCookieForWorkspace(context.Background(), 1, "admin", "ws-1", time.Now().Add(time.Hour))
	if err == nil {
		t.Fatal("expected error when no transport in context, got nil")
	}
}

// TestSetCookieForWorkspace_JWTBoundToWorkspace verifies the B-01 contract:
// the cookie set by SetCookieForWorkspace carries a JWT whose WorkspaceID
// claim matches the workspaceID argument (not DefaultWorkspaceID).
//
// This is the core of B-01 P2-A: workspace_id is stamped into the JWT at
// login, so subsequent requests carry it via cookie rather than client headers.
func TestSetCookieForWorkspace_JWTBoundToWorkspace(t *testing.T) {
	const testSecret = "test-secret-key-32bytes-minimum!!"
	prev := SetSecretForTesting(testSecret)
	defer SetSecretForTesting(prev)

	tr := newStubTransporter()
	ctx := transport.NewServerContext(context.Background(), tr)

	if err := SetCookieForWorkspace(ctx, 42, "admin", "ws-tenant-a", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SetCookieForWorkspace: %v", err)
	}

	cookies := tr.reply.Values("Set-Cookie")
	if len(cookies) == 0 {
		t.Fatal("expected Set-Cookie header, got none")
	}
	raw := cookies[0]
	// Cookie format: "access_token=<jwt>; Path=/; Expires=...; HttpOnly; SameSite=Lax"
	idx := strings.Index(raw, ";")
	if idx < 0 {
		t.Fatalf("malformed cookie: %q", raw)
	}
	prefix := cookieNameFromEnv("KRATOS_AUTH_COOKIE") + "="
	if !strings.HasPrefix(raw, prefix) {
		t.Fatalf("cookie should start with %q, got %q", prefix, raw)
	}
	tokenStr := raw[len(prefix):idx]

	claims, err := ParseToken(tokenStr, testSecret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.WorkspaceID != "ws-tenant-a" {
		t.Fatalf("expected JWT WorkspaceID %q, got %q", "ws-tenant-a", claims.WorkspaceID)
	}
	if claims.UserID != 42 {
		t.Fatalf("expected UserID 42, got %d", claims.UserID)
	}
	if claims.Access != "admin" {
		t.Fatalf("expected Access %q, got %q", "admin", claims.Access)
	}
}

// TestSetCookieForWorkspace_EmptyWorkspaceNormalized verifies that an empty
// workspaceID is normalized to DefaultWorkspaceID inside the JWT (matches
// GenerateTokenForWorkspace behavior).
func TestSetCookieForWorkspace_EmptyWorkspaceNormalized(t *testing.T) {
	const testSecret = "test-secret-key-32bytes-minimum!!"
	prev := SetSecretForTesting(testSecret)
	defer SetSecretForTesting(prev)

	tr := newStubTransporter()
	ctx := transport.NewServerContext(context.Background(), tr)

	if err := SetCookieForWorkspace(ctx, 1, "user", "", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SetCookieForWorkspace: %v", err)
	}

	cookies := tr.reply.Values("Set-Cookie")
	if len(cookies) == 0 {
		t.Fatal("expected Set-Cookie header, got none")
	}
	raw := cookies[0]
	idx := strings.Index(raw, ";")
	if idx < 0 {
		t.Fatalf("malformed cookie: %q", raw)
	}
	prefix := cookieNameFromEnv("KRATOS_AUTH_COOKIE") + "="
	tokenStr := raw[len(prefix):idx]

	claims, err := ParseToken(tokenStr, testSecret)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.WorkspaceID != DefaultWorkspaceID {
		t.Fatalf("expected normalized WorkspaceID %q, got %q", DefaultWorkspaceID, claims.WorkspaceID)
	}
}
