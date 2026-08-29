package service

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

// --- postProcessTurn 竞态测试 spies ---

type recordingSessionTransitor struct {
	noopSessionStateTransitor
	calls []sessstatus.SessionStatus
}

func (r *recordingSessionTransitor) TransitionStatus(_ context.Context, _ string, s sessstatus.SessionStatus, _ sessstatus.SessionStatusReason) {
	r.calls = append(r.calls, s)
}

type recordingTurnEventPublisher struct {
	noopTurnEventPublisher
	bumps []string
}

func (r *recordingTurnEventPublisher) BumpSessionRevision(_ context.Context, sessionID string) {
	r.bumps = append(r.bumps, sessionID)
}

type recordingTurnRecorder struct {
	noopTurnRecorder
	sessionTurns int
}

func (r *recordingTurnRecorder) RecordSessionTurn(context.Context, SessionTurnRecordParams) {
	r.sessionTurns++
}

type postProcessFixture struct {
	orch      *ChatOrchestrator
	rs        *recordingRunStatusTracker
	transitor *recordingSessionTransitor
	publisher *recordingTurnEventPublisher
	recorder  *recordingTurnRecorder
}

func newPostProcessFixture(reg *rt.RunRegistry) *postProcessFixture {
	rs := &recordingRunStatusTracker{}
	transitor := &recordingSessionTransitor{}
	publisher := &recordingTurnEventPublisher{}
	recorder := &recordingTurnRecorder{}
	orch := &ChatOrchestrator{
		core: chatTurnCoreDeps{TD: rt.TurnDeps{Pipeline: rt.EventPipeline{}}},
		runs: reg,
		turnLC: &chatTurnLifecycleImpl{
			sessionStateTransitor: transitor,
			turnRecorder:          recorder,
			turnEventPublisher:    publisher,
		},
		runMgr: &chatRunManagerImpl{
			runStatusTracker:    rs,
			pendingQueueManager: noopPendingQueueManager{},
			awaitCoordinator:    noopAwaitCoordinator{},
			sessionRunLifecycle: noopSessionRunLifecycle{},
		},
	}
	return &postProcessFixture{orch: orch, rs: rs, transitor: transitor, publisher: publisher, recorder: recorder}
}

func (f *postProcessFixture) run() {
	f.runWithClarifySuspended(false)
}

func (f *postProcessFixture) runWithClarifySuspended(clarifySuspended bool) {
	emitter := event.NewTraceEmitter(nil, event.TraceContext{SessionID: "sess-1", RunID: "run-1"}, loggateway.NewNoop())
	f.orch.postProcessTurn(
		context.Background(),
		biz.Session{ID: "sess-1"},
		biz.Agent{ID: "agent-1"},
		biz.TurnInput{SessionID: "sess-1", Content: "hi"},
		turnAdmissionResult{runID: "run-1", provider: "p", model: "m"},
		turnExecuteResult{userMsg: biz.ChatMessage{ID: "u-1"}},
		turnPersistResult{assistantMsg: biz.ChatMessage{ID: "a-1", ContentMarkdown: "reply"}, promptTok: 3, completionTok: 5},
		emitter,
		time.Now(),
		"ok",
		clarifySuspended,
	)
}

// TestPostProcessTurn_SkipsCompletedWhenRunCancelled F4：取消竞态——run 已被
// cancelActiveRun 置 cancelled 后，EXECUTE/PERSIST 成功路径不得再发布
// completed 状态、翻转 Session 状态、bump revision 或打"执行完成"流程日志；
// 但用量记账必须保留（助手消息已落库，token 真实消耗）。
func TestPostProcessTurn_SkipsCompletedWhenRunCancelled(t *testing.T) {
	reg := rt.NewRunRegistry()
	reg.SetStatus("sess-1", "run-1", biz.SessionRunPhaseCancelled, "")
	f := newPostProcessFixture(reg)

	f.run()

	for _, c := range f.rs.setCalls {
		if c.status == "completed" {
			t.Fatalf("cancelled run must not be overwritten to completed: %+v", f.rs.setCalls)
		}
	}
	for _, s := range f.transitor.calls {
		if s == sessstatus.SessionStatusCompleted {
			t.Fatalf("cancelled run must not transition session to completed: %+v", f.transitor.calls)
		}
	}
	if len(f.publisher.bumps) != 0 {
		t.Fatalf("cancelled run must not bump session revision: %+v", f.publisher.bumps)
	}
	if f.recorder.sessionTurns != 1 {
		t.Fatalf("usage accounting must be preserved, got %d RecordSessionTurn calls", f.recorder.sessionTurns)
	}
}

// TestPostProcessTurn_CompletesNormallyWhenNotCancelled 对照组：未取消时
// completed 状态、Session 翻转、revision bump 全部照常，防止过度跳过。
func TestPostProcessTurn_CompletesNormallyWhenNotCancelled(t *testing.T) {
	f := newPostProcessFixture(rt.NewRunRegistry())

	f.run()

	var completedSeen bool
	for _, c := range f.rs.setCalls {
		if c.status == "completed" {
			completedSeen = true
		}
	}
	if !completedSeen {
		t.Fatalf("expected SetRunStatus(completed), got %+v", f.rs.setCalls)
	}
	var sessionCompleted bool
	for _, s := range f.transitor.calls {
		if s == sessstatus.SessionStatusCompleted {
			sessionCompleted = true
		}
	}
	if !sessionCompleted {
		t.Fatalf("expected session transition to completed, got %+v", f.transitor.calls)
	}
	if len(f.publisher.bumps) != 1 {
		t.Fatalf("expected 1 revision bump, got %+v", f.publisher.bumps)
	}
	if f.recorder.sessionTurns != 1 {
		t.Fatalf("expected 1 RecordSessionTurn call, got %d", f.recorder.sessionTurns)
	}
}

// TestPostProcessTurn_SkipsSessionCompletedWhenClarifySuspended P2/G2：后置澄清
// 已把 session 翻转为 awaiting_confirmation 时，postProcessTurn 不得再落
// session completed 或 run completed；run 记 awaiting_user，revision bump 与
// 用量记账照常。
func TestPostProcessTurn_SkipsSessionCompletedWhenClarifySuspended(t *testing.T) {
	f := newPostProcessFixture(rt.NewRunRegistry())

	f.runWithClarifySuspended(true)

	for _, c := range f.rs.setCalls {
		if c.status == "completed" {
			t.Fatalf("clarify-suspended turn must not SetRunStatus(completed): %+v", f.rs.setCalls)
		}
	}
	var awaiting bool
	for _, c := range f.rs.setCalls {
		if c.status == string(biz.RunStateAwaitingUser) {
			awaiting = true
		}
	}
	if !awaiting {
		t.Fatalf("expected SetRunStatus(awaiting_user), got %+v", f.rs.setCalls)
	}
	for _, s := range f.transitor.calls {
		if s == sessstatus.SessionStatusCompleted {
			t.Fatalf("clarify-suspended turn must not transition session to completed: %+v", f.transitor.calls)
		}
	}
	if len(f.publisher.bumps) != 1 {
		t.Fatalf("expected 1 revision bump, got %+v", f.publisher.bumps)
	}
	if f.recorder.sessionTurns != 1 {
		t.Fatalf("expected 1 RecordSessionTurn call, got %d", f.recorder.sessionTurns)
	}
}

// TestPostProcessTurn_KeepsSessionRunningWhenSpiritPlanInFlight SM-01: the
// first plan_and_execute reply must not flip the root session to completed
// while a non-direct plan is still draft/executing (teams may not exist yet).
func TestPostProcessTurn_KeepsSessionRunningWhenSpiritPlanInFlight(t *testing.T) {
	f := newPostProcessFixture(rt.NewRunRegistry())
	f.orch.teamExecDeps.Team.TaskPlanner = &stubTaskPlanner{plan: &biz.TaskPlan{
		ID:       "plan-b1",
		Strategy: biz.StrategyParallel,
		Status:   biz.TaskPlanStatusDraft,
	}}

	f.run()

	var runCompletedSeen bool
	for _, c := range f.rs.setCalls {
		if c.status == "completed" {
			runCompletedSeen = true
		}
	}
	if !runCompletedSeen {
		t.Fatalf("expected SetRunStatus(completed), got %+v", f.rs.setCalls)
	}
	for _, s := range f.transitor.calls {
		if s == sessstatus.SessionStatusCompleted {
			t.Fatalf("in-flight spirit plan must not transition session to completed: %+v", f.transitor.calls)
		}
	}
	if len(f.publisher.bumps) != 1 {
		t.Fatalf("expected 1 revision bump, got %+v", f.publisher.bumps)
	}
}

// TestPostProcessTurn_CompletesWhenSpiritPlanTerminal SM-01 control: a
// finished parallel plan is not "work in flight" — session may complete
// (synthesis already happened or there is nothing left to wait for).
func TestPostProcessTurn_CompletesWhenSpiritPlanTerminal(t *testing.T) {
	f := newPostProcessFixture(rt.NewRunRegistry())
	f.orch.teamExecDeps.Team.TaskPlanner = &stubTaskPlanner{plan: &biz.TaskPlan{
		ID:       "plan-done",
		Strategy: biz.StrategyParallel,
		Status:   biz.TaskPlanStatusCompleted,
	}}

	f.run()

	var sessionCompleted bool
	for _, s := range f.transitor.calls {
		if s == sessstatus.SessionStatusCompleted {
			sessionCompleted = true
		}
	}
	if !sessionCompleted {
		t.Fatalf("terminal spirit plan should allow session completed, got %+v", f.transitor.calls)
	}
}

func TestPersistRememberIfRequested_NoopWhenWriterNil(t *testing.T) {
	f := newPostProcessFixture(rt.NewRunRegistry())
	f.orch.persistRememberIfRequested(
		context.Background(),
		biz.Session{ID: "sess-1", UserID: "u1"},
		biz.Agent{ID: "agent-1"},
		biz.TurnInput{Content: "记住：以后都用中文回复，结论先行。"},
		turnExecuteResult{userMsg: biz.ChatMessage{ID: "u-1"}},
	)
}
