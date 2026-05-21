package a2a

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
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
