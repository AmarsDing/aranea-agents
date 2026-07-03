package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/apierror"
)

func TestPublishTurnFailure_usesEnvelopeErrorFromTurn(t *testing.T) {
	activityBus := &captureEventBus{}
	evtPub := newChatTurnEventPublisher(nil, activityBus, nil)
	orch := &ChatOrchestrator{
		core: chatTurnCoreDeps{TD: rt.TurnDeps{Pipeline: rt.EventPipeline{}}},
		turnLC: &chatTurnLifecycleImpl{
			sessionStateTransitor: noopSessionStateTransitor{},
			turnRecorder:          noopTurnRecorder{},
			turnEventPublisher:    evtPub,
		},
	}
	te := TurnError(TurnErrLLMCallFailed, "connection reset")
	orch.publishTurnFailure("sess-1", "run-1", "chat-service", te, "")

	events := activityBus.snapshot()
	if len(events) != 1 {
		t.Fatalf("expected 1 Event, got %d", len(events))
	}
	ev, ok := events[0].(*biz.TaskFailedEvent)
	if !ok {
		t.Fatalf("expected *biz.TaskFailedEvent, got %T", events[0])
	}
	if ev.EventKind() != biz.EventKindTaskFailed {
		t.Fatalf("expected event kind %s, got %s", biz.EventKindTaskFailed, ev.EventKind())
	}
	if ev.Task.Status != biz.TaskStatusFailed {
		t.Fatalf("expected task status %s, got %s", biz.TaskStatusFailed, ev.Task.Status)
	}
	if ev.Task.SessionID != "sess-1" {
		t.Fatalf("expected session id sess-1, got %s", ev.Task.SessionID)
	}
}

func TestPublishTurnFailure_pendingID(t *testing.T) {
	activityBus := &captureEventBus{}
	evtPub := newChatTurnEventPublisher(nil, activityBus, nil)
	orch := &ChatOrchestrator{
		core: chatTurnCoreDeps{TD: rt.TurnDeps{Pipeline: rt.EventPipeline{}}},
		turnLC: &chatTurnLifecycleImpl{
			sessionStateTransitor: noopSessionStateTransitor{},
			turnRecorder:          noopTurnRecorder{},
			turnEventPublisher:    evtPub,
		},
	}
	// Phase 3b-D: v2 Task entity has no Meta field, so pending_id is no longer
	// propagated. This test now verifies that a non-empty pendingID does not
	// break the publish path and still emits exactly one TaskFailedEvent.
	orch.publishTurnFailure("sess-1", "", "pending-queue", TurnError(TurnErrTurnTimeout, "5m"), "pend-1")

	events := activityBus.snapshot()
	if len(events) != 1 {
		t.Fatalf("expected 1 Event, got %d", len(events))
	}
	if _, ok := events[0].(*biz.TaskFailedEvent); !ok {
		t.Fatalf("expected *biz.TaskFailedEvent, got %T", events[0])
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

// newFailTurnTestOrch builds a ChatOrchestrator wired with the given
// runStatusTracker, suitable for testing failTurn/markAndPublish.
func newFailTurnTestOrch(activityBus *captureEventBus, rs runStatusTracker) *ChatOrchestrator {
	evtPub := newChatTurnEventPublisher(nil, activityBus, nil)
	return &ChatOrchestrator{
		core: chatTurnCoreDeps{TD: rt.TurnDeps{Pipeline: rt.EventPipeline{}}},
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
	activityBus := &captureEventBus{}
	rs := &recordingRunStatusTracker{}
	orch := newFailTurnTestOrch(activityBus, rs)

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

	// publishTurnFailure emitted a failed TaskFailedEvent.
	events := activityBus.snapshot()
	if len(events) != 1 {
		t.Fatalf("expected 1 Event, got %d", len(events))
	}
	ev, ok := events[0].(*biz.TaskFailedEvent)
	if !ok {
		t.Fatalf("expected *biz.TaskFailedEvent, got %T", events[0])
	}
	if ev.Task.SessionID != "sess-1" {
		t.Errorf("expected session id sess-1, got %s", ev.Task.SessionID)
	}
}

func TestFailTurn_beforePublishCallback(t *testing.T) {
	activityBus := &captureEventBus{}
	rs := &recordingRunStatusTracker{}
	orch := newFailTurnTestOrch(activityBus, rs)

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
	activityBus := &captureEventBus{}
	orch := newFailTurnTestOrch(activityBus, noopRunStatusTracker{})

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

	// publishTurnFailure emitted a failed TaskFailedEvent.
	events := activityBus.snapshot()
	if len(events) != 1 {
		t.Fatalf("expected 1 Event, got %d", len(events))
	}
	ev, ok := events[0].(*biz.TaskFailedEvent)
	if !ok {
		t.Fatalf("expected *biz.TaskFailedEvent, got %T", events[0])
	}
	if ev.Task.SessionID != "sess-1" {
		t.Errorf("expected session id sess-1, got %s", ev.Task.SessionID)
	}
}
