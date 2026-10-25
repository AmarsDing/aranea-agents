package v2

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
)

// E2E-C1: when the outbox Insert fails (including after the FR-9 retry), the
// event's delivery seq must be RESET to 0 before live fan-out. Otherwise the
// live frame still carries an outbox cursor event_id for a row that does not
// exist — the client advances its replay cursor to that phantom id, and on
// reconnect ListAfter(afterEventID) finds no row and replays nothing (the
// ws_v2_subscriber invariant "cursor advances only on replayable frames" is
// broken; missed critical frames — e.g. team_summary — are lost permanently).

func TestSequencer_OutboxInsertFailure_ClearsDeliverySeq(t *testing.T) {
	t.Parallel()

	outbox := &fr9ConflictOutbox{alwaysErr: errors.New("db down")}
	s := fr9TestSequencer(t, outbox)

	ctx := context.Background()
	task := biz.Task{ID: "task-c1", SessionID: "sess-1", Status: biz.TaskStatusCompleted, Seq: 1, Version: 1}
	ev := biz.NewTaskCompletedEvent(task)
	s.Publish(ctx, ev)
	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	if len(outbox.inserted) != 0 {
		t.Fatalf("outbox durable rows = %d, want 0 (insert must fail)", len(outbox.inserted))
	}
	if got := ev.DeliverySeqValue(); got != 0 {
		t.Fatalf("E2E-C1: delivery seq after failed insert = %d, want 0 (live frame must carry no cursor)", got)
	}
}

func TestSequencer_OutboxSeqConflictRetryExhausted_ClearsDeliverySeq(t *testing.T) {
	t.Parallel()

	// Both the initial insert and the FR-9 conflict retry fail.
	outbox := &fr9ConflictOutbox{failNext: 2}
	s := fr9TestSequencer(t, outbox)

	ctx := context.Background()
	task := biz.Task{ID: "task-c1-2x", SessionID: "sess-1", Status: biz.TaskStatusCompleted, Seq: 1, Version: 1}
	ev := biz.NewTaskCompletedEvent(task)
	s.Publish(ctx, ev)
	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	outbox.mu.Lock()
	defer outbox.mu.Unlock()
	if outbox.insertCalls != 2 {
		t.Fatalf("Insert calls = %d, want 2 (initial + one FR-9 retry)", outbox.insertCalls)
	}
	if got := ev.DeliverySeqValue(); got != 0 {
		t.Fatalf("E2E-C1: delivery seq after exhausted retry = %d, want 0", got)
	}
}
