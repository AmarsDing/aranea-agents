package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

func newTestLimiter(start time.Time) (*loginLimiter, *time.Time) {
	now := &start
	l := newLoginLimiter(func() time.Time { return *now })
	return l, now
}

func TestLoginLimiterAllowsWithinBurst(t *testing.T) {
	l, _ := newTestLimiter(time.Now())
	for i := 0; i < l.burst; i++ {
		if wait, locked := l.check("1.2.3.4"); locked || wait > 0 {
			t.Fatalf("attempt %d should pass, got wait=%v locked=%v", i, wait, locked)
		}
	}
}

func TestLoginLimiterRejectsWhenBucketExhausted(t *testing.T) {
	l, _ := newTestLimiter(time.Now())
	for i := 0; i < l.burst; i++ {
		l.check("1.2.3.4")
	}
	wait, locked := l.check("1.2.3.4")
	if locked {
		t.Fatal("should not be locked, only rate limited")
	}
	if wait <= 0 {
		t.Fatal("expected positive retry wait")
	}
}

func TestLoginLimiterRefillsAfterInterval(t *testing.T) {
	start := time.Now()
	l, now := newTestLimiter(start)
	for i := 0; i < l.burst; i++ {
		l.check("1.2.3.4")
	}
	*now = start.Add(l.interval + time.Millisecond)
	if wait, locked := l.check("1.2.3.4"); locked || wait > 0 {
		t.Fatalf("should pass after refill, got wait=%v locked=%v", wait, locked)
	}
}

func TestLoginLimiterLocksAfterMaxFailures(t *testing.T) {
	start := time.Now()
	l, now := newTestLimiter(start)
	ip := "5.6.7.8"
	for i := 0; i < l.maxFailures; i++ {
		l.record(ip, false)
	}
	wait, locked := l.check(ip)
	if !locked {
		t.Fatal("should be locked after max failures")
	}
	if wait <= 0 || wait > l.lockout {
		t.Fatalf("unexpected lock wait: %v", wait)
	}
	// Still locked mid-window.
	*now = start.Add(l.lockout / 2)
	if _, locked := l.check(ip); !locked {
		t.Fatal("should still be locked inside lockout window")
	}
	// Released after lockout.
	*now = start.Add(l.lockout + time.Millisecond)
	if _, locked := l.check(ip); locked {
		t.Fatal("should be released after lockout")
	}
}

func TestLoginLimiterSuccessResetsFailures(t *testing.T) {
	l, _ := newTestLimiter(time.Now())
	ip := "5.6.7.8"
	for i := 0; i < l.maxFailures-1; i++ {
		l.record(ip, false)
	}
	l.record(ip, true)
	for i := 0; i < l.maxFailures-1; i++ {
		l.record(ip, false)
	}
	if _, locked := l.check(ip); locked {
		t.Fatal("success should have reset failure count")
	}
}

func TestLoginLimiterBucketsArePerIP(t *testing.T) {
	l, _ := newTestLimiter(time.Now())
	for i := 0; i < l.maxFailures; i++ {
		l.record("1.1.1.1", false)
	}
	if _, locked := l.check("2.2.2.2"); locked {
		t.Fatal("other IP must not be affected")
	}
}

func TestLoginLimiterEvictsIdleBuckets(t *testing.T) {
	start := time.Now()
	l, now := newTestLimiter(start)
	l.check("1.2.3.4")
	*now = start.Add(l.idleTTL + time.Millisecond)
	l.check("9.9.9.9") // triggers sweep
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.buckets["1.2.3.4"]; ok {
		t.Fatal("idle bucket should be evicted")
	}
}

// ─── HTTP filter ────────────────────────────────────────────────────────────

func loginRequest(t *testing.T, remoteAddr, xff string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/v1/admins/login", strings.NewReader(`{"username":"a","password":"b"}`))
	r.RemoteAddr = remoteAddr
	if xff != "" {
		r.Header.Set("X-Forwarded-For", xff)
	}
	return r
}

func runFilter(l *loginLimiter, r *http.Request, status int) *httptest.ResponseRecorder {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	})
	rec := httptest.NewRecorder()
	loginRateLimitFilter(l, loggateway.NewNoop())(next).ServeHTTP(rec, r)
	return rec
}

func TestFilterPassesNonLoginPaths(t *testing.T) {
	l, _ := newTestLimiter(time.Now())
	r := httptest.NewRequest(http.MethodGet, "/v1/agents", nil)
	rec := runFilter(l, r, http.StatusOK)
	if rec.Code != http.StatusOK {
		t.Fatalf("non-login path should pass through, got %d", rec.Code)
	}
}

func TestFilterRejectsExcessAttemptsWith429(t *testing.T) {
	l, _ := newTestLimiter(time.Now())
	r := loginRequest(t, "10.0.0.1:1234", "")
	var rec *httptest.ResponseRecorder
	for i := 0; i < l.burst; i++ {
		rec = runFilter(l, loginRequest(t, "10.0.0.1:1234", ""), http.StatusOK)
	}
	rec = runFilter(l, r, http.StatusOK)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header")
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response should be JSON: %v", err)
	}
	if body["reason"] != "LOGIN_RATE_LIMITED" {
		t.Fatalf("unexpected reason: %v", body["reason"])
	}
}

func TestFilterCountsFailuresAndLocks(t *testing.T) {
	l, _ := newTestLimiter(time.Now())
	ip := "10.0.0.9"
	for i := 0; i < l.maxFailures; i++ {
		rec := runFilter(l, loginRequest(t, ip+":1234", ""), http.StatusUnauthorized)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("failure %d should pass through to client, got %d", i, rec.Code)
		}
	}
	// Burst may still allow the request into the limiter, but lock must trigger 429.
	rec := runFilter(l, loginRequest(t, ip+":1234", ""), http.StatusOK)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("locked IP should get 429, got %d", rec.Code)
	}
	ra, _ := strconv.Atoi(rec.Header().Get("Retry-After"))
	if ra <= 0 {
		t.Fatalf("expected positive Retry-After, got %q", rec.Header().Get("Retry-After"))
	}
}

func TestFilterSuccessClearsFailureCount(t *testing.T) {
	l, _ := newTestLimiter(time.Now())
	ip := "10.0.0.10"
	for i := 0; i < l.maxFailures-1; i++ {
		runFilter(l, loginRequest(t, ip+":1234", ""), http.StatusUnauthorized)
	}
	runFilter(l, loginRequest(t, ip+":1234", ""), http.StatusOK)
	l.mu.Lock()
	failures := l.buckets[ip].failures
	l.mu.Unlock()
	if failures != 0 {
		t.Fatalf("success should reset failures, got %d", failures)
	}
}

func TestFilterIgnoresServerErrors(t *testing.T) {
	l, _ := newTestLimiter(time.Now())
	ip := "10.0.0.11"
	for i := 0; i < l.maxFailures+2; i++ {
		runFilter(l, loginRequest(t, ip+":1234", ""), http.StatusInternalServerError)
	}
	if _, locked := l.check(ip); locked {
		t.Fatal("5xx must not count toward lockout")
	}
}

func TestFilterPrefersXForwardedFor(t *testing.T) {
	l, _ := newTestLimiter(time.Now())
	for i := 0; i < l.maxFailures; i++ {
		runFilter(l, loginRequest(t, "127.0.0.1:1234", "203.0.113.7, 70.41.3.18"), http.StatusUnauthorized)
	}
	// The XFF first hop must be the locked identity, not the proxy peer.
	if _, locked := l.check("203.0.113.7"); !locked {
		t.Fatal("expected lockout keyed by XFF first hop")
	}
	if _, locked := l.check("127.0.0.1"); locked {
		t.Fatal("proxy peer must not be locked")
	}
}

func TestFilterConcurrentSafety(t *testing.T) {
	l, _ := newTestLimiter(time.Now())
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runFilter(l, loginRequest(t, "10.1.1.1:1234", ""), http.StatusUnauthorized)
		}()
	}
	wg.Wait()
}
