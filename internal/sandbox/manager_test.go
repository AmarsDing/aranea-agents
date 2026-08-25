package sandbox

import (
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeEngine is an in-memory Engine for unit tests.
type fakeEngine struct {
	mu          sync.Mutex
	created     map[string]Profile
	destroyed   []string
	execFunc    func(h Handle, spec ExecSpec) (ExecResult, error)
	createErr   error
	listHandles []Handle
}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{created: map[string]Profile{}}
}

func (f *fakeEngine) Create(ctx context.Context, sandboxID string, p Profile, labels map[string]string) (Handle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return Handle{}, f.createErr
	}
	f.created[sandboxID] = p
	return Handle{ID: sandboxID, SandboxID: sandboxID}, nil
}

func (f *fakeEngine) Exec(ctx context.Context, h Handle, spec ExecSpec) (ExecResult, error) {
	if f.execFunc != nil {
		return f.execFunc(h, spec)
	}
	return ExecResult{Stdout: "ok"}, nil
}

func (f *fakeEngine) CopyTo(ctx context.Context, h Handle, path string, r io.Reader) error {
	return nil
}

func (f *fakeEngine) CopyFrom(ctx context.Context, h Handle, path string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}

func (f *fakeEngine) Destroy(ctx context.Context, h Handle) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.destroyed = append(f.destroyed, h.SandboxID)
	delete(f.created, h.SandboxID)
	return nil
}

func (f *fakeEngine) ListByLabels(ctx context.Context, labels map[string]string) ([]Handle, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Handle(nil), f.listHandles...), nil
}

func (f *fakeEngine) destroyCount(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, d := range f.destroyed {
		if d == id {
			n++
		}
	}
	return n
}

func newTestManager(t *testing.T, eng Engine, mutate func(*Config)) *Manager {
	t.Helper()
	cfg := DefaultConfig()
	cfg.Pool.MinReady = 0 // tests drive the pool explicitly
	if mutate != nil {
		mutate(&cfg)
	}
	m := NewManager(cfg, eng, nil)
	now := time.Now()
	m.now = func() time.Time { return now }
	t.Cleanup(m.Close)
	return m
}

func TestAcquireColdAndRelease(t *testing.T) {
	eng := newFakeEngine()
	m := newTestManager(t, eng, nil)

	lease, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a1"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if len(eng.created) != 1 {
		t.Fatalf("expected 1 cold create, got %d", len(eng.created))
	}
	if got := m.registry.countReady(DefaultProfileName); got != 0 {
		t.Fatalf("pool should be empty, got %d", got)
	}
	if g, _ := m.quota.Snapshot(); g != 1 {
		t.Fatalf("quota global=%d, want 1", g)
	}

	if err := lease.Release(context.Background()); err != nil {
		t.Fatalf("Release: %v", err)
	}
	if n := eng.destroyCount(lease.SandboxID()); n != 1 {
		t.Fatalf("destroy calls=%d, want 1", n)
	}
	if g, _ := m.quota.Snapshot(); g != 0 {
		t.Fatalf("quota global=%d after release, want 0", g)
	}
	// Release is idempotent.
	_ = lease.Release(context.Background())
	if n := eng.destroyCount(lease.SandboxID()); n != 1 {
		t.Fatalf("destroy after 2nd release=%d, want 1", n)
	}
}

func TestAcquireWarmHit(t *testing.T) {
	eng := newFakeEngine()
	m := newTestManager(t, eng, func(c *Config) { c.Pool.MinReady = 2 })

	m.replenish(context.Background())
	if got := m.registry.countReady(DefaultProfileName); got != 2 {
		t.Fatalf("ready=%d, want 2", got)
	}

	lease, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a1"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if got := m.registry.countReady(DefaultProfileName); got != 1 {
		t.Fatalf("ready after warm acquire=%d, want 1", got)
	}
	if m.st.acquireWarm.Load() != 1 {
		t.Fatalf("warm acquires=%d, want 1", m.st.acquireWarm.Load())
	}

	// Replenish refills the deficit with a BRAND-NEW instance (never the released one).
	m.replenish(context.Background())
	if got := m.registry.countReady(DefaultProfileName); got != 2 {
		t.Fatalf("ready after replenish=%d, want 2", got)
	}
	_ = lease.Release(context.Background())
	m.replenish(context.Background())
	if got := m.registry.countReady(DefaultProfileName); got != 2 {
		t.Fatalf("ready stays capped at min_ready, got %d", got)
	}
}

func TestQuotaGlobalAndPerAgent(t *testing.T) {
	eng := newFakeEngine()
	m := newTestManager(t, eng, func(c *Config) {
		c.Limits.GlobalMaxActive = 2
		c.Limits.PerAgentMaxActive = 1
	})

	if _, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a1"}); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	// Per-agent fires first for the same agent.
	_, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a1"})
	var qe *QuotaError
	if !errors.As(err, &qe) || qe.Scope != QuotaScopeAgent {
		t.Fatalf("want per-agent quota error, got %v", err)
	}
	// A second agent fills the global slot; the third hits the global gate.
	if _, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a2"}); err != nil {
		t.Fatalf("second agent acquire: %v", err)
	}
	_, err = m.Acquire(context.Background(), AcquireReq{AgentKey: "a3"})
	if !errors.As(err, &qe) || qe.Scope != QuotaScopeGlobal {
		t.Fatalf("want global quota error, got %v", err)
	}
	if got := m.st.quotaReject.snapshot()[QuotaScopeGlobal]; got != 1 {
		t.Fatalf("global rejects=%d, want 1", got)
	}
}

func TestConcurrentDoubleDestroySingleFire(t *testing.T) {
	eng := newFakeEngine()
	m := newTestManager(t, eng, nil)
	lease, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a1"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _ = lease.Release(context.Background()) }()
		go func() { defer wg.Done(); _ = m.ForceKill(lease.SandboxID(), "test", "op") }()
	}
	wg.Wait()
	if n := eng.destroyCount(lease.SandboxID()); n != 1 {
		t.Fatalf("destroy fired %d times under concurrency, want exactly 1", n)
	}
	if g, _ := m.quota.Snapshot(); g != 0 {
		t.Fatalf("quota leaked: global=%d", g)
	}
}

func TestGCTTLAndIdle(t *testing.T) {
	eng := newFakeEngine()
	base := time.Now()
	var now atomic.Int64
	now.Store(base.UnixNano())
	m := newTestManager(t, eng, func(c *Config) {
		c.TTL.Default = 10 * time.Minute
		c.TTL.IdleTimeout = 5 * time.Minute
	})
	m.now = func() time.Time { return time.Unix(0, now.Load()) }

	ttlLease, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "ttl"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	idleLease, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "idle"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	// idle lease: exec once now, then let it go stale (but within TTL).
	if _, err := idleLease.Exec(context.Background(), ExecSpec{Argv: []string{"true"}}); err != nil {
		t.Fatalf("Exec: %v", err)
	}

	// +6m: idle lease exceeds idle timeout; ttl lease still alive (exec keeps fresh? no — ttl lease never exec'd, so it is ALSO idle-stale).
	now.Add(int64(6 * time.Minute))
	m.gcOnce()
	if n := eng.destroyCount(idleLease.SandboxID()); n != 1 {
		t.Fatalf("idle destroy=%d, want 1", n)
	}

	// +11m: ttl lease past its 10m deadline.
	now.Add(int64(5 * time.Minute))
	m.gcOnce()
	if n := eng.destroyCount(ttlLease.SandboxID()); n != 1 {
		t.Fatalf("ttl destroy=%d, want 1", n)
	}
}

func TestGCPoolEvict(t *testing.T) {
	eng := newFakeEngine()
	base := time.Now()
	var now atomic.Int64
	now.Store(base.UnixNano())
	m := newTestManager(t, eng, func(c *Config) {
		c.Pool.MinReady = 1
		c.Pool.MaxPoolAge = time.Minute
	})
	m.now = func() time.Time { return time.Unix(0, now.Load()) }

	m.replenish(context.Background())
	if got := m.registry.countReady(DefaultProfileName); got != 1 {
		t.Fatalf("ready=%d, want 1", got)
	}
	now.Add(int64(2 * time.Minute))
	m.gcOnce()
	if got := m.registry.countReady(DefaultProfileName); got != 0 {
		t.Fatalf("aged pool instance not evicted, ready=%d", got)
	}
	if got := m.st.destroy.snapshot()[ReasonPoolEvict]; got != 1 {
		t.Fatalf("pool_evict destroys=%d, want 1", got)
	}
}

func TestReconcileReapsOrphans(t *testing.T) {
	eng := newFakeEngine()
	eng.listHandles = []Handle{
		{ID: "sbx-orphan-1", SandboxID: "sbx-orphan-1"},
		{ID: "sbx-orphan-2", SandboxID: "sbx-orphan-2"},
	}
	m := newTestManager(t, eng, nil)
	m.reconcileOnce(context.Background())
	if n := eng.destroyCount("sbx-orphan-1"); n != 1 {
		t.Fatalf("orphan-1 destroyed %d, want 1", n)
	}
	if n := eng.destroyCount("sbx-orphan-2"); n != 1 {
		t.Fatalf("orphan-2 destroyed %d, want 1", n)
	}
	if got := m.st.destroy.snapshot()[ReasonReconcile]; got != 2 {
		t.Fatalf("reconcile destroys=%d, want 2", got)
	}
}

func TestLeaseExecTouchAndNotFoundAfterDestroy(t *testing.T) {
	eng := newFakeEngine()
	m := newTestManager(t, eng, nil)
	lease, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a1"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if _, err := lease.Exec(context.Background(), ExecSpec{Argv: []string{"true"}}); err != nil {
		t.Fatalf("Exec: %v", err)
	}
	e, ok := m.registry.get(lease.SandboxID())
	if !ok || e.view.ExecCount != 1 || e.view.LastExecAt.IsZero() {
		t.Fatalf("touch not recorded: %+v", e.view)
	}
	_ = lease.Release(context.Background())
	if _, err := lease.Exec(context.Background(), ExecSpec{Argv: []string{"true"}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("exec after release: want ErrNotFound, got %v", err)
	}
}

func TestRenewCappedAtMax(t *testing.T) {
	eng := newFakeEngine()
	m := newTestManager(t, eng, func(c *Config) {
		c.TTL.Default = time.Minute
		c.TTL.Max = 2 * time.Minute
	})
	lease, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a1"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if err := lease.Renew(context.Background(), 10*time.Minute); err != nil {
		t.Fatalf("Renew: %v", err)
	}
	e, _ := m.registry.get(lease.SandboxID())
	want := m.now().Add(2 * time.Minute)
	if !e.view.Deadline.Equal(want) {
		t.Fatalf("deadline=%v, want capped %v", e.view.Deadline, want)
	}
}

func TestDisabledAndEngineDown(t *testing.T) {
	eng := newFakeEngine()
	m := newTestManager(t, eng, func(c *Config) { c.Enabled = false })
	if _, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a1"}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("want ErrDisabled, got %v", err)
	}

	m2 := NewManager(DefaultConfig(), nil, nil)
	defer m2.Close()
	if _, err := m2.Acquire(context.Background(), AcquireReq{AgentKey: "a1"}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("nil engine: want ErrDisabled, got %v", err)
	}
}

func TestColdCreateFailureReleasesQuota(t *testing.T) {
	eng := newFakeEngine()
	eng.createErr = errors.New("daemon exploded")
	m := newTestManager(t, eng, nil)
	if _, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a1"}); err == nil {
		t.Fatal("want create error")
	}
	if g, _ := m.quota.Snapshot(); g != 0 {
		t.Fatalf("quota leaked after failed create: global=%d", g)
	}
	if m.st.acquireFail.Load() != 1 {
		t.Fatalf("fail acquires=%d, want 1", m.st.acquireFail.Load())
	}
}

func TestUnknownProfile(t *testing.T) {
	eng := newFakeEngine()
	m := newTestManager(t, eng, nil)
	if _, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a1", Profile: "nope"}); !errors.Is(err, ErrProfileUnknown) {
		t.Fatalf("want ErrProfileUnknown, got %v", err)
	}
}
