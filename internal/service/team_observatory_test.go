package service

import (
	"context"
	"fmt"
	"testing"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type observatoryTeamRepo struct {
	team  biz.Team
	run   biz.TeamRunRecord
	steps []biz.TeamRunStep
	runs  []biz.TeamRunRecord
}

func (r *observatoryTeamRepo) ListTeams(context.Context) ([]biz.Team, error) { return nil, nil }
func (r *observatoryTeamRepo) ListTeamsByStatus(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (r *observatoryTeamRepo) GetTeamByID(_ context.Context, id string) (biz.Team, error) {
	if id == r.team.ID {
		return r.team, nil
	}
	return biz.Team{}, fmt.Errorf("team not found")
}
func (r *observatoryTeamRepo) CreateTeam(context.Context, biz.Team) (biz.Team, error) {
	return biz.Team{}, nil
}
func (r *observatoryTeamRepo) UpdateTeam(context.Context, biz.Team) (biz.Team, error) {
	return biz.Team{}, nil
}
func (r *observatoryTeamRepo) UpdateTeamWhereStatus(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}
func (r *observatoryTeamRepo) DeleteTeam(context.Context, string) error { return nil }
func (r *observatoryTeamRepo) BatchArchiveTeams(_ context.Context, _ []string) (int, error) {
	return 0, nil
}
func (r *observatoryTeamRepo) ListTeamRuns(_ context.Context, teamID string, _ int) ([]biz.TeamRunRecord, error) {
	if teamID == r.team.ID {
		return r.runs, nil
	}
	return nil, nil
}
func (r *observatoryTeamRepo) ListTeamRunsByTeamIDs(_ context.Context, _ []string, _ int) (map[string][]biz.TeamRunRecord, error) {
	return nil, nil
}
func (r *observatoryTeamRepo) HasActiveTeamRun(context.Context, string) (bool, error) {
	return false, nil
}
func (r *observatoryTeamRepo) GetTeamRunByID(_ context.Context, id string) (biz.TeamRunRecord, error) {
	if id == r.run.ID {
		return r.run, nil
	}
	return biz.TeamRunRecord{}, fmt.Errorf("run not found")
}
func (r *observatoryTeamRepo) ListTeamRunSteps(_ context.Context, runID string) ([]biz.TeamRunStep, error) {
	if runID == r.run.ID {
		return r.steps, nil
	}
	return nil, nil
}
func (r *observatoryTeamRepo) CreateTeamRun(context.Context, biz.TeamRunRecord) (biz.TeamRunRecord, error) {
	return biz.TeamRunRecord{}, nil
}
func (r *observatoryTeamRepo) UpdateTeamRun(context.Context, biz.TeamRunRecord) error { return nil }
func (r *observatoryTeamRepo) UpdateTeamRunWhereStatus(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}
func (r *observatoryTeamRepo) UpdateTeamRunSummaryJSON(context.Context, string, string) error {
	return nil
}
func (r *observatoryTeamRepo) UpdateTeamRunGraphExecutionID(context.Context, string, string) error {
	return nil
}
func (r *observatoryTeamRepo) UpdateTeamRunTraceID(context.Context, string, string) error { return nil }
func (r *observatoryTeamRepo) BatchCreateOrchestrationSteps(context.Context, []biz.OrchestrationStep) error {
	return nil
}
func (r *observatoryTeamRepo) ListOrchestrationSteps(_ context.Context, runID, _ string, _ int) ([]biz.OrchestrationStep, error) {
	if runID == r.run.ID {
		return nil, nil
	}
	return nil, nil
}
func (r *observatoryTeamRepo) CreateTaskDeadLetter(_ context.Context, _ biz.TaskDeadLetter) error {
	return nil
}
func (r *observatoryTeamRepo) ListTaskDeadLetters(_ context.Context, _ biz.TaskDeadLetterListFilter) ([]biz.TaskDeadLetter, error) {
	return nil, nil
}
func (r *observatoryTeamRepo) ResolveTaskDeadLetter(_ context.Context, _ string) (biz.TaskDeadLetter, error) {
	return biz.TaskDeadLetter{}, nil
}
func (r *observatoryTeamRepo) CreateTeamRunStep(context.Context, biz.TeamRunStep) (biz.TeamRunStep, error) {
	return biz.TeamRunStep{}, nil
}
func (r *observatoryTeamRepo) GetTeamByKey(_ context.Context, _ string) (biz.Team, error) {
	return biz.Team{}, nil
}
func (r *observatoryTeamRepo) ListBySpiritSessionID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (r *observatoryTeamRepo) ListTeamsByDepartmentID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (r *observatoryTeamRepo) ListTeamsByWorkspace(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (r *observatoryTeamRepo) CountTeamsByWorkspace(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func TestGetTeamRunObservatory(t *testing.T) {
	repo := &observatoryTeamRepo{
		team: biz.Team{
			ID:             "t1",
			DefinitionJSON: `{"members":[{"agent_id":"a1","sort_order":1,"name":"A"}]}`,
		},
		run: biz.TeamRunRecord{ID: "run-1", TeamID: "t1", SessionID: "s1", Status: biz.TeamRunStatusSuccess, Mode: "sequential"},
		steps: []biz.TeamRunStep{
			{AgentID: "a1", AgentKey: "k1", AgentName: "A", SortOrder: 1, Status: biz.TeamMemberStepStatusOK, OutputPreview: "done"},
		},
	}
	svc := &TeamService{uc: biz.NewTeamUsecase(biz.TeamUsecaseOpts{Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo, StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop()})}
	resp, err := svc.GetTeamRunObservatory(context.Background(), &v1.GetTeamRunObservatoryRequest{RunId: "run-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Nodes) != 1 || resp.Nodes[0].Status != string(biz.AgentNodeStatusSuccess) {
		t.Fatalf("resp: %+v", resp.Nodes)
	}
}
