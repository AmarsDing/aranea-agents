package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// stubTransport is a controllable http.RoundTripper for testing.
type stubTransport struct {
	// responses: each RoundTrip call pops the next response. If responses is
	// exhausted, the last entry is reused.
	responses []*http.Response
	// errors: parallel to responses; if non-nil, RoundTrip returns this error.
	errs []error
	// callCount tracks how many times RoundTrip was invoked.
	callCount int32
	// delay simulates transport latency.
	delay time.Duration
}

func (s *stubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	atomic.AddInt32(&s.callCount, 1)
	if s.delay > 0 {
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-time.After(s.delay):
		}
	}
	idx := int(atomic.LoadInt32(&s.callCount)) - 1
	if idx >= len(s.responses) {
		idx = len(s.responses) - 1
	}
	var resp *http.Response
	var err error
	if idx < len(s.responses) {
		resp = s.responses[idx]
	}
	if idx < len(s.errs) {
		err = s.errs[idx]
	}
	return resp, err
}

func newOKResponse() *http.Response {
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(bytes.NewReader([]byte("ok"))),
		Header:     make(http.Header),
	}
}

func newErrorResponse(status int) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader([]byte(fmt.Sprintf("error %d", status)))),
		Header:     make(http.Header),
	}
}

func newRetryTransportForTest(base http.RoundTripper, maxRetries int, onRetry RetryCallback) http.RoundTripper {
	return newRetryTransport(base, maxRetries, 1*time.Millisecond, 10*time.Millisecond, loggateway.NewNoop(), onRetry)
}

func TestRetryTransport_SuccessNoRetry(t *testing.T) {
	stub := &stubTransport{
		responses: []*http.Response{newOKResponse()},
	}
	rt := newRetryTransportForTest(stub, 3, nil)

	req, err := http.NewRequestWithContext(context.Background(), "GET", "http://test", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&stub.callCount); got != 1 {
		t.Errorf("callCount = %d, want 1", got)
	}
}

func TestRetryTransport_5xxThenSuccess(t *testing.T) {
	stub := &stubTransport{
		responses: []*http.Response{
			newErrorResponse(500),
			newErrorResponse(503),
			newOKResponse(),
		},
	}
	var retryCalls int32
	rt := newRetryTransportForTest(stub, 5, func(req *http.Request, attempt, maxRetries int, err error, delay time.Duration) {
		atomic.AddInt32(&retryCalls, 1)
	})

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&stub.callCount); got != 3 {
		t.Errorf("callCount = %d, want 3", got)
	}
	if got := atomic.LoadInt32(&retryCalls); got != 2 {
		t.Errorf("retryCalls = %d, want 2", got)
	}
}

func TestRetryTransport_429ThenSuccess(t *testing.T) {
	stub := &stubTransport{
		responses: []*http.Response{
			newErrorResponse(429),
			newOKResponse(),
		},
	}
	rt := newRetryTransportForTest(stub, 3, nil)

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&stub.callCount); got != 2 {
		t.Errorf("callCount = %d, want 2", got)
	}
}

func TestRetryTransport_NetworkErrorThenSuccess(t *testing.T) {
	stub := &stubTransport{
		responses: []*http.Response{nil, newOKResponse()},
		errs: []error{
			errors.New("connection refused"),
			nil,
		},
	}
	rt := newRetryTransportForTest(stub, 3, nil)

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&stub.callCount); got != 2 {
		t.Errorf("callCount = %d, want 2", got)
	}
}

func TestRetryTransport_MaxRetriesExhausted(t *testing.T) {
	stub := &stubTransport{
		responses: []*http.Response{newErrorResponse(500)},
	}
	rt := newRetryTransportForTest(stub, 2, nil)

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://test", nil)
	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// 1 initial + 2 retries = 3 total
	if got := atomic.LoadInt32(&stub.callCount); got != 3 {
		t.Errorf("callCount = %d, want 3", got)
	}
}

func TestRetryTransport_InfiniteRetry(t *testing.T) {
	// Simulate 5 failures then success with maxRetries=-1 (infinite).
	responses := make([]*http.Response, 0, 6)
	for i := 0; i < 5; i++ {
		responses = append(responses, newErrorResponse(500))
	}
	responses = append(responses, newOKResponse())
	stub := &stubTransport{responses: responses}
	rt := newRetryTransportForTest(stub, -1, nil)

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if got := atomic.LoadInt32(&stub.callCount); got != 6 {
		t.Errorf("callCount = %d, want 6", got)
	}
}

// TestRetryTransport_ConnRefusedCappedUnderInfinitePolicy verifies that an
// ECONNREFUSED-class error terminates after connRefusedMaxAttempts retries
// even when the transport is configured for infinite retry (-1): the
// per-error-class cap from ClassifyRetry overrides the infinite default.
func TestRetryTransport_ConnRefusedCappedUnderInfinitePolicy(t *testing.T) {
	connRefused := &net.OpError{Op: "dial", Net: "tcp", Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED}}
	stub := &stubTransport{
		responses: []*http.Response{nil},
		errs:      []error{connRefused},
	}
	rt := newRetryTransportForTest(stub, -1, nil)

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://test", nil)
	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error after cap, got nil")
	}
	// 1 initial + connRefusedMaxAttempts retries.
	want := int32(1 + connRefusedMaxAttempts)
	if got := atomic.LoadInt32(&stub.callCount); got != want {
		t.Errorf("callCount = %d, want %d", got, want)
	}
}

// TestRetryTransport_ConnRefusedTransportCapTighter verifies that when the
// transport cap is tighter than the per-error-class cap, the transport cap
// wins (min semantics).
func TestRetryTransport_ConnRefusedTransportCapTighter(t *testing.T) {
	connRefused := &net.OpError{Op: "dial", Net: "tcp", Err: &os.SyscallError{Syscall: "connect", Err: syscall.ECONNREFUSED}}
	stub := &stubTransport{
		responses: []*http.Response{nil},
		errs:      []error{connRefused},
	}
	rt := newRetryTransportForTest(stub, 1, nil)

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://test", nil)
	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected error after cap, got nil")
	}
	// 1 initial + 1 retry (transport cap tighter than connRefusedMaxAttempts).
	if got := atomic.LoadInt32(&stub.callCount); got != 2 {
		t.Errorf("callCount = %d, want 2", got)
	}
}

func TestRetryTransport_4xxNoRetry(t *testing.T) {
	stub := &stubTransport{
		responses: []*http.Response{newErrorResponse(400)},
	}
	rt := newRetryTransportForTest(stub, 3, nil)

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	// 4xx should not retry
	if got := atomic.LoadInt32(&stub.callCount); got != 1 {
		t.Errorf("callCount = %d, want 1", got)
	}
}

func TestRetryTransport_ContextCancelStopsRetry(t *testing.T) {
	stub := &stubTransport{
		responses: []*http.Response{newErrorResponse(500)},
		delay:     5 * time.Millisecond,
	}
	rt := newRetryTransportForTest(stub, -1, nil)

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after first attempt to stop the infinite retry loop.
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	req, _ := http.NewRequestWithContext(ctx, "GET", "http://test", nil)
	_, err := rt.RoundTrip(req)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestRetryTransport_CallbackInvoked(t *testing.T) {
	stub := &stubTransport{
		responses: []*http.Response{
			newErrorResponse(500),
			newOKResponse(),
		},
	}
	var attempts []int
	var delays []time.Duration
	rt := newRetryTransportForTest(stub, 3, func(req *http.Request, attempt, maxRetries int, err error, delay time.Duration) {
		attempts = append(attempts, attempt)
		delays = append(delays, delay)
	})

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if len(attempts) != 1 {
		t.Errorf("callback calls = %d, want 1", len(attempts))
	}
	if attempts[0] != 1 {
		t.Errorf("attempt[0] = %d, want 1", attempts[0])
	}
	// First retry delay = baseDelay * 2^0 = 1ms
	if delays[0] != 1*time.Millisecond {
		t.Errorf("delay[0] = %v, want 1ms", delays[0])
	}
}

func TestRetryTransport_ExponentialBackoff(t *testing.T) {
	stub := &stubTransport{
		responses: []*http.Response{
			newErrorResponse(500),
			newErrorResponse(500),
			newErrorResponse(500),
			newOKResponse(),
		},
	}
	var delays []time.Duration
	rt := newRetryTransportForTest(stub, 5, func(req *http.Request, attempt, maxRetries int, err error, delay time.Duration) {
		delays = append(delays, delay)
	})

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	// Delays: 1ms, 2ms, 4ms (baseDelay * 2^(attempt-1))
	expected := []time.Duration{1 * time.Millisecond, 2 * time.Millisecond, 4 * time.Millisecond}
	if len(delays) != len(expected) {
		t.Fatalf("delays len = %d, want %d", len(delays), len(expected))
	}
	for i, want := range expected {
		if delays[i] != want {
			t.Errorf("delay[%d] = %v, want %v", i, delays[i], want)
		}
	}
}

func TestRetryTransport_BackoffCappedAtMaxDelay(t *testing.T) {
	stub := &stubTransport{
		responses: []*http.Response{
			newErrorResponse(500),
			newErrorResponse(500),
			newErrorResponse(500),
			newErrorResponse(500),
			newOKResponse(),
		},
	}
	var delays []time.Duration
	// maxDelay = 10ms, so delays should cap at 10ms
	rt := newRetryTransportForTest(stub, 10, func(req *http.Request, attempt, maxRetries int, err error, delay time.Duration) {
		delays = append(delays, delay)
	})

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	// Delays: 1ms, 2ms, 4ms, 8ms (all under 10ms cap)
	// 5th retry would be 16ms but capped at 10ms — but we only have 4 retries here
	for i, d := range delays {
		if d > 10*time.Millisecond {
			t.Errorf("delay[%d] = %v, exceeds max 10ms", i, d)
		}
	}
}

func TestRetryTransport_ResetRequestBody(t *testing.T) {
	bodyContent := []byte("test body")
	stub := &stubTransport{
		responses: []*http.Response{
			newErrorResponse(500),
			newOKResponse(),
		},
	}
	var bodies [][]byte
	capture := &bodyCaptureTransport{inner: stub, bodies: &bodies}
	rt := newRetryTransportForTest(capture, 3, nil)

	req, _ := http.NewRequestWithContext(context.Background(), "POST", "http://test", bytes.NewReader(bodyContent))
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if len(bodies) != 2 {
		t.Fatalf("expected 2 body captures, got %d", len(bodies))
	}
	for i, b := range bodies {
		if !bytes.Equal(b, bodyContent) {
			t.Errorf("body[%d] = %q, want %q", i, b, bodyContent)
		}
	}
}

// bodyCaptureTransport captures the request body on each RoundTrip call.
type bodyCaptureTransport struct {
	inner  http.RoundTripper
	bodies *[][]byte
}

func (t *bodyCaptureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		*t.bodies = append(*t.bodies, b)
		req.Body.Close()
		// Restore body for the inner transport to read.
		req.Body = io.NopCloser(bytes.NewReader(b))
	}
	return t.inner.RoundTrip(req)
}

func TestRetryTransport_MaxRetriesZeroReturnsBase(t *testing.T) {
	stub := &stubTransport{
		responses: []*http.Response{newOKResponse()},
	}
	// maxRetries=0 should return the base transport directly (no wrapping).
	rt := newRetryTransport(stub, 0, 1*time.Millisecond, 10*time.Millisecond, loggateway.NewNoop(), nil)

	if rt != http.RoundTripper(stub) {
		t.Errorf("expected base transport to be returned when maxRetries=0")
	}
}

func TestRetryTransport_NilBaseUsesDefault(t *testing.T) {
	// maxRetries=0 with nil base should return http.DefaultTransport.
	rt := newRetryTransport(nil, 0, 1*time.Millisecond, 10*time.Millisecond, loggateway.NewNoop(), nil)
	if rt != http.DefaultTransport {
		t.Errorf("expected http.DefaultTransport, got %T", rt)
	}
}

func TestShouldRetry_Infinite(t *testing.T) {
	rt := &retryTransport{maxRetries: -1}
	for i := 0; i < 100; i++ {
		if !rt.shouldRetry(i, 0) {
			t.Errorf("shouldRetry(%d) = false, want true (infinite)", i)
		}
	}
}

// TestShouldRetry_DecisionCapsInfinite verifies that a per-error-class cap
// (decisionMax > 0) bounds an otherwise infinite transport policy.
func TestShouldRetry_DecisionCapsInfinite(t *testing.T) {
	rt := &retryTransport{maxRetries: -1}
	for i := 0; i < connRefusedMaxAttempts; i++ {
		if !rt.shouldRetry(i, connRefusedMaxAttempts) {
			t.Errorf("shouldRetry(%d, %d) = false, want true", i, connRefusedMaxAttempts)
		}
	}
	if rt.shouldRetry(connRefusedMaxAttempts, connRefusedMaxAttempts) {
		t.Errorf("shouldRetry(%d, %d) = true, want false", connRefusedMaxAttempts, connRefusedMaxAttempts)
	}
}

func TestShouldRetry_Finite(t *testing.T) {
	rt := &retryTransport{maxRetries: 3}
	// attempt 0,1,2 should retry; attempt 3 should not.
	for i := 0; i < 3; i++ {
		if !rt.shouldRetry(i, 0) {
			t.Errorf("shouldRetry(%d) = false, want true", i)
		}
	}
	if rt.shouldRetry(3, 0) {
		t.Errorf("shouldRetry(3) = true, want false")
	}
}

func TestResetRequestBody_NoBody(t *testing.T) {
	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://test", nil)
	if err := resetRequestBody(req); err != nil {
		t.Errorf("resetRequestBody: %v", err)
	}
}

func TestResetRequestBody_NilGetBody(t *testing.T) {
	// Construct a request with a body but no GetBody (streaming body).
	req, _ := http.NewRequestWithContext(context.Background(), "POST", "http://test", &unseekableReader{})
	if err := resetRequestBody(req); err == nil {
		t.Error("expected error for unseekable body, got nil")
	}
}

type unseekableReader struct{}

func (u *unseekableReader) Read(p []byte) (int, error) { return 0, io.EOF }

func TestDefaultRetryMaxAttempts(t *testing.T) {
	tests := []struct {
		name       string
		configured int
		want       int
	}{
		{"default (0) → infinite (-1)", 0, -1},
		{"explicit -1 → infinite", -1, -1},
		{"explicit 1", 1, 1},
		{"explicit 5", 5, 5},
		{"explicit 100", 100, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := defaultRetryMaxAttempts(tt.configured)
			if got != tt.want {
				t.Errorf("defaultRetryMaxAttempts(%d) = %d, want %d", tt.configured, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Retry-After header parsing and usage
// ---------------------------------------------------------------------------

func TestParseRetryAfter_Seconds(t *testing.T) {
	cases := []struct {
		input string
		want  time.Duration
	}{
		{"120", 120 * time.Second},
		{"0", 0},
		{"-1", 0},
		{"", 0},
		{"not-a-number", 0},
	}
	for _, tc := range cases {
		got := parseRetryAfter(tc.input)
		if got != tc.want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestParseRetryAfter_HTTPDate(t *testing.T) {
	// HTTP date format — future time.
	future := time.Now().Add(2 * time.Minute).UTC().Format(http.TimeFormat)
	got := parseRetryAfter(future)
	// Should be roughly 2 minutes (allow 5s slack for test execution).
	if got <= 0 || got > 125*time.Second {
		t.Errorf("parseRetryAfter(%q) = %v, want ~120s", future, got)
	}

	// Past date — should return 0.
	past := time.Now().Add(-1 * time.Minute).UTC().Format(http.TimeFormat)
	if got := parseRetryAfter(past); got != 0 {
		t.Errorf("parseRetryAfter(past date) = %v, want 0", got)
	}
}

func TestRetryTransport_RetryAfterHeaderHonoured(t *testing.T) {
	// First response: 429 with Retry-After: 1 (1 second)
	retryResp := newErrorResponse(429)
	retryResp.Header.Set("Retry-After", "1")
	// Second response: 200 OK
	okResp := newOKResponse()
	stub := &stubTransport{
		responses: []*http.Response{retryResp, okResp},
	}
	var delays []time.Duration
	// 使用 maxDelay=2s（大于 Retry-After=1s），验证 Retry-After 值被原样使用，
	// 而非被封顶。封顶行为由 TestRetryTransport_RetryAfterCappedAtMaxDelay 验证。
	rt := newRetryTransport(stub, 3, 1*time.Millisecond, 2*time.Second, loggateway.NewNoop(), func(req *http.Request, attempt, maxRetries int, err error, delay time.Duration) {
		delays = append(delays, delay)
	})

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	// Retry-After=1s should be used instead of baseDelay (1ms).
	if len(delays) != 1 {
		t.Fatalf("expected 1 retry, got %d", len(delays))
	}
	if delays[0] != 1*time.Second {
		t.Errorf("delay = %v, want 1s (from Retry-After header)", delays[0])
	}
}

func TestRetryTransport_RetryAfterCappedAtMaxDelay(t *testing.T) {
	// Retry-After: 999999 (way above maxDelay of 10ms)
	retryResp := newErrorResponse(429)
	retryResp.Header.Set("Retry-After", "999999")
	okResp := newOKResponse()
	stub := &stubTransport{
		responses: []*http.Response{retryResp, okResp},
	}
	var delays []time.Duration
	// maxDelay = 10ms (from newRetryTransportForTest)
	rt := newRetryTransportForTest(stub, 3, func(req *http.Request, attempt, maxRetries int, err error, delay time.Duration) {
		delays = append(delays, delay)
	})

	req, _ := http.NewRequestWithContext(context.Background(), "GET", "http://test", nil)
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	defer resp.Body.Close()

	if len(delays) != 1 {
		t.Fatalf("expected 1 retry, got %d", len(delays))
	}
	// Retry-After 999999s should be capped at maxDelay (10ms).
	if delays[0] != 10*time.Millisecond {
		t.Errorf("delay = %v, want 10ms (capped at maxDelay)", delays[0])
	}
}
