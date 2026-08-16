package webresearch

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/outboundguard"
)

func buildHTTPClient(cfg Config, lg loggateway.Logger) *http.Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeoutSec * time.Second
	}
	client := outboundguard.NewClient(timeout)
	// Configure proxy support. The SSRF-safe CheckRedirect lives on the
	// http.Client (set by outboundguard.NewClient), and the SSRF-safe
	// DialContext lives on the Transport. When replacing the Transport we
	// must preserve the DialContext from the outboundguard client.
	if proxyURL := strings.TrimSpace(cfg.HTTPProxy); proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			// Preserve the outboundguard Transport's DialContext (SSRF dial-time
			// IP filter) when setting a proxy. Fall back to cloning the current
			// Transport so any non-nil DialContext is kept.
			if existing, ok := client.Transport.(*http.Transport); ok && existing.DialContext != nil {
				cloned := existing.Clone()
				cloned.Proxy = http.ProxyURL(u)
				client.Transport = cloned
			} else if base, ok := http.DefaultTransport.(*http.Transport); ok {
				cloned := base.Clone()
				cloned.Proxy = http.ProxyURL(u)
				client.Transport = cloned
			} else {
				client.Transport = &http.Transport{Proxy: http.ProxyURL(u)}
			}
		} else {
			// Sanitize proxy URL before logging — strip userinfo to avoid
			// leaking credentials in log output.
			sanitized := sanitizeURLForLog(proxyURL)
			lg.Warn("failed to parse proxy URL",
				loggateway.StepID("tool.webresearch.proxy_parse_fail"),
				loggateway.Str("proxy_url", sanitized),
				loggateway.Err(err))
		}
	}
	return client
}

// sanitizeURLForLog strips userinfo from a URL to prevent credential leakage in logs.
func sanitizeURLForLog(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "<invalid-url>"
	}
	u.User = nil
	return u.String()
}
