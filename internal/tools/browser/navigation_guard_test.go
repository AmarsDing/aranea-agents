package browser

import (
	"testing"
)

func TestNavigationPolicy_Validate_EmptyURL(t *testing.T) {
	p := NavigationPolicy{}
	if err := p.Validate(""); err != nil {
		t.Fatalf("empty URL should be allowed, got %v", err)
	}
	if err := p.Validate("   "); err != nil {
		t.Fatalf("whitespace-only URL should be allowed, got %v", err)
	}
}

func TestNavigationPolicy_Validate_AboutBlank(t *testing.T) {
	p := NavigationPolicy{}
	if err := p.Validate("about:blank"); err != nil {
		t.Fatalf("about:blank should be allowed, got %v", err)
	}
}

func TestNavigationPolicy_Validate_HTTPS(t *testing.T) {
	p := NavigationPolicy{}
	if err := p.Validate("https://example.com"); err != nil {
		t.Fatalf("https URL should be allowed, got %v", err)
	}
	if err := p.Validate("http://example.com"); err != nil {
		t.Fatalf("http URL should be allowed, got %v", err)
	}
}

func TestNavigationPolicy_Validate_FileURL_DefaultBlocked(t *testing.T) {
	p := NavigationPolicy{}
	if err := p.Validate("file:///etc/passwd"); err == nil {
		t.Fatal("file URL should be blocked by default")
	}
}

func TestNavigationPolicy_Validate_FileURL_AllowedWhenFlagSet(t *testing.T) {
	p := NavigationPolicy{AllowFileURLs: true}
	if err := p.Validate("file:///tmp/test.txt"); err != nil {
		t.Fatalf("file URL should be allowed when AllowFileURLs=true, got %v", err)
	}
}

func TestNavigationPolicy_Validate_LoopbackHost_DefaultBlocked(t *testing.T) {
	p := NavigationPolicy{}
	tests := []string{
		"http://localhost",
		"http://localhost:8080",
		"http://sub.localhost",
		"http://127.0.0.1",
		"http://127.0.0.1:8080",
		"http://[::1]",
		"http://[::1]:8080",
	}
	for _, url := range tests {
		if err := p.Validate(url); err == nil {
			t.Fatalf("loopback URL %q should be blocked by default", url)
		}
	}
}

func TestNavigationPolicy_Validate_LoopbackHost_AllowedWhenFlagSet(t *testing.T) {
	p := NavigationPolicy{AllowLoopback: true}
	if err := p.Validate("http://localhost:8080"); err != nil {
		t.Fatalf("loopback URL should be allowed when AllowLoopback=true, got %v", err)
	}
	if err := p.Validate("http://127.0.0.1"); err != nil {
		t.Fatalf("loopback IP should be allowed when AllowLoopback=true, got %v", err)
	}
}

func TestNavigationPolicy_Validate_PrivateNetwork_DefaultBlocked(t *testing.T) {
	p := NavigationPolicy{}
	tests := []string{
		"http://10.0.0.1",
		"http://10.255.255.255",
		"http://172.16.0.1",
		"http://172.31.255.255",
		"http://192.168.1.1",
		"http://192.168.0.100",
	}
	for _, url := range tests {
		if err := p.Validate(url); err == nil {
			t.Fatalf("private network URL %q should be blocked by default", url)
		}
	}
}

func TestNavigationPolicy_Validate_PrivateNetwork_AllowedWhenFlagSet(t *testing.T) {
	p := NavigationPolicy{AllowPrivateNet: true}
	if err := p.Validate("http://192.168.1.1"); err != nil {
		t.Fatalf("private network URL should be allowed when AllowPrivateNet=true, got %v", err)
	}
}

func TestNavigationPolicy_Validate_BlockedDomains(t *testing.T) {
	p := NavigationPolicy{BlockedDomains: []string{"evil.com", "malicious.org"}}
	if err := p.Validate("https://evil.com"); err == nil {
		t.Fatal("blocked domain should be rejected")
	}
	if err := p.Validate("https://sub.evil.com"); err == nil {
		t.Fatal("subdomain of blocked domain should be rejected")
	}
	if err := p.Validate("https://malicious.org/path"); err == nil {
		t.Fatal("blocked domain with path should be rejected")
	}
	// Non-blocked domain should pass
	if err := p.Validate("https://example.com"); err != nil {
		t.Fatalf("non-blocked domain should be allowed, got %v", err)
	}
}

func TestNavigationPolicy_Validate_AllowedDomains(t *testing.T) {
	p := NavigationPolicy{AllowedDomains: []string{"example.com", "api.example.org"}}
	if err := p.Validate("https://example.com"); err != nil {
		t.Fatalf("allowed domain should pass, got %v", err)
	}
	if err := p.Validate("https://sub.example.com"); err != nil {
		t.Fatalf("subdomain of allowed domain should pass, got %v", err)
	}
	if err := p.Validate("https://api.example.org"); err != nil {
		t.Fatalf("second allowed domain should pass, got %v", err)
	}
	// Non-allowed domain should be blocked
	if err := p.Validate("https://other.com"); err == nil {
		t.Fatal("non-allowed domain should be blocked when AllowedDomains is set")
	}
}

func TestNavigationPolicy_Validate_BlockedOverridesAllowed(t *testing.T) {
	p := NavigationPolicy{
		AllowedDomains: []string{"example.com"},
		BlockedDomains: []string{"bad.example.com"},
	}
	// blocked.example.com is in both allowed (via example.com) and blocked lists.
	// Blocked should win.
	if err := p.Validate("https://bad.example.com"); err == nil {
		t.Fatal("blocked domain should be rejected even when parent domain is allowed")
	}
	if err := p.Validate("https://good.example.com"); err != nil {
		t.Fatalf("allowed domain should pass, got %v", err)
	}
}

func TestNavigationPolicy_Validate_InvalidURL(t *testing.T) {
	p := NavigationPolicy{}
	// url.Parse may accept some malformed URLs; we focus on clearly invalid input.
	if err := p.Validate("://no-scheme"); err == nil {
		t.Fatal("URL without scheme should be rejected or handled")
	}
}

func TestNavigationPolicy_Validate_UnsupportedScheme(t *testing.T) {
	p := NavigationPolicy{}
	if err := p.Validate("ftp://example.com"); err == nil {
		t.Fatal("ftp scheme should be blocked")
	}
	if err := p.Validate("javascript:alert(1)"); err == nil {
		t.Fatal("javascript scheme should be blocked")
	}
}

func TestNavigationPolicy_Validate_HostNormalization(t *testing.T) {
	p := NavigationPolicy{AllowedDomains: []string{"Example.COM"}}
	// Host comparison should be case-insensitive.
	if err := p.Validate("https://example.com"); err != nil {
		t.Fatalf("case-insensitive domain match should pass, got %v", err)
	}
	if err := p.Validate("https://EXAMPLE.COM"); err != nil {
		t.Fatalf("uppercase host should match lowercase allowed domain, got %v", err)
	}
}

func TestNormalizeDomains(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  int
	}{
		{"nil input", nil, 0},
		{"empty input", []string{}, 0},
		{"single domain", []string{"example.com"}, 1},
		{"duplicates removed", []string{"example.com", "example.com", "EXAMPLE.COM"}, 1},
		{"trailing dot trimmed", []string{"example.com."}, 1},
		{"whitespace trimmed", []string{"  example.com  "}, 1},
		{"empty strings skipped", []string{"", "  ", "example.com"}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeDomains(tt.input)
			if len(got) != tt.want {
				t.Fatalf("got %d domains, want %d (got=%v)", len(got), tt.want, got)
			}
		})
	}
}

func TestHostMatchesDomain(t *testing.T) {
	tests := []struct {
		host   string
		domain string
		want   bool
	}{
		{"example.com", "example.com", true},
		{"sub.example.com", "example.com", true},
		{"deep.sub.example.com", "example.com", true},
		{"notexample.com", "example.com", false},
		{"example.com.evil.com", "example.com", false},
		{"", "example.com", false},
		{"example.com", "", false},
	}
	for _, tt := range tests {
		got := hostMatchesDomain(tt.host, tt.domain)
		if got != tt.want {
			t.Errorf("hostMatchesDomain(%q, %q) = %v, want %v", tt.host, tt.domain, got, tt.want)
		}
	}
}
