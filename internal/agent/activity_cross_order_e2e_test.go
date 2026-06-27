package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// TestActivityProjector_CrossOrderE2E simulates a realistic LLM turn:
// thinking → tool → thinking → reply, and verifies the publish order
// matches the expected business order.
func TestActivityProjector_CrossOrderE2E(t *testing.T) {
	t.Parallel()

	eventBus := &recordingEventBus{}
	repo := &fakeActivityRepo{}
	proj := NewActivityProjector(eventBus, repo, nil)
	proj.Configure(ProjectMeta{
		SessionID: "sess-e2e",
		RequestID: "turn-e2e",
		AgentID:   "agent-1",
	}, nil)
	proj.Reset()
	proj.OnTurnStart(context.Background(), ProjectMeta{
		SessionID: "sess-e2e",
		RequestID: "turn-e2e",
		AgentID:   "agent-1",
	})

	ctx := context.Background()
	startedAt := time.Now().UTC()
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		proj.OnReasoningDelta(ctx, "agent-1", "Let me think", true)
		proj.OnReasoningDelta(ctx, "agent-1", " about this problem", true)
		proj.OnReasoningDone(ctx, "agent-1", "Let me think about this problem", false)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		proj.OnToolCall(ctx, "tc-1", "search", `{"q":"foo"}`, "agent-1", startedAt)
		proj.OnToolResult(ctx, "tc-1", `{"results":[]}`, "success", "", 100)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		proj.OnReasoningDelta(ctx, "agent-1", "Now I have results", true)
		proj.OnReasoningDone(ctx, "agent-1", "Now I have results", false)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		proj.OnTextDelta(ctx, "agent-1", "The answer is 42")
		proj.OnTextDone(ctx, "agent-1", "The answer is 42")
	}()

	wg.Wait()
	proj.OnTurnEnd(ctx, &ActivityUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150})
	proj.Close()

	received := eventBus.received()
	if len(received) < 4 {
		t.Fatalf("expected ≥4 events, got %d", len(received))
	}

	seqToKind := make(map[int64]biz.ActivityKind)
	for _, a := range received {
		seqToKind[a.Seq] = a.Kind
	}

	var thinkingSeqs, replySeqs []int64
	for seq, kind := range seqToKind {
		if kind == biz.ActivityKindThinking {
			thinkingSeqs = append(thinkingSeqs, seq)
		}
		if kind == biz.ActivityKindReply {
			replySeqs = append(replySeqs, seq)
		}
	}

	if len(thinkingSeqs) == 0 {
		t.Fatal("no thinking activity in received events")
	}
	if len(replySeqs) == 0 {
		t.Fatal("no reply activity in received events")
	}

	maxThinkingSeq := thinkingSeqs[0]
	for _, s := range thinkingSeqs {
		if s > maxThinkingSeq {
			maxThinkingSeq = s
		}
	}
	minReplySeq := replySeqs[0]
	for _, s := range replySeqs {
		if s < minReplySeq {
			minReplySeq = s
		}
	}

	if minReplySeq <= maxThinkingSeq {
		t.Errorf("reply (min seq=%d) appeared before thinking (max seq=%d) — cross-activity order broken", minReplySeq, maxThinkingSeq)
	}
}
