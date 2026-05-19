package auth

import (
	"net/http"
	"strings"
	"testing"
	"time"
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
