package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/testutil"
	"aranea-agents/pkg/apierror"
)

func TestPublishTurnFailure_usesEnvelopeErrorFromTurn(t *testing.T) {
	bus := testutil.NewRecordingBus()
	activityBus := &gateCaptureBus{}
	evtPub := newChatTurnEventPublisher(nil, bus, activityBus, nil)
	orch := &ChatOrchestrator{
		core: chatTurnCoreDeps{TD: rt.TurnDeps{Pipeline: rt.EventPipeline{Bus: bus}}},
		turnLC: &chatTurnLifecycleImpl{
			sessionStateTransitor: noopSessionStateTransitor{},
			turnRecorder:          noopTurnRecorder{},
			turnEventPublisher:    evtPub,
		},
	}
	te := TurnError(TurnErrLLMCallFailed, "connection reset")
	orch.publishTurnFailure("sess-1", "run-1", "chat-service", te, "")

	events := activityBus.events()
	if len(events) != 1 {
		t.Fatalf("expected 1 ActivityEvent, got %d", len(events))
	}
	ev := events[0]
	if ev.Event != biz.ActivityEventFailed {
		t.Fatalf("expected event %s, got %s", biz.ActivityEventFailed, ev.Event)
	}
	if ev.Activity.Kind != biz.ActivityKindTask {
		t.Fatalf("expected kind %s, got %s", biz.ActivityKindTask, ev.Activity.Kind)
	}
	if ev.Activity.Status != biz.ActivityStatusFailed {
		t.Fatalf("expected status %s, got %s", biz.ActivityStatusFailed, ev.Activity.Status)
	}
	if ev.Activity.Meta["error_code"] != string(TurnErrLLMCallFailed) {
		t.Fatalf("unexpected error_code: %v", ev.Activity.Meta["error_code"])
	}
	if ev.Activity.Meta["run_id"] != "run-1" {
		t.Fatalf("expected run_id run-1, got %v", ev.Activity.Meta["run_id"])
	}
	if ev.Activity.Meta["error_hint"] == "" {
		t.Fatal("expected non-empty hint for LLM_CALL_FAILED")
	}
}

func TestPublishTurnFailure_pendingID(t *testing.T) {
	bus := testutil.NewRecordingBus()
	activityBus := &gateCaptureBus{}
	evtPub := newChatTurnEventPublisher(nil, bus, activityBus, nil)
	orch := &ChatOrchestrator{
		core: chatTurnCoreDeps{TD: rt.TurnDeps{Pipeline: rt.EventPipeline{Bus: bus}}},
		turnLC: &chatTurnLifecycleImpl{
			sessionStateTransitor: noopSessionStateTransitor{},
			turnRecorder:          noopTurnRecorder{},
			turnEventPublisher:    evtPub,
		},
	}
	orch.publishTurnFailure("sess-1", "", "pending-queue", TurnError(TurnErrTurnTimeout, "5m"), "pend-1")

	events := activityBus.events()
	if len(events) != 1 || events[0].Activity.Meta["pending_id"] != "pend-1" {
		t.Fatalf("expected pending_id pend-1, got %+v", events)
	}
}

func TestEnvelopeErrorFromTurn_redactsUnknownDetail(t *testing.T) {
	envErr := envelopeErrorFromTurn("", `POST "https://api.deepseek.com/v1/chat/completions": 400 Bad Request`)
	if envErr == nil {
		t.Fatal("expected error payload")
	}
	if envErr.Message == "" || envErr.Message == `POST "https://api.deepseek.com/v1/chat/completions": 400 Bad Request` {
		t.Fatalf("expected redacted user-facing message, got %q", envErr.Message)
	}
	if envErr.Hint == "" {
		t.Fatal("expected recovery hint")
	}
}

func TestTurnErrorCodeFromErr_kratos(t *testing.T) {
	err := TurnError(TurnErrTurnTimeout, "5m")
	if TurnErrorCodeFromErr(err) != TurnErrTurnTimeout {
		t.Fatalf("expected TURN_TIMEOUT")
	}
	generic := apierror.Internal("X", "something else")
	if TurnErrorCodeFromErr(generic) != "" {
		t.Fatal("expected empty code for unmapped error")
	}
}

// --- failTurn / markAndPublish tests ---

// recordingRunStatusTracker records PublishRunStatus calls for verification.
type recordingRunStatusTracker struct {
	noopRunStatusTracker
	published []runStatusPublish
}

type runStatusPublish struct {
	sessionID, runID, status, errMsg string
}

func (r *recordingRunStatusTracker) PublishRunStatus(sessionID, runID, status, errMsg string) {
	r.published = append(r.published, runStatusPublish{sessionID, runID, status, errMsg})
}

// newFailTurnTestOrch builds a ChatOrchestrator wired with a recording bus and
// the given runStatusTracker, suitable for testing failTurn/markAndPublish.
func newFailTurnTestOrch(bus *testutil.RecordingBus, activityBus *gateCaptureBus, rs runStatusTracker) *ChatOrchestrator {
	evtPub := newChatTurnEventPublisher(nil, bus, activityBus, nil)
	return &ChatOrchestrator{
		core: chatTurnCoreDeps{TD: rt.TurnDeps{Pipeline: rt.EventPipeline{Bus: bus}}},
		turnLC: &chatTurnLifecycleImpl{
			sessionStateTransitor: noopSessionStateTransitor{},
			turnRecorder:          noopTurnRecorder{},
			turnEventPublisher:    evtPub,
		},
		runMgr: &chatRunManagerImpl{
			runStatusTracker:    rs,
			pendingQueueManager: noopPendingQueueManager{},
			awaitCoordinator:    noopAwaitCoordinator{},
			sessionRunLifecycle: noopSessionRunLifecycle{},
		},
	}
}

func TestFailTurn_cascadeAndReturn(t *testing.T) {
	bus := testutil.NewRecordingBus()
	activityBus := &gateCaptureBus{}
	rs := &recordingRunStatusTracker{}
	orch := newFailTurnTestOrch(bus, activityBus, rs)

	turnStatus := "ok"
	var turnErr error
	var turnErrMsg string
	turnError := TurnError(TurnErrLLMCallFailed, "conn reset")

	userMsg, asstMsg, err := orch.failTurn(
		context.Background(), "sess-1", "run-1",
		&turnStatus, &turnErr, &turnErrMsg,
		turnError,
	)

	// Return values: error is propagated, messages are zero.
	if err != turnError {
		t.Fatalf("expected returned err to be the input err")
	}
	if userMsg.ID != "" || asstMsg.ID != "" {
		t.Fatalf("expected zero ChatMessage values, got userMsg.ID=%q asstMsg.ID=%q", userMsg.ID, asstMsg.ID)
	}

	// Turn state is marked.
	if turnStatus != "error" {
		t.Errorf("turnStatus = %q, want %q", turnStatus, "error")
	}
	if turnErr != turnError {
		t.Errorf("turnErr mismatch")
	}
	if turnErrMsg != turnError.Error() {
		t.Errorf("turnErrMsg = %q, want %q", turnErrMsg, turnError.Error())
	}

	// publishRunStatus called with status="failed".
	if len(rs.published) != 1 {
		t.Fatalf("expected 1 PublishRunStatus call, got %d", len(rs.published))
	}
	p := rs.published[0]
	if p.sessionID != "sess-1" || p.runID != "run-1" || p.status != "failed" {
		t.Errorf("PublishRunStatus = %+v", p)
	}

	// publishTurnFailure emitted a failed ActivityEvent.
	events := activityBus.events()
	if len(events) != 1 {
		t.Fatalf("expected 1 ActivityEvent, got %d", len(events))
	}
	if events[0].Activity.Meta["run_id"] != "run-1" {
		t.Errorf("expected run_id run-1, got %v", events[0].Activity.Meta["run_id"])
	}
}

func TestFailTurn_beforePublishCallback(t *testing.T) {
	bus := testutil.NewRecordingBus()
	activityBus := &gateCaptureBus{}
	rs := &recordingRunStatusTracker{}
	orch := newFailTurnTestOrch(bus, activityBus, rs)

	var callbackCalled bool
	var callbackRanBeforePublish bool

	turnStatus := "ok"
	var turnErr error
	var turnErrMsg string

	_, _, _ = orch.failTurn(
		context.Background(), "sess-1", "run-1",
		&turnStatus, &turnErr, &turnErrMsg,
		errors.New("test error"),
		withBeforePublish(func() {
			callbackCalled = true
			// beforePublish must run BEFORE publishRunStatus.
			callbackRanBeforePublish = len(rs.published) == 0
		}),
	)

	if !callbackCalled {
		t.Fatal("expected beforePublish callback to be called")
	}
	if !callbackRanBeforePublish {
		t.Fatal("expected beforePublish to run before publishRunStatus")
	}
	// After failTurn completes, publishRunStatus should have been called.
	if len(rs.published) != 1 {
		t.Fatalf("expected 1 PublishRunStatus call after failTurn, got %d", len(rs.published))
	}
}

func TestMarkAndPublish_setsStateAndPublishes(t *testing.T) {
	bus := testutil.NewRecordingBus()
	activityBus := &gateCaptureBus{}
	orch := newFailTurnTestOrch(bus, activityBus, noopRunStatusTracker{})

	turnStatus := "ok"
	var turnErr error
	var turnErrMsg string
	testErr := errors.New("persist failed")

	orch.markAndPublish("sess-1", "run-1", &turnStatus, &turnErr, &turnErrMsg, testErr)

	// Turn state is marked.
	if turnStatus != "error" {
		t.Errorf("turnStatus = %q, want %q", turnStatus, "error")
	}
	if turnErr != testErr {
		t.Errorf("turnErr mismatch")
	}
	if turnErrMsg != testErr.Error() {
		t.Errorf("turnErrMsg = %q, want %q", turnErrMsg, testErr.Error())
	}

	// publishTurnFailure emitted a failed ActivityEvent.
	events := activityBus.events()
	if len(events) != 1 {
		t.Fatalf("expected 1 ActivityEvent, got %d", len(events))
	}
	if events[0].Activity.Meta["run_id"] != "run-1" {
		t.Errorf("expected run_id run-1, got %v", events[0].Activity.Meta["run_id"])
	}
}
