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

func TestValidateURL_ExplicitAllowHosts(t *testing.T) {
	t.Setenv("ARANEA_OUTBOUND_ALLOW_HOSTS", " host.docker.internal , mcp-host.local ")
	// 精确命中的主机放行（即便 *.internal / 私网解析）
	if err := outboundguard.ValidateURL("http://host.docker.internal:8951/sse"); err != nil {
		t.Errorf("ValidateURL(host.docker.internal) with allowlist = %v, want nil", err)
	}
	// 大小写不敏感
	if err := outboundguard.ValidateURL("http://HOST.DOCKER.INTERNAL/"); err != nil {
		t.Errorf("ValidateURL(HOST.DOCKER.INTERNAL) with allowlist = %v, want nil", err)
	}
	// 未列出的 *.internal 仍阻断（无后缀通配）
	if err := outboundguard.ValidateURL("http://evil.docker.internal/"); err == nil {
		t.Error("ValidateURL(evil.docker.internal) should be blocked (no wildcard)")
	}
	if err := outboundguard.ValidateURL("http://host.docker.internal.evil.com/"); err == nil {
		t.Error("ValidateURL(host.docker.internal.evil.com) should be blocked")
	}
	// 字面量私网 IP 不受主机名清单影响
	if err := outboundguard.ValidateURL("http://192.168.1.1/"); err == nil {
		t.Error("ValidateURL(192.168.1.1) should still be blocked")
	}
	// 缓存校验器同口径
	v := outboundguard.NewCachedValidator(0)
	if err := v.ValidateURL("http://mcp-host.local:8951/sse"); err != nil {
		t.Errorf("cachedValidator.ValidateURL(mcp-host.local) with allowlist = %v, want nil", err)
	}
	if err := v.ValidateURL("http://other.internal/"); err == nil {
		t.Error("cachedValidator.ValidateURL(other.internal) should be blocked")
	}
}

func TestValidateURL_AllowHostsDefaultOff(t *testing.T) {
	// 环境变量缺省时行为不变：*.internal 依旧阻断
	if err := outboundguard.ValidateURL("http://host.docker.internal:8951/sse"); err == nil {
		t.Error("ValidateURL(host.docker.internal) without allowlist should be blocked")
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

// --- P0-1 ④ AllowlistMatch / ValidateAllowlist 纯函数（review §12.1-⑦⑧）---

func TestAllowlistMatch(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		host string
		want bool
	}{
		{name: "exact match", raw: "host.docker.internal", host: "host.docker.internal", want: true},
		{name: "case-insensitive", raw: "Host.Docker.Internal", host: "host.docker.INTERNAL", want: true},
		{name: "trim spaces", raw: "  a.com , b.com ", host: "b.com", want: true},
		{name: "comma list middle", raw: "x.com,api.corp.com,y.com", host: "api.corp.com", want: true},
		{name: "no wildcard suffix", raw: "example.com", host: "api.example.com", want: false},
		{name: "empty raw", raw: "", host: "anything", want: false},
		{name: "blank raw", raw: "   ", host: "anything", want: false},
		// 空 entry 匹配空 host 是既有 isExplicitlyAllowed 的原样语义（生产不可
		// 达：两处调用点先拒空 host）。§12.1-⑦ 提炼必须零行为变更——此处 pin
		// 现状，不在纯函数里顺手"修复"语义。
		{name: "empty entry matches empty host (inherited edge)", raw: ",,", host: "", want: true},
		{name: "host empty no match", raw: "a.com", host: "", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := outboundguard.AllowlistMatch(tc.raw, tc.host); got != tc.want {
				t.Fatalf("AllowlistMatch(%q,%q)=%v, want %v", tc.raw, tc.host, got, tc.want)
			}
		})
	}
}

func TestValidateAllowlist(t *testing.T) {
	t.Run("accepts safe entries", func(t *testing.T) {
		good := [][]string{
			nil,
			{},
			{"api.openai.com"},
			{"Host.Docker.Internal", "mcp.corp.example.com"}, // lowercased + trimmed internally
			{"8.8.8.8"},   // public IPv4
			{"2001:4860:4860::8888"}, // public IPv6
		}
		for _, hosts := range good {
			if err := outboundguard.ValidateAllowlist(hosts); err != nil {
				t.Fatalf("ValidateAllowlist(%v) = %v, want nil", hosts, err)
			}
		}
	})
	t.Run("rejects foot-guns fail-closed", func(t *testing.T) {
		bad := [][]string{
			{""},
			{"   "},
			{"*"},
			{"*.example.com"},
			{"http://api.openai.com"},
			{"https://a.com/path"},
			{"a.com/"},
			{"a.com:8080"},   // host:port
			{"localhost"},
			{"evil.localhost"},
			{"printer.local"}, // mDNS
			{"metadata.google.internal"}, // 确定性元数据主机（裸 *.internal 按设计允许）
			{"127.0.0.1"},
			{"::1"},
			{"10.0.0.5"},
			{"192.168.1.10"},
			{"169.254.169.254"}, // link-local metadata IP
			{"ok.com", "localhost"}, // poison in list
		}
		for _, hosts := range bad {
			if err := outboundguard.ValidateAllowlist(hosts); err == nil {
				t.Fatalf("ValidateAllowlist(%v) = nil, want error", hosts)
			}
		}
	})
}
