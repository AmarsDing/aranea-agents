package biz

import (
	"context"
	"fmt"
	"testing"

	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ---------------------------------------------------------------------------
// Mock repos for SpiritTeamUsecase tests
// ---------------------------------------------------------------------------

type memSpiritTeamRepo struct {
	items map[string]Team
}

func newMemSpiritTeamRepo() *memSpiritTeamRepo {
	return &memSpiritTeamRepo{items: make(map[string]Team)}
}

func (m *memSpiritTeamRepo) ListTeams(_ context.Context) ([]Team, error) {
	out := make([]Team, 0, len(m.items))
	for _, t := range m.items {
		out = append(out, t)
	}
	return out, nil
}
func (m *memSpiritTeamRepo) ListTeamsByStatus(_ context.Context, status string) ([]Team, error) {
	var out []Team
	for _, t := range m.items {
		if t.Status == status {
			out = append(out, t)
		}
	}
	return out, nil
}
func (m *memSpiritTeamRepo) GetTeamByID(_ context.Context, id string) (Team, error) {
	t, ok := m.items[id]
	if !ok {
		return Team{}, fmt.Errorf("not found: %s", id)
	}
	return t, nil
}
func (m *memSpiritTeamRepo) CreateTeam(_ context.Context, t Team) (Team, error) {
	if t.ID == "" {
		t.ID = fmt.Sprintf("tid-%d", len(m.items)+1)
	}
	m.items[t.ID] = t
	return t, nil
}
func (m *memSpiritTeamRepo) UpdateTeam(_ context.Context, t Team) (Team, error) {
	m.items[t.ID] = t
	return t, nil
}
func (m *memSpiritTeamRepo) DeleteTeam(_ context.Context, id string) error {
	delete(m.items, id)
	return nil
}
func (m *memSpiritTeamRepo) ListBySpiritSessionID(_ context.Context, spiritSessionID string) ([]Team, error) {
	var out []Team
	for _, t := range m.items {
		if t.SpiritSessionID == spiritSessionID {
			out = append(out, t)
		}
	}
	return out, nil
}
func (m *memSpiritTeamRepo) ListTeamRuns(_ context.Context, _ string, _ int) ([]TeamRun, error) {
	return nil, nil
}
func (m *memSpiritTeamRepo) HasActiveTeamRun(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (m *memSpiritTeamRepo) GetTeamRunByID(_ context.Context, id string) (TeamRun, error) {
	return TeamRun{}, fmt.Errorf("team run not found: %s", id)
}
func (m *memSpiritTeamRepo) ListTeamRunSteps(_ context.Context, _ string) ([]TeamRunStep, error) {
	return nil, nil
}
func (m *memSpiritTeamRepo) CreateTeamRun(_ context.Context, r TeamRun) (TeamRun, error) {
	return r, nil
}
func (m *memSpiritTeamRepo) UpdateTeamRun(_ context.Context, _ TeamRun) error          { return nil }
func (m *memSpiritTeamRepo) UpdateTeamRunSummaryJSON(_ context.Context, _, _ string) error { return nil }
func (m *memSpiritTeamRepo) UpdateTeamRunGraphExecutionID(_ context.Context, _, _ string) error {
	return nil
}
func (m *memSpiritTeamRepo) UpdateTeamRunTraceID(_ context.Context, _, _ string) error { return nil }
func (m *memSpiritTeamRepo) BatchCreateOrchestrationSteps(_ context.Context, _ []OrchestrationStep) error {
	return nil
}
func (m *memSpiritTeamRepo) ListOrchestrationSteps(_ context.Context, _, _ string, _ int) ([]OrchestrationStep, error) {
	return nil, nil
}
func (m *memSpiritTeamRepo) CreateTaskDeadLetter(_ context.Context, _ TaskDeadLetter) error {
	return nil
}
func (m *memSpiritTeamRepo) ListTaskDeadLetters(_ context.Context, _ TaskDeadLetterListFilter) ([]TaskDeadLetter, error) {
	return nil, nil
}
func (m *memSpiritTeamRepo) ResolveTaskDeadLetter(_ context.Context, _ string) (TaskDeadLetter, error) {
	return TaskDeadLetter{}, nil
}
func (m *memSpiritTeamRepo) CreateTeamRunStep(_ context.Context, s TeamRunStep) (TeamRunStep, error) {
	return s, nil
}

// memSpiritSessionRepo is a minimal in-memory session repo for SpiritTeamUsecase tests.
type memSpiritSessionRepo struct {
	items   map[string]Session
	failAll bool // if true, Create always fails (for rollback testing)
}

func newMemSpiritSessionRepo() *memSpiritSessionRepo {
	return &memSpiritSessionRepo{items: make(map[string]Session)}
}

func (m *memSpiritSessionRepo) Create(_ context.Context, in Session) (Session, error) {
	if m.failAll {
		return Session{}, kerrors.InternalServer("SESSION", "simulated failure")
	}
	if in.ID == "" {
		in.ID = fmt.Sprintf("sess-%d", len(m.items)+1)
	}
	m.items[in.ID] = in
	return in, nil
}
func (m *memSpiritSessionRepo) Get(_ context.Context, id string) (Session, error) {
	s, ok := m.items[id]
	if !ok {
		return Session{}, fmt.Errorf("not found: %s", id)
	}
	return s, nil
}
func (m *memSpiritSessionRepo) Update(_ context.Context, id string, patch SessionUpdateFields) (Session, error) {
	s, ok := m.items[id]
	if !ok {
		return Session{}, fmt.Errorf("not found: %s", id)
	}
	if patch.Status != nil {
		s.Status = *patch.Status
	}
	m.items[id] = s
	return s, nil
}
func (m *memSpiritSessionRepo) ListBySpiritSessionID(_ context.Context, _ string) ([]Session, error) {
	return nil, nil
}
func (m *memSpiritSessionRepo) Search(_ context.Context, q SessionSearchQuery) (SessionListResult, error) {
	var items []Session
	for _, s := range m.items {
		if q.TeamID != "" && s.TeamID != q.TeamID {
			continue
		}
		items = append(items, s)
	}
	if q.Limit > 0 && len(items) > q.Limit {
		items = items[:q.Limit]
	}
	return SessionListResult{Items: items, Total: len(items)}, nil
}
func (m *memSpiritSessionRepo) ListMessagesRecent(_ context.Context, _ string, _ int) ([]Message, error) {
	return nil, nil
}
func (m *memSpiritSessionRepo) TransitionStatus(_ context.Context, _ string, _ SessionStatus, _ string) error {
	return nil
}

// memSpiritTransactor tracks transaction calls for testing.
type memSpiritTransactor struct {
	callCount int
	fn        func(ctx context.Context) error // the actual function to execute
}

func (t *memSpiritTransactor) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	t.callCount++
	t.fn = fn
	return fn(ctx)
}

// ---------------------------------------------------------------------------
// T1.1: DAG root node initial status tests
// ---------------------------------------------------------------------------

func TestAssembleTeam_InitialStatus_Pending(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	sessionRepo := newMemSpiritSessionRepo()
	transactor := &memSpiritTransactor{}

	teamUC := NewTeamUsecase(teamRepo, nil)
	sessionUC := NewSessionUsecase(sessionRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	uc := NewSpiritTeamUsecase(teamUC, sessionUC, nil, transactor, loggateway.NewNoop())

	ctx := context.Background()

	// T1.1: AutoStart=false DAG root node should have initial status "pending"
	result, err := uc.AssembleTeam(ctx, SpiritTeamParams{
		SpiritSessionID: "spirit-1",
		TaskDescription: "test task",
		AgentKeys:       []string{"agent-1"},
		Mode:            "coordinator",
		AutoStart:       false,
	})
	if err != nil {
		t.Fatalf("AssembleTeam failed: %v", err)
	}
	if result.Team.Status != TeamStatusPending {
		t.Errorf("AutoStart=false team should have status pending, got %q", result.Team.Status)
	}

	// AutoStart=true team should also start as pending (transitions to running via StartTeamTurn)
	result2, err := uc.AssembleTeam(ctx, SpiritTeamParams{
		SpiritSessionID: "spirit-1",
		TaskDescription: "test task 2",
		AgentKeys:       []string{"agent-1"},
		Mode:            "coordinator",
		AutoStart:       true,
	})
	if err != nil {
		t.Fatalf("AssembleTeam (autoStart=true) failed: %v", err)
	}
	if result2.Team.Status != TeamStatusPending {
		t.Errorf("AutoStart=true team should initially have status pending, got %q", result2.Team.Status)
	}
}

func TestAssembleTeam_DAGDependentNode_InitialStatus_Pending(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	sessionRepo := newMemSpiritSessionRepo()
	transactor := &memSpiritTransactor{}

	teamUC := NewTeamUsecase(teamRepo, nil)
	sessionUC := NewSessionUsecase(sessionRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	uc := NewSpiritTeamUsecase(teamUC, sessionUC, nil, transactor, loggateway.NewNoop())

	ctx := context.Background()

	// DAG dependent node (has depends_on) should also have initial status "pending"
	result, err := uc.AssembleTeam(ctx, SpiritTeamParams{
		SpiritSessionID: "spirit-1",
		TaskDescription: "dependent task",
		AgentKeys:       []string{"agent-1"},
		Mode:            "coordinator",
		DagNodeID:       "node-2",
		DependsOn:       []string{"node-1"},
		AutoStart:       false,
	})
	if err != nil {
		t.Fatalf("AssembleTeam failed: %v", err)
	}
	if result.Team.Status != TeamStatusPending {
		t.Errorf("DAG dependent node should have status pending, got %q", result.Team.Status)
	}
	if len(result.Team.DependsOn) != 1 || result.Team.DependsOn[0] != "node-1" {
		t.Errorf("DAG dependent node should have depends_on=[node-1], got %v", result.Team.DependsOn)
	}
}

// ---------------------------------------------------------------------------
// T1.2: TransitionStatus state machine enforcement tests
// ---------------------------------------------------------------------------

func TestTransitionStatus_ValidTransitions(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	uc := NewTeamUsecase(teamRepo, nil)
	ctx := context.Background()

	// Create a team in pending status
	team, err := uc.Create(ctx, Team{TeamKey: "test", DisplayName: "Test"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// pending → running
	updated, err := uc.TransitionStatus(ctx, team.ID, TeamStatusRunning)
	if err != nil {
		t.Fatalf("pending → running should succeed: %v", err)
	}
	if updated.Status != TeamStatusRunning {
		t.Errorf("expected running, got %q", updated.Status)
	}

	// running → completed
	updated, err = uc.TransitionStatus(ctx, team.ID, TeamStatusCompleted)
	if err != nil {
		t.Fatalf("running → completed should succeed: %v", err)
	}
	if updated.Status != TeamStatusCompleted {
		t.Errorf("expected completed, got %q", updated.Status)
	}
}

func TestTransitionStatus_InvalidTransitions(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	uc := NewTeamUsecase(teamRepo, nil)
	ctx := context.Background()

	// Create a team in pending status
	team, err := uc.Create(ctx, Team{TeamKey: "test2", DisplayName: "Test2"})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// pending → completed (invalid: cannot skip running)
	_, err = uc.TransitionStatus(ctx, team.ID, TeamStatusCompleted)
	if err == nil {
		t.Error("pending → completed should fail")
	}

	// pending → failed (invalid: cannot skip running)
	_, err = uc.TransitionStatus(ctx, team.ID, TeamStatusFailed)
	if err == nil {
		t.Error("pending → failed should fail")
	}

	// pending → interrupted (invalid)
	_, err = uc.TransitionStatus(ctx, team.ID, TeamStatusInterrupted)
	if err == nil {
		t.Error("pending → interrupted should fail")
	}

	// Transition to running first
	_, _ = uc.TransitionStatus(ctx, team.ID, TeamStatusRunning)

	// running → pending (invalid: cannot go back)
	_, err = uc.TransitionStatus(ctx, team.ID, TeamStatusPending)
	if err == nil {
		t.Error("running → pending should fail")
	}

	// Transition to completed
	_, _ = uc.TransitionStatus(ctx, team.ID, TeamStatusCompleted)

	// completed → running (invalid: terminal state)
	_, err = uc.TransitionStatus(ctx, team.ID, TeamStatusRunning)
	if err == nil {
		t.Error("completed → running should fail (terminal state)")
	}
}

func TestTransitionStatus_InterruptedRecovery(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	uc := NewTeamUsecase(teamRepo, nil)
	ctx := context.Background()

	team, _ := uc.Create(ctx, Team{TeamKey: "recovery", DisplayName: "Recovery"})
	_, _ = uc.TransitionStatus(ctx, team.ID, TeamStatusRunning)
	_, _ = uc.TransitionStatus(ctx, team.ID, TeamStatusInterrupted)

	// interrupted → running (recovery)
	updated, err := uc.TransitionStatus(ctx, team.ID, TeamStatusRunning)
	if err != nil {
		t.Fatalf("interrupted → running should succeed: %v", err)
	}
	if updated.Status != TeamStatusRunning {
		t.Errorf("expected running after recovery, got %q", updated.Status)
	}

	// interrupted → completed (invalid)
	_, _ = uc.TransitionStatus(ctx, team.ID, TeamStatusInterrupted) // back to interrupted
	_, err = uc.TransitionStatus(ctx, team.ID, TeamStatusCompleted)
	if err == nil {
		t.Error("interrupted → completed should fail (must go through running)")
	}
}

func TestCancelTeam_UsesTransitionStatus(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	sessionRepo := newMemSpiritSessionRepo()
	transactor := &memSpiritTransactor{}

	teamUC := NewTeamUsecase(teamRepo, nil)
	sessionUC := NewSessionUsecase(sessionRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	uc := NewSpiritTeamUsecase(teamUC, sessionUC, nil, transactor, loggateway.NewNoop())

	ctx := context.Background()

	// Create a pending team
	result, _ := uc.AssembleTeam(ctx, SpiritTeamParams{
		SpiritSessionID: "spirit-cancel",
		TaskDescription: "cancel test",
		AgentKeys:       []string{"agent-1"},
		Mode:            "coordinator",
		AutoStart:       false,
	})

	// Cancel should succeed (pending → cancelled is valid)
	err := uc.CancelTeam(ctx, result.Team.ID)
	if err != nil {
		t.Fatalf("CancelTeam should succeed for pending team: %v", err)
	}

	// Verify status is cancelled
	team, _ := teamUC.Get(ctx, result.Team.ID)
	if team.Status != TeamStatusCancelled {
		t.Errorf("expected cancelled, got %q", team.Status)
	}

	// Cancel again should fail (cancelled → cancelled is same-state, which is allowed)
	// Actually, same-state transitions are allowed by ValidTeamStatusTransition
	// But let's test that cancelling a completed team fails
	team2, _ := uc.AssembleTeam(ctx, SpiritTeamParams{
		SpiritSessionID: "spirit-cancel2",
		TaskDescription: "cancel test 2",
		AgentKeys:       []string{"agent-1"},
		Mode:            "coordinator",
		AutoStart:       false,
	})
	_, _ = teamUC.TransitionStatus(ctx, team2.Team.ID, TeamStatusRunning)
	_, _ = teamUC.TransitionStatus(ctx, team2.Team.ID, TeamStatusCompleted)

	err = uc.CancelTeam(ctx, team2.Team.ID)
	if err == nil {
		t.Error("CancelTeam should fail for completed team (terminal state)")
	}
}

// ---------------------------------------------------------------------------
// T1.3: Team+Session joint transaction tests
// ---------------------------------------------------------------------------

func TestAssembleTeam_TransactionRollback(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	sessionRepo := newMemSpiritSessionRepo()
	transactor := &memSpiritTransactor{}

	teamUC := NewTeamUsecase(teamRepo, nil)
	sessionUC := NewSessionUsecase(sessionRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	uc := NewSpiritTeamUsecase(teamUC, sessionUC, nil, transactor, loggateway.NewNoop())

	ctx := context.Background()

	// Count teams before
	teamsBefore, _ := teamRepo.ListTeams(ctx)
	countBefore := len(teamsBefore)

	// Make session creation fail
	sessionRepo.failAll = true

	// AssembleTeam should fail because session creation fails
	_, err := uc.AssembleTeam(ctx, SpiritTeamParams{
		SpiritSessionID: "spirit-tx",
		TaskDescription: "tx test",
		AgentKeys:       []string{"agent-1"},
		Mode:            "coordinator",
		AutoStart:       false,
	})
	if err == nil {
		t.Fatal("AssembleTeam should fail when session creation fails")
	}

	// Verify transaction was used
	if transactor.callCount != 1 {
		t.Errorf("expected 1 transaction call, got %d", transactor.callCount)
	}

	// Verify team was NOT persisted (rolled back)
	// Note: In the in-memory mock, the transaction callback runs the function
	// and if it returns an error, the result is not stored in the outer scope.
	// The real Ent transaction would roll back the DB writes.
	// Here we verify the result variable was not set (which simulates rollback).
	teamsAfter, _ := teamRepo.ListTeams(ctx)
	if len(teamsAfter) != countBefore {
		t.Errorf("team count should not change after rollback: before=%d, after=%d", countBefore, len(teamsAfter))
	}
}

func TestAssembleTeam_TransactionSuccess(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	sessionRepo := newMemSpiritSessionRepo()
	transactor := &memSpiritTransactor{}

	teamUC := NewTeamUsecase(teamRepo, nil)
	sessionUC := NewSessionUsecase(sessionRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	uc := NewSpiritTeamUsecase(teamUC, sessionUC, nil, transactor, loggateway.NewNoop())

	ctx := context.Background()

	// Normal case: both team and session should be created
	result, err := uc.AssembleTeam(ctx, SpiritTeamParams{
		SpiritSessionID: "spirit-tx-ok",
		TaskDescription: "tx success test",
		AgentKeys:       []string{"agent-1"},
		Mode:            "coordinator",
		AutoStart:       false,
	})
	if err != nil {
		t.Fatalf("AssembleTeam should succeed: %v", err)
	}

	// Verify team was created
	if result.Team.ID == "" {
		t.Error("team ID should not be empty")
	}
	if result.Team.Status != TeamStatusPending {
		t.Errorf("team status should be pending, got %q", result.Team.Status)
	}

	// Verify session was created
	if result.Session.ID == "" {
		t.Error("session ID should not be empty")
	}
	if result.Session.TeamID != result.Team.ID {
		t.Errorf("session TeamID should match team ID: session.TeamID=%q, team.ID=%q", result.Session.TeamID, result.Team.ID)
	}
	if result.Session.OwnerType != "team" {
		t.Errorf("session OwnerType should be team, got %q", result.Session.OwnerType)
	}

	// Verify transaction was used
	if transactor.callCount != 1 {
		t.Errorf("expected 1 transaction call, got %d", transactor.callCount)
	}
}
