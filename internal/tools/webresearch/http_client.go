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
	// http.Client (set by outboundguard.NewClient), so replacing the
	// Transport does not affect redirect validation.
	if proxyURL := strings.TrimSpace(cfg.HTTPProxy); proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			// Use a checked type assertion to avoid a panic when
			// http.DefaultTransport has been replaced (e.g., in tests).
			base, ok := http.DefaultTransport.(*http.Transport)
			if !ok {
				client.Transport = &http.Transport{Proxy: http.ProxyURL(u)}
			} else {
				cloned := base.Clone()
				cloned.Proxy = http.ProxyURL(u)
				client.Transport = cloned
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
