package provider

import (
	"errors"
	"net"
	"net/http"
	"strings"
)

// FailureKind is a user-facing classification of an LLM provider failure.
// Network/rate-limit failures are retryable; billing/auth/stall are fatal
// and should be shown as a persistent banner instead of a reconnect spinner.
type FailureKind string

const (
	FailureNetwork       FailureKind = "network"
	FailureRateLimit     FailureKind = "rate_limit"
	FailureBilling       FailureKind = "billing"
	FailureAuth          FailureKind = "auth"
	FailureOverflow      FailureKind = "overflow"
	FailureContentFilter FailureKind = "content_filter"
	FailureStall         FailureKind = "stall"
	FailureUnknown       FailureKind = "unknown"
)

// System.notice types surfaced to the chat UI (llm_retry already existed).
const (
	NoticeLLMRetry   = "llm_retry"
	NoticeLLMBilling = "llm_billing"
	NoticeLLMAuth    = "llm_auth"
	NoticeLLMStall   = "llm_stall"
)

// Failure is the classified outcome of an LLM call or stream error.
type Failure struct {
	Kind      FailureKind
	Retryable bool
	Notice    string
	Message   string
}

var billingMarkers = []string{
	"insufficient_quota",
	"insufficient_balance",
	"insufficient balance",
	"insufficient credit",
	"out of credit",
	"out of credits",
	"account_deactivated",
	"billing_not_active",
	"please top up",
	"please recharge",
	"exceeded your current quota",
	"credit is not enough",
	"payment required",
	"余额不足",
	"欠费",
	"账户余额",
	"请充值",
}

var authMarkers = []string{
	"invalid api key",
	"incorrect api key",
	"invalid_api_key",
	"authentication fails",
	"authentication failed",
	"unauthorized",
	"forbidden",
}

var stallMarkers = []string{
	"first byte timeout",
	"first-byte timeout",
}

// ClassifyFailure maps a provider/stream error to a UI-facing failure kind.
// Prefer this over ad-hoc string matching in chat/team/channel layers.
func ClassifyFailure(msg string, err error) Failure {
	combined := strings.ToLower(strings.TrimSpace(msg))
	if err != nil {
		if combined == "" {
			combined = strings.ToLower(err.Error())
		} else {
			combined = combined + " " + strings.ToLower(err.Error())
		}
	}
	switch {
	case matchAny(combined, billingMarkers):
		return Failure{Kind: FailureBilling, Retryable: false, Notice: NoticeLLMBilling, Message: "模型账户欠费或余额不足，请充值后再试"}
	case matchAny(combined, stallMarkers):
		return Failure{Kind: FailureStall, Retryable: false, Notice: NoticeLLMStall, Message: "供应商长时间无响应，已中止本轮"}
	case matchAny(combined, contextOverflowMarkers):
		return Failure{Kind: FailureOverflow, Retryable: false, Notice: "", Message: "请求超出模型上下文长度"}
	case matchAny(combined, contentFilterMarkers):
		return Failure{Kind: FailureContentFilter, Retryable: false, Notice: "", Message: "内容被供应商安全策略拦截"}
	case matchAny(combined, authMarkers):
		return Failure{Kind: FailureAuth, Retryable: false, Notice: NoticeLLMAuth, Message: "模型鉴权失败，请检查供应商密钥"}
	}

	if err != nil {
		var attemptTimeout *attemptTimeoutError
		if errors.As(err, &attemptTimeout) {
			return Failure{Kind: FailureNetwork, Retryable: true, Notice: NoticeLLMRetry, Message: "模型连接中断，正在自动重试"}
		}
		var netErr net.Error
		if errors.As(err, &netErr) {
			return Failure{Kind: FailureNetwork, Retryable: true, Notice: NoticeLLMRetry, Message: "模型连接中断，正在自动重试"}
		}
	}

	if strings.Contains(combined, "429") || strings.Contains(combined, "rate limit") || strings.Contains(combined, "too many requests") {
		return Failure{Kind: FailureRateLimit, Retryable: true, Notice: NoticeLLMRetry, Message: "模型调用过于频繁，正在等待后重试"}
	}
	return Failure{Kind: FailureUnknown, Retryable: false, Notice: "", Message: "模型调用失败"}
}

// ClassifyHTTPFailure classifies an HTTP status (and optional body) from the
// provider. 402 and billing-marked 401/403 are fatal billing failures rather
// than credential-rebuild retries.
func ClassifyHTTPFailure(status int, body string) Failure {
	lower := strings.ToLower(body)
	if status == http.StatusPaymentRequired || matchAny(lower, billingMarkers) {
		return Failure{Kind: FailureBilling, Retryable: false, Notice: NoticeLLMBilling, Message: "模型账户欠费或余额不足，请充值后再试"}
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		if matchAny(lower, billingMarkers) {
			return Failure{Kind: FailureBilling, Retryable: false, Notice: NoticeLLMBilling, Message: "模型账户欠费或余额不足，请充值后再试"}
		}
		return Failure{Kind: FailureAuth, Retryable: false, Notice: NoticeLLMAuth, Message: "模型鉴权失败，请检查供应商密钥"}
	}
	if status == http.StatusTooManyRequests {
		return Failure{Kind: FailureRateLimit, Retryable: true, Notice: NoticeLLMRetry, Message: "模型调用过于频繁，正在等待后重试"}
	}
	if status >= 500 {
		return Failure{Kind: FailureNetwork, Retryable: true, Notice: NoticeLLMRetry, Message: "供应商暂时不可用，正在自动重试"}
	}
	return ClassifyFailure(body, nil)
}
