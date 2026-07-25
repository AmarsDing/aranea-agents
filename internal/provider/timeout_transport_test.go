package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestTimeoutTransport_AllowsStreamingBodyRead verifies that the context is
// not cancelled until the response body is fully read. This is a regression
// test for the bug where defer cancel() after RoundTrip broke streaming LLM
// responses, surfacing as "context canceled" errors in the event stream.
func TestTimeoutTransport_AllowsStreamingBodyRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// Write chunks after RoundTrip has already returned.
		for i := 0; i < 3; i++ {
			time.Sleep(20 * time.Millisecond)
			_, _ = w.Write([]byte("chunk"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
	}))
	defer server.Close()

	client := &http.Client{
		Transport: newTimeoutTransport(http.DefaultTransport, 5*time.Second),
	}

	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("RoundTrip failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading streaming body failed: %v", err)
	}
	if got, want := string(body), "chunkchunkchunk"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

// TestTimeoutTransport_EnforcesTimeout verifies that the timeout is still
// applied when the server is too slow to produce a response.
func TestTimeoutTransport_EnforcesTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Wait longer than the transport timeout before writing any data.
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{
		Transport: newTimeoutTransport(http.DefaultTransport, 50*time.Millisecond),
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	_, err = client.Do(req)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

// TestTimeoutTransport_AttemptTimeoutMarkedRetryable verifies that a hang
// (server never responds within the per-attempt timeout while the caller ctx
// stays alive) is surfaced as attemptTimeoutError so the retry layer treats
// it as transient and reconnects instead of failing the run.
func TestTimeoutTransport_AttemptTimeoutMarkedRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := &http.Client{
		Transport: newTimeoutTransport(http.DefaultTransport, 50*time.Millisecond),
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	_, err = client.Do(req)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	var attemptTimeout *attemptTimeoutError
	if !errors.As(err, &attemptTimeout) {
		t.Fatalf("expected attemptTimeoutError, got %T: %v", err, err)
	}
	if decision := ClassifyRetry(nil, err); decision.Type != RetryWithBackoff {
		t.Fatalf("expected RetryWithBackoff for attempt timeout, got %v", decision.Type)
	}
}

// TestTimeoutTransport_ParentCancelNotMarked verifies that caller-initiated
// cancellation is NOT marked as attemptTimeoutError, so it stays fatal and is
// never retried.
func TestTimeoutTransport_ParentCancelNotMarked(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done() // block until the client goes away
	}))
	defer server.Close()

	client := &http.Client{
		Transport: newTimeoutTransport(http.DefaultTransport, 5*time.Second),
	}

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err = client.Do(req)
	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
	var attemptTimeout *attemptTimeoutError
	if errors.As(err, &attemptTimeout) {
		t.Fatalf("caller cancellation must not be marked as attemptTimeoutError: %v", err)
	}
	if decision := ClassifyRetry(nil, err); decision.Type != RetryFatal {
		t.Fatalf("expected RetryFatal for caller cancellation, got %v", decision.Type)
	}
}
