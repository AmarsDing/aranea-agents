package service

import (
	"context"
	"fmt"
	"testing"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
)

type summaryTeamRepo struct {
	biz.TeamRepository
	runs  map[string]biz.TeamRun
	steps map[string][]biz.TeamRunStep
}

func (r *summaryTeamRepo) GetTeamRunByID(_ context.Context, id string) (biz.TeamRun, error) {
	run, ok := r.runs[id]
	if !ok {
		return biz.TeamRun{}, fmt.Errorf("not found")
	}
	return run, nil
}

func (r *summaryTeamRepo) ListTeamRunSteps(_ context.Context, runID string) ([]biz.TeamRunStep, error) {
	return r.steps[runID], nil
}

func (r *summaryTeamRepo) UpdateTeamRunSummaryJSON(_ context.Context, _, _ string) error { return nil }
func (r *summaryTeamRepo) UpdateTeamRunGraphExecutionID(_ context.Context, _, _ string) error { return nil }
func (r *summaryTeamRepo) UpdateTeamRunTraceID(_ context.Context, _, _ string) error             { return nil }
func (r *summaryTeamRepo) BatchCreateOrchestrationSteps(_ context.Context, _ []biz.OrchestrationStep) error {
	return nil
}
func (r *summaryTeamRepo) ListOrchestrationSteps(_ context.Context, _, _ string, _ int) ([]biz.OrchestrationStep, error) {
	return nil, nil
}
func (r *summaryTeamRepo) CreateTaskDeadLetter(_ context.Context, _ biz.TaskDeadLetter) error { return nil }
func (r *summaryTeamRepo) ListTaskDeadLetters(_ context.Context, _ biz.TaskDeadLetterListFilter) ([]biz.TaskDeadLetter, error) {
	return nil, nil
}
func (r *summaryTeamRepo) ResolveTaskDeadLetter(_ context.Context, _ string) (biz.TaskDeadLetter, error) {
	return biz.TaskDeadLetter{}, nil
}

func (r *summaryTeamRepo) GetTeamByID(_ context.Context, id string) (biz.Team, error) {
	if id == "t1" {
		return biz.Team{ID: "t1"}, nil
	}
	return biz.Team{}, fmt.Errorf("not found")
}

func TestGetTeamRunSummary_AggregatesSteps(t *testing.T) {
	repo := &summaryTeamRepo{
		runs: map[string]biz.TeamRun{
			"run-1": {ID: "run-1", TeamID: "t1", SessionID: "s1", Mode: "sequential", Status: "success", TokenIn: 5, TokenOut: 10},
		},
		steps: map[string][]biz.TeamRunStep{
			"run-1": {
				{AgentKey: "a1", AgentName: "One", Role: "worker", ToolCallCount: 3, TokenOut: 10},
			},
		},
	}
	svc := NewTeamService(biz.NewTeamUsecase(repo, nil), nil, nil, nil, nil, nil, nil)

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
	repo := &summaryTeamRepo{runs: map[string]biz.TeamRun{}}
	svc := NewTeamService(biz.NewTeamUsecase(repo, nil), nil, nil, nil, nil, nil, nil)
	_, err := svc.RunTeamTest(context.Background(), &v1.RunTeamTestRequest{Id: "t1"})
	if err == nil {
		t.Fatal("expected error when team runner is nil")
	}
}
