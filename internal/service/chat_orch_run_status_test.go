package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/apierror"
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

// stubHydrateTracker stubs the registry miss + hydration snapshot for
// ChatService.GetRunStatus handler tests.
type stubHydrateTracker struct {
	noopRunStatusTracker
	snap persistedRunStatus
	ok   bool
}

func (s stubHydrateTracker) HydrateRunStatusFromSession(context.Context, string) (persistedRunStatus, bool) {
	return s.snap, s.ok
}

func newGetRunStatusTestService(tracker runStatusTracker) *ChatService {
	orch := &ChatOrchestrator{
		runs: rt.NewRunRegistry(),
		runMgr: &chatRunManagerImpl{
			runStatusTracker:    tracker,
			pendingQueueManager: noopPendingQueueManager{},
			awaitCoordinator:    noopAwaitCoordinator{},
			sessionRunLifecycle: noopSessionRunLifecycle{},
		},
	}
	return &ChatService{orch: orch, lg: loggateway.NewNoop()}
}

// TestGetRunStatus_HydratedTerminalWithoutRunIDReturns404 pins SP-1e (R3-Q13):
// terminal persistence clears run_id, so a hydrated snapshot with empty RunID
// is a stale leftover (S05/S10 evidence: runId="" + status=cancelled + day-old
// updatedAt). The API must answer 404 instead of serving the dirty snapshot.
func TestGetRunStatus_HydratedTerminalWithoutRunIDReturns404(t *testing.T) {
	svc := newGetRunStatusTestService(stubHydrateTracker{
		snap: persistedRunStatus{RunID: "", Status: "cancelled", UpdatedAt: "2026-08-28T04:04:50Z"},
		ok:   true,
	})
	_, err := svc.GetRunStatus(context.Background(), &chatv1.GetRunStatusRequest{SessionId: "sess-1"})
	if err == nil {
		t.Fatal("expected 404 for hydrated terminal snapshot without run_id")
	}
	if !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("expected NotFound, got %v", err)
	}
}

// TestGetRunStatus_HydratedLiveRunSurvives guards crash recovery: a non-terminal
// hydrated snapshot keeps its run_id and must still be served (status reattach).
func TestGetRunStatus_HydratedLiveRunSurvives(t *testing.T) {
	svc := newGetRunStatusTestService(stubHydrateTracker{
		snap: persistedRunStatus{RunID: "run-1", Status: "awaiting_user", UpdatedAt: "2026-08-29T12:00:00+08:00"},
		ok:   true,
	})
	resp, err := svc.GetRunStatus(context.Background(), &chatv1.GetRunStatusRequest{SessionId: "sess-1"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.RunId != "run-1" || resp.Status != "awaiting_user" {
		t.Fatalf("got runId=%q status=%q", resp.RunId, resp.Status)
	}
}

// TestGetRunStatus_NoRunAnywhereReturnsIdle pins the unchanged default: a
// session that never ran gets the idle zero-state, not a 404.
func TestGetRunStatus_NoRunAnywhereReturnsIdle(t *testing.T) {
	svc := newGetRunStatusTestService(stubHydrateTracker{ok: false})
	resp, err := svc.GetRunStatus(context.Background(), &chatv1.GetRunStatusRequest{SessionId: "sess-1"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != "idle" {
		t.Fatalf("got status=%q, want idle", resp.Status)
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

// TestClearAwaitingRunStateSync_AtomicPatchNoRMW pins the P2-2 fix
// (2026-10-19): clearing await state must go through a single atomic
// PatchSessionState delete call and must NOT round-trip through
// Get+delete+Save — that read-modify-write could silently overwrite a
// concurrent await-marker patch landing between the read and the write.
func TestClearAwaitingRunStateSync_AtomicPatchNoRMW(t *testing.T) {
	sessions := newRecordingSessionStatePort()
	tracker := newChatRunStatusTracker(rt.NewRunRegistry(), sessions, &captureEventBus{}, loggateway.NewNoop())

	// Pre-populate state with run/await keys plus a foreign key that must survive.
	sessions.mu.Lock()
	sessions.state[stateKeyRunID] = "run-old"
	sessions.state[stateKeyRunStatus] = "awaiting_user"
	sessions.state[stateKeyRunError] = "boom"
	sessions.state[stateKeyRunUpdatedAt] = "2026-10-19T00:00:00Z"
	sessions.state[stateKeyAwaitRunID] = "run-old"
	sessions.state[stateKeyAwaitSince] = "2026-10-19T00:00:01Z"
	sessions.state[stateKeyAwaitKind] = "tool"
	sessions.state[stateKeyAwaitToolKey] = "k"
	sessions.state[stateKeyAwaitToolCallID] = "tc-1"
	sessions.state["custom.keep"] = "v"
	sessions.mu.Unlock()

	if err := tracker.ClearAwaitingRunStateSync(context.Background(), "sess-1"); err != nil {
		t.Fatalf("ClearAwaitingRunStateSync failed: %v", err)
	}

	sessions.mu.Lock()
	defer sessions.mu.Unlock()

	// (1) No read-modify-write: SaveSessionState must never be called.
	if sessions.saves != 0 {
		t.Fatalf("SaveSessionState called %d times; want 0 (atomic patch only, no RMW)", sessions.saves)
	}

	// (2) Exactly one patch: nil sets, all nine obsolete keys in deletes.
	if len(sessions.patches) != 1 {
		t.Fatalf("expected 1 patch, got %d", len(sessions.patches))
	}
	patch := sessions.patches[0]
	if len(patch.sets) != 0 {
		t.Fatalf("patch sets = %v; want empty (delete-only patch)", patch.sets)
	}
	wantDeletes := map[string]bool{
		stateKeyRunID: false, stateKeyRunStatus: false, stateKeyRunError: false,
		stateKeyRunUpdatedAt: false, stateKeyAwaitRunID: false, stateKeyAwaitSince: false,
		stateKeyAwaitKind: false, stateKeyAwaitToolKey: false, stateKeyAwaitToolCallID: false,
	}
	if len(patch.deletes) != len(wantDeletes) {
		t.Fatalf("patch deletes = %v; want %d keys", patch.deletes, len(wantDeletes))
	}
	for _, d := range patch.deletes {
		if _, ok := wantDeletes[d]; !ok {
			t.Fatalf("unexpected delete key %q in %v", d, patch.deletes)
		}
		wantDeletes[d] = true
	}
	for k, seen := range wantDeletes {
		if !seen {
			t.Fatalf("expected delete key %q missing from %v", k, patch.deletes)
		}
	}

	// (3) The foreign key survives; the run/await keys are gone.
	if sessions.state["custom.keep"] != "v" {
		t.Fatalf("unrelated key custom.keep lost: state = %v", sessions.state)
	}
	for k := range wantDeletes {
		if _, ok := sessions.state[k]; ok {
			t.Fatalf("key %q still present after clear: state = %v", k, sessions.state)
		}
	}
}
