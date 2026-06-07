package biz

import (
	"context"
	"fmt"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestBuildTeamRunSummaryData(t *testing.T) {
	run := TeamRun{
		ID: "run-1", TeamID: "team-1", SessionID: "sess-1",
		Mode: "sequential", Status: TeamRunStatusSuccess,
		TokenIn: 10, TokenOut: 20, DurationMS: 100,
	}
	steps := []TeamRunStep{
		{AgentKey: "a1", AgentName: "Agent One", Role: "worker", SortOrder: 0, Status: TeamMemberStepStatusOK, TokenOut: 20, ToolCallCount: 2},
	}
	data := BuildTeamRunSummaryData(run, steps)
	if data.RunID != "run-1" || data.ToolCallCount != 2 || data.MemberCount != 1 {
		t.Fatalf("summary=%+v", data)
	}
	if len(data.Members) != 1 || data.Members[0].AgentKey != "a1" || data.Members[0].ToolCallCount != 2 {
		t.Fatalf("members=%+v", data.Members)
	}
}

func TestTeamUsecase_GetRunSummary(t *testing.T) {
	repo := &runSummaryRepo{
		runs: map[string]TeamRun{
			"run-1": {ID: "run-1", TeamID: "t1", SessionID: "s1", Mode: "sequential", Status: TeamRunStatusSuccess},
		},
		steps: map[string][]TeamRunStep{
			"run-1": {{AgentKey: "a1", ToolCallCount: 4}},
		},
	}
	uc := NewTeamUsecase(repo, repo, repo, repo, repo, repo, nil, nil, nil, nil, loggateway.NewNoop())
	data, err := uc.GetRunSummary(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if data.ToolCallCount != 4 {
		t.Fatalf("tool_call_count=%d", data.ToolCallCount)
	}
}

type runSummaryRepo struct {
	TeamRepository
	runs  map[string]TeamRun
	steps map[string][]TeamRunStep
}

func (r *runSummaryRepo) GetTeamRunByID(_ context.Context, id string) (TeamRun, error) {
	run, ok := r.runs[id]
	if !ok {
		return TeamRun{}, fmt.Errorf("not found")
	}
	return run, nil
}

func (r *runSummaryRepo) ListTeamRunSteps(_ context.Context, runID string) ([]TeamRunStep, error) {
	return r.steps[runID], nil
}
func (r *runSummaryRepo) ListBySpiritSessionID(_ context.Context, _ string) ([]Team, error) {
	return nil, nil
}
func (r *runSummaryRepo) ListTeamsByStatus(_ context.Context, _ string) ([]Team, error) {
	return nil, nil
}
func (r *runSummaryRepo) ListTeamsByDepartmentID(_ context.Context, _ string) ([]Team, error) {
	return nil, nil
}
