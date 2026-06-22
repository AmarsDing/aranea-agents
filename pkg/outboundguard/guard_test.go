package outboundguard_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/pkg/outboundguard"
)

func TestValidateURL_Allowed(t *testing.T) {
	cases := []string{
		"https://api.openai.com/v1/models",
		"http://example.com:8080/path",
		"https://openrouter.ai/api/v1/models",
	}
	for _, u := range cases {
		if err := outboundguard.ValidateURL(u); err != nil {
			// DNS resolution may fail in CI with no network — skip those
			if isNetworkError(err) {
				t.Skipf("skipping DNS-dependent test (no network): %v", err)
			}
			t.Errorf("ValidateURL(%q) = %v, want nil", u, err)
		}
	}
}

func TestValidateURL_Blocked(t *testing.T) {
	cases := []struct {
		url    string
		reason string
	}{
		{"http://localhost/api", "localhost"},
		{"http://127.0.0.1/api", "loopback IP"},
		{"http://0.0.0.0/api", "unspecified IP"},
		{"ftp://example.com/file", "non-http scheme"},
		{"", "empty URL"},
		{"not-a-url", "no scheme"},
		{"file:///etc/passwd", "file scheme"},
		{"http://192.168.1.1/admin", "private IP"},
		{"http://10.0.0.1/internal", "private 10.x"},
		{"http://172.16.0.1/", "private 172.16.x"},
	}
	for _, tc := range cases {
		err := outboundguard.ValidateURL(tc.url)
		if err == nil {
			t.Errorf("ValidateURL(%q) should be blocked (%s) but returned nil", tc.url, tc.reason)
		}
	}
}

func TestValidateURL_LocalhostSubdomain(t *testing.T) {
	if err := outboundguard.ValidateURL("http://evil.localhost/"); err == nil {
		t.Error("ValidateURL(evil.localhost) should be blocked")
	}
}

func TestNewClient_BlocksRedirectToPrivate(t *testing.T) {
	// Set up a server that redirects to a private address
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1:9999/", http.StatusFound)
	}))
	defer srv.Close()

	client := outboundguard.NewClient(0)
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	resp, err := client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Error("expected redirect to private address to be blocked, but got nil error")
	}
}

// isNetworkError returns true for errors that indicate no external network access.
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "no such host") ||
		contains(msg, "dial tcp") ||
		contains(msg, "lookup") ||
		contains(msg, "network") ||
		contains(msg, "connection refused")
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > 0 && len(substr) > 0 && containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
