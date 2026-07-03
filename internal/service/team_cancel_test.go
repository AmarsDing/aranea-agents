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
	runs map[string]biz.TeamRunRecord
	// teamRunsByTeamID configures ListTeamRuns results per team_id.
	// Inject tests populate this; cancel/pause/unpause tests leave it nil.
	teamRunsByTeamID map[string][]biz.TeamRunRecord
	// teamByID configures GetTeamByID. Inject tests use this to satisfy the
	// "team exists" check; default (zero value) returns ErrNotFound.
	teamByID map[string]biz.Team
}

// TeamReader stubs
func (r *cancelTeamRunRepo) ListTeams(_ context.Context) ([]biz.Team, error) { return nil, nil }
func (r *cancelTeamRunRepo) ListTeamsByStatus(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (r *cancelTeamRunRepo) GetTeamByID(_ context.Context, id string) (biz.Team, error) {
	if t, ok := r.teamByID[id]; ok {
		return t, nil
	}
	return biz.Team{}, biz.ErrNotFound
}
func (r *cancelTeamRunRepo) GetTeamByKey(_ context.Context, _ string) (biz.Team, error) {
	return biz.Team{}, biz.ErrNotFound
}
func (r *cancelTeamRunRepo) ListBySpiritSessionID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (r *cancelTeamRunRepo) ListTeamsByDepartmentID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}

// TeamWriter stubs
func (r *cancelTeamRunRepo) CreateTeam(_ context.Context, t biz.Team) (biz.Team, error) {
	return t, nil
}
func (r *cancelTeamRunRepo) UpdateTeam(_ context.Context, t biz.Team) (biz.Team, error) {
	return t, nil
}
func (r *cancelTeamRunRepo) UpdateTeamWhereStatus(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}
func (r *cancelTeamRunRepo) DeleteTeam(_ context.Context, _ string) error { return nil }
func (r *cancelTeamRunRepo) BatchArchiveTeams(_ context.Context, _ []string) (int, error) {
	return 0, nil
}

// TeamRunReader
func (r *cancelTeamRunRepo) ListTeamRuns(_ context.Context, teamID string, _ int) ([]biz.TeamRunRecord, error) {
	if runs, ok := r.teamRunsByTeamID[teamID]; ok {
		return runs, nil
	}
	return nil, nil
}
func (r *cancelTeamRunRepo) ListTeamRunsByTeamIDs(_ context.Context, _ []string, _ int) (map[string][]biz.TeamRunRecord, error) {
	return nil, nil
}
func (r *cancelTeamRunRepo) GetTeamRunByID(_ context.Context, id string) (biz.TeamRunRecord, error) {
	run, ok := r.runs[id]
	if !ok {
		return biz.TeamRunRecord{}, fmt.Errorf("not found")
	}
	return run, nil
}
func (r *cancelTeamRunRepo) ListTeamRunSteps(_ context.Context, _ string) ([]biz.TeamRunStep, error) {
	return nil, nil
}
func (r *cancelTeamRunRepo) HasActiveTeamRun(_ context.Context, _ string) (bool, error) {
	return false, nil
}

// TeamRunWriter stubs
func (r *cancelTeamRunRepo) CreateTeamRun(_ context.Context, run biz.TeamRunRecord) (biz.TeamRunRecord, error) {
	return run, nil
}
func (r *cancelTeamRunRepo) UpdateTeamRun(_ context.Context, run biz.TeamRunRecord) error {
	r.runs[run.ID] = run
	return nil
}
func (r *cancelTeamRunRepo) UpdateTeamRunWhereStatus(_ context.Context, runID, newStatus, oldStatus string) (bool, error) {
	run, ok := r.runs[runID]
	if !ok {
		return false, nil
	}
	if run.Status != oldStatus {
		return false, nil
	}
	run.Status = newStatus
	r.runs[runID] = run
	return true, nil
}
func (r *cancelTeamRunRepo) UpdateTeamRunSummaryJSON(_ context.Context, _, _ string) error {
	return nil
}
func (r *cancelTeamRunRepo) UpdateTeamRunGraphExecutionID(_ context.Context, _, _ string) error {
	return nil
}
func (r *cancelTeamRunRepo) UpdateTeamRunTraceID(_ context.Context, _, _ string) error { return nil }
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
func (r *cancelTeamRunRepo) CreateTaskDeadLetter(_ context.Context, _ biz.TaskDeadLetter) error {
	return nil
}
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
	// enqueued records every EnqueueUserMessage call as {sessionID, message}.
	// Tests assert on this slice to verify InjectTeamMessage routing.
	enqueued []struct{ sessionID, message string }
	// enqueueAccept controls the bool returned by EnqueueUserMessage.
	// Defaults to true so InjectTeamMessage tests see accepted=true.
	enqueuedAccept bool
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

func (t *testRunRegistry) EnqueueUserMessage(sessionID, message string) (bool, error) {
	t.enqueued = append(t.enqueued, struct{ sessionID, message string }{sessionID, message})
	// Default to true when caller hasn't set enqueuedAccept (zero value is false,
	// so callers wanting true must set it; mirrors production semantics where
	// an active runner accepts the message).
	if t.enqueuedAccept {
		return true, nil
	}
	return false, nil
}

func TestCancelTeamRun_PublishesCancelledRunStatus(t *testing.T) {
	bus := event.NewV2Bus()
	ch, unsub := bus.Subscribe(biz.EventSubscribeOptions{})
	defer unsub()

	reg := &testRunRegistry{
		statuses: map[string]biz.RunStatusEntry{
			"sess-team-1": {RunID: "run-team-1", Status: biz.TeamRunStatusRunning},
		},
		cancelled: map[string]bool{},
	}

	repo := &cancelTeamRunRepo{runs: map[string]biz.TeamRunRecord{
		"tr-1": {ID: "tr-1", SessionID: "sess-team-1", Status: biz.TeamRunStatusRunning},
	}}
	svc := NewTeamService(biz.NewTeamUsecase(biz.TeamUsecaseOpts{Reader: repo, Writer: repo, RunReader: repo, RunWriter: repo, StepRepo: repo, DeadLetter: repo, Lg: loggateway.NewNoop()}), nil, nil, nil, nil, reg, bus, loggateway.NewNoop(), nil, nil, nil, nil)

	resp, err := svc.CancelTeamRun(context.Background(), &v1.CancelTeamRunRequest{Id: "tr-1"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetStatus() != biz.TeamRunStatusCancelled {
		t.Fatalf("status=%q", resp.GetStatus())
	}

	select {
	case ev := <-ch:
		rse, ok := ev.(*biz.RunStatusEvent)
		if !ok {
			t.Fatalf("expected *RunStatusEvent, got %T", ev)
		}
		if rse.Status != biz.TeamRunStatusCancelled {
			t.Fatalf("status=%s", rse.Status)
		}
		if rse.SpiritSessionID() != "sess-team-1" {
			t.Fatalf("session=%q", rse.SpiritSessionID())
		}
	default:
		t.Fatal("expected run_status event")
	}
}
