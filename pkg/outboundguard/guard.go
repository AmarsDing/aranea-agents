// Package outboundguard provides SSRF protection for outbound HTTP requests.
//
// All admin-configurable outbound URLs (LLM provider api_base_url, inspect endpoints,
// health probes, webhook targets) must be validated with ValidateURL or use NewClient
// before making any network call.
//
// # DNS TOCTOU Mitigation
//
// ValidateURL resolves DNS at call time, but the TCP connection is made later by the
// HTTP client. An attacker controlling DNS (TTL=0) could switch the record to a private
// IP between validation and connection (DNS rebinding). NewClient mitigates this by
// installing a custom DialContext that re-checks the resolved IP at connect time
// (see ssrfDialer). CheckRedirect continues to validate every redirect hop, so both
// the initial connection and all redirects are covered.
package outboundguard

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	maxRedirects   = 10
	defaultTimeout = 15 * time.Second
)

// allowHostsEnv is an escape hatch for legitimately private outbound targets
// (e.g. host.docker.internal when a Docker-deployed aranea must reach a
// host-side MCP server). Comma-separated, exact hostname match only — no
// suffix/wildcard. Default empty = no behavior change.
const allowHostsEnv = "ARANEA_OUTBOUND_ALLOW_HOSTS"

// isExplicitlyAllowed reports whether host appears verbatim (case-insensitive)
// in ARANEA_OUTBOUND_ALLOW_HOSTS. Exact match after trim/lower; a listed host
// bypasses ALL subsequent block rules (localhost/.internal/private-IP), so
// operators must only list hosts they fully control.
func isExplicitlyAllowed(host string) bool {
	raw := strings.TrimSpace(os.Getenv(allowHostsEnv))
	if raw == "" {
		return false
	}
	for _, h := range strings.Split(raw, ",") {
		if strings.EqualFold(strings.TrimSpace(h), host) {
			return true
		}
	}
	return false
}

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
	if isExplicitlyAllowed(host) {
		return nil
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

// ── Cached host validation ───────────────────────────────────────────────────

// cachedValidator caches SUCCESSFUL host validations for a bounded TTL so
// hot paths (e.g. LLM model builds validating the same provider base URL on
// every agent construction) do not perform a live DNS lookup each time.
//
// 00:52 会话运行时取证：trpc_llm 每次构建 agent 都实时解析 provider 域名，
// 一次瞬时 DNS 失败（lookup api.deepseek.com: no such host）即导致整个
// team graph build 失败、团队运行终止。缓存成功结果后，稳态构建不再触达
// DNS，瞬时解析抖动不再杀死运行。
//
// 安全语义（与包级 KNOWN LIMITATION 一致）：
//   - 仅缓存成功结果；失败与私网阻断永不入缓存（无负缓存，故障恢复即时生效）。
//   - 缓存把既有的 DNS TOCTOU 重绑定窗口从「校验→连接」拉长到 TTL 量级；
//     包文档已声明该限制并以 connect-time 校验为后续强化方向，此处不降低
//     现有安全水位。
//   - 字面量 IP 不经过缓存与 DNS，每次直接校验（零成本）。
type cachedValidator struct {
	ttl      time.Duration
	lookupIP func(string) ([]net.IP, error)

	mu    sync.Mutex
	valid map[string]time.Time // host(lower) → 缓存到期时刻
}

// newCachedValidator 创建缓存校验器。ttl ≤ 0 时使用 5 分钟默认值。
// lookupIP 为 nil 时使用 net.LookupIP（测试可注入替身）。
func newCachedValidator(ttl time.Duration, lookupIP func(string) ([]net.IP, error)) *cachedValidator {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	if lookupIP == nil {
		lookupIP = net.LookupIP
	}
	return &cachedValidator{ttl: ttl, lookupIP: lookupIP, valid: make(map[string]time.Time)}
}

// NewCachedValidator 导出构造函数，供 LLM provider 等热路径调用方使用。
func NewCachedValidator(ttl time.Duration) *cachedValidator {
	return newCachedValidator(ttl, nil)
}

// ValidateURL 与包级 ValidateURL 语义一致，但成功的主机校验在 TTL 内缓存。
func (v *cachedValidator) ValidateURL(rawURL string) error {
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
	return v.validateHost(parsed.Hostname())
}

func (v *cachedValidator) validateHost(host string) error {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return fmt.Errorf("outboundguard: host is required")
	}
	if isExplicitlyAllowed(host) {
		return nil
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("outboundguard: localhost is not allowed")
	}
	if host == "metadata.google.internal" || strings.HasSuffix(host, ".internal") {
		return fmt.Errorf("outboundguard: cloud metadata host %q is not allowed", host)
	}
	// 字面量 IP：无 DNS、无缓存，直接校验。
	if addr, err := netip.ParseAddr(host); err == nil {
		if isBlockedAddr(addr) {
			return fmt.Errorf("outboundguard: private or local address %s is not allowed", addr)
		}
		return nil
	}

	now := time.Now()
	v.mu.Lock()
	if expiry, ok := v.valid[host]; ok && now.Before(expiry) {
		v.mu.Unlock()
		return nil
	}
	v.mu.Unlock()

	ips, err := v.lookupIP(host)
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

	v.mu.Lock()
	v.valid[host] = now.Add(v.ttl)
	v.mu.Unlock()
	return nil
}

// ssrfDialer returns a net.Dialer that rejects connections to private,
// loopback, link-local, or metadata IP addresses at connect time.
// This closes the DNS TOCTOU window left by ValidateURL: even if DNS
// rebinding changes the record after validation, the TCP handshake will
// still be blocked here.
//
// allowAddrs carries the pre-resolved IPs of hosts listed in
// ARANEA_OUTBOUND_ALLOW_HOSTS so that explicitly-allowed targets are not
// blocked at dial time (the hostname-level check is done by ValidateURL).
func ssrfDialer(timeout time.Duration, allowAddrs []netip.Addr) *net.Dialer {
	allowed := make(map[netip.Addr]struct{}, len(allowAddrs))
	for _, a := range allowAddrs {
		allowed[a] = struct{}{}
	}
	return &net.Dialer{
		Timeout: timeout,
		Control: func(network, address string, c syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("outboundguard: invalid IP %q", host)
			}
			addr, ok := netip.AddrFromSlice(ip)
			if !ok {
				return fmt.Errorf("outboundguard: unrecognised IP %s", ip)
			}
			if _, ok := allowed[addr]; ok {
				return nil
			}
			if isBlockedAddr(addr) {
				return fmt.Errorf("outboundguard: blocked IP %s (private/loopback/link-local)", ip)
			}
			return nil
		},
	}
}

// resolveAllowedHosts parses ARANEA_OUTBOUND_ALLOW_HOSTS and resolves each
// host to its current IP addresses. Errors are silently ignored — an
// unresolvable allowed host simply won't get the dial-time bypass, but the
// hostname-level check in ValidateURL still applies.
func resolveAllowedHosts() []netip.Addr {
	raw := strings.TrimSpace(os.Getenv(allowHostsEnv))
	if raw == "" {
		return nil
	}
	var out []netip.Addr
	for _, h := range strings.Split(raw, ",") {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		ips, err := net.LookupIP(h)
		if err != nil {
			continue
		}
		for _, ip := range ips {
			if addr, ok := netip.AddrFromSlice(ip); ok {
				out = append(out, addr)
			}
		}
	}
	return out
}

// NewClient returns an *http.Client with SSRF protection on both the
// initial connection and every redirect hop. The DialContext blocks
// private/loopback/link-local IPs at connect time (TOCTOU fix), while
// CheckRedirect validates redirect targets with the same host rules.
// Timeout defaults to 15 s; pass ≤ 0 for default.
func NewClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = ssrfDialer(timeout, resolveAllowedHosts()).DialContext
	return &http.Client{
		Timeout:       timeout,
		CheckRedirect: checkRedirect,
		Transport:     transport,
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
