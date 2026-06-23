package provider

import (
	"fmt"
	"net/http"

	biztool "aranea-agents/internal/biz/tool"
	"aranea-agents/pkg/loggateway"
)

// circuitBreakerTransport wraps a base http.RoundTripper with circuit breaker
// protection. When the circuit breaker is open, requests fail fast without
// hitting the provider.
type circuitBreakerTransport struct {
	base http.RoundTripper
	cb   *biztool.CircuitBreaker
	lg   loggateway.Logger
}

func newCircuitBreakerTransport(base http.RoundTripper, cb *biztool.CircuitBreaker, lg loggateway.Logger) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &circuitBreakerTransport{base: base, cb: cb, lg: lg}
}

func (t *circuitBreakerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	allowed, state := t.cb.Allow()
	if !allowed {
		if t.lg != nil {
			t.lg.Warn("provider 熔断器开启，请求被拒绝",
				loggateway.StepID("provider.circuit_breaker"),
				loggateway.Str("breaker_name", t.cb.Name()),
				loggateway.Str("state", string(state)))
		}
		return nil, fmt.Errorf("circuit breaker open for provider %s", t.cb.Name())
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		t.cb.RecordFailure()
		return resp, err
	}
	// 429（限流）和 5xx（服务端错误）都表示 provider 不可用，
	// 应触发熔断。否则 retry 层会无限重试 429，而熔断器永不开启，
	// 形成对已过载 provider 的持续请求（雪崩）。
	if resp.StatusCode >= 500 || resp.StatusCode == 429 {
		t.cb.RecordFailure()
		return resp, nil
	}
	t.cb.RecordSuccess()
	return resp, nil
}
