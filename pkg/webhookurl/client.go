package webhookurl

import (
	"net/http"
	"time"
)

// NewOutboundHTTPClient returns a client for webhook POSTs: per-request timeout, no redirects.
func NewOutboundHTTPClient(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}
