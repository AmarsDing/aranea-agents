package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/internal/mcp/config"
)

func TestConnectivityProbe_Name(t *testing.T) {
	p := ConnectivityProbe{}
	if p.Name() != "connectivity" {
		t.Errorf("expected name=connectivity, got %q", p.Name())
	}
}

func TestConnectivityProbe_StdioNoCommand(t *testing.T) {
	p := ConnectivityProbe{}
	result := p.Probe(context.Background(), config.ServerConfig{Transport: config.TransportStdio})
	if result.OK {
		t.Error("expected OK=false for stdio without command")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error, got %q", result.Status)
	}
}

func TestConnectivityProbe_UnknownTransport(t *testing.T) {
	p := ConnectivityProbe{}
	result := p.Probe(context.Background(), config.ServerConfig{Transport: "websocket"})
	if result.OK {
		t.Error("expected OK=false for unknown transport")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error, got %q", result.Status)
	}
}

func TestAuthAwareProbe_Name(t *testing.T) {
	p := NewAuthAwareProbe(nil)
	if p.Name() != "auth_aware" {
		t.Errorf("expected name=auth_aware, got %q", p.Name())
	}
}

func TestAuthAwareProbe_StdioDelegatesToInner(t *testing.T) {
	p := NewAuthAwareProbe(nil)
	result := p.Probe(context.Background(), config.ServerConfig{Transport: config.TransportStdio})
	if result.OK {
		t.Error("expected OK=false for stdio without command")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error, got %q", result.Status)
	}
}

func TestAuthAwareProbe_NonHTTPTransport(t *testing.T) {
	p := NewAuthAwareProbe(nil)
	result := p.Probe(context.Background(), config.ServerConfig{Transport: "websocket"})
	if result.OK {
		t.Error("expected OK=false for non-HTTP transport")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error, got %q", result.Status)
	}
}

func TestAuthAwareProbe_NoURL(t *testing.T) {
	p := NewAuthAwareProbe(nil)
	result := p.Probe(context.Background(), config.ServerConfig{
		Transport: config.TransportStreamable,
	})
	if result.OK {
		t.Error("expected OK=false for HTTP without URL")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error, got %q", result.Status)
	}
}

func TestAuthAwareProbe_TokenResolverFails(t *testing.T) {
	resolver := func(_ context.Context, _ config.AuthConfig) (string, error) {
		return "", http.ErrServerClosed
	}
	p := NewAuthAwareProbe(resolver)
	result := p.Probe(context.Background(), config.ServerConfig{
		Transport: config.TransportStreamable,
		URL:       "https://example.com/mcp",
		Auth:      config.AuthConfig{APIKey: "some-key"},
	})
	if result.OK {
		t.Error("expected OK=false when token resolver fails")
	}
	if result.Status != "auth_failed" {
		t.Errorf("expected status=auth_failed, got %q", result.Status)
	}
}

func TestAuthAwareProbe_NoAuthNoURL(t *testing.T) {
	p := NewAuthAwareProbe(nil)
	result := p.Probe(context.Background(), config.ServerConfig{
		Transport: config.TransportStreamable,
	})
	if result.OK {
		t.Error("expected OK=false for HTTP without URL and no auth")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error, got %q", result.Status)
	}
}

func TestDoHTTPProbe_authAware_401IsAuthRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	result := doHTTPProbe(srv.URL, map[string]string{"Authorization": "Bearer bad-key"}, http.DefaultClient)
	if !result.OK {
		t.Error("doHTTPProbe returns OK=true for 401 (network reachable)")
	}
	if result.Status != "auth_required" {
		t.Errorf("expected status=auth_required from doHTTPProbe, got %q", result.Status)
	}
}

func TestDoHTTPProbe_authAware_403IsAuthRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	result := doHTTPProbe(srv.URL, map[string]string{"Authorization": "Bearer bad-key"}, http.DefaultClient)
	if !result.OK {
		t.Error("doHTTPProbe returns OK=true for 403 (network reachable)")
	}
	if result.Status != "auth_required" {
		t.Errorf("expected status=auth_required from doHTTPProbe, got %q", result.Status)
	}
}

func TestDoHTTPProbe_authAware_SuccessWithToken(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	result := doHTTPProbe(srv.URL, map[string]string{"Authorization": "Bearer test-key-123"}, http.DefaultClient)
	if !result.OK {
		t.Errorf("expected OK=true, got OK=false; message: %s", result.Message)
	}
	if result.Status != "ok" {
		t.Errorf("expected status=ok, got %q", result.Status)
	}
	if receivedAuth != "Bearer test-key-123" {
		t.Errorf("expected Authorization header 'Bearer test-key-123', got %q", receivedAuth)
	}
}

func TestDoHTTPProbe_authAware_CustomHeaderName(t *testing.T) {
	var receivedKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedKey = r.Header.Get("X-API-Key")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	result := doHTTPProbe(srv.URL, map[string]string{"X-API-Key": "my-api-key"}, http.DefaultClient)
	if !result.OK {
		t.Errorf("expected OK=true, got OK=false; message: %s", result.Message)
	}
	if receivedKey != "my-api-key" {
		t.Errorf("expected X-API-Key header 'my-api-key', got %q", receivedKey)
	}
}

func TestProber_DefaultConnectivity(t *testing.T) {
	p := NewProber(nil)
	result := p.Evaluate(context.Background(), true, `{"transport":"stdio"}`)
	if result.OK {
		t.Error("expected OK=false for stdio without command")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error, got %q", result.Status)
	}
}

func TestProber_AuthAwareMode_Stdio(t *testing.T) {
	p := NewProber(nil)
	result := p.Evaluate(context.Background(), true, `{"transport":"stdio","probe_mode":"auth_aware"}`)
	if result.OK {
		t.Error("expected OK=false for stdio without command")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error, got %q", result.Status)
	}
}

func TestProber_FullHandshakeNotImplemented(t *testing.T) {
	p := NewProber(nil)
	result := p.Evaluate(context.Background(), true, `{"transport":"stdio","probe_mode":"full_handshake"}`)
	if result.OK {
		t.Error("expected OK=false for full_handshake (not implemented)")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error, got %q", result.Status)
	}
}

func TestProber_UnknownProbeMode(t *testing.T) {
	p := NewProber(nil)
	result := p.Evaluate(context.Background(), true, `{"transport":"stdio","probe_mode":"invalid_mode"}`)
	if result.OK {
		t.Error("expected OK=false for unknown probe_mode")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error, got %q", result.Status)
	}
}

func TestProber_Disabled(t *testing.T) {
	p := NewProber(nil)
	result := p.Evaluate(context.Background(), false, `{"transport":"stdio"}`)
	if result.OK {
		t.Error("expected OK=false for disabled server")
	}
	if result.Status != "unknown" {
		t.Errorf("expected status=unknown, got %q", result.Status)
	}
}

func TestProber_InvalidJSON(t *testing.T) {
	p := NewProber(nil)
	result := p.Evaluate(context.Background(), true, `{invalid`)
	if result.OK {
		t.Error("expected OK=false for invalid JSON")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error, got %q", result.Status)
	}
}

func TestProber_EmptyProbeMode_DefaultsToConnectivity(t *testing.T) {
	p := NewProber(nil)
	result := p.Evaluate(context.Background(), true, `{"transport":"stdio"}`)
	if result.Status != "error" {
		t.Errorf("expected status=error (connectivity mode, no command), got %q", result.Status)
	}
}

func TestAuthAwareProbe_ConstructsBearerHeader(t *testing.T) {
	resolver := func(_ context.Context, auth config.AuthConfig) (string, error) {
		return "resolved-token", nil
	}
	p := NewAuthAwareProbe(resolver)

	headers := p.buildAuthHeaders(config.ServerConfig{
		Headers: map[string]string{"X-Custom": "value"},
		Auth:    config.AuthConfig{},
	}, "resolved-token")

	if headers["Authorization"] != "Bearer resolved-token" {
		t.Errorf("expected Authorization=Bearer resolved-token, got %q", headers["Authorization"])
	}
	if headers["X-Custom"] != "value" {
		t.Errorf("expected X-Custom=value, got %q", headers["X-Custom"])
	}
}

func TestAuthAwareProbe_ConstructsCustomHeader(t *testing.T) {
	resolver := func(_ context.Context, auth config.AuthConfig) (string, error) {
		return "resolved-token", nil
	}
	p := NewAuthAwareProbe(resolver)

	headers := p.buildAuthHeaders(config.ServerConfig{
		Headers: map[string]string{"X-Custom": "value"},
		Auth:    config.AuthConfig{HeaderName: "X-API-Key"},
	}, "resolved-token")

	if headers["X-API-Key"] != "resolved-token" {
		t.Errorf("expected X-API-Key=resolved-token, got %q", headers["X-API-Key"])
	}
	if headers["X-Custom"] != "value" {
		t.Errorf("expected X-Custom=value, got %q", headers["X-Custom"])
	}
	_, hasBearer := headers["Authorization"]
	if hasBearer {
		t.Error("expected no Authorization header when custom HeaderName is set")
	}
}
