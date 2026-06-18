package provider

import (
	"fmt"
	"net/http"
	"time"

	"aranea-agents/pkg/loggateway"
)

// RetryCallback is invoked before each retry attempt (i.e. after a failure
// and before sleeping the backoff delay). attempt is 1-indexed (first retry
// = 1). maxRetries is the configured cap (-1 = infinite). delay is the
// backoff duration that will be slept. err is the error that triggered the
// retry. req is the original HTTP request; the callback can extract context
// info (e.g. session ID) from it.
type RetryCallback func(req *http.Request, attempt, maxRetries int, err error, delay time.Duration)

// retryTransport wraps a base http.RoundTripper with exponential backoff retry.
// It retries on server errors (5xx) and 429 (rate limited), up to maxRetries
// additional attempts beyond the initial one. A maxRetries of -1 means
// infinite retry (bounded only by request context cancellation).
type retryTransport struct {
	base       http.RoundTripper
	maxRetries int // -1 = infinite, 0 = disabled (should not reach here), >0 = finite
	baseDelay  time.Duration
	maxDelay   time.Duration
	lg         loggateway.Logger
	onRetry    RetryCallback // optional; called before each retry
}

func newRetryTransport(base http.RoundTripper, maxRetries int, baseDelay, maxDelay time.Duration, lg loggateway.Logger, onRetry RetryCallback) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	// maxRetries == 0 means disabled; caller should not invoke us, but guard anyway.
	if maxRetries == 0 {
		return base
	}
	return &retryTransport{base: base, maxRetries: maxRetries, baseDelay: baseDelay, maxDelay: maxDelay, lg: lg, onRetry: onRetry}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var lastErr error
	attempt := 0
	for {
		// On retry, reset the request body so the full payload is sent again.
		// Without this, the body ReadCloser is already at EOF after the first
		// attempt and the retried request would be sent with an empty body.
		if attempt > 0 {
			if err := resetRequestBody(req); err != nil {
				return nil, fmt.Errorf("retry: reset body: %w", err)
			}
			delay := t.baseDelay * time.Duration(1<<(attempt-1)) // exponential backoff
			if delay > t.maxDelay {
				delay = t.maxDelay
			}
			// Notify callback before sleeping so the frontend can show
			// "正在重试" feedback.
			if t.onRetry != nil {
				t.onRetry(req, attempt, t.maxRetries, lastErr, delay)
			}
			select {
			case <-req.Context().Done():
				return nil, req.Context().Err()
			case <-time.After(delay):
			}
		}
		resp, err := t.base.RoundTrip(req)
		if err != nil {
			lastErr = err
			if t.lg != nil {
				t.lg.Warn("provider 请求失败，准备重试",
					loggateway.StepID("provider.retry"),
					loggateway.Int("attempt", attempt+1),
					loggateway.Int("max_retries", t.maxRetries),
					loggateway.Err(err))
			}
			if t.shouldRetry(attempt) {
				attempt++
				continue
			}
			return nil, lastErr
		}
		// Retry on server errors (5xx) and 429 (rate limited).
		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			resp.Body.Close()
			lastErr = fmt.Errorf("server returned %d", resp.StatusCode)
			if t.lg != nil {
				t.lg.Warn("provider 响应错误，准备重试",
					loggateway.StepID("provider.retry"),
					loggateway.Int("attempt", attempt+1),
					loggateway.Int("max_retries", t.maxRetries),
					loggateway.Int("status_code", resp.StatusCode))
			}
			if t.shouldRetry(attempt) {
				attempt++
				continue
			}
			return nil, lastErr
		}
		return resp, nil
	}
}

// shouldRetry returns true if the transport should attempt another retry
// after the given attempt index (0-indexed: attempt 0 = first try).
func (t *retryTransport) shouldRetry(attempt int) bool {
	if t.maxRetries < 0 {
		return true // infinite
	}
	return attempt < t.maxRetries
}

// resetRequestBody rewinds the request body using GetBody, which is set by
// http.NewRequest for common body types ([]byte, strings.Reader, bytes.Reader).
// If GetBody is nil (e.g. streaming body), retry is not possible and an error
// is returned.
func resetRequestBody(req *http.Request) error {
	if req.Body == nil {
		return nil
	}
	if req.GetBody == nil {
		return fmt.Errorf("request body cannot be retried: GetBody is nil")
	}
	body, err := req.GetBody()
	if err != nil {
		return fmt.Errorf("GetBody: %w", err)
	}
	req.Body = body
	return nil
}
