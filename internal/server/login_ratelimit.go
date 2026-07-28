package server

import (
	"encoding/json"
	"net"
	nethttp "net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"aranea-agents/pkg/loggateway"
)

// loginLimiter is a per-client-IP rate limiter + failure lockout for the
// admin login endpoint. It protects the publicly exposed
// POST /v1/admins/login from online password brute force.
//
// Two independent guards:
//   - token bucket (burst, then 1 token per interval) throttles attempt rate
//   - consecutive auth failures (401/403) lock the IP for lockout duration
type loginLimiter struct {
	mu      sync.Mutex
	buckets map[string]*loginBucket
	now     func() time.Time

	interval    time.Duration
	burst       int
	maxFailures int
	lockout     time.Duration
	idleTTL     time.Duration
}

type loginBucket struct {
	tokens      float64
	lastRefill  time.Time
	failures    int
	lockedUntil time.Time
	lastSeen    time.Time
}

func newLoginLimiter(now func() time.Time) *loginLimiter {
	return &loginLimiter{
		buckets:     make(map[string]*loginBucket),
		now:         now,
		interval:    5 * time.Second,
		burst:       5,
		maxFailures: 5,
		lockout:     15 * time.Minute,
		idleTTL:     10 * time.Minute,
	}
}

// check reports whether the IP may proceed. When locked it returns the
// remaining lock time; when the bucket is empty it returns the wait until
// the next token. A permitted call consumes one token.
func (l *loginLimiter) check(ip string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweepLocked(now)

	b := l.bucketLocked(ip, now)
	if now.Before(b.lockedUntil) {
		return b.lockedUntil.Sub(now), true
	}

	elapsed := now.Sub(b.lastRefill)
	b.tokens = minF(float64(l.burst), b.tokens+elapsed.Seconds()/l.interval.Seconds())
	b.lastRefill = now
	if b.tokens >= 1 {
		b.tokens--
		return 0, false
	}
	return time.Duration((1-b.tokens)*float64(l.interval)), false
}

// record notes the outcome of a login attempt. Only auth failures (401/403)
// count toward lockout; server errors are ignored.
func (l *loginLimiter) record(ip string, success bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	b := l.bucketLocked(ip, now)
	if success {
		b.failures = 0
		return
	}
	b.failures++
	if b.failures >= l.maxFailures {
		b.lockedUntil = now.Add(l.lockout)
		b.failures = 0
	}
}

func (l *loginLimiter) bucketLocked(ip string, now time.Time) *loginBucket {
	b, ok := l.buckets[ip]
	if !ok {
		b = &loginBucket{tokens: float64(l.burst), lastRefill: now}
		l.buckets[ip] = b
	}
	b.lastSeen = now
	return b
}

// sweepLocked evicts idle buckets (not currently locked) to bound memory.
func (l *loginLimiter) sweepLocked(now time.Time) {
	for ip, b := range l.buckets {
		if now.Before(b.lockedUntil) {
			continue
		}
		if now.Sub(b.lastSeen) > l.idleTTL {
			delete(l.buckets, ip)
		}
	}
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// loginRateLimitFilter throttles POST /v1/admins/login per client IP and
// returns 429 with Retry-After when throttled. All other traffic passes
// through untouched.
func loginRateLimitFilter(l *loginLimiter, lg loggateway.Logger) func(nethttp.Handler) nethttp.Handler {
	lg = lg.With(loggateway.Domain("login-ratelimit"))
	return func(next nethttp.Handler) nethttp.Handler {
		return nethttp.HandlerFunc(func(w nethttp.ResponseWriter, r *nethttp.Request) {
			if r.Method != nethttp.MethodPost || r.URL.Path != "/v1/admins/login" {
				next.ServeHTTP(w, r)
				return
			}
			ip := clientIP(r)
			if wait, locked := l.check(ip); wait > 0 || locked {
				reason := "rate limited"
				if locked {
					reason = "locked after repeated failures"
				}
				lg.Warn("login attempt throttled",
					loggateway.Str("client_ip", ip),
					loggateway.Str("reason", reason),
					loggateway.Str("retry_after", wait.String()))
				writeRateLimited(w, wait, locked)
				return
			}
			rec := &statusRecorder{ResponseWriter: w, status: nethttp.StatusOK}
			next.ServeHTTP(rec, r)
			switch rec.status {
			case nethttp.StatusOK:
				l.record(ip, true)
			case nethttp.StatusUnauthorized, nethttp.StatusForbidden:
				l.record(ip, false)
			}
		})
	}
}

func writeRateLimited(w nethttp.ResponseWriter, wait time.Duration, locked bool) {
	reason := "LOGIN_RATE_LIMITED"
	message := "too many login attempts, retry later"
	if locked {
		reason = "LOGIN_LOCKED"
		message = "too many failed login attempts, temporarily locked"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
	w.WriteHeader(nethttp.StatusTooManyRequests)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    nethttp.StatusTooManyRequests,
		"reason":  reason,
		"message": message,
	})
}

// clientIP extracts the end-user IP. The Go backend sits behind frps/Caddy,
// so the direct peer is always 127.0.0.1 and the real client must come from
// the X-Forwarded-For first hop.
func clientIP(r *nethttp.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if idx := strings.IndexByte(xff, ','); idx >= 0 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

type statusRecorder struct {
	nethttp.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
