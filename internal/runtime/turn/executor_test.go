package turn

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
)

type stubRunner struct {
	native biz.NativeTurnResult
	err    error
}

func (s stubRunner) RunWithOutcome(context.Context, biz.TurnInput) (biz.NativeTurnResult, error) {
	return s.native, s.err
}

func TestClassifyNativeOutcome_completed(t *testing.T) {
	native := biz.NativeTurnResult{
		Outcome:      biz.NativeTurnOutcomeCompleted,
		UserMsg:      biz.ChatMessage{ID: "u1", ContentMarkdown: "hi"},
		AssistantMsg: biz.ChatMessage{ID: "a1", ContentMarkdown: "hello"},
	}
	got, err := ClassifyNativeOutcome(native, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != biz.TurnOutcomeCompleted {
		t.Fatalf("outcome=%q", got.Outcome)
	}
	if got.Reply != "hello" {
		t.Fatalf("reply=%q", got.Reply)
	}
}

func TestClassifyNativeOutcome_queued(t *testing.T) {
	native := biz.NativeTurnResult{
		Outcome:   biz.NativeTurnOutcomeQueued,
		PendingID: "p1",
	}
	got, err := ClassifyNativeOutcome(native, queuedSentinel())
	if !errors.Is(err, QueuedSentinel) {
		t.Fatalf("err=%v", err)
	}
	if got.Outcome != biz.TurnOutcomeQueued || got.PendingID != "p1" {
		t.Fatalf("got=%+v", got)
	}
}

func TestClassifyNativeOutcome_failed(t *testing.T) {
	got, err := ClassifyNativeOutcome(biz.NativeTurnResult{Outcome: biz.NativeTurnOutcomeFailed}, errors.New("boom"))
	if err == nil {
		t.Fatal("expected error")
	}
	if got.Outcome != biz.TurnOutcomeFailed {
		t.Fatalf("outcome=%q", got.Outcome)
	}
}

func TestExecutor_Execute_delegates(t *testing.T) {
	ex := NewExecutor(stubRunner{
		native: biz.NativeTurnResult{
			Outcome:      biz.NativeTurnOutcomeCompleted,
			AssistantMsg: biz.ChatMessage{ContentMarkdown: "ok"},
		},
	})
	got, err := ex.Execute(context.Background(), biz.TurnInput{SessionID: "s1", Content: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Outcome != biz.TurnOutcomeCompleted {
		t.Fatalf("outcome=%q", got.Outcome)
	}
}

func TestExecutor_Execute_nilRunner(t *testing.T) {
	var ex *Executor
	_, err := ex.Execute(context.Background(), biz.TurnInput{SessionID: "s1", Content: "hi"})
	if err == nil {
		t.Fatal("expected error for nil runner")
	}
}

func TestClassifyNativeOutcome_queuedWithoutError(t *testing.T) {
	// Native outcome says queued but no error passed — still returns queued.
	native := biz.NativeTurnResult{
		Outcome:   biz.NativeTurnOutcomeQueued,
		PendingID: "p2",
	}
	got, err := ClassifyNativeOutcome(native, nil)
	if !errors.Is(err, QueuedSentinel) {
		t.Fatalf("expected QueuedSentinel, got err=%v", err)
	}
	if got.Outcome != biz.TurnOutcomeQueued || got.PendingID != "p2" {
		t.Fatalf("got=%+v", got)
	}
}

func TestClassifyNativeOutcome_failedWithoutError(t *testing.T) {
	// Native outcome says failed but no error — returns failed with nil error.
	got, err := ClassifyNativeOutcome(biz.NativeTurnResult{Outcome: biz.NativeTurnOutcomeFailed}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Outcome != biz.TurnOutcomeFailed {
		t.Fatalf("outcome=%q", got.Outcome)
	}
}

func TestClassifyNativeOutcome_unknownOutcome(t *testing.T) {
	// Unknown outcome value falls through to failed.
	got, err := ClassifyNativeOutcome(biz.NativeTurnResult{Outcome: "unknown_outcome"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Outcome != biz.TurnOutcomeFailed {
		t.Fatalf("outcome=%q want failed", got.Outcome)
	}
}

func TestClassifyNativeOutcome_errorWithQueuedSentinel(t *testing.T) {
	// Error is QueuedSentinel — should classify as queued.
	native := biz.NativeTurnResult{
		Outcome:   biz.NativeTurnOutcomeCompleted,
		PendingID: "p3",
	}
	got, err := ClassifyNativeOutcome(native, QueuedSentinel)
	if !errors.Is(err, QueuedSentinel) {
		t.Fatalf("expected QueuedSentinel, got err=%v", err)
	}
	if got.Outcome != biz.TurnOutcomeQueued || got.PendingID != "p3" {
		t.Fatalf("got=%+v", got)
	}
}

func TestExecutor_Execute_queued(t *testing.T) {
	ex := NewExecutor(stubRunner{
		native: biz.NativeTurnResult{
			Outcome:   biz.NativeTurnOutcomeQueued,
			PendingID: "pq1",
		},
		err: QueuedSentinel,
	})
	got, err := ex.Execute(context.Background(), biz.TurnInput{SessionID: "s1", Content: "hi"})
	if !errors.Is(err, QueuedSentinel) {
		t.Fatalf("err=%v", err)
	}
	if got.Outcome != biz.TurnOutcomeQueued || got.PendingID != "pq1" {
		t.Fatalf("got=%+v", got)
	}
}

func TestExecutor_Execute_failed(t *testing.T) {
	ex := NewExecutor(stubRunner{
		native: biz.NativeTurnResult{Outcome: biz.NativeTurnOutcomeFailed},
		err:    errors.New("agent error"),
	})
	got, err := ex.Execute(context.Background(), biz.TurnInput{SessionID: "s1", Content: "hi"})
	if err == nil {
		t.Fatal("expected error")
	}
	if got.Outcome != biz.TurnOutcomeFailed {
		t.Fatalf("outcome=%q", got.Outcome)
	}
}
