package service

import (
	"context"
	"fmt"
	"strings"
	"testing"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

type summaryTeamRepo struct {
	runs  map[string]biz.TeamRunRecord
	steps map[string][]biz.TeamRunStep
}

// TeamReader stubs
func (r *summaryTeamRepo) ListTeams(_ context.Context) ([]biz.Team, error) { return nil, nil }
func (r *summaryTeamRepo) GetTeamByID(_ context.Context, id string) (biz.Team, error) {
	if id == "t1" {
		return biz.Team{ID: "t1"}, nil
	}
	return biz.Team{}, fmt.Errorf("not found")
}
func (r *summaryTeamRepo) GetTeamByKey(_ context.Context, _ string) (biz.Team, error) {
	return biz.Team{}, fmt.Errorf("not found")
}
func (r *summaryTeamRepo) ListBySpiritSessionID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (r *summaryTeamRepo) ListTeamsByStatus(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (r *summaryTeamRepo) ListTeamsByDepartmentID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (r *summaryTeamRepo) ListTeamsByWorkspace(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (r *summaryTeamRepo) CountTeamsByWorkspace(_ context.Context, _ string) (int, error) {
	return 0, nil
}

// TeamWriter stubs
func (r *summaryTeamRepo) CreateTeam(_ context.Context, t biz.Team) (biz.Team, error) { return t, nil }
func (r *summaryTeamRepo) UpdateTeam(_ context.Context, t biz.Team) (biz.Team, error) { return t, nil }
func (r *summaryTeamRepo) UpdateTeamWhereStatus(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}
func (r *summaryTeamRepo) DeleteTeam(_ context.Context, _ string) error { return nil }
func (r *summaryTeamRepo) BatchArchiveTeams(_ context.Context, _ []string) (int, error) {
	return 0, nil
}

// TeamRunReader
func (r *summaryTeamRepo) GetTeamRunByID(_ context.Context, id string) (biz.TeamRunRecord, error) {
	run, ok := r.runs[id]
	if !ok {
		return biz.TeamRunRecord{}, fmt.Errorf("not found")
	}
	return run, nil
}
func (r *summaryTeamRepo) ListTeamRunSteps(_ context.Context, runID string) ([]biz.TeamRunStep, error) {
	return r.steps[runID], nil
}
func (r *summaryTeamRepo) ListTeamRuns(_ context.Context, _ string, _ int) ([]biz.TeamRunRecord, error) {
	return nil, nil
}
func (r *summaryTeamRepo) ListTeamRunsByTeamIDs(_ context.Context, _ []string, _ int) (map[string][]biz.TeamRunRecord, error) {
	return nil, nil
}
func (r *summaryTeamRepo) HasActiveTeamRun(_ context.Context, _ string) (bool, error) {
	return false, nil
}

// TeamRunWriter stubs
func (r *summaryTeamRepo) CreateTeamRun(_ context.Context, run biz.TeamRunRecord) (biz.TeamRunRecord, error) {
	return run, nil
}
func (r *summaryTeamRepo) UpdateTeamRun(_ context.Context, _ biz.TeamRunRecord) error { return nil }
func (r *summaryTeamRepo) UpdateTeamRunWhereStatus(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}
func (r *summaryTeamRepo) UpdateTeamRunGraphExecutionID(_ context.Context, _, _ string) error {
	return nil
}
func (r *summaryTeamRepo) UpdateTeamRunTraceID(_ context.Context, _, _ string) error     { return nil }
func (r *summaryTeamRepo) UpdateTeamRunSummaryJSON(_ context.Context, _, _ string) error { return nil }
func (r *summaryTeamRepo) CreateTeamRunStep(_ context.Context, s biz.TeamRunStep) (biz.TeamRunStep, error) {
	return s, nil
}

// OrchestrationStepRepo stubs
func (r *summaryTeamRepo) BatchCreateOrchestrationSteps(_ context.Context, _ []biz.OrchestrationStep) error {
	return nil
}
func (r *summaryTeamRepo) ListOrchestrationSteps(_ context.Context, _, _ string, _ int) ([]biz.OrchestrationStep, error) {
	return nil, nil
}

// TaskDeadLetterRepo stubs
func (r *summaryTeamRepo) CreateTaskDeadLetter(_ context.Context, _ biz.TaskDeadLetter) error {
	return nil
}
func (r *summaryTeamRepo) ListTaskDeadLetters(_ context.Context, _ biz.TaskDeadLetterListFilter) ([]biz.TaskDeadLetter, error) {
	return nil, nil
}
func (r *summaryTeamRepo) ResolveTaskDeadLetter(_ context.Context, _ string) (biz.TaskDeadLetter, error) {
	return biz.TaskDeadLetter{}, nil
}
func (r *summaryTeamRepo) GetTaskDeadLetter(_ context.Context, _ string) (biz.TaskDeadLetter, error) {
	return biz.TaskDeadLetter{}, biz.ErrNotFound
}

func TestGetTeamRunSummary_AggregatesSteps(t *testing.T) {
	repo := &summaryTeamRepo{
		runs: map[string]biz.TeamRunRecord{
			"run-1": {ID: "run-1", TeamID: "t1", SessionID: "s1", Mode: "sequential", Status: biz.TeamRunStatusSuccess, TokenIn: 5, TokenOut: 10},
		},
		steps: map[string][]biz.TeamRunStep{
			"run-1": {
				{AgentKey: "a1", AgentName: "One", Role: "worker", ToolCallCount: 3, TokenOut: 10},
			},
		},
	}
	svc := NewTeamService(biz.NewTeamUsecase(biz.TeamUsecaseOpts{Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo, StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop()}), nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	resp, err := svc.GetTeamRunSummary(context.Background(), &v1.GetTeamRunSummaryRequest{Id: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	sum := resp.GetSummary()
	if sum.GetToolCallCount() != 3 {
		t.Fatalf("tool_call_count=%d", sum.GetToolCallCount())
	}
	if len(sum.GetMembers()) != 1 || sum.GetMembers()[0].GetToolCallCount() != 3 {
		t.Fatalf("members=%+v", sum.GetMembers())
	}
}

func TestRunTeamTest_RequiresRuntime(t *testing.T) {
	repo := &summaryTeamRepo{runs: map[string]biz.TeamRunRecord{}}
	svc := NewTeamService(biz.NewTeamUsecase(biz.TeamUsecaseOpts{Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo, StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop()}), nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	_, err := svc.RunTeamTest(context.Background(), &v1.RunTeamTestRequest{Id: "t1"})
	if err == nil {
		t.Fatal("expected error when team runner is nil")
	}
}

// ── t-dr-1：守卫 abort → 结构化 run 响应（而非 500 空 body）──────────────────

// teamTestSessionRepo 支撑 RunTeamTest 的临时会话 Create/Delete 全链路。
type teamTestSessionRepo struct {
	biz.SessionRepo
	last biz.Session
}

func (r *teamTestSessionRepo) CreateSession(_ context.Context, s biz.Session) (biz.Session, error) {
	r.last = s
	return s, nil
}

func (r *teamTestSessionRepo) GetSessionByID(_ context.Context, id string) (biz.Session, error) {
	if r.last.ID == id {
		return r.last, nil
	}
	return biz.Session{}, biz.ErrNotFound
}

func (r *teamTestSessionRepo) DeleteSession(_ context.Context, _ string) (int, error) {
	return 1, nil
}

// guardAbortTeamRepo 按"已建测试会话"动态回填 run 记录（会话 ID 由
// SessionUsecase.Create 随机生成，静态预置无法匹配 findTeamTestRun 的
// SessionID 过滤）。
type guardAbortTeamRepo struct {
	*cancelTeamRunRepo
	sessRepo *teamTestSessionRepo
}

func (r *guardAbortTeamRepo) ListTeamRuns(_ context.Context, teamID string, _ int) ([]biz.TeamRunRecord, error) {
	return []biz.TeamRunRecord{{
		ID:           "run-g1",
		TeamID:       teamID,
		SessionID:    r.sessRepo.last.ID,
		Status:       biz.TeamRunStatusFailed,
		ErrorMessage: "context canceled (cancel_reason=team_token_budget_exceeded)",
	}}, nil
}

// TestRunTeamTest_GuardAbortReturnsStructuredRun 覆盖 t-dr-1（2026-08-27）：
// token 预算闸等守卫 abort run ctx 时 RunTurnFromInput 返回 context.Canceled，
// 但 run 终态（failed + cancel_reason）已落库——handler 必须返回结构化
// run 记录（200），调用方从 run.status/error_message 读到终止来源；修复前
// 裸错误透传被渲染成 500 空 body。
func TestRunTeamTest_GuardAbortReturnsStructuredRun(t *testing.T) {
	sessRepo := &teamTestSessionRepo{}
	repo := &guardAbortTeamRepo{
		cancelTeamRunRepo: &cancelTeamRunRepo{
			teamByID: map[string]biz.Team{"t1": {ID: "t1", TeamKey: "t1"}},
		},
		sessRepo: sessRepo,
	}
	uc := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo,
		StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop(),
	})
	sessionUC := biz.NewSessionUsecase(sessRepo, nil, biz.NewSessionTeamLookup(repo), nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil)
	runner := &capturingTeamRunner{runErr: context.Canceled}
	svc := NewTeamService(uc, nil, nil, sessionUC, runner, &testRunRegistry{}, event.NewV2Bus(), loggateway.NewNoop(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	resp, err := svc.RunTeamTest(wsCtx(workspace.SystemWorkspaceID), &v1.RunTeamTestRequest{Id: "t1", Content: "hi"})
	if err != nil {
		t.Fatalf("guard-aborted run must return structured response, got err: %v", err)
	}
	if got := resp.GetRun().GetId(); got != "run-g1" {
		t.Errorf("run.id = %q, want run-g1", got)
	}
	if got := resp.GetRun().GetStatus(); got != biz.TeamRunStatusFailed {
		t.Errorf("run.status = %q, want %q", got, biz.TeamRunStatusFailed)
	}
	if got := resp.GetRun().GetErrorMessage(); !strings.Contains(got, "team_token_budget_exceeded") {
		t.Errorf("run.error_message = %q, want containing team_token_budget_exceeded", got)
	}
}

// TestRunTeamTest_NonGuardErrorPropagates：非守卫类错误（非 context.Canceled/
// DeadlineExceeded）保持原有透传语义——内部错误必须显式失败而非被结构化
// 响应掩盖。
func TestRunTeamTest_NonGuardErrorPropagates(t *testing.T) {
	sessRepo := &teamTestSessionRepo{}
	repo := &guardAbortTeamRepo{
		cancelTeamRunRepo: &cancelTeamRunRepo{
			teamByID: map[string]biz.Team{"t1": {ID: "t1", TeamKey: "t1"}},
		},
		sessRepo: sessRepo,
	}
	uc := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo,
		StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop(),
	})
	sessionUC := biz.NewSessionUsecase(sessRepo, nil, biz.NewSessionTeamLookup(repo), nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil)
	runner := &capturingTeamRunner{runErr: fmt.Errorf("llm provider boom")}
	svc := NewTeamService(uc, nil, nil, sessionUC, runner, &testRunRegistry{}, event.NewV2Bus(), loggateway.NewNoop(), nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)

	_, err := svc.RunTeamTest(wsCtx(workspace.SystemWorkspaceID), &v1.RunTeamTestRequest{Id: "t1", Content: "hi"})
	if err == nil {
		t.Fatal("non-guard runner error must propagate")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("err = %v, want containing boom", err)
	}
}
