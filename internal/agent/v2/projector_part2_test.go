package v2

import (
	"aranea-agents/internal/biz"
	"context"
	"strings"
	"testing"
)

// TestEmitStreaming_CheckpointStripsFactMarks verifies checkpoint snapshots
// pass through StripFactMarks (publish copy only; the entity keeps the raw
// accumulation for the final OnTextDone authoritative strip).
func TestEmitStreaming_CheckpointStripsFactMarks(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil

	p.handleTextDelta(context.Background(), `你好。<fact type="identity" confidence="high">name=张三</fact>`)
	var ckpt *biz.StepUpdatedEvent
	for _, ev := range capture.events {
		if u, ok := ev.(*biz.StepUpdatedEvent); ok {
			ckpt = u
		}
	}
	if ckpt == nil {
		t.Fatal("expected checkpoint step.updated on first delta")
	}
	if strings.Contains(ckpt.Step.Content, "<fact") {
		t.Errorf("expected fact tags stripped in checkpoint snapshot, got %q", ckpt.Step.Content)
	}
	if !strings.Contains(ckpt.Step.Content, "你好。") {
		t.Errorf("expected visible text preserved, got %q", ckpt.Step.Content)
	}
}

// TestHandleTextDone_EmptyContentCancelled verifies that when a ReplyStep was
// created (e.g. by an earlier non-blank delta that turned out to be only
// whitespace after TrimSpace) but finalContent is empty, the step is finalized
// with Status=cancelled (not completed) so the frontend can filter it out.
//
// Spec: docs/superpowers/specs/2026-07-04-empty-reply-step-cleanup-design.md §4.1.2
func TestHandleTextDone_EmptyContentCancelled(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil

	stepID := p.BeginStep(context.Background(), p.meta, biz.StepKindReply)
	p.mu.Lock()
	if step, ok := p.activeStep[stepID]; ok {
		step.Content = " "
	}
	p.mu.Unlock()
	p.replyStepIDs[p.meta.AgentKey] = stepID
	capture.events = nil

	p.handleTextDone(context.Background(), "")

	if len(capture.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(capture.events))
	}
	completed, ok := capture.events[0].(*biz.StepCompletedEvent)
	if !ok {
		t.Fatalf("expected StepCompletedEvent, got %T", capture.events[0])
	}
	if completed.Step.Status != biz.StepStatusCancelled {
		t.Errorf("expected status=cancelled, got %s", completed.Step.Status)
	}
	if completed.Step.IsFinal {
		t.Error("expected IsFinal=false for cancelled empty reply")
	}
	if stepID, ok := p.replyStepIDs[p.meta.AgentKey]; ok && stepID != "" {
		t.Errorf("expected replyStepIDs cleared after done, got %q", stepID)
	}
}

// TestHandleTextDone_NormalReply verifies the normal path still works:
// real content → Status=completed, IsFinal=true.
func TestHandleTextDone_NormalReply(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil

	p.handleTextDelta(context.Background(), "Hello")
	capture.events = nil

	p.handleTextDone(context.Background(), "Hello world")

	if len(capture.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(capture.events))
	}
	completed, ok := capture.events[0].(*biz.StepCompletedEvent)
	if !ok {
		t.Fatalf("expected StepCompletedEvent, got %T", capture.events[0])
	}
	if completed.Step.Status != biz.StepStatusCompleted {
		t.Errorf("expected status=completed, got %s", completed.Step.Status)
	}
	if !completed.Step.IsFinal {
		t.Error("expected IsFinal=true for normal reply")
	}
	if completed.Step.Content != "Hello world" {
		t.Errorf("expected content='Hello world', got %q", completed.Step.Content)
	}
}

// TestHandleTextDone_StripsFactMarks verifies that <fact> machine-extraction
// tags (injected via the memory prompt convention) are stripped from the
// persisted/displayed reply content at finalize time. Fact extraction itself
// happens upstream (v1 orchestrator immediateFactWriter); the projector only
// ensures user-visible content never carries raw tags.
func TestHandleTextDone_StripsFactMarks(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil

	p.handleTextDelta(context.Background(), "你好，张三！")
	capture.events = nil

	raw := "你好，张三！很高兴认识你。\n\n" +
		`<fact type="identity" confidence="high">The user's name is 张三</fact>` + "\n" +
		`<fact type="preference" confidence="high">User likes coffee</fact>`
	p.handleTextDone(context.Background(), raw)

	if len(capture.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(capture.events))
	}
	completed, ok := capture.events[0].(*biz.StepCompletedEvent)
	if !ok {
		t.Fatalf("expected StepCompletedEvent, got %T", capture.events[0])
	}
	if strings.Contains(completed.Step.Content, "<fact") || strings.Contains(completed.Step.Content, "</fact>") {
		t.Errorf("expected fact tags stripped, got %q", completed.Step.Content)
	}
	if !strings.Contains(completed.Step.Content, "你好，张三！很高兴认识你。") {
		t.Errorf("expected visible text preserved, got %q", completed.Step.Content)
	}
}

// TestHandleTextDone_NoReplyStep_NoOp verifies that when no ReplyStep exists
// and finalContent is empty, no events are emitted (existing no-op path).
func TestHandleTextDone_NoReplyStep_NoOp(t *testing.T) {
	p, capture := testProjector()
	capture.events = nil

	p.handleTextDone(context.Background(), "")

	if len(capture.events) != 0 {
		t.Fatalf("expected 0 events for no-op, got %d", len(capture.events))
	}
	if stepID, ok := p.replyStepIDs[p.meta.AgentKey]; ok && stepID != "" {
		t.Errorf("expected replyStepIDs empty, got %q", stepID)
	}
}

func TestNewStep_SynthesisTurnReplyAuthorOverridden(t *testing.T) {
	meta := ProjectMeta{AgentKey: "agent-spirit", Synthesis: true}
	reply := meta.newStep("st-1", biz.StepKindReply, 1)
	if reply.AuthorAgentKey != biz.SynthesisAuthorAgentKey {
		t.Fatalf("synthesis reply AuthorAgentKey = %q, want %q", reply.AuthorAgentKey, biz.SynthesisAuthorAgentKey)
	}
}

func TestNewStep_SynthesisTurnNonReplyNotOverridden(t *testing.T) {
	meta := ProjectMeta{AgentKey: "agent-spirit", Synthesis: true}
	for _, kind := range []biz.StepKind{biz.StepKindThinking, biz.StepKindAction, biz.StepKindNotice} {
		step := meta.newStep("st-x", kind, 1)
		if step.AuthorAgentKey != "agent-spirit" {
			t.Fatalf("kind=%s AuthorAgentKey = %q, want original agent key", kind, step.AuthorAgentKey)
		}
	}
}

func TestNewStep_NormalTurnReplyNotOverridden(t *testing.T) {
	meta := ProjectMeta{AgentKey: "agent-spirit"}
	reply := meta.newStep("st-1", biz.StepKindReply, 1)
	if reply.AuthorAgentKey != "agent-spirit" {
		t.Fatalf("normal reply AuthorAgentKey = %q, want agent-spirit", reply.AuthorAgentKey)
	}
}
