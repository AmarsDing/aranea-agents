package client

import (
	"math/rand"
	"net/http"
	"strconv"
	"time"
)

// retryDoer wraps http.Client with retry logic for idempotent requests.
type retryDoer struct {
	inner *http.Client
}

func (d *retryDoer) Do(req *http.Request) (*http.Response, error) {
	// Only retry idempotent methods: GET, HEAD, OPTIONS.
	if !isIdempotent(req.Method) {
		return d.inner.Do(req)
	}
	return doWithRetry(d.inner, req, 3)
}

// doWithRetry attempts the request up to maxRetries times with exponential backoff.
func doWithRetry(c *http.Client, req *http.Request, maxRetries int) (*http.Response, error) {
	var (
		resp *http.Response
		err  error
	)
	delays := []time.Duration{200 * time.Millisecond, 600 * time.Millisecond, 2 * time.Second}
	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err = c.Do(req)
		if err == nil && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			return resp, nil
		}
		if err == nil {
			resp.Body.Close()
		}
		if attempt < len(delays) {
			time.Sleep(retryDelay(resp, delays[attempt]))
		}
	}
	return resp, err
}

func retryDelay(resp *http.Response, fallback time.Duration) time.Duration {
	if resp != nil {
		if v := resp.Header.Get("Retry-After"); v != "" {
			if seconds, err := strconv.Atoi(v); err == nil && seconds >= 0 {
				return time.Duration(seconds) * time.Second
			}
			if at, err := http.ParseTime(v); err == nil {
				if d := time.Until(at); d > 0 {
					return d
				}
			}
		}
	}
	jitter := 0.8 + rand.Float64()*0.4
	return time.Duration(float64(fallback) * jitter)
}

func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}
