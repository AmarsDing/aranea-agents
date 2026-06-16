package service

import (
	"context"
	"fmt"
	"testing"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

type cancelTeamRunRepo struct {
	runs map[string]biz.TeamRun
}

// TeamReader stubs
func (r *cancelTeamRunRepo) ListTeams(_ context.Context) ([]biz.Team, error)                     { return nil, nil }
func (r *cancelTeamRunRepo) ListTeamsByStatus(_ context.Context, _ string) ([]biz.Team, error)   { return nil, nil }
func (r *cancelTeamRunRepo) GetTeamByID(_ context.Context, _ string) (biz.Team, error) {
	return biz.Team{}, biz.ErrNotFound
}
func (r *cancelTeamRunRepo) GetTeamByKey(_ context.Context, _ string) (biz.Team, error)          { return biz.Team{}, biz.ErrNotFound }
func (r *cancelTeamRunRepo) ListBySpiritSessionID(_ context.Context, _ string) ([]biz.Team, error) { return nil, nil }
func (r *cancelTeamRunRepo) ListTeamsByDepartmentID(_ context.Context, _ string) ([]biz.Team, error) { return nil, nil }

// TeamWriter stubs
func (r *cancelTeamRunRepo) CreateTeam(_ context.Context, t biz.Team) (biz.Team, error) { return t, nil }
func (r *cancelTeamRunRepo) UpdateTeam(_ context.Context, t biz.Team) (biz.Team, error) { return t, nil }
func (r *cancelTeamRunRepo) DeleteTeam(_ context.Context, _ string) error                 { return nil }
func (r *cancelTeamRunRepo) BatchArchiveTeams(_ context.Context, _ []string) (int, error) { return 0, nil }

// TeamRunReader
func (r *cancelTeamRunRepo) ListTeamRuns(_ context.Context, _ string, _ int) ([]biz.TeamRun, error) {
	return nil, nil
}
func (r *cancelTeamRunRepo) ListTeamRunsByTeamIDs(_ context.Context, _ []string, _ int) (map[string][]biz.TeamRun, error) {
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
func (r *cancelTeamRunRepo) HasActiveTeamRun(_ context.Context, _ string) (bool, error) { return false, nil }

// TeamRunWriter stubs
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
func (r *cancelTeamRunRepo) CreateTeamRunStep(_ context.Context, step biz.TeamRunStep) (biz.TeamRunStep, error) {
	return step, nil
}

// OrchestrationStepRepo stubs
func (r *cancelTeamRunRepo) BatchCreateOrchestrationSteps(_ context.Context, _ []biz.OrchestrationStep) error {
	return nil
}
func (r *cancelTeamRunRepo) ListOrchestrationSteps(_ context.Context, _, _ string, _ int) ([]biz.OrchestrationStep, error) {
	return nil, nil
}

// TaskDeadLetterRepo stubs
func (r *cancelTeamRunRepo) CreateTaskDeadLetter(_ context.Context, _ biz.TaskDeadLetter) error { return nil }
func (r *cancelTeamRunRepo) ListTaskDeadLetters(_ context.Context, _ biz.TaskDeadLetterListFilter) ([]biz.TaskDeadLetter, error) {
	return nil, nil
}
func (r *cancelTeamRunRepo) ResolveTaskDeadLetter(_ context.Context, _ string) (biz.TaskDeadLetter, error) {
	return biz.TaskDeadLetter{}, nil
}

// testRunRegistry is a minimal stub implementing biz.RunRegistryPort for tests.
type testRunRegistry struct {
	statuses  map[string]biz.RunStatusEntry
	cancelled map[string]bool
}

func (t *testRunRegistry) Cancel(sessionID, _ string) (bool, string) {
	entry, ok := t.statuses[sessionID]
	if !ok {
		return false, ""
	}
	t.cancelled[sessionID] = true
	return true, entry.RunID
}

func (t *testRunRegistry) GetStatus(sessionID string) (biz.RunStatusEntry, bool) {
	entry, ok := t.statuses[sessionID]
	return entry, ok
}

func TestCancelTeamRun_PublishesCancelledRunStatus(t *testing.T) {
	bus := event.NewBus(nil)
	ch, unsub := bus.Subscribe(event.SubscribeOptions{BufferSize: 4})
	defer unsub()

	reg := &testRunRegistry{
		statuses: map[string]biz.RunStatusEntry{
			"sess-team-1": {RunID: "run-team-1", Status: biz.TeamRunStatusRunning},
		},
		cancelled: map[string]bool{},
	}

	repo := &cancelTeamRunRepo{runs: map[string]biz.TeamRun{
		"tr-1": {ID: "tr-1", SessionID: "sess-team-1", Status: biz.TeamRunStatusRunning},
	}}
	svc := NewTeamService(biz.NewTeamUsecase(biz.TeamUsecaseOpts{Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo, StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop()}), nil, nil, nil, nil, reg, bus, loggateway.NewNoop(), nil)

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
