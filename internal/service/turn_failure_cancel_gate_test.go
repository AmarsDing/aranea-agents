package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
)

// E2E-B2: on user-cancel the projector emits the authoritative task.cancelled;
// publishing task.failed on top upserts a GHOST failure card (fabricated
// uuid Task with no persisted counterpart) showing two conflicting terminal
// states for the same turn. Both terminal cascades — failTurn (BUILD phase)
// and the runSingleAgentViaTRPC EXECUTE defer — must skip the failure event
// once the run is cancelled.

func TestFailTurn_skipsFailureEventOnUserCancel(t *testing.T) {
	activityBus := &captureEventBus{}
	rs := &recordingRunStatusTracker{}
	orch := newFailTurnTestOrch(activityBus, rs)
	reg := rt.NewRunRegistry()
	reg.SetStatus("sess-1", "run-1", biz.SessionRunPhaseCancelled, "user stop")
	orch.runs = reg

	turnStatus := "ok"
	var turnErr error
	var turnErrMsg string

	_, _, err := orch.failTurn(
		context.Background(), "sess-1", "run-1",
		&turnStatus, &turnErr, &turnErrMsg,
		errors.New("llm blew up mid-build"),
	)
	if err == nil {
		t.Fatal("failTurn must still propagate the input error")
	}
	// No ghost task.failed on the cancel path — the projector's
	// task.cancelled is the single authoritative terminal.
	if events := activityBus.snapshot(); len(events) != 0 {
		t.Fatalf("cancel path must not publish failure events, got %d", len(events))
	}
	// No failed status overwrite either (P1-03 semantics unchanged).
	if len(rs.setCalls) != 0 {
		t.Fatalf("cancel path must not set failed run status, got %d SetRunStatus calls", len(rs.setCalls))
	}
}

func TestFailTurn_publishesFailureEventOnGenuineError(t *testing.T) {
	activityBus := &captureEventBus{}
	rs := &recordingRunStatusTracker{}
	orch := newFailTurnTestOrch(activityBus, rs)
	orch.runs = rt.NewRunRegistry() // no cancelled marker — genuine failure

	turnStatus := "ok"
	var turnErr error
	var turnErrMsg string

	_, _, _ = orch.failTurn(
		context.Background(), "sess-1", "run-1",
		&turnStatus, &turnErr, &turnErrMsg,
		errors.New("provider 500"),
	)
	events := activityBus.snapshot()
	if len(events) != 1 {
		t.Fatalf("genuine failure must publish exactly 1 failure event, got %d", len(events))
	}
	if _, ok := events[0].(*biz.TaskFailedEvent); !ok {
		t.Fatalf("expected *biz.TaskFailedEvent, got %T", events[0])
	}
	if len(rs.setCalls) != 1 || rs.setCalls[0].status != "failed" {
		t.Fatalf("genuine failure must set failed run status, got %+v", rs.setCalls)
	}
}
