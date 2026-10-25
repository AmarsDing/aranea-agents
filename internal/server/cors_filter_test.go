package server

import (
	"net"
	"net/http/httptest"
	"testing"
)

// TestOriginAllowedLocalhostGate pins F1: reflexive localhost reflection
// (with credentials) is a dev/test convenience; a named non-dev deployment
// must opt in per origin via KRATOS_HTTP_EXTRA_CORS_ORIGINS.
func TestOriginAllowedLocalhostGate(t *testing.T) {
	const origin = "http://localhost:9301"

	t.Run("unset env allows localhost reflection", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "")
		t.Setenv("KRATOS_HTTP_EXTRA_CORS_ORIGINS", "")
		if !OriginAllowed(origin) {
			t.Fatalf("default deployment should allow localhost origin")
		}
	})
	t.Run("dev env allows localhost reflection", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "dev")
		t.Setenv("KRATOS_HTTP_EXTRA_CORS_ORIGINS", "")
		if !OriginAllowed(origin) {
			t.Fatalf("dev deployment should allow localhost origin")
		}
	})
	t.Run("prod env refuses localhost reflection", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "production")
		t.Setenv("KRATOS_HTTP_EXTRA_CORS_ORIGINS", "")
		if OriginAllowed(origin) {
			t.Fatalf("prod deployment must not reflect arbitrary localhost origins")
		}
	})
	t.Run("explicit extra origin works in prod", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "production")
		t.Setenv("KRATOS_HTTP_EXTRA_CORS_ORIGINS", "https://desk.example.com, "+origin)
		if !OriginAllowed(origin) {
			t.Fatalf("explicitly listed origin must be allowed in prod")
		}
		if OriginAllowed("http://localhost:9999") {
			t.Fatalf("non-listed localhost origin must stay refused in prod")
		}
	})
	t.Run("empty origin never allowed", func(t *testing.T) {
		t.Setenv("DEPLOY_ENV", "")
		if OriginAllowed("") {
			t.Fatalf("empty origin must never be allowed")
		}
	})
}

// TestClientIPXFFTrustGate pins F2: X-Forwarded-For only steers the limiter
// when the direct peer is loopback (or an allowlisted proxy); a spoofed XFF
// from an internet peer must fall back to the remote address.
func TestClientIPXFFTrustGate(t *testing.T) {
	t.Run("loopback peer honors xff first hop", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/v1/admins/login", nil)
		r.RemoteAddr = "127.0.0.1:43210"
		r.Header.Set("X-Forwarded-For", "203.0.113.7, 70.41.3.18")
		if got := clientIP(r); got != "203.0.113.7" {
			t.Fatalf("loopback proxy should honor xff first hop, got %q", got)
		}
	})
	t.Run("internet peer cannot spoof xff", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/v1/admins/login", nil)
		r.RemoteAddr = "198.51.100.23:43210"
		r.Header.Set("X-Forwarded-For", "203.0.113.7")
		if got := clientIP(r); got != "198.51.100.23" {
			t.Fatalf("untrusted peer xff must be ignored, got %q", got)
		}
	})
	t.Run("no xff falls back to remote addr", func(t *testing.T) {
		r := httptest.NewRequest("POST", "/v1/admins/login", nil)
		r.RemoteAddr = "198.51.100.23:43210"
		if got := clientIP(r); got != "198.51.100.23" {
			t.Fatalf("expected remote addr fallback, got %q", got)
		}
	})
}

// TestParseTrustedProxyCIDRs pins the F2 allowlist parser: CIDRs and bare IPs
// are both honored; unrelated networks are not.
func TestParseTrustedProxyCIDRs(t *testing.T) {
	cidrs := parseTrustedProxyCIDRs("10.0.0.0/8, 192.168.1.5")
	if len(cidrs) != 2 {
		t.Fatalf("expected 2 parsed cidrs, got %d", len(cidrs))
	}
	contains := func(ip string) bool {
		parsed := net.ParseIP(ip)
		for _, n := range cidrs {
			if n.Contains(parsed) {
				return true
			}
		}
		return false
	}
	if !contains("10.9.9.9") || !contains("192.168.1.5") {
		t.Fatalf("allowlisted addresses must match")
	}
	if contains("203.0.113.7") {
		t.Fatalf("unlisted address must not match")
	}
}
