package provider

import "net/http"

// RoundTrip carries the HTTP client for vendor calls (timeouts set by caller / wire).
type RoundTrip struct {
	HTTP *http.Client
}

// Client returns a non-nil HTTP client.
func (rt *RoundTrip) Client() *http.Client {
	if rt == nil || rt.HTTP == nil {
		return http.DefaultClient
	}
	return rt.HTTP
}

func roundOrNil(rt *RoundTrip) *RoundTrip {
	if rt == nil {
		return &RoundTrip{}
	}
	return rt
}
