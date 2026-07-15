package a2a

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// ssrfSafeDialer returns a net.Dialer that rejects connections to private,
// loopback, link-local, or metadata IP addresses. This prevents SSRF attacks
// where a remote URL resolves to an internal network address.
//
// C-07: A2A remote URL SSRF protection.
func ssrfSafeDialer(timeout time.Duration) *net.Dialer {
	return &net.Dialer{
		Timeout: timeout,
		Control: func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("ssrf: invalid IP %q", host)
			}
			if isBlockedIP(ip) {
				return fmt.Errorf("ssrf: blocked IP %s (private/loopback/link-local/metadata)", ip)
			}
			return nil
		},
	}
}

// isBlockedIP returns true for IP addresses that must not be accessed
// from a server-side fetch (SSRF protection).
func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	// AWS/GCP metadata endpoint 169.254.169.254 is covered by IsLinkLocalUnicast
	// but we add explicit checks for common metadata IPs as defense-in-depth.
	if ip.Equal(net.IPv4(169, 254, 169, 254)) {
		return true
	}
	return false
}

// ssrfCheckRedirect validates that redirect targets do not point to
// blocked IP addresses.
func ssrfCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return fmt.Errorf("ssrf: too many redirects (>10)")
	}
	if req.URL == nil {
		return fmt.Errorf("ssrf: redirect with nil URL")
	}
	host := req.URL.Hostname()
	if host == "" {
		return fmt.Errorf("ssrf: redirect with empty host")
	}
	// Resolve host and check all IPs
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("ssrf: redirect host %q DNS lookup failed: %w", host, err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("ssrf: redirect to blocked IP %s", ip)
		}
	}
	return nil
}

// newSSRFSafeHTTPClient creates an HTTP client with SSRF protection:
//   - DialContext with IP filtering (rejects private/loopback/metadata IPs)
//   - CheckRedirect that validates each redirect target
//
// C-07: prevents A2A egress SSRF (internal network probing, metadata service access).
func newSSRFSafeHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: ssrfCheckRedirect,
		Transport: &http.Transport{
			DialContext: ssrfSafeDialer(timeout).DialContext,
		},
	}
}

// validateRemoteURL performs basic URL validation and DNS-based SSRF check
// before creating an A2A client. Returns an error if the URL is invalid or
// resolves to a blocked IP.
func validateRemoteURL(remoteURL string) error {
	u, err := url.Parse(remoteURL)
	if err != nil {
		return fmt.Errorf("ssrf: invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("ssrf: unsupported scheme %q (only http/https)", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("ssrf: empty host in URL")
	}
	// If host is already an IP, check it directly
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("ssrf: URL points to blocked IP %s", ip)
		}
		return nil
	}
	// Resolve hostname and check all IPs
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("ssrf: DNS lookup for %q failed: %w", host, err)
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return fmt.Errorf("ssrf: %q resolves to blocked IP %s", host, ip)
		}
	}
	return nil
}
