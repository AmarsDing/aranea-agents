package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
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
// transitions records UpdateTeamWhereStatus calls so tests can assert that
// teams-table status transitions happened (or not) with the expected target.
type stubTeamWriter struct {
	mu          sync.Mutex
	transitions []stubTeamTransition
}

type stubTeamTransition struct {
	id, newStatus, expectCurrent string
}

func (s *stubTeamWriter) CreateTeam(_ context.Context, t biz.Team) (biz.Team, error) { return t, nil }
func (s *stubTeamWriter) UpdateTeam(_ context.Context, t biz.Team) (biz.Team, error) { return t, nil }
func (s *stubTeamWriter) DeleteTeam(_ context.Context, _ string) error               { return nil }
func (s *stubTeamWriter) BatchArchiveTeams(_ context.Context, _ []string) (int, error) {
	return 0, nil
}
func (s *stubTeamWriter) UpdateTeamWhereStatus(_ context.Context, id, newStatus, expectCurrent string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transitions = append(s.transitions, stubTeamTransition{id, newStatus, expectCurrent})
	return true, nil
}
func (s *stubTeamWriter) snapshotTransitions() []stubTeamTransition {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]stubTeamTransition, len(s.transitions))
	copy(out, s.transitions)
	return out
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
// memberEvidence customizes the F10 outcome evidence per member session ID;
// missing entry = no failure evidence.
type stubSpiritTeamController struct {
	recordCompletionCalls   int
	scheduleDependentsCalls int
	cancelTimeoutCalls      int
	completedResult         biz.AllTeamsCompletedResult
	hasRealDeliverable      bool
	hasRealDeliverableErr   error
	failureBriefs           []biz.TeamFailureBrief
	memberEvidence          map[string]stubMemberEvidence
	memberEvidenceCalls     int
	deliverableGateCalls    int
	// F9 验证门（outcome pass 步骤②）桩字段：零值 = approved（等价于定义中
	// 无 verification_gates）；gateReject/gateErr 模拟拒绝与执行错误（均须
	// fail-closed 翻转 failed）。
	gateReject  bool
	gateReasons []string
	gateErr     error
	gateCalls   int
}

// stubMemberEvidence is the per-session canned result for MemberExecutionEvidence.
type stubMemberEvidence struct {
	failed bool
	reason string
}

func (s *stubSpiritTeamController) CancelTimeoutTimer(_ string) { s.cancelTimeoutCalls++ }
func (s *stubSpiritTeamController) RecordTeamCompletion(_ context.Context, _ biz.Team, _ int64) (float64, biz.TopologyType) {
	s.recordCompletionCalls++
	return 0, ""
}
func (s *stubSpiritTeamController) ScheduleDependentTeams(_ context.Context, _ string, _ biz.Team) []biz.DependentTeamAction {
	s.scheduleDependentsCalls++
	return nil
}
func (s *stubSpiritTeamController) HasRealDeliverable(_ context.Context, _ biz.Team) (bool, error) {
	s.deliverableGateCalls++
	return s.hasRealDeliverable, s.hasRealDeliverableErr
}
func (s *stubSpiritTeamController) ListFailedTeamBriefs(_ context.Context, _ string) []biz.TeamFailureBrief {
	return s.failureBriefs
}
func (s *stubSpiritTeamController) ListTeamDeliverableDigests(_ context.Context, _ string) []biz.TeamDeliverableDigest {
	return nil
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
func (s *stubSpiritTeamController) MemberExecutionEvidence(_ context.Context, sessionID string) (bool, string) {
	s.memberEvidenceCalls++
	if s.memberEvidence == nil {
		return false, ""
	}
	ev, ok := s.memberEvidence[sessionID]
	if !ok {
		return false, ""
	}
	return ev.failed, ev.reason
}
func (s *stubSpiritTeamController) ExecuteVerificationGates(_ context.Context, _ string, _ string) (bool, []string, error) {
	s.gateCalls++
	if s.gateErr != nil {
		return false, nil, s.gateErr
	}
	if s.gateReject {
		return false, s.gateReasons, nil
	}
	return true, nil, nil
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
func (c *capturingSeq) snapshot() []biz.Event {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]biz.Event, len(c.events))
	copy(out, c.events)
	return out
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

	if controller.gateCalls != 1 {
		t.Errorf("ExecuteVerificationGates called %d times, want 1 (approved path must still execute the gate)", controller.gateCalls)
	}
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

// ── F10（Phase 11）结果导向成员状态 ────────────────────────────────────────
// 12:33 根因链末环：成员状态按消息生命周期（团队回调 completed → 全员
// completed）而非执行结果显示。修复后成员状态必须以执行结果为据：
//  1) per-member MemberExecutionEvidence（session interrupted / step
//     failed|cancelled → failed，附原因）；
//  2) 单成员团队追加交付物证据（HasRealDeliverable=false → failed）；
//     交付物证据限定 DAG 团队（HasRealDeliverable 对非 DAG 团队恒 false —
//     无交付物义务），且多成员团队不用（MDC 共享黑板无法按成员归因）；
//  3) cancelled → skipped 不被证据覆盖；team failed → failed 无需证据。
// 详见 docs/development/11-multi-agent.development.md Phase 11 F10。

// resolveMemberOutcomeStatus 单元测试：基础状态非 completed 时证据不介入。

func TestResolveMemberOutcomeStatus_TeamFailed_StaysFailed(t *testing.T) {
	status, reason := resolveMemberOutcomeStatus(context.Background(), nil, biz.Team{},
		biz.MemberSessionStatusFailed, "sess-x", true)
	if status != biz.MemberSessionStatusFailed || reason != "" {
		t.Fatalf("team failed → member failed without evidence, got (%s, %q)", status, reason)
	}
}

// cancelled → skipped 保持：证据（即使存在失败证据）不得覆盖。
func TestResolveMemberOutcomeStatus_Cancelled_StaysSkipped(t *testing.T) {
	ctrl := &stubSpiritTeamController{memberEvidence: map[string]stubMemberEvidence{
		"sess-x": {failed: true, reason: "step failed: boom"},
	}}
	status, reason := resolveMemberOutcomeStatus(context.Background(), ctrl, biz.Team{},
		biz.MemberSessionStatusSkipped, "sess-x", true)
	if status != biz.MemberSessionStatusSkipped || reason != "" {
		t.Fatalf("cancelled must stay skipped, got (%s, %q)", status, reason)
	}
	if ctrl.memberEvidenceCalls != 0 {
		t.Fatalf("evidence must not be consulted for skipped members, called %d times", ctrl.memberEvidenceCalls)
	}
}

// 团队 completed + 成员有失败证据 → failed 且携带原因。
func TestResolveMemberOutcomeStatus_EvidenceFailed_OverridesCompleted(t *testing.T) {
	ctrl := &stubSpiritTeamController{memberEvidence: map[string]stubMemberEvidence{
		"sess-a": {failed: true, reason: "step failed: 安装失败: skill 已存在"},
	}}
	status, reason := resolveMemberOutcomeStatus(context.Background(), ctrl,
		biz.Team{ID: "t1", DagNodeID: "st_1"},
		biz.MemberSessionStatusCompleted, "sess-a", true)
	if status != biz.MemberSessionStatusFailed {
		t.Fatalf("failure evidence must flip completed member to failed, got %s", status)
	}
	if !strings.Contains(reason, "安装失败") {
		t.Fatalf("reason must carry the evidence summary, got %q", reason)
	}
}

// 多成员团队：无失败证据时不用交付物证据（归因边界），成员保持 completed。
func TestResolveMemberOutcomeStatus_MultiMember_NoDeliverableCheck(t *testing.T) {
	ctrl := &stubSpiritTeamController{hasRealDeliverable: false}
	status, reason := resolveMemberOutcomeStatus(context.Background(), ctrl,
		biz.Team{ID: "t1", DagNodeID: "st_1"},
		biz.MemberSessionStatusCompleted, "sess-a", false)
	if status != biz.MemberSessionStatusCompleted || reason != "" {
		t.Fatalf("multi-member team must not use deliverable evidence, got (%s, %q)", status, reason)
	}
	if ctrl.deliverableGateCalls != 0 {
		t.Fatalf("HasRealDeliverable must not be consulted for multi-member teams, called %d times", ctrl.deliverableGateCalls)
	}
}

// 单成员 DAG 团队：无失败证据 + 无真实交付物 → failed（completed 必须被证明）。
func TestResolveMemberOutcomeStatus_SingleMemberDAG_NoDeliverable_Failed(t *testing.T) {
	ctrl := &stubSpiritTeamController{hasRealDeliverable: false}
	status, reason := resolveMemberOutcomeStatus(context.Background(), ctrl,
		biz.Team{ID: "t1", DagNodeID: "st_1"},
		biz.MemberSessionStatusCompleted, "sess-a", true)
	if status != biz.MemberSessionStatusFailed {
		t.Fatalf("single-member DAG team without real deliverable must be failed, got %s", status)
	}
	if !strings.Contains(reason, "set_deliverable") {
		t.Fatalf("reason must explain the missing deliverable, got %q", reason)
	}
}

// 单成员 DAG 团队：有真实交付物 → completed。
func TestResolveMemberOutcomeStatus_SingleMemberDAG_HasDeliverable_Completed(t *testing.T) {
	ctrl := &stubSpiritTeamController{hasRealDeliverable: true}
	status, reason := resolveMemberOutcomeStatus(context.Background(), ctrl,
		biz.Team{ID: "t1", DagNodeID: "st_1"},
		biz.MemberSessionStatusCompleted, "sess-a", true)
	if status != biz.MemberSessionStatusCompleted || reason != "" {
		t.Fatalf("real deliverable must keep member completed, got (%s, %q)", status, reason)
	}
}

// 单成员非 DAG 团队：无交付物义务（HasRealDeliverable 恒 false），不得误翻。
func TestResolveMemberOutcomeStatus_SingleMemberNonDAG_NoObligation(t *testing.T) {
	ctrl := &stubSpiritTeamController{hasRealDeliverable: false}
	status, reason := resolveMemberOutcomeStatus(context.Background(), ctrl,
		biz.Team{ID: "t1"}, // DagNodeID 为空 = 非 DAG 团队
		biz.MemberSessionStatusCompleted, "sess-a", true)
	if status != biz.MemberSessionStatusCompleted || reason != "" {
		t.Fatalf("non-DAG team carries no deliverable obligation, got (%s, %q)", status, reason)
	}
	if ctrl.deliverableGateCalls != 0 {
		t.Fatalf("HasRealDeliverable must not be consulted for non-DAG teams, called %d times", ctrl.deliverableGateCalls)
	}
}

// 交付物校验 infra 错误：无法证明成功 → failed（与 Fix 1 闸门同姿态：
// gateErr 按无交付物处理）。
func TestResolveMemberOutcomeStatus_DeliverableCheckError_Failed(t *testing.T) {
	ctrl := &stubSpiritTeamController{hasRealDeliverableErr: errors.New("graph state store down")}
	status, reason := resolveMemberOutcomeStatus(context.Background(), ctrl,
		biz.Team{ID: "t1", DagNodeID: "st_1"},
		biz.MemberSessionStatusCompleted, "sess-a", true)
	if status != biz.MemberSessionStatusFailed || reason == "" {
		t.Fatalf("deliverable check infra error must flip to failed with reason, got (%s, %q)", status, reason)
	}
}

// controller 未接线（nil）：保守不翻转，保持 completed（防御 nil 解引用）。
func TestResolveMemberOutcomeStatus_NilController_Completed(t *testing.T) {
	status, reason := resolveMemberOutcomeStatus(context.Background(), nil,
		biz.Team{ID: "t1", DagNodeID: "st_1"},
		biz.MemberSessionStatusCompleted, "sess-a", true)
	if status != biz.MemberSessionStatusCompleted || reason != "" {
		t.Fatalf("nil controller must not flip status, got (%s, %q)", status, reason)
	}
}

// ── F10 接线集成测试（publishV2TeamRunCompletion 成员循环）────────────────

// f10SessionRepo 按 TeamID 返回成员会话（publishV2TeamRunCompletion 的
// sessions.Search 数据源）。
type f10SessionRepo struct {
	biz.SessionRepo
	sessions []biz.Session
	// interruptCalls records UpdateSession calls that set Status=interrupted
	// (S-2: standalone cancelled must interrupt running member sessions).
	interruptCalls []string
}

func (r *f10SessionRepo) SearchSessions(_ context.Context, q biz.SessionSearchQuery) (biz.SessionListResult, error) {
	var items []biz.Session
	for _, s := range r.sessions {
		if s.TeamID == q.TeamID {
			items = append(items, s)
		}
	}
	return biz.SessionListResult{Items: items, Total: len(items)}, nil
}

func (r *f10SessionRepo) GetSessionByID(_ context.Context, id string) (biz.Session, error) {
	for _, s := range r.sessions {
		if s.ID == id {
			return s, nil
		}
	}
	return biz.Session{}, biz.ErrNotFound
}

func (r *f10SessionRepo) UpdateSession(_ context.Context, id string, fields biz.SessionUpdateFields) (biz.Session, error) {
	if fields.Status != nil && *fields.Status == string(sessstatus.SessionStatusInterrupted) {
		r.interruptCalls = append(r.interruptCalls, id)
	}
	for i, s := range r.sessions {
		if s.ID == id {
			if fields.Status != nil {
				r.sessions[i].Status = *fields.Status
			}
			return r.sessions[i], nil
		}
	}
	return biz.Session{}, biz.ErrNotFound
}

func newF10Starter(team biz.Team, controller *stubSpiritTeamController, sessions []biz.Session, seq *capturingSeq) *TeamStarter {
	teamUC := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader:    &stubTeamReader{teams: map[string]biz.Team{team.ID: team}},
		Writer:    &stubTeamWriter{},
		RunReader: &stubTeamRunReader{},
		Lg:        loggateway.NewNoop(),
	})
	return &TeamStarter{
		sessions: biz.NewSessionUsecase(&f10SessionRepo{sessions: sessions}, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop()),
		team:     TeamOrchestrationDeps{TeamUC: teamUC, SpiritUC: controller},
		seq:      seq,
		lg:       loggateway.NewNoop(),
		teamRunR: &stubTeamRunV2Reader{runs: map[string]biz.TeamRun{}},
		trSM:     biz.NewTeamRunV2StateMachine(),
	}
}

func memberSessionsByAgentKey(seq *capturingSeq) map[string]biz.MemberSession {
	out := make(map[string]biz.MemberSession)
	for _, ev := range seq.snapshot() {
		if msEv, ok := ev.(*biz.MemberSessionUpdatedEvent); ok {
			out[msEv.MemberSession.AgentKey] = msEv.MemberSession
		}
	}
	return out
}

// 多成员团队：仅失败证据命中的成员翻 failed（携带原因），其余保持 completed。
func TestPublishV2TeamRunCompletion_F10_PerMemberEvidenceOverride(t *testing.T) {
	team := biz.Team{
		ID: "team-f10", DisplayName: "安装团队", SpiritSessionID: "spirit-1",
		AutoCreated: true, Status: biz.TeamStatusCompleted, DagNodeID: "st_1",
	}
	sessions := []biz.Session{
		{ID: "sess-team", TeamID: team.ID, SessionType: "team"},
		{ID: "sess-a", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-a"},
		{ID: "sess-b", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-b"},
	}
	ctrl := &stubSpiritTeamController{
		hasRealDeliverable: true,
		memberEvidence: map[string]stubMemberEvidence{
			"sess-a": {failed: true, reason: "step failed: 安装失败: skill 已存在"},
		},
	}
	seq := &capturingSeq{}
	s := newF10Starter(team, ctrl, sessions, seq)

	s.publishV2TeamRunCompletion(context.Background(), team.SpiritSessionID, team.ID,
		biz.TeamStageStatusCompleted, biz.TeamStatusCompleted)

	got := memberSessionsByAgentKey(seq)
	msA, ok := got["agent-a"]
	if !ok {
		t.Fatalf("no MemberSession event for agent-a, got keys %v", got)
	}
	if msA.Status != biz.MemberSessionStatusFailed {
		t.Errorf("agent-a status = %s, want failed (failure evidence)", msA.Status)
	}
	if !strings.Contains(msA.Error, "安装失败") {
		t.Errorf("agent-a Error should carry the evidence reason, got %q", msA.Error)
	}
	msB, ok := got["agent-b"]
	if !ok {
		t.Fatalf("no MemberSession event for agent-b")
	}
	if msB.Status != biz.MemberSessionStatusCompleted {
		t.Errorf("agent-b status = %s, want completed (no evidence)", msB.Status)
	}
	if msB.Error != "" {
		t.Errorf("agent-b Error should be empty, got %q", msB.Error)
	}
}

// 单成员 DAG 团队：无失败证据但无真实交付物 → 成员 failed。
func TestPublishV2TeamRunCompletion_F10_SingleMemberNoDeliverable_Failed(t *testing.T) {
	team := biz.Team{
		ID: "team-f10-single", DisplayName: "单成员团队", SpiritSessionID: "spirit-1",
		AutoCreated: true, Status: biz.TeamStatusCompleted, DagNodeID: "st_1",
	}
	sessions := []biz.Session{
		{ID: "sess-team", TeamID: team.ID, SessionType: "team"},
		{ID: "sess-a", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-a"},
	}
	ctrl := &stubSpiritTeamController{hasRealDeliverable: false}
	seq := &capturingSeq{}
	s := newF10Starter(team, ctrl, sessions, seq)

	s.publishV2TeamRunCompletion(context.Background(), team.SpiritSessionID, team.ID,
		biz.TeamStageStatusCompleted, biz.TeamStatusCompleted)

	got := memberSessionsByAgentKey(seq)
	msA, ok := got["agent-a"]
	if !ok {
		t.Fatalf("no MemberSession event for agent-a")
	}
	if msA.Status != biz.MemberSessionStatusFailed {
		t.Errorf("agent-a status = %s, want failed (no real deliverable)", msA.Status)
	}
	if msA.Error == "" {
		t.Errorf("agent-a Error should explain the missing deliverable")
	}
}

// 团队 cancelled：成员保持 skipped，证据不得覆盖。
func TestPublishV2TeamRunCompletion_F10_Cancelled_StaysSkipped(t *testing.T) {
	team := biz.Team{
		ID: "team-f10-cancel", DisplayName: "取消团队", SpiritSessionID: "spirit-1",
		AutoCreated: true, Status: biz.TeamStatusCancelled, DagNodeID: "st_1",
	}
	sessions := []biz.Session{
		{ID: "sess-a", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-a"},
	}
	ctrl := &stubSpiritTeamController{memberEvidence: map[string]stubMemberEvidence{
		"sess-a": {failed: true, reason: "session interrupted: cancelled"},
	}}
	seq := &capturingSeq{}
	s := newF10Starter(team, ctrl, sessions, seq)

	s.publishV2TeamRunCompletion(context.Background(), team.SpiritSessionID, team.ID,
		biz.TeamStageStatusCancelled, biz.TeamStatusCancelled)

	got := memberSessionsByAgentKey(seq)
	msA, ok := got["agent-a"]
	if !ok {
		t.Fatalf("no MemberSession event for agent-a")
	}
	if msA.Status != biz.MemberSessionStatusSkipped {
		t.Errorf("agent-a status = %s, want skipped (cancelled not overridable)", msA.Status)
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

// ── 成员终态权威分层（2026-07-28 单写者重设计）──────────────────────────────
// member_sessions_v2 的 Version 是写者权威层级而非任意编号：
//   created=1（runner，生命周期事实）< outcome=哨兵（service 终态裁决，唯一
//   可宣布成员成功的写者族；2026-07-29 哨兵化为 1<<40，保证 pause/resume
//   的 Version++ 写者无法到达，终态恒赢）。runner 不再投影成员 completed
//   （消息生命周期 ≠ 工作结果）；service 终态事件必须恒为 outcome 哨兵带，
//   保证 UpsertMemberSession 的 VersionLT 守卫与前端 activityV2Store 守卫
//   都单调通过——F10 的 failed 覆盖与团队 failed 传播不再被静默丢弃。

// 主循环：终态成员事件必须携带 outcome 权威版本（哨兵带）。
func TestPublishV2TeamRunCompletion_TerminalEventsCarryOutcomeVersion(t *testing.T) {
	team := biz.Team{
		ID: "team-ver", DisplayName: "版本团队", SpiritSessionID: "spirit-1",
		AutoCreated: true, Status: biz.TeamStatusCompleted, DagNodeID: "st_1",
	}
	sessions := []biz.Session{
		{ID: "sess-team", TeamID: team.ID, SessionType: "team"},
		{ID: "sess-a", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-a"},
		{ID: "sess-b", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-b"},
	}
	ctrl := &stubSpiritTeamController{hasRealDeliverable: true}
	seq := &capturingSeq{}
	s := newF10Starter(team, ctrl, sessions, seq)

	s.publishV2TeamRunCompletion(context.Background(), team.SpiritSessionID, team.ID,
		biz.TeamStageStatusCompleted, biz.TeamStatusCompleted)

	got := memberSessionsByAgentKey(seq)
	if len(got) != 2 {
		t.Fatalf("published member sessions = %d, want 2", len(got))
	}
	for key, ms := range got {
		if ms.Version != biz.MemberSessionVersionOutcome {
			t.Errorf("member %s terminal Version = %d, want outcome authority band (%d)", key, ms.Version, biz.MemberSessionVersionOutcome)
		}
	}
}

// F4 兜底（定义成员无 agent session）：终态事件同样必须携带 outcome 哨兵带。
func TestPublishV2TeamRunCompletion_DefinitionFallback_CarriesOutcomeVersion(t *testing.T) {
	team := biz.Team{
		ID: "team-ver-fb", DisplayName: "兜底团队", SpiritSessionID: "spirit-1",
		AutoCreated: true, Status: biz.TeamStatusCompleted, DagNodeID: "st_1",
		DefinitionJSON: `{"members":[{"agent_key":"ghost-agent","name":"幽灵成员"}]}`,
	}
	sessions := []biz.Session{
		{ID: "sess-team", TeamID: team.ID, SessionType: "team"},
	}
	ctrl := &stubSpiritTeamController{hasRealDeliverable: true}
	seq := &capturingSeq{}
	s := newF10Starter(team, ctrl, sessions, seq)

	s.publishV2TeamRunCompletion(context.Background(), team.SpiritSessionID, team.ID,
		biz.TeamStageStatusCompleted, biz.TeamStatusCompleted)

	got := memberSessionsByAgentKey(seq)
	ms, ok := got["ghost-agent"]
	if !ok {
		t.Fatalf("no fallback MemberSession event for ghost-agent, got keys %v", got)
	}
	if ms.Version != biz.MemberSessionVersionOutcome {
		t.Errorf("fallback terminal Version = %d, want outcome authority band (%d)", ms.Version, biz.MemberSessionVersionOutcome)
	}
}

// ── F9 验证门挂接（outcome pass 步骤②，2026-07-28）─────────────────────────
// 设计裁决：团队终态是唯一真相裁决点。Fix-1 交付物门之后必须执行 definition
// verification_gates（当前唯一自动来源：skill 安装 tool_assertion 门）；
// 拒绝或执行错误均 fail-closed 翻转 failed —— 「装了但不可用」不得报成功。

// completed 回调 + 交付物门通过 + 验证门拒绝 → 团队 failed：不记录完成、
// 级联调度一次、发布 TeamStageFailedEvent。
func TestHandleTeamTurnResult_VerificationGateRejected_MarkedFailed(t *testing.T) {
	team := biz.Team{
		ID: "team-gate-rej", DisplayName: "安装团队", SpiritSessionID: "spirit-1",
		AutoCreated: true, Status: biz.TeamStatusRunning, DagNodeID: "st_1",
	}
	controller := &stubSpiritTeamController{
		hasRealDeliverable: true,
		gateReject:         true,
		gateReasons:        []string{"skill 'X' 安装校验失败: enabled=false"},
	}
	eventBus := &capturingEventBus{}
	s := newDeliverableGateStarter(team, controller, eventBus)

	s.HandleTeamTurnResult(context.Background(), team.SpiritSessionID, team.ID, biz.TeamStatusCompleted, "", "")

	if controller.gateCalls != 1 {
		t.Errorf("ExecuteVerificationGates called %d times, want 1", controller.gateCalls)
	}
	if controller.recordCompletionCalls != 0 {
		t.Errorf("RecordTeamCompletion called %d times, want 0 (gate rejected)", controller.recordCompletionCalls)
	}
	if controller.scheduleDependentsCalls != 1 {
		t.Errorf("ScheduleDependentTeams called %d times, want 1 (cascade fail downstream)", controller.scheduleDependentsCalls)
	}
	var failedFound bool
	for _, ev := range eventBus.snapshot() {
		if tsEv, ok := ev.(*biz.TeamStageFailedEvent); ok && tsEv.TeamStage.TeamID == team.ID {
			failedFound = true
		}
		if _, ok := ev.(*biz.TeamStageCompletedEvent); ok {
			t.Errorf("TeamStageCompletedEvent must not be published when the verification gate rejects")
		}
	}
	if !failedFound {
		t.Errorf("no TeamStageFailedEvent published, want one for the gate-rejected team")
	}
}

// completed 回调 + 交付物门通过 + 验证门执行错误（infra）→ fail-closed
// 翻转 failed（与 Fix-1 门同姿态：校验不可用不等于通过）。
func TestHandleTeamTurnResult_VerificationGateError_MarkedFailed(t *testing.T) {
	team := biz.Team{
		ID: "team-gate-err", DisplayName: "安装团队", SpiritSessionID: "spirit-1",
		AutoCreated: true, Status: biz.TeamStatusRunning, DagNodeID: "st_1",
	}
	controller := &stubSpiritTeamController{
		hasRealDeliverable: true,
		gateErr:            errors.New("tool_assertion gate: no tool invoker configured"),
	}
	eventBus := &capturingEventBus{}
	s := newDeliverableGateStarter(team, controller, eventBus)

	s.HandleTeamTurnResult(context.Background(), team.SpiritSessionID, team.ID, biz.TeamStatusCompleted, "", "")

	if controller.gateCalls != 1 {
		t.Errorf("ExecuteVerificationGates called %d times, want 1", controller.gateCalls)
	}
	if controller.recordCompletionCalls != 0 {
		t.Errorf("RecordTeamCompletion called %d times, want 0 (gate error, fail-closed)", controller.recordCompletionCalls)
	}
	var failedFound bool
	for _, ev := range eventBus.snapshot() {
		if tsEv, ok := ev.(*biz.TeamStageFailedEvent); ok && tsEv.TeamStage.TeamID == team.ID {
			failedFound = true
		}
		if _, ok := ev.(*biz.TeamStageCompletedEvent); ok {
			t.Errorf("TeamStageCompletedEvent must not be published on gate infra error")
		}
	}
	if !failedFound {
		t.Errorf("no TeamStageFailedEvent published, want one for the gate-error team")
	}
}

// ── F-1 standalone（Mode A）终态 pass（2026-07-29）──────────────────────────
// 此前 HandleTeamTurnResult 对非 AutoCreated 团队直接早退，standalone 团队的
// teams 状态、TeamStage、TeamRun、MemberSession 全部永不达终态（成员永
// running）。修复后 standalone 走精简终态 pass：teams 转换 + TeamStage 终态
// + 成员 outcome pass（F10 证据 + F4 兜底，哨兵版本带）；编排专属职责
// （deliverable 门 / F9 验证门 / recordTeamCompletion / 依赖调度 / synthesis）
// 不适用，必须不被触发。

// newStandaloneStarter 构造 standalone 终态 pass 测试用的 TeamStarter：
// seq 承载成员事件与 TeamStage 事件（publishV2Event 优先 seq），
// teamStageR 预置 Running TeamStage（V=1）供 resolveTeamStageUpdate 推导 V=2。
func newStandaloneStarter(
	team biz.Team,
	controller *stubSpiritTeamController,
	writer *stubTeamWriter,
	sessions []biz.Session,
	seq *capturingSeq,
) *TeamStarter {
	return newStandaloneStarterFromRepo(team, controller, writer, &f10SessionRepo{sessions: sessions}, seq)
}

// newStandaloneStarterFromRepo 与 newStandaloneStarter 同构，但共享调用方
// 持有的 f10SessionRepo——测试可断言 session 层副作用（S-2 interrupted 转换）。
func newStandaloneStarterFromRepo(
	team biz.Team,
	controller *stubSpiritTeamController,
	writer *stubTeamWriter,
	repo *f10SessionRepo,
	seq *capturingSeq,
) *TeamStarter {
	tsID := string(agent.NewTeamStageActivityID(team.ID))
	teamUC := biz.NewTeamUsecase(biz.TeamUsecaseOpts{
		Reader:    &stubTeamReader{teams: map[string]biz.Team{team.ID: team}},
		Writer:    writer,
		RunReader: &stubTeamRunReader{},
		Lg:        loggateway.NewNoop(),
	})
	return &TeamStarter{
		sessions: biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop()),
		team:     TeamOrchestrationDeps{TeamUC: teamUC, SpiritUC: controller},
		seq:      seq,
		lg:       loggateway.NewNoop(),
		teamStageR: &stubTeamStageV2Reader{stages: map[string]biz.TeamStage{
			tsID: {ID: tsID, TeamID: team.ID, SessionID: team.SpiritSessionID,
				Status: biz.TeamStageStatusRunning, Stage: biz.TeamStageStageExecuting, Version: 1},
		}},
		tsSM:     biz.NewTeamStageStateMachine(),
		teamRunR: &stubTeamRunV2Reader{runs: map[string]biz.TeamRun{}},
		trSM:     biz.NewTeamRunV2StateMachine(),
	}
}

// standalone completed：teams 转 completed + TeamStageCompletedEvent + 成员
// completed（outcome 哨兵带）；编排专属职责全部不触发。
func TestHandleTeamTurnResult_Standalone_Completed_TerminalPass(t *testing.T) {
	team := biz.Team{
		ID: "team-sa-ok", DisplayName: "独立团队", SpiritSessionID: "sess-sa-ok",
		AutoCreated: false, Status: biz.TeamStatusRunning,
	}
	sessions := []biz.Session{
		{ID: "sess-sa-ok", TeamID: team.ID, SessionType: "team"},
		{ID: "sess-sa-a", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-a"},
	}
	controller := &stubSpiritTeamController{}
	writer := &stubTeamWriter{}
	seq := &capturingSeq{}
	s := newStandaloneStarter(team, controller, writer, sessions, seq)

	// 聚合根回退（F-3）：standalone 无 ParentSessionID，调用方以 team session
	// ID 作为 spiritSessionID 传入。
	s.HandleTeamTurnResult(context.Background(), "sess-sa-ok", team.ID, biz.TeamStatusCompleted, "", "")

	// teams 表转换到 completed。
	trs := writer.snapshotTransitions()
	if len(trs) != 1 || trs[0].newStatus != biz.TeamStatusCompleted || trs[0].id != team.ID {
		t.Fatalf("teams transitions = %+v, want exactly one → completed for %s", trs, team.ID)
	}
	if controller.cancelTimeoutCalls != 1 {
		t.Errorf("CancelTimeoutTimer called %d times, want 1", controller.cancelTimeoutCalls)
	}
	// 编排专属职责不触发。
	if controller.deliverableGateCalls != 0 {
		t.Errorf("HasRealDeliverable called %d times, want 0 for standalone", controller.deliverableGateCalls)
	}
	if controller.gateCalls != 0 {
		t.Errorf("ExecuteVerificationGates called %d times, want 0 for standalone", controller.gateCalls)
	}
	if controller.recordCompletionCalls != 0 {
		t.Errorf("RecordTeamCompletion called %d times, want 0 for standalone", controller.recordCompletionCalls)
	}
	if controller.scheduleDependentsCalls != 0 {
		t.Errorf("ScheduleDependentTeams called %d times, want 0 for standalone", controller.scheduleDependentsCalls)
	}
	// TeamStage 终态事件。
	var tsCompleted bool
	for _, ev := range seq.snapshot() {
		if tsEv, ok := ev.(*biz.TeamStageCompletedEvent); ok && tsEv.TeamStage.TeamID == team.ID {
			tsCompleted = true
			if tsEv.TeamStage.Version != 2 {
				t.Errorf("TeamStage terminal Version = %d, want 2 (resolveTeamStageUpdate from V=1)", tsEv.TeamStage.Version)
			}
		}
	}
	if !tsCompleted {
		t.Errorf("no TeamStageCompletedEvent published for standalone team")
	}
	// 成员终态：completed + outcome 哨兵带。
	got := memberSessionsByAgentKey(seq)
	msA, ok := got["agent-a"]
	if !ok {
		t.Fatalf("no MemberSession event for agent-a")
	}
	if msA.Status != biz.MemberSessionStatusCompleted {
		t.Errorf("agent-a status = %s, want completed", msA.Status)
	}
	if msA.Version != biz.MemberSessionVersionOutcome {
		t.Errorf("agent-a Version = %d, want outcome sentinel (%d)", msA.Version, biz.MemberSessionVersionOutcome)
	}
	if msA.SpiritSessionID != "sess-sa-ok" {
		t.Errorf("agent-a SpiritSessionID = %q, want fallback aggregate root sess-sa-ok", msA.SpiritSessionID)
	}
}

// standalone failed：teams 转 failed + TeamStageFailedEvent + 成员 failed
// （outcome 哨兵带，无需证据——团队 failed 直接传播）。
func TestHandleTeamTurnResult_Standalone_Failed_TerminalPass(t *testing.T) {
	team := biz.Team{
		ID: "team-sa-fail", DisplayName: "独立团队", SpiritSessionID: "sess-sa-fail",
		AutoCreated: false, Status: biz.TeamStatusRunning,
	}
	sessions := []biz.Session{
		{ID: "sess-sa-fail", TeamID: team.ID, SessionType: "team"},
		{ID: "sess-sa-a", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-a"},
	}
	controller := &stubSpiritTeamController{}
	writer := &stubTeamWriter{}
	seq := &capturingSeq{}
	s := newStandaloneStarter(team, controller, writer, sessions, seq)

	s.HandleTeamTurnResult(context.Background(), "sess-sa-fail", team.ID, biz.TeamStatusFailed, "boom", "")

	trs := writer.snapshotTransitions()
	if len(trs) != 1 || trs[0].newStatus != biz.TeamStatusFailed {
		t.Fatalf("teams transitions = %+v, want exactly one → failed", trs)
	}
	var tsFailed bool
	for _, ev := range seq.snapshot() {
		if tsEv, ok := ev.(*biz.TeamStageFailedEvent); ok && tsEv.TeamStage.TeamID == team.ID {
			tsFailed = true
		}
		if _, ok := ev.(*biz.TeamStageCompletedEvent); ok {
			t.Errorf("TeamStageCompletedEvent must not be published for a failed standalone team")
		}
	}
	if !tsFailed {
		t.Errorf("no TeamStageFailedEvent published for failed standalone team")
	}
	got := memberSessionsByAgentKey(seq)
	msA, ok := got["agent-a"]
	if !ok {
		t.Fatalf("no MemberSession event for agent-a")
	}
	if msA.Status != biz.MemberSessionStatusFailed {
		t.Errorf("agent-a status = %s, want failed (team failure propagates)", msA.Status)
	}
	if msA.Version != biz.MemberSessionVersionOutcome {
		t.Errorf("agent-a Version = %d, want outcome sentinel (%d)", msA.Version, biz.MemberSessionVersionOutcome)
	}
	// 团队 failed 传播无需成员证据。
	if controller.memberEvidenceCalls != 0 {
		t.Errorf("MemberExecutionEvidence called %d times, want 0 (team failed needs no evidence)", controller.memberEvidenceCalls)
	}
}

// standalone 空 spiritSessionID 回退（2026-07-29 S-1）：CancelTeam 等无
// session 上下文的入口传空 spiritSessionID（team.SpiritSessionID=="",
// CreateTeam 不落该字段）时，service 必须内部回退查 team session 作聚合根
// （F-3 语义兜底）；此前 publishV2TeamRunCompletion 的空守卫直接 return，
// TeamStage/TeamRun/MemberSession 永不达终态（成员永 running）。
func TestHandleTeamTurnResult_Standalone_EmptySpiritID_FallbackToTeamSession(t *testing.T) {
	team := biz.Team{
		ID: "team-sa-e", DisplayName: "独立团队", SpiritSessionID: "", // 生产真实状态：CreateTeam 不传
		AutoCreated: false, Status: biz.TeamStatusRunning,
	}
	sessions := []biz.Session{
		{ID: "sess-sa-e", TeamID: team.ID, SessionType: "team"},
		{ID: "sess-sa-e-a", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-a"},
	}
	controller := &stubSpiritTeamController{}
	writer := &stubTeamWriter{}
	seq := &capturingSeq{}
	s := newStandaloneStarter(team, controller, writer, sessions, seq)

	// CancelTeam 入口语义：spiritSessionID 为空。
	s.HandleTeamTurnResult(context.Background(), "", team.ID, biz.TeamStatusCompleted, "", "")

	// 成员终态事件必须发布，且 SpiritSessionID 回退为 team session ID。
	got := memberSessionsByAgentKey(seq)
	msA, ok := got["agent-a"]
	if !ok {
		t.Fatalf("no MemberSession event for agent-a (empty spiritSessionID must fall back to team session)")
	}
	if msA.Status != biz.MemberSessionStatusCompleted {
		t.Errorf("agent-a status = %s, want completed", msA.Status)
	}
	if msA.Version != biz.MemberSessionVersionOutcome {
		t.Errorf("agent-a Version = %d, want outcome sentinel (%d)", msA.Version, biz.MemberSessionVersionOutcome)
	}
	if msA.SpiritSessionID != "sess-sa-e" {
		t.Errorf("agent-a SpiritSessionID = %q, want fallback team session sess-sa-e", msA.SpiritSessionID)
	}
	// TeamStage 终态事件同样携带回退后的聚合根。
	var tsSessionID string
	for _, ev := range seq.snapshot() {
		if tsEv, ok := ev.(*biz.TeamStageCompletedEvent); ok && tsEv.TeamStage.TeamID == team.ID {
			tsSessionID = tsEv.TeamStage.SessionID
		}
	}
	if tsSessionID != "sess-sa-e" {
		t.Errorf("TeamStage SessionID = %q, want fallback team session sess-sa-e", tsSessionID)
	}
}

// standalone cancelled（2026-07-29 S-1+S-2）：teams 已由调用方（biz.CancelTeam）
// 转换、不重复；成员 skipped（outcome 哨兵带）；running 成员 session 转
// interrupted（对齐 AutoCreated 路径）；TeamStage cancelled 事件发布。
func TestHandleTeamTurnResult_Standalone_Cancelled_TerminalPass(t *testing.T) {
	team := biz.Team{
		ID: "team-sa-x", DisplayName: "独立团队", SpiritSessionID: "",
		AutoCreated: false, Status: biz.TeamStatusCancelled, // biz.CancelTeam 已转换
	}
	repo := &f10SessionRepo{sessions: []biz.Session{
		{ID: "sess-sa-x", TeamID: team.ID, SessionType: "team", Status: string(sessstatus.SessionStatusRunning)},
		{ID: "sess-sa-x-a", TeamID: team.ID, SessionType: "agent", MemberAgentKey: "agent-a", Status: string(sessstatus.SessionStatusRunning)},
	}}
	controller := &stubSpiritTeamController{}
	writer := &stubTeamWriter{}
	seq := &capturingSeq{}
	s := newStandaloneStarterFromRepo(team, controller, writer, repo, seq)

	// CancelTeam 入口：spiritSessionID 为空 → 内部回退。
	s.HandleTeamTurnResult(context.Background(), "", team.ID, biz.TeamStatusCancelled, "", "")

	// teams 表不重复转换（cancelled 由调用方已转换）。
	if trs := writer.snapshotTransitions(); len(trs) != 0 {
		t.Errorf("teams transitions = %+v, want none for cancelled (caller already transitioned)", trs)
	}
	// TeamStage cancelled 事件（UpdatedEvent 语义，无 CancelledEvent 工厂）。
	var tsCancelled bool
	for _, ev := range seq.snapshot() {
		if tsEv, ok := ev.(*biz.TeamStageUpdatedEvent); ok && tsEv.TeamStage.TeamID == team.ID &&
			tsEv.TeamStage.Status == biz.TeamStageStatusCancelled {
			tsCancelled = true
			if tsEv.TeamStage.SessionID != "sess-sa-x" {
				t.Errorf("TeamStage SessionID = %q, want fallback team session sess-sa-x", tsEv.TeamStage.SessionID)
			}
		}
	}
	if !tsCancelled {
		t.Errorf("no TeamStage cancelled event published for standalone team")
	}
	// 成员 skipped + 哨兵带。
	got := memberSessionsByAgentKey(seq)
	msA, ok := got["agent-a"]
	if !ok {
		t.Fatalf("no MemberSession event for agent-a")
	}
	if msA.Status != biz.MemberSessionStatusSkipped {
		t.Errorf("agent-a status = %s, want skipped (team cancelled)", msA.Status)
	}
	if msA.Version != biz.MemberSessionVersionOutcome {
		t.Errorf("agent-a Version = %d, want outcome sentinel (%d)", msA.Version, biz.MemberSessionVersionOutcome)
	}
	// running 成员 session 必须转 interrupted（S-2，对齐 AutoCreated 路径）。
	if len(repo.interruptCalls) == 0 {
		t.Errorf("no member session interrupted transition, want running sessions interrupted on cancel")
	}
}
