package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"aranea-agents/internal/mcp/config"
)

func TestDoHTTPProbe_oauthRequired(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		code := code
		t.Run(http.StatusText(code), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(code)
			}))
			t.Cleanup(srv.Close)

			result := doHTTPProbe(srv.URL, nil, http.DefaultClient)

			if !result.OK {
				t.Errorf("HTTP %d: expected OK=true (network reachable), got OK=false; message: %s", code, result.Message)
			}
			if result.Status != "auth_required" {
				t.Errorf("HTTP %d: expected status=auth_required, got %q", code, result.Status)
			}
		})
	}
}

func TestDoHTTPProbe_serverError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	result := doHTTPProbe(srv.URL, nil, http.DefaultClient)
	if result.OK {
		t.Error("expected OK=false for HTTP 500, got OK=true")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error for HTTP 500, got %q", result.Status)
	}
}

func TestDoHTTPProbe_success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	result := doHTTPProbe(srv.URL, nil, http.DefaultClient)
	if !result.OK {
		t.Errorf("expected OK=true for HTTP 200, got OK=false; message: %s", result.Message)
	}
	if result.Status != "ok" {
		t.Errorf("expected status=ok, got %q", result.Status)
	}
}

func TestEvaluate_disabled(t *testing.T) {
	result := NewProber(nil).Evaluate(context.Background(), false, "{}")
	if result.OK {
		t.Error("expected OK=false for disabled server")
	}
	if result.Status != "unknown" {
		t.Errorf("expected status=unknown, got %q", result.Status)
	}
}

func TestEvaluate_stdioNoCommand(t *testing.T) {
	result := NewProber(nil).Evaluate(context.Background(), true, `{"transport":"stdio"}`)
	if result.OK {
		t.Error("expected OK=false for stdio without command")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error, got %q", result.Status)
	}
}

func TestEvaluate_httpNoURL(t *testing.T) {
	result := NewProber(nil).Evaluate(context.Background(), true, `{"transport":"sse"}`)
	if result.OK {
		t.Error("expected OK=false for HTTP without URL")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error, got %q", result.Status)
	}
}

func TestEvaluate_invalidJSON(t *testing.T) {
	result := NewProber(nil).Evaluate(context.Background(), true, `{invalid`)
	if result.OK {
		t.Error("expected OK=false for invalid JSON")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error, got %q", result.Status)
	}
}

func TestEvaluate_transportAutoNormalize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	result := doHTTPProbe(srv.URL, nil, http.DefaultClient)
	if !result.OK {
		t.Errorf("expected OK=true for streamable_http alias, got OK=false; message: %s", result.Message)
	}
	if result.Status != "ok" {
		t.Errorf("expected status=ok, got %q", result.Status)
	}
}

func TestEvaluate_unknownTransport(t *testing.T) {
	result := NewProber(nil).Evaluate(context.Background(), true, `{"transport":"websocket"}`)
	if result.OK {
		t.Error("expected OK=false for unknown transport")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error, got %q", result.Status)
	}
}

func TestEvaluate_httpWithHeaders(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	result := doHTTPProbe(srv.URL, map[string]string{"Authorization": "Bearer test123"}, http.DefaultClient)
	if !result.OK {
		t.Errorf("expected OK=true, got OK=false; message: %s", result.Message)
	}
	if receivedAuth != "Bearer test123" {
		t.Errorf("expected Authorization header to be forwarded, got %q", receivedAuth)
	}
}

// TestDoStreamableInitializeProbe 2026-11-03 44 实录：trpc-mcp-go streamable
// 服务对裸 GET 回 400，但 POST initialize 返回 200。探针必须走 POST。
func TestDoStreamableInitializeProbe(t *testing.T) {
	var gotMethod, receivedContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusBadRequest) // trpc-mcp-go 对裸 GET 的真实行为
			return
		}
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	result := doStreamableInitializeProbe(srv.URL, nil, http.DefaultClient)
	if !result.OK {
		t.Errorf("expected OK=true for POST initialize, got OK=false; message: %s", result.Message)
	}
	if result.Status != "ok" {
		t.Errorf("expected status=ok, got %q", result.Status)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %q", gotMethod)
	}
	if receivedContentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", receivedContentType)
	}
}

// TestConnectivityProbe_StreamableUsesPOSTInitialize ConnectivityProbe 对
// streamable 传输必须走 POST initialize（不是 GET）。测试服务器在 127.0.0.1，
// outboundguard 的 SSRF 拦截会拒绝——把 URL 校验和客户端都换成允许私网的变体
// （探针语义是「验证目标可达」，目标是否私网由部署方负责）。
func TestConnectivityProbe_StreamableUsesPOSTInitialize(t *testing.T) {
	var gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		gotMethod = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	origClient := newProbeClient
	newProbeClient = func(timeout time.Duration) *http.Client {
		return &http.Client{Timeout: timeout}
	}
	t.Cleanup(func() { newProbeClient = origClient })

	origValidate := validateHTTPConfigURL
	validateHTTPConfigURL = func(rawURL string) error { return nil }
	t.Cleanup(func() { validateHTTPConfigURL = origValidate })

	p := ConnectivityProbe{}
	result := p.Probe(context.Background(), config.ServerConfig{
		Transport: config.TransportStreamable,
		URL:       srv.URL,
	})
	if !result.OK {
		t.Errorf("expected OK=true for streamable POST initialize, got OK=false; message: %s", result.Message)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("expected POST, got %q", gotMethod)
	}
}
