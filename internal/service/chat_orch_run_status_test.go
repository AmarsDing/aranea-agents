package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

func TestCriticalTerminalRunStatus(t *testing.T) {
	for _, s := range []string{"completed", "failed", "cancelled", "interrupted", "COMPLETED"} {
		if !criticalTerminalRunStatus(s) {
			t.Fatalf("%q should be critical terminal", s)
		}
	}
	for _, s := range []string{"running", "awaiting_user", "idle", ""} {
		if criticalTerminalRunStatus(s) {
			t.Fatalf("%q should not be critical terminal", s)
		}
	}
}

func TestSetRunStatusWithAwait_TerminalPersistFailureSkipsPublish(t *testing.T) {
	sessions := newRecordingSessionStatePort()
	sessions.err = errors.New("db down")
	bus := &captureEventBus{}
	reg := rt.NewRunRegistry()
	reg.SetStatus("sess-1", "run-1", "running", "")
	tracker := newChatRunStatusTracker(reg, sessions, bus, loggateway.NewNoop())

	err := tracker.SetRunStatusWithAwait(context.Background(), "sess-1", "run-1", "completed", "", nil)
	if err == nil {
		t.Fatal("expected persist error")
	}
	if bus.count() != 0 {
		t.Fatalf("expected no WS publish on terminal persist failure, got %d", bus.count())
	}
	entry, ok := reg.GetStatus("sess-1")
	if !ok || entry.Status != "running" {
		t.Fatalf("memory status should remain running, got ok=%v status=%q", ok, entry.Status)
	}
}

func TestSetRunStatusWithAwait_TerminalPersistThenPublish(t *testing.T) {
	sessions := newRecordingSessionStatePort()
	bus := &captureEventBus{}
	reg := rt.NewRunRegistry()
	reg.SetStatus("sess-1", "run-1", "running", "")
	tracker := newChatRunStatusTracker(reg, sessions, bus, loggateway.NewNoop())

	if err := tracker.SetRunStatusWithAwait(context.Background(), "sess-1", "run-1", "completed", "", nil); err != nil {
		t.Fatal(err)
	}
	if bus.count() != 1 {
		t.Fatalf("expected 1 WS publish, got %d", bus.count())
	}
	ev, ok := bus.snapshot()[0].(*biz.RunStatusEvent)
	if !ok || ev.Status != "completed" {
		t.Fatalf("expected completed RunStatusEvent, got %#v", bus.snapshot()[0])
	}
	if len(sessions.patches) == 0 {
		t.Fatal("expected persist patch")
	}
	if sessions.patches[0].sets[stateKeyRunStatus] != "completed" {
		t.Fatalf("persist status = %q", sessions.patches[0].sets[stateKeyRunStatus])
	}
}

func TestSetRunStatusWithAwait_NonTerminalBestEffortOnPersistFailure(t *testing.T) {
	sessions := newRecordingSessionStatePort()
	sessions.err = errors.New("db down")
	bus := &captureEventBus{}
	reg := rt.NewRunRegistry()
	tracker := newChatRunStatusTracker(reg, sessions, bus, loggateway.NewNoop())

	if err := tracker.SetRunStatusWithAwait(context.Background(), "sess-1", "run-1", "running", "", nil); err != nil {
		t.Fatalf("non-terminal should not return persist error: %v", err)
	}
	if bus.count() != 1 {
		t.Fatalf("expected WS publish despite persist failure, got %d", bus.count())
	}
}

func TestSetRunStatusWithAwait_CancelledWinsOverFailed(t *testing.T) {
	sessions := newRecordingSessionStatePort()
	bus := &captureEventBus{}
	reg := rt.NewRunRegistry()
	reg.SetStatus("sess-1", "run-1", biz.SessionRunPhaseCancelled, "")
	tracker := newChatRunStatusTracker(reg, sessions, bus, loggateway.NewNoop())

	if err := tracker.SetRunStatusWithAwait(context.Background(), "sess-1", "run-1", "failed", "boom", nil); err != nil {
		t.Fatalf("cancelled→failed should be silent no-op, got %v", err)
	}
	if bus.count() != 0 {
		t.Fatalf("expected no publish when refusing failed overwrite, got %d", bus.count())
	}
	entry, ok := reg.GetStatus("sess-1")
	if !ok || entry.Status != biz.SessionRunPhaseCancelled {
		t.Fatalf("status should stay cancelled, got ok=%v status=%q", ok, entry.Status)
	}
	if len(sessions.patches) != 0 {
		t.Fatalf("expected no persist on cancelled→failed refuse, got %d patches", len(sessions.patches))
	}
}

func TestRunWasCancelled_StatusAndContext(t *testing.T) {
	reg := rt.NewRunRegistry()
	reg.SetStatus("sess-1", "run-1", biz.SessionRunPhaseCancelled, "")
	orch := &ChatOrchestrator{runs: reg}

	if !orch.runWasCancelled(context.Background(), "sess-1", nil) {
		t.Fatal("expected cancelled from registry status")
	}

	orch2 := &ChatOrchestrator{runs: rt.NewRunRegistry()}
	if !orch2.runWasCancelled(context.Background(), "sess-2", context.Canceled) {
		t.Fatal("expected cancelled from turnErr")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !orch2.runWasCancelled(ctx, "sess-2", errors.New("other")) {
		t.Fatal("expected cancelled from ctx.Err()")
	}

	causeCtx, causeCancel := context.WithCancelCause(context.Background())
	causeCancel(context.Canceled)
	if !orch2.runWasCancelled(causeCtx, "sess-2", nil) {
		t.Fatal("expected cancelled from context.Cause")
	}

	if orch2.runWasCancelled(context.Background(), "sess-2", errors.New("boom")) {
		t.Fatal("expected not cancelled")
	}
}

func TestFailTurn_skipsFailedWhenCancelled(t *testing.T) {
	activityBus := &captureEventBus{}
	rs := &recordingRunStatusTracker{}
	reg := rt.NewRunRegistry()
	reg.SetStatus("sess-1", "run-1", biz.SessionRunPhaseCancelled, "")
	orch := newFailTurnTestOrch(activityBus, rs)
	orch.runs = reg

	turnStatus := "ok"
	var turnErr error
	var turnErrMsg string
	_, _, _ = orch.failTurn(
		context.Background(), "sess-1", "run-1",
		&turnStatus, &turnErr, &turnErrMsg,
		errors.New("runner exit"),
	)
	if len(rs.published) != 0 {
		t.Fatalf("expected no failed PublishRunStatus, got %+v", rs.published)
	}
	if !strings.Contains(turnErrMsg, "runner exit") {
		t.Fatalf("turn still marked error, msg=%q", turnErrMsg)
	}
}
