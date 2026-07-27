package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
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
func (s *stubTeamReader) ListTeams(_ context.Context) ([]biz.Team, error) { return nil, nil }
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
func (s *stubTeamReader) ListTeamsByWorkspace(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
}
func (s *stubTeamReader) CountTeamsByWorkspace(_ context.Context, _ string) (int, error) {
	return 0, nil
}

// stubTeamWriter implements biz.TeamWriter for testing.
type stubTeamWriter struct{}

func (s *stubTeamWriter) CreateTeam(_ context.Context, t biz.Team) (biz.Team, error) { return t, nil }
func (s *stubTeamWriter) UpdateTeam(_ context.Context, t biz.Team) (biz.Team, error) { return t, nil }
func (s *stubTeamWriter) DeleteTeam(_ context.Context, _ string) error               { return nil }
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
// completedResult customizes CheckAllTeamsCompleted; zero value = not done.
// hasRealDeliverable customizes the Fix-1 deliverable gate; zero value (false)
// means "no real deliverable" — completed callbacks get flipped to failed.
type stubSpiritTeamController struct {
	recordCompletionCalls   int
	scheduleDependentsCalls int
	completedResult         biz.AllTeamsCompletedResult
	hasRealDeliverable      bool
	hasRealDeliverableErr   error
	failureBriefs           []biz.TeamFailureBrief
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
func (s *stubSpiritTeamController) HasRealDeliverable(_ context.Context, _ biz.Team) (bool, error) {
	return s.hasRealDeliverable, s.hasRealDeliverableErr
}
func (s *stubSpiritTeamController) ListFailedTeamBriefs(_ context.Context, _ string) []biz.TeamFailureBrief {
	return s.failureBriefs
}
func (s *stubSpiritTeamController) CheckAllTeamsCompleted(_ context.Context, _ string) biz.AllTeamsCompletedResult {
	return s.completedResult
}
func (s *stubSpiritTeamController) GetParallelConfig(_ context.Context, _ string) biz.ParallelConfig {
	return biz.ParallelConfig{}
}
func (s *stubSpiritTeamController) AutoArchiveCompletedTeams(_ context.Context, _ string) {}
func (s *stubSpiritTeamController) ReadUpstreamDeliverable(_ context.Context, _, _ string, _ int) (biz.UpstreamDeliverableContent, error) {
	return biz.UpstreamDeliverableContent{}, nil
}

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
		sessions:   nil, // not needed for this path
		team:       TeamOrchestrationDeps{TeamUC: teamUC, SpiritUC: &stubSpiritTeamController{}},
		eventBus:   eventBus,
		lg:         loggateway.NewNoop(),
		teamStageR: stubTSReader,
		tsSM:       biz.NewTeamStageStateMachine(),
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
		team:       TeamOrchestrationDeps{TeamUC: teamUC, SpiritUC: &stubSpiritTeamController{}},
		eventBus:   eventBus,
		lg:         loggateway.NewNoop(),
		teamStageR: stubTSReader,
		tsSM:       biz.NewTeamStageStateMachine(),
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
	trID := agent.NewTeamRunV2ID(tsID)

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
		team: TeamOrchestrationDeps{TeamUC: teamUC},
		seq:  seq,
		lg:   loggateway.NewNoop(),
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

// ── 2026-07-25 Fix 1+2 真实产出闸门 ─────────────────────────────────────────
// 19:29 场景：DAG 团队只提问澄清、从未调用 set_deliverable，runner 却回调
// completed。闸门必须在状态转换前把团队翻转为 failed，下游经既有
// anyDepFailed 级联失败 —— 不允许「无交付物的成功」。

func newDeliverableGateStarter(team biz.Team, controller *stubSpiritTeamController, eventBus *capturingEventBus) *TeamStarter {
	tsID := "ts-" + team.ID
	teamUC := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader:    &stubTeamReader{teams: map[string]biz.Team{team.ID: team}},
		Writer:    &stubTeamWriter{},
		RunReader: &stubTeamRunReader{},
		Lg:        loggateway.NewNoop(),
	})
	return &TeamStarter{
		team:     TeamOrchestrationDeps{TeamUC: teamUC, SpiritUC: controller},
		eventBus: eventBus,
		lg:       loggateway.NewNoop(),
		teamStageR: &stubTeamStageV2Reader{stages: map[string]biz.TeamStage{
			tsID: {ID: tsID, TeamID: team.ID, SessionID: team.SpiritSessionID,
				Status: biz.TeamStageStatusRunning, Stage: biz.TeamStageStageExecuting, Version: 1},
		}},
		tsSM: biz.NewTeamStageStateMachine(),
	}
}

// completed 回调 + 无真实交付物 → 团队标 failed：不记录完成、级联调度一次
// （让下游检测依赖失败）、发布 TeamStageFailedEvent。
func TestHandleTeamTurnResult_CompletedWithoutRealDeliverable_MarkedFailed(t *testing.T) {
	team := biz.Team{
		ID: "team-nodeliv", DisplayName: "无产出团队", SpiritSessionID: "spirit-1",
		AutoCreated: true, Status: biz.TeamStatusRunning, DagNodeID: "st_1",
	}
	controller := &stubSpiritTeamController{hasRealDeliverable: false}
	eventBus := &capturingEventBus{}
	s := newDeliverableGateStarter(team, controller, eventBus)

	s.HandleTeamTurnResult(context.Background(), team.SpiritSessionID, team.ID, biz.TeamStatusCompleted, "", "")

	if controller.recordCompletionCalls != 0 {
		t.Errorf("RecordTeamCompletion called %d times, want 0 (no real deliverable)", controller.recordCompletionCalls)
	}
	if controller.scheduleDependentsCalls != 1 {
		t.Errorf("ScheduleDependentTeams called %d times, want 1 (cascade fail downstream)", controller.scheduleDependentsCalls)
	}
	var failedFound bool
	for _, ev := range eventBus.snapshot() {
		if tsEv, ok := ev.(*biz.TeamStageFailedEvent); ok && tsEv.TeamStage.TeamID == team.ID {
			failedFound = true
			if tsEv.TeamStage.Status != biz.TeamStageStatusFailed {
				t.Errorf("TeamStage event status = %s, want failed", tsEv.TeamStage.Status)
			}
		}
		if _, ok := ev.(*biz.TeamStageCompletedEvent); ok {
			t.Errorf("TeamStageCompletedEvent must not be published for a deliverable-less team")
		}
	}
	if !failedFound {
		t.Errorf("no TeamStageFailedEvent published, want exactly one for the flipped team")
	}
}

// completed 回调 + 有真实交付物 → 正常 completed 路径（记录完成 + completed 事件）。
func TestHandleTeamTurnResult_CompletedWithRealDeliverable_StaysCompleted(t *testing.T) {
	team := biz.Team{
		ID: "team-deliv", DisplayName: "有产出团队", SpiritSessionID: "spirit-1",
		AutoCreated: true, Status: biz.TeamStatusRunning, DagNodeID: "st_1",
	}
	controller := &stubSpiritTeamController{hasRealDeliverable: true}
	eventBus := &capturingEventBus{}
	s := newDeliverableGateStarter(team, controller, eventBus)

	s.HandleTeamTurnResult(context.Background(), team.SpiritSessionID, team.ID, biz.TeamStatusCompleted, "", "")

	if controller.recordCompletionCalls != 1 {
		t.Errorf("RecordTeamCompletion called %d times, want 1", controller.recordCompletionCalls)
	}
	if controller.scheduleDependentsCalls != 1 {
		t.Errorf("ScheduleDependentTeams called %d times, want 1 (activate downstream)", controller.scheduleDependentsCalls)
	}
	var completedFound bool
	for _, ev := range eventBus.snapshot() {
		if tsEv, ok := ev.(*biz.TeamStageCompletedEvent); ok && tsEv.TeamStage.TeamID == team.ID {
			completedFound = true
		}
		if _, ok := ev.(*biz.TeamStageFailedEvent); ok {
			t.Errorf("TeamStageFailedEvent must not be published for a team with a real deliverable")
		}
	}
	if !completedFound {
		t.Errorf("no TeamStageCompletedEvent published for a team with a real deliverable")
	}
}

// ── 2026-07-25 Fix 3 收尾报告诚实化 ─────────────────────────────────────────
// 19:29 根因链末环：存在失败团队时，注入 Spirit 会话的总结触发文本仍声称
// 「所有团队已完成」，LLM 据此幻觉出成功报告。触发文本必须由真实完成/失败
// 计数与失败简报动态构建；兜底通知同样不得谎报。

// capturingTurnGateway captures the TurnInput injected by checkAllTeamsCompleted.
type capturingTurnGateway struct {
	mu     sync.Mutex
	inputs []biz.TurnInput
	err    error
}

func (g *capturingTurnGateway) ExecuteTurn(_ context.Context, in biz.TurnInput) (biz.TurnResult, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.inputs = append(g.inputs, in)
	return biz.TurnResult{}, g.err
}
func (g *capturingTurnGateway) RunNativeTurn(_ context.Context, _ biz.TurnInput) (biz.ChatMessage, biz.ChatMessage, error) {
	return biz.ChatMessage{}, biz.ChatMessage{}, nil
}
func (g *capturingTurnGateway) RunNativeTurnWithOutcome(_ context.Context, _ biz.TurnInput) (biz.TurnResult, error) {
	return biz.TurnResult{}, nil
}
func (g *capturingTurnGateway) HasActiveRun(_ string) bool                        { return false }
func (g *capturingTurnGateway) CancelRun(_ context.Context, _ string) bool        { return false }
func (g *capturingTurnGateway) SetRunStatus(_ context.Context, _, _, _, _ string) {}
func (g *capturingTurnGateway) LastPendingMessageID(_ string) string              { return "" }

func (g *capturingTurnGateway) snapshot() []biz.TurnInput {
	g.mu.Lock()
	defer g.mu.Unlock()
	out := make([]biz.TurnInput, len(g.inputs))
	copy(out, g.inputs)
	return out
}

func newSynthesisStarter(controller *stubSpiritTeamController, gw *capturingTurnGateway, eventBus *capturingEventBus) *TeamStarter {
	return &TeamStarter{
		team:        TeamOrchestrationDeps{SpiritUC: controller},
		eventBus:    eventBus,
		turnGateway: gw,
		lg:          loggateway.NewNoop(),
	}
}

// 存在失败团队 → 注入的总结触发文本必须诚实：不得声称「所有团队已完成」，
// 必须携带失败团队简报（名称/原因/遗留疑问）。
func TestCheckAllTeamsCompleted_WithFailures_InjectsHonestTrigger(t *testing.T) {
	controller := &stubSpiritTeamController{
		completedResult: biz.AllTeamsCompletedResult{
			AllDone: true, TeamIDs: []string{"t1", "t2"},
			TotalTeams: 2, CompletedTeams: 1, FailedTeams: 1,
		},
		failureBriefs: []biz.TeamFailureBrief{{
			TeamName:  "数据采集团队",
			TaskName:  "采集竞品价格",
			Reason:    "团队未通过 set_deliverable 提交真实交付物",
			LastReply: "需要您澄清：目标竞品名单？",
		}},
	}
	gw := &capturingTurnGateway{}
	s := newSynthesisStarter(controller, gw, &capturingEventBus{})

	s.checkAllTeamsCompleted(context.Background(), "sp1")

	inputs := gw.snapshot()
	if len(inputs) != 1 {
		t.Fatalf("ExecuteTurn called %d times, want 1 (synthesis turn)", len(inputs))
	}
	content := inputs[0].Content
	if strings.Contains(content, "所有团队已完成") {
		t.Fatalf("synthesis trigger lies when failures exist, got:\n%s", content)
	}
	for _, want := range []string{"数据采集团队", "需要您澄清：目标竞品名单？", "## 未解决问题"} {
		if !strings.Contains(content, want) {
			t.Fatalf("honest trigger missing %q, got:\n%s", want, content)
		}
	}
}

// 全部完成 → 触发文本保留成功结构。
func TestCheckAllTeamsCompleted_AllCompleted_InjectsSuccessTrigger(t *testing.T) {
	controller := &stubSpiritTeamController{
		completedResult: biz.AllTeamsCompletedResult{
			AllDone: true, TeamIDs: []string{"t1"},
			TotalTeams: 1, CompletedTeams: 1,
		},
	}
	gw := &capturingTurnGateway{}
	s := newSynthesisStarter(controller, gw, &capturingEventBus{})

	s.checkAllTeamsCompleted(context.Background(), "sp1")

	inputs := gw.snapshot()
	if len(inputs) != 1 {
		t.Fatalf("ExecuteTurn called %d times, want 1", len(inputs))
	}
	content := inputs[0].Content
	if !strings.Contains(content, "所有团队已完成") {
		t.Fatalf("all-completed trigger should keep the success opening, got:\n%s", content)
	}
	if strings.Contains(content, "## 未解决问题") {
		t.Fatalf("success trigger must not demand an unresolved-questions section, got:\n%s", content)
	}
}

// ── 2026-07-27 修复3：ErrTurnMessageQueued 语义校正 ────────────────────────
// 总结触发文本被 steer 注入活跃 turn 或进入排队队列 = 成功受理（活跃 turn
// 结束后 processPendingQueue 保证排空执行），不是失败。旧行为把 queued 当
// 错误：发兜底通知（总结必将产出 → 双重信号）且 Warn 误报。
// 真实失败（消息未受理）必须释放 CAS 守卫 —— 烧掉守卫 = 30s 轮询路径永远
// 无法重试，本轮总结永久丢失。

// queued → 视为成功：不发兜底通知；CAS 保持占用（消息已受理，守卫继续
// 阻止轮询重复注入），第二次调用不再触发 ExecuteTurn。
func TestCheckAllTeamsCompleted_Queued_TreatedAsSuccess(t *testing.T) {
	controller := &stubSpiritTeamController{
		completedResult: biz.AllTeamsCompletedResult{
			AllDone: true, TeamIDs: []string{"t1"},
			TotalTeams: 1, CompletedTeams: 1,
		},
	}
	gw := &capturingTurnGateway{err: ErrTurnMessageQueued}
	eventBus := &capturingEventBus{}
	s := newSynthesisStarter(controller, gw, eventBus)

	s.checkAllTeamsCompleted(context.Background(), "sp1")

	if got := len(gw.snapshot()); got != 1 {
		t.Fatalf("ExecuteTurn called %d times, want 1", got)
	}
	for _, ev := range eventBus.snapshot() {
		if stepEv, ok := ev.(*biz.StepCreatedEvent); ok && stepEv.Step.Kind == biz.StepKindNotice {
			t.Fatalf("queued outcome must not publish fallback notice (synthesis will be produced by queue drain), got %q", stepEv.Step.Content)
		}
	}

	// CAS 保持：消息已受理，轮询/重复回调不得二次注入触发文本。
	s.checkAllTeamsCompleted(context.Background(), "sp1")
	if got := len(gw.snapshot()); got != 1 {
		t.Fatalf("after accepted-queued, CAS guard must stay burned: ExecuteTurn called %d times, want 1", got)
	}
}

// 真实失败（消息未受理）→ 释放 CAS：30s 轮询路径可重试总结 turn；
// 兜底通知仍发布（本轮 UX 终态信号）。
func TestCheckAllTeamsCompleted_TurnFails_ReleasesCASForRetry(t *testing.T) {
	controller := &stubSpiritTeamController{
		completedResult: biz.AllTeamsCompletedResult{
			AllDone: true, TeamIDs: []string{"t1"},
			TotalTeams: 1, CompletedTeams: 1,
		},
	}
	gw := &capturingTurnGateway{err: errors.New("turn gateway down")}
	eventBus := &capturingEventBus{}
	s := newSynthesisStarter(controller, gw, eventBus)

	s.checkAllTeamsCompleted(context.Background(), "sp1")
	if got := len(gw.snapshot()); got != 1 {
		t.Fatalf("ExecuteTurn called %d times, want 1", got)
	}
	var notices int
	for _, ev := range eventBus.snapshot() {
		if stepEv, ok := ev.(*biz.StepCreatedEvent); ok && stepEv.Step.Kind == biz.StepKindNotice {
			notices++
		}
	}
	if notices != 1 {
		t.Fatalf("real failure must publish exactly one fallback notice, got %d", notices)
	}

	// CAS 已释放 → 轮询重试再次触发 ExecuteTurn。
	s.checkAllTeamsCompleted(context.Background(), "sp1")
	if got := len(gw.snapshot()); got != 2 {
		t.Fatalf("after real failure, CAS guard must be released for poller retry: ExecuteTurn called %d times, want 2", got)
	}
}

// 总结 turn 触发失败 → 兜底通知同样诚实：warning 级别 + 真实完成/失败计数，
// 不得发布「所有团队已完成」成功通知。
func TestCheckAllTeamsCompleted_TurnFails_PublishesHonestFallbackNotice(t *testing.T) {
	controller := &stubSpiritTeamController{
		completedResult: biz.AllTeamsCompletedResult{
			AllDone: true, TeamIDs: []string{"t1", "t2"},
			TotalTeams: 2, CompletedTeams: 1, FailedTeams: 1,
		},
	}
	gw := &capturingTurnGateway{err: errors.New("turn gateway down")}
	eventBus := &capturingEventBus{}
	s := newSynthesisStarter(controller, gw, eventBus)

	s.checkAllTeamsCompleted(context.Background(), "sp1")

	var notice *biz.Step
	for _, ev := range eventBus.snapshot() {
		if stepEv, ok := ev.(*biz.StepCreatedEvent); ok && stepEv.Step.Kind == biz.StepKindNotice {
			s := stepEv.Step
			notice = &s
		}
	}
	if notice == nil {
		t.Fatal("no fallback notice published when the synthesis turn fails")
	}
	if strings.Contains(notice.Content, "所有团队已完成") {
		t.Fatalf("fallback notice lies when failures exist, got %q", notice.Content)
	}
	if !strings.Contains(notice.Content, "1") {
		t.Fatalf("fallback notice should carry truthful counts, got %q", notice.Content)
	}
	if notice.NoticeType != "warning" {
		t.Fatalf("fallback notice type = %q, want warning when failures exist", notice.NoticeType)
	}
}
