package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type deadLetterTeamRepo struct {
	items []biz.TaskDeadLetter
}

func (r *deadLetterTeamRepo) ListTeams(context.Context) ([]biz.Team, error) { return nil, nil }
func (r *deadLetterTeamRepo) ListTeamsByStatus(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (r *deadLetterTeamRepo) GetTeamByID(context.Context, string) (biz.Team, error) {
	return biz.Team{}, biz.ErrNotFound
}
func (r *deadLetterTeamRepo) CreateTeam(context.Context, biz.Team) (biz.Team, error) {
	return biz.Team{}, nil
}
func (r *deadLetterTeamRepo) UpdateTeam(context.Context, biz.Team) (biz.Team, error) {
	return biz.Team{}, nil
}
func (r *deadLetterTeamRepo) UpdateTeamWhereStatus(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}
func (r *deadLetterTeamRepo) DeleteTeam(context.Context, string) error { return nil }
func (r *deadLetterTeamRepo) BatchArchiveTeams(_ context.Context, _ []string) (int, error) {
	return 0, nil
}
func (r *deadLetterTeamRepo) ListTeamRuns(context.Context, string, int) ([]biz.TeamRunRecord, error) {
	return nil, nil
}
func (r *deadLetterTeamRepo) ListTeamRunsByTeamIDs(context.Context, []string, int) (map[string][]biz.TeamRunRecord, error) {
	return nil, nil
}
func (r *deadLetterTeamRepo) HasActiveTeamRun(context.Context, string) (bool, error) {
	return false, nil
}
func (r *deadLetterTeamRepo) GetTeamRunByID(context.Context, string) (biz.TeamRunRecord, error) {
	return biz.TeamRunRecord{}, biz.ErrNotFound
}
func (r *deadLetterTeamRepo) ListTeamRunSteps(context.Context, string) ([]biz.TeamRunStep, error) {
	return nil, nil
}
func (r *deadLetterTeamRepo) CreateTeamRun(context.Context, biz.TeamRunRecord) (biz.TeamRunRecord, error) {
	return biz.TeamRunRecord{}, nil
}
func (r *deadLetterTeamRepo) UpdateTeamRun(context.Context, biz.TeamRunRecord) error { return nil }
func (r *deadLetterTeamRepo) UpdateTeamRunWhereStatus(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}
func (r *deadLetterTeamRepo) UpdateTeamRunGraphExecutionID(context.Context, string, string) error {
	return nil
}
func (r *deadLetterTeamRepo) UpdateTeamRunTraceID(context.Context, string, string) error { return nil }
func (r *deadLetterTeamRepo) UpdateTeamRunSummaryJSON(context.Context, string, string) error {
	return nil
}
func (r *deadLetterTeamRepo) CreateTeamRunStep(context.Context, biz.TeamRunStep) (biz.TeamRunStep, error) {
	return biz.TeamRunStep{}, nil
}
func (r *deadLetterTeamRepo) GetTeamByKey(_ context.Context, _ string) (biz.Team, error) {
	return biz.Team{}, nil
}
func (r *deadLetterTeamRepo) ListBySpiritSessionID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (r *deadLetterTeamRepo) ListTeamsByDepartmentID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (r *deadLetterTeamRepo) ListTeamsByWorkspace(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (r *deadLetterTeamRepo) BatchCreateOrchestrationSteps(context.Context, []biz.OrchestrationStep) error {
	return nil
}
func (r *deadLetterTeamRepo) ListOrchestrationSteps(context.Context, string, string, int) ([]biz.OrchestrationStep, error) {
	return nil, nil
}
func (r *deadLetterTeamRepo) CreateTaskDeadLetter(_ context.Context, dl biz.TaskDeadLetter) error {
	r.items = append(r.items, dl)
	return nil
}
func (r *deadLetterTeamRepo) ListTaskDeadLetters(_ context.Context, filter biz.TaskDeadLetterListFilter) ([]biz.TaskDeadLetter, error) {
	out := make([]biz.TaskDeadLetter, 0)
	for _, item := range r.items {
		if filter.SessionID != "" && item.SessionID != filter.SessionID {
			continue
		}
		if filter.Status != "" && item.Status != filter.Status {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}
func (r *deadLetterTeamRepo) ResolveTaskDeadLetter(_ context.Context, id string) (biz.TaskDeadLetter, error) {
	for i, item := range r.items {
		if item.ID != id {
			continue
		}
		item.Status = biz.TaskDeadLetterStatusResolved
		item.ResolvedAt = "2026-05-23T00:00:00Z"
		r.items[i] = item
		return item, nil
	}
	return biz.TaskDeadLetter{}, biz.ErrNotFound
}

func TestTeamService_ListTaskDeadLetters_requiresScope(t *testing.T) {
	svc := NewTeamService(biz.NewTeamUsecase(biz.TeamUsecaseOpts{Reader: &deadLetterTeamRepo{}, Writer: &deadLetterTeamRepo{}, RunReader: &deadLetterTeamRepo{}, RunWriter: &deadLetterTeamRepo{}, StepRepo: &deadLetterTeamRepo{}, DeadLetter: &deadLetterTeamRepo{}, Lg: loggateway.NewNoop()}), nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil, nil, nil, nil, nil, nil, nil)
	_, err := svc.ListTaskDeadLetters(context.Background(), &v1.ListTaskDeadLettersRequest{})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestTeamService_ListAndResolveTaskDeadLetters(t *testing.T) {
	repo := &deadLetterTeamRepo{items: []biz.TaskDeadLetter{{
		ID: "dl-1", SessionID: "sess-1", Status: biz.TaskDeadLetterStatusPending, SourceType: "team_run",
	}}}
	svc := NewTeamService(biz.NewTeamUsecase(biz.TeamUsecaseOpts{Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo, StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop()}), nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil, nil, nil, nil, nil, nil, nil)
	resp, err := svc.ListTaskDeadLetters(context.Background(), &v1.ListTaskDeadLettersRequest{
		SessionId: "sess-1",
		Status:    biz.TaskDeadLetterStatusPending,
	})
	if err != nil || len(resp.Items) != 1 {
		t.Fatalf("list: err=%v items=%d", err, len(resp.GetItems()))
	}
	resolved, err := svc.ResolveTaskDeadLetter(context.Background(), &v1.ResolveTaskDeadLetterRequest{Id: "dl-1"})
	if err != nil || resolved.GetItem().GetStatus() != biz.TaskDeadLetterStatusResolved {
		t.Fatalf("resolve: err=%v item=%+v", err, resolved.GetItem())
	}
	again, err := svc.ResolveTaskDeadLetter(context.Background(), &v1.ResolveTaskDeadLetterRequest{Id: "dl-1"})
	if err != nil || again.GetItem().GetStatus() != biz.TaskDeadLetterStatusResolved {
		t.Fatalf("idempotent resolve: err=%v item=%+v", err, again.GetItem())
	}
}
