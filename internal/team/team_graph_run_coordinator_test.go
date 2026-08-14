package team

import (
	"context"
	"errors"
	"testing"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type stubCoordEventPublisher struct {
	published []biz.Event
}

func (s *stubCoordEventPublisher) Publish(_ context.Context, e biz.Event) {
	s.published = append(s.published, e)
}

type memTeamRunRepoCoord struct {
	runs map[string]biz.TeamRunRecord
}

func (m *memTeamRunRepoCoord) ListTeamRuns(context.Context, string, int) ([]biz.TeamRunRecord, error) {
	return nil, nil
}
func (m *memTeamRunRepoCoord) ListTeamRunsByTeamIDs(context.Context, []string, int) (map[string][]biz.TeamRunRecord, error) {
	return nil, nil
}
func (m *memTeamRunRepoCoord) HasActiveTeamRun(context.Context, string) (bool, error) {
	return false, nil
}
func (m *memTeamRunRepoCoord) GetTeamRunByID(_ context.Context, id string) (biz.TeamRunRecord, error) {
	if r, ok := m.runs[id]; ok {
		return r, nil
	}
	return biz.TeamRunRecord{}, biz.ErrNotFound
}
func (m *memTeamRunRepoCoord) ListTeamRunSteps(context.Context, string) ([]biz.TeamRunStep, error) {
	return nil, nil
}
func (m *memTeamRunRepoCoord) CreateTeamRun(_ context.Context, r biz.TeamRunRecord) (biz.TeamRunRecord, error) {
	if m.runs == nil {
		m.runs = map[string]biz.TeamRunRecord{}
	}
	m.runs[r.ID] = r
	return r, nil
}
func (m *memTeamRunRepoCoord) UpdateTeamRun(_ context.Context, r biz.TeamRunRecord) error {
	if m.runs == nil {
		m.runs = map[string]biz.TeamRunRecord{}
	}
	m.runs[r.ID] = r
	return nil
}
func (m *memTeamRunRepoCoord) UpdateTeamRunWhereStatus(_ context.Context, _, _, _ string) (bool, error) {
	return true, nil
}
func (m *memTeamRunRepoCoord) UpdateTeamRunGraphExecutionID(context.Context, string, string) error {
	return nil
}
func (m *memTeamRunRepoCoord) UpdateTeamRunTraceID(context.Context, string, string) error { return nil }
func (m *memTeamRunRepoCoord) UpdateTeamRunSummaryJSON(context.Context, string, string) error {
	return nil
}
func (m *memTeamRunRepoCoord) CreateTeamRunStep(context.Context, biz.TeamRunStep) (biz.TeamRunStep, error) {
	return biz.TeamRunStep{}, nil
}
func (m *memTeamRunRepoCoord) TransitionRunStatus(_ context.Context, runID string, newStatus string) (biz.TeamRunRecord, error) {
	r, ok := m.runs[runID]
	if !ok {
		return biz.TeamRunRecord{}, biz.ErrNotFound
	}
	r.Status = newStatus
	m.runs[runID] = r
	return r, nil
}

type memSessionRepo struct {
	sessions map[string]biz.TeamGraphSession
}

func newMemSessionRepo() *memSessionRepo {
	return &memSessionRepo{sessions: make(map[string]biz.TeamGraphSession)}
}

func (m *memSessionRepo) SaveSession(_ context.Context, s biz.TeamGraphSession) error {
	m.sessions[s.ExecID] = s
	return nil
}
func (m *memSessionRepo) UpdateSessionStatus(_ context.Context, execID, status string) error {
	if s, ok := m.sessions[execID]; ok {
		s.Status = status
		m.sessions[execID] = s
	}
	return nil
}
func (m *memSessionRepo) GetSession(_ context.Context, execID string) (biz.TeamGraphSession, error) {
	if s, ok := m.sessions[execID]; ok {
		return s, nil
	}
	return biz.TeamGraphSession{}, biz.ErrNotFound
}
func (m *memSessionRepo) ListActiveSessions(_ context.Context) ([]biz.TeamGraphSession, error) {
	var out []biz.TeamGraphSession
	for _, s := range m.sessions {
		if s.Status == biz.TeamRunStatusRunning || s.Status == biz.TeamRunStatusWaitingHuman {
			out = append(out, s)
		}
	}
	return out, nil
}
func (m *memSessionRepo) DeleteSession(_ context.Context, execID string) error {
	delete(m.sessions, execID)
	return nil
}
func (m *memSessionRepo) MarkOrphanedSessionsTerminal(_ context.Context) (int, error) {
	n := 0
	for id, s := range m.sessions {
		if s.Status == biz.TeamRunStatusRunning {
			s.Status = "cancelled"
			m.sessions[id] = s
			n++
		}
	}
	return n, nil
}

func TestShouldCreateTeamGraphTaskNode_onlyHumanNodes(t *testing.T) {
	if !biz.ShouldCreateTeamGraphTaskNode(&biz.NodeDef{Type: "review"}) {
		t.Fatal("review should create task")
	}
	if biz.ShouldCreateTeamGraphTaskNode(&biz.NodeDef{Type: "agent", AgentName: "a"}) {
		t.Fatal("agent should not create team graph task")
	}
}

type coordGraphBackend struct {
	repo *memGraphRunRepoCoord
	uc   *biz.GraphUsecase
}

type memGraphRunRepoCoord struct {
	runs map[string]*biz.GraphExecution
}

func (m *memGraphRunRepoCoord) SaveRun(_ context.Context, exec *biz.GraphExecution) error {
	if m.runs == nil {
		m.runs = map[string]*biz.GraphExecution{}
	}
	m.runs[exec.ID] = exec
	return nil
}

func (m *memGraphRunRepoCoord) GetRun(_ context.Context, id string) (*biz.GraphExecution, error) {
	if exec, ok := m.runs[id]; ok {
		return exec, nil
	}
	return nil, biz.ErrNotFound
}

func (m *memGraphRunRepoCoord) ListRunsByGraph(context.Context, string, int, string, ...biz.GraphRunListOption) ([]*biz.GraphExecution, string, error) {
	return nil, "", nil
}

func (m *memGraphRunRepoCoord) UpdateRun(_ context.Context, exec *biz.GraphExecution) error {
	if m.runs == nil {
		m.runs = map[string]*biz.GraphExecution{}
	}
	m.runs[exec.ID] = exec
	return nil
}

func (b *coordGraphBackend) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID string, ct *biz.CompiledTeam) error {
	return b.uc.RegisterTeamGraphExecution(ctx, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID, ct)
}

func (b *coordGraphBackend) MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error {
	return b.uc.MarkTeamGraphInterrupt(ctx, execID, nodeID, lineageID)
}

func (b *coordGraphBackend) ResumeExecution(ctx context.Context, executionID string, resumeValue map[string]any) (*biz.GraphExecution, error) {
	return b.uc.ResumeExecution(ctx, executionID, resumeValue)
}

func (b *coordGraphBackend) GetExecution(ctx context.Context, executionID string) (*biz.GraphExecution, error) {
	return b.uc.GetExecution(ctx, executionID)
}

func (b *coordGraphBackend) RecordTeamGraphNodeEnd(ctx context.Context, execID, nodeID string, stepIndex int, status, errMsg string) error {
	return b.uc.RecordTeamGraphNodeEnd(ctx, execID, nodeID, stepIndex, status, errMsg)
}

func (b *coordGraphBackend) FinalizeTeamGraphExecution(ctx context.Context, execID string, failed bool, errMsg string) error {
	return b.uc.FinalizeTeamGraphExecution(ctx, execID, failed, errMsg)
}

func newCoordTestBackend() *coordGraphBackend {
	repo := &memGraphRunRepoCoord{runs: map[string]*biz.GraphExecution{}}
	return &coordGraphBackend{repo: repo, uc: biz.NewGraphUsecase(biz.GraphUsecaseDeps{RunRepo: repo, Lg: loggateway.NewNoop()})}
}

func TestTeamGraphRunCoordinator_DeferTeamRunSuccessIfHITL(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{"run-1": {ID: "run-1", Status: biz.TeamRunStatusRunning}}}
	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, nil, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil, nil)
	if err := coord.RegisterTeamGraphExecution(context.Background(), "exec-1", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}
	if err := coord.MarkTeamGraphInterrupt(context.Background(), "exec-1", "review-1", "lineage-1"); err != nil {
		t.Fatal(err)
	}
	run := biz.TeamRunRecord{ID: "run-1", Status: biz.TeamRunStatusRunning}
	deferred, err := coord.DeferTeamRunSuccessIfHITL(context.Background(), "exec-1", &run)
	if err != nil || !deferred {
		t.Fatalf("deferred=%v err=%v run=%+v", deferred, err, run)
	}
	if run.Status != biz.TeamRunStatusWaitingHuman {
		t.Fatalf("status=%q", run.Status)
	}
}

func TestTeamGraphRunCoordinator_persistSession(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{"run-1": {ID: "run-1", Status: biz.TeamRunStatusRunning}}}
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, sessRepo, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}
	dbSess, err := sessRepo.GetSession(ctx, "exec-1")
	if err != nil {
		t.Fatalf("session not persisted: %v", err)
	}
	if dbSess.Status != biz.TeamRunStatusRunning {
		t.Fatalf("expected running, got %q", dbSess.Status)
	}
	if dbSess.TeamRunID != "run-1" {
		t.Fatalf("expected run-1, got %q", dbSess.TeamRunID)
	}
}

func TestTeamGraphRunCoordinator_evictDeletesFromDB(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusWaitingHuman}}}
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, sessRepo, nil, loggateway.NewNoop())
	ctx := context.Background()
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil, nil)

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}
	if _, err := sessRepo.GetSession(ctx, "exec-1"); err != nil {
		t.Fatalf("session should exist before evict: %v", err)
	}
	coord.finalizeTeamRun(ctx, coord.session("exec-1"), false, "")
	if _, err := sessRepo.GetSession(ctx, "exec-1"); err == nil {
		t.Fatal("session should be deleted after evict")
	}
}

func TestTeamGraphRunCoordinator_RecoverSessions(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{
		"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusWaitingHuman, DefinitionSnapshotJSON: `{"members":[{"agent_id":"a1","name":"Agent1"}],"mode":"pipeline"}`},
	}}
	sessRepo := newMemSessionRepo()
	sessRepo.SaveSession(context.Background(), biz.TeamGraphSession{
		ExecID:         "exec-1",
		TeamRunID:      "run-1",
		TeamID:         "team-1",
		SessionID:      "sess-1",
		Status:         biz.TeamRunStatusWaitingHuman,
		DefinitionJSON: `{"members":[{"agent_id":"a1","name":"Agent1"}],"mode":"pipeline"}`,
	})
	sessRepo.SaveSession(context.Background(), biz.TeamGraphSession{
		ExecID:    "exec-2",
		TeamRunID: "run-2",
		TeamID:    "team-1",
		SessionID: "sess-2",
		Status:    biz.TeamRunStatusRunning,
	})

	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, sessRepo, nil, loggateway.NewNoop())
	coord.RecoverSessions(context.Background())

	sess := coord.session("exec-1")
	if sess == nil {
		t.Fatal("waiting_human session should be recovered")
	}
	if sess.teamRunID != "run-1" {
		t.Fatalf("expected run-1, got %q", sess.teamRunID)
	}
	sess2 := coord.session("exec-2")
	if sess2 != nil {
		t.Fatal("running session should not be recovered (orphaned)")
	}
}

func TestTeamGraphRunCoordinator_MarkInterruptUpdatesDB(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{"run-1": {ID: "run-1", Status: biz.TeamRunStatusRunning}}}
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, sessRepo, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}
	if err := coord.MarkTeamGraphInterrupt(ctx, "exec-1", "review-1", "lineage-1"); err != nil {
		t.Fatal(err)
	}
	dbSess, err := sessRepo.GetSession(ctx, "exec-1")
	if err != nil {
		t.Fatalf("session not found in DB: %v", err)
	}
	if dbSess.Status != biz.TeamRunStatusWaitingHuman {
		t.Fatalf("expected waiting_human, got %q", dbSess.Status)
	}
}

// Y1/N3: a HITL interrupt arrives at the watch as a graph_stage "step" notice
// carrying interrupt metadata (the Pregel interrupt is the only reachable
// carrier). The watch must mark the team run waiting_human — today the notice
// is silently dropped and the run stays running until the watch times out.
func TestHandleGraphWatchNotice_Interrupt_MarksWaitingHuman(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{"run-1": {ID: "run-1", Status: biz.TeamRunStatusRunning}}}
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, sessRepo, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil, nil)
	ctx := context.Background()
	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}
	sess := coord.session("exec-1")

	notice := biz.NewSystemNoticeEvent("sess-1", "step", "", map[string]any{
		"activity_kind": string(biz.ActivityKindGraphStage),
		"execution_id":  "exec-1",
		"interrupt_key": "hitl",
		"node_id":       "review-1",
		"lineage_id":    "lineage-1",
	})
	done, failed, _ := coord.handleGraphWatchNotice(ctx, sess, notice, graphWatchStepsAndFinalize)
	if done || failed {
		t.Fatalf("interrupt must not finalize the watch: done=%v failed=%v", done, failed)
	}
	run, err := repo.GetTeamRunByID(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != biz.TeamRunStatusWaitingHuman {
		t.Fatalf("team run must transition to waiting_human on interrupt, got %q", run.Status)
	}
}

// N3: a graph-level fatal error (Pregel error: max steps / panic) arrives as a
// graph_stage "step" notice carrying an error key. In finalize mode the watch
// must converge the run to failed immediately instead of waiting for the
// 30-minute watch timeout.
func TestHandleGraphWatchNotice_PregelError_ReturnsFailed(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{"run-1": {ID: "run-1", Status: biz.TeamRunStatusRunning}}}
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, sessRepo, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil, nil)
	ctx := context.Background()
	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}
	sess := coord.session("exec-1")

	notice := biz.NewSystemNoticeEvent("sess-1", "step", "graph execution exceeded max steps", map[string]any{
		"activity_kind": string(biz.ActivityKindGraphStage),
		"execution_id":  "exec-1",
		"error":         "graph execution exceeded max steps",
	})
	done, failed, errMsg := coord.handleGraphWatchNotice(ctx, sess, notice, graphWatchStepsAndFinalize)
	if !done || !failed {
		t.Fatalf("pregel error must finalize as failed: done=%v failed=%v", done, failed)
	}
	if errMsg == "" {
		t.Fatal("errMsg must carry the pregel error")
	}
}

func TestTeamGraphRunCoordinator_CleanupStaleDeletesFromDB(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusRunning}}}
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, sessRepo, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}
	sess := coord.session("exec-1")
	sess.registeredAt = time.Now().Add(-3 * time.Hour)

	coord.CleanupStaleSessions()

	if _, err := sessRepo.GetSession(ctx, "exec-1"); err == nil {
		t.Fatal("stale session should be deleted from DB after cleanup")
	}
}

type failingResumeBackend struct {
	inner TeamGraphExecutionBackend
}

func (b *failingResumeBackend) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID string, ct *biz.CompiledTeam) error {
	return b.inner.RegisterTeamGraphExecution(ctx, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID, ct)
}
func (b *failingResumeBackend) MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error {
	return b.inner.MarkTeamGraphInterrupt(ctx, execID, nodeID, lineageID)
}
func (b *failingResumeBackend) ResumeExecution(context.Context, string, map[string]any) (*biz.GraphExecution, error) {
	return nil, errors.New("resume failed")
}
func (b *failingResumeBackend) RecordTeamGraphNodeEnd(ctx context.Context, execID, nodeID string, stepIndex int, status, errMsg string) error {
	return b.inner.RecordTeamGraphNodeEnd(ctx, execID, nodeID, stepIndex, status, errMsg)
}
func (b *failingResumeBackend) FinalizeTeamGraphExecution(ctx context.Context, execID string, failed bool, errMsg string) error {
	return b.inner.FinalizeTeamGraphExecution(ctx, execID, failed, errMsg)
}
func (b *failingResumeBackend) GetExecution(ctx context.Context, executionID string) (*biz.GraphExecution, error) {
	return b.inner.GetExecution(ctx, executionID)
}

type succeedingResumeBackend struct {
	inner TeamGraphExecutionBackend
}

func (b *succeedingResumeBackend) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID string, ct *biz.CompiledTeam) error {
	return b.inner.RegisterTeamGraphExecution(ctx, execID, sessionID, spiritSessionID, teamID, teamRunID, linkedGraphID, ct)
}
func (b *succeedingResumeBackend) MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error {
	return b.inner.MarkTeamGraphInterrupt(ctx, execID, nodeID, lineageID)
}
func (b *succeedingResumeBackend) RecordTeamGraphNodeEnd(ctx context.Context, execID, nodeID string, stepIndex int, status, errMsg string) error {
	return b.inner.RecordTeamGraphNodeEnd(ctx, execID, nodeID, stepIndex, status, errMsg)
}
func (b *succeedingResumeBackend) FinalizeTeamGraphExecution(ctx context.Context, execID string, failed bool, errMsg string) error {
	return b.inner.FinalizeTeamGraphExecution(ctx, execID, failed, errMsg)
}
func (b *succeedingResumeBackend) ResumeExecution(ctx context.Context, executionID string, resumeValue map[string]any) (*biz.GraphExecution, error) {
	exec, err := b.inner.GetExecution(ctx, executionID)
	if err != nil {
		return nil, err
	}
	exec.Status = biz.TeamRunStatusRunning
	return exec, nil
}
func (b *succeedingResumeBackend) GetExecution(ctx context.Context, executionID string) (*biz.GraphExecution, error) {
	return b.inner.GetExecution(ctx, executionID)
}

func TestShouldResumeTeamGraph(t *testing.T) {
	if shouldResumeTeamGraph(nil, "node-1") {
		t.Fatal("nil exec should not resume")
	}
	if shouldResumeTeamGraph(&biz.GraphExecution{Status: biz.TeamRunStatusWaitingHuman, InterruptNode: "node-1"}, "") {
		t.Fatal("empty nodeID should not resume")
	}
	if !shouldResumeTeamGraph(&biz.GraphExecution{Status: biz.TeamRunStatusWaitingHuman, InterruptNode: "node-1"}, "node-1") {
		t.Fatal("waiting_human + matching InterruptNode should resume")
	}
	if !shouldResumeTeamGraph(&biz.GraphExecution{Status: biz.TeamRunStatusWaitingHuman, CurrentNode: "node-1"}, "node-1") {
		t.Fatal("waiting_human + matching CurrentNode should resume")
	}
	if shouldResumeTeamGraph(&biz.GraphExecution{Status: biz.TeamRunStatusRunning, InterruptNode: "node-1"}, "node-1") {
		t.Fatal("running + matching InterruptNode should not resume")
	}
	if !shouldResumeTeamGraph(&biz.GraphExecution{GraphID: "team:abc", InterruptNode: "node-1"}, "node-1") {
		t.Fatal("team: prefixed GraphID + matching InterruptNode should resume")
	}
	// B9：真实资产 ID（无 team: 前缀）的 team 执行同样可 resume。
	if !shouldResumeTeamGraph(&biz.GraphExecution{GraphID: "g-asset-1", InterruptNode: "node-1"}, "node-1") {
		t.Fatal("real asset GraphID + matching InterruptNode should resume")
	}
}

func TestTeamGraphRunCoordinator_HandleTaskCompleted_ResumeFail(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusWaitingHuman}}}
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(&failingResumeBackend{inner: backend}, repo, repo, repo, nil, nil, sessRepo, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}
	if err := coord.MarkTeamGraphInterrupt(ctx, "exec-1", "review-1", "lineage-1"); err != nil {
		t.Fatal(err)
	}
	task := &biz.GraphTask{TaskID: "task-1", ExecutionID: "exec-1", NodeID: "review-1"}
	handled, err := coord.HandleTeamGraphTaskCompleted(ctx, task, nil)
	if !handled {
		t.Fatal("should be handled")
	}
	if err == nil {
		t.Fatal("expected error from ResumeExecution")
	}
	run, _ := repo.GetTeamRunByID(ctx, "run-1")
	if run.Status != biz.TeamRunStatusFailed {
		t.Fatalf("expected failed, got %q", run.Status)
	}
	if coord.session("exec-1") != nil {
		t.Fatal("session should be evicted after resume failure")
	}
}

// TestTeamGraphRunCoordinator_ResumeFailTeamStageID_RunIsolated verifies the
// TeamStage failed event emitted on the resume-failure path uses the
// run-isolated stage ID captured at registration — NOT a RootTaskActivityID
// lookup on the resume ctx (which originates from the task-completion handler
// and never carries one, see startGraphWatch's emitter comment).
// S-3（2026-08-05）：wrong ID → upsert writes a second team_stages_v2 row
// while the real stage row stays non-terminal forever.
func TestTeamGraphRunCoordinator_ResumeFailTeamStageID_RunIsolated(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusWaitingHuman}}}
	sessRepo := newMemSessionRepo()
	seq := &stubCoordEventPublisher{}
	coord := NewTeamGraphRunCoordinator(&failingResumeBackend{inner: backend}, repo, repo, repo, nil, seq, sessRepo, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil, nil)

	// Registration ctx carries the run's RootTaskActivityID (runner run ctx).
	regCtx := chatagent.ContextWithRootTaskActivityID(context.Background(), chatagent.RootTaskActivityID("root-task-1"))
	if err := coord.RegisterTeamGraphExecution(regCtx, "exec-1", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}
	if err := coord.MarkTeamGraphInterrupt(regCtx, "exec-1", "review-1", "lineage-1"); err != nil {
		t.Fatal(err)
	}
	// Resume ctx comes from the task-completion handler — no RootTaskActivityID.
	task := &biz.GraphTask{TaskID: "task-1", ExecutionID: "exec-1", NodeID: "review-1"}
	if _, err := coord.HandleTeamGraphTaskCompleted(context.Background(), task, nil); err == nil {
		t.Fatal("expected error from ResumeExecution")
	}
	wantID := string(chatagent.NewTeamStageActivityID("team-1", "root-task-1"))
	var stageEvent *biz.TeamStageFailedEvent
	for _, e := range seq.published {
		if fe, ok := e.(*biz.TeamStageFailedEvent); ok {
			stageEvent = fe
		}
	}
	if stageEvent == nil {
		t.Fatal("expected TeamStageFailedEvent published")
	}
	if got := stageEvent.TeamStage.ID; got != wantID {
		t.Errorf("TeamStage.ID = %q, want run-isolated %q", got, wantID)
	}
}

func TestTeamGraphRunCoordinator_HandleTaskCompleted_ResumeSuccess(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusWaitingHuman}}}
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(&succeedingResumeBackend{inner: backend}, repo, repo, repo, nil, nil, sessRepo, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}
	if err := coord.MarkTeamGraphInterrupt(ctx, "exec-1", "review-1", "lineage-1"); err != nil {
		t.Fatal(err)
	}
	task := &biz.GraphTask{TaskID: "task-1", ExecutionID: "exec-1", NodeID: "review-1"}
	handled, err := coord.HandleTeamGraphTaskCompleted(ctx, task, map[string]any{"output": "done"})
	if !handled {
		t.Fatal("should be handled")
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	run, _ := repo.GetTeamRunByID(ctx, "run-1")
	if run.Status != biz.TeamRunStatusRunning {
		t.Fatalf("expected running, got %q", run.Status)
	}
}

func TestTeamGraphRunCoordinator_RegisterExecutionIdempotent(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{"run-1": {ID: "run-1", Status: biz.TeamRunStatusRunning}}}
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, sessRepo, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}
	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}
	sess := coord.session("exec-1")
	if sess == nil {
		t.Fatal("session should exist after double register")
	}
	if sess.execID != "exec-1" {
		t.Fatalf("expected exec-1, got %q", sess.execID)
	}
	dbSess, err := sessRepo.GetSession(ctx, "exec-1")
	if err != nil {
		t.Fatalf("session not in DB: %v", err)
	}
	if dbSess.ExecID != "exec-1" {
		t.Fatalf("expected exec-1, got %q", dbSess.ExecID)
	}
}

func TestTeamGraphRunCoordinator_DeferTeamRunSuccessIfHITL_NoInterrupt(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{"run-1": {ID: "run-1", Status: biz.TeamRunStatusRunning}}}
	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, nil, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}
	run := biz.TeamRunRecord{ID: "run-1", Status: biz.TeamRunStatusRunning}
	deferred, err := coord.DeferTeamRunSuccessIfHITL(ctx, "exec-1", &run)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deferred {
		t.Fatal("should not defer when no interrupt marked")
	}
}

func TestTeamGraphRunCoordinator_CleanupStale_LeavesActiveSessions(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRunRepoCoord{runs: map[string]biz.TeamRunRecord{"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusRunning}}}
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(backend, repo, repo, repo, nil, nil, sessRepo, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "sess-1", "team-1", "run-1", "", ct); err != nil {
		t.Fatal(err)
	}
	coord.CleanupStaleSessions()
	if _, err := sessRepo.GetSession(ctx, "exec-1"); err != nil {
		t.Fatal("fresh session should not be cleaned up")
	}
}
