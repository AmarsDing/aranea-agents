package team

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

type memTeamRepoCoord struct {
	runs map[string]biz.TeamRun
}

func (m *memTeamRepoCoord) ListTeams(context.Context) ([]biz.Team, error) { return nil, nil }
func (m *memTeamRepoCoord) GetTeamByID(context.Context, string) (biz.Team, error) {
	return biz.Team{}, biz.ErrNotFound
}
func (m *memTeamRepoCoord) CreateTeam(context.Context, biz.Team) (biz.Team, error) { return biz.Team{}, nil }
func (m *memTeamRepoCoord) UpdateTeam(context.Context, biz.Team) (biz.Team, error) { return biz.Team{}, nil }
func (m *memTeamRepoCoord) DeleteTeam(context.Context, string) error { return nil }
func (m *memTeamRepoCoord) ListTeamRuns(context.Context, string, int) ([]biz.TeamRun, error) {
	return nil, nil
}
func (m *memTeamRepoCoord) HasActiveTeamRun(context.Context, string) (bool, error) {
	return false, nil
}
func (m *memTeamRepoCoord) GetTeamRunByID(_ context.Context, id string) (biz.TeamRun, error) {
	if r, ok := m.runs[id]; ok {
		return r, nil
	}
	return biz.TeamRun{}, biz.ErrNotFound
}
func (m *memTeamRepoCoord) ListTeamRunSteps(context.Context, string) ([]biz.TeamRunStep, error) {
	return nil, nil
}
func (m *memTeamRepoCoord) CreateTeamRun(_ context.Context, r biz.TeamRun) (biz.TeamRun, error) {
	if m.runs == nil {
		m.runs = map[string]biz.TeamRun{}
	}
	m.runs[r.ID] = r
	return r, nil
}
func (m *memTeamRepoCoord) UpdateTeamRun(_ context.Context, r biz.TeamRun) error {
	if m.runs == nil {
		m.runs = map[string]biz.TeamRun{}
	}
	m.runs[r.ID] = r
	return nil
}
func (m *memTeamRepoCoord) UpdateTeamRunGraphExecutionID(context.Context, string, string) error {
	return nil
}
func (m *memTeamRepoCoord) UpdateTeamRunTraceID(context.Context, string, string) error { return nil }
func (m *memTeamRepoCoord) UpdateTeamRunSummaryJSON(context.Context, string, string) error { return nil }
func (m *memTeamRepoCoord) CreateTeamRunStep(context.Context, biz.TeamRunStep) (biz.TeamRunStep, error) {
	return biz.TeamRunStep{}, nil
}
func (m *memTeamRepoCoord) BatchCreateOrchestrationSteps(context.Context, []biz.OrchestrationStep) error {
	return nil
}
func (m *memTeamRepoCoord) ListOrchestrationSteps(context.Context, string, string, int) ([]biz.OrchestrationStep, error) {
	return nil, nil
}
func (m *memTeamRepoCoord) CreateTaskDeadLetter(context.Context, biz.TaskDeadLetter) error { return nil }
func (m *memTeamRepoCoord) ListTaskDeadLetters(context.Context, biz.TaskDeadLetterListFilter) ([]biz.TaskDeadLetter, error) {
	return nil, nil
}
func (m *memTeamRepoCoord) ResolveTaskDeadLetter(context.Context, string) (biz.TaskDeadLetter, error) {
	return biz.TaskDeadLetter{}, nil
}
func (m *memTeamRepoCoord) ListBySpiritSessionID(_ context.Context, _ string) ([]biz.Team, error) {
	return nil, nil
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

func TestTaskNodesFromBuildConfig_skipsAgentNodes(t *testing.T) {
	cfg := biz.GraphBuildConfig{
		Nodes: []biz.NodeDef{
			{ID: "a1", Type: "agent"},
			{ID: "r1", Type: "review"},
		},
	}
	nodes := TaskNodesFromBuildConfig(cfg)
	if len(nodes) != 1 || nodes["r1"].Type != "review" {
		t.Fatalf("nodes=%v", nodes)
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

func (b *coordGraphBackend) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, teamID, teamRunID string, ct *biz.CompiledTeam) error {
	return b.uc.RegisterTeamGraphExecution(ctx, execID, sessionID, teamID, teamRunID, ct)
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

func newCoordTestBackend() *coordGraphBackend {
	repo := &memGraphRunRepoCoord{runs: map[string]*biz.GraphExecution{}}
	return &coordGraphBackend{repo: repo, uc: biz.NewGraphUsecase(nil, repo, nil, nil, loggateway.NewNoop())}
}

func TestTeamGraphRunCoordinator_DeferTeamRunSuccessIfHITL(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRepoCoord{runs: map[string]biz.TeamRun{"run-1": {ID: "run-1", Status: biz.TeamRunStatusRunning}}}
	bus := event.NewBus(loggateway.NewNoop())
	coord := NewTeamGraphRunCoordinator(backend, repo, bus, nil, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil)
	if err := coord.RegisterTeamGraphExecution(context.Background(), "exec-1", "sess-1", "team-1", "run-1", ct); err != nil {
		t.Fatal(err)
	}
	if err := coord.MarkTeamGraphInterrupt(context.Background(), "exec-1", "review-1", "lineage-1"); err != nil {
		t.Fatal(err)
	}
	run := biz.TeamRun{ID: "run-1", Status: biz.TeamRunStatusRunning}
	deferred, err := coord.DeferTeamRunSuccessIfHITL(context.Background(), "exec-1", &run)
	if err != nil || !deferred {
		t.Fatalf("deferred=%v err=%v run=%+v", deferred, err, run)
	}
	if run.Status != biz.TeamRunStatusWaitingHuman {
		t.Fatalf("status=%q", run.Status)
	}
}

func TestTeamGraphRunCoordinator_finalizeTeamRun(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRepoCoord{runs: map[string]biz.TeamRun{"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusWaitingHuman}}}
	bus := event.NewBus(loggateway.NewNoop())
	coord := NewTeamGraphRunCoordinator(backend, repo, bus, nil, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "team-1", "run-1", ct); err != nil {
		t.Fatal(err)
	}

	outCh, unsub := bus.Subscribe(event.SubscribeOptions{
		SessionID:  "sess-1",
		EventTypes: []event.EnvelopeType{event.EnvelopeTypeTeamRunFinished},
		BufferSize: 4,
	})
	defer unsub()

	coord.finalizeTeamRun(ctx, coord.session("exec-1"), false, "")

	select {
	case <-outCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for team_run_finished")
	}
	got, err := repo.GetTeamRunByID(ctx, "run-1")
	if err != nil || got.Status != biz.TeamRunStatusSuccess {
		t.Fatalf("run=%+v err=%v", got, err)
	}
}

func TestTeamGraphRunCoordinator_persistSession(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRepoCoord{runs: map[string]biz.TeamRun{"run-1": {ID: "run-1", Status: biz.TeamRunStatusRunning}}}
	bus := event.NewBus(loggateway.NewNoop())
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(backend, repo, bus, sessRepo, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "team-1", "run-1", ct); err != nil {
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
	repo := &memTeamRepoCoord{runs: map[string]biz.TeamRun{"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusWaitingHuman}}}
	bus := event.NewBus(loggateway.NewNoop())
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(backend, repo, bus, sessRepo, nil, loggateway.NewNoop())
	ctx := context.Background()
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil)

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "team-1", "run-1", ct); err != nil {
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
	repo := &memTeamRepoCoord{runs: map[string]biz.TeamRun{
		"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusWaitingHuman, DefinitionSnapshotJSON: `{"members":[{"agent_id":"a1","name":"Agent1"}],"mode":"pipeline"}`},
	}}
	bus := event.NewBus(loggateway.NewNoop())
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

	coord := NewTeamGraphRunCoordinator(backend, repo, bus, sessRepo, nil, loggateway.NewNoop())
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
	repo := &memTeamRepoCoord{runs: map[string]biz.TeamRun{"run-1": {ID: "run-1", Status: biz.TeamRunStatusRunning}}}
	bus := event.NewBus(loggateway.NewNoop())
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(backend, repo, bus, sessRepo, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "team-1", "run-1", ct); err != nil {
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

func TestTeamGraphRunCoordinator_CleanupStaleDeletesFromDB(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRepoCoord{runs: map[string]biz.TeamRun{"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusRunning}}}
	bus := event.NewBus(loggateway.NewNoop())
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(backend, repo, bus, sessRepo, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "team-1", "run-1", ct); err != nil {
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

func (b *failingResumeBackend) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, teamID, teamRunID string, ct *biz.CompiledTeam) error {
	return b.inner.RegisterTeamGraphExecution(ctx, execID, sessionID, teamID, teamRunID, ct)
}
func (b *failingResumeBackend) MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error {
	return b.inner.MarkTeamGraphInterrupt(ctx, execID, nodeID, lineageID)
}
func (b *failingResumeBackend) ResumeExecution(context.Context, string, map[string]any) (*biz.GraphExecution, error) {
	return nil, errors.New("resume failed")
}
func (b *failingResumeBackend) GetExecution(ctx context.Context, executionID string) (*biz.GraphExecution, error) {
	return b.inner.GetExecution(ctx, executionID)
}

type succeedingResumeBackend struct {
	inner TeamGraphExecutionBackend
}

func (b *succeedingResumeBackend) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, teamID, teamRunID string, ct *biz.CompiledTeam) error {
	return b.inner.RegisterTeamGraphExecution(ctx, execID, sessionID, teamID, teamRunID, ct)
}
func (b *succeedingResumeBackend) MarkTeamGraphInterrupt(ctx context.Context, execID, nodeID, lineageID string) error {
	return b.inner.MarkTeamGraphInterrupt(ctx, execID, nodeID, lineageID)
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
}

func TestTeamGraphRunCoordinator_HandleTaskCompleted_ResumeFail(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRepoCoord{runs: map[string]biz.TeamRun{"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusWaitingHuman}}}
	bus := event.NewBus(loggateway.NewNoop())
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(&failingResumeBackend{inner: backend}, repo, bus, sessRepo, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "team-1", "run-1", ct); err != nil {
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

func TestTeamGraphRunCoordinator_HandleTaskCompleted_ResumeSuccess(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRepoCoord{runs: map[string]biz.TeamRun{"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusWaitingHuman}}}
	bus := event.NewBus(loggateway.NewNoop())
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(&succeedingResumeBackend{inner: backend}, repo, bus, sessRepo, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "team-1", "run-1", ct); err != nil {
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
	repo := &memTeamRepoCoord{runs: map[string]biz.TeamRun{"run-1": {ID: "run-1", Status: biz.TeamRunStatusRunning}}}
	bus := event.NewBus(loggateway.NewNoop())
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(backend, repo, bus, sessRepo, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "team-1", "run-1", ct); err != nil {
		t.Fatal(err)
	}
	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "team-1", "run-1", ct); err != nil {
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
	repo := &memTeamRepoCoord{runs: map[string]biz.TeamRun{"run-1": {ID: "run-1", Status: biz.TeamRunStatusRunning}}}
	bus := event.NewBus(loggateway.NewNoop())
	coord := NewTeamGraphRunCoordinator(backend, repo, bus, nil, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "team-1", "run-1", ct); err != nil {
		t.Fatal(err)
	}
	run := biz.TeamRun{ID: "run-1", Status: biz.TeamRunStatusRunning}
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
	repo := &memTeamRepoCoord{runs: map[string]biz.TeamRun{"run-1": {ID: "run-1", TeamID: "team-1", SessionID: "sess-1", Status: biz.TeamRunStatusRunning}}}
	bus := event.NewBus(loggateway.NewNoop())
	sessRepo := newMemSessionRepo()
	coord := NewTeamGraphRunCoordinator(backend, repo, bus, sessRepo, nil, loggateway.NewNoop())
	ct := biz.NewCompiledTeam(biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}, nil, nil)
	ctx := context.Background()

	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "team-1", "run-1", ct); err != nil {
		t.Fatal(err)
	}
	coord.CleanupStaleSessions()
	if _, err := sessRepo.GetSession(ctx, "exec-1"); err != nil {
		t.Fatal("fresh session should not be cleaned up")
	}
}
