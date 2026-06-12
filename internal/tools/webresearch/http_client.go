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
			transport := client.Transport.(*http.Transport)
			if transport == nil {
				transport = http.DefaultTransport.(*http.Transport).Clone()
			} else {
				transport = transport.Clone()
			}
			transport.Proxy = http.ProxyURL(u)
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
