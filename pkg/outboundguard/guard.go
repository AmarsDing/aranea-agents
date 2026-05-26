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
//
// # Relationship to pkg/webhookurl
//
// pkg/webhookurl provides a stricter SSRF guard specifically for webhook POST targets
// (blocks all redirects via http.ErrUseLastResponse, uses net/netip for IP literals,
// and explicitly blocks cloud metadata hosts). outboundguard is for general LLM/API
// endpoint probing where controlled redirects are acceptable. Do not merge the two
// unless the semantics are explicitly reconciled.
package outboundguard

import (
	"fmt"
	"net"
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
	return validatePublicHost(parsed.Hostname())
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
	if err := validatePublicHost(req.URL.Hostname()); err != nil {
		return fmt.Errorf("outboundguard: redirect blocked: %w", err)
	}
	return nil
}

// validatePublicHost resolves host and rejects loopback, private, link-local,
// and unspecified addresses to prevent SSRF.
func validatePublicHost(host string) error {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return fmt.Errorf("outboundguard: host is required")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("outboundguard: localhost is not allowed")
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
			return fmt.Errorf("outboundguard: private or local address %s is not allowed", ip)
		}
	}
	return nil
}
