// Package browser provides browser automation tool assembly with SSRF
// protection. The navigation guard validates URLs before the browser
// is allowed to navigate to them, blocking loopback/private network
// addresses and enforcing domain allow/deny lists.
//
// The guard logic is ported from the trpc-agent-go openclaw reference
// implementation (pkg/trpc-agent-go/openclaw/internal/browser) with
// project-specific packaging. The original Apache 2.0 license applies.
package browser

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

// NavigationPolicy controls which URLs the browser tool may navigate to.
//
// Fields:
//   - AllowedDomains: whitelist of domains (empty = allow all non-blocked)
//   - BlockedDomains: blacklist of domains (always denied, takes priority)
//   - AllowLoopback:  permit localhost/127.0.0.0/8/::1 (default false)
//   - AllowPrivateNet: permit RFC 1918 private ranges (default false)
//   - AllowFileURLs:  permit file:// URLs (default false)
type NavigationPolicy struct {
	AllowedDomains  []string
	BlockedDomains  []string
	AllowLoopback   bool
	AllowPrivateNet bool
	AllowFileURLs   bool
}

// Validate checks whether the browser is allowed to navigate to raw.
// Returns nil if navigation is permitted; otherwise an error describing
// why the URL was blocked.
//
// Validation order:
//  1. Empty URL → allowed (no-op navigation)
//  2. URL parse failure → blocked
//  3. about:blank → allowed
//  4. file:// → blocked unless AllowFileURLs
//  5. Non-http(s) schemes → blocked
//  6. Loopback host → blocked unless AllowLoopback
//  7. Private IP → blocked unless AllowPrivateNet
//  8. BlockedDomains match → blocked
//  9. AllowedDomains non-empty and no match → blocked
func (p NavigationPolicy) Validate(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid browser url %q: %w", raw, err)
	}

	scheme := strings.ToLower(strings.TrimSpace(u.Scheme))
	switch scheme {
	case "http", "https", "":
	case "about":
		return nil
	case "file":
		if p.AllowFileURLs {
			return nil
		}
		return fmt.Errorf("browser file URLs are blocked: %s", raw)
	default:
		return fmt.Errorf("browser url scheme %q is not allowed", u.Scheme)
	}

	host := normalizeHost(u.Hostname())
	if host == "" {
		return nil
	}

	if isLoopbackHost(host) && !p.AllowLoopback {
		return fmt.Errorf("browser loopback host is blocked: %s", host)
	}

	if ip, err := netip.ParseAddr(host); err == nil {
		if ip.IsLoopback() && !p.AllowLoopback {
			return fmt.Errorf("browser loopback address is blocked: %s", host)
		}
		if isPrivateAddress(ip) && !p.AllowPrivateNet {
			return fmt.Errorf("browser private network address is blocked: %s", host)
		}
	}

	for i := range p.BlockedDomains {
		if hostMatchesDomain(host, p.BlockedDomains[i]) {
			return fmt.Errorf("browser domain is blocked: %s", host)
		}
	}

	if len(p.AllowedDomains) == 0 {
		return nil
	}
	for i := range p.AllowedDomains {
		if hostMatchesDomain(host, p.AllowedDomains[i]) {
			return nil
		}
	}
	return fmt.Errorf("browser domain is not allowed: %s", host)
}

// normalizeDomains deduplicates and lowercases a domain list, trimming
// trailing dots. Returns nil for empty input so policies serialize cleanly.
func normalizeDomains(input []string) []string {
	if len(input) == 0 {
		return nil
	}

	out := make([]string, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for i := range input {
		host := normalizeHost(input[i])
		if host == "" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeHost(raw string) string {
	return strings.TrimSuffix(
		strings.ToLower(strings.TrimSpace(raw)),
		".",
	)
}

func hostMatchesDomain(host, domain string) bool {
	domain = normalizeHost(domain)
	if host == domain {
		return true
	}
	return strings.HasSuffix(host, "."+domain)
}

func isLoopbackHost(host string) bool {
	return host == "localhost" ||
		strings.HasSuffix(host, ".localhost")
}

func isPrivateAddress(addr netip.Addr) bool {
	return addr.IsPrivate() ||
		addr.IsLinkLocalUnicast() ||
		addr.IsLinkLocalMulticast() ||
		addr.IsMulticast() ||
		addr.IsUnspecified()
}
