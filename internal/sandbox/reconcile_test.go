package sandbox

import (
	"context"
	"sync"
	"testing"
)

// lightReconcile: label-carrying daemon instances absent from the registry
// are reaped; registry-leased entries visible on the daemon are untouched.
func TestLightReconcileOrphansSkippedForLeased(t *testing.T) {
	eng := newFakeEngine()
	m := newTestManager(t, eng, nil)
	lease, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a1"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	eng.mu.Lock()
	eng.listHandles = []Handle{
		{ID: "sbx-orphan", SandboxID: "sbx-orphan"},
		{ID: lease.SandboxID(), SandboxID: lease.SandboxID()},
	}
	eng.mu.Unlock()

	reaped, drifted := m.lightReconcile(context.Background())
	if reaped != 1 || drifted != 0 {
		t.Fatalf("reaped=%d drifted=%d, want 1/0", reaped, drifted)
	}
	if n := eng.destroyCount("sbx-orphan"); n != 1 {
		t.Fatalf("orphan destroyed %d, want 1", n)
	}
	if n := eng.destroyCount(lease.SandboxID()); n != 0 {
		t.Fatalf("leased sandbox destroyed %d times by light reconcile, want 0", n)
	}
	if !lease.Alive() {
		t.Fatal("lease must survive light reconcile")
	}
}

// Reverse drift: registry-leased entry missing from the daemon scan is NOT
// destroyed here (lazy re-lease via Lease.Alive) — drift is metric-warned.
func TestLightReconcileReverseDriftWarnedOnly(t *testing.T) {
	eng := newFakeEngine()
	m := newTestManager(t, eng, nil)
	lease, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a1"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	// Daemon reports nothing — the leased instance has vanished engine-side.
	before := countMetric(t, reconcileDriftTotal, "daemon_missing")
	reaped, drifted := m.lightReconcile(context.Background())
	if reaped != 0 || drifted != 1 {
		t.Fatalf("reaped=%d drifted=%d, want 0/1", reaped, drifted)
	}
	if n := eng.destroyCount(lease.SandboxID()); n != 0 {
		t.Fatalf("drifted lease destroyed %d times, want 0 (lazy re-lease)", n)
	}
	if got := countMetric(t, reconcileDriftTotal, "daemon_missing") - before; got != 1 {
		t.Fatalf("drift metric delta=%v, want 1", got)
	}
	// Registry entry intact; GC/destroy paths still govern teardown.
	e, ok := m.registry.get(lease.SandboxID())
	if !ok || e.entryState != StateLeased {
		t.Fatalf("registry entry lost: ok=%v", ok)
	}
}

// listSyncEngine keeps listHandles consistent with Destroy, like a real
// daemon: once the instance is rm'd it no longer shows up in the label scan.
// (The plain fakeEngine leaves listHandles static, which would make a
// post-destroy lightReconcile see a phantom orphan — unreachable in reality.)
type listSyncEngine struct {
	*fakeEngine
}

func (s *listSyncEngine) Destroy(ctx context.Context, h Handle) error {
	err := s.fakeEngine.Destroy(ctx, h)
	s.mu.Lock()
	defer s.mu.Unlock()
	out := s.listHandles[:0]
	for _, lh := range s.listHandles {
		if lh.SandboxID != h.SandboxID {
			out = append(out, lh)
		}
	}
	s.listHandles = out
	return err
}

// Concurrent light reconcile + destroy must stay single-fire (A2 acceptance):
// the reconcile never double-destroys a registry-converged entry.
func TestLightReconcileConcurrentDestroySingleFire(t *testing.T) {
	eng := &listSyncEngine{fakeEngine: newFakeEngine()}
	m := newTestManager(t, eng, nil)
	lease, err := m.Acquire(context.Background(), AcquireReq{AgentKey: "a1"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	eng.mu.Lock()
	eng.listHandles = []Handle{{ID: lease.SandboxID(), SandboxID: lease.SandboxID()}}
	eng.mu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = lease.Release(context.Background())
		}()
	}
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = m.lightReconcile(context.Background())
		}()
	}
	wg.Wait()
	if n := eng.destroyCount(lease.SandboxID()); n != 1 {
		t.Fatalf("destroy fired %d times under reconcile×destroy race, want exactly 1", n)
	}
}
