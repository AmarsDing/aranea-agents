package service_test

import (
	"context"
	"fmt"
	"testing"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/loggateway"
)

// memTeamRepo is an in-memory repo that satisfies all team-related narrow interfaces.
type memTeamRepo struct {
	teams map[string]biz.Team
}

func newMemTeamRepo() *memTeamRepo {
	return &memTeamRepo{teams: make(map[string]biz.Team)}
}

func (m *memTeamRepo) ListTeams(_ context.Context) ([]biz.Team, error) {
	out := make([]biz.Team, 0, len(m.teams))
	for _, t := range m.teams {
		out = append(out, t)
	}
	return out, nil
}

func (m *memTeamRepo) ListTeamsByStatus(_ context.Context, status string) ([]biz.Team, error) {
	out := make([]biz.Team, 0)
	for _, t := range m.teams {
		if t.Status == status {
			out = append(out, t)
		}
	}
	return out, nil
}

func (m *memTeamRepo) GetTeamByID(_ context.Context, id string) (biz.Team, error) {
	t, ok := m.teams[id]
	if !ok {
		return biz.Team{}, fmt.Errorf("team not found: %s", id)
	}
	return t, nil
}

func (m *memTeamRepo) CreateTeam(_ context.Context, t biz.Team) (biz.Team, error) {
	if t.ID == "" {
		t.ID = fmt.Sprintf("tid-%d", len(m.teams)+1)
	}
	m.teams[t.ID] = t
	return t, nil
}

func (m *memTeamRepo) UpdateTeam(_ context.Context, t biz.Team) (biz.Team, error) {
	if _, ok := m.teams[t.ID]; !ok {
		return biz.Team{}, fmt.Errorf("team not found: %s", t.ID)
	}
	m.teams[t.ID] = t
	return t, nil
}

func (m *memTeamRepo) DeleteTeam(_ context.Context, id string) error {
	delete(m.teams, id)
	return nil
}
func (m *memTeamRepo) BatchArchiveTeams(_ context.Context, _ []string) (int, error) { return 0, nil }

func (m *memTeamRepo) ListTeamRuns(_ context.Context, _ string, _ int) ([]biz.TeamRun, error) {
	return nil, nil
}
func (m *memTeamRepo) ListTeamRunsByTeamIDs(_ context.Context, _ []string, _ int) (map[string][]biz.TeamRun, error) {
	return nil, nil
}
func (m *memTeamRepo) HasActiveTeamRun(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (m *memTeamRepo) GetTeamRunByID(_ context.Context, id string) (biz.TeamRun, error) {
	return biz.TeamRun{}, fmt.Errorf("team run not found: %s", id)
}
func (m *memTeamRepo) ListTeamRunSteps(_ context.Context, _ string) ([]biz.TeamRunStep, error) {
	return nil, nil
}
func (m *memTeamRepo) CreateTeamRun(_ context.Context, r biz.TeamRun) (biz.TeamRun, error) {
	return r, nil
}
func (m *memTeamRepo) UpdateTeamRun(_ context.Context, _ biz.TeamRun) error               { return nil }
func (m *memTeamRepo) UpdateTeamRunSummaryJSON(_ context.Context, _, _ string) error      { return nil }
func (m *memTeamRepo) UpdateTeamRunGraphExecutionID(_ context.Context, _, _ string) error { return nil }
func (m *memTeamRepo) UpdateTeamRunTraceID(_ context.Context, _, _ string) error          { return nil }
func (m *memTeamRepo) BatchCreateOrchestrationSteps(_ context.Context, _ []biz.OrchestrationStep) error {
	return nil
}
func (m *memTeamRepo) ListOrchestrationSteps(_ context.Context, _, _ string, _ int) ([]biz.OrchestrationStep, error) {
	return nil, nil
}
func (m *memTeamRepo) CreateTaskDeadLetter(_ context.Context, _ biz.TaskDeadLetter) error { return nil }
func (m *memTeamRepo) ListTaskDeadLetters(_ context.Context, _ biz.TaskDeadLetterListFilter) ([]biz.TaskDeadLetter, error) {
	return nil, nil
}
func (m *memTeamRepo) ResolveTaskDeadLetter(_ context.Context, _ string) (biz.TaskDeadLetter, error) {
	return biz.TaskDeadLetter{}, nil
}
func (m *memTeamRepo) CreateTeamRunStep(_ context.Context, s biz.TeamRunStep) (biz.TeamRunStep, error) {
	return s, nil
}
func (m *memTeamRepo) GetTeamByKey(_ context.Context, _ string) (biz.Team, error) {
	return biz.Team{}, fmt.Errorf("team not found by key")
}
func (m *memTeamRepo) ListBySpiritSessionID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (m *memTeamRepo) ListTeamsByDepartmentID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}

func newTeamService() *service.TeamService {
	repo := newMemTeamRepo()
	return service.NewTeamService(biz.NewTeamUsecase(repo, repo, repo, repo, repo, repo, nil, nil, nil, nil, loggateway.NewNoop()), nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil)
}

func TestTeamService_CreateListGetDelete(t *testing.T) {
	svc := newTeamService()
	ctx := context.Background()

	created, err := svc.CreateTeam(ctx, &v1.CreateTeamRequest{
		TeamKey:     "team-alpha",
		DisplayName: "Alpha Team",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.GetTeamKey() != "team-alpha" {
		t.Errorf("key mismatch: %s", created.GetTeamKey())
	}

	list, err := svc.ListTeams(ctx, &v1.ListTeamsRequest{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.GetItems()) != 1 {
		t.Errorf("expected 1, got %d", len(list.GetItems()))
	}

	got, err := svc.GetTeam(ctx, &v1.GetTeamRequest{Id: created.GetId()})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.GetId() != created.GetId() {
		t.Errorf("id mismatch")
	}

	_, err = svc.DeleteTeam(ctx, &v1.DeleteTeamRequest{Id: created.GetId()})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	list2, _ := svc.ListTeams(ctx, &v1.ListTeamsRequest{})
	if len(list2.GetItems()) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(list2.GetItems()))
	}
}

func TestTeamService_Update(t *testing.T) {
	svc := newTeamService()
	ctx := context.Background()

	created, _ := svc.CreateTeam(ctx, &v1.CreateTeamRequest{TeamKey: "t1", DisplayName: "Original"})

	updated, err := svc.UpdateTeam(ctx, &v1.UpdateTeamRequest{
		Id:   created.GetId(),
		Team: &v1.Team{Id: created.GetId(), TeamKey: "t1", DisplayName: "Updated"},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.GetDisplayName() != "Updated" {
		t.Errorf("display_name mismatch: %s", updated.GetDisplayName())
	}
}

func TestTeamService_Update_RequiresBody(t *testing.T) {
	svc := newTeamService()
	ctx := context.Background()
	_, err := svc.UpdateTeam(ctx, &v1.UpdateTeamRequest{Id: "any"})
	if err == nil {
		t.Error("expected error for nil team body")
	}
}
