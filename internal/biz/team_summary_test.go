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
	uc := NewTeamUsecase(TeamUsecaseOpts{Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo, StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop()})
	data, err := uc.GetRunSummary(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if data.ToolCallCount != 4 {
		t.Fatalf("tool_call_count=%d", data.ToolCallCount)
	}
}

type runSummaryRepo struct {
	runs  map[string]TeamRun
	steps map[string][]TeamRunStep
}

// TeamReader stubs
func (r *runSummaryRepo) ListTeams(_ context.Context) ([]Team, error)                         { return nil, nil }
func (r *runSummaryRepo) ListTeamsByStatus(_ context.Context, _ string) ([]Team, error)       { return nil, nil }
func (r *runSummaryRepo) GetTeamByID(_ context.Context, _ string) (Team, error)               { return Team{}, fmt.Errorf("not found") }
func (r *runSummaryRepo) GetTeamByKey(_ context.Context, _ string) (Team, error)              { return Team{}, fmt.Errorf("not found") }
func (r *runSummaryRepo) ListBySpiritSessionID(_ context.Context, _ string) ([]Team, error)   { return nil, nil }
func (r *runSummaryRepo) ListTeamsByDepartmentID(_ context.Context, _ string) ([]Team, error) { return nil, nil }

// TeamWriter stubs
func (r *runSummaryRepo) CreateTeam(_ context.Context, t Team) (Team, error)         { return t, nil }
func (r *runSummaryRepo) UpdateTeam(_ context.Context, t Team) (Team, error)         { return t, nil }
func (r *runSummaryRepo) DeleteTeam(_ context.Context, _ string) error               { return nil }
func (r *runSummaryRepo) BatchArchiveTeams(_ context.Context, _ []string) (int, error) { return 0, nil }
func (r *runSummaryRepo) UpdateTeamWhereStatus(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}

// TeamRunReader
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
func (r *runSummaryRepo) ListTeamRuns(_ context.Context, _ string, _ int) ([]TeamRun, error) {
	return nil, nil
}
func (r *runSummaryRepo) ListTeamRunsByTeamIDs(_ context.Context, _ []string, _ int) (map[string][]TeamRun, error) {
	return nil, nil
}
func (r *runSummaryRepo) HasActiveTeamRun(_ context.Context, _ string) (bool, error) { return false, nil }

// TeamRunWriter stubs
func (r *runSummaryRepo) CreateTeamRun(_ context.Context, run TeamRun) (TeamRun, error) { return run, nil }
func (r *runSummaryRepo) UpdateTeamRun(_ context.Context, _ TeamRun) error              { return nil }
func (r *runSummaryRepo) UpdateTeamRunWhereStatus(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}
func (r *runSummaryRepo) UpdateTeamRunGraphExecutionID(_ context.Context, _, _ string) error {
	return nil
}
func (r *runSummaryRepo) UpdateTeamRunTraceID(_ context.Context, _, _ string) error { return nil }
func (r *runSummaryRepo) UpdateTeamRunSummaryJSON(_ context.Context, _, _ string) error {
	return nil
}
func (r *runSummaryRepo) CreateTeamRunStep(_ context.Context, s TeamRunStep) (TeamRunStep, error) {
	return s, nil
}

// OrchestrationStepRepo stubs
func (r *runSummaryRepo) BatchCreateOrchestrationSteps(_ context.Context, _ []OrchestrationStep) error {
	return nil
}
func (r *runSummaryRepo) ListOrchestrationSteps(_ context.Context, _, _ string, _ int) ([]OrchestrationStep, error) {
	return nil, nil
}

// TaskDeadLetterRepo stubs
func (r *runSummaryRepo) CreateTaskDeadLetter(_ context.Context, _ TaskDeadLetter) error { return nil }
func (r *runSummaryRepo) ListTaskDeadLetters(_ context.Context, _ TaskDeadLetterListFilter) ([]TaskDeadLetter, error) {
	return nil, nil
}
func (r *runSummaryRepo) ResolveTaskDeadLetter(_ context.Context, _ string) (TaskDeadLetter, error) {
	return TaskDeadLetter{}, nil
}
