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
// OnAbnormalFinish is an optional finish_reason/EOF 观测钩子（Q5 批 D3，
// 94 三件套）：trpc_llm 在 LivenessGuard 之外把单个逻辑 GenerateContent 的
// 纯事实快照交给它，nil 时观测层整体 pass-through（零行为变更）。
type RoundTrip struct {
	HTTP             *http.Client
	OnRetry          RetryCallback
	FlowBus          contract.MonitorBus
	OnAbnormalFinish FinishObservedHook
}

// Client returns a non-nil HTTP client.
func (rt *RoundTrip) Client() *http.Client {
	if rt == nil || rt.HTTP == nil {
		return http.DefaultClient
	}
	return rt.HTTP
}
