package biz

import (
	"context"
	"sync"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// orphanRecoveryRepo records run-level CAS calls so tests can assert which
// runs RecoverOrphanedRunningTeams attempted to fail.
type orphanRecoveryRepo struct {
	stubTeamReader
	stubTeamWriter
	stubTeamRunReader
	stubTeamRunWriter

	mu           sync.Mutex
	teams        []Team
	runs         []TeamRunRecord
	casRunStatus map[string]string
}

func (r *orphanRecoveryRepo) ListTeamsByStatus(_ context.Context, status string) ([]Team, error) {
	var out []Team
	for _, t := range r.teams {
		if t.Status == status {
			out = append(out, t)
		}
	}
	return out, nil
}

func (r *orphanRecoveryRepo) GetTeamByID(_ context.Context, id string) (Team, error) {
	for _, t := range r.teams {
		if t.ID == id {
			return t, nil
		}
	}
	return Team{}, ErrNotFound
}

func (r *orphanRecoveryRepo) UpdateTeamWhereStatus(_ context.Context, id, newStatus, _ string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.teams {
		if r.teams[i].ID == id {
			r.teams[i].Status = newStatus
			return true, nil
		}
	}
	return false, nil
}

func (r *orphanRecoveryRepo) UpdateTeam(_ context.Context, t Team) (Team, error) { return t, nil }

func (r *orphanRecoveryRepo) ListTeamRuns(_ context.Context, teamID string, _ int) ([]TeamRunRecord, error) {
	var out []TeamRunRecord
	for _, run := range r.runs {
		if run.TeamID == teamID {
			out = append(out, run)
		}
	}
	return out, nil
}

func (r *orphanRecoveryRepo) GetTeamRunByID(_ context.Context, id string) (TeamRunRecord, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, run := range r.runs {
		if run.ID == id {
			return run, nil
		}
	}
	return TeamRunRecord{}, ErrNotFound
}

func (r *orphanRecoveryRepo) UpdateTeamRunWhereStatus(_ context.Context, runID, newStatus, _ string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.casRunStatus == nil {
		r.casRunStatus = make(map[string]string)
	}
	r.casRunStatus[runID] = newStatus
	for i := range r.runs {
		if r.runs[i].ID == runID {
			r.runs[i].Status = newStatus
			return true, nil
		}
	}
	return false, nil
}

func (r *orphanRecoveryRepo) casStatusFor(runID string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.casRunStatus[runID]
	return s, ok
}

// TestRecoverOrphanedRunningTeams_SkipsWaitingHumanAndPausedRuns is the B2
// regression test: startup orphan recovery must only fail pending/running
// runs. waiting_human runs are owned by the graph-session HITL recovery
// channel (RecoverSessions + completion watch) and must NOT be force-failed,
// otherwise the human verdict after restart would be silently discarded.
func TestRecoverOrphanedRunningTeams_SkipsWaitingHumanAndPausedRuns(t *testing.T) {
	repo := &orphanRecoveryRepo{
		teams: []Team{{ID: "team-1", Status: TeamStatusRunning}},
		runs: []TeamRunRecord{
			{ID: "run-pending", TeamID: "team-1", Status: TeamRunStatusPending},
			{ID: "run-running", TeamID: "team-1", Status: TeamRunStatusRunning},
			{ID: "run-hitl", TeamID: "team-1", Status: TeamRunStatusWaitingHuman},
			{ID: "run-paused", TeamID: "team-1", Status: TeamRunStatusPaused},
			{ID: "run-done", TeamID: "team-1", Status: TeamRunStatusSuccess},
		},
	}
	uc := NewTeamUsecase(TeamUsecaseOpts{
		Reader:    repo,
		Writer:    repo,
		RunReader: repo,
		RunWriter: repo,
		Lg:        loggateway.NewNoop(),
	})

	recovered, err := uc.RecoverOrphanedRunningTeams(context.Background())
	if err != nil {
		t.Fatalf("RecoverOrphanedRunningTeams: unexpected error: %v", err)
	}
	if len(recovered) != 1 {
		t.Fatalf("recovered teams = %d, want 1", len(recovered))
	}

	if s, ok := repo.casStatusFor("run-pending"); !ok || s != TeamRunStatusCancelled {
		t.Errorf("run-pending: CAS = (%q, %v), want (%q, true)", s, ok, TeamRunStatusCancelled)
	}
	if s, ok := repo.casStatusFor("run-running"); !ok || s != TeamRunStatusFailed {
		t.Errorf("run-running: CAS = (%q, %v), want (%q, true)", s, ok, TeamRunStatusFailed)
	}
	for _, id := range []string{"run-hitl", "run-paused", "run-done"} {
		if s, ok := repo.casStatusFor(id); ok {
			t.Errorf("run %s (waiting_human/paused/terminal) must NOT be touched by orphan recovery, got CAS to %q", id, s)
		}
	}
}

type stubStartupResumeMarker map[string]bool

func (m stubStartupResumeMarker) WasStartupResumed(runID string) bool { return m[runID] }

// 83-长时运行韧性：team 的活跃 running run 已在启动对账中从 checkpoint
// 续跑成功（marker 命中）→ 整个 team 跳过判死：team 不 interrupted，run 不动。
func TestRecoverOrphanedRunningTeamsEx_SkipsStartupResumedTeam(t *testing.T) {
	repo := &orphanRecoveryRepo{
		teams: []Team{
			{ID: "team-resumed", Status: TeamStatusRunning},
			{ID: "team-orphan", Status: TeamStatusRunning},
		},
		runs: []TeamRunRecord{
			{ID: "run-resumed", TeamID: "team-resumed", Status: TeamRunStatusRunning},
			{ID: "run-orphan", TeamID: "team-orphan", Status: TeamRunStatusRunning},
		},
	}
	uc := NewTeamUsecase(TeamUsecaseOpts{
		Reader:    repo,
		Writer:    repo,
		RunReader: repo,
		RunWriter: repo,
		Lg:        loggateway.NewNoop(),
	})
	marker := stubStartupResumeMarker{"run-resumed": true}

	recovered, err := uc.RecoverOrphanedRunningTeamsEx(context.Background(), marker)
	if err != nil {
		t.Fatalf("RecoverOrphanedRunningTeamsEx: unexpected error: %v", err)
	}
	if len(recovered) != 1 || recovered[0].ID != "team-orphan" {
		t.Fatalf("recovered = %v, want only team-orphan", recovered)
	}
	// marker 命中：team-resumed 与其 run 均不被触碰。
	for _, tm := range repo.teams {
		if tm.ID == "team-resumed" && tm.Status != TeamStatusRunning {
			t.Errorf("team-resumed status = %q, want running untouched", tm.Status)
		}
	}
	if s, ok := repo.casStatusFor("run-resumed"); ok {
		t.Errorf("run-resumed must NOT be killed, got CAS to %q", s)
	}
	// 未命中：走旧判死路径。
	if s, ok := repo.casStatusFor("run-orphan"); !ok || s != TeamRunStatusFailed {
		t.Errorf("run-orphan: CAS = (%q, %v), want (%q, true)", s, ok, TeamRunStatusFailed)
	}
}

// 83-长时运行韧性：nil marker 退化为旧行为（等价 RecoverOrphanedRunningTeams）。
func TestRecoverOrphanedRunningTeamsEx_NilMarkerLegacy(t *testing.T) {
	repo := &orphanRecoveryRepo{
		teams: []Team{{ID: "team-1", Status: TeamStatusRunning}},
		runs:  []TeamRunRecord{{ID: "run-1", TeamID: "team-1", Status: TeamRunStatusRunning}},
	}
	uc := NewTeamUsecase(TeamUsecaseOpts{
		Reader:    repo,
		Writer:    repo,
		RunReader: repo,
		RunWriter: repo,
		Lg:        loggateway.NewNoop(),
	})
	if _, err := uc.RecoverOrphanedRunningTeamsEx(context.Background(), nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s, ok := repo.casStatusFor("run-1"); !ok || s != TeamRunStatusFailed {
		t.Errorf("run-1: CAS = (%q, %v), want (%q, true)", s, ok, TeamRunStatusFailed)
	}
}
