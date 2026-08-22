package trpcmem

import (
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/pkg/loggateway"
)

// fakeDeadLetterSink is a test double for biz.MemoryDeadLetterSink.
type fakeDeadLetterSink struct {
	mu      sync.Mutex
	entries []fakeDeadLetterEntry
}

type fakeDeadLetterEntry struct {
	Request biz.MemoryDeadLetterRequest
	Reason  biz.MemoryDeadLetterReason
	LastErr string
}

func (f *fakeDeadLetterSink) WriteMemoryDeadLetter(r biz.MemoryDeadLetterRequest, reason biz.MemoryDeadLetterReason, lastErr string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, fakeDeadLetterEntry{Request: r, Reason: reason, LastErr: lastErr})
}

func (f *fakeDeadLetterSink) Entries() []fakeDeadLetterEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]fakeDeadLetterEntry, len(f.entries))
	copy(cp, f.entries)
	return cp
}

// TestMemoryJobQueue_CoalescedJobsSkipDeadLetter R3（2026-08-22）：trailing-edge
// 合并后被取代的请求不写死信（它们已并入存活请求，不是丢失）。旧 P2-03 行为
// （写 debounced 死信）会让 replayer 把已合并的请求重新入队，去抖形同虚设。
func TestMemoryJobQueue_CoalescedJobsSkipDeadLetter(t *testing.T) {
	sink := &fakeDeadLetterSink{}
	q := NewMemoryJobQueue((*conf.Runtime)(nil), 4, 60*time.Millisecond, loggateway.NewNoop())
	defer q.Close()
	q.SetDeadLetterSink(sink)

	for i := 0; i < 3; i++ {
		q.Enqueue(AutoMemoryJobRequest{
			AppName:    "app1",
			SessionID:  "sess-dlq-debounce",
			Priority:   MemoryJobPriorityNormal,
			EnqueuedAt: time.Now(),
		})
	}

	// 合并计数仍然递增（观测口径保留）。
	_, debounced := q.Stats()
	if debounced != 2 {
		t.Fatalf("expected 2 coalesced, got %d", debounced)
	}

	// 静默期满后恰好交付一条。
	select {
	case <-q.Chan():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for coalesced job")
	}

	// 死信 sink 不得出现 debounced 条目。
	for _, e := range sink.Entries() {
		if e.Reason == biz.MemoryDeadLetterReasonDebounced {
			t.Fatalf("coalesced jobs must not be dead-lettered (R3), got: %+v", e)
		}
	}
}

// TestMemoryJobQueue_CoalesceNoDeadLetterWithoutSink verifies that when
// no dead-letter sink is wired, coalescing still works and no panic occurs.
func TestMemoryJobQueue_CoalesceNoDeadLetterWithoutSink(t *testing.T) {
	q := NewMemoryJobQueue((*conf.Runtime)(nil), 4, 60*time.Millisecond, loggateway.NewNoop())
	defer q.Close()
	// No SetDeadLetterSink — sink is nil.

	for i := 0; i < 2; i++ {
		q.Enqueue(AutoMemoryJobRequest{
			AppName:    "app1",
			SessionID:  "sess-no-sink",
			Priority:   MemoryJobPriorityNormal,
			EnqueuedAt: time.Now(),
		})
	}

	_, debounced := q.Stats()
	if debounced == 0 {
		t.Fatal("expected debounced count > 0 even without sink")
	}
	select {
	case <-q.Chan():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for coalesced job")
	}
}

// TestMemoryJobQueue_QuotaExceededWritesDeadLetter verifies that the
// existing dead-letter path (quota_exceeded) still works after the P2-03
// change. Uses the deterministic quota-exceeded path instead of queue-full
// (which races with the background drain goroutine). Both paths call the
// same writeDeadLetter helper, so this serves as a regression check for the
// writeDeadLetter mechanism.
func TestMemoryJobQueue_QuotaExceededWritesDeadLetter(t *testing.T) {
	sink := &fakeDeadLetterSink{}
	memConf := &conf.Runtime_MemoryQueue{
		HighCap:              64,
		NormalCap:            256,
		LowCap:               128,
		MaxTenantNormalSlots: 1, // only 1 in-flight normal job per tenant
	}
	rc := &conf.Runtime{MemoryQueue: memConf}
	// R3：normal 走 trailing-edge 合并，测试用小窗口；配额校验发生在 firing 时。
	q := NewMemoryJobQueue(rc, 4, 20*time.Millisecond, loggateway.NewNoop())
	defer q.Close()
	q.SetDeadLetterSink(sink)

	// First job: succeeds, reserves the tenant slot (in-flight → 1).
	// Use distinct session IDs so the debounce check doesn't suppress job 2.
	req1 := AutoMemoryJobRequest{
		AppName:    "app1",
		SessionID:  "sess-quota-1",
		Priority:   MemoryJobPriorityNormal,
		EnqueuedAt: time.Now(),
	}
	q.Enqueue(req1)
	// Drain job 1 to keep the normal queue clear; do NOT call AckDone so
	// the tenant in-flight counter stays at 1.
	select {
	case <-q.Chan():
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for job 1")
	}

	// Second job: same tenant (app1), in-flight=1 >= MaxTenantNormalSlots=1 → DLQ.
	req2 := AutoMemoryJobRequest{
		AppName:    "app1",
		SessionID:  "sess-quota-2",
		Priority:   MemoryJobPriorityNormal,
		EnqueuedAt: time.Now(),
	}
	q.Enqueue(req2)

	// 配额死信发生在 job2 的 debounce 窗口期满后（异步），轮询等待。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		found := false
		for _, e := range sink.Entries() {
			if e.Reason == biz.MemoryDeadLetterReasonQuotaExceeded {
				found = true
				break
			}
		}
		if found {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected quota_exceeded dead-letter within 2s, got: %v", sink.Entries())
}
