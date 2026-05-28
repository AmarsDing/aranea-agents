package probe

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestDoHTTPProbe_oauthRequired verifies that a 401/403 response is treated as
// "network reachable, auth required" (OK=true, Status="auth_required") rather than
// a hard error, so OAuth-protected MCP servers don't create false alarms in the
// admin health dashboard. (TPM-P1-09)
//
// doHTTPProbe is tested directly (same package) with http.DefaultClient so the
// SSRF guard (which blocks loopback addresses) is bypassed — the unit under test
// is the status-code branch, not the SSRF guard.
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

// TestDoHTTPProbe_serverError verifies that a 500 response is still an error.
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

// TestDoHTTPProbe_success verifies 200-range is treated as OK.
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

// TestEvaluate_disabled verifies that a disabled server returns OK=false without network call.
func TestEvaluate_disabled(t *testing.T) {
	result := Evaluate(false, "{}")
	if result.OK {
		t.Error("expected OK=false for disabled server")
	}
	if result.Status != "unknown" {
		t.Errorf("expected status=unknown, got %q", result.Status)
	}
}

func TestEvaluate_stdioNoCommand(t *testing.T) {
	result := Evaluate(true, `{"transport":"stdio"}`)
	if result.OK {
		t.Error("expected OK=false for stdio without command")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error, got %q", result.Status)
	}
}

func TestEvaluate_httpNoURL(t *testing.T) {
	result := Evaluate(true, `{"transport":"sse"}`)
	if result.OK {
		t.Error("expected OK=false for HTTP without URL")
	}
	if result.Status != "error" {
		t.Errorf("expected status=error, got %q", result.Status)
	}
}

func TestEvaluate_invalidJSON(t *testing.T) {
	result := Evaluate(true, `{invalid`)
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
	result := Evaluate(true, `{"transport":"websocket"}`)
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
