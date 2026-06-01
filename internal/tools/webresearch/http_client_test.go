package webresearch_test

import (
	"net/http"
	"testing"
	"time"

	"aranea-agents/internal/tools/webresearch"
)

func TestBuildHTTPClient_defaultTimeout(t *testing.T) {
	cfg := webresearch.Config{}
	client := webresearch.BuildHTTPClient(cfg)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.Timeout != 15*time.Second {
		t.Fatalf("Timeout = %v, want 15s", client.Timeout)
	}
}

func TestBuildHTTPClient_zeroTimeout(t *testing.T) {
	cfg := webresearch.Config{Timeout: 0}
	client := webresearch.BuildHTTPClient(cfg)
	if client.Timeout != 15*time.Second {
		t.Fatalf("Timeout = %v, want 15s for zero timeout", client.Timeout)
	}
}

func TestBuildHTTPClient_negativeTimeout(t *testing.T) {
	cfg := webresearch.Config{Timeout: -1 * time.Second}
	client := webresearch.BuildHTTPClient(cfg)
	if client.Timeout != 15*time.Second {
		t.Fatalf("Timeout = %v, want 15s for negative timeout", client.Timeout)
	}
}

func TestBuildHTTPClient_customTimeout(t *testing.T) {
	cfg := webresearch.Config{Timeout: 30 * time.Second}
	client := webresearch.BuildHTTPClient(cfg)
	if client.Timeout != 30*time.Second {
		t.Fatalf("Timeout = %v, want 30s", client.Timeout)
	}
}

func TestBuildHTTPClient_validProxy(t *testing.T) {
	cfg := webresearch.Config{
		Timeout:   5 * time.Second,
		HTTPProxy: "http://proxy.example.com:8080",
	}
	client := webresearch.BuildHTTPClient(cfg)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
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
	client := webresearch.BuildHTTPClient(cfg)
	if client == nil {
		t.Fatal("expected non-nil client even with invalid proxy")
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("invalid proxy should fall back to default, got error: %v", err)
	}
	if proxyURL != nil {
		if proxyURL.Host == "invalid-url" {
			t.Fatal("invalid proxy URL should not be used as proxy target")
		}
	}
}

func TestBuildHTTPClient_whitespaceProxy(t *testing.T) {
	cfg := webresearch.Config{
		Timeout:   5 * time.Second,
		HTTPProxy: "   ",
	}
	client := webresearch.BuildHTTPClient(cfg)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}
	req, _ := http.NewRequest(http.MethodGet, "http://example.com", nil)
	proxyURL, err := transport.Proxy(req)
	if err != nil {
		t.Fatalf("whitespace proxy should fall back to default, got error: %v", err)
	}
	if proxyURL != nil && proxyURL.Scheme != "" {
		t.Fatalf("whitespace proxy should not set explicit proxy, got %v", proxyURL)
	}
}
