package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// TestActivityProjector_SeqAllocatedInMainFlow verifies that every Activity
// created via the public On* methods has its Seq assigned BEFORE the publish
// task is enqueued. This is the core invariant restored by the seq
// pre-allocation refactor (fix for cross-activity ordering bug where reply
// appeared before thinking).
//
// Root cause of the original bug: activitySeq was lazy-allocated inside the
// per-activity consumer goroutine, so concurrent publish tasks for different
// activities acquired their Seq in bus-send order rather than creation order.
// When the bus channel was drained, the reply consumer sometimes won the race
// and the frontend saw reply before thinking.
//
// Invariant under test: every Activity created in an On* entry point has a
// non-zero Seq, and the creation order matches the Seq order
// (i.e. OnReasoningDelta -> thinking.Seq < OnTextDelta -> reply.Seq).
//
// Note: we use a sync barrier (thinkingDone) to enforce a deterministic
// ordering — OnReasoningDelta completes before OnTextDelta starts. Without
// the barrier, Go's goroutine scheduler does not guarantee that the first
// goroutine in source order acquires p.mu first, making the seq-order
// assertion flaky.
func TestActivityProjector_SeqAllocatedInMainFlow(t *testing.T) {
	p, _, repo := newTestProjector(t)
	ctx := context.Background()

	meta := ProjectMeta{
		SessionID: "sess-1",
		RequestID: "turn-1",
		AgentID:   "agent-1",
	}
	p.Configure(meta, nil)
	p.OnTurnStart(ctx, meta)

	// OnReasoningDelta runs first and signals thinkingDone. OnTextDelta waits
	// on thinkingDone so the lock-acquisition order is deterministic.
	var wg sync.WaitGroup
	var thinkingDone sync.WaitGroup
	thinkingDone.Add(1)
	wg.Add(2)
	go func() {
		defer wg.Done()
		p.OnReasoningDelta(ctx, "author-1", "thinking content", true)
		thinkingDone.Done()
	}()
	go func() {
		defer wg.Done()
		thinkingDone.Wait()
		p.OnTextDelta(ctx, "author-1", "reply content")
	}()
	wg.Wait()

	// Wait for async persist writes to land in the repo. The sequencer's
	// persist worker writes after publish, so the bus is not enough — we
	// poll the repo with a short timeout.
	var taskSeq, thinkingSeq, replySeq int64
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		repo.mu.Lock()
		for _, a := range repo.activities {
			if a.Kind == biz.ActivityKindReply {
				replySeq = a.Seq
			}
			if a.Kind == biz.ActivityKindThinking {
				thinkingSeq = a.Seq
			}
			if a.Kind == biz.ActivityKindTask {
				taskSeq = a.Seq
			}
		}
		repo.mu.Unlock()
		if taskSeq != 0 && thinkingSeq != 0 && replySeq != 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if taskSeq == 0 {
		t.Errorf("task activity has Seq=0 (not pre-allocated)")
	}
	if thinkingSeq == 0 {
		t.Errorf("thinking activity has Seq=0 (not pre-allocated)")
	}
	if replySeq == 0 {
		t.Errorf("reply activity has Seq=0 (not pre-allocated)")
	}
	// Strict monotonic ordering: OnTurnStart < OnReasoningDelta < OnTextDelta
	if !(taskSeq < thinkingSeq && thinkingSeq < replySeq) {
		t.Errorf("expected task.Seq (%d) < thinking.Seq (%d) < reply.Seq (%d) — pre-allocation broken",
			taskSeq, thinkingSeq, replySeq)
	}
}
