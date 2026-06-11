// Package outboundguard provides SSRF protection for outbound HTTP requests.
//
// All admin-configurable outbound URLs (LLM provider api_base_url, inspect endpoints,
// health probes, webhook targets) must be validated with ValidateURL or use NewClient
// before making any network call.
//
// # KNOWN LIMITATION — DNS TOCTOU
//
// ValidateURL resolves DNS at call time, but the TCP connection is made later by the
// HTTP client. An attacker controlling DNS (TTL=0) could switch the record to a private
// IP between validation and connection (DNS rebinding). NewClient's CheckRedirect
// re-validates every redirect hop, but the initial connection's DNS is only checked
// once. For stronger protection the caller should use a custom net.Dialer that checks
// the resolved IP at connect time (future work).
package outboundguard

import (
	"fmt"
	"net"
	"net/netip"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	maxRedirects   = 10
	defaultTimeout = 15 * time.Second
)

// ValidateURL checks that rawURL:
//   - is a well-formed http/https URL
//   - resolves to a public, non-loopback, non-private IP address
//
// It returns a non-nil error if any SSRF risk is detected.
func ValidateURL(rawURL string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("outboundguard: URL is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("outboundguard: malformed URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("outboundguard: scheme %q is not allowed (http/https only)", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("outboundguard: URL has no host")
	}
	return ValidatePublicHost(parsed.Hostname())
}

// ValidatePublicHost checks that a hostname resolves to a public IP address.
// It blocks:
//   - localhost and *.localhost
//   - cloud metadata hosts (*.internal, metadata.google.internal)
//   - loopback, private, link-local, and unspecified IP addresses
//
// This is the single source of truth for SSRF host validation across the project.
// Other packages (webhookurl, modelregistry, cli_admin) should call this function
// instead of implementing their own IP checks.
func ValidatePublicHost(host string) error {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return fmt.Errorf("outboundguard: host is required")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("outboundguard: localhost is not allowed")
	}
	// Block cloud metadata hosts (e.g. GCP metadata.google.internal,
	// AWS/Azure *.internal) to prevent SSRF via cloud metadata endpoints.
	if host == "metadata.google.internal" || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("outboundguard: cloud metadata host %q is not allowed", host)
	}
	// Fast path: if host is a literal IP, check directly without DNS lookup.
	if addr, err := netip.ParseAddr(host); err == nil {
		if isBlockedAddr(addr) {
			return fmt.Errorf("outboundguard: private or local address %s is not allowed", addr)
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("outboundguard: host lookup failed: %w", err)
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		if isBlockedAddr(addr) {
			return fmt.Errorf("outboundguard: private or local address %s is not allowed", ip)
		}
	}
	return nil
}

// isBlockedAddr reports whether the address is loopback, private, link-local,
// or unspecified — all of which indicate a non-public IP that should be blocked
// for SSRF prevention.
func isBlockedAddr(addr netip.Addr) bool {
	if !addr.IsValid() {
		return true
	}
	return addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() || addr.IsUnspecified()
}

// NewClient returns an *http.Client whose CheckRedirect validates every
// redirect hop with the same SSRF rules, and whose Timeout defaults to 15 s.
// Pass timeout ≤ 0 to use the default.
func NewClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: checkRedirect,
	}
}

// checkRedirect is the CheckRedirect hook used by NewClient. It blocks redirects
// to private/loopback addresses or unsupported schemes.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("outboundguard: stopped after %d redirects", maxRedirects)
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return fmt.Errorf("outboundguard: redirect to scheme %q is not allowed", req.URL.Scheme)
	}
	if err := ValidatePublicHost(req.URL.Hostname()); err != nil {
		return fmt.Errorf("outboundguard: redirect blocked: %w", err)
	}
	return nil
}
