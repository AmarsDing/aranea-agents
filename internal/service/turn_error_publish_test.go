package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

func TestPublishTurnFailure_usesEnvelopeErrorFromTurn(t *testing.T) {
	activityBus := &captureEventBus{}
	evtPub := newChatTurnEventPublisher(nil, activityBus, nil, loggateway.NewNoop())
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
	evtPub := newChatTurnEventPublisher(nil, activityBus, nil, loggateway.NewNoop())
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

// recordingRunStatusTracker records SetRunStatus / PublishRunStatus calls.
type recordingRunStatusTracker struct {
	noopRunStatusTracker
	published []runStatusPublish
	setCalls  []runStatusPublish
}

type runStatusPublish struct {
	sessionID, runID, status, errMsg string
}

func (r *recordingRunStatusTracker) SetRunStatus(_ context.Context, sessionID, runID, status, errMsg string) error {
	r.setCalls = append(r.setCalls, runStatusPublish{sessionID, runID, status, errMsg})
	// Mirror prior PublishRunStatus observation used by failTurn tests (C-11
	// now routes through SetRunStatus → persist+publish).
	r.published = append(r.published, runStatusPublish{sessionID, runID, status, errMsg})
	return nil
}

func (r *recordingRunStatusTracker) PublishRunStatus(sessionID, runID, status, errMsg string) {
	r.published = append(r.published, runStatusPublish{sessionID, runID, status, errMsg})
}

// newFailTurnTestOrch builds a ChatOrchestrator wired with the given
// runStatusTracker, suitable for testing failTurn/markAndPublish.
func newFailTurnTestOrch(activityBus *captureEventBus, rs runStatusTracker) *ChatOrchestrator {
	evtPub := newChatTurnEventPublisher(nil, activityBus, nil, loggateway.NewNoop())
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

	// setRunStatus called with status="failed" (C-11: persist+publish path).
	if len(rs.setCalls) != 1 {
		t.Fatalf("expected 1 SetRunStatus call, got %d", len(rs.setCalls))
	}
	p := rs.setCalls[0]
	if p.sessionID != "sess-1" || p.runID != "run-1" || p.status != "failed" {
		t.Errorf("SetRunStatus = %+v", p)
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
			// beforePublish must run BEFORE setRunStatus (terminal persist+publish).
			callbackRanBeforePublish = len(rs.setCalls) == 0
		}),
	)

	if !callbackCalled {
		t.Fatal("expected beforePublish callback to be called")
	}
	if !callbackRanBeforePublish {
		t.Fatal("expected beforePublish to run before SetRunStatus")
	}
	// After failTurn completes, SetRunStatus should have been called.
	if len(rs.setCalls) != 1 {
		t.Fatalf("expected 1 SetRunStatus call after failTurn, got %d", len(rs.setCalls))
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

// ── D1-F1：统一取消判别（isUserCancelledErr）────────────────────────────────
// 87-orchestration-chain-hardening：三处团队回调（executeTeamTurnViaHooks /
// pending-queue / StartTeamTurn）必须以同一判别分流「用户取消」——err 链
// context.Canceled / ctx.Err() / context.Cause(ctx) 三源任一命中即为取消。
// context.DeadlineExceeded 显式不是用户取消（ TeamMaxLifetime 墙钟超时是
// infra 失败语义，必须走 failed 路径）。

func TestIsUserCancelledErr(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	causeCtx, causeCancel := context.WithCancelCause(context.Background())
	causeCancel(context.Canceled)

	// 已超时的 deadline ctx：Err() == DeadlineExceeded，不是取消。
	deadlineCtx, deadlineCancel := context.WithTimeout(context.Background(), -time.Second)
	defer deadlineCancel()

	tests := []struct {
		name string
		ctx  context.Context
		err  error
		want bool
	}{
		{name: "err wraps context.Canceled", ctx: context.Background(), err: fmt.Errorf("runner: %w", context.Canceled), want: true},
		{name: "bare context.Canceled err", ctx: context.Background(), err: context.Canceled, want: true},
		{name: "ctx cancelled, nil err", ctx: cancelledCtx, err: nil, want: true},
		{name: "ctx cancelled + generic err", ctx: cancelledCtx, err: errors.New("boom"), want: true},
		{name: "WithCancelCause(context.Canceled)", ctx: causeCtx, err: nil, want: true},
		{name: "DeadlineExceeded err is NOT user cancel", ctx: context.Background(), err: context.DeadlineExceeded, want: false},
		{name: "exceeded deadline ctx is NOT user cancel", ctx: deadlineCtx, err: nil, want: false},
		{name: "generic error, live ctx", ctx: context.Background(), err: errors.New("boom"), want: false},
		{name: "nil ctx", ctx: nil, err: context.Canceled, want: true},
		{name: "nil everything", ctx: nil, err: nil, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isUserCancelledErr(tc.ctx, tc.err); got != tc.want {
				t.Fatalf("isUserCancelledErr = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestRunWasCancelled_DelegatesCancelSignals pins the delegation contract:
// registry(SessionRunPhaseCancelled) stays the priority check, then the
// discriminator defers to isUserCancelledErr (shared with TeamStarter).
func TestRunWasCancelled_DelegatesCancelSignals(t *testing.T) {
	newOrch := func() (*ChatOrchestrator, *rt.RunRegistry) {
		reg := rt.NewRunRegistry()
		orch := &ChatOrchestrator{
			runs:      reg,
			turnLC:    newNoopChatTurnLifecycle(),
			runMgr:    newNoopChatRunManager(),
			infraDeps: ChatInfraDeps{LG: loggateway.NewNoop()},
		}
		return orch, reg
	}

	t.Run("registry cancelled status wins priority", func(t *testing.T) {
		orch, reg := newOrch()
		reg.SetStatus("sess-1", "run-1", biz.SessionRunPhaseCancelled, "")
		// Even with a non-cancellation error signal, registry status short-circuits.
		if !orch.runWasCancelled(context.Background(), "sess-1", errors.New("boom")) {
			t.Fatal("registry cancelled status must short-circuit to true")
		}
	})

	t.Run("wrapped context.Canceled delegates to isUserCancelledErr", func(t *testing.T) {
		orch, _ := newOrch()
		if !orch.runWasCancelled(context.Background(), "sess-1", fmt.Errorf("llm stream: %w", context.Canceled)) {
			t.Fatal("wrapped context.Canceled must be treated as user cancel")
		}
	})

	t.Run("ctx cancelled with nil err delegates", func(t *testing.T) {
		orch, _ := newOrch()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if !orch.runWasCancelled(ctx, "sess-1", nil) {
			t.Fatal("cancelled ctx must be treated as user cancel")
		}
	})

	t.Run("DeadlineExceeded stays not-user-cancel", func(t *testing.T) {
		orch, _ := newOrch()
		if orch.runWasCancelled(context.Background(), "sess-1", context.DeadlineExceeded) {
			t.Fatal("DeadlineExceeded must NOT be treated as user cancel (wall-clock timeout = failure)")
		}
	})

	t.Run("generic error is not user cancel", func(t *testing.T) {
		orch, _ := newOrch()
		if orch.runWasCancelled(context.Background(), "sess-1", errors.New("boom")) {
			t.Fatal("generic error must not be treated as user cancel")
		}
	})
}
