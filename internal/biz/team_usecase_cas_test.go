package biz

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// casTeamRepo is a stub that simulates CAS (Compare-And-Swap) behavior for
// concurrent status updates: only the first UpdateTeamWhereStatus call
// succeeds, subsequent calls return updated=false (simulating that the
// status no longer matches the expected value).
//
// Compile-time checks: casTeamRepo must satisfy TeamReader + TeamWriter +
// TeamRunReader + TeamRunWriter.
var (
	_ TeamReader    = (*casTeamRepo)(nil)
	_ TeamWriter    = (*casTeamRepo)(nil)
	_ TeamRunReader = (*casTeamRepo)(nil)
	_ TeamRunWriter = (*casTeamRepo)(nil)
)

type casTeamRepo struct {
	stubTeamReader
	stubTeamRunReader
	stubOrchestrationStepRepo
	stubTaskDeadLetterRepo

	mu             sync.Mutex
	team           Team
	teamRun        TeamRun
	updateTeamCnt  int32
	updateRunCnt   int32
	teamCASFail    bool // if true, second and subsequent CAS calls return false
	runCASFail     bool
}

func (r *casTeamRepo) GetTeamByID(_ context.Context, id string) (Team, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.team.ID != id {
		return Team{}, apierror.NotFound("TEAM", "team not found")
	}
	return r.team, nil
}

func (r *casTeamRepo) UpdateTeamWhereStatus(_ context.Context, id, newStatus, expectedCurrentStatus string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cnt := atomic.AddInt32(&r.updateTeamCnt, 1)
	if cnt > 1 && r.teamCASFail {
		return false, nil // CAS failed: status no longer matches
	}
	r.team.Status = newStatus
	return true, nil
}

// TeamWriter stubs (not used in CAS tests but required by interface)
func (r *casTeamRepo) CreateTeam(_ context.Context, t Team) (Team, error) { return t, nil }
func (r *casTeamRepo) UpdateTeam(_ context.Context, t Team) (Team, error) { return t, nil }
func (r *casTeamRepo) DeleteTeam(_ context.Context, _ string) error       { return nil }
func (r *casTeamRepo) BatchArchiveTeams(_ context.Context, _ []string) (int, error) { return 0, nil }

func (r *casTeamRepo) GetTeamRunByID(_ context.Context, id string) (TeamRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.teamRun.ID != id {
		return TeamRun{}, apierror.NotFound("TEAM", "team run not found")
	}
	return r.teamRun, nil
}

func (r *casTeamRepo) UpdateTeamRunWhereStatus(_ context.Context, id, newStatus, expectedCurrentStatus string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cnt := atomic.AddInt32(&r.updateRunCnt, 1)
	if cnt > 1 && r.runCASFail {
		return false, nil // CAS failed: status no longer matches
	}
	r.teamRun.Status = newStatus
	return true, nil
}

// TeamRunWriter stubs (not used in CAS tests but required by interface)
func (r *casTeamRepo) CreateTeamRun(_ context.Context, run TeamRun) (TeamRun, error) { return run, nil }
func (r *casTeamRepo) UpdateTeamRun(_ context.Context, _ TeamRun) error              { return nil }
func (r *casTeamRepo) UpdateTeamRunGraphExecutionID(_ context.Context, _, _ string) error { return nil }
func (r *casTeamRepo) UpdateTeamRunTraceID(_ context.Context, _, _ string) error        { return nil }
func (r *casTeamRepo) UpdateTeamRunSummaryJSON(_ context.Context, _, _ string) error    { return nil }
func (r *casTeamRepo) CreateTeamRunStep(_ context.Context, s TeamRunStep) (TeamRunStep, error) { return s, nil }

// TestTeamUsecase_TransitionStatus_CAS_ConcurrentReject verifies that when
// two concurrent TransitionStatus calls try to transition the same team,
// only one succeeds and the other is rejected (P2-15).
//
// The rejection may come from either:
// - CAS failure (Conflict): the goroutine read the old status but CAS fails
// - State machine validation (BadRequest): the goroutine reads the updated
//   status and the transition is invalid
// Both outcomes are acceptable — the key invariant is that exactly one
// transition succeeds.
func TestTeamUsecase_TransitionStatus_CAS_ConcurrentReject(t *testing.T) {
	t.Parallel()
	repo := &casTeamRepo{
		team: Team{ID: "team-1", Status: TeamStatusPending},
		teamCASFail: true,
	}
	uc := NewTeamUsecase(TeamUsecaseOpts{
		Reader:      repo,
		Writer:      repo,
		RunReader:   repo,
		RunWriter:   repo,
		StepRepo:    repo,
		DeadLetter:  repo,
		Lg:          loggateway.NewNoop(),
	})

	var wg sync.WaitGroup
	var successCnt, rejectCnt int32

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := uc.TransitionStatus(context.Background(), "team-1", TeamStatusRunning)
			if err == nil {
				atomic.AddInt32(&successCnt, 1)
			} else {
				atomic.AddInt32(&rejectCnt, 1)
			}
		}()
	}
	wg.Wait()

	if successCnt != 1 {
		t.Fatalf("expected exactly 1 successful transition, got %d", successCnt)
	}
	if rejectCnt != 1 {
		t.Fatalf("expected exactly 1 rejection, got %d", rejectCnt)
	}
}

// TestTeamUsecase_TransitionRunStatus_CAS_ConcurrentReject verifies that
// concurrent TransitionRunStatus calls are protected by CAS (P2-15).
func TestTeamUsecase_TransitionRunStatus_CAS_ConcurrentReject(t *testing.T) {
	t.Parallel()
	repo := &casTeamRepo{
		teamRun: TeamRun{ID: "run-1", Status: TeamRunStatusRunning},
		runCASFail: true,
	}
	uc := NewTeamUsecase(TeamUsecaseOpts{
		Reader:      repo,
		Writer:      repo,
		RunReader:   repo,
		RunWriter:   repo,
		StepRepo:    repo,
		DeadLetter:  repo,
		Lg:          loggateway.NewNoop(),
	})

	var wg sync.WaitGroup
	var successCnt, rejectCnt int32

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := uc.TransitionRunStatus(context.Background(), "run-1", TeamRunStatusSuccess)
			if err == nil {
				atomic.AddInt32(&successCnt, 1)
			} else {
				atomic.AddInt32(&rejectCnt, 1)
			}
		}()
	}
	wg.Wait()

	if successCnt != 1 {
		t.Fatalf("expected exactly 1 successful transition, got %d", successCnt)
	}
	if rejectCnt != 1 {
		t.Fatalf("expected exactly 1 rejection, got %d", rejectCnt)
	}
}

// TestTeamUsecase_TransitionStatus_CAS_SequentialSuccess verifies that
// sequential CAS transitions work correctly (no false conflicts).
func TestTeamUsecase_TransitionStatus_CAS_SequentialSuccess(t *testing.T) {
	t.Parallel()
	repo := &casTeamRepo{
		team: Team{ID: "team-1", Status: TeamStatusPending},
	}
	uc := NewTeamUsecase(TeamUsecaseOpts{
		Reader:      repo,
		Writer:      repo,
		RunReader:   repo,
		RunWriter:   repo,
		StepRepo:    repo,
		DeadLetter:  repo,
		Lg:          loggateway.NewNoop(),
	})

	// pending → running
	_, err := uc.TransitionStatus(context.Background(), "team-1", TeamStatusRunning)
	if err != nil {
		t.Fatalf("first transition failed: %v", err)
	}

	// running → completed
	_, err = uc.TransitionStatus(context.Background(), "team-1", TeamStatusCompleted)
	if err != nil {
		t.Fatalf("second transition failed: %v", err)
	}
}
