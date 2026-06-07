package service

import (
	"context"
	"database/sql"
	"testing"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/team"
	"aranea-agents/pkg/loggateway"
)

type compileTeamRepo struct {
	team biz.Team
}

func (r *compileTeamRepo) ListTeams(context.Context) ([]biz.Team, error)                    { return nil, nil }
func (r *compileTeamRepo) ListTeamsByStatus(_ context.Context, _ string) ([]biz.Team, error) { return nil, nil }
func (r *compileTeamRepo) GetTeamByID(_ context.Context, id string) (biz.Team, error) {
	if id == r.team.ID {
		return r.team, nil
	}
	return biz.Team{}, sql.ErrNoRows
}
func (r *compileTeamRepo) CreateTeam(context.Context, biz.Team) (biz.Team, error) {
	return biz.Team{}, nil
}
func (r *compileTeamRepo) UpdateTeam(context.Context, biz.Team) (biz.Team, error) {
	return biz.Team{}, nil
}
func (r *compileTeamRepo) DeleteTeam(context.Context, string) error { return nil }
func (r *compileTeamRepo) BatchArchiveTeams(_ context.Context, _ []string) (int, error) { return 0, nil }
func (r *compileTeamRepo) ListTeamRuns(context.Context, string, int) ([]biz.TeamRun, error) {
	return nil, nil
}
func (r *compileTeamRepo) ListTeamRunsByTeamIDs(context.Context, []string, int) (map[string][]biz.TeamRun, error) {
	return nil, nil
}
func (r *compileTeamRepo) HasActiveTeamRun(context.Context, string) (bool, error) {
	return false, nil
}
func (r *compileTeamRepo) GetTeamRunByID(context.Context, string) (biz.TeamRun, error) {
	return biz.TeamRun{}, sql.ErrNoRows
}
func (r *compileTeamRepo) ListTeamRunSteps(context.Context, string) ([]biz.TeamRunStep, error) {
	return nil, nil
}
func (r *compileTeamRepo) CreateTeamRun(context.Context, biz.TeamRun) (biz.TeamRun, error) {
	return biz.TeamRun{}, nil
}
func (r *compileTeamRepo) UpdateTeamRun(context.Context, biz.TeamRun) error { return nil }
func (r *compileTeamRepo) UpdateTeamRunSummaryJSON(context.Context, string, string) error {
	return nil
}
func (r *compileTeamRepo) UpdateTeamRunGraphExecutionID(context.Context, string, string) error {
	return nil
}
func (r *compileTeamRepo) UpdateTeamRunTraceID(context.Context, string, string) error { return nil }
func (r *compileTeamRepo) BatchCreateOrchestrationSteps(context.Context, []biz.OrchestrationStep) error {
	return nil
}
func (r *compileTeamRepo) ListOrchestrationSteps(context.Context, string, string, int) ([]biz.OrchestrationStep, error) {
	return nil, nil
}
func (r *compileTeamRepo) CreateTaskDeadLetter(_ context.Context, _ biz.TaskDeadLetter) error {
	return nil
}
func (r *compileTeamRepo) ListTaskDeadLetters(_ context.Context, _ biz.TaskDeadLetterListFilter) ([]biz.TaskDeadLetter, error) {
	return nil, nil
}
func (r *compileTeamRepo) ResolveTaskDeadLetter(_ context.Context, _ string) (biz.TaskDeadLetter, error) {
	return biz.TaskDeadLetter{}, nil
}
func (r *compileTeamRepo) CreateTeamRunStep(context.Context, biz.TeamRunStep) (biz.TeamRunStep, error) {
	return biz.TeamRunStep{}, nil
}
func (r *compileTeamRepo) GetTeamByKey(_ context.Context, _ string) (biz.Team, error) {
	return biz.Team{}, nil
}
func (r *compileTeamRepo) ListBySpiritSessionID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}

func TestCompileTeamGraph_sequential(t *testing.T) {
	repo := &compileTeamRepo{team: biz.Team{
		ID:             "t1",
		DefinitionJSON: `{"version":1,"mode":"sequential","members":[{"agent_id":"a1","role":"worker","sort_order":1,"enabled":true},{"agent_id":"a2","role":"worker","sort_order":2,"enabled":true}]}`,
	}}
	svc := NewTeamService(biz.NewTeamUsecase(repo, repo, repo, repo, repo, repo, nil, nil, nil, nil, loggateway.NewNoop()), nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	resp, err := svc.CompileTeamGraph(context.Background(), &v1.CompileTeamGraphRequest{TeamId: "t1"})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.GetValid() {
		t.Fatalf("valid=false issues=%v", resp.GetIssues())
	}
	if resp.GetTemplateId() != team.CompileTemplateID("sequential") {
		t.Fatalf("template=%q", resp.GetTemplateId())
	}
	if len(resp.GetNodes()) != 2 {
		t.Fatalf("nodes=%d", len(resp.GetNodes()))
	}
}
