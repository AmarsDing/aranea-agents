package tools

import (
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

func (s *resumePlanStub) Plan(_ context.Context, in biz.PlanInput) (*biz.TaskPlan, error) {
	if in.ResumePlanID != "" {
		select {
		case s.resumeSeen <- in.ResumePlanID:
		default:
		}
		return &biz.TaskPlan{
			ID:              in.ResumePlanID,
			Strategy:        biz.StrategyDirect,
			ComplexityLevel: biz.ComplexitySimple,
		}, nil
	}
	if !in.DeferLLMDecompose {
		return nil, errors.New("plan_and_execute must set DeferLLMDecompose")
	}
	return s.plan, nil
}

func TestPlanAndExecute_DeferredDecompose_ReturnsPlanningInProgress(t *testing.T) {
	resumeSeen := make(chan string, 1)
	planner := &resumePlanStub{
		clarifyPlanStub: clarifyPlanStub{plan: &biz.TaskPlan{
			ID:              "tp_deferred",
			SpiritSessionID: "spirit-1",
			Strategy:        biz.StrategyDAG,
			ComplexityLevel: biz.ComplexityComplex,
			DecomposeReason: biz.DecomposeReasonDeferred,
		}},
		resumeSeen: resumeSeen,
	}
	tool := NewPlanAndExecuteTool(planner, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil)
	ctx := trpcagent.NewInvocationContext(context.Background(), &trpcagent.Invocation{
		Session: &trpcsession.Session{ID: "spirit-1"},
	})
	res, err := tool.Call(ctx, []byte(`{"task_prompt":"组建两个团队做跨部门交付","mode":"dag"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	out, ok := res.(PlanAndExecuteOutput)
	if !ok {
		t.Fatalf("result type = %T", res)
	}
	if out.NextAction != biz.PlanningInProgressNextAction {
		t.Fatalf("NextAction = %q, want %s", out.NextAction, biz.PlanningInProgressNextAction)
	}
	select {
	case id := <-resumeSeen:
		if id != "tp_deferred" {
			t.Fatalf("ResumePlanID = %q, want tp_deferred", id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("background Plan(ResumePlanID) was not called")
	}
}

type deferCaptureStub struct {
	clarifyPlanStub
	deferSeen chan bool
}

func (s *deferCaptureStub) Plan(_ context.Context, in biz.PlanInput) (*biz.TaskPlan, error) {
	select {
	case s.deferSeen <- in.DeferLLMDecompose:
	default:
	}
	return s.plan, nil
}

func TestPlanAndExecute_CommittedPlanTeam_DoesNotDefer(t *testing.T) {
	seen := make(chan bool, 1)
	planner := &deferCaptureStub{
		clarifyPlanStub: clarifyPlanStub{plan: &biz.TaskPlan{
			ID:              "tp_commit",
			SpiritSessionID: "spirit-1",
			Strategy:        biz.StrategyParallel,
			ComplexityLevel: biz.ComplexityModerate,
			DecomposeReason: biz.DecomposeReasonFailed,
		}},
		deferSeen: seen,
	}
	tool := NewPlanAndExecuteTool(planner, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil)
	invCtx := trpcagent.NewInvocationContext(context.Background(), &trpcagent.Invocation{
		Session: &trpcsession.Session{ID: "spirit-1"},
	})
	ctx := biz.ContextWithRouteDecision(invCtx, biz.RouteDecision{
		Lane: biz.RouteLanePlanTeam, Mode: "dag", ForcePlanning: true,
	})
	res, err := tool.Call(ctx, []byte(`{"task_prompt":"让市场部出一版 Q3 推广文案框架","mode":"dag"}`))
	if err != nil {
		t.Fatalf("Call err: %v", err)
	}
	out, ok := res.(PlanAndExecuteOutput)
	if !ok {
		t.Fatalf("result type = %T", res)
	}
	select {
	case def := <-seen:
		if def {
			t.Fatal("committed PlanTeam must not DeferLLMDecompose")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Plan() was not called")
	}
	if out.NextAction != biz.DecomposeFailedNextAction {
		t.Fatalf("NextAction = %q, want decompose_failed (no DIY gap)", out.NextAction)
	}
}

type stubStartupWaiter struct {
	ok     bool
	reason string
	called chan string // boardID
}

func (s *stubStartupWaiter) WaitBoardStartup(_ context.Context, boardID string, _ time.Duration) (bool, string) {
	if s.called != nil {
		select {
		case s.called <- boardID:
		default:
		}
	}
	return s.ok, s.reason
}

// 对账失败（零 team_runs）→ orchestrate 阶段返回错误，Spirit 必须如实告知，
// 不得声称已组建团队（S07 事故回归守卫）。
func TestExecuteOrchestratePhase_StartupDrift_Blocked(t *testing.T) {
	waiter := &stubStartupWaiter{ok: false, reason: "step s1 failed: orchestrate: agent keys not found or not active: ghost", called: make(chan string, 1)}
	deps := planAndExecuteDeps{lg: loggateway.NewNoop(), startupWaiter: waiter}
	plan := &biz.TaskPlan{ID: "tp_drift", SpiritSessionID: "spirit-1", Strategy: biz.StrategyParallel}
	alloc := &biz.AllocationPlan{ID: "ap_drift"}

	handle, step, err := executeOrchestratePhase(context.Background(), plan, alloc, "board-drift", deps)
	if err == nil {
		t.Fatal("expected error when startup reconciliation fails, got nil")
	}
	if handle != nil {
		t.Fatalf("handle = %+v, want nil on reconciliation failure", handle)
	}
	if step.StepName != "orchestrate" || step.Status != "failed" {
		t.Fatalf("step = %+v, want orchestrate/failed", step)
	}
	if !strings.Contains(err.Error(), "启动对账") {
		t.Fatalf("err = %v, want contains 启动对账", err)
	}
	select {
	case boardID := <-waiter.called:
		if boardID != "board-drift" {
			t.Fatalf("waiter boardID = %q, want board-drift", boardID)
		}
	default:
		t.Fatal("startup waiter was not consulted")
	}
}

// 对账通过（首个 team 已落地）→ 正常返回 running handle。
func TestExecuteOrchestratePhase_StartupOK_Running(t *testing.T) {
	waiter := &stubStartupWaiter{ok: true}
	deps := planAndExecuteDeps{lg: loggateway.NewNoop(), startupWaiter: waiter}
	plan := &biz.TaskPlan{ID: "tp_ok", SpiritSessionID: "spirit-1", Strategy: biz.StrategyParallel}
	alloc := &biz.AllocationPlan{ID: "ap_ok"}

	handle, step, err := executeOrchestratePhase(context.Background(), plan, alloc, "board-ok", deps)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if handle == nil || handle.ID != "board-ok" || handle.Status != biz.OrchestrationStatusRunning {
		t.Fatalf("handle = %+v, want running handle with board id", handle)
	}
	if step.Status != "running" {
		t.Fatalf("step.Status = %q, want running", step.Status)
	}
}

// nil waiter（v1-only / 旧装配路径）→ 跳过对账，保持既有行为。
func TestExecuteOrchestratePhase_NilWaiter_Skips(t *testing.T) {
	deps := planAndExecuteDeps{lg: loggateway.NewNoop()}
	plan := &biz.TaskPlan{ID: "tp_nil", SpiritSessionID: "spirit-1", Strategy: biz.StrategyParallel}
	alloc := &biz.AllocationPlan{ID: "ap_nil"}

	handle, _, err := executeOrchestratePhase(context.Background(), plan, alloc, "board-nil", deps)
	if err != nil {
		t.Fatalf("unexpected err with nil waiter: %v", err)
	}
	if handle == nil || handle.ID != "board-nil" {
		t.Fatalf("handle = %+v, want board-nil", handle)
	}
}
