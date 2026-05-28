package team

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
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

func (m *memGraphRunRepoCoord) ListRunsByGraph(context.Context, string, int, string) ([]*biz.GraphExecution, string, error) {
	return nil, "", nil
}

func (m *memGraphRunRepoCoord) UpdateRun(_ context.Context, exec *biz.GraphExecution) error {
	if m.runs == nil {
		m.runs = map[string]*biz.GraphExecution{}
	}
	m.runs[exec.ID] = exec
	return nil
}

func (b *coordGraphBackend) RegisterTeamGraphExecution(ctx context.Context, execID, sessionID, teamID, teamRunID string, cfg biz.GraphBuildConfig) error {
	return b.uc.RegisterTeamGraphExecution(ctx, execID, sessionID, teamID, teamRunID, cfg)
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
	return &coordGraphBackend{repo: repo, uc: biz.NewGraphUsecase(nil, repo, nil, nil)}
}

func TestTeamGraphRunCoordinator_DeferTeamRunSuccessIfHITL(t *testing.T) {
	backend := newCoordTestBackend()
	repo := &memTeamRepoCoord{runs: map[string]biz.TeamRun{"run-1": {ID: "run-1", Status: biz.TeamRunStatusRunning}}}
	bus := event.NewBus()
	coord := NewTeamGraphRunCoordinator(backend, repo, bus)
	cfg := biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}
	if err := coord.RegisterTeamGraphExecution(context.Background(), "exec-1", "sess-1", "team-1", "run-1", cfg); err != nil {
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
	bus := event.NewBus()
	coord := NewTeamGraphRunCoordinator(backend, repo, bus)
	ctx := context.Background()
	cfg := biz.GraphBuildConfig{Nodes: []biz.NodeDef{{ID: "review-1", Type: "review"}}}
	if err := coord.RegisterTeamGraphExecution(ctx, "exec-1", "sess-1", "team-1", "run-1", cfg); err != nil {
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
