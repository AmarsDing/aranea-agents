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
	b.last = now
	b.tokens += elapsed * float64(b.capacity) / 60
	if b.tokens > float64(b.capacity) {
		b.tokens = float64(b.capacity)
	}
	if b.tokens < 1 {
		return fmt.Errorf("provider rate limit exceeded")
	}
	b.tokens--
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
	if err := t.limiter.allow(); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}
