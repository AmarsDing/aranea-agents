package service

import (
	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/agentbridge"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"
)

// BUG-02: the confirm decision is addressed to THIS step's ToolCallID scope,
// so parallel confirmations on the same session each reach their own gate.
func TestConfirmActivity_RoutesDecisionToStepToolScope(t *testing.T) {
	var sent biz.AwaitReplyMsg
	coord := stubAwaitCoord{trySendFn: func(_ string, msg biz.AwaitReplyMsg) bool {
		sent = msg
		return true
	}}
	step := toolBlockedConfirmStep()
	step.ToolCallID = "toolu_01ABC"
	svc, _, stepWriter := newConfirmActivityTestSvc(coord, step)

	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	resp, err := svc.ConfirmActivity(ctx, &chatv1.ConfirmActivityRequest{
		SessionId:  "sess-1",
		ActivityId: step.ID,
		Approved:   boolPtr(true),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.GetAccepted() {
		t.Fatal("scoped delivery must accept")
	}
	if sent.ToolCallID != "toolu_01ABC" {
		t.Fatalf("reply ToolCallID = %q, want the step's scope %q", sent.ToolCallID, "toolu_01ABC")
	}
	if len(stepWriter.updated) != 1 || stepWriter.updated[0].Status != biz.StepStatusCompleted {
		t.Fatalf("delivered confirm must persist completed, got %+v", stepWriter.updated)
	}
}

// hookStepWriter wraps a StepV2Writer to observe/serialize UpdateStep calls
// (delivery-vs-persist ordering and parallel-confirm tests).
type hookStepWriter struct {
	biz.StepV2Writer
	mu       sync.Mutex
	onUpdate func()
}

func (w *hookStepWriter) UpdateStep(ctx context.Context, step biz.Step) (biz.Step, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.onUpdate != nil {
		w.onUpdate()
	}
	return w.StepV2Writer.UpdateStep(ctx, step)
}

// BUG-02: delivery happens BEFORE the step is persisted — only a delivered
// decision may transition the step to its terminal state.
func TestConfirmActivity_DeliversBeforePersistingStep(t *testing.T) {
	var order []string
	coord := stubAwaitCoord{trySendFn: func(string, biz.AwaitReplyMsg) bool {
		order = append(order, "deliver")
		return true
	}}
	svc, orch, stepWriter := newConfirmActivityTestSvc(coord, toolBlockedConfirmStep())
	orch.core.StepWriter = &hookStepWriter{
		StepV2Writer: stepWriter,
		onUpdate:     func() { order = append(order, "persist") },
	}

	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	resp, err := svc.ConfirmActivity(ctx, &chatv1.ConfirmActivityRequest{
		SessionId:  "sess-1",
		ActivityId: "step-confirm-1",
		Approved:   boolPtr(true),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.GetAccepted() {
		t.Fatal("live channel path must accept")
	}
	if len(order) != 2 || order[0] != "deliver" || order[1] != "persist" {
		t.Fatalf("delivery must precede persist, got order %v", order)
	}
}

// stubStepV2ReaderMap serves distinct steps by ID (parallel-confirm tests).
type stubStepV2ReaderMap struct {
	biz.StepV2Reader
	steps map[string]biz.Step
}

func (s stubStepV2ReaderMap) GetStep(_ context.Context, id string) (biz.Step, error) {
	step, ok := s.steps[id]
	if !ok {
		return biz.Step{}, apierror.NotFound(apierror.DomainChat, "step not found")
	}
	return step, nil
}

// BUG-02 regression for the TASK-B race (taskb2-run.log): three parallel tool
// confirmations on one session — originally one accepted, one desynced
// (accepted:false + completed), one 400. With tool-scoped delivery every
// confirm reaches its own gate: all accepted, all persisted exactly once.
func TestConfirmActivity_ParallelToolConfirmsAllAccepted(t *testing.T) {
	ids := []string{"step-a", "step-b", "step-c"}
	steps := make(map[string]biz.Step, len(ids))
	for _, id := range ids {
		step := toolBlockedConfirmStep()
		step.ID = id
		step.ToolCallID = "tc-" + id
		steps[id] = step
	}
	coord := stubAwaitCoord{trySendFn: func(string, biz.AwaitReplyMsg) bool { return true }}
	svc, orch, stepWriter := newConfirmActivityTestSvc(coord, toolBlockedConfirmStep())
	orch.core.StepReader = stubStepV2ReaderMap{steps: steps}
	orch.core.StepWriter = &hookStepWriter{StepV2Writer: stepWriter}

	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	var wg sync.WaitGroup
	errs := make([]error, len(ids))
	accepted := make([]bool, len(ids))
	for i, id := range ids {
		wg.Add(1)
		go func(i int, activityID string) {
			defer wg.Done()
			resp, err := svc.ConfirmActivity(ctx, &chatv1.ConfirmActivityRequest{
				SessionId:  "sess-1",
				ActivityId: activityID,
				Approved:   boolPtr(true),
			})
			errs[i] = err
			if err == nil {
				accepted[i] = resp.GetAccepted()
			}
		}(i, id)
	}
	wg.Wait()

	for i, id := range ids {
		if errs[i] != nil {
			t.Fatalf("confirm %s returned error: %v", id, errs[i])
		}
		if !accepted[i] {
			t.Fatalf("confirm %s was not accepted", id)
		}
	}
	if len(stepWriter.updated) != len(ids) {
		t.Fatalf("expected %d persisted steps, got %d: %+v", len(ids), len(stepWriter.updated), stepWriter.updated)
	}
	persisted := map[string]biz.StepStatus{}
	for _, st := range stepWriter.updated {
		persisted[st.ID] = st.Status
	}
	for _, id := range ids {
		if persisted[id] != biz.StepStatusCompleted {
			t.Fatalf("step %s persisted status = %q, want completed", id, persisted[id])
		}
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
		Approved:   boolPtr(true),
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
		return biz.Session{ID: "sess-1", UserID: ""}, nil
	}}

	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	resp, err := svc.ConfirmActivity(ctx, &chatv1.ConfirmActivityRequest{
		SessionId:  "sess-1",
		ActivityId: "step-confirm-1",
		Approved:   boolPtr(true),
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
		Approved:   boolPtr(true),
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
		Approved:   boolPtr(true),
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
		Approved:   boolPtr(true),
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
		Approved:   boolPtr(false),
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
		Approved:   boolPtr(true),
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

// D6-f2 C1+C2 钉：chat_confirm 接线缝——CAS 输家（别副本已先到 approve）的
// 本地 deny 必须以收敛值贯通三个面：waiter chan（Resolve 驱动值）、确认卡
// 持久化、API 响应。回退 signalPlaybookConfirm 接线（Resolve 用原始值）或
// persist/响应改用原始值，本钉即红——此前四颗 Note 直调钉无一经 ChatService
// 驱动该缝，接线回退全绿（F1 窗重开）。
func TestConfirmActivity_PlaybookConfirmDrivesWaiterWithConvergedValue(t *testing.T) {
	coord := stubAwaitCoord{
		trySendFn:   func(string, biz.AwaitReplyMsg) bool { return false },
		canResumeFn: func(context.Context, string) (string, bool) { return "run-9", true },
	}
	step := playbookConfirmBlockedStep()
	svc, _, writer := newConfirmActivityTestSvc(coord, step)
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	port := NewInMemExecutorWaiterDecisions()
	pe.SetWaiterDecisionPort(port)
	svc.planExec = pe

	// 别副本先到 approve（双键同占，CAS 先到者赢）；本进程注册 waiter。
	ctx := context.Background()
	for _, id := range playbookConfirmLookupIDs("sess-1", "st-1") {
		if _, err := port.TryRecordDecision(ctx, biz.ExecutorWaiterDecision{
			ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{
				Kind: biz.WaiterDecisionKindPlaybookConfirm, SessionID: "sess-1", ScopeID: id,
			},
			Approved: true, Actor: "replica-b", DecidedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("seed first-writer decision for %q: %v", id, err)
		}
	}
	wait := pe.registerPlaybookConfirm("sess-1", "st-1")

	// 本副本操作员迟到 deny——CAS 输家，三个面都必须收敛到先到 approve。
	resp, err := svc.ConfirmActivity(ctxuser.WithUserID(ctx, "user-1"), &chatv1.ConfirmActivityRequest{
		SessionId:  "sess-1",
		ActivityId: step.ID,
		Approved:   boolPtr(false),
	})
	if err != nil || resp == nil || !resp.GetAccepted() {
		t.Fatalf("err=%v resp=%v", err, resp)
	}
	if resp.GetStatus() != "confirmed" {
		t.Fatalf("API response must report the converged decision (confirmed), got %q", resp.GetStatus())
	}
	if len(writer.created) == 0 || writer.created[len(writer.created)-1].Status != biz.StepStatusCompleted {
		t.Fatalf("persisted card must carry the converged decision (completed), got %+v", writer.created)
	}
	select {
	case decided := <-wait.ch:
		if !decided {
			t.Fatal("waiter chan must receive the converged approve, not the losing deny")
		}
	case <-time.After(time.Second):
		t.Fatal("waiter not resolved")
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

func TestConfirmActivity_ExternalCodingSkipsChatAwait(t *testing.T) {
	step := toolBlockedConfirmStep()
	step.ToolName = agentbridge.ToolExternalCoding
	step.TaskID = "task-bridge-1"
	// S6（M4 P0-3）绑定纪律：桥接卡片必须带 bridge-confirm- 前缀；
	// 真实卡经 emitApprovalRequest 生成即此前缀，测试同步对齐。
	step.ID = "bridge-confirm-test-1"
	step.TurnID = step.ID
	step.ToolArgs = json.RawMessage(`{"source":"external_coding","task_id":"task-bridge-1"}`)
	resumed := false
	coord := stubAwaitCoord{
		trySendFn:   func(string, biz.AwaitReplyMsg) bool { return false },
		canResumeFn: func(context.Context, string) (string, bool) { return "", false },
	}
	svc, orch, writer := newConfirmActivityTestSvc(coord, step)
	orch.resumeAwaitFn = func(context.Context, string, string, string) error {
		resumed = true
		return nil
	}
	bridge := NewAgentBridgeService(AgentBridgeServiceDeps{Logger: loggateway.NewNoop()})
	pending := &pendingApproval{
		taskID: "task-bridge-1",
		options: []agentbridge.PermissionOption{
			{OptionID: "allow", Kind: "allow_once"},
			{OptionID: "deny", Kind: "reject_once"},
		},
		done: make(chan approvalDecision, 1),
	}
	bridge.storePending(pending)
	svc.BindAgentBridge(bridge)

	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	resp, err := svc.ConfirmActivity(ctx, &chatv1.ConfirmActivityRequest{
		SessionId:  "sess-1",
		ActivityId: step.ID,
		Approved:   boolPtr(true),
	})
	if err != nil {
		t.Fatalf("ConfirmActivity: %v", err)
	}
	if resp == nil || !resp.GetAccepted() {
		t.Fatal("external_coding confirm must accept without a chat await channel")
	}
	if resumed {
		t.Fatal("must not resume the chat turn for a coding-bridge card")
	}
	select {
	case dec := <-pending.done:
		if dec.optionID != "allow" {
			t.Fatalf("option = %q, want allow", dec.optionID)
		}
	default:
		t.Fatal("bridge permission was not unblocked")
	}
	if len(writer.updated) != 1 || writer.updated[0].Status != biz.StepStatusCompleted {
		t.Fatalf("step updates = %+v", writer.updated)
	}
}
