package biz

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSessionLockManager_LockExclusive(t *testing.T) {
	m := NewSessionLockManager()
	defer m.Close()

	unlockA := m.Lock("s-1")
	var bGotLock atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		unlockB := m.Lock("s-1")
		bGotLock.Store(true)
		unlockB()
	}()

	// Give goroutine B a chance to start. It should still be blocked.
	time.Sleep(20 * time.Millisecond)
	if bGotLock.Load() {
		t.Fatal("Lock failed: goroutine B acquired the mutex while A held it")
	}
	unlockA()
	wg.Wait()
	if !bGotLock.Load() {
		t.Fatal("Lock failed: goroutine B never acquired the mutex after A released")
	}
}

func TestSessionLockManager_DifferentSessionsIndependent(t *testing.T) {
	m := NewSessionLockManager()
	defer m.Close()

	unlockA := m.Lock("s-1")
	unlockB := m.Lock("s-2")
	// Both must coexist; if not, this would deadlock.
	unlockA()
	unlockB()
}

func TestSessionLockManager_ReusesEntries(t *testing.T) {
	m := NewSessionLockManager()
	defer m.Close()

	unlock := m.Lock("s-1")
	unlock()
	unlock = m.Lock("s-1")
	unlock()
	if m.Len() != 1 {
		t.Fatalf("expected 1 entry after reusing, got %d", m.Len())
	}
}

func TestSessionLockManager_NilSafe(t *testing.T) {
	var m *SessionLockManager
	// All methods must be safe on a nil receiver.
	if n := m.Len(); n != 0 {
		t.Fatalf("nil Len() = %d, want 0", n)
	}
	m.Close()
	// Lock on nil would panic by design — that path is not expected to be
	// exercised in production because constructors always return a real
	// manager. The test merely documents the contract.
}

func TestSessionLockManager_CloseIdempotent(t *testing.T) {
	m := NewSessionLockManager()
	m.Close()
	m.Close() // must not panic on second close
}

func TestSessionLockManager_SatisfiesChatSessionLocker(t *testing.T) {
	// Compile-time check: *SessionLockManager must satisfy the existing
	// biz.ChatSessionLocker port. This guards against signature drift.
	var _ ChatSessionLocker = NewSessionLockManager()
}

// TestSessionLockManager_SweepPreservesHeldLock is a regression test for the
// defect where sweep() reaped an entry whose per-session mutex was still held
// by a long-running turn. Reaping a held entry lets the next Lock() create a
// fresh mutex and run concurrently with the in-flight turn, breaking mutual
// exclusion. The fix uses TryLock to skip held entries.
func TestSessionLockManager_SweepPreservesHeldLock(t *testing.T) {
	m := NewSessionLockManager()
	defer m.Close()

	unlock := m.Lock("s-long")

	// Force the entry to look idle beyond the threshold, then sweep.
	m.mu.Lock()
	e := m.entries["s-long"]
	e.lastUsed = time.Now().Add(-(SessionLockMaxIdle + time.Minute))
	m.mu.Unlock()

	m.sweep()

	// A concurrent acquirer for the same session must still block on the
	// original mutex, proving the held entry was preserved.
	var gotLock atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		u := m.Lock("s-long")
		gotLock.Store(true)
		u()
	}()
	time.Sleep(20 * time.Millisecond)
	if gotLock.Load() {
		unlock()
		t.Fatal("sweep reaped a held lock: concurrent acquirer ran while the original is still held")
	}
	unlock()
	wg.Wait()
	if !gotLock.Load() {
		t.Fatal("concurrent acquirer never acquired after the original was released")
	}
}

// TestSessionLockManager_SweepReapsIdleUnheld confirms that unheld idle
// entries are still reaped (the fix must not disable GC entirely).
func TestSessionLockManager_SweepReapsIdleUnheld(t *testing.T) {
	m := NewSessionLockManager()
	defer m.Close()

	u := m.Lock("s-idle")
	u() // release immediately → unheld

	m.mu.Lock()
	e := m.entries["s-idle"]
	e.lastUsed = time.Now().Add(-(SessionLockMaxIdle + time.Minute))
	m.mu.Unlock()

	m.sweep()
	if m.Len() != 0 {
		t.Fatalf("expected idle unheld entry to be reaped, got len=%d", m.Len())
	}
}
