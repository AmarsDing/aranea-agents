package service

import (
	"context"
	"fmt"
	"testing"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

type cancelTeamRunRepo struct {
	biz.TeamRepository
	runs map[string]biz.TeamRun
}

func (r *cancelTeamRunRepo) ListTeams(_ context.Context) ([]biz.Team, error)          { return nil, nil }
func (r *cancelTeamRunRepo) ListTeamsByStatus(_ context.Context, _ string) ([]biz.Team, error) { return nil, nil }
func (r *cancelTeamRunRepo) GetTeamByID(_ context.Context, _ string) (biz.Team, error) {
	return biz.Team{}, biz.ErrNotFound
}
func (r *cancelTeamRunRepo) CreateTeam(_ context.Context, t biz.Team) (biz.Team, error) { return t, nil }
func (r *cancelTeamRunRepo) UpdateTeam(_ context.Context, t biz.Team) (biz.Team, error) { return t, nil }
func (r *cancelTeamRunRepo) DeleteTeam(_ context.Context, _ string) error                 { return nil }
func (r *cancelTeamRunRepo) ListTeamRuns(_ context.Context, _ string, _ int) ([]biz.TeamRun, error) {
	return nil, nil
}
func (r *cancelTeamRunRepo) GetTeamRunByID(_ context.Context, id string) (biz.TeamRun, error) {
	run, ok := r.runs[id]
	if !ok {
		return biz.TeamRun{}, fmt.Errorf("not found")
	}
	return run, nil
}
func (r *cancelTeamRunRepo) ListTeamRunSteps(_ context.Context, _ string) ([]biz.TeamRunStep, error) {
	return nil, nil
}
func (r *cancelTeamRunRepo) CreateTeamRun(_ context.Context, run biz.TeamRun) (biz.TeamRun, error) {
	return run, nil
}
func (r *cancelTeamRunRepo) UpdateTeamRun(_ context.Context, run biz.TeamRun) error {
	r.runs[run.ID] = run
	return nil
}
func (r *cancelTeamRunRepo) UpdateTeamRunSummaryJSON(_ context.Context, _, _ string) error { return nil }
func (r *cancelTeamRunRepo) UpdateTeamRunGraphExecutionID(_ context.Context, _, _ string) error { return nil }
func (r *cancelTeamRunRepo) UpdateTeamRunTraceID(_ context.Context, _, _ string) error             { return nil }
func (r *cancelTeamRunRepo) BatchCreateOrchestrationSteps(_ context.Context, _ []biz.OrchestrationStep) error {
	return nil
}
func (r *cancelTeamRunRepo) ListOrchestrationSteps(_ context.Context, _, _ string, _ int) ([]biz.OrchestrationStep, error) {
	return nil, nil
}
func (r *cancelTeamRunRepo) CreateTaskDeadLetter(_ context.Context, _ biz.TaskDeadLetter) error { return nil }
func (r *cancelTeamRunRepo) ListTaskDeadLetters(_ context.Context, _ biz.TaskDeadLetterListFilter) ([]biz.TaskDeadLetter, error) {
	return nil, nil
}
func (r *cancelTeamRunRepo) ResolveTaskDeadLetter(_ context.Context, _ string) (biz.TaskDeadLetter, error) {
	return biz.TaskDeadLetter{}, nil
}
func (r *cancelTeamRunRepo) CreateTeamRunStep(_ context.Context, step biz.TeamRunStep) (biz.TeamRunStep, error) {
	return step, nil
}
func (r *cancelTeamRunRepo) ListBySpiritSessionID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}

func TestCancelTeamRun_PublishesCancelledRunStatus(t *testing.T) {
	bus := event.NewBus(nil)
	ch, unsub := bus.Subscribe(event.SubscribeOptions{BufferSize: 4})
	defer unsub()

	reg := rt.NewRunRegistry()
	reg.SetStatus("sess-team-1", "run-team-1", biz.TeamRunStatusRunning, "")
	reg.StoreCancelable("sess-team-1", "run-team-1", func() {})

	repo := &cancelTeamRunRepo{runs: map[string]biz.TeamRun{
		"tr-1": {ID: "tr-1", SessionID: "sess-team-1", Status: biz.TeamRunStatusRunning},
	}}
	svc := NewTeamService(biz.NewTeamUsecase(repo, repo, repo, repo, repo, repo, nil), nil, nil, nil, nil, reg, bus, loggateway.NewNoop())

	resp, err := svc.CancelTeamRun(context.Background(), &v1.CancelTeamRunRequest{Id: "tr-1"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetStatus() != biz.TeamRunStatusCancelled {
		t.Fatalf("status=%q", resp.GetStatus())
	}

	select {
	case env := <-ch:
		if env.Type != event.EnvelopeTypeRunStatus {
			t.Fatalf("type=%s", env.Type)
		}
		if env.SessionID != "sess-team-1" {
			t.Fatalf("session=%q", env.SessionID)
		}
		if env.Metadata["status"] != biz.TeamRunStatusCancelled {
			t.Fatalf("status=%v", env.Metadata["status"])
		}
	default:
		t.Fatal("expected run_status envelope")
	}
}
