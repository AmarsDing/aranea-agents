package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// fakeOrchestrator implements TeamOrchestrator for testing.
// It records Orchestrate calls and provides completeStep to signal completion.
//
type fakeOrchestrator struct {
	mu      sync.Mutex
	pending map[string]chan biz.TeamCompleteEvent
	calls   []string // stepIDs in dispatch order
	seq     *fakeSeq // 用于发布 TeamStageCreatedEvent 模拟真实流程
}

func newFakeOrchestrator() *fakeOrchestrator {
	return &fakeOrchestrator{pending: make(map[string]chan biz.TeamCompleteEvent)}
}

// withSeq injects a fakeSeq so Orchestrate can publish TeamStageCreatedEvent.
func (f *fakeOrchestrator) withSeq(seq *fakeSeq) *fakeOrchestrator {
	f.seq = seq
	return f
}

func (f *fakeOrchestrator) Orchestrate(ctx context.Context, step biz.PlanStep, _ biz.TeamStage) (*OrchestrateResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	ch := make(chan biz.TeamCompleteEvent, 1)
	f.pending[step.ID] = ch
	f.calls = append(f.calls, step.ID)
	teamID := "fake-team-" + step.ID
	teamStageID := "fake-team-stage-" + step.ID
	// 模拟 publishSpiritTeamAssembled：发布 TeamStageCreatedEvent（带 Members）。
	// 真实流程中这是 AssembleTeam → publishSpiritTeamAssembled 的副作用。
	if f.seq != nil {
		createdTS := biz.TeamStage{
			ID:        teamStageID,
			TeamID:    teamID,
			SessionID: step.TaskID, // 测试用 taskID 作为 sessionID 占位
			Status:    biz.TeamStageStatusPending,
			Stage:     biz.TeamStageStageAssembled,
			Members: []biz.MemberInfo{
				{AgentKey: "fake-agent-key", AgentName: "fake-agent-key", ChildSessionID: "fake-member-session-" + step.ID, Status: "pending"},
			},
			StartedAt: time.Now(),
			Version:   1,
		}
		f.seq.Publish(ctx, biz.NewTeamStageCreatedEvent(createdTS))
	}
	return &OrchestrateResult{
		Team: biz.Team{
			ID: teamID,
		},
		TeamSession: biz.Session{
			ID: "fake-team-session-" + step.ID,
		},
		MemberSessions: map[string]string{
			"fake-agent-key": "fake-member-session-" + step.ID,
		},
		TeamStageID:    teamStageID,
		CompletionChan: ch,
	}, nil
}

// completeStep signals completion for a step. Safe for concurrent use.
func (f *fakeOrchestrator) completeStep(stepID string, success bool, errMsg string) {
	f.mu.Lock()
	ch, ok := f.pending[stepID]
	delete(f.pending, stepID)
	f.mu.Unlock()
	if !ok {
		return
	}
	ch <- biz.TeamCompleteEvent{StepID: stepID, Success: success, ErrorMsg: errMsg}
	close(ch)
}

// waitForCall polls until stepID appears in the orchestrate call list or times out.
func (f *fakeOrchestrator) waitForCall(stepID string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		for _, id := range f.calls {
			if id == stepID {
				f.mu.Unlock()
				return true
			}
		}
		f.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	return false
}

// fakeReposForExecutor implements executorRepos for testing.
type fakeReposForExecutor struct {
	mu          sync.Mutex
	steps       map[string]biz.PlanStep
	stages      map[string]biz.TeamStage
	board       *biz.PlanBoard
	graphStage  *biz.GraphStage
	graphNodes  map[string]biz.GraphNode
}

func newFakeReposForExecutor() *fakeReposForExecutor {
	return &fakeReposForExecutor{
		steps:      make(map[string]biz.PlanStep),
		stages:     make(map[string]biz.TeamStage),
		graphNodes: make(map[string]biz.GraphNode),
	}
}

func (f *fakeReposForExecutor) UpsertPlanStep(_ context.Context, ps biz.PlanStep) (biz.PlanStep, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.steps[ps.ID] = ps
	return ps, nil
}

func (f *fakeReposForExecutor) UpsertTeamStage(_ context.Context, ts biz.TeamStage) (biz.TeamStage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stages[ts.ID] = ts
	return ts, nil
}

func (f *fakeReposForExecutor) UpsertPlanBoard(_ context.Context, pb biz.PlanBoard) (biz.PlanBoard, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := pb
	f.board = &cp
	return pb, nil
}

func (f *fakeReposForExecutor) GetPlanStep(_ context.Context, id string) (biz.PlanStep, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.steps[id]; ok {
		return s, nil
	}
	return biz.PlanStep{}, errors.New("not found")
}

// UpsertGraphStage stores the GraphStage in memory.
func (f *fakeReposForExecutor) UpsertGraphStage(_ context.Context, gs biz.GraphStage) (biz.GraphStage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := gs
	f.graphStage = &cp
	return gs, nil
}

// UpsertGraphNode stores the GraphNode in memory.
func (f *fakeReposForExecutor) UpsertGraphNode(_ context.Context, gn biz.GraphNode) (biz.GraphNode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.graphNodes[gn.ID] = gn
	return gn, nil
}

// GetGraphStageByPlanBoard returns the stored GraphStage if its PlanBoardID matches.
func (f *fakeReposForExecutor) GetGraphStageByPlanBoard(_ context.Context, planBoardID string) (biz.GraphStage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.graphStage != nil && f.graphStage.PlanBoardID == planBoardID {
		return *f.graphStage, nil
	}
	return biz.GraphStage{}, errors.New("not found")
}

// fakeSeq implements sequencerPublisher for testing.
type fakeSeq struct {
	mu     sync.Mutex
	events []biz.Event
	repos  *fakeReposForExecutor // 用于模拟 EventRouter 持久化 PlanStep
}

func (f *fakeSeq) Publish(ctx context.Context, e biz.Event) {
	f.mu.Lock()
	f.events = append(f.events, e)
	f.mu.Unlock()
	// 模拟 EventRouter：将 PlanStep 事件路由到 repos.UpsertPlanStep。
	// 真实 EventRouter 在 event_router.go 中做同样的路由。
	if f.repos == nil {
		return
	}
	switch ev := e.(type) {
	case *biz.PlanStepStartedEvent:
		_, _ = f.repos.UpsertPlanStep(ctx, ev.PlanStep)
	case *biz.PlanStepCompletedEvent:
		_, _ = f.repos.UpsertPlanStep(ctx, ev.PlanStep)
	case *biz.PlanStepFailedEvent:
		_, _ = f.repos.UpsertPlanStep(ctx, ev.PlanStep)
	case *biz.PlanStepSkippedEvent:
		_, _ = f.repos.UpsertPlanStep(ctx, ev.PlanStep)
	}
}

func (f *fakeSeq) snapshot() []biz.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]biz.Event, len(f.events))
	copy(out, f.events)
	return out
}

// countingEventKinds counts events by kind in the snapshot.
func countingEventKinds(events []biz.Event) map[biz.EventKind]int {
	counts := make(map[biz.EventKind]int)
	for _, e := range events {
		counts[e.EventKind()]++
	}
	return counts
}

// TestPlanExecutor_SequentialDAG verifies a 3-step linear DAG (s1→s2→s3)
// dispatches steps sequentially: s2 only starts after s1 completes, etc.
func TestPlanExecutor_SequentialDAG(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-seq",
		TaskID:    "task-1",
		SessionID: "sess-1",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "board-seq", TaskID: "task-1", Label: "step1", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "s2", PlanID: "board-seq", TaskID: "task-1", Label: "step2", DependsOn: []string{"s1"}, Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "s3", PlanID: "board-seq", TaskID: "task-1", Label: "step3", DependsOn: []string{"s2"}, Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	// s1 should be dispatched first; s2 and s3 must NOT be dispatched yet.
	if !orch.waitForCall("s1", 2*time.Second) {
		t.Fatal("s1 was not dispatched in time")
	}
	if orch.waitForCall("s2", 100*time.Millisecond) {
		t.Fatal("s2 dispatched before s1 completed — sequential order violated")
	}
	orch.completeStep("s1", true, "")

	if !orch.waitForCall("s2", 2*time.Second) {
		t.Fatal("s2 was not dispatched after s1 completed")
	}
	if orch.waitForCall("s3", 100*time.Millisecond) {
		t.Fatal("s3 dispatched before s2 completed — sequential order violated")
	}
	orch.completeStep("s2", true, "")

	if !orch.waitForCall("s3", 2*time.Second) {
		t.Fatal("s3 was not dispatched after s2 completed")
	}
	orch.completeStep("s3", true, "")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Subscribe returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}

	// Verify all steps reached completed status.
	repos.mu.Lock()
	defer repos.mu.Unlock()
	for _, id := range []string{"s1", "s2", "s3"} {
		step, ok := repos.steps[id]
		if !ok {
			t.Errorf("step %s not persisted", id)
			continue
		}
		if step.Status != biz.PlanStepStatusCompleted {
			t.Errorf("step %s status = %s, want %s", id, step.Status, biz.PlanStepStatusCompleted)
		}
		if step.CompletedAt == nil {
			t.Errorf("step %s CompletedAt is nil", id)
		}
	}

	// Verify event counts: 3 started + 3 completed + 3 team_stage.created = 9.
	kinds := countingEventKinds(seq.snapshot())
	if kinds[biz.EventKindPlanStepStarted] != 3 {
		t.Errorf("PlanStepStarted events = %d, want 3", kinds[biz.EventKindPlanStepStarted])
	}
	if kinds[biz.EventKindPlanStepCompleted] != 3 {
		t.Errorf("PlanStepCompleted events = %d, want 3", kinds[biz.EventKindPlanStepCompleted])
	}
	if kinds[biz.EventKindTeamStageCreated] != 3 {
		t.Errorf("TeamStageCreated events = %d, want 3", kinds[biz.EventKindTeamStageCreated])
	}
}

// TestPlanExecutor_ParallelRoots verifies two independent root steps dispatch
// concurrently (both appear in the orchestrator's call list before either is
// completed).
func TestPlanExecutor_ParallelRoots(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-par",
		TaskID:    "task-2",
		SessionID: "sess-2",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "r1", PlanID: "board-par", TaskID: "task-2", Label: "root1", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "r2", PlanID: "board-par", TaskID: "task-2", Label: "root2", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	// Both roots should be dispatched without needing any completion signal.
	if !orch.waitForCall("r1", 2*time.Second) {
		t.Fatal("r1 was not dispatched")
	}
	if !orch.waitForCall("r2", 2*time.Second) {
		t.Fatal("r2 was not dispatched")
	}

	orch.completeStep("r1", true, "")
	orch.completeStep("r2", true, "")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Subscribe returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}

	repos.mu.Lock()
	defer repos.mu.Unlock()
	for _, id := range []string{"r1", "r2"} {
		step, ok := repos.steps[id]
		if !ok {
			t.Errorf("step %s not persisted", id)
			continue
		}
		if step.Status != biz.PlanStepStatusCompleted {
			t.Errorf("step %s status = %s, want %s", id, step.Status, biz.PlanStepStatusCompleted)
		}
	}
}

// TestPlanExecutor_FailedStepBlocksDownstream verifies that when s1 fails,
// its dependent s2 is marked skipped (not dispatched to the orchestrator).
func TestPlanExecutor_FailedStepBlocksDownstream(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-fail",
		TaskID:    "task-3",
		SessionID: "sess-3",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "f1", PlanID: "board-fail", TaskID: "task-3", Label: "failstep", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "f2", PlanID: "board-fail", TaskID: "task-3", Label: "depstep", DependsOn: []string{"f1"}, Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	if !orch.waitForCall("f1", 2*time.Second) {
		t.Fatal("f1 was not dispatched")
	}
	// f2 must NOT be dispatched before f1 completes.
	if orch.waitForCall("f2", 100*time.Millisecond) {
		t.Fatal("f2 dispatched before f1 completed")
	}
	orch.completeStep("f1", false, "team execution failed")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Subscribe returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}

	// f2 must never have been dispatched to the orchestrator.
	orch.mu.Lock()
	for _, id := range orch.calls {
		if id == "f2" {
			orch.mu.Unlock()
			t.Fatal("f2 was dispatched to orchestrator despite dependency failure")
		}
	}
	orch.mu.Unlock()

	repos.mu.Lock()
	defer repos.mu.Unlock()
	f1 := repos.steps["f1"]
	if f1.Status != biz.PlanStepStatusFailed {
		t.Errorf("f1 status = %s, want %s", f1.Status, biz.PlanStepStatusFailed)
	}
	if f1.Error == nil || f1.Error.Message == "" {
		t.Errorf("f1.Error not set with failure message")
	}
	f2 := repos.steps["f2"]
	if f2.Status != biz.PlanStepStatusSkipped {
		t.Errorf("f2 status = %s, want %s", f2.Status, biz.PlanStepStatusSkipped)
	}

	// Verify events: f1 started + failed; f2 skipped (no started event for f2).
	kinds := countingEventKinds(seq.snapshot())
	if kinds[biz.EventKindPlanStepStarted] != 1 {
		t.Errorf("PlanStepStarted events = %d, want 1 (only f1)", kinds[biz.EventKindPlanStepStarted])
	}
	if kinds[biz.EventKindPlanStepFailed] != 1 {
		t.Errorf("PlanStepFailed events = %d, want 1", kinds[biz.EventKindPlanStepFailed])
	}
	if kinds[biz.EventKindPlanStepSkipped] != 1 {
		t.Errorf("PlanStepSkipped events = %d, want 1", kinds[biz.EventKindPlanStepSkipped])
	}
}
