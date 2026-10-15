package sandbox

import (
	"context"
	"testing"
	"time"
)

// Review 2026-08-26 #2: file ops refresh the idle clock (touch was Exec-only,
// so fs-only sessions were idle-destroyed while actively in use).
func TestFileOpsTouchIdleClock(t *testing.T) {
	eng := newFakeEngine()
	m := newTestManager(t, eng, nil)
	base := time.Now()
	cur := base
	m.now = func() time.Time { return cur }
	ctx := context.Background()
	lease, err := m.Acquire(ctx, AcquireReq{AgentKey: "a1"})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	cur = base.Add(5 * time.Minute)
	if err := lease.WriteFile(ctx, "/tmp/a.txt", []byte("x")); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	e, ok := m.registry.get(lease.SandboxID())
	if !ok {
		t.Fatal("lease gone")
	}
	if got := e.view.LastExecAt; !got.Equal(cur) {
		t.Fatalf("LastExecAt after WriteFile=%v, want %v", got, cur)
	}

	cur = base.Add(9 * time.Minute)
	_, _, _ = lease.ReadFile(ctx, "/tmp/missing", 0)
	if got := e.view.LastExecAt; !got.Equal(cur) {
		t.Fatalf("LastExecAt after ReadFile=%v, want %v", got, cur)
	}

	cur = base.Add(11 * time.Minute)
	if rc, err := lease.CopyDirFrom(ctx, "/workspace/out"); err == nil {
		_ = rc.Close()
	}
	if got := e.view.LastExecAt; !got.Equal(cur) {
		t.Fatalf("LastExecAt after CopyDirFrom=%v, want %v", got, cur)
	}

	cur = base.Add(14 * time.Minute)
	m.gcOnce()
	if _, ok := m.registry.get(lease.SandboxID()); !ok {
		t.Fatal("active fs-only lease was idle-destroyed (review #2 regression)")
	}
}

func TestSessionLeasePropagatesRunID(t *testing.T) {
	eng := newFakeEngine()
	m := newTestManager(t, eng, nil)
	store := NewSessionLeases(m)

	ctx := WithRunID(context.Background(), "run-42")
	lease, err := store.Acquire(ctx, "sess-1", "agent-x")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	e, ok := m.registry.get(lease.SandboxID())
	if !ok {
		t.Fatal("lease not registered")
	}
	if e.view.RunID != "run-42" {
		t.Fatalf("RunID=%q, want run-42", e.view.RunID)
	}
	if e.view.AgentKey != "agent-x" {
		t.Fatalf("AgentKey=%q, want agent-x (review 2026-08-26 #1)", e.view.AgentKey)
	}
	if got := m.quota.RunSnapshot()["run-42"]; got != 1 {
		t.Fatalf("run-42 created=%d, want 1", got)
	}

	_ = lease.Release(ctx)
	if _, err2 := store.Acquire(ctx, "sess-1", "agent-x"); err2 != nil {
		t.Fatalf("re-Acquire after release: %v", err2)
	}
	if l := store.leases["sess-1"]; l == lease {
		t.Fatal("stale lease entry survived manager-side destroy (hook misfire)")
	}
}
