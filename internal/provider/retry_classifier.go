package provider

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
)

// RetryDecisionType is the classified action for a failed LLM request.
type RetryDecisionType int

const (
	// Retry retries immediately with the same configuration.
	Retry RetryDecisionType = iota
	// RetryWithBackoff retries after a backoff delay (transient errors, 429, 5xx).
	RetryWithBackoff
	// RetryWithImageStrip retries after stripping image content (upper layer handles).
	RetryWithImageStrip
	// RetryWithClientRebuild retries after rebuilding the client (e.g. credential refresh).
	RetryWithClientRebuild
	// EmitToSession surfaces the failure to the session instead of retrying.
	EmitToSession
	// RetryFatal does not retry; the error propagates to the caller.
	RetryFatal
)

func (t RetryDecisionType) String() string {
	switch t {
	case Retry:
		return "retry"
	case RetryWithBackoff:
		return "retry_with_backoff"
	case RetryWithImageStrip:
		return "retry_with_image_strip"
	case RetryWithClientRebuild:
		return "retry_with_client_rebuild"
	case EmitToSession:
		return "emit_to_session"
	case RetryFatal:
		return "retry_fatal"
	default:
		return "unknown"
	}
}

// RetryDecision is the output of ClassifyRetry. MaxAttempts == 0 means "use
// the transport/caller default"; BackoffStrategy is "exponential" or
// "retry_after" (server-provided).
type RetryDecision struct {
	Type            RetryDecisionType
	IsRateLimited   bool
	MaxAttempts     int
	BackoffStrategy string
}

// contextOverflowMarkers match provider "request too large" errors. Retrying
// an identical oversized payload is pointless, so these are fatal.
var contextOverflowMarkers = []string{
	"context length exceeded",
	"context_length_exceeded",
	"context window",
	"maximum context length",
	"token limit exceeded",
	"request too large",
}

// contentFilterMarkers match provider safety-system rejections; these should
// be surfaced to the session rather than retried.
var contentFilterMarkers = []string{
	"content_filter",
	"content filter",
	"safety system",
	"blocked by",
	"moderation",
}

// imageErrorMarkers match model rejections of image content; an upper layer
// may strip the images and retry.
var imageErrorMarkers = []string{
	"unsupported image",
	"image format",
	"invalid image",
	"image_url",
}

func matchAny(errMsg string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(errMsg, m) {
			return true
		}
	}
	return false
}

// ClassifyRetry is a pure function mapping (response, error) to a retry
// decision. No I/O, no logging, no clock — all time-based decisions stay in
// the transport. Callers pass resp == nil for transport-level errors and
// err == nil for HTTP error responses.
func ClassifyRetry(resp *http.Response, err error) RetryDecision {
	if err != nil {
		return classifyError(err)
	}
	if resp != nil {
		return classifyStatus(resp.StatusCode)
	}
	return RetryDecision{Type: RetryFatal, BackoffStrategy: "exponential"}
}

func classifyError(err error) RetryDecision {
	// Client-side cancellation must not be retried.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return RetryDecision{Type: RetryFatal, BackoffStrategy: "exponential"}
	}
	msg := strings.ToLower(err.Error())
	switch {
	case matchAny(msg, contextOverflowMarkers):
		return RetryDecision{Type: RetryFatal, BackoffStrategy: "exponential"}
	case matchAny(msg, contentFilterMarkers):
		return RetryDecision{Type: EmitToSession, BackoffStrategy: "exponential"}
	case matchAny(msg, imageErrorMarkers):
		return RetryDecision{Type: RetryWithImageStrip, BackoffStrategy: "exponential"}
	}
	// Transient network/IO errors and unknown errors: retry with backoff
	// (preserves the historical behavior of retrying all transport errors).
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return RetryDecision{Type: RetryWithBackoff, BackoffStrategy: "exponential"}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return RetryDecision{Type: RetryWithBackoff, BackoffStrategy: "exponential"}
	}
	return RetryDecision{Type: RetryWithBackoff, BackoffStrategy: "exponential"}
}

func classifyStatus(code int) RetryDecision {
	switch {
	case code == http.StatusTooManyRequests:
		return RetryDecision{Type: RetryWithBackoff, IsRateLimited: true, BackoffStrategy: "retry_after"}
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		// Credentials may be stale; an upper layer rebuilds the client.
		return RetryDecision{Type: RetryWithClientRebuild, BackoffStrategy: "exponential"}
	case code >= 500:
		return RetryDecision{Type: RetryWithBackoff, BackoffStrategy: "retry_after"}
	case code == http.StatusRequestEntityTooLarge:
		return RetryDecision{Type: RetryFatal, BackoffStrategy: "exponential"}
	case code >= 400:
		// Other 4xx: retrying an identical request is pointless.
		return RetryDecision{Type: RetryFatal, BackoffStrategy: "exponential"}
	default:
		return RetryDecision{Type: Retry, BackoffStrategy: "exponential"}
	}
}
