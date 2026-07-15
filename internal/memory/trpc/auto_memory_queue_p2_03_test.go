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

// TestMemoryJobQueue_DebounceWritesDeadLetter verifies that when a
// normal-priority job is debounced, a dead-letter entry is written with
// reason "debounced" (P2-03).
func TestMemoryJobQueue_DebounceWritesDeadLetter(t *testing.T) {
	sink := &fakeDeadLetterSink{}
	q := NewMemoryJobQueue((*conf.Runtime)(nil), 4, 10*time.Second, loggateway.NewNoop())
	defer q.Close()
	q.SetDeadLetterSink(sink)

	// First job: should be enqueued (not debounced).
	req1 := AutoMemoryJobRequest{
		AppName:    "app1",
		SessionID:  "sess-dlq-debounce",
		Priority:   MemoryJobPriorityNormal,
		EnqueuedAt: time.Now(),
	}
	q.Enqueue(req1)
	// Drain the first job so the debounce window starts.
	<-q.Chan()

	// Second job within debounce window: should be debounced.
	req2 := AutoMemoryJobRequest{
		AppName:    "app1",
		SessionID:  "sess-dlq-debounce",
		Priority:   MemoryJobPriorityNormal,
		EnqueuedAt: time.Now(),
	}
	q.Enqueue(req2)

	// Verify the debounced counter incremented.
	_, debounced := q.Stats()
	if debounced == 0 {
		t.Fatal("expected debounced count > 0")
	}

	// Verify the dead-letter sink received an entry with reason "debounced".
	entries := sink.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 dead-letter entry, got %d", len(entries))
	}
	if entries[0].Reason != biz.MemoryDeadLetterReasonDebounced {
		t.Errorf("expected reason %q, got %q", biz.MemoryDeadLetterReasonDebounced, entries[0].Reason)
	}
	if entries[0].Request.SessionID != "sess-dlq-debounce" {
		t.Errorf("expected session_id %q, got %q", "sess-dlq-debounce", entries[0].Request.SessionID)
	}
}

// TestMemoryJobQueue_DebounceNoDeadLetterWithoutSink verifies that when
// no dead-letter sink is wired, debounced jobs are still counted but no
// DLQ entry is written (no panic).
func TestMemoryJobQueue_DebounceNoDeadLetterWithoutSink(t *testing.T) {
	q := NewMemoryJobQueue((*conf.Runtime)(nil), 4, 10*time.Second, loggateway.NewNoop())
	defer q.Close()
	// No SetDeadLetterSink — sink is nil.

	req1 := AutoMemoryJobRequest{
		AppName:    "app1",
		SessionID:  "sess-no-sink",
		Priority:   MemoryJobPriorityNormal,
		EnqueuedAt: time.Now(),
	}
	q.Enqueue(req1)
	<-q.Chan()

	req2 := AutoMemoryJobRequest{
		AppName:    "app1",
		SessionID:  "sess-no-sink",
		Priority:   MemoryJobPriorityNormal,
		EnqueuedAt: time.Now(),
	}
	q.Enqueue(req2)

	_, debounced := q.Stats()
	if debounced == 0 {
		t.Fatal("expected debounced count > 0 even without sink")
	}
	// No panic should occur — writeDeadLetter handles nil sink.
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
	q := NewMemoryJobQueue(rc, 4, 0, loggateway.NewNoop())
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
	<-q.Chan()

	// Second job: same tenant (app1), in-flight=1 >= MaxTenantNormalSlots=1 → DLQ.
	req2 := AutoMemoryJobRequest{
		AppName:    "app1",
		SessionID:  "sess-quota-2",
		Priority:   MemoryJobPriorityNormal,
		EnqueuedAt: time.Now(),
	}
	q.Enqueue(req2)

	entries := sink.Entries()
	if len(entries) == 0 {
		t.Fatal("expected at least 1 dead-letter entry for quota_exceeded")
	}
	found := false
	for _, e := range entries {
		if e.Reason == biz.MemoryDeadLetterReasonQuotaExceeded {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected quota_exceeded reason, got: %v", entries)
	}
}
