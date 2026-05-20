package trpc

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestA2AProxyClientAuthOptions(t *testing.T) {
	t.Parallel()

	t.Run("none", func(t *testing.T) {
		opts, err := a2aProxyClientAuthOptions(biz.A2AProxyConfig{AuthType: "none"})
		if err != nil || len(opts) != 0 {
			t.Fatalf("got opts=%d err=%v", len(opts), err)
		}
	})

	t.Run("api_key", func(t *testing.T) {
		opts, err := a2aProxyClientAuthOptions(biz.A2AProxyConfig{
			AuthType:       "api_key",
			AuthConfigJSON: `{"api_key":"secret","header_name":"X-Custom-Key"}`,
		})
		if err != nil || len(opts) != 1 {
			t.Fatalf("got opts=%d err=%v", len(opts), err)
		}
	})

	t.Run("bearer", func(t *testing.T) {
		opts, err := a2aProxyClientAuthOptions(biz.A2AProxyConfig{
			AuthType:       "bearer",
			AuthConfigJSON: `{"token":"abc123"}`,
		})
		if err != nil || len(opts) != 1 {
			t.Fatalf("got opts=%d err=%v", len(opts), err)
		}
	})

	t.Run("missing secret", func(t *testing.T) {
		_, err := a2aProxyClientAuthOptions(biz.A2AProxyConfig{AuthType: "api_key"})
		if err == nil || !strings.Contains(err.Error(), "api_key or token") {
			t.Fatalf("expected missing secret error, got %v", err)
		}
	})

	t.Run("mtls", func(t *testing.T) {
		_, err := a2aProxyClientAuthOptions(biz.A2AProxyConfig{
			AuthType:       "mtls",
			AuthConfigJSON: `{}`,
		})
		if err == nil || !strings.Contains(err.Error(), "cert_file") {
			t.Fatalf("expected mtls config error, got %v", err)
		}
	})
}
