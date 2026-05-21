// Package webhookurl validates outbound Hook/Gateway webhook targets (SSRF guard).
package webhookurl

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strings"
)

// ValidateNotifyURL ensures url is a safe outbound HTTPS/HTTP POST target.
// Also used for Gateway outbound webhooks (gateway_webhooks.url).
func ValidateNotifyURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid webhook url: %w", err)
	}
	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	if scheme != "https" && scheme != "http" {
		return fmt.Errorf("webhook url must use http or https")
	}
	host := strings.TrimSpace(u.Hostname())
	if host == "" {
		return fmt.Errorf("webhook url host is required")
	}
	lower := strings.ToLower(host)
	if lower == "localhost" || strings.HasSuffix(lower, ".localhost") {
		return fmt.Errorf("webhook url host %q is not allowed", host)
	}
	if lower == "metadata.google.internal" || strings.HasSuffix(lower, ".internal") {
		return fmt.Errorf("webhook url host %q is not allowed", host)
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("webhook url must not target private or loopback addresses")
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("webhook url host lookup failed: %w", err)
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		if isBlockedIP(addr) {
			return fmt.Errorf("webhook url resolves to blocked address %s", ip.String())
		}
	}
	return nil
}

func isBlockedIP(ip netip.Addr) bool {
	if !ip.IsValid() {
		return true
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
