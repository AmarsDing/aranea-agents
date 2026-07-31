package provider

import (
	"net/http"

	"aranea-agents/internal/event/contract"
)

// RoundTrip carries the HTTP client for vendor calls (timeouts set by caller / wire).
// OnRetry is an optional callback invoked by the retry transport before each
// retry attempt. It lets higher layers (with event bus access) publish
// llm_retry events without the provider layer depending on the event package.
// FlowBus is an optional monitor event bus enabling user-visible flow logs
// (流程日志, e.g. HA 切换) from the provider layer; nil disables emission.
type RoundTrip struct {
	HTTP    *http.Client
	OnRetry RetryCallback
	FlowBus contract.MonitorBus
}

// Client returns a non-nil HTTP client.
func (rt *RoundTrip) Client() *http.Client {
	if rt == nil || rt.HTTP == nil {
		return http.DefaultClient
	}
	return rt.HTTP
}
