package tools

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
)

// ---------------------------------------------------------------------------
// D2-P2（87-orchestration-chain-hardening）：并行配额按「占座」口径计数。
// interrupted 团队（仅产自崩溃恢复，零执行、可经 ResumeTeamIfInterrupted
// 回到 running 重新占座）不占座，不再永久卡死 plan_and_execute；
// pending/running 仍占座。失败降级（ListActiveTeams 出错）保持既有
// warn+跳过检查语义（INV-Q3），不在本用例集新增断言。
// ---------------------------------------------------------------------------

// stubQuotaTeamQuery feeds the parallel-quota block with a fixed active-team
// roster and seat ceiling.
type stubQuotaTeamQuery struct {
	activeTeams []biz.Team
	maxParallel int
}

func (s *stubQuotaTeamQuery) ListActiveTeams(_ context.Context, _ string) ([]biz.Team, error) {
	return s.activeTeams, nil
}

func (s *stubQuotaTeamQuery) ListAllTeams(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}

func (s *stubQuotaTeamQuery) GetMaxParallelTeams(_ context.Context, _ string) int {
	return s.maxParallel
}

// stubQuotaPlanner proves the plan phase was reached (quota放行) by counting
// Plan calls, and answers a direct-strategy plan so the tool returns right
// after Phase 1 without needing an allocator/orchestrator.
type stubQuotaPlanner struct {
	planCalls int
}

func (s *stubQuotaPlanner) Plan(_ context.Context, in biz.PlanInput) (*biz.TaskPlan, error) {
	s.planCalls++
	return &biz.TaskPlan{
		ID:              "plan-quota",
		SpiritSessionID: in.SpiritSessionID,
		Strategy:        biz.StrategyDirect,
	}, nil
}

func (s *stubQuotaPlanner) QuickAssess(_ context.Context, _ biz.PlanInput) (biz.ComplexityLevel, float64, error) {
	return biz.ComplexitySimple, 0, nil
}

func (s *stubQuotaPlanner) GetPlan(_ context.Context, _ string) (*biz.TaskPlan, error) {
	return nil, nil
}

func (s *stubQuotaPlanner) ListPlans(_ context.Context, _ string) ([]*biz.TaskPlan, error) {
	return nil, nil
}

func (s *stubQuotaPlanner) ConfirmPlan(_ context.Context, planID string, _ biz.PlanAdjustments) (*biz.TaskPlan, error) {
	return &biz.TaskPlan{ID: planID}, nil
}

func (s *stubQuotaPlanner) PublishV2Board(_ context.Context, _ *biz.TaskPlan, _ *biz.AllocationPlan, _ string) (biz.PlanBoard, error) {
	return biz.PlanBoard{}, nil
}

func quotaInvocationCtx() context.Context {
	inv := &trpcagent.Invocation{
		Session: &trpcsession.Session{AppName: "app", UserID: "u1", ID: "spirit-quota"},
	}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

func callPlanAndExecuteForQuota(t *testing.T, tq SpiritTeamQueryPort, planner *stubQuotaPlanner) (PlanAndExecuteOutput, error) {
	t.Helper()
	tool := NewPlanAndExecuteTool(planner, nil, nil, tq, nil, nil, loggateway.NewNoop(), nil)
	res, err := tool.Call(quotaInvocationCtx(), []byte(`{"task_prompt":"写一份行业调研报告并汇总分析","force_new":true}`))
	if err != nil {
		return PlanAndExecuteOutput{}, err
	}
	out, ok := res.(PlanAndExecuteOutput)
	if !ok {
		t.Fatalf("plan_and_execute returned %T, want PlanAndExecuteOutput", res)
	}
	return out, nil
}

// maxParallel=1 + 1 interrupted 团队 → quota 放行：进入 plan 阶段，不再
// BadRequest（interrupted 不占座）。
func TestPlanAndExecuteQuota_InterruptedNotOccupied(t *testing.T) {
	t.Parallel()
	tq := &stubQuotaTeamQuery{
		maxParallel: 1,
		activeTeams: []biz.Team{{ID: "t-int", Status: biz.TeamStatusInterrupted}},
	}
	planner := &stubQuotaPlanner{}

	out, err := callPlanAndExecuteForQuota(t, tq, planner)
	if err != nil {
		t.Fatalf("interrupted team must not occupy the parallel seat, got err=%v", err)
	}
	if planner.planCalls != 1 {
		t.Fatalf("quota放行后必须进入 plan 阶段，planCalls=%d want 1", planner.planCalls)
	}
	if out.PlanID != "plan-quota" {
		t.Fatalf("PlanID=%q, want plan-quota (plan phase reached)", out.PlanID)
	}
}

// maxParallel=1 + 1 running 团队 → 仍拒绝 BadRequest（running 占座）。
func TestPlanAndExecuteQuota_RunningOccupiedStillRejected(t *testing.T) {
	t.Parallel()
	tq := &stubQuotaTeamQuery{
		maxParallel: 1,
		activeTeams: []biz.Team{{ID: "t-run", Status: biz.TeamStatusRunning}},
	}
	planner := &stubQuotaPlanner{}

	_, err := callPlanAndExecuteForQuota(t, tq, planner)
	if err == nil {
		t.Fatal("running team occupies the seat: quota must reject with BadRequest")
	}
	if !strings.Contains(err.Error(), "并行团队数已达上限") {
		t.Fatalf("err=%q, want quota BadRequest message", err.Error())
	}
	if planner.planCalls != 0 {
		t.Fatalf("rejected request must not enter plan phase, planCalls=%d", planner.planCalls)
	}
}

// maxParallel=1 + 1 pending 团队 → 仍拒绝（pending 即将执行，占座）。
func TestPlanAndExecuteQuota_PendingOccupiedStillRejected(t *testing.T) {
	t.Parallel()
	tq := &stubQuotaTeamQuery{
		maxParallel: 1,
		activeTeams: []biz.Team{{ID: "t-pend", Status: biz.TeamStatusPending}},
	}
	planner := &stubQuotaPlanner{}

	_, err := callPlanAndExecuteForQuota(t, tq, planner)
	if err == nil {
		t.Fatal("pending team occupies the seat: quota must reject with BadRequest")
	}
	if !strings.Contains(err.Error(), "并行团队数已达上限") {
		t.Fatalf("err=%q, want quota BadRequest message", err.Error())
	}
	if planner.planCalls != 0 {
		t.Fatalf("rejected request must not enter plan phase, planCalls=%d", planner.planCalls)
	}
}
