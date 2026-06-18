package provider

import "net/http"

// RoundTrip carries the HTTP client for vendor calls (timeouts set by caller / wire).
// OnRetry is an optional callback invoked by the retry transport before each
// retry attempt. It lets higher layers (with event bus access) publish
// llm_retry events without the provider layer depending on the event package.
type RoundTrip struct {
	HTTP    *http.Client
	OnRetry RetryCallback
}

// Client returns a non-nil HTTP client.
func (rt *RoundTrip) Client() *http.Client {
	if rt == nil || rt.HTTP == nil {
		return http.DefaultClient
	}
	return rt.HTTP
}
