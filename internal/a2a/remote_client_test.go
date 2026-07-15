package a2a

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	a2abiz "aranea-agents/internal/biz/a2a"
	"aranea-agents/pkg/loggateway"
)

func TestClientAuthOptionsMTLSRequiresFiles(t *testing.T) {
	t.Parallel()
	_, err := ClientAuthOptions("mtls", `{}`)
	if err == nil || !strings.Contains(err.Error(), "cert_file") {
		t.Fatalf("expected cert error, got %v", err)
	}
}

func TestProxyClientAuthOptionsNone(t *testing.T) {
	t.Parallel()
	opts, err := ProxyClientAuthOptions(biz.A2AProxyConfig{AuthType: "none"})
	if err != nil || len(opts) != 0 {
		t.Fatalf("got opts=%d err=%v", len(opts), err)
	}
}

// TestFetchRemoteAgentCard_SSRFBlocked asserts that loopback and metadata
// IPs are rejected by validateRemoteURL before any network dial (C-07).
func TestFetchRemoteAgentCard_SSRFBlocked(t *testing.T) {
	t.Parallel()
	lg := loggateway.NewNoop()
	blocked := []string{
		"http://127.0.0.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://10.0.0.1/agent-card",
	}
	for _, u := range blocked {
		_, err := FetchRemoteAgentCard(context.Background(), u, "none", "", lg)
		if err == nil {
			t.Fatalf("expected SSRF error for %q, got nil", u)
		}
		if !strings.Contains(err.Error(), "SSRF") && !strings.Contains(err.Error(), "ssrf") && !strings.Contains(err.Error(), "blocked") {
			t.Fatalf("expected SSRF/blocked error for %q, got %v", u, err)
		}
	}
}

// TestInvokeRemoteRegistry_SSRFBlocked asserts invoke path rejects blocked
// IPs before dial (C-07).
func TestInvokeRemoteRegistry_SSRFBlocked(t *testing.T) {
	t.Parallel()
	lg := loggateway.NewNoop()
	remote := biz.A2ARemoteAgent{
		Enabled:   true,
		RemoteURL: "http://127.0.0.1/",
		DiscoveredCard: biz.A2AAgentCard{
			Enabled:      true,
			Capabilities: []biz.A2ACapability{{Name: "chat"}},
		},
	}
	_, err := InvokeRemoteRegistry(context.Background(), remote, "chat", `{}`, 5, lg, a2abiz.RetryPolicy{})
	if err == nil {
		t.Fatal("expected SSRF error for 127.0.0.1, got nil")
	}
	if !strings.Contains(err.Error(), "SSRF") && !strings.Contains(err.Error(), "ssrf") && !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("expected SSRF/blocked error, got %v", err)
	}
}
