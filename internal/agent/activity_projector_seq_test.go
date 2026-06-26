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

	// Trigger concurrent thinking + reply creation. The On* methods take
	// p.mu internally, so the actual race surface is the activity creation
	// order vs the seq allocation order.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		p.OnReasoningDelta(ctx, "author-1", "thinking content", true)
	}()
	go func() {
		defer wg.Done()
		p.OnTextDelta(ctx, "author-1", "reply content")
	}()
	wg.Wait()

	// Wait for async persist writes to land in the repo. The sequencer's
	// persist worker writes after publish, so the bus is not enough — we
	// poll the repo with a short timeout.
	var thinkingSeq, replySeq int64
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
		}
		repo.mu.Unlock()
		if thinkingSeq != 0 && replySeq != 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if thinkingSeq == 0 {
		t.Errorf("thinking activity has Seq=0 (not pre-allocated)")
	}
	if replySeq == 0 {
		t.Errorf("reply activity has Seq=0 (not pre-allocated)")
	}
	if thinkingSeq >= replySeq {
		t.Errorf("expected thinking.Seq (%d) < reply.Seq (%d) — pre-allocation broken", thinkingSeq, replySeq)
	}
}
