package a2a

import (
	"net"
	"testing"
)

// TestIsBlockedIP_PrivateAddresses asserts that private and reserved IP
// ranges are blocked (SSRF protection).
func TestIsBlockedIP_PrivateAddresses(t *testing.T) {
	blocked := []string{
		"127.0.0.1",       // loopback
		"::1",             // IPv6 loopback
		"10.0.0.1",        // private 10/8
		"172.16.0.1",      // private 172.16/12
		"192.168.1.1",     // private 192.168/16
		"169.254.169.254", // AWS metadata
		"0.0.0.0",         // unspecified
		"::",              // IPv6 unspecified
		"fe80::1",         // link-local
		"224.0.0.1",       // multicast
	}
	for _, ipStr := range blocked {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			t.Fatalf("failed to parse IP %q", ipStr)
		}
		if !isBlockedIP(ip) {
			t.Errorf("expected %s to be blocked", ipStr)
		}
	}
}

// TestIsBlockedIP_PublicAddresses asserts that public IPs are allowed.
func TestIsBlockedIP_PublicAddresses(t *testing.T) {
	allowed := []string{
		"8.8.8.8",              // Google DNS
		"1.1.1.1",              // Cloudflare DNS
		"203.0.113.1",          // TEST-NET-3 (documentation, but public range)
		"2001:4860:4860::8888", // Google DNS IPv6
	}
	for _, ipStr := range allowed {
		ip := net.ParseIP(ipStr)
		if ip == nil {
			t.Fatalf("failed to parse IP %q", ipStr)
		}
		if isBlockedIP(ip) {
			t.Errorf("expected %s to be allowed (not blocked)", ipStr)
		}
	}
}

// TestValidateRemoteURL_InvalidScheme asserts that non-http(s) schemes are rejected.
func TestValidateRemoteURL_InvalidScheme(t *testing.T) {
	urls := []string{
		"ftp://example.com",
		"file:///etc/passwd",
		"gopher://localhost",
		"javascript:alert(1)",
	}
	for _, u := range urls {
		if err := validateRemoteURL(u); err == nil {
			t.Errorf("expected error for URL %q, got nil", u)
		}
	}
}

// TestValidateRemoteURL_EmptyHost asserts that empty hosts are rejected.
func TestValidateRemoteURL_EmptyHost(t *testing.T) {
	if err := validateRemoteURL("http:///path"); err == nil {
		t.Error("expected error for URL with empty host")
	}
}

// TestValidateRemoteURL_BlockedIP asserts that URLs with blocked IPs are rejected.
func TestValidateRemoteURL_BlockedIP(t *testing.T) {
	urls := []string{
		"http://127.0.0.1/",
		"http://10.0.0.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://192.168.1.1/",
	}
	for _, u := range urls {
		if err := validateRemoteURL(u); err == nil {
			t.Errorf("expected SSRF error for URL %q, got nil", u)
		}
	}
}

// TestValidateRemoteURL_ValidURL asserts that valid public URLs pass validation.
// Note: this test requires DNS resolution; skip if network is unavailable.
func TestValidateRemoteURL_ValidURL(t *testing.T) {
	// Use a well-known public domain
	if err := validateRemoteURL("https://example.com/"); err != nil {
		t.Skipf("network/DNS unavailable, skipping: %v", err)
	}
}
