package biz

import (
	"context"
	"errors"
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

// --- P1 FR-6：LockCtx 支持 ctx 取消（M-1） ---

// TestSessionLockManager_LockCtxCancelledWhileWaiting 复现 M-1(lock 侧)：
// Lock 不接受 ctx——被限流 turn 一旦阻塞在互斥上，连取消都解除不了。
// 修复期望：LockCtx 在 ctx 到期时返回 error，不取得锁。
func TestSessionLockManager_LockCtxCancelledWhileWaiting(t *testing.T) {
	m := NewSessionLockManager()
	defer m.Close()

	unlockA := m.Lock("s-ctx")
	defer unlockA()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	untilCtx, err := m.LockCtx(ctx, "s-ctx")
	if err == nil {
		untilCtx()
		t.Fatal("P1 FR-6: LockCtx must fail when ctx expires while waiting")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

// TestSessionLockManager_LockCtxAcquiresWhenFree：空锁快路径立即成功；
// 取消后再次 LockCtx 仍可获得（无残留等待态）。
func TestSessionLockManager_LockCtxAcquiresWhenFree(t *testing.T) {
	m := NewSessionLockManager()
	defer m.Close()

	unlock, err := m.LockCtx(context.Background(), "s-free")
	if err != nil {
		t.Fatalf("LockCtx on free session: %v", err)
	}
	unlock()

	// 占住后取消一个 waiter，再释放——waiter 不得残留影响后续获取。
	held := m.Lock("s-free")
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		u, err := m.LockCtx(ctx, "s-free")
		if err == nil {
			u()
		}
		done <- err
	}()
	time.Sleep(30 * time.Millisecond) // 让 waiter 进入重试循环
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled for waiter, got %v", err)
	}
	held()

	u2, err := m.LockCtx(context.Background(), "s-free")
	if err != nil {
		t.Fatalf("post-cancel LockCtx must succeed, got %v", err)
	}
	u2()
}

// TestSessionLockManager_LockDelegatesToLockCtx：原 Lock 语义不变（等至获得）。
func TestSessionLockManager_LockDelegatesToLockCtx(t *testing.T) {
	m := NewSessionLockManager()
	defer m.Close()

	held := m.Lock("s-delegate")
	var got atomic.Bool
	go func() {
		u := m.Lock("s-delegate")
		got.Store(true)
		u()
	}()
	time.Sleep(30 * time.Millisecond)
	if got.Load() {
		t.Fatal("Lock must block until the holder releases")
	}
	held()
	// 释放后等待 goroutine 获得锁：轮询截止 2s，避免调度时延抖动误红。
	deadline := time.Now().Add(2 * time.Second)
	for !got.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !got.Load() {
		t.Fatal("Lock must acquire once released")
	}
}
