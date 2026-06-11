package provider

import (
	"fmt"
	"net/http"
	"sync"
	"time"
)

type rateLimitTransport struct {
	base    http.RoundTripper
	limiter *tokenBucket
}

type tokenBucket struct {
	mu       sync.Mutex
	capacity int
	tokens   float64
	last     time.Time
}

func newTokenBucket(rpm int) *tokenBucket {
	if rpm <= 0 {
		return nil
	}
	return &tokenBucket{
		capacity: rpm,
		tokens:   float64(rpm),
		last:     time.Now(),
	}
}

func (b *tokenBucket) allow() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(b.last).Seconds()
	// BUG-8 fix: clock skew (NTP step, container pause/resume) can yield a
	// negative elapsed. Clamp to 0 so we neither grant phantom tokens nor
	// subtract from the bucket.
	if elapsed < 0 {
		elapsed = 0
	}
	b.tokens += elapsed * float64(b.capacity) / 60
	if b.tokens > float64(b.capacity) {
		b.tokens = float64(b.capacity)
	}
	if b.tokens < 1 {
		// Token insufficient — do NOT update b.last so the next call
		// computes elapsed from the last successful consumption, keeping
		// the refill rate accurate.
		return fmt.Errorf("provider rate limit exceeded (capacity=%d): %w", b.capacity, ErrProviderRateLimit)
	}
	b.tokens--
	b.last = now
	return nil
}

func wrapRateLimitTransport(base http.RoundTripper, rpm int) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if rpm <= 0 {
		return base
	}
	return &rateLimitTransport{base: base, limiter: newTokenBucket(rpm)}
}

func (t *rateLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Check if the request context is already cancelled before consuming a token.
	select {
	case <-req.Context().Done():
		return nil, req.Context().Err()
	default:
	}
	if err := t.limiter.allow(); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}
