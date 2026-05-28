package webresearch

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"aranea-agents/internal/event"
)

func buildHTTPClient(cfg Config) *http.Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeoutSec * time.Second
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if proxyURL := strings.TrimSpace(cfg.HTTPProxy); proxyURL != "" {
		if u, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(u)
		} else {
			event.SysLogWarn("webresearch.proxy_parse", fmt.Sprintf("failed to parse proxy URL %q: %v", proxyURL, err))
		}
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
