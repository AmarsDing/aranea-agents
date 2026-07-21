package service

import (
	"context"
	"sync"
	"testing"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// ── mocks ──────────────────────────────────────────────────────────────────

// stubTeamReader implements biz.TeamReader for testing.
type stubTeamReader struct {
	teams map[string]biz.Team
}

func (s *stubTeamReader) GetTeamByID(_ context.Context, id string) (biz.Team, error) {
	if t, ok := s.teams[id]; ok {
		return t, nil
	}
	return biz.Team{}, biz.ErrNotFound
}
func (s *stubTeamReader) ListTeams(_ context.Context) ([]biz.Team, error)         { return nil, nil }
func (s *stubTeamReader) ListTeamsByStatus(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (s *stubTeamReader) GetTeamByKey(_ context.Context, _ string) (biz.Team, error) {
	return biz.Team{}, biz.ErrNotFound
}
func (s *stubTeamReader) ListBySpiritSessionID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (s *stubTeamReader) ListTeamsByDepartmentID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}

// stubTeamWriter implements biz.TeamWriter for testing.
type stubTeamWriter struct{}

func (s *stubTeamWriter) CreateTeam(_ context.Context, t biz.Team) (biz.Team, error) { return t, nil }
func (s *stubTeamWriter) UpdateTeam(_ context.Context, t biz.Team) (biz.Team, error) { return t, nil }
func (s *stubTeamWriter) DeleteTeam(_ context.Context, _ string) error                { return nil }
func (s *stubTeamWriter) BatchArchiveTeams(_ context.Context, _ []string) (int, error) {
	return 0, nil
}
func (s *stubTeamWriter) UpdateTeamWhereStatus(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}

// stubTeamRunReader implements biz.TeamRunReader for testing.
type stubTeamRunReader struct{}

func (s *stubTeamRunReader) ListTeamRuns(_ context.Context, _ string, _ int) ([]biz.TeamRunRecord, error) {
	return nil, nil
}
func (s *stubTeamRunReader) ListTeamRunsByTeamIDs(_ context.Context, _ []string, _ int) (map[string][]biz.TeamRunRecord, error) {
	return nil, nil
}
func (s *stubTeamRunReader) HasActiveTeamRun(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (s *stubTeamRunReader) GetTeamRunByID(_ context.Context, _ string) (biz.TeamRunRecord, error) {
	return biz.TeamRunRecord{}, biz.ErrNotFound
}
func (s *stubTeamRunReader) ListTeamRunSteps(_ context.Context, _ string) ([]biz.TeamRunStep, error) {
	return nil, nil
}

// stubTeamStageV2Reader implements biz.TeamStageV2Reader for testing.
type stubTeamStageV2Reader struct {
	stages map[string]biz.TeamStage
}

func (s *stubTeamStageV2Reader) GetTeamStage(_ context.Context, id string) (biz.TeamStage, error) {
	if ts, ok := s.stages[id]; ok {
		return ts, nil
	}
	return biz.TeamStage{}, biz.ErrNotFound
}
func (s *stubTeamStageV2Reader) ListTeamStagesByTask(_ context.Context, _ string) ([]biz.TeamStage, error) {
	return nil, nil
}

// stubSpiritTeamController implements biz.SpiritTeamController for testing.
// Counters track side-effect invocations so tests can assert that stale
// callbacks do not trigger completion recording or dependent scheduling.
type stubSpiritTeamController struct {
	recordCompletionCalls   int
	scheduleDependentsCalls int
}

func (s *stubSpiritTeamController) CancelTimeoutTimer(_ string) {}
func (s *stubSpiritTeamController) RecordTeamCompletion(_ context.Context, _ biz.Team, _ int64) (float64, biz.TopologyType) {
	s.recordCompletionCalls++
	return 0, ""
}
func (s *stubSpiritTeamController) ScheduleDependentTeams(_ context.Context, _ string, _ biz.Team) []biz.DependentTeamAction {
	s.scheduleDependentsCalls++
	return nil
}
func (s *stubSpiritTeamController) CheckAllTeamsCompleted(_ context.Context, _ string) biz.AllTeamsCompletedResult {
	return biz.AllTeamsCompletedResult{AllDone: false}
}
func (s *stubSpiritTeamController) GetParallelConfig(_ context.Context, _ string) biz.ParallelConfig {
	return biz.ParallelConfig{}
}
func (s *stubSpiritTeamController) AutoArchiveCompletedTeams(_ context.Context, _ string) {}

// capturingEventBus captures published v2 events for assertion.
type capturingEventBus struct {
	mu     sync.Mutex
	events []biz.Event
}

func (b *capturingEventBus) Publish(_ context.Context, e biz.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.events = append(b.events, e)
}
func (b *capturingEventBus) Subscribe(_ biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return nil, func() {}
}
func (b *capturingEventBus) snapshot() []biz.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]biz.Event, len(b.events))
	copy(out, b.events)
	return out
}

// ── test ───────────────────────────────────────────────────────────────────

// TestHandleTeamTurnResult_TerminalEventVersion verifies that terminal
// TeamStage events (completed/failed/cancelled) published by
// HandleTeamTurnResult carry a non-zero Version so that UpsertTeamStage's
// VersionLT guard accepts them.
//
// Root cause: the terminal TeamStage was constructed without Version
// (Go zero value = 0), which is always rejected by the VersionLT(0) guard
// because existing records have version >= 1. This caused terminal status
// to never be persisted (51/51 rows stuck in 'running').
func TestHandleTeamTurnResult_TerminalEventVersion(t *testing.T) {
	teamID := "team-1"
	spiritSessionID := "spirit-sess-1"
	tsID := "ts-" + teamID // NewTeamStageActivityID generates this format

	// Pre-populate a Running TeamStage at Version=1 so resolveTeamStageUpdate
	// can compute Version=2 for the terminal transition.
	stubTSReader := &stubTeamStageV2Reader{stages: map[string]biz.TeamStage{
		tsID: {
			ID: tsID, TeamID: teamID, SessionID: spiritSessionID,
			Status: biz.TeamStageStatusRunning, Stage: biz.TeamStageStageExecuting,
			Version: 1,
		},
	}}

	team := biz.Team{
		ID: teamID, DisplayName: "测试团队", SpiritSessionID: spiritSessionID,
		AutoCreated: true, Status: biz.TeamStatusRunning,
	}

	teamUC := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader:    &stubTeamReader{teams: map[string]biz.Team{teamID: team}},
		Writer:    &stubTeamWriter{},
		RunReader: &stubTeamRunReader{},
		Lg:        loggateway.NewNoop(),
	})

	eventBus := &capturingEventBus{}

	s := &TeamStarter{
		sessions:  nil, // not needed for this path
		team:      TeamOrchestrationDeps{TeamUC: teamUC, SpiritUC: &stubSpiritTeamController{}},
		eventBus:  eventBus,
		lg:        loggateway.NewNoop(),
		teamStageR: stubTSReader,
		tsSM:      biz.NewTeamStageStateMachine(),
	}

	terminalCases := []struct {
		name       string
		status     string
		wantStatus biz.TeamStageStatus
		wantEvent  string // event type name for assertion
	}{
		// Note: "cancelled" case is tested separately to avoid the sessions.Search
		// dependency (HandleTeamTurnResult L358 calls s.sessions.Search for cancelled).
		{"completed", biz.TeamStatusCompleted, biz.TeamStageStatusCompleted, "*biz.TeamStageCompletedEvent"},
		{"failed", biz.TeamStatusFailed, biz.TeamStageStatusFailed, "*biz.TeamStageFailedEvent"},
	}

	for _, tc := range terminalCases {
		t.Run(tc.name, func(t *testing.T) {
			eventBus.mu.Lock()
			eventBus.events = nil
			eventBus.mu.Unlock()

			s.HandleTeamTurnResult(context.Background(), spiritSessionID, teamID, tc.status, "", "")

			events := eventBus.snapshot()
			// Find the terminal TeamStage event (not the progress or notice events).
			var found bool
			for _, ev := range events {
				var ts biz.TeamStage
				switch e := ev.(type) {
				case *biz.TeamStageCompletedEvent:
					ts = e.TeamStage
				case *biz.TeamStageFailedEvent:
					ts = e.TeamStage
				case *biz.TeamStageUpdatedEvent:
					ts = e.TeamStage
				default:
					continue
				}
				// Only check the primary terminal event for this team.
				if ts.TeamID != teamID {
					continue
				}
				found = true
				if ts.Version <= 0 {
					t.Errorf("terminal TeamStage event Version = %d, want > 0", ts.Version)
				}
				if ts.Status != tc.wantStatus {
					t.Errorf("terminal TeamStage event Status = %s, want %s", ts.Status, tc.wantStatus)
				}
				break
			}
			if !found {
				t.Errorf("no terminal TeamStage event found for team %s", teamID)
			}
		})
	}
}

// TestHandleTeamTurnResult_ProgressEventVersion verifies that the progress
// (Running) TeamStage event also carries a non-zero Version. This was already
// fixed by P1 #9d-2 but we add a regression guard.
func TestHandleTeamTurnResult_ProgressEventVersion(t *testing.T) {
	teamID := "team-2"
	spiritSessionID := "spirit-sess-2"
	tsID := "ts-" + teamID

	stubTSReader := &stubTeamStageV2Reader{stages: map[string]biz.TeamStage{
		tsID: {
			ID: tsID, TeamID: teamID, SessionID: spiritSessionID,
			Status: biz.TeamStageStatusPending, Stage: biz.TeamStageStageAssembled,
			Version: 1,
		},
	}}

	team := biz.Team{
		ID: teamID, DisplayName: "测试团队2", SpiritSessionID: spiritSessionID,
		AutoCreated: true, Status: biz.TeamStatusRunning,
	}

	teamUC := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader:    &stubTeamReader{teams: map[string]biz.Team{teamID: team}},
		Writer:    &stubTeamWriter{},
		RunReader: &stubTeamRunReader{},
		Lg:        loggateway.NewNoop(),
	})

	eventBus := &capturingEventBus{}

	s := &TeamStarter{
		team:      TeamOrchestrationDeps{TeamUC: teamUC, SpiritUC: &stubSpiritTeamController{}},
		eventBus:  eventBus,
		lg:        loggateway.NewNoop(),
		teamStageR: stubTSReader,
		tsSM:      biz.NewTeamStageStateMachine(),
	}

	s.HandleTeamTurnResult(context.Background(), spiritSessionID, teamID, biz.TeamStatusRunning, "", "")

	events := eventBus.snapshot()
	for _, ev := range events {
		tsEv, ok := ev.(*biz.TeamStageUpdatedEvent)
		if !ok {
			continue
		}
		ts := tsEv.TeamStage
		if ts.TeamID != teamID {
			continue
		}
		if ts.Version <= 0 {
			t.Errorf("progress TeamStage event Version = %d, want > 0", ts.Version)
		}
		return
	}
	t.Error("no progress TeamStageUpdatedEvent found")
}

// ── P0-2 取消竞态修复测试 ────────────────────────────────────────────────────

// stubTeamRunV2Reader implements biz.TeamRunV2Reader for testing.
type stubTeamRunV2Reader struct {
	runs map[string]biz.TeamRun
}

func (s *stubTeamRunV2Reader) GetTeamRun(_ context.Context, id string) (biz.TeamRun, error) {
	if tr, ok := s.runs[id]; ok {
		return tr, nil
	}
	return biz.TeamRun{}, biz.ErrNotFound
}
func (s *stubTeamRunV2Reader) ListTeamRunsByStage(_ context.Context, _ string) ([]biz.TeamRun, error) {
	return nil, nil
}

// capturingSeq implements rt.EventPublisher for testing.
type capturingSeq struct {
	mu     sync.Mutex
	events []biz.Event
}

func (c *capturingSeq) Publish(_ context.Context, e biz.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, e)
}
func (c *capturingSeq) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.events)
}

// TestResolveTeamStageUpdate_RejectsTerminalOverwrite verifies that when the
// current TeamStage is already in a terminal state (e.g. cancelled) and the
// state machine rejects the transition, resolveTeamStageUpdate reports
// ok=false so the caller skips publishing — instead of falling back to
// (fallbackStatus, Version=100), which would overwrite the terminal state.
//
// P0-2 root cause: a cancelled team whose runner keeps executing eventually
// calls HandleTeamTurnResult(completed); the fallback overwrote
// team_stages_v2 cancelled → completed.
func TestResolveTeamStageUpdate_RejectsTerminalOverwrite(t *testing.T) {
	reader := &stubTeamStageV2Reader{stages: map[string]biz.TeamStage{
		"ts-1": {ID: "ts-1", Status: biz.TeamStageStatusCancelled, Version: 3},
	}}
	sm := biz.NewTeamStageStateMachine()

	_, _, ok := resolveTeamStageUpdate(context.Background(), reader, sm, "ts-1",
		biz.TeamStageEventComplete, biz.TeamStageStatusCompleted, loggateway.NewNoop())
	if ok {
		t.Error("resolveTeamStageUpdate ok = true, want false when current is terminal (transition rejected)")
	}
}

// TestResolveTeamStageUpdate_ReadFailureFallsBack verifies the read-failure
// fallback is preserved: when the current TeamStage cannot be loaded (e.g.
// record does not exist yet), the caller still publishes with
// (fallbackStatus, Version=100).
func TestResolveTeamStageUpdate_ReadFailureFallsBack(t *testing.T) {
	reader := &stubTeamStageV2Reader{stages: map[string]biz.TeamStage{}}
	sm := biz.NewTeamStageStateMachine()

	status, version, ok := resolveTeamStageUpdate(context.Background(), reader, sm, "ts-missing",
		biz.TeamStageEventComplete, biz.TeamStageStatusCompleted, loggateway.NewNoop())
	if !ok {
		t.Error("resolveTeamStageUpdate ok = false, want true on read failure (fallback path)")
	}
	if status != biz.TeamStageStatusCompleted {
		t.Errorf("fallback status = %s, want %s", status, biz.TeamStageStatusCompleted)
	}
	if version != 100 {
		t.Errorf("fallback version = %d, want 100", version)
	}
}

// TestResolveTeamStageUpdate_ValidTransition verifies the happy path still
// returns (newStatus, current.Version+1, true).
func TestResolveTeamStageUpdate_ValidTransition(t *testing.T) {
	reader := &stubTeamStageV2Reader{stages: map[string]biz.TeamStage{
		"ts-1": {ID: "ts-1", Status: biz.TeamStageStatusRunning, Version: 1},
	}}
	sm := biz.NewTeamStageStateMachine()

	status, version, ok := resolveTeamStageUpdate(context.Background(), reader, sm, "ts-1",
		biz.TeamStageEventComplete, biz.TeamStageStatusCompleted, loggateway.NewNoop())
	if !ok {
		t.Error("resolveTeamStageUpdate ok = false, want true for valid transition")
	}
	if status != biz.TeamStageStatusCompleted {
		t.Errorf("status = %s, want %s", status, biz.TeamStageStatusCompleted)
	}
	if version != 2 {
		t.Errorf("version = %d, want 2", version)
	}
}

// TestPublishV2TeamRunCompletion_SkipsWhenCurrentTerminal verifies that when
// the persisted TeamRun is already terminal (e.g. cancelled), a stale
// completion callback does NOT publish a terminal event that would overwrite
// the cancelled state (team_runs_v2 cancelled → completed race).
func TestPublishV2TeamRunCompletion_SkipsWhenCurrentTerminal(t *testing.T) {
	teamID := "team-x"
	spiritSessionID := "spirit-1"
	tsID := string(agent.NewTeamStageActivityID(teamID))
	trID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.team_run.v2:"+tsID)).String()

	team := biz.Team{
		ID: teamID, DisplayName: "团队X", SpiritSessionID: spiritSessionID,
		AutoCreated: true, Status: biz.TeamStatusCancelled,
	}
	teamUC := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader:    &stubTeamReader{teams: map[string]biz.Team{teamID: team}},
		Writer:    &stubTeamWriter{},
		RunReader: &stubTeamRunReader{},
		Lg:        loggateway.NewNoop(),
	})

	seq := &capturingSeq{}
	s := &TeamStarter{
		team:     TeamOrchestrationDeps{TeamUC: teamUC},
		seq:      seq,
		lg:       loggateway.NewNoop(),
		teamRunR: &stubTeamRunV2Reader{runs: map[string]biz.TeamRun{
			trID: {ID: trID, TeamStageID: tsID, Status: biz.TeamRunV2StatusCancelled, Version: 5},
		}},
		trSM: biz.NewTeamRunV2StateMachine(),
	}

	// The pre-fix code publishes the event and then panics on the nil
	// sessions dependency further down; recover so the assertion below runs.
	func() {
		defer func() { _ = recover() }()
		s.publishV2TeamRunCompletion(context.Background(), spiritSessionID, teamID,
			biz.TeamStageStatusCompleted, biz.TeamStatusCompleted)
	}()

	if got := seq.count(); got != 0 {
		t.Errorf("publishV2TeamRunCompletion published %d events, want 0 when current TeamRun is terminal", got)
	}
}

// TestHandleTeamTurnResult_StaleCallbackAfterTerminalSkipped verifies that a
// stale runner callback (e.g. completed arriving after the team was
// cancelled) is discarded entirely: no TeamStage events, no completion
// recording, no dependent-team scheduling.
func TestHandleTeamTurnResult_StaleCallbackAfterTerminalSkipped(t *testing.T) {
	teamID := "team-stale"
	spiritSessionID := "spirit-stale"
	tsID := "ts-" + teamID

	controller := &stubSpiritTeamController{}
	team := biz.Team{
		ID: teamID, DisplayName: "已取消团队", SpiritSessionID: spiritSessionID,
		AutoCreated: true, Status: biz.TeamStatusCancelled,
	}
	teamUC := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader:    &stubTeamReader{teams: map[string]biz.Team{teamID: team}},
		Writer:    &stubTeamWriter{},
		RunReader: &stubTeamRunReader{},
		Lg:        loggateway.NewNoop(),
	})

	eventBus := &capturingEventBus{}
	s := &TeamStarter{
		team:     TeamOrchestrationDeps{TeamUC: teamUC, SpiritUC: controller},
		eventBus: eventBus,
		lg:       loggateway.NewNoop(),
		teamStageR: &stubTeamStageV2Reader{stages: map[string]biz.TeamStage{
			tsID: {ID: tsID, TeamID: teamID, Status: biz.TeamStageStatusCancelled, Version: 2},
		}},
		tsSM: biz.NewTeamStageStateMachine(),
	}

	// Stale "completed" callback after cancel.
	s.HandleTeamTurnResult(context.Background(), spiritSessionID, teamID, biz.TeamStatusCompleted, "", "")

	if got := len(eventBus.snapshot()); got != 0 {
		t.Errorf("stale callback published %d events, want 0", got)
	}
	if controller.recordCompletionCalls != 0 {
		t.Errorf("RecordTeamCompletion called %d times, want 0 for stale callback", controller.recordCompletionCalls)
	}
	if controller.scheduleDependentsCalls != 0 {
		t.Errorf("ScheduleDependentTeams called %d times, want 0 for stale callback", controller.scheduleDependentsCalls)
	}
}
