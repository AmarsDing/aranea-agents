package service

import (
	"context"
	"errors"
	"testing"

	v1 "aranea-agents/api/kratos/team/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/activityevent"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// newPauseTestService wires a TeamService with the cancelTeamRunRepo and
// testRunRegistry configured for pause/unpause/inject scenarios.
// Helper kept minimal — callers further mutate repo.runs before invoking.
func newPauseTestService(t *testing.T, repo *cancelTeamRunRepo, reg *testRunRegistry) (*TeamService, <-chan biz.ActivityEvent, func()) {
	t.Helper()
	bus := activityevent.New(nil, nil)
	ch, unsub := bus.Subscribe(biz.ActivityEventSubscribeOptions{BufferSize: 4, GlobalMode: true})
	uc := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader:     repo,
		Writer:     repo,
		RunReader:  repo,
		RunWriter:  repo,
		StepRepo:   repo,
		DeadLetter: repo,
		Lg:         loggateway.NewNoop(),
	})
	svc := NewTeamService(uc, nil, nil, nil, nil, reg, bus, loggateway.NewNoop(), nil, nil, nil)
	return svc, ch, unsub
}

func TestPauseTeamRun_RunningToPaused(t *testing.T) {
	repo := &cancelTeamRunRepo{runs: map[string]biz.TeamRunRecord{
		"tr-1": {ID: "tr-1", SessionID: "sess-team-1", Status: biz.TeamRunStatusRunning},
	}}
	reg := &testRunRegistry{
		statuses: map[string]biz.RunStatusEntry{
			"sess-team-1": {RunID: "run-1", Status: biz.TeamRunStatusRunning},
		},
		cancelled: map[string]bool{},
	}
	svc, ch, unsub := newPauseTestService(t, repo, reg)
	defer unsub()

	resp, err := svc.PauseTeamRun(context.Background(), &v1.PauseTeamRunRequest{Id: "tr-1"})
	if err != nil {
		t.Fatalf("PauseTeamRun err=%v", err)
	}
	if resp.GetRunId() != "tr-1" {
		t.Fatalf("run_id=%q want tr-1", resp.GetRunId())
	}
	if resp.GetStatus() != biz.TeamRunStatusPaused {
		t.Fatalf("status=%q want %q", resp.GetStatus(), biz.TeamRunStatusPaused)
	}

	// Active runner should have been cancelled.
	if !reg.cancelled["sess-team-1"] {
		t.Fatal("expected runner Cancel to be called for sess-team-1")
	}

	// run_status notice (paused) should be published.
	select {
	case ev := <-ch:
		if ev.Activity.Stage != "run_status" {
			t.Fatalf("stage=%q want run_status", ev.Activity.Stage)
		}
		if ev.Activity.SessionID != "sess-team-1" {
			t.Fatalf("session_id=%q want sess-team-1", ev.Activity.SessionID)
		}
		if ev.Activity.Meta["status"] != biz.TeamRunStatusPaused {
			t.Fatalf("meta.status=%v want %q", ev.Activity.Meta["status"], biz.TeamRunStatusPaused)
		}
	default:
		t.Fatal("expected run_status paused event")
	}
}

func TestPauseTeamRun_NotRunningRejected(t *testing.T) {
	repo := &cancelTeamRunRepo{runs: map[string]biz.TeamRunRecord{
		"tr-2": {ID: "tr-2", SessionID: "sess-team-2", Status: biz.TeamRunStatusSuccess},
	}}
	reg := &testRunRegistry{}
	svc, _, unsub := newPauseTestService(t, repo, reg)
	defer unsub()

	_, err := svc.PauseTeamRun(context.Background(), &v1.PauseTeamRunRequest{Id: "tr-2"})
	if err == nil {
		t.Fatal("expected Conflict error when pausing a completed run")
	}
	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected apierror.Error, got %T", err)
	}
	if apiErr.Code != apierror.CodeConflict {
		t.Fatalf("code=%v want %v", apiErr.Code, apierror.CodeConflict)
	}
}

func TestPauseTeamRun_EmptyIDRejected(t *testing.T) {
	repo := &cancelTeamRunRepo{}
	reg := &testRunRegistry{}
	svc, _, unsub := newPauseTestService(t, repo, reg)
	defer unsub()

	_, err := svc.PauseTeamRun(context.Background(), &v1.PauseTeamRunRequest{Id: "  "})
	if err == nil {
		t.Fatal("expected BadRequest error for empty id")
	}
	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.Code != apierror.CodeBadRequest {
		t.Fatalf("expected BadRequest, got %v", err)
	}
}

func TestUnpauseTeamRun_PausedToRunning(t *testing.T) {
	repo := &cancelTeamRunRepo{runs: map[string]biz.TeamRunRecord{
		"tr-3": {ID: "tr-3", SessionID: "sess-team-3", Status: biz.TeamRunStatusPaused},
	}}
	reg := &testRunRegistry{}
	svc, ch, unsub := newPauseTestService(t, repo, reg)
	defer unsub()

	resp, err := svc.UnpauseTeamRun(context.Background(), &v1.UnpauseTeamRunRequest{Id: "tr-3"})
	if err != nil {
		t.Fatalf("UnpauseTeamRun err=%v", err)
	}
	if resp.GetStatus() != biz.TeamRunStatusRunning {
		t.Fatalf("status=%q want %q", resp.GetStatus(), biz.TeamRunStatusRunning)
	}

	// run_status notice (running) should be published.
	select {
	case ev := <-ch:
		if ev.Activity.Stage != "run_status" {
			t.Fatalf("stage=%q want run_status", ev.Activity.Stage)
		}
		if ev.Activity.Meta["status"] != biz.TeamRunStatusRunning {
			t.Fatalf("meta.status=%v want %q", ev.Activity.Meta["status"], biz.TeamRunStatusRunning)
		}
	default:
		t.Fatal("expected run_status running event")
	}
}

func TestUnpauseTeamRun_NotPausedRejected(t *testing.T) {
	repo := &cancelTeamRunRepo{runs: map[string]biz.TeamRunRecord{
		"tr-4": {ID: "tr-4", SessionID: "sess-team-4", Status: biz.TeamRunStatusRunning},
	}}
	reg := &testRunRegistry{}
	svc, _, unsub := newPauseTestService(t, repo, reg)
	defer unsub()

	_, err := svc.UnpauseTeamRun(context.Background(), &v1.UnpauseTeamRunRequest{Id: "tr-4"})
	if err == nil {
		t.Fatal("expected Conflict error when unpausing a running run")
	}
	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.Code != apierror.CodeConflict {
		t.Fatalf("expected Conflict, got %v", err)
	}
}

func TestInjectTeamMessage_RoutesToActiveRunSession(t *testing.T) {
	repo := &cancelTeamRunRepo{
		teamByID: map[string]biz.Team{
			"team-A": {ID: "team-A"},
		},
		teamRunsByTeamID: map[string][]biz.TeamRunRecord{
			"team-A": {
				// Older completed run — should be skipped.
				{ID: "run-old", SessionID: "sess-old", Status: biz.TeamRunStatusSuccess, CreatedAt: "2026-01-01T00:00:00Z"},
				// Active running run — should be selected (newer timestamp).
				{ID: "run-active", SessionID: "sess-active", Status: biz.TeamRunStatusRunning, CreatedAt: "2026-06-01T00:00:00Z"},
			},
		},
	}
	reg := &testRunRegistry{enqueuedAccept: true}
	svc, _, unsub := newPauseTestService(t, repo, reg)
	defer unsub()

	resp, err := svc.InjectTeamMessage(context.Background(), &v1.InjectTeamMessageRequest{
		TeamId:  "team-A",
		Message: "please prioritize the second analysis",
	})
	if err != nil {
		t.Fatalf("InjectTeamMessage err=%v", err)
	}
	if !resp.GetAccepted() {
		t.Fatal("expected accepted=true")
	}
	if len(reg.enqueued) != 1 {
		t.Fatalf("enqueued calls=%d want 1", len(reg.enqueued))
	}
	if reg.enqueued[0].sessionID != "sess-active" {
		t.Fatalf("sessionID=%q want sess-active", reg.enqueued[0].sessionID)
	}
	if reg.enqueued[0].message != "please prioritize the second analysis" {
		t.Fatalf("message=%q want the injected text", reg.enqueued[0].message)
	}
}

func TestInjectTeamMessage_PrefersPausedRunWhenNoRunning(t *testing.T) {
	repo := &cancelTeamRunRepo{
		teamByID: map[string]biz.Team{"team-B": {ID: "team-B"}},
		teamRunsByTeamID: map[string][]biz.TeamRunRecord{
			"team-B": {
				{ID: "run-paused", SessionID: "sess-paused", Status: biz.TeamRunStatusPaused, CreatedAt: "2026-06-01T00:00:00Z"},
				// Newer but terminal — should be skipped.
				{ID: "run-failed", SessionID: "sess-failed", Status: biz.TeamRunStatusFailed, CreatedAt: "2026-06-02T00:00:00Z"},
			},
		},
	}
	reg := &testRunRegistry{enqueuedAccept: true}
	svc, _, unsub := newPauseTestService(t, repo, reg)
	defer unsub()

	resp, err := svc.InjectTeamMessage(context.Background(), &v1.InjectTeamMessageRequest{
		TeamId:  "team-B",
		Message: "resume with revised scope",
	})
	if err != nil {
		t.Fatalf("InjectTeamMessage err=%v", err)
	}
	if !resp.GetAccepted() {
		t.Fatal("expected accepted=true for paused run")
	}
	if len(reg.enqueued) != 1 || reg.enqueued[0].sessionID != "sess-paused" {
		t.Fatalf("enqueued=%+v want sess-paused", reg.enqueued)
	}
}

func TestInjectTeamMessage_NoActiveRunRejected(t *testing.T) {
	repo := &cancelTeamRunRepo{
		teamByID: map[string]biz.Team{"team-C": {ID: "team-C"}},
		teamRunsByTeamID: map[string][]biz.TeamRunRecord{
			"team-C": {
				{ID: "run-done", SessionID: "sess-done", Status: biz.TeamRunStatusSuccess, CreatedAt: "2026-01-01T00:00:00Z"},
			},
		},
	}
	reg := &testRunRegistry{}
	svc, _, unsub := newPauseTestService(t, repo, reg)
	defer unsub()

	_, err := svc.InjectTeamMessage(context.Background(), &v1.InjectTeamMessageRequest{
		TeamId:  "team-C",
		Message: "do something",
	})
	if err == nil {
		t.Fatal("expected Conflict error when no active/paused run exists")
	}
	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.Code != apierror.CodeConflict {
		t.Fatalf("expected Conflict, got %v", err)
	}
	if len(reg.enqueued) != 0 {
		t.Fatalf("enqueued=%+v want 0 calls", reg.enqueued)
	}
}

func TestInjectTeamMessage_EmptyMessageRejected(t *testing.T) {
	repo := &cancelTeamRunRepo{
		teamByID: map[string]biz.Team{"team-D": {ID: "team-D"}},
	}
	reg := &testRunRegistry{}
	svc, _, unsub := newPauseTestService(t, repo, reg)
	defer unsub()

	_, err := svc.InjectTeamMessage(context.Background(), &v1.InjectTeamMessageRequest{
		TeamId:  "team-D",
		Message: "   ",
	})
	if err == nil {
		t.Fatal("expected BadRequest for empty message")
	}
	var apiErr *apierror.Error
	if !errors.As(err, &apiErr) || apiErr.Code != apierror.CodeBadRequest {
		t.Fatalf("expected BadRequest, got %v", err)
	}
}
