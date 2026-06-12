package webresearch_test

import (
	"net/http"
	"testing"
	"time"

	"aranea-agents/internal/tools/webresearch"
	"aranea-agents/pkg/loggateway"
)

func TestBuildHTTPClient_defaultTimeout(t *testing.T) {
	cfg := webresearch.Config{}
	client := webresearch.BuildHTTPClient(cfg, loggateway.NewNoop())
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Timeout != 15*time.Second {
		t.Fatalf("Timeout = %v, want 15s", client.Timeout)
	}
}

func TestBuildHTTPClient_zeroTimeout(t *testing.T) {
	cfg := webresearch.Config{Timeout: 0}
	client := webresearch.BuildHTTPClient(cfg, loggateway.NewNoop())
	if client.Timeout != 15*time.Second {
		t.Fatalf("Timeout = %v, want 15s for zero timeout", client.Timeout)
	}
}

func TestBuildHTTPClient_negativeTimeout(t *testing.T) {
	cfg := webresearch.Config{Timeout: -1 * time.Second}
	client := webresearch.BuildHTTPClient(cfg, loggateway.NewNoop())
	if client.Timeout != 15*time.Second {
		t.Fatalf("Timeout = %v, want 15s for negative timeout", client.Timeout)
	}
}

func TestBuildHTTPClient_customTimeout(t *testing.T) {
	cfg := webresearch.Config{Timeout: 30 * time.Second}
	client := webresearch.BuildHTTPClient(cfg, loggateway.NewNoop())
	if client.Timeout != 30*time.Second {
		t.Fatalf("Timeout = %v, want 30s", client.Timeout)
	}
}

func TestBuildHTTPClient_validProxy(t *testing.T) {
	cfg := webresearch.Config{
		Timeout:   5 * time.Second,
		HTTPProxy: "http://proxy.example.com:8080",
	}
	client := webresearch.BuildHTTPClient(cfg, loggateway.NewNoop())
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("Proxy() error: %v", err)
	}
	if proxyURL == nil {
		t.Fatal("expected proxy URL")
	}
	if proxyURL.Host != "proxy.example.com:8080" {
		t.Fatalf("proxy host = %q, want proxy.example.com:8080", proxyURL.Host)
	}
}

func TestBuildHTTPClient_invalidProxy(t *testing.T) {
	cfg := webresearch.Config{
		Timeout:   5 * time.Second,
		HTTPProxy: "://invalid-url",
	}
	client := webresearch.BuildHTTPClient(cfg, loggateway.NewNoop())
	if client == nil {
		t.Fatal("expected non-nil client even with invalid proxy")
	}
	// Invalid proxy falls back to the default transport (no custom Transport set).
	// The client still has SSRF-safe CheckRedirect from outboundguard.
}

func TestBuildHTTPClient_whitespaceProxy(t *testing.T) {
	cfg := webresearch.Config{
		Timeout:   5 * time.Second,
		HTTPProxy: "   ",
	}
	client := webresearch.BuildHTTPClient(cfg, loggateway.NewNoop())
	// Whitespace proxy is treated as no proxy; client uses default transport
	// with SSRF-safe CheckRedirect from outboundguard.
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}
