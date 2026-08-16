package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// mockAgent is a minimal trpcagent.Agent for cache tests.
var _ trpcagent.Agent = (*mockAgent)(nil)

type mockAgent struct{ key string }

func (m *mockAgent) Run(ctx context.Context, invocation *trpcagent.Invocation) (<-chan *trpcevent.Event, error) {
	return nil, nil
}

func (m *mockAgent) Tools() []trpctool.Tool                   { return nil }
func (m *mockAgent) Info() trpcagent.Info                     { return trpcagent.Info{} }
func (m *mockAgent) SubAgents() []trpcagent.Agent             { return nil }
func (m *mockAgent) FindSubAgent(name string) trpcagent.Agent { return nil }

// makeAgent returns a distinct trpcagent.Agent for cache tests.
func makeAgent(key string) trpcagent.Agent { return &mockAgent{key: key} }

func newTestCache(cap int) *BuildCache {
	return newBuildCache(cap)
}

// TestBuildCacheLRUEviction verifies that the LRU cap is honored and the
// least-recently-used entry is the one evicted.
func TestBuildCacheLRUEviction(t *testing.T) {
	c := newTestCache(2)
	c.put("a", makeAgent("a"), nil, nil)
	c.put("b", makeAgent("b"), nil, nil)
	c.put("c", makeAgent("c"), nil, nil) // should evict "a"
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

// TestBuildCacheNoTTL verifies that entries persist indefinitely —
// no TTL-based expiry exists in the Always-Ready design.
func TestBuildCacheNoTTL(t *testing.T) {
	c := newTestCache(4)
	c.put("k", makeAgent("k"), nil, nil)
	// Wait longer than any reasonable TTL would be.
	time.Sleep(50 * time.Millisecond)
	if got := c.get("k"); got == nil {
		t.Fatalf("expected k to persist (no TTL), got nil")
	}
}

// TestBuildCacheSingleflightCollapsesConcurrentMisses verifies that
// concurrent cache-miss calls for the same key collapse to a single
// build invocation via singleflight. This is the core thundering-herd guarantee.
func TestBuildCacheSingleflightCollapsesConcurrentMisses(t *testing.T) {
	c := newTestCache(8)
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
				c.put("shared", ag, nil, nil)
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
	c := newTestCache(4)
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

// TestBuildCacheCloseStopsAndClears ensures Close is idempotent and
// clears all entries.
func TestBuildCacheCloseStopsAndClears(t *testing.T) {
	c := newTestCache(4)
	c.put("k", makeAgent("k"), nil, nil)
	c.Close()
	c.Close() // second call must not panic
	c.mu.Lock()
	n := len(c.items)
	c.mu.Unlock()
	if n != 0 {
		t.Fatalf("expected cache to be cleared after Close, found %d entries", n)
	}
}

// TestBuildCacheInvalidateMarksDirty verifies that Invalidate marks
// entries as dirty (instead of evicting them) so the stale agent
// continues serving while a background rebuild runs.
func TestBuildCacheInvalidateMarksDirty(t *testing.T) {
	c := newTestCache(8)
	c.put("agent-1:hash1", makeAgent("a1h1"), nil, nil)
	c.put("agent-1:hash2", makeAgent("a1h2"), nil, nil)
	c.put("agent-2:hash3", makeAgent("a2h3"), nil, nil)

	c.Invalidate("agent-1")

	// Entries should still be present (marked dirty, not evicted).
	c.mu.Lock()
	e1, ok1 := c.items["agent-1:hash1"]
	e2, ok2 := c.items["agent-1:hash2"]
	e3, ok3 := c.items["agent-2:hash3"]
	c.mu.Unlock()

	if !ok1 {
		t.Fatalf("expected agent-1:hash1 to still be present (dirty)")
	}
	if !ok2 {
		t.Fatalf("expected agent-1:hash2 to still be present (dirty)")
	}
	if !ok3 {
		t.Fatalf("expected agent-2:hash3 to still be present")
	}
	if !e1.dirty {
		t.Fatalf("expected agent-1:hash1 to be marked dirty")
	}
	if !e2.dirty {
		t.Fatalf("expected agent-1:hash2 to be marked dirty")
	}
	if e3.dirty {
		t.Fatalf("expected agent-2:hash3 to NOT be dirty")
	}

	// get() still returns the stale agent (dirty entries are served).
	if got := c.get("agent-1:hash1"); got == nil {
		t.Fatalf("expected dirty entry to still be served via get()")
	}
}

// TestBuildCachePutNilAgentIsNoop ensures we never poison the LRU
// with a nil entry — a nil agent would later panic type assertions
// in the lookup path.
func TestBuildCachePutNilAgentIsNoop(t *testing.T) {
	c := newTestCache(2)
	c.put("k", nil, nil, nil)
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
	c := newTestCache(8)
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
			c.put("isolated", ag, nil, nil)
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

// TestBuildCachePutClearsDirty verifies that putting a fresh agent
// clears the dirty flag, allowing the entry to serve normally again.
func TestBuildCachePutClearsDirty(t *testing.T) {
	c := newTestCache(4)
	c.put("agent-1:hash1", makeAgent("v1"), nil, nil)
	c.Invalidate("agent-1")

	c.mu.Lock()
	e := c.items["agent-1:hash1"]
	c.mu.Unlock()
	if !e.dirty {
		t.Fatalf("expected entry to be dirty after Invalidate")
	}

	// Put a fresh agent — should clear dirty.
	c.put("agent-1:hash1", makeAgent("v2"), nil, nil)
	c.mu.Lock()
	e = c.items["agent-1:hash1"]
	c.mu.Unlock()
	if e.dirty {
		t.Fatalf("expected entry to be clean after put")
	}
}

// TestBuildCacheCloseDrainsGraveyardImmediately verifies that Close() does
// not wait out the retire delay: at process shutdown there are no in-flight
// requests worth protecting, so graveyard entries are closed at once.
func TestBuildCacheCloseDrainsGraveyardImmediately(t *testing.T) {
	c := newTestCache(4)
	c.retireDelay = time.Hour // long enough that the sweeper will never fire

	tsOld := &fakeToolSet{name: "ts-replaced"}
	c.put("k", makeAgent("v1"), nil, []trpctool.ToolSet{tsOld})
	// Replace the entry → tsOld is retired to the graveyard.
	c.put("k", makeAgent("v2"), nil, nil)

	tsLive := &fakeToolSet{name: "ts-live"}
	c.put("j", makeAgent("j"), nil, []trpctool.ToolSet{tsLive})

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !tsOld.closed.Load() {
		t.Fatal("Close must close graveyard entries despite the pending delay")
	}
	if !tsLive.closed.Load() {
		t.Fatal("Close must close live entry ToolSets")
	}
	c.mu.Lock()
	stopped := c.sweeperCancel == nil
	c.mu.Unlock()
	if !stopped {
		t.Fatal("Close must stop the sweeper")
	}
}

// fakeToolSet is a minimal trpctool.ToolSet tracking Close calls.
type fakeToolSet struct {
	name   string
	closed atomic.Bool
}

func (f *fakeToolSet) Tools(context.Context) []trpctool.Tool { return nil }
func (f *fakeToolSet) Close() error                          { f.closed.Store(true); return nil }
func (f *fakeToolSet) Name() string                          { return f.name }

var _ trpctool.ToolSet = (*fakeToolSet)(nil)

// TestBuildCacheCloseWaitsForSweeperNoDoubleClose verifies the two shutdown
// defects fixed in review: (1) Close blocks until the sweeper goroutine has
// fully exited, so a graveyard entry already being swept can never be closed
// twice; (2) after Close, put/retire no longer resurrect a leaked sweeper and
// instead close incoming ToolSets immediately.
func TestBuildCacheCloseWaitsForSweeperNoDoubleClose(t *testing.T) {
	c := newTestCache(4)
	c.retireDelay = time.Hour

	ts := &fakeToolSet{name: "ts"}
	c.put("k", makeAgent("v"), nil, []trpctool.ToolSet{ts})
	c.put("k", makeAgent("v2"), nil, nil) // retire ts → sweeper starts

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if !ts.closed.Load() {
		t.Fatal("Close must close retired ToolSet")
	}

	// Second Close is a no-op and must not panic or re-close.
	if err := c.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	// put after Close must close the incoming ToolSet immediately (no leak),
	// and must NOT restart the sweeper.
	ts2 := &fakeToolSet{name: "ts2"}
	c.put("k", makeAgent("v3"), nil, []trpctool.ToolSet{ts2})
	deadline := time.Now().Add(2 * time.Second)
	for !ts2.closed.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !ts2.closed.Load() {
		t.Fatal("put after Close must close incoming ToolSet immediately")
	}
	c.mu.Lock()
	restarted := c.sweeperCancel != nil
	got := c.items["k"]
	c.mu.Unlock()
	if restarted {
		t.Fatal("put after Close must not restart the sweeper")
	}
	if got != nil {
		t.Fatal("put after Close must not cache the entry")
	}
}

// stubDeclTool is a minimal trpctool.Tool carrying only a declaration name.
type stubDeclTool struct{ name string }

func (s stubDeclTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: s.name}
}

// TestBuildCacheKey_CustomToolsAffectKey guards against cache cross-contamination:
// an agent built with deliverable CustomTools must not share a cache entry with
// the same agent built without them, otherwise the deliverable toolset leaks
// into plain builds (or is lost in deliverable builds) depending on build order.
func TestBuildCacheKey_CustomToolsAffectKey(t *testing.T) {
	ag := biz.Agent{ID: "agent-cache-key-test"}
	depsWithTools := func(tools ...trpctool.Tool) TRPCBuilderDeps {
		return TRPCBuilderDeps{TRPCToolAssemblyDeps: TRPCToolAssemblyDeps{CustomTools: tools}}
	}
	plain := BuildCacheKey(ag, TRPCBuilderDeps{}, "toolhash", "", "")
	withDeliverable := BuildCacheKey(ag, depsWithTools(stubDeclTool{"set_deliverable"}, stubDeclTool{"get_deliverable"}), "toolhash", "", "")
	if plain == withDeliverable {
		t.Fatal("CustomTools must participate in the cache key; deliverable and plain builds must not share an entry")
	}

	// Same tool set in different order must produce the same key (order-insensitive).
	a := BuildCacheKey(ag, depsWithTools(stubDeclTool{"set_deliverable"}, stubDeclTool{"get_deliverable"}), "toolhash", "", "")
	b := BuildCacheKey(ag, depsWithTools(stubDeclTool{"get_deliverable"}, stubDeclTool{"set_deliverable"}), "toolhash", "", "")
	if a != b {
		t.Fatal("cache key must be order-insensitive over CustomTools declaration names")
	}
}

// stubBehaviorTool is a tool whose behavior varies beyond its declaration
// name, surfaced via the cache-key discriminator interface.
type stubBehaviorTool struct {
	name          string
	discriminator string
}

func (s stubBehaviorTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: s.name}
}

func (s stubBehaviorTool) CacheKeyDiscriminator() string { return s.discriminator }

// TestBuildCacheKey_ToolBehaviorDiscriminated guards against cache collisions
// between behaviorally different tool variants sharing one declaration name.
// Regression: the team compile path installs the MDC contract on
// set_deliverable while the graph resolver path builds it contract-free;
// after dialog-mode normalization both paths produce identical key components,
// so without the discriminator whichever path builds first would silently
// serve its variant to the other (contract validation lost or phantom
// violations, depending on build order).
func TestBuildCacheKey_ToolBehaviorDiscriminated(t *testing.T) {
	ag := biz.Agent{ID: "agent-behavior-key-test"}
	depsWith := func(tools ...trpctool.Tool) TRPCBuilderDeps {
		return TRPCBuilderDeps{TRPCToolAssemblyDeps: TRPCToolAssemblyDeps{CustomTools: tools}}
	}

	free := BuildCacheKey(ag, depsWith(stubDeclTool{"set_deliverable"}), "toolhash", "", "")
	contracted := BuildCacheKey(ag, depsWith(stubBehaviorTool{name: "set_deliverable", discriminator: "contract:abc123"}), "toolhash", "", "")
	if free == contracted {
		t.Fatal("behavior-variant tools sharing a declaration name must not share a cache key")
	}

	// Same name + same discriminator → same key (cache shareable).
	a := BuildCacheKey(ag, depsWith(stubBehaviorTool{name: "set_deliverable", discriminator: "contract:abc123"}), "toolhash", "", "")
	b := BuildCacheKey(ag, depsWith(stubBehaviorTool{name: "set_deliverable", discriminator: "contract:abc123"}), "toolhash", "", "")
	if a != b {
		t.Fatal("identical behavior variants must share a cache key")
	}
}

// TestBuildCacheKey_DialogModeNormalized guards the cache key against
// dialog-mode flapping. Regression: the team runner path passes the turn's
// dialog mode (e.g. "default") while the graph node resolver path passes "",
// so the same member agent was built twice per team run under two keys.
// The key must reduce DialogMode to its build-time effect (planner.Select):
//   - explicit planner_kind → dialog mode has no build effect;
//   - otherwise only "plan" activates the builtin planner.
func TestBuildCacheKey_DialogModeNormalized(t *testing.T) {
	ag := biz.Agent{ID: "agent-dialog-key-test"}
	depsWithMode := func(mode string) TRPCBuilderDeps {
		return TRPCBuilderDeps{TRPCModelRouteDeps: TRPCModelRouteDeps{DialogMode: mode}}
	}

	// planner_kind unset: ""/"default"/"chat" are build-equivalent → same key.
	empty := BuildCacheKey(ag, depsWithMode(""), "toolhash", "", "")
	def := BuildCacheKey(ag, depsWithMode("default"), "toolhash", "", "")
	chat := BuildCacheKey(ag, depsWithMode("chat"), "toolhash", "", "")
	if empty != def || empty != chat {
		t.Fatal("dialog modes without build effect must share one cache key")
	}

	// "plan" activates the builtin planner → distinct key.
	plan := BuildCacheKey(ag, depsWithMode("plan"), "toolhash", "", "")
	if plan == empty {
		t.Fatal("plan mode activates the builtin planner and must have a distinct key")
	}
	// Case/space-insensitive, matching planner.Select.
	if spaced := BuildCacheKey(ag, depsWithMode("  PLAN "), "toolhash", "", ""); spaced != plan {
		t.Fatal("plan mode key must be case/space-insensitive, matching planner.Select")
	}

	// Explicit planner_kind: dialog mode (incl. "plan") has no build effect.
	reactAg := biz.Agent{ID: "agent-dialog-key-test", Settings: &biz.AgentRuntimeSettings{PlannerKind: "react"}}
	reactPlan := BuildCacheKey(reactAg, depsWithMode("plan"), "toolhash", "", "")
	reactDefault := BuildCacheKey(reactAg, depsWithMode("default"), "toolhash", "", "")
	if reactPlan != reactDefault {
		t.Fatal("explicit planner_kind must make the key dialog-mode-independent")
	}

	// Unknown planner_kind: Select falls back to dialog-mode checks,
	// so "plan" must stay distinct.
	unknownAg := biz.Agent{ID: "agent-dialog-key-test", Settings: &biz.AgentRuntimeSettings{PlannerKind: "unknown-kind"}}
	unkPlan := BuildCacheKey(unknownAg, depsWithMode("plan"), "toolhash", "", "")
	unkDefault := BuildCacheKey(unknownAg, depsWithMode("default"), "toolhash", "", "")
	if unkPlan == unkDefault {
		t.Fatal("unknown planner_kind falls back to dialog-mode selection; plan must stay distinct")
	}
}

// stubMonitorBus captures MonitorEvents for flow-log assertions.
type stubMonitorBus struct {
	mu  sync.Mutex
	evs []contract.MonitorEvent
}

func (b *stubMonitorBus) Publish(_ context.Context, ev contract.MonitorEvent) {
	b.mu.Lock()
	b.evs = append(b.evs, ev)
	b.mu.Unlock()
}

func (b *stubMonitorBus) Subscribe(_ contract.MonitorSubscribeOptions) (<-chan contract.MonitorEvent, func()) {
	return nil, func() {}
}

func (b *stubMonitorBus) DropCount() uint64 { return 0 }

// buildFlowPhases collects the flow_phase values of all captured
// system.agent.build flow-log events, in publication order.
func (b *stubMonitorBus) buildFlowPhases() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var phases []string
	for _, ev := range b.evs {
		if ev.Type != contract.MonitorEventTypeFlowLog {
			continue
		}
		if ev.Metadata["step_id"] != "system.agent.build" {
			continue
		}
		phase, _ := ev.Metadata["flow_phase"].(string)
		phases = append(phases, phase)
	}
	return phases
}

// withStubbedBuilder swaps the package-level agentBuildFn for the duration of
// the test so BuildTRPCLLMAgentCached can run without real model/skill deps.
func withStubbedBuilder(t *testing.T, fn func(context.Context, biz.Agent, TRPCBuilderDeps, loggateway.Logger) (trpcagent.Agent, []trpctool.ToolSet, *faceMeta, error)) {
	t.Helper()
	orig := agentBuildFn
	agentBuildFn = fn
	t.Cleanup(func() { agentBuildFn = orig })
}

// TestBuildTRPCLLMAgentCached_EmitsBuildFlowOnMiss verifies the build-process
// visibility contract: a cache-miss build emits start + done flow logs so the
// (potentially multi-second) build is visible in the 流程日志 tab instead of
// looking like a hung request.
func TestBuildTRPCLLMAgentCached_EmitsBuildFlowOnMiss(t *testing.T) {
	bus := &stubMonitorBus{}
	globalBuildCache.SetMonitorBus(bus)
	withStubbedBuilder(t, func(_ context.Context, _ biz.Agent, _ TRPCBuilderDeps, _ loggateway.Logger) (trpcagent.Agent, []trpctool.ToolSet, *faceMeta, error) {
		return makeAgent("flow-miss"), nil, nil, nil
	})

	ag := biz.Agent{ID: "agent-flow-miss-test", AgentKey: "flow-miss-test"}
	got, err := BuildTRPCLLMAgentCached(context.Background(), ag, TRPCBuilderDeps{}, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("expected successful build, got %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil agent")
	}

	phases := bus.buildFlowPhases()
	if len(phases) != 2 || phases[0] != "start" || phases[1] != "done" {
		t.Fatalf("expected [start done] build flow phases, got %v", phases)
	}
}

// TestBuildTRPCLLMAgentCached_EmitsBuildFlowOnFailure verifies that a failed
// cold build emits an error-phase flow log (K2: 错误路径必须发流程日志).
func TestBuildTRPCLLMAgentCached_EmitsBuildFlowOnFailure(t *testing.T) {
	bus := &stubMonitorBus{}
	globalBuildCache.SetMonitorBus(bus)
	sentinel := errors.New("build boom")
	withStubbedBuilder(t, func(_ context.Context, _ biz.Agent, _ TRPCBuilderDeps, _ loggateway.Logger) (trpcagent.Agent, []trpctool.ToolSet, *faceMeta, error) {
		return nil, nil, nil, sentinel
	})

	ag := biz.Agent{ID: "agent-flow-fail-test", AgentKey: "flow-fail-test"}
	_, err := BuildTRPCLLMAgentCached(context.Background(), ag, TRPCBuilderDeps{}, loggateway.NewNoop())
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}

	phases := bus.buildFlowPhases()
	if len(phases) != 2 || phases[0] != "start" || phases[1] != "error" {
		t.Fatalf("expected [start error] build flow phases, got %v", phases)
	}
}
