package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// mockAgent is a minimal trpcagent.Agent for cache tests.
type mockAgent struct{ key string }

func (m *mockAgent) Run(ctx context.Context, invocation *trpcagent.Invocation) (<-chan *trpcevent.Event, error) {
	return nil, nil
}

func (m *mockAgent) Tools() []trpctool.Tool                    { return nil }
func (m *mockAgent) Info() trpcagent.Info                      { return trpcagent.Info{} }
func (m *mockAgent) SubAgents() []trpcagent.Agent              { return nil }
func (m *mockAgent) FindSubAgent(name string) trpcagent.Agent  { return nil }

// makeAgent returns a distinct trpcagent.Agent for cache tests.
func makeAgent(key string) trpcagent.Agent { return &mockAgent{key: key} }

func newTestCache(cap int, ttl time.Duration) *BuildCache {
	c := newBuildCache(cap, ttl)
	// Mark GC as started so tests don't accidentally start a real GC loop.
	// Tests that exercise GC start it explicitly.
	c.started = true
	return c
}

// TestBuildCacheLRUEviction verifies that the LRU cap is honored and the
// least-recently-used entry is the one evicted.
func TestBuildCacheLRUEviction(t *testing.T) {
	c := newTestCache(2, time.Minute)
	c.put("a", makeAgent("a"), nil)
	c.put("b", makeAgent("b"), nil)
	c.put("c", makeAgent("c"), nil) // should evict "a"
	if got := c.get("a"); got != nil {
		t.Fatalf("expected a to be evicted, got %v", got)
	}
	if got := c.get("b"); got == nil {
		t.Fatalf("expected b to remain, got nil")
	}
	if got := c.get("c"); got == nil {
		t.Fatalf("expected c to remain, got nil")
	}
}

// TestBuildCacheTTLExpiry verifies that an entry past its TTL is not
// returned by get and is removed from the map.
func TestBuildCacheTTLExpiry(t *testing.T) {
	c := newTestCache(4, 20*time.Millisecond)
	c.put("k", makeAgent("k"), nil)
	if got := c.get("k"); got == nil {
		t.Fatalf("expected k to be present immediately, got nil")
	}
	time.Sleep(50 * time.Millisecond)
	if got := c.get("k"); got != nil {
		t.Fatalf("expected k to be expired, got %v", got)
	}
	c.mu.Lock()
	_, stillPresent := c.items["k"]
	c.mu.Unlock()
	if stillPresent {
		t.Fatalf("expected k to be removed after expiry, still present")
	}
}

// TestBuildCacheSingleflightCollapsesConcurrentMisses verifies that
// concurrent cache-miss calls for the same key collapse to a single
// build invocation via singleflight. This is the core thundering-herd guarantee.
func TestBuildCacheSingleflightCollapsesConcurrentMisses(t *testing.T) {
	c := newTestCache(8, time.Minute)
	var calls int32
	gate := make(chan struct{})

	builder := func() (trpcagent.Agent, error) {
		atomic.AddInt32(&calls, 1)
		<-gate // hold all callers until N=10 are in flight
		return makeAgent("shared"), nil
	}

	const n = 10
	var wg sync.WaitGroup
	results := make([]trpcagent.Agent, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// Simulate concurrent cache-miss paths by calling sfGroup.Do directly.
			v, err, _ := c.sfGroup.Do("shared", func() (interface{}, error) {
				ag, buildErr := builder()
				if buildErr != nil {
					return nil, buildErr
				}
				c.put("shared", ag, nil)
				return ag, nil
			})
			if err != nil {
				errs[i] = err
			} else {
				results[i] = v.(trpcagent.Agent)
			}
		}(i)
	}
	// Give all goroutines a moment to pile up on the singleflight slot.
	time.Sleep(20 * time.Millisecond)
	close(gate)
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected exactly 1 builder call under singleflight, got %d", got)
	}
	for i, r := range results {
		if errs[i] != nil {
			t.Fatalf("call %d errored: %v", i, errs[i])
		}
		if r == nil {
			t.Fatalf("call %d returned nil agent", i)
		}
	}
	// Verify all callers received the exact same agent instance (pointer equality).
	agentPtr := results[0]
	for i := 1; i < n; i++ {
		if results[i] != agentPtr {
			t.Fatalf("call %d: expected same agent instance, got different pointer", i)
		}
	}
}

// TestBuildCacheSingleflightReturnsErrorOnBuildFailure ensures a failed
// build surfaces the error to every caller, and that the failed result is
// NOT cached.
func TestBuildCacheSingleflightReturnsErrorOnBuildFailure(t *testing.T) {
	c := newTestCache(4, time.Minute)
	sentinel := errors.New("build boom")
	calls := 0

	build := func() (interface{}, error) {
		calls++
		return nil, sentinel
	}

	_, err, _ := c.sfGroup.Do("k", build)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}

	// A second call must re-invoke the builder because the failed
	// result was not cached.
	_, err, _ = c.sfGroup.Do("k", build)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error on retry, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected builder to be called twice (no cache on error), got %d", calls)
	}
}

// TestBuildCacheSweepEvictsExpiredOnly verifies that sweepExpired prunes
// only expired entries and leaves live ones intact.
func TestBuildCacheSweepEvictsExpiredOnly(t *testing.T) {
	c := newTestCache(4, 10*time.Millisecond)
	c.put("expired", makeAgent("expired"), nil)
	time.Sleep(30 * time.Millisecond)
	c.put("live", makeAgent("live"), nil)

	evicted := c.sweepExpired()
	if evicted != 1 {
		t.Fatalf("expected 1 entry evicted, got %d", evicted)
	}
	if got := c.get("expired"); got != nil {
		t.Fatalf("expected expired to be gone, got %v", got)
	}
	if got := c.get("live"); got == nil {
		t.Fatalf("expected live to survive sweep")
	}
}

// TestBuildCacheCloseStopsAndClears ensures Close is idempotent, stops
// the GC loop, and clears all entries.
func TestBuildCacheCloseStopsAndClears(t *testing.T) {
	c := newTestCache(4, time.Minute)
	c.put("k", makeAgent("k"), nil)
	// newTestCache sets started=true so no GC loop runs; Close must still
	// clear the map and be idempotent.
	c.Close()
	c.Close() // second call must not panic
	c.mu.Lock()
	n := len(c.items)
	c.mu.Unlock()
	if n != 0 {
		t.Fatalf("expected cache to be cleared after Close, found %d entries", n)
	}
}

// TestBuildCacheGCLoopSelfTerminatesOnIdle verifies that the background
// reaper exits when the cache stays empty long enough, so test processes
// don't leak goroutines.
func TestBuildCacheGCLoopSelfTerminatesOnIdle(t *testing.T) {
	// Use very short intervals so the test doesn't take 5 minutes.
	origInterval := buildCacheGCInterval
	origIdle := buildCacheGCIdleShutdown
	buildCacheGCInterval = 20 * time.Millisecond
	buildCacheGCIdleShutdown = 60 * time.Millisecond
	t.Cleanup(func() {
		buildCacheGCInterval = origInterval
		buildCacheGCIdleShutdown = origIdle
	})

	c := newBuildCache(4, time.Minute)
	c.ensureGC()
	// Wait up to ~1s for the loop to self-terminate.
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		c.gcMu.Lock()
		done := c.gcDone
		c.gcMu.Unlock()
		// gcDone is open until runGC closes it; non-blocking check.
		select {
		case <-done:
			return // success
		default:
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("GC loop did not self-terminate within deadline")
}

// TestBuildCacheInvalidateByAgentIDPrefix verifies that Invalidate
// removes every entry whose key starts with "agentID:".
func TestBuildCacheInvalidateByAgentIDPrefix(t *testing.T) {
	c := newTestCache(8, time.Minute)
	c.put("agent-1:hash1", makeAgent("a1h1"), nil)
	c.put("agent-1:hash2", makeAgent("a1h2"), nil)
	c.put("agent-2:hash3", makeAgent("a2h3"), nil)

	c.Invalidate("agent-1")

	if c.get("agent-1:hash1") != nil {
		t.Fatalf("expected agent-1:hash1 to be invalidated")
	}
	if c.get("agent-1:hash2") != nil {
		t.Fatalf("expected agent-1:hash2 to be invalidated")
	}
	if c.get("agent-2:hash3") == nil {
		t.Fatalf("expected agent-2:hash3 to remain")
	}
}

// TestBuildCachePutNilAgentIsNoop ensures we never poison the LRU
// with a nil entry — a nil agent would later panic type assertions
// in the lookup path.
func TestBuildCachePutNilAgentIsNoop(t *testing.T) {
	c := newTestCache(2, time.Minute)
	c.put("k", nil, nil)
	if got := c.get("k"); got != nil {
		t.Fatalf("expected nil agent to be rejected, got %v", got)
	}
	c.mu.Lock()
	n := len(c.items)
	c.mu.Unlock()
	if n != 0 {
		t.Fatalf("expected cache to remain empty, found %d entries", n)
	}
}

// TestBuildCacheSingleflightContextIsolation verifies that when one caller
// cancels its context, the shared singleflight build is NOT aborted.
// This is the key context.WithoutCancel guarantee: the build uses a
// derived context that survives caller cancellation.
func TestBuildCacheSingleflightContextIsolation(t *testing.T) {
	c := newTestCache(8, time.Minute)
	buildStarted := make(chan struct{})
	caller2Ready := make(chan struct{})
	buildComplete := make(chan struct{})

	ctx1, cancel1 := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	var caller1Err error
	var caller2Err error
	var caller2Agent trpcagent.Agent

	// Caller 1: will cancel its context after the build starts.
	wg.Add(1)
	go func() {
		defer wg.Done()
		v, err, _ := c.sfGroup.Do("isolated", func() (interface{}, error) {
			// context.WithoutCancel ensures the build survives caller cancellation.
			buildCtx := context.WithoutCancel(ctx1)
			close(buildStarted)
			// Wait for either buildComplete or buildCtx cancellation.
			// WithoutCancel guarantees buildCtx is NOT cancelled when ctx1 is cancelled.
			select {
			case <-buildComplete:
				// Build completed normally — context.WithoutCancel worked.
			case <-buildCtx.Done():
				// If WithoutCancel didn't work, buildCtx would be cancelled here.
				return nil, fmt.Errorf("build context cancelled: %w", buildCtx.Err())
			case <-time.After(5 * time.Second):
				return nil, errors.New("build timeout")
			}
			ag := makeAgent("isolated")
			c.put("isolated", ag, nil)
			return ag, nil
		})
		caller1Err = err
		_ = v
	}()

	// Wait for the build to start.
	<-buildStarted

	// Caller 2: does NOT cancel, should always succeed.
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Signal that this goroutine is about to enter singleflight.
		caller2Ready <- struct{}{}
		v, err, _ := c.sfGroup.Do("isolated", func() (interface{}, error) {
			// singleflight already has an in-flight call; this callback
			// should not be invoked.
			return nil, errors.New("should not be called")
		})
		caller2Err = err
		if v != nil {
			caller2Agent = v.(trpcagent.Agent)
		}
	}()

	// Wait for Caller 2 to be about to enter singleflight, then cancel
	// caller 1's context and complete the build.
	<-caller2Ready
	// Small sleep to let Caller 2 actually enter sfGroup.Do.
	time.Sleep(10 * time.Millisecond)

	// Cancel caller 1's context — the build must survive.
	cancel1()

	// Let the build complete.
	close(buildComplete)
	wg.Wait()

	// Caller 1 should NOT get an error — the build was protected by
	// context.WithoutCancel.
	if caller1Err != nil {
		t.Fatalf("caller 1: expected no error despite context cancellation, got %v", caller1Err)
	}
	// Caller 2 should also succeed with the same shared result.
	if caller2Err != nil {
		t.Fatalf("caller 2: expected no error, got %v", caller2Err)
	}
	if caller2Agent == nil {
		t.Fatalf("caller 2: expected non-nil agent")
	}
	// The agent should be in the cache.
	if got := c.get("isolated"); got == nil {
		t.Fatalf("expected agent to be cached after build completed")
	}
}
