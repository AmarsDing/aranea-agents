package biz

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"aranea-agents/internal/biz/session"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
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
func (m *memSpiritTeamRepo) BatchArchiveTeams(_ context.Context, ids []string) (int, error) {
	n := 0
	for _, id := range ids {
		if t, ok := m.items[id]; ok {
			t.Status = TeamStatusArchived
			m.items[id] = t
			n++
		}
	}
	return n, nil
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
func (m *memSpiritTeamRepo) GetTeamByKey(_ context.Context, _ string) (Team, error) {
	return Team{}, fmt.Errorf("not found")
}
func (m *memSpiritTeamRepo) ListTeamsByDepartmentID(_ context.Context, _ string) ([]Team, error) {
	return nil, nil
}
func (m *memSpiritTeamRepo) ListTeamsByWorkspace(_ context.Context, _ string) ([]Team, error) {
	return nil, nil
}
func (m *memSpiritTeamRepo) CountTeamsByWorkspace(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *memSpiritTeamRepo) ListTeamRuns(_ context.Context, _ string, _ int) ([]TeamRunRecord, error) {
	return nil, nil
}
func (m *memSpiritTeamRepo) ListTeamRunsByTeamIDs(_ context.Context, _ []string, _ int) (map[string][]TeamRunRecord, error) {
	return nil, nil
}
func (m *memSpiritTeamRepo) HasActiveTeamRun(_ context.Context, _ string) (bool, error) {
	return false, nil
}
func (m *memSpiritTeamRepo) GetTeamRunByID(_ context.Context, id string) (TeamRunRecord, error) {
	return TeamRunRecord{}, fmt.Errorf("team run not found: %s", id)
}
func (m *memSpiritTeamRepo) ListTeamRunSteps(_ context.Context, _ string) ([]TeamRunStep, error) {
	return nil, nil
}
func (m *memSpiritTeamRepo) CreateTeamRun(_ context.Context, r TeamRunRecord) (TeamRunRecord, error) {
	return r, nil
}
func (m *memSpiritTeamRepo) UpdateTeamRun(_ context.Context, _ TeamRunRecord) error { return nil }
func (m *memSpiritTeamRepo) UpdateTeamWhereStatus(_ context.Context, id, newStatus, whereStatus string) (bool, error) {
	t, ok := m.items[id]
	if !ok || t.Status != whereStatus {
		return false, nil
	}
	t.Status = newStatus
	m.items[id] = t
	return true, nil
}
func (m *memSpiritTeamRepo) UpdateTeamRunWhereStatus(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}
func (m *memSpiritTeamRepo) UpdateTeamRunSummaryJSON(_ context.Context, _, _ string) error {
	return nil
}
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

// memSpiritAgentResolver is a minimal SpiritAgentResolver stub: returns an
// empty active-agent list so resolveAgentKeyToIDMap falls back to "agent_"+key.
type memSpiritAgentResolver struct{}

func (m *memSpiritAgentResolver) List(_ context.Context, _ AgentListQuery) (AgentListResult, error) {
	return AgentListResult{}, nil
}

// memSpiritAgentLookup is a minimal session.AgentLookup stub: AssembleTeam
// creates member sessions with OwnerType="agent", and SessionUsecase.Create
// validates agent existence through this lookup. Always succeeds.
type memSpiritAgentLookup struct{}

func (m *memSpiritAgentLookup) GetAgentByID(_ context.Context, _ string) (struct{}, error) {
	return struct{}{}, nil
}

// memSpiritSessionRepo is a minimal in-memory session repo for SpiritTeamUsecase tests.
// It implements session.SessionRepo by embedding stub implementations for all sub-interfaces.
type memSpiritSessionRepo struct {
	items   map[string]Session
	failAll bool // if true, Create always fails (for rollback testing)
}

func newMemSpiritSessionRepo() *memSpiritSessionRepo {
	return &memSpiritSessionRepo{items: make(map[string]Session)}
}

// seedSpirit pre-creates spirit parent sessions so AssembleTeam's
// sessionUC.Get(spiritSessionID) + depth validation succeed.
// Writes items directly (bypasses failAll) since the spirit session is
// a precondition, not part of the transaction under test.
func (m *memSpiritSessionRepo) seedSpirit(ids ...string) {
	for _, id := range ids {
		m.items[id] = Session{ID: id, OwnerType: "spirit"}
	}
}

// --- SessionReader ---

func (m *memSpiritSessionRepo) SearchSessions(_ context.Context, q SessionSearchQuery) (SessionListResult, error) {
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
func (m *memSpiritSessionRepo) GetSessionByID(_ context.Context, id string) (Session, error) {
	s, ok := m.items[id]
	if !ok {
		return Session{}, fmt.Errorf("not found: %s", id)
	}
	return s, nil
}
func (m *memSpiritSessionRepo) GetSessionRevision(_ context.Context, _ string) (int64, error) {
	return 0, nil
}
func (m *memSpiritSessionRepo) ListSessionsForBatch(_ context.Context, _ SessionSearchQuery) ([]Session, error) {
	return nil, nil
}
func (m *memSpiritSessionRepo) ListSessionsByIDs(_ context.Context, _ []string) ([]Session, error) {
	return nil, nil
}
func (m *memSpiritSessionRepo) ListActiveAgentUserKeys(_ context.Context, _ int) ([]session.AgentUserKey, error) {
	return nil, nil
}

// --- SessionTreeReader ---

func (m *memSpiritSessionRepo) ListByParentSessionID(_ context.Context, _ string) ([]Session, error) {
	return nil, nil
}
func (m *memSpiritSessionRepo) GetSessionTree(_ context.Context, _ string) (*session.SessionTree, error) {
	return nil, nil
}
func (m *memSpiritSessionRepo) ListChildSessions(_ context.Context, _ string) ([]Session, error) {
	return nil, nil
}
func (m *memSpiritSessionRepo) ListTeamAgentSessions(_ context.Context, _ string) ([]Session, error) {
	return nil, nil
}

// --- SessionWriter ---

func (m *memSpiritSessionRepo) CreateSession(_ context.Context, in Session) (Session, error) {
	if m.failAll {
		return Session{}, apierror.Internal("SESSION", "simulated failure")
	}
	if in.ID == "" {
		in.ID = fmt.Sprintf("sess-%d", len(m.items)+1)
	}
	m.items[in.ID] = in
	return in, nil
}
func (m *memSpiritSessionRepo) UpdateSessionTitle(_ context.Context, id, title string) (Session, error) {
	s, ok := m.items[id]
	if !ok {
		return Session{}, fmt.Errorf("not found: %s", id)
	}
	s.Title = title
	m.items[id] = s
	return s, nil
}
func (m *memSpiritSessionRepo) UpdateSession(_ context.Context, id string, patch SessionUpdateFields) (Session, error) {
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
func (m *memSpiritSessionRepo) RestoreSession(_ context.Context, id string) (Session, error) {
	s, ok := m.items[id]
	if !ok {
		return Session{}, fmt.Errorf("not found: %s", id)
	}
	return s, nil
}
func (m *memSpiritSessionRepo) BumpSessionRevision(_ context.Context, _ string) (int64, error) {
	return 1, nil
}

// --- SessionMutator ---

func (m *memSpiritSessionRepo) ArchiveSession(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *memSpiritSessionRepo) DeleteSession(_ context.Context, _ string) (int, error)    { return 0, nil }
func (m *memSpiritSessionRepo) DeleteSessionsByAgentID(_ context.Context, _ string) error { return nil }
func (m *memSpiritSessionRepo) PinSession(_ context.Context, _ string) (Session, error) {
	return Session{}, nil
}
func (m *memSpiritSessionRepo) UnpinSession(_ context.Context, _ string) (Session, error) {
	return Session{}, nil
}

// --- SessionBatchMutator ---

func (m *memSpiritSessionRepo) ArchiveSessionsByIDs(_ context.Context, _ []string) (int, []string, error) {
	return 0, nil, nil
}
func (m *memSpiritSessionRepo) DeleteSessionsByIDs(_ context.Context, _ []string) (int, []string, error) {
	return 0, nil, nil
}

// --- MessageReader ---

func (m *memSpiritSessionRepo) CountMessagesBySession(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *memSpiritSessionRepo) ListMessagesBySession(_ context.Context, _ string, _, _ int) ([]ChatMessage, error) {
	return nil, nil
}
func (m *memSpiritSessionRepo) ListMessagesAfterTurn(_ context.Context, _ string, _ int) ([]ChatMessage, error) {
	return nil, nil
}
func (m *memSpiritSessionRepo) ListMessagesRecent(_ context.Context, _ string, _ int) ([]ChatMessage, error) {
	return nil, nil
}
func (m *memSpiritSessionRepo) ListMessagesByIDs(_ context.Context, _ string, _ []string) ([]ChatMessage, error) {
	return nil, nil
}

// --- MessageSearchReader ---

func (m *memSpiritSessionRepo) ListMessagesByStatus(_ context.Context, _, _ string, _ int) ([]ChatMessage, error) {
	return nil, nil
}
func (m *memSpiritSessionRepo) SearchMessages(_ context.Context, _ MessageSearchQuery) (MessageSearchResult, error) {
	return MessageSearchResult{}, nil
}
func (m *memSpiritSessionRepo) ListMessagesAfterRevision(_ context.Context, _ string, _ int64) ([]ChatMessage, error) {
	return nil, nil
}

// --- MessageWriter ---

func (m *memSpiritSessionRepo) AppendChatTurn(_ context.Context, _ string, _, _ ChatMessage) error {
	return nil
}
func (m *memSpiritSessionRepo) AppendChatMessage(_ context.Context, _ string, _ ChatMessage, _ bool) error {
	return nil
}
func (m *memSpiritSessionRepo) UpdateMessageFeedbackJSON(_ context.Context, _, _, _, _ string) error {
	return nil
}
func (m *memSpiritSessionRepo) UpsertChatActivityMessage(_ context.Context, _ string, _ ChatMessage) (bool, error) {
	return false, nil
}

// --- MessageStatusWriter ---

func (m *memSpiritSessionRepo) UpdateChatMessageStatus(_ context.Context, _, _, _, _ string) error {
	return nil
}

// --- TimelineReader ---

func (m *memSpiritSessionRepo) ListTimelineEventRefsPaged(_ context.Context, _ string, _ TimelineQuery) ([]TimelineEventRef, int, error) {
	return nil, 0, nil
}
func (m *memSpiritSessionRepo) ListToolInvocationsByIDs(_ context.Context, _ string, _ []string) ([]ToolInvocationView, error) {
	return nil, nil
}
func (m *memSpiritSessionRepo) ListSkillInvocationsByIDs(_ context.Context, _ string, _ []string) ([]SkillInvocationView, error) {
	return nil, nil
}
func (m *memSpiritSessionRepo) LookupAgentDisplayNames(_ context.Context, _ []string) (map[string]string, error) {
	return nil, nil
}

// --- InvocationReader ---

func (m *memSpiritSessionRepo) ListToolInvocationsBySession(_ context.Context, _ string, _ int) ([]ToolInvocationView, error) {
	return nil, nil
}
func (m *memSpiritSessionRepo) ListSkillInvocationsBySession(_ context.Context, _ string, _ int) ([]SkillInvocationView, error) {
	return nil, nil
}

// --- SummaryReader ---

func (m *memSpiritSessionRepo) MaxSessionSummaryToTurn(_ context.Context, _ string) (int, error) {
	return 0, nil
}
func (m *memSpiritSessionRepo) ListSessionSummaries(_ context.Context, _ string) ([]SessionSummary, error) {
	return nil, nil
}
func (m *memSpiritSessionRepo) LatestSessionSummaryTime(_ context.Context, _ string) (string, error) {
	return "", nil
}

// --- SummaryWriter ---

func (m *memSpiritSessionRepo) InsertSessionSummary(_ context.Context, _ SessionSummary) error {
	return nil
}
func (m *memSpiritSessionRepo) DeleteSessionSummaries(_ context.Context, _ string) error {
	return nil
}
func (m *memSpiritSessionRepo) UpdateSessionListSummary(_ context.Context, _, _ string) error {
	return nil
}
func (m *memSpiritSessionRepo) SessionSummaryExists(_ context.Context, _ string, _, _ int) (bool, error) {
	return false, nil
}

// --- StateRepo ---

func (m *memSpiritSessionRepo) GetSessionState(_ context.Context, _ string) (map[string]string, error) {
	return nil, nil
}
func (m *memSpiritSessionRepo) SaveSessionState(_ context.Context, _ string, _ map[string]string) error {
	return nil
}
func (m *memSpiritSessionRepo) PatchSessionState(_ context.Context, _ string, _ map[string]string, _ []string) error {
	return nil
}

// --- TurnRepo ---

func (m *memSpiritSessionRepo) CreateSessionTurn(_ context.Context, turn SessionTurn) (SessionTurn, error) {
	return turn, nil
}
func (m *memSpiritSessionRepo) UpdateSessionTurn(_ context.Context, _ string, _ SessionTurnUpdateFields) (SessionTurn, error) {
	return SessionTurn{}, nil
}
func (m *memSpiritSessionRepo) ListSessionTurns(_ context.Context, _ string, _, _ int) (SessionTurnListResult, error) {
	return SessionTurnListResult{}, nil
}
func (m *memSpiritSessionRepo) GetSessionTurn(_ context.Context, _ string) (SessionTurn, error) {
	return SessionTurn{}, nil
}

// --- ContextUpdater ---

func (m *memSpiritSessionRepo) UpdateRunnerSnapshotJSON(_ context.Context, _, _ string) error {
	return nil
}
func (m *memSpiritSessionRepo) UpdateSessionContextFromLLMUsage(_ context.Context, _ string, _, _, _ int) error {
	return nil
}
func (m *memSpiritSessionRepo) UpdateSessionContextAfterCompression(_ context.Context, _ string, _, _ int) error {
	return nil
}
func (m *memSpiritSessionRepo) IncrementInvocationCounts(_ context.Context, _ string, _, _, _ int) error {
	return nil
}
func (m *memSpiritSessionRepo) ApplyMetricsDelta(_ context.Context, _ *session.SessionMetricsDelta) error {
	return nil
}

// --- CompressRepo ---

func (m *memSpiritSessionRepo) TryIncrementCompressVersion(_ context.Context, _ string) (int64, error) {
	return 0, nil
}
func (m *memSpiritSessionRepo) CompressSessionInTx(_ context.Context, _ string, fn func(ctx context.Context) error) error {
	return fn(context.Background())
}

// memSpiritTransactor tracks transaction calls for testing.
// snapshot/restore hooks let tests simulate rollback semantics that the
// in-memory repos lack natively (a real Ent tx rolls back DB writes on error).
type memSpiritTransactor struct {
	callCount int
	fn        func(ctx context.Context) error // the actual function to execute
	snapshot  func()                          // optional: capture repo state before fn
	restore   func()                          // optional: restore repo state when fn errors
}

func (t *memSpiritTransactor) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	t.callCount++
	t.fn = fn
	if t.snapshot != nil {
		t.snapshot()
	}
	err := fn(ctx)
	if err != nil && t.restore != nil {
		t.restore()
	}
	return err
}

// ---------------------------------------------------------------------------
// T1.1: DAG root node initial status tests
// ---------------------------------------------------------------------------

func TestAssembleTeam_InitialStatus_Pending(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	sessionRepo := newMemSpiritSessionRepo()
	sessionRepo.seedSpirit("spirit-1")
	transactor := &memSpiritTransactor{}

	teamUC := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, RunWriter: teamRepo, StepRepo: teamRepo, DeadLetter: teamRepo, Lg: loggateway.NewNoop()})
	sessionUC := NewSessionUsecase(sessionRepo, &memSpiritAgentLookup{}, NewSessionTeamLookup(teamRepo), nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	uc := NewSpiritTeamUsecase(teamUC, sessionUC, &memSpiritAgentResolver{}, loggateway.NewNoop(), WithSpiritTransactor(transactor))

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
	sessionRepo.seedSpirit("spirit-1")
	transactor := &memSpiritTransactor{}

	teamUC := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, RunWriter: teamRepo, StepRepo: teamRepo, DeadLetter: teamRepo, Lg: loggateway.NewNoop()})
	sessionUC := NewSessionUsecase(sessionRepo, &memSpiritAgentLookup{}, NewSessionTeamLookup(teamRepo), nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	uc := NewSpiritTeamUsecase(teamUC, sessionUC, &memSpiritAgentResolver{}, loggateway.NewNoop(), WithSpiritTransactor(transactor))

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

// TestAssembleTeam_PersistsDeliverableContracts (P1 形式契约 B.10.15.2)
// verifies SpiritTeamParams contracts are serialized onto the Team record so
// dagRun advisory validation and downstream injection can read them.
func TestAssembleTeam_PersistsDeliverableContracts(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	sessionRepo := newMemSpiritSessionRepo()
	sessionRepo.seedSpirit("spirit-1")
	transactor := &memSpiritTransactor{}

	teamUC := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, RunWriter: teamRepo, StepRepo: teamRepo, DeadLetter: teamRepo, Lg: loggateway.NewNoop()})
	sessionUC := NewSessionUsecase(sessionRepo, &memSpiritAgentLookup{}, NewSessionTeamLookup(teamRepo), nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	uc := NewSpiritTeamUsecase(teamUC, sessionUC, &memSpiritAgentResolver{}, loggateway.NewNoop(), WithSpiritTransactor(transactor))

	result, err := uc.AssembleTeam(context.Background(), SpiritTeamParams{
		SpiritSessionID: "spirit-1",
		TaskDescription: "research task",
		AgentKeys:       []string{"agent-1"},
		Mode:            "coordinator",
		DagNodeID:       "node-1",
		Deliverables: []DeliverableContract{
			{Name: "research_report", Type: "document", Format: "markdown", Description: "调研报告"},
		},
		InputContract: []DeliverableContract{
			{Name: "brief", Type: "document", Format: "markdown"},
		},
	})
	if err != nil {
		t.Fatalf("AssembleTeam failed: %v", err)
	}
	deliverables, err := ParseDeliverableContracts(result.Team.Deliverables)
	if err != nil {
		t.Fatalf("Team.Deliverables not parseable: %v (raw=%q)", err, result.Team.Deliverables)
	}
	if len(deliverables) != 1 || deliverables[0].Name != "research_report" || deliverables[0].Type != "document" || deliverables[0].Format != "markdown" {
		t.Fatalf("Team.Deliverables = %+v, want research_report/document/markdown", deliverables)
	}
	inputs, err := ParseDeliverableContracts(result.Team.InputContract)
	if err != nil {
		t.Fatalf("Team.InputContract not parseable: %v (raw=%q)", err, result.Team.InputContract)
	}
	if len(inputs) != 1 || inputs[0].Name != "brief" {
		t.Fatalf("Team.InputContract = %+v, want brief", inputs)
	}
}

// ---------------------------------------------------------------------------
// T1.2: TransitionStatus state machine enforcement tests
// ---------------------------------------------------------------------------

func TestTransitionStatus_ValidTransitions(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	uc := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, RunWriter: teamRepo, StepRepo: teamRepo, DeadLetter: teamRepo, Lg: loggateway.NewNoop()})
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
	uc := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, RunWriter: teamRepo, StepRepo: teamRepo, DeadLetter: teamRepo, Lg: loggateway.NewNoop()})
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

	// pending → interrupted (invalid)
	_, err = uc.TransitionStatus(ctx, team.ID, TeamStatusInterrupted)
	if err == nil {
		t.Error("pending → interrupted should fail")
	}

	// pending → failed is a legal transition (B-01: DAG dependency failure or
	// pending timeout marks the team failed without executing).
	if _, err = uc.TransitionStatus(ctx, team.ID, TeamStatusFailed); err != nil {
		t.Errorf("pending → failed should succeed (B-01): %v", err)
	}
	// failed → pending (recover) to continue the scenario.
	if _, err = uc.TransitionStatus(ctx, team.ID, TeamStatusPending); err != nil {
		t.Fatalf("failed → pending (recover) should succeed: %v", err)
	}

	// Transition to running first
	if _, err = uc.TransitionStatus(ctx, team.ID, TeamStatusRunning); err != nil {
		t.Fatalf("pending → running should succeed: %v", err)
	}

	// running → pending is a legal transition (B-02: rework after the
	// verification gate rejects the team's output).
	if _, err = uc.TransitionStatus(ctx, team.ID, TeamStatusPending); err != nil {
		t.Errorf("running → pending should succeed (B-02 rework): %v", err)
	}

	// Back to running, then to completed
	if _, err = uc.TransitionStatus(ctx, team.ID, TeamStatusRunning); err != nil {
		t.Fatalf("pending → running should succeed: %v", err)
	}
	if _, err = uc.TransitionStatus(ctx, team.ID, TeamStatusCompleted); err != nil {
		t.Fatalf("running → completed should succeed: %v", err)
	}

	// completed → running (invalid: terminal state)
	_, err = uc.TransitionStatus(ctx, team.ID, TeamStatusRunning)
	if err == nil {
		t.Error("completed → running should fail (terminal state)")
	}
}

func TestTransitionStatus_InterruptedRecovery(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	uc := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, RunWriter: teamRepo, StepRepo: teamRepo, DeadLetter: teamRepo, Lg: loggateway.NewNoop()})
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
	sessionRepo.seedSpirit("spirit-cancel", "spirit-cancel2")
	transactor := &memSpiritTransactor{}

	teamUC := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, RunWriter: teamRepo, StepRepo: teamRepo, DeadLetter: teamRepo, Lg: loggateway.NewNoop()})
	sessionUC := NewSessionUsecase(sessionRepo, &memSpiritAgentLookup{}, NewSessionTeamLookup(teamRepo), nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	uc := NewSpiritTeamUsecase(teamUC, sessionUC, &memSpiritAgentResolver{}, loggateway.NewNoop(), WithSpiritTransactor(transactor))

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

	// Cancelling a completed team must fail: completed has no outgoing
	// transition to cancelled (only → archived) in the Team state machine.
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
	sessionRepo.seedSpirit("spirit-tx")
	// Simulate real transaction rollback: snapshot teamRepo before fn runs and
	// restore it when fn returns an error (in-memory repos have no native tx).
	var teamSnapshot map[string]Team
	transactor := &memSpiritTransactor{
		snapshot: func() {
			teamSnapshot = make(map[string]Team, len(teamRepo.items))
			for k, v := range teamRepo.items {
				teamSnapshot[k] = v
			}
		},
		restore: func() { teamRepo.items = teamSnapshot },
	}

	teamUC := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, RunWriter: teamRepo, StepRepo: teamRepo, DeadLetter: teamRepo, Lg: loggateway.NewNoop()})
	sessionUC := NewSessionUsecase(sessionRepo, &memSpiritAgentLookup{}, NewSessionTeamLookup(teamRepo), nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	uc := NewSpiritTeamUsecase(teamUC, sessionUC, &memSpiritAgentResolver{}, loggateway.NewNoop(), WithSpiritTransactor(transactor))

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

	// Verify team was NOT persisted (rolled back).
	// The transactor's snapshot/restore hooks simulate what a real Ent
	// transaction guarantees: DB writes are rolled back when fn errors.
	teamsAfter, _ := teamRepo.ListTeams(ctx)
	if len(teamsAfter) != countBefore {
		t.Errorf("team count should not change after rollback: before=%d, after=%d", countBefore, len(teamsAfter))
	}
}

func TestAssembleTeam_TransactionSuccess(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	sessionRepo := newMemSpiritSessionRepo()
	sessionRepo.seedSpirit("spirit-tx-ok")
	transactor := &memSpiritTransactor{}

	teamUC := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, RunWriter: teamRepo, StepRepo: teamRepo, DeadLetter: teamRepo, Lg: loggateway.NewNoop()})
	sessionUC := NewSessionUsecase(sessionRepo, &memSpiritAgentLookup{}, NewSessionTeamLookup(teamRepo), nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	uc := NewSpiritTeamUsecase(teamUC, sessionUC, &memSpiritAgentResolver{}, loggateway.NewNoop(), WithSpiritTransactor(transactor))

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

// ---------------------------------------------------------------------------
// C1/C3: enable_state_deliverable auto-enable for multi-member teams
// ---------------------------------------------------------------------------

func TestBuildSpiritTeamDefinitionJSON_MultiMember_EnableStateDeliverable(t *testing.T) {
	defJSON := buildSpiritTeamDefinitionJSON("coordinator", []string{"agent-a", "agent-b"}, loggateway.NewNoop(), false)

	var def map[string]any
	if err := json.Unmarshal([]byte(defJSON), &def); err != nil {
		t.Fatalf("definition JSON not parseable: %v", err)
	}
	v, ok := def["enable_state_deliverable"]
	if !ok {
		t.Fatal("multi-member team should have enable_state_deliverable")
	}
	if v != true {
		t.Fatalf("enable_state_deliverable should be true, got %v", v)
	}
}

func TestBuildSpiritTeamDefinitionJSON_SingleMember_NoEnableStateDeliverable(t *testing.T) {
	defJSON := buildSpiritTeamDefinitionJSON("coordinator", []string{"agent-a"}, loggateway.NewNoop(), false)

	var def map[string]any
	if err := json.Unmarshal([]byte(defJSON), &def); err != nil {
		t.Fatalf("definition JSON not parseable: %v", err)
	}
	if _, ok := def["enable_state_deliverable"]; ok {
		t.Fatal("single-member team should NOT have enable_state_deliverable")
	}
}

// 2026-07-25 Fix 2b：DAG 团队（requireDeliverable=true）即使单成员也必须开启
// enable_state_deliverable —— 没有交付通道，Fix 1 的真实产出闸门会把所有
// 单成员 DAG 团队误判为 failed。
func TestBuildSpiritTeamDefinitionJSON_DAGSingleMember_EnableStateDeliverable(t *testing.T) {
	defJSON := buildSpiritTeamDefinitionJSON("coordinator", []string{"agent-a"}, loggateway.NewNoop(), true)

	var def map[string]any
	if err := json.Unmarshal([]byte(defJSON), &def); err != nil {
		t.Fatalf("definition JSON not parseable: %v", err)
	}
	v, ok := def["enable_state_deliverable"]
	if !ok {
		t.Fatal("DAG single-member team should have enable_state_deliverable")
	}
	if v != true {
		t.Fatalf("enable_state_deliverable should be true, got %v", v)
	}
}

// 2026-07-28 Fix: a single-member "coordinator" team is degenerate — the only
// member was assigned role=synthesizer (a coordination role that does not
// execute tools), so the agent hallucinated task completion with zero tool
// calls and fake set_deliverable output. Single-member teams must normalize
// to sequential mode so the member is built as a worker.
func TestAssembleTeam_SingleMemberCoordinator_NormalizesToSequentialWorker(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	sessionRepo := newMemSpiritSessionRepo()
	sessionRepo.seedSpirit("spirit-1")
	transactor := &memSpiritTransactor{}

	teamUC := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, RunWriter: teamRepo, StepRepo: teamRepo, DeadLetter: teamRepo, Lg: loggateway.NewNoop()})
	sessionUC := NewSessionUsecase(sessionRepo, &memSpiritAgentLookup{}, NewSessionTeamLookup(teamRepo), nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	uc := NewSpiritTeamUsecase(teamUC, sessionUC, &memSpiritAgentResolver{}, loggateway.NewNoop(), WithSpiritTransactor(transactor))

	result, err := uc.AssembleTeam(context.Background(), SpiritTeamParams{
		SpiritSessionID: "spirit-1",
		TaskDescription: "install skill from url",
		AgentKeys:       []string{"agent-1"},
		Mode:            "coordinator",
		AutoStart:       false,
	})
	if err != nil {
		t.Fatalf("AssembleTeam failed: %v", err)
	}

	var def struct {
		Mode    string `json:"mode"`
		Members []struct {
			Role string `json:"role"`
		} `json:"members"`
	}
	if err := json.Unmarshal([]byte(result.Team.DefinitionJSON), &def); err != nil {
		t.Fatalf("definition JSON not parseable: %v", err)
	}
	if def.Mode != TeamModeSequential {
		t.Errorf("single-member team mode should normalize to sequential, got %q", def.Mode)
	}
	if len(def.Members) != 1 {
		t.Fatalf("expected 1 member, got %d", len(def.Members))
	}
	if def.Members[0].Role != RoleWorker {
		t.Errorf("single-member team member must be worker, got %q", def.Members[0].Role)
	}
	if result.Team.Topology != TeamModeSequential {
		t.Errorf("team topology should be sequential, got %q", result.Team.Topology)
	}
}

func TestAssembleTeam_MultiMember_DefinitionJSON_HasEnableStateDeliverable(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	sessionRepo := newMemSpiritSessionRepo()
	sessionRepo.seedSpirit("spirit-esd")
	transactor := &memSpiritTransactor{}

	teamUC := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, RunWriter: teamRepo, StepRepo: teamRepo, DeadLetter: teamRepo, Lg: loggateway.NewNoop()})
	sessionUC := NewSessionUsecase(sessionRepo, &memSpiritAgentLookup{}, NewSessionTeamLookup(teamRepo), nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	uc := NewSpiritTeamUsecase(teamUC, sessionUC, &memSpiritAgentResolver{}, loggateway.NewNoop(), WithSpiritTransactor(transactor))

	result, err := uc.AssembleTeam(context.Background(), SpiritTeamParams{
		SpiritSessionID: "spirit-esd",
		TaskDescription: "multi-member deliverable test",
		AgentKeys:       []string{"agent-1", "agent-2"},
		Mode:            "coordinator",
		AutoStart:       false,
	})
	if err != nil {
		t.Fatalf("AssembleTeam failed: %v", err)
	}

	var def map[string]any
	if err := json.Unmarshal([]byte(result.Team.DefinitionJSON), &def); err != nil {
		t.Fatalf("DefinitionJSON not parseable: %v (raw=%q)", err, result.Team.DefinitionJSON)
	}
	v, ok := def["enable_state_deliverable"]
	if !ok {
		t.Fatal("multi-member team DefinitionJSON should have enable_state_deliverable")
	}
	if v != true {
		t.Fatalf("enable_state_deliverable should be true, got %v", v)
	}
}

// 2026-07-25 Fix 2b：DAG 团队（DagNodeID 非空）单成员也必须开启
// enable_state_deliverable，否则真实产出闸门必然误判 failed。
func TestAssembleTeam_DAGSingleMember_DefinitionJSON_HasEnableStateDeliverable(t *testing.T) {
	teamRepo := newMemSpiritTeamRepo()
	sessionRepo := newMemSpiritSessionRepo()
	sessionRepo.seedSpirit("spirit-esd-dag")
	transactor := &memSpiritTransactor{}

	teamUC := NewTeamUsecase(TeamUsecaseOpts{Reader: teamRepo, Writer: teamRepo, RunReader: teamRepo, RunWriter: teamRepo, StepRepo: teamRepo, DeadLetter: teamRepo, Lg: loggateway.NewNoop()})
	sessionUC := NewSessionUsecase(sessionRepo, &memSpiritAgentLookup{}, NewSessionTeamLookup(teamRepo), nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	uc := NewSpiritTeamUsecase(teamUC, sessionUC, &memSpiritAgentResolver{}, loggateway.NewNoop(), WithSpiritTransactor(transactor))

	result, err := uc.AssembleTeam(context.Background(), SpiritTeamParams{
		SpiritSessionID: "spirit-esd-dag",
		TaskDescription: "dag single-member deliverable test",
		AgentKeys:       []string{"agent-1"},
		Mode:            "coordinator",
		AutoStart:       false,
		DagNodeID:       "st_1",
	})
	if err != nil {
		t.Fatalf("AssembleTeam failed: %v", err)
	}

	var def map[string]any
	if err := json.Unmarshal([]byte(result.Team.DefinitionJSON), &def); err != nil {
		t.Fatalf("DefinitionJSON not parseable: %v (raw=%q)", err, result.Team.DefinitionJSON)
	}
	v, ok := def["enable_state_deliverable"]
	if !ok {
		t.Fatal("DAG single-member team DefinitionJSON should have enable_state_deliverable")
	}
	if v != true {
		t.Fatalf("enable_state_deliverable should be true, got %v", v)
	}
}
