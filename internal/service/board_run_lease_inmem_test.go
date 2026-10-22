package service

import (
	"context"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
)

// P0-6 阶段 0：InMem 语义钉死——包现有 boardRunRegistry 行为（先存者赢、
// 幂等释放、session 匹配门控、并发幂等），为阶段 1 PG 迁移提供对照基线。

func TestInMemBoardRunLease_TryClaimFirstWins(t *testing.T) {
	t.Parallel()
	l := NewInMemBoardRunLease()
	ok := l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "b1", SessionID: "s1"})
	if !ok {
		t.Fatal("first claim must win")
	}
	// 同 board 二次认领必拒，且不覆盖既有租约（含 session 归属）。
	ok = l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "b1", SessionID: "s2"})
	if ok {
		t.Fatal("second claim for same board must lose")
	}
	if !l.HasActiveForSession("s1") || l.HasActiveForSession("s2") {
		t.Fatal("existing lease session must not be overwritten")
	}
	// 释放后可重新认领。
	l.Release("b1")
	if !l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "b1", SessionID: "s2"}) {
		t.Fatal("re-claim after release must win")
	}
}

func TestInMemBoardRunLease_BadInputRejected(t *testing.T) {
	t.Parallel()
	l := NewInMemBoardRunLease()
	if l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: ""}) {
		t.Fatal("empty board id must be rejected")
	}
	if l.Renew(context.Background(), "") || l.Renew(context.Background(), "nope") {
		t.Fatal("renew on missing/empty id must return false")
	}
	l.Release("") // 幂等不 panic
	if l.HasActiveForSession("") {
		t.Fatal("empty session must never be active")
	}
	var nilLease *InMemBoardRunLease
	if nilLease.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "x"}) {
		t.Fatal("nil receiver must reject")
	}
	nilLease.Release("x")
}

func TestInMemBoardRunLease_RenewUnknownFalse(t *testing.T) {
	t.Parallel()
	l := NewInMemBoardRunLease()
	if l.Renew(context.Background(), "b1") {
		t.Fatal("renew unknown board must be false")
	}
	l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "b1", SessionID: "s1"})
	if !l.Renew(context.Background(), "b1") {
		t.Fatal("renew active board must be true")
	}
	l.Release("b1")
	if l.Renew(context.Background(), "b1") {
		t.Fatal("renew released board must be false")
	}
}

// 并发幂等：N goroutine 抢同一 board 恰有 1 个赢家；释放后计数归零。
func TestInMemBoardRunLease_ConcurrentSingleWinner(t *testing.T) {
	t.Parallel()
	l := NewInMemBoardRunLease()
	var wg sync.WaitGroup
	wins := make(chan bool, 64)
	for i := 0; i < 64; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wins <- l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "b", SessionID: "s"})
		}()
	}
	wg.Wait()
	close(wins)
	won := 0
	for w := range wins {
		if w {
			won++
		}
	}
	if won != 1 {
		t.Fatalf("winners=%d, want exactly 1", won)
	}
	if l.ActiveLeaseCount() != 1 {
		t.Fatalf("ActiveLeaseCount=%d", l.ActiveLeaseCount())
	}
	l.Release("b")
	if l.ActiveLeaseCount() != 0 {
		t.Fatalf("after release ActiveLeaseCount=%d", l.ActiveLeaseCount())
	}
	// 与现有 running 行为对齐：Release 幂等（重复释放不得 panic/误删他人租约）。
	l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "b2", SessionID: "s2"})
	l.Release("b")
	if l.ActiveLeaseCount() != 1 || !l.HasActiveForSession("s2") {
		t.Fatal("idempotent release must not drop other leases")
	}
}

// HasActiveForSession 语义与 PlanExecutor.HasActiveRunForSession 对齐。
func TestInMemBoardRunLease_HasActiveForSession(t *testing.T) {
	t.Parallel()
	l := NewInMemBoardRunLease()
	l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "b1", SessionID: "s1"})
	l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "b2", SessionID: "s1"})
	l.TryClaim(context.Background(), biz.BoardRunLease{BoardID: "b3", SessionID: "s2"})
	if !l.HasActiveForSession("s1") || !l.HasActiveForSession("s2") {
		t.Fatal("expected active sessions s1/s2")
	}
	if l.HasActiveForSession("s3") {
		t.Fatal("unknown session must be inactive")
	}
	l.Release("b1")
	l.Release("b2")
	if l.HasActiveForSession("s1") {
		t.Fatal("session must be inactive after both leases released")
	}
}
