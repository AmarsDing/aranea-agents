package service

import (
	"testing"
	"time"
)

func TestTurnIdemRegistry_ClaimFirstAndDuplicate(t *testing.T) {
	r := newTurnIdemRegistry()
	if !r.claim("s1", "req-1") {
		t.Fatal("first claim must succeed")
	}
	if r.claim("s1", "req-1") {
		t.Fatal("duplicate claim within TTL must be rejected")
	}
}

func TestTurnIdemRegistry_KeyScopedBySession(t *testing.T) {
	r := newTurnIdemRegistry()
	if !r.claim("s1", "req-1") {
		t.Fatal("first claim must succeed")
	}
	// 同一 requestID 落在不同 session 不互相去重（客户端键仅 session 内唯一）。
	if !r.claim("s2", "req-1") {
		t.Fatal("same requestID in another session must not be deduped")
	}
}

func TestTurnIdemRegistry_EmptyRequestIDNeverDeduped(t *testing.T) {
	r := newTurnIdemRegistry()
	for i := 0; i < 3; i++ {
		if !r.claim("s1", "") {
			t.Fatal("empty requestID must bypass dedup (channel/cron entries)")
		}
		if !r.claim("s1", "   ") {
			t.Fatal("blank requestID must bypass dedup")
		}
	}
}

func TestTurnIdemRegistry_ReleaseAllowsRetry(t *testing.T) {
	r := newTurnIdemRegistry()
	if !r.claim("s1", "req-1") {
		t.Fatal("first claim must succeed")
	}
	r.release("s1", "req-1")
	if !r.claim("s1", "req-1") {
		t.Fatal("claim after release must succeed (failed turn retry)")
	}
}

func TestTurnIdemRegistry_ExpiredKeyReclaimable(t *testing.T) {
	r := newTurnIdemRegistry()
	if !r.claim("s1", "req-1") {
		t.Fatal("first claim must succeed")
	}
	// 白盒：把登记时间拨到 TTL 之外，模拟窗口过期。
	r.mu.Lock()
	r.seen[turnIdemKey("s1", "req-1")] = time.Now().Add(-turnIdemTTL - time.Second)
	r.mu.Unlock()
	if !r.claim("s1", "req-1") {
		t.Fatal("claim after TTL expiry must succeed")
	}
}

func TestTurnIdemRegistry_SweepEvictsExpired(t *testing.T) {
	r := newTurnIdemRegistry()
	r.seen[turnIdemKey("s1", "old")] = time.Now().Add(-turnIdemTTL - time.Second)
	r.seen[turnIdemKey("s1", "fresh")] = time.Now()
	r.sweepLocked(time.Now())
	if _, ok := r.seen[turnIdemKey("s1", "old")]; ok {
		t.Fatal("sweep must evict expired keys")
	}
	if _, ok := r.seen[turnIdemKey("s1", "fresh")]; !ok {
		t.Fatal("sweep must keep fresh keys")
	}
}

func TestTurnIdemRegistry_NilSafe(t *testing.T) {
	var r *turnIdemRegistry
	if !r.claim("s1", "req-1") {
		t.Fatal("nil registry must not dedup")
	}
	r.release("s1", "req-1") // must not panic
}

func TestTurnIdemRegistry_OrchestratorNilSafe(t *testing.T) {
	var o *ChatOrchestrator
	if !o.claimTurnIdem("s1", "req-1") {
		t.Fatal("nil orchestrator must not dedup")
	}
	o.releaseTurnIdem("s1", "req-1") // must not panic
}
