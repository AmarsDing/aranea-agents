package webresearch

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

func buildHTTPClient(cfg Config, lg loggateway.Logger) *http.Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeoutSec * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL := strings.TrimSpace(cfg.HTTPProxy); proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		} else {
			lg.Warn("failed to parse proxy URL",
				loggateway.StepID("tool.webresearch.proxy_parse_fail"),
				loggateway.Str("proxy_url", proxyURL),
				loggateway.Err(err))
		}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
