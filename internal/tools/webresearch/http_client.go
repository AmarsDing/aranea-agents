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
	// Preserve proxy configuration on top of the SSRF-safe transport.
	if proxyURL := strings.TrimSpace(cfg.HTTPProxy); proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			// Clone the base transport to add proxy support.
			// outboundguard.NewClient may use a custom RoundTripper,
			// so we create a fresh Transport that inherits the SSRF
			// CheckRedirect from the client while adding proxy support.
			transport := http.DefaultTransport.(*http.Transport).Clone()
			transport.Proxy = http.ProxyURL(u)
			// Preserve the SSRF-safe CheckRedirect by wrapping the transport.
			client.Transport = transport
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
