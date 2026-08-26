package service

import (
	"context"
	"strings"
	"testing"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	araneasession "aranea-agents/internal/session"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"

	trpcinmemory "trpc.group/trpc-go/trpc-agent-go/session/inmemory"
)

// ---------------------------------------------------------------------------
// Stubs
// ---------------------------------------------------------------------------

// stubTaskPlanner embeds the port interface and overrides only GetPlan /
// ConfirmPlan, which ConfirmPlan exercises.
type stubTaskPlanner struct {
	biz.TaskPlannerPort
	plan           *biz.TaskPlan
	confirmed      *biz.TaskPlan
	confirmCalled  bool
	gotAdjustments biz.PlanAdjustments
}

func (s *stubTaskPlanner) GetPlan(context.Context, string) (*biz.TaskPlan, error) {
	return s.plan, nil
}

func (s *stubTaskPlanner) ListPlans(context.Context, string) ([]*biz.TaskPlan, error) {
	if s.plan == nil {
		return nil, nil
	}
	return []*biz.TaskPlan{s.plan}, nil
}

func (s *stubTaskPlanner) ConfirmPlan(_ context.Context, _ string, adj biz.PlanAdjustments) (*biz.TaskPlan, error) {
	s.confirmCalled = true
	s.gotAdjustments = adj
	return s.confirmed, nil
}

// stubPlanSessionManager embeds the composite interface; Get serves the
// ownership check and AppendChatMessage captures the persisted system row.
type stubPlanSessionManager struct {
	biz.SessionTurnManager
	getFn    func(ctx context.Context, id string) (biz.Session, error)
	appended []biz.ChatMessage
}

func (s *stubPlanSessionManager) Get(ctx context.Context, id string) (biz.Session, error) {
	return s.getFn(ctx, id)
}

func (s *stubPlanSessionManager) AppendChatMessage(_ context.Context, _ string, msg biz.ChatMessage, _ bool) error {
	s.appended = append(s.appended, msg)
	return nil
}

func draftPlanFixture() *biz.TaskPlan {
	return &biz.TaskPlan{
		ID:              "plan-1",
		SpiritSessionID: "sess-1",
		Status:          biz.TaskPlanStatusDraft,
		Strategy:        biz.StrategyDAG,
		SubTasks: []biz.SubTask{
			{ID: "st-1", Name: "调研"},
			{ID: "st-2", Name: "实现"},
		},
	}
}

func newPlanConfirmTestSvc(planner biz.TaskPlannerPort, sessions biz.SessionTurnManager) (*ChatService, *ChatOrchestrator, *araneasession.Runtime) {
	sessRT := araneasession.NewRuntime(trpcinmemory.NewSessionService(), loggateway.NewNoop())
	orch := &ChatOrchestrator{
		runs: rt.NewRunRegistry(),
		runMgr: &chatRunManagerImpl{
			runStatusTracker:    noopRunStatusTracker{},
			pendingQueueManager: noopPendingQueueManager{},
			awaitCoordinator:    noopAwaitCoordinator{},
			sessionRunLifecycle: noopSessionRunLifecycle{},
		},
		infraDeps: ChatInfraDeps{LG: loggateway.NewNoop()},
	}
	orch.core.TD.Sessions = sessions
	orch.core.TD.SessionRT = sessRT
	orch.teamExecDeps.Team.TaskPlanner = planner
	return &ChatService{orch: orch, lg: loggateway.NewNoop()}, orch, sessRT
}

// sessionEventContent returns the first user-authored session event content
// containing want, or "" when none matches.
func sessionEventContent(t *testing.T, sessRT *araneasession.Runtime, userID, sessionID, want string) string {
	t.Helper()
	sess, err := sessRT.Get(context.Background(), userID, sessionID)
	if err != nil {
		t.Fatalf("session runtime Get: %v", err)
	}
	for i := range sess.Events {
		e := &sess.Events[i]
		if e.Author != "user" || e.Response == nil {
			continue
		}
		for _, ch := range e.Response.Choices {
			if strings.Contains(ch.Message.Content, want) {
				return ch.Message.Content
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// buildPlanDecisionContext unit tests
// ---------------------------------------------------------------------------

func TestBuildPlanDecisionContext_Approve(t *testing.T) {
	plan := draftPlanFixture()
	text := buildPlanDecisionContext(plan, true, "优先保证数据正确性", "")
	for _, want := range []string{"计划确认", "已批准", "dag", "2", "优先保证数据正确性"} {
		if !strings.Contains(text, want) {
			t.Fatalf("approve context missing %q, got %q", want, text)
		}
	}
	if strings.Contains(text, "禁止") {
		t.Fatalf("approve context must not carry rejection guidance, got %q", text)
	}
}

func TestBuildPlanDecisionContext_ApproveStrategyOverride(t *testing.T) {
	plan := draftPlanFixture()
	plan.Strategy = biz.StrategyParallel // confirmed plan reflects the override
	text := buildPlanDecisionContext(plan, true, "", "parallel")
	if !strings.Contains(text, "parallel") {
		t.Fatalf("override strategy must appear in context, got %q", text)
	}
}

func TestBuildPlanDecisionContext_Reject(t *testing.T) {
	plan := draftPlanFixture()
	text := buildPlanDecisionContext(plan, false, "拆得太细了", "")
	for _, want := range []string{"计划确认", "拒绝", "拆得太细了", "禁止"} {
		if !strings.Contains(text, want) {
			t.Fatalf("reject context missing %q, got %q", want, text)
		}
	}
}

func TestBuildPlanDecisionContext_RejectWithoutReason(t *testing.T) {
	plan := draftPlanFixture()
	text := buildPlanDecisionContext(plan, false, "", "")
	if !strings.Contains(text, "拒绝") {
		t.Fatalf("reject context must state the rejection, got %q", text)
	}
	// Guidance must still be present so the LLM does not blindly re-plan.
	if !strings.Contains(text, "禁止") {
		t.Fatalf("reject context must forbid blind re-planning even without reason, got %q", text)
	}
}

// ---------------------------------------------------------------------------
// ConfirmPlan service-level injection tests
// ---------------------------------------------------------------------------

// P1 core regression: when the user confirms a plan, the decision (+ reason)
// must enter the session context so subsequent spirit turns see it — both as
// a live trpc session event (LLM history) and as a persisted system message
// (audit trail that survives compression).
func TestConfirmPlan_ApproveInjectsDecisionContext(t *testing.T) {
	planner := &stubTaskPlanner{plan: draftPlanFixture(), confirmed: func() *biz.TaskPlan {
		p := draftPlanFixture()
		p.Status = biz.TaskPlanStatusConfirmed
		return p
	}()}
	sessions := &stubPlanSessionManager{getFn: func(context.Context, string) (biz.Session, error) {
		return biz.Session{ID: "sess-1", UserID: "user-1"}, nil
	}}
	svc, _, sessRT := newPlanConfirmTestSvc(planner, sessions)

	reason := "优先保证数据正确性"
	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	resp, err := svc.ConfirmPlan(ctx, &chatv1.ConfirmPlanRequest{
		PlanId:    "plan-1",
		SessionId: "sess-1",
		Approved:  boolPtr(true),
		Reason:    &reason,
	})
	if err != nil {
		t.Fatalf("ConfirmPlan: %v", err)
	}
	if resp.GetStatus() != "confirmed" {
		t.Fatalf("status = %q, want confirmed", resp.GetStatus())
	}
	if !planner.confirmCalled {
		t.Fatal("planner.ConfirmPlan must be called on approval")
	}
	if planner.gotAdjustments.Reason != reason {
		t.Fatalf("adjustments.Reason = %q, want %q", planner.gotAdjustments.Reason, reason)
	}

	// 1) Live session event → LLM history of subsequent turns.
	eventText := sessionEventContent(t, sessRT, "user-1", "sess-1", "计划确认")
	if eventText == "" {
		t.Fatal("no plan-decision event injected into the trpc session")
	}
	for _, want := range []string{"已批准", reason} {
		if !strings.Contains(eventText, want) {
			t.Fatalf("injected event missing %q, got %q", want, eventText)
		}
	}

	// 2) Persisted system message → audit + compression survival.
	if len(sessions.appended) != 1 {
		t.Fatalf("expected 1 persisted system message, got %d", len(sessions.appended))
	}
	msg := sessions.appended[0]
	if msg.Role != "system" {
		t.Fatalf("persisted message role = %q, want system", msg.Role)
	}
	if !strings.Contains(msg.ContentMarkdown, "计划确认") || !strings.Contains(msg.ContentMarkdown, reason) {
		t.Fatalf("persisted message missing decision/reason, got %q", msg.ContentMarkdown)
	}
}

// Rejection must also be injected: the next user turn must see that the plan
// was rejected and why, so the spirit does not blindly regenerate the same plan.
func TestConfirmPlan_RejectInjectsDecisionContext(t *testing.T) {
	planner := &stubTaskPlanner{plan: draftPlanFixture()}
	sessions := &stubPlanSessionManager{getFn: func(context.Context, string) (biz.Session, error) {
		return biz.Session{ID: "sess-1", UserID: "user-1"}, nil
	}}
	svc, _, sessRT := newPlanConfirmTestSvc(planner, sessions)

	reason := "拆得太细了"
	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	resp, err := svc.ConfirmPlan(ctx, &chatv1.ConfirmPlanRequest{
		PlanId:    "plan-1",
		SessionId: "sess-1",
		Approved:  boolPtr(false),
		Reason:    &reason,
	})
	if err != nil {
		t.Fatalf("ConfirmPlan: %v", err)
	}
	if resp.GetStatus() != "rejected" {
		t.Fatalf("status = %q, want rejected", resp.GetStatus())
	}
	if planner.confirmCalled {
		t.Fatal("planner.ConfirmPlan must NOT be called on rejection")
	}

	eventText := sessionEventContent(t, sessRT, "user-1", "sess-1", "计划确认")
	if eventText == "" {
		t.Fatal("no plan-decision event injected into the trpc session on rejection")
	}
	for _, want := range []string{"拒绝", reason, "禁止"} {
		if !strings.Contains(eventText, want) {
			t.Fatalf("reject event missing %q, got %q", want, eventText)
		}
	}

	if len(sessions.appended) != 1 {
		t.Fatalf("expected 1 persisted system message, got %d", len(sessions.appended))
	}
	if got := sessions.appended[0].ContentMarkdown; !strings.Contains(got, "拒绝") || !strings.Contains(got, reason) {
		t.Fatalf("persisted message missing rejection/reason, got %q", got)
	}
}

// Injection failure must never fail plan confirmation itself: with no session
// runtime wired, approval still succeeds.
func TestConfirmPlan_InjectionNonFatal(t *testing.T) {
	planner := &stubTaskPlanner{plan: draftPlanFixture(), confirmed: func() *biz.TaskPlan {
		p := draftPlanFixture()
		p.Status = biz.TaskPlanStatusConfirmed
		return p
	}()}
	sessions := &stubPlanSessionManager{getFn: func(context.Context, string) (biz.Session, error) {
		return biz.Session{ID: "sess-1", UserID: "user-1"}, nil
	}}
	svc, orch, _ := newPlanConfirmTestSvc(planner, sessions)
	orch.core.TD.SessionRT = nil // simulate unavailable runtime

	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	resp, err := svc.ConfirmPlan(ctx, &chatv1.ConfirmPlanRequest{
		PlanId:    "plan-1",
		SessionId: "sess-1",
		Approved:  boolPtr(true),
	})
	if err != nil {
		t.Fatalf("injection failure must not fail ConfirmPlan: %v", err)
	}
	if resp.GetStatus() != "confirmed" {
		t.Fatalf("status = %q, want confirmed", resp.GetStatus())
	}
}

// ---------------------------------------------------------------------------
// Ownership semantics tests (TS9-BUG-2)
// ---------------------------------------------------------------------------
//
// Sessions created via API / channel ingress may carry an empty UserID. The
// ownership check must match chat_clarify.go semantics: empty session owner
// is allowed (only cross-user access is rejected). Strict equality would 403
// such sessions for ListPlans / GetPlan / ConfirmPlan.

func TestConfirmPlan_EmptyOwnerSessionAllowed(t *testing.T) {
	planner := &stubTaskPlanner{plan: draftPlanFixture(), confirmed: func() *biz.TaskPlan {
		p := draftPlanFixture()
		p.Status = biz.TaskPlanStatusConfirmed
		return p
	}()}
	sessions := &stubPlanSessionManager{getFn: func(context.Context, string) (biz.Session, error) {
		return biz.Session{ID: "sess-1", UserID: ""}, nil // API-created session
	}}
	svc, _, _ := newPlanConfirmTestSvc(planner, sessions)

	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	resp, err := svc.ConfirmPlan(ctx, &chatv1.ConfirmPlanRequest{
		PlanId:    "plan-1",
		SessionId: "sess-1",
		Approved:  boolPtr(true),
	})
	if err != nil {
		t.Fatalf("empty-owner session must be allowed: %v", err)
	}
	if resp.GetStatus() != "confirmed" {
		t.Fatalf("status = %q, want confirmed", resp.GetStatus())
	}
}

func TestConfirmPlan_CrossUserDenied(t *testing.T) {
	planner := &stubTaskPlanner{plan: draftPlanFixture()}
	sessions := &stubPlanSessionManager{getFn: func(context.Context, string) (biz.Session, error) {
		return biz.Session{ID: "sess-1", UserID: "user-2"}, nil
	}}
	svc, _, _ := newPlanConfirmTestSvc(planner, sessions)

	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	_, err := svc.ConfirmPlan(ctx, &chatv1.ConfirmPlanRequest{
		PlanId:    "plan-1",
		SessionId: "sess-1",
		Approved:  boolPtr(true),
	})
	if err == nil {
		t.Fatal("cross-user access must be denied")
	}
	if planner.confirmCalled {
		t.Fatal("planner.ConfirmPlan must NOT be called for cross-user access")
	}
}

func TestListPlans_EmptyOwnerSessionAllowed(t *testing.T) {
	planner := &stubTaskPlanner{}
	sessions := &stubPlanSessionManager{getFn: func(context.Context, string) (biz.Session, error) {
		return biz.Session{ID: "sess-1", UserID: ""}, nil
	}}
	svc, _, _ := newPlanConfirmTestSvc(planner, sessions)

	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	if _, err := svc.ListPlans(ctx, &chatv1.ListPlansRequest{SessionId: "sess-1"}); err != nil {
		t.Fatalf("empty-owner session must be allowed: %v", err)
	}
}

func TestGetPlan_EmptyOwnerSessionAllowed(t *testing.T) {
	planner := &stubTaskPlanner{plan: draftPlanFixture()}
	sessions := &stubPlanSessionManager{getFn: func(context.Context, string) (biz.Session, error) {
		return biz.Session{ID: "sess-1", UserID: ""}, nil
	}}
	svc, _, _ := newPlanConfirmTestSvc(planner, sessions)

	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	resp, err := svc.GetPlan(ctx, &chatv1.GetPlanRequest{PlanId: "plan-1", SessionId: "sess-1"})
	if err != nil {
		t.Fatalf("empty-owner session must be allowed: %v", err)
	}
	if resp.GetPlan() == nil || resp.GetPlan().GetPlanId() != "plan-1" {
		t.Fatalf("unexpected plan: %+v", resp.GetPlan())
	}
}

func TestGetPlan_CrossUserDenied(t *testing.T) {
	planner := &stubTaskPlanner{plan: draftPlanFixture()}
	sessions := &stubPlanSessionManager{getFn: func(context.Context, string) (biz.Session, error) {
		return biz.Session{ID: "sess-1", UserID: "user-2"}, nil
	}}
	svc, _, _ := newPlanConfirmTestSvc(planner, sessions)

	ctx := ctxuser.WithUserID(context.Background(), "user-1")
	if _, err := svc.GetPlan(ctx, &chatv1.GetPlanRequest{PlanId: "plan-1", SessionId: "sess-1"}); err == nil {
		t.Fatal("cross-user access must be denied")
	}
}
