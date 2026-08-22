package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

// stubAwaitCoord is a configurable awaitCoordinator for submitAwaitReply tests.
// Unset function fields fall back to the noop behavior (channel miss / cannot
// resume), so each test only wires the branch it exercises.
type stubAwaitCoord struct {
	noopAwaitCoordinator
	trySendFn   func(sessionID string, msg biz.AwaitReplyMsg) bool
	loadFn      func(sessionID string) (biz.AwaitChannel, bool)
	canResumeFn func(ctx context.Context, sessionID string) (string, bool)
}

func (s stubAwaitCoord) TrySendAwaitChannel(sessionID string, msg biz.AwaitReplyMsg) bool {
	if s.trySendFn != nil {
		return s.trySendFn(sessionID, msg)
	}
	return false
}

func (s stubAwaitCoord) LoadAwaitChannel(sessionID string) (biz.AwaitChannel, bool) {
	if s.loadFn != nil {
		return s.loadFn(sessionID)
	}
	return nil, false
}

func (s stubAwaitCoord) CanResumeAwait(ctx context.Context, sessionID string) (string, bool) {
	if s.canResumeFn != nil {
		return s.canResumeFn(ctx, sessionID)
	}
	return "", false
}

// stubSessionTurnManagerGet embeds the composite interface and overrides only
// Get, which ConfirmActivity needs for the ownership check.
type stubSessionTurnManagerGet struct {
	biz.SessionTurnManager
	getFn func(ctx context.Context, id string) (biz.Session, error)
}

func (s stubSessionTurnManagerGet) Get(ctx context.Context, id string) (biz.Session, error) {
	return s.getFn(ctx, id)
}

// stubStepV2ReaderGet embeds the reader interface and returns a fixed step.
type stubStepV2ReaderGet struct {
	biz.StepV2Reader
	step biz.Step
}

func (s stubStepV2ReaderGet) GetStep(context.Context, string) (biz.Step, error) {
	return s.step, nil
}

func newSubmitAwaitReplyTestOrch(coord awaitCoordinator) *ChatOrchestrator {
	return &ChatOrchestrator{
		runs: rt.NewRunRegistry(),
		runMgr: &chatRunManagerImpl{
			runStatusTracker:    noopRunStatusTracker{},
			pendingQueueManager: noopPendingQueueManager{},
			awaitCoordinator:    coord,
			sessionRunLifecycle: noopSessionRunLifecycle{},
		},
		infraDeps: ChatInfraDeps{LG: loggateway.NewNoop()},
	}
}

// ---------------------------------------------------------------------------
// submitAwaitReply unit tests
// ---------------------------------------------------------------------------

func TestSubmitAwaitReply_DeliveredViaChannel(t *testing.T) {
	var sent biz.AwaitReplyMsg
	coord := stubAwaitCoord{trySendFn: func(_ string, msg biz.AwaitReplyMsg) bool {
		sent = msg
		return true
	}}
	orch := newSubmitAwaitReplyTestOrch(coord)
	resumeCalled := false
	orch.resumeAwaitFn = func(context.Context, string, string, string) error {
		resumeCalled = true
		return nil
	}

	outcome, err := orch.submitAwaitReply(context.Background(), "sess-1", awaitReply{
		runID:         "run-1",
		token:         "approved",
		resumeContent: "用户已批准",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != awaitReplyDelivered {
		t.Fatalf("outcome = %v, want awaitReplyDelivered", outcome)
	}
	if sent.RunID != "run-1" || sent.Reply != "approved" {
		t.Fatalf("channel received wrong msg: %+v", sent)
	}
	if resumeCalled {
		t.Fatal("resume must not be called when the channel accepts the reply")
	}
}

func TestSubmitAwaitReply_ChannelFullRejected(t *testing.T) {
	coord := stubAwaitCoord{
		trySendFn: func(string, biz.AwaitReplyMsg) bool { return false },
		loadFn: func(string) (biz.AwaitChannel, bool) {
			return make(biz.AwaitChannel, 1), true // entry still exists → merely full
		},
		canResumeFn: func(context.Context, string) (string, bool) { return "run-1", true },
	}
	orch := newSubmitAwaitReplyTestOrch(coord)
	resumeCalled := false
	orch.resumeAwaitFn = func(context.Context, string, string, string) error {
		resumeCalled = true
		return nil
	}

	outcome, err := orch.submitAwaitReply(context.Background(), "sess-1", awaitReply{token: "approved"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != awaitReplyRejected {
		t.Fatalf("outcome = %v, want awaitReplyRejected (channel full is not a restart)", outcome)
	}
	if resumeCalled {
		t.Fatal("resume must not run while the channel entry still exists (double-delivery risk)")
	}
}

func TestSubmitAwaitReply_RestartResumeUsesPersistedRunID(t *testing.T) {
	coord := stubAwaitCoord{
		trySendFn:   func(string, biz.AwaitReplyMsg) bool { return false },
		canResumeFn: func(context.Context, string) (string, bool) { return "run-persisted", true },
	}
	orch := newSubmitAwaitReplyTestOrch(coord)
	var gotReply, gotRunID string
	orch.resumeAwaitFn = func(_ context.Context, _, reply, runID string) error {
		gotReply, gotRunID = reply, runID
		return nil
	}

	outcome, err := orch.submitAwaitReply(context.Background(), "sess-1", awaitReply{
		token:         "approved",
		resumeContent: "用户已批准执行工具 bash",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != awaitReplyResumed {
		t.Fatalf("outcome = %v, want awaitReplyResumed", outcome)
	}
	if gotRunID != "run-persisted" {
		t.Fatalf("resume runID = %q, want persisted %q", gotRunID, "run-persisted")
	}
	if gotReply != "用户已批准执行工具 bash" {
		t.Fatalf("resume content = %q, want semantic resumeContent (not the machine token)", gotReply)
	}
}

func TestSubmitAwaitReply_RestartResumeExplicitRunIDWins(t *testing.T) {
	coord := stubAwaitCoord{
		trySendFn:   func(string, biz.AwaitReplyMsg) bool { return false },
		canResumeFn: func(context.Context, string) (string, bool) { return "run-persisted", true },
	}
	orch := newSubmitAwaitReplyTestOrch(coord)
	var gotRunID string
	orch.resumeAwaitFn = func(_ context.Context, _, _, runID string) error {
		gotRunID = runID
		return nil
	}

	outcome, err := orch.submitAwaitReply(context.Background(), "sess-1", awaitReply{
		runID: "run-explicit",
		token: "hello",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != awaitReplyResumed {
		t.Fatalf("outcome = %v, want awaitReplyResumed", outcome)
	}
	if gotRunID != "run-explicit" {
		t.Fatalf("resume runID = %q, want explicit %q", gotRunID, "run-explicit")
	}
}

func TestSubmitAwaitReply_RestartResumeEmptyContentFallsBackToToken(t *testing.T) {
	coord := stubAwaitCoord{
		trySendFn:   func(string, biz.AwaitReplyMsg) bool { return false },
		canResumeFn: func(context.Context, string) (string, bool) { return "", true },
	}
	orch := newSubmitAwaitReplyTestOrch(coord)
	var gotReply string
	orch.resumeAwaitFn = func(_ context.Context, _, reply, _ string) error {
		gotReply = reply
		return nil
	}

	if _, err := orch.submitAwaitReply(context.Background(), "sess-1", awaitReply{token: "用户自由文本"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotReply != "用户自由文本" {
		t.Fatalf("resume content = %q, want fallback to token", gotReply)
	}
}

func TestSubmitAwaitReply_NoRouteRejected(t *testing.T) {
	coord := stubAwaitCoord{
		trySendFn:   func(string, biz.AwaitReplyMsg) bool { return false },
		canResumeFn: func(context.Context, string) (string, bool) { return "", false },
	}
	orch := newSubmitAwaitReplyTestOrch(coord)
	resumeCalled := false
	orch.resumeAwaitFn = func(context.Context, string, string, string) error {
		resumeCalled = true
		return nil
	}

	outcome, err := orch.submitAwaitReply(context.Background(), "sess-1", awaitReply{token: "approved"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outcome != awaitReplyRejected {
		t.Fatalf("outcome = %v, want awaitReplyRejected", outcome)
	}
	if resumeCalled {
		t.Fatal("resume must not run when canResumeAwait reports no route")
	}
}

func TestSubmitAwaitReply_ResumeInFlightRejectedWithoutError(t *testing.T) {
	coord := stubAwaitCoord{
		trySendFn:   func(string, biz.AwaitReplyMsg) bool { return false },
		canResumeFn: func(context.Context, string) (string, bool) { return "run-1", true },
	}
	orch := newSubmitAwaitReplyTestOrch(coord)
	orch.resumeAwaitFn = func(context.Context, string, string, string) error {
		return errResumeInFlight
	}

	outcome, err := orch.submitAwaitReply(context.Background(), "sess-1", awaitReply{token: "approved"})
	if err != nil {
		t.Fatalf("errResumeInFlight must map to rejected without error, got %v", err)
	}
	if outcome != awaitReplyRejected {
		t.Fatalf("outcome = %v, want awaitReplyRejected", outcome)
	}
}

func TestSubmitAwaitReply_ResumeErrorPropagates(t *testing.T) {
	coord := stubAwaitCoord{
		trySendFn:   func(string, biz.AwaitReplyMsg) bool { return false },
		canResumeFn: func(context.Context, string) (string, bool) { return "run-1", true },
	}
	orch := newSubmitAwaitReplyTestOrch(coord)
	boom := errors.New("boom")
	orch.resumeAwaitFn = func(context.Context, string, string, string) error { return boom }

	_, err := orch.submitAwaitReply(context.Background(), "sess-1", awaitReply{token: "approved"})
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want boom", err)
	}
}

// ---------------------------------------------------------------------------
// ConfirmActivity service-level regression tests
// ---------------------------------------------------------------------------

func newConfirmActivityTestSvc(coord awaitCoordinator, step biz.Step) (*ChatService, *ChatOrchestrator, *stubStepV2Writer) {
	stepWriter := &stubStepV2Writer{}
	orch := newSubmitAwaitReplyTestOrch(coord)
	orch.core.StepReader = stubStepV2ReaderGet{step: step}
	orch.core.StepWriter = stepWriter
	orch.core.TD.Sessions = stubSessionTurnManagerGet{getFn: func(context.Context, string) (biz.Session, error) {
		return biz.Session{ID: "sess-1", UserID: "user-1"}, nil
	}}
	svc := &ChatService{orch: orch, lg: loggateway.NewNoop()}
	return svc, orch, stepWriter
}

func toolBlockedConfirmStep() biz.Step {
	return biz.Step{
		ID:        "step-confirm-1",
		SessionID: "sess-1",
		Kind:      biz.StepKindConfirm,
		Status:    biz.StepStatusToolBlocked,
		ToolName:  "bash",
		Content:   "执行 rm -rf /tmp/data",
	}
}

// P3 core regression: after a process restart the in-memory await channel is
// gone, but the confirm step is still tool_blocked and the session still
// persists awaiting_user. The user's decision must resume the run with a
// semantic natural-language statement (not the bare machine token), so the
// LLM receives the confirmed decision as meaningful context.
func TestConfirmActivity_RestartFallbackResumesWithSemanticContent(t *testing.T) {
	coord := stubAwaitCoord{
		trySendFn:   func(string, biz.AwaitReplyMsg) bool { return false }, // channel gone after restart
		canResumeFn: func(context.Context, string) (string, bool) { return "run-9", true },
	}
	svc, orch, stepWriter := newConfirmActivityTestSvc(coord, toolBlockedConfirmStep())

	var gotReply, gotRunID string
	orch.resumeAwaitFn = func(_ context.Context, _, reply, runID string) error {
		gotReply, gotRunID = reply, runID
		return nil
	}

	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	resp, err := svc.ConfirmActivity(ctx, &chatv1.ConfirmActivityRequest{
		SessionId:  "sess-1",
		ActivityId: "step-confirm-1",
		Approved:   true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.GetAccepted() {
		t.Fatal("restart fallback must accept the confirmation (run resumed), got Accepted=false")
	}
	if gotRunID != "run-9" {
		t.Fatalf("resume runID = %q, want %q", gotRunID, "run-9")
	}
	if gotReply == "" {
		t.Fatal("resume content must not be empty")
	}
	if gotReply == "approved" || gotReply == "rejected" {
		t.Fatalf("resume content must be semantic, not the bare machine token %q", gotReply)
	}
	if !strings.Contains(gotReply, "bash") {
		t.Fatalf("resume content must name the confirmed tool, got %q", gotReply)
	}
	if !strings.Contains(gotReply, "批准") {
		t.Fatalf("resume content must carry the approval decision, got %q", gotReply)
	}

	// Step must transition to its terminal state exactly once.
	if len(stepWriter.updated) != 1 {
		t.Fatalf("expected 1 step update, got %d", len(stepWriter.updated))
	}
	if stepWriter.updated[0].Status != biz.StepStatusCompleted {
		t.Fatalf("step status = %q, want %q", stepWriter.updated[0].Status, biz.StepStatusCompleted)
	}
}

func TestConfirmActivity_RestartFallbackDenyContentIsSemantic(t *testing.T) {
	coord := stubAwaitCoord{
		trySendFn:   func(string, biz.AwaitReplyMsg) bool { return false },
		canResumeFn: func(context.Context, string) (string, bool) { return "run-9", true },
	}
	svc, orch, _ := newConfirmActivityTestSvc(coord, toolBlockedConfirmStep())

	var gotReply string
	orch.resumeAwaitFn = func(_ context.Context, _, reply, _ string) error {
		gotReply = reply
		return nil
	}

	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	resp, err := svc.ConfirmActivity(ctx, &chatv1.ConfirmActivityRequest{
		SessionId:  "sess-1",
		ActivityId: "step-confirm-1",
		Approved:   false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.GetAccepted() {
		t.Fatal("restart fallback must accept the rejection (run resumed), got Accepted=false")
	}
	if !strings.Contains(gotReply, "bash") || !strings.Contains(gotReply, "拒绝") {
		t.Fatalf("deny resume content must name the tool and the rejection, got %q", gotReply)
	}
	if !strings.Contains(gotReply, "禁止重试") {
		t.Fatalf("deny resume content must forbid retrying the rejected tool, got %q", gotReply)
	}
}

// Channel full (not restart): no fallback, reply rejected.
func TestConfirmActivity_ChannelFullRejected(t *testing.T) {
	coord := stubAwaitCoord{
		trySendFn: func(string, biz.AwaitReplyMsg) bool { return false },
		loadFn: func(string) (biz.AwaitChannel, bool) {
			return make(biz.AwaitChannel, 1), true
		},
		canResumeFn: func(context.Context, string) (string, bool) { return "run-9", true },
	}
	svc, orch, _ := newConfirmActivityTestSvc(coord, toolBlockedConfirmStep())
	resumeCalled := false
	orch.resumeAwaitFn = func(context.Context, string, string, string) error {
		resumeCalled = true
		return nil
	}

	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	resp, err := svc.ConfirmActivity(ctx, &chatv1.ConfirmActivityRequest{
		SessionId:  "sess-1",
		ActivityId: "step-confirm-1",
		Approved:   true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetAccepted() {
		t.Fatal("channel-full must not be treated as delivered")
	}
	if resumeCalled {
		t.Fatal("channel-full must not trigger restart resume (double-delivery risk)")
	}
}

// Fast path unchanged: live channel receives the machine token.
func TestConfirmActivity_LiveChannelReceivesToken(t *testing.T) {
	var sent biz.AwaitReplyMsg
	coord := stubAwaitCoord{trySendFn: func(_ string, msg biz.AwaitReplyMsg) bool {
		sent = msg
		return true
	}}
	svc, orch, _ := newConfirmActivityTestSvc(coord, toolBlockedConfirmStep())
	orch.resumeAwaitFn = func(context.Context, string, string, string) error {
		t.Fatal("resume must not run when the live channel accepts")
		return nil
	}

	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	resp, err := svc.ConfirmActivity(ctx, &chatv1.ConfirmActivityRequest{
		SessionId:  "sess-1",
		ActivityId: "step-confirm-1",
		Approved:   true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.GetAccepted() {
		t.Fatal("live channel path must accept")
	}
	if sent.Reply != "approved" {
		t.Fatalf("live channel must receive the machine token, got %q", sent.Reply)
	}
}

// Channel-ingress sessions (e.g. Feishu) are created with an empty UserID.
// The web console operator must be able to confirm tool cards on those
// sessions — same semantics as assertSessionAccess / ConfirmPlan /
// SubmitClarification: empty UserID is allowed, only cross-user is denied.
func TestConfirmActivity_ChannelSessionEmptyUserIDAllowed(t *testing.T) {
	coord := stubAwaitCoord{trySendFn: func(string, biz.AwaitReplyMsg) bool { return true }}
	svc, orch, _ := newConfirmActivityTestSvc(coord, toolBlockedConfirmStep())
	orch.core.TD.Sessions = stubSessionTurnManagerGet{getFn: func(context.Context, string) (biz.Session, error) {
		return biz.Session{ID: "sess-1", UserID: ""}, nil // channel-created session
	}}

	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	resp, err := svc.ConfirmActivity(ctx, &chatv1.ConfirmActivityRequest{
		SessionId:  "sess-1",
		ActivityId: "step-confirm-1",
		Approved:   true,
	})
	if err != nil {
		t.Fatalf("channel session (empty UserID) must be confirmable by any authenticated user: %v", err)
	}
	if !resp.GetAccepted() {
		t.Fatal("live channel path must accept")
	}
}

// Cross-user access must still be denied for owned sessions.
func TestConfirmActivity_CrossUserDenied(t *testing.T) {
	coord := stubAwaitCoord{trySendFn: func(string, biz.AwaitReplyMsg) bool { return true }}
	svc, orch, _ := newConfirmActivityTestSvc(coord, toolBlockedConfirmStep())
	orch.core.TD.Sessions = stubSessionTurnManagerGet{getFn: func(context.Context, string) (biz.Session, error) {
		return biz.Session{ID: "sess-1", UserID: "user-2"}, nil
	}}

	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	_, err := svc.ConfirmActivity(ctx, &chatv1.ConfirmActivityRequest{
		SessionId:  "sess-1",
		ActivityId: "step-confirm-1",
		Approved:   true,
	})
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("cross-user confirm must be Forbidden, got %v", err)
	}
}

func TestConfirmActivity_PlaybookCardNeverFallsToToolAwait(t *testing.T) {
	coord := stubAwaitCoord{
		trySendFn: func(string, biz.AwaitReplyMsg) bool {
			t.Fatal("playbook confirm must not use tool await")
			return false
		},
		canResumeFn: func(context.Context, string) (string, bool) { return "run-9", true },
	}
	actID := biz.PlaybookConfirmActivityID("sess-1", "st-1")
	step := biz.Step{
		ID:        actID,
		SessionID: "sess-1",
		Kind:      biz.StepKindConfirm,
		Status:    biz.StepStatusToolBlocked,
		ToolName:  biz.ToolPlaybookConfirmBefore,
	}
	svc, orch, writer := newConfirmActivityTestSvc(coord, step)
	svc.planExec = NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	orch.resumeAwaitFn = func(context.Context, string, string, string) error {
		t.Fatal("playbook confirm must not resume tool turn")
		return nil
	}
	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	resp, err := svc.ConfirmActivity(ctx, &chatv1.ConfirmActivityRequest{
		SessionId:  "sess-1",
		ActivityId: actID,
		Approved:   true,
	})
	if err != nil || resp == nil || !resp.GetAccepted() {
		t.Fatalf("err=%v resp=%v", err, resp)
	}
	if len(writer.created) == 0 || writer.created[len(writer.created)-1].Status != biz.StepStatusCompleted {
		t.Fatalf("card persist=%v", writer.created)
	}
	approved, ok := svc.planExec.lookupPlaybookConfirmDecision("sess-1", "st-1")
	if !ok || !approved {
		t.Fatalf("noted decision missing ok=%v approved=%v", ok, approved)
	}
}

func playbookConfirmBlockedStep() biz.Step {
	actID := biz.PlaybookConfirmActivityID("sess-1", "st-1")
	return biz.Step{
		ID:        actID,
		SessionID: "sess-1",
		Kind:      biz.StepKindConfirm,
		Status:    biz.StepStatusToolBlocked,
		ToolName:  biz.ToolPlaybookConfirmBefore,
	}
}

func TestConfirmActivity_PlaybookReapproveIsIdempotent(t *testing.T) {
	step := playbookConfirmBlockedStep()
	step.Status = biz.StepStatusCompleted
	svc, _, writer := newConfirmActivityTestSvc(stubAwaitCoord{}, step)
	svc.planExec = NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	resp, err := svc.ConfirmActivity(ctx, &chatv1.ConfirmActivityRequest{
		SessionId:  "sess-1",
		ActivityId: step.ID,
		Approved:   true,
	})
	if err != nil || resp == nil || !resp.GetAccepted() {
		t.Fatalf("re-approve must succeed err=%v resp=%v", err, resp)
	}
	if len(writer.created) != 0 {
		t.Fatalf("idempotent approve must not rewrite card, got %d writes", len(writer.created))
	}
	approved, ok := svc.planExec.lookupPlaybookConfirmDecision("sess-1", "st-1")
	if !ok || !approved {
		t.Fatalf("idempotent approve must still note decision ok=%v approved=%v", ok, approved)
	}
}

func TestConfirmActivity_PlaybookCompletedCannotFlipToDeny(t *testing.T) {
	step := playbookConfirmBlockedStep()
	step.Status = biz.StepStatusCompleted
	svc, _, writer := newConfirmActivityTestSvc(stubAwaitCoord{}, step)
	svc.planExec = NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	_, err := svc.ConfirmActivity(ctx, &chatv1.ConfirmActivityRequest{
		SessionId:  "sess-1",
		ActivityId: step.ID,
		Approved:   false,
	})
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("completed→deny must be BadRequest, got %v", err)
	}
	if len(writer.created) != 0 {
		t.Fatal("illegal flip must not persist")
	}
	if _, ok := svc.planExec.lookupPlaybookConfirmDecision("sess-1", "st-1"); ok {
		t.Fatal("illegal flip must not note a decision")
	}
}

func TestConfirmActivity_PlaybookPersistFailDoesNotAccept(t *testing.T) {
	step := playbookConfirmBlockedStep()
	svc, _, writer := newConfirmActivityTestSvc(stubAwaitCoord{}, step)
	writer.err = errors.New("db down")
	svc.planExec = NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	resp, err := svc.ConfirmActivity(ctx, &chatv1.ConfirmActivityRequest{
		SessionId:  "sess-1",
		ActivityId: step.ID,
		Approved:   true,
	})
	if err == nil {
		t.Fatal("persist failure must not return success")
	}
	if resp != nil && resp.GetAccepted() {
		t.Fatal("persist failure must not set Accepted")
	}
	if _, ok := svc.planExec.lookupPlaybookConfirmDecision("sess-1", "st-1"); ok {
		t.Fatal("persist failure must not Note/Resolve")
	}
}

func TestConfirmToolGateForCard_PlaybookCompletedCannotFlip(t *testing.T) {
	step := playbookConfirmBlockedStep()
	step.Status = biz.StepStatusCompleted
	svc, _, _ := newConfirmActivityTestSvc(stubAwaitCoord{}, step)
	ok, msg := svc.ConfirmToolGateForCard(context.Background(), "sess-1", step.ID, serviceawaitreply.ReplyDeny)
	if ok {
		t.Fatalf("channel path must reject flip, msg=%s", msg)
	}
}
