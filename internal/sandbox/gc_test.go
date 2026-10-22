package sandbox

import (
	"context"
	"testing"
	"time"
)

// maybeLightReconcile gating: interval-based cadence, 0/negative disables.
func TestMaybeLightReconcileGating(t *testing.T) {
	newManagerWithOrphan := func(t *testing.T) (*Manager, *fakeEngine) {
		eng := newFakeEngine()
		eng.listHandles = []Handle{{ID: "sbx-orphan", SandboxID: "sbx-orphan"}}
		m := newTestManager(t, eng, nil)
		return m, eng
	}

	t.Run("first call runs (lastReconcile zero)", func(t *testing.T) {
		m, eng := newManagerWithOrphan(t)
		m.reconcileInterval = time.Minute
		m.maybeLightReconcile(context.Background())
		if n := eng.destroyCount("sbx-orphan"); n != 1 {
			t.Fatalf("orphan reaped %d, want 1", n)
		}
	})

	t.Run("within interval skipped", func(t *testing.T) {
		m, eng := newManagerWithOrphan(t)
		m.reconcileInterval = time.Minute
		m.maybeLightReconcile(context.Background())
		// same m.now() (fixed test clock) → second call must skip
		m.maybeLightReconcile(context.Background())
		if n := eng.destroyCount("sbx-orphan"); n != 1 {
			t.Fatalf("reconcile re-ran inside interval: destroys=%d, want 1", n)
		}
	})

	t.Run("after interval reruns", func(t *testing.T) {
		m, eng := newManagerWithOrphan(t)
		m.reconcileInterval = time.Minute
		m.maybeLightReconcile(context.Background())
		// Advance the clock one interval and surface a fresh orphan.
		base := m.now()
		m.now = func() time.Time { return base.Add(2 * time.Minute) }
		eng.mu.Lock()
		eng.listHandles = []Handle{{ID: "sbx-orphan-2", SandboxID: "sbx-orphan-2"}}
		eng.mu.Unlock()
		m.maybeLightReconcile(context.Background())
		if n := eng.destroyCount("sbx-orphan-2"); n != 1 {
			t.Fatalf("second-interval orphan reaped %d, want 1", n)
		}
	})

	t.Run("disabled when interval non-positive", func(t *testing.T) {
		m, eng := newManagerWithOrphan(t)
		m.reconcileInterval = 0
		m.maybeLightReconcile(context.Background())
		if n := eng.destroyCount("sbx-orphan"); n != 0 {
			t.Fatalf("disabled reconcile reaped %d, want 0", n)
		}
	})
}
// Env override for the cadence: "0" disables; a duration overrides the default.
func TestReconcileIntervalEnv(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		m := NewManager(DefaultConfig(), newFakeEngine(), nil)
		defer m.Close()
		if m.reconcileInterval != defaultReconcileInterval {
			t.Fatalf("interval=%v, want %v", m.reconcileInterval, defaultReconcileInterval)
		}
	})
	t.Run("override", func(t *testing.T) {
		t.Setenv("ARANEA_SANDBOX_RECONCILE_INTERVAL", "90s")
		m := NewManager(DefaultConfig(), newFakeEngine(), nil)
		defer m.Close()
		if m.reconcileInterval != 90*time.Second {
			t.Fatalf("interval=%v, want 90s", m.reconcileInterval)
		}
	})
	t.Run("zero disables", func(t *testing.T) {
		t.Setenv("ARANEA_SANDBOX_RECONCILE_INTERVAL", "0")
		m := NewManager(DefaultConfig(), newFakeEngine(), nil)
		defer m.Close()
		if m.reconcileInterval != 0 {
			t.Fatalf("interval=%v, want 0 (disabled)", m.reconcileInterval)
		}
	})
	t.Run("unparseable keeps default", func(t *testing.T) {
		t.Setenv("ARANEA_SANDBOX_RECONCILE_INTERVAL", "soon-ish")
		m := NewManager(DefaultConfig(), newFakeEngine(), nil)
		defer m.Close()
		if m.reconcileInterval != defaultReconcileInterval {
			t.Fatalf("interval=%v, want default on bad env", m.reconcileInterval)
		}
	})
}
