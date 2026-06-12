package provider

import (
	"fmt"
	"net/http"
	"time"

	"aranea-agents/pkg/loggateway"
)

// retryTransport wraps a base http.RoundTripper with exponential backoff retry.
// It retries on server errors (5xx) and 429 (rate limited), up to maxRetries
// additional attempts beyond the initial one.
type retryTransport struct {
	base       http.RoundTripper
	maxRetries int
	baseDelay  time.Duration
	maxDelay   time.Duration
	lg         loggateway.Logger
}

func newRetryTransport(base http.RoundTripper, maxRetries int, baseDelay, maxDelay time.Duration, lg loggateway.Logger) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if maxRetries <= 0 {
		return base
	}
	return &retryTransport{base: base, maxRetries: maxRetries, baseDelay: baseDelay, maxDelay: maxDelay, lg: lg}
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var lastErr error
	for attempt := 0; attempt <= t.maxRetries; attempt++ {
		if attempt > 0 {
			delay := t.baseDelay * time.Duration(1<<(attempt-1)) // exponential backoff
			if delay > t.maxDelay {
				delay = t.maxDelay
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
			continue
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
			continue
		}
		return resp, nil
	}
	return nil, lastErr
}
