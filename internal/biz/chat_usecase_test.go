package biz

import (
	"context"
	"errors"
	"sync"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// --- stubs for ChatUsecase tests ---

// Compile-time checks: ensure stubs satisfy their respective interfaces.
var (
	_ ChatRunGateway         = (*stubChatRunGateway)(nil)
	_ ChatSessionLocker      = (*stubChatSessionLocker)(nil)
	_ ChatPendingQueue       = (*stubChatPendingQueue)(nil)
	_ ChatRunStatusPersister = (*stubChatPersister)(nil)
	_ ChatEventPublisher     = (*stubChatEventPublisher)(nil)
)

type stubChatRunGateway struct {
	mu             sync.Mutex
	hasActive      bool
	status         ChatRunStatus
	hasStatus      bool
	setStatusCnt   int
	setStatusCalls []stubChatSetStatusCall
}

type stubChatSetStatusCall struct {
	SessionID string
	RunID     string
	Status    string
	ErrMsg    string
}

func (s *stubChatRunGateway) HasActive(_ string) bool                  { return s.hasActive }
func (s *stubChatRunGateway) Cancel(_ string, _ string) (bool, string) { return false, "" }
func (s *stubChatRunGateway) EnqueueUserMessage(_ string, _ string) (bool, error) {
	return false, nil
}
func (s *stubChatRunGateway) SetStatus(sessionID, runID, status, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.setStatusCnt++
	s.setStatusCalls = append(s.setStatusCalls, stubChatSetStatusCall{
		SessionID: sessionID, RunID: runID, Status: status, ErrMsg: errMsg,
	})
	s.status = ChatRunStatus{RunID: runID, Status: status, ErrMsg: errMsg}
	s.hasStatus = true
}
func (s *stubChatRunGateway) GetStatus(_ string) (ChatRunStatus, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, s.hasStatus
}

type stubChatSessionLocker struct{}

func (stubChatSessionLocker) Lock(_ string) func() { return func() {} }

type stubChatPendingQueue struct{}

func (stubChatPendingQueue) List(string) []PendingQueueEntry    { return nil }
func (stubChatPendingQueue) Enqueue(_, _ string) string         { return "" }
func (stubChatPendingQueue) EnqueueFollowup(_, _ string) string { return "" }
func (stubChatPendingQueue) Dequeue(string) (PendingQueueEntry, bool) {
	return PendingQueueEntry{}, false
}
func (stubChatPendingQueue) Peek(string) (PendingQueueEntry, bool) { return PendingQueueEntry{}, false }
func (stubChatPendingQueue) Remove(string, string) bool            { return false }
func (stubChatPendingQueue) Update(string, string, string) bool    { return false }
func (stubChatPendingQueue) PromoteToFront(string, string) error   { return nil }
func (stubChatPendingQueue) SetPriority(string, string, int) error { return nil }
func (stubChatPendingQueue) Close()                                {}

type stubChatPersister struct {
	mu             sync.Mutex
	persistCnt     int
	persistCalls   []stubChatPersistCall
	failWith       error // if non-nil, PersistRunStatus returns this error
	awaitMarkerCnt int
	clearAwaitCnt  int
}

type stubChatPersistCall struct {
	SessionID string
	RunID     string
	Status    string
	ErrMsg    string
}

func (s *stubChatPersister) PersistRunStatus(_ context.Context, sessionID, runID, status, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.persistCnt++
	s.persistCalls = append(s.persistCalls, stubChatPersistCall{
		SessionID: sessionID, RunID: runID, Status: status, ErrMsg: errMsg,
	})
	return s.failWith
}
func (s *stubChatPersister) PersistAwaitMarkers(_ context.Context, _, _ string, _ ChatAwaitMeta) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.awaitMarkerCnt++
}
func (s *stubChatPersister) ClearAwaitingRunState(_ context.Context, _ string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.clearAwaitCnt++
}

type stubChatEventPublisher struct {
	mu                  sync.Mutex
	publishRunStatusCnt int
	publishRunCalls     []stubChatPublishCall
	publishQueuedCnt    int
}

type stubChatPublishCall struct {
	SessionID string
	RunID     string
	Status    string
	ErrMsg    string
}

func (s *stubChatEventPublisher) PublishRunStatus(sessionID, runID, status, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publishRunStatusCnt++
	s.publishRunCalls = append(s.publishRunCalls, stubChatPublishCall{
		SessionID: sessionID, RunID: runID, Status: status, ErrMsg: errMsg,
	})
}
func (s *stubChatEventPublisher) PublishMessageQueued(_ string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publishQueuedCnt++
}

func newChatUsecaseForTest(
	runs ChatRunGateway,
	persist ChatRunStatusPersister,
	publisher ChatEventPublisher,
) *ChatUsecase {
	return NewChatUsecase(
		runs,
		stubChatSessionLocker{},
		stubChatPendingQueue{},
		persist,
		publisher,
		loggateway.NewNoop(),
	)
}

// --- tests ---

// TestChatUsecase_SetRunStatusWithError_WBPF_PersistFailure verifies the WBPF
// invariant: when PersistRunStatus fails, in-memory state and event publishing
// must NOT be updated, keeping DB and memory consistent (BD1).
func TestChatUsecase_SetRunStatusWithError_WBPF_PersistFailure(t *testing.T) {
	t.Parallel()
	persistErr := errors.New("db connection lost")
	persist := &stubChatPersister{failWith: persistErr}
	runs := &stubChatRunGateway{}
	pub := &stubChatEventPublisher{}
	uc := newChatUsecaseForTest(runs, persist, pub)

	err := uc.SetRunStatusWithError(context.Background(), "sess-1", "run-1", "running", "")
	if !errors.Is(err, persistErr) {
		t.Fatalf("expected persist error %v, got %v", persistErr, err)
	}

	if persist.persistCnt != 1 {
		t.Fatalf("expected 1 persist call, got %d", persist.persistCnt)
	}
	if runs.setStatusCnt != 0 {
		t.Fatalf("WBPF violated: in-memory SetStatus called %d times after persist failure", runs.setStatusCnt)
	}
	if pub.publishRunStatusCnt != 0 {
		t.Fatalf("WBPF violated: PublishRunStatus called %d times after persist failure", pub.publishRunStatusCnt)
	}
}

// TestChatUsecase_SetRunStatusWithError_WBPF_Success verifies that on
// successful persist, both in-memory state and event publishing are updated.
func TestChatUsecase_SetRunStatusWithError_WBPF_Success(t *testing.T) {
	t.Parallel()
	persist := &stubChatPersister{}
	runs := &stubChatRunGateway{}
	pub := &stubChatEventPublisher{}
	uc := newChatUsecaseForTest(runs, persist, pub)

	err := uc.SetRunStatusWithError(context.Background(), "sess-1", "run-1", "running", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if persist.persistCnt != 1 {
		t.Fatalf("expected 1 persist call, got %d", persist.persistCnt)
	}
	if runs.setStatusCnt != 1 {
		t.Fatalf("expected 1 SetStatus call, got %d", runs.setStatusCnt)
	}
	if pub.publishRunStatusCnt != 1 {
		t.Fatalf("expected 1 PublishRunStatus call, got %d", pub.publishRunStatusCnt)
	}
}

// TestChatUsecase_SetRunStatusWithError_Bootstrap_SkipsValidation verifies
// that when no prior status exists (bootstrap/crash recovery), state machine
// validation is skipped to allow the first status to be set.
func TestChatUsecase_SetRunStatusWithError_Bootstrap_SkipsValidation(t *testing.T) {
	t.Parallel()
	persist := &stubChatPersister{}
	runs := &stubChatRunGateway{hasStatus: false} // no prior status
	pub := &stubChatEventPublisher{}
	uc := newChatUsecaseForTest(runs, persist, pub)

	// Even "completed" (a terminal state) should be accepted on bootstrap.
	err := uc.SetRunStatusWithError(context.Background(), "sess-1", "run-1", "completed", "")
	if err != nil {
		t.Fatalf("bootstrap should skip validation, got error: %v", err)
	}
	if persist.persistCnt != 1 {
		t.Fatalf("expected 1 persist call, got %d", persist.persistCnt)
	}
}

// TestChatUsecase_SetRunStatusWithError_IllegalTransition_Rejected verifies
// that illegal state transitions are rejected by the state machine (AS-FSM-01).
func TestChatUsecase_SetRunStatusWithError_IllegalTransition_Rejected(t *testing.T) {
	t.Parallel()
	persist := &stubChatPersister{}
	runs := &stubChatRunGateway{
		hasStatus: true,
		status:    ChatRunStatus{RunID: "run-1", Status: "completed"}, // terminal state
	}
	pub := &stubChatEventPublisher{}
	uc := newChatUsecaseForTest(runs, persist, pub)

	// completed → running is illegal (terminal state has no outgoing transitions)
	err := uc.SetRunStatusWithError(context.Background(), "sess-1", "run-1", "running", "")
	if err == nil {
		t.Fatal("expected error for illegal transition completed → running")
	}
	ae, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected apierror, got %T: %v", err, err)
	}
	if ae.Code != apierror.CodeBadRequest {
		t.Fatalf("expected code %s, got %s", apierror.CodeBadRequest, ae.Code)
	}
	// Persist must NOT be called for rejected transitions
	if persist.persistCnt != 0 {
		t.Fatalf("persist should not be called for rejected transition, got %d calls", persist.persistCnt)
	}
	if runs.setStatusCnt != 0 {
		t.Fatalf("SetStatus should not be called for rejected transition, got %d calls", runs.setStatusCnt)
	}
}

// TestChatUsecase_SetRunStatusWithError_ValidTransitions verifies that all
// legal transitions defined in runTransitionRules are accepted.
func TestChatUsecase_SetRunStatusWithError_ValidTransitions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		fromStatus string
		toStatus   string
	}{
		{"running_to_completed", "running", "completed"},
		{"running_to_failed", "running", "failed"},
		{"running_to_cancelled", "running", "cancelled"},
		{"running_to_awaiting_user", "running", "awaiting_user"},
		{"awaiting_user_to_running", "awaiting_user", "running"},
		{"awaiting_user_to_cancelled", "awaiting_user", "cancelled"},
		{"awaiting_user_to_failed", "awaiting_user", "failed"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			persist := &stubChatPersister{}
			runs := &stubChatRunGateway{
				hasStatus: true,
				status:    ChatRunStatus{RunID: "run-1", Status: tc.fromStatus},
			}
			pub := &stubChatEventPublisher{}
			uc := newChatUsecaseForTest(runs, persist, pub)

			err := uc.SetRunStatusWithError(context.Background(), "sess-1", "run-1", tc.toStatus, "")
			if err != nil {
				t.Fatalf("expected transition %s → %s to be valid, got error: %v",
					tc.fromStatus, tc.toStatus, err)
			}
			if persist.persistCnt != 1 {
				t.Fatalf("expected 1 persist call, got %d", persist.persistCnt)
			}
			if runs.setStatusCnt != 1 {
				t.Fatalf("expected 1 SetStatus call, got %d", runs.setStatusCnt)
			}
		})
	}
}

// TestChatUsecase_SetRunStatusWithError_TerminalStatus_CleansAwaitChannel
// verifies that reaching a terminal status proactively cleans up the await
// channel to prevent memory leaks.
func TestChatUsecase_SetRunStatusWithError_TerminalStatus_CleansAwaitChannel(t *testing.T) {
	t.Parallel()
	cases := []string{"completed", "failed", "cancelled"}
	for _, status := range cases {
		status := status
		t.Run(status, func(t *testing.T) {
			t.Parallel()
			persist := &stubChatPersister{}
			runs := &stubChatRunGateway{
				hasStatus: true,
				status:    ChatRunStatus{RunID: "run-1", Status: "running"},
			}
			pub := &stubChatEventPublisher{}
			uc := newChatUsecaseForTest(runs, persist, pub)

			// Register an await channel
			ch := make(AwaitChannel, 1)
			uc.RegisterAwaitChannel("sess-1", ch)
			if _, ok := uc.LoadAwaitChannel("sess-1"); !ok {
				t.Fatal("await channel should be registered")
			}

			err := uc.SetRunStatusWithError(context.Background(), "sess-1", "run-1", status, "")
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			// Await channel should be cleaned up on terminal status
			if _, ok := uc.LoadAwaitChannel("sess-1"); ok {
				t.Fatal("await channel should be cleaned up on terminal status")
			}
		})
	}
}

// TestChatUsecase_SetRunStatusWithError_NonTerminalStatus_KeepsAwaitChannel
// verifies that non-terminal statuses do NOT clean up the await channel.
func TestChatUsecase_SetRunStatusWithError_NonTerminalStatus_KeepsAwaitChannel(t *testing.T) {
	t.Parallel()
	persist := &stubChatPersister{}
	runs := &stubChatRunGateway{
		hasStatus: true,
		status:    ChatRunStatus{RunID: "run-1", Status: "running"},
	}
	pub := &stubChatEventPublisher{}
	uc := newChatUsecaseForTest(runs, persist, pub)

	ch := make(AwaitChannel, 1)
	uc.RegisterAwaitChannel("sess-1", ch)

	// running → awaiting_user is a valid non-terminal transition
	err := uc.SetRunStatusWithError(context.Background(), "sess-1", "run-1", "awaiting_user", "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if _, ok := uc.LoadAwaitChannel("sess-1"); !ok {
		t.Fatal("await channel should still exist after non-terminal status")
	}
}

// TestChatUsecase_SetRunStatus_DelegatesToWithError verifies that the
// legacy SetRunStatus method delegates to SetRunStatusWithError and discards
// the error (backward compatibility).
func TestChatUsecase_SetRunStatus_DelegatesToWithError(t *testing.T) {
	t.Parallel()
	persistErr := errors.New("persist failed")
	persist := &stubChatPersister{failWith: persistErr}
	runs := &stubChatRunGateway{}
	pub := &stubChatEventPublisher{}
	uc := newChatUsecaseForTest(runs, persist, pub)

	// SetRunStatus should not panic and should swallow the error
	uc.SetRunStatus(context.Background(), "sess-1", "run-1", "running", "")

	if persist.persistCnt != 1 {
		t.Fatalf("expected 1 persist call, got %d", persist.persistCnt)
	}
	// Even though persist failed, SetRunStatus swallows the error
	if runs.setStatusCnt != 0 {
		t.Fatalf("WBPF violated: SetStatus called %d times after persist failure", runs.setStatusCnt)
	}
}

// TestChatUsecase_SetRunStatusWithError_SameState_NoValidation verifies that
// when fromState == toState (idempotent update), state machine validation is
// skipped and the update proceeds normally.
func TestChatUsecase_SetRunStatusWithError_SameState_NoValidation(t *testing.T) {
	t.Parallel()
	persist := &stubChatPersister{}
	runs := &stubChatRunGateway{
		hasStatus: true,
		status:    ChatRunStatus{RunID: "run-1", Status: "running"},
	}
	pub := &stubChatEventPublisher{}
	uc := newChatUsecaseForTest(runs, persist, pub)

	// running → running (same state) should skip validation and succeed
	err := uc.SetRunStatusWithError(context.Background(), "sess-1", "run-1", "running", "")
	if err != nil {
		t.Fatalf("expected no error for same-state update, got %v", err)
	}
	if persist.persistCnt != 1 {
		t.Fatalf("expected 1 persist call, got %d", persist.persistCnt)
	}
}
