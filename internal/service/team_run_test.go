package service

import (
	"context"
	"fmt"
	"testing"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
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
	svc := NewTeamService(biz.NewTeamUsecase(biz.TeamUsecaseOpts{Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo, StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop()}), nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil, nil, nil, nil)

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
	svc := NewTeamService(biz.NewTeamUsecase(biz.TeamUsecaseOpts{Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo, StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop()}), nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil, nil, nil, nil)
	_, err := svc.RunTeamTest(context.Background(), &v1.RunTeamTestRequest{Id: "t1"})
	if err == nil {
		t.Fatal("expected error when team runner is nil")
	}
}
