package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// fakeOrchestrator implements TeamOrchestrator for testing.
// It records Orchestrate calls and provides completeStep to signal completion.
type fakeOrchestrator struct {
	mu      sync.Mutex
	pending map[string]chan biz.TeamCompleteEvent
	calls   []string // stepIDs in dispatch order
	seq     *fakeSeq // 用于发布 TeamStageCreatedEvent 模拟真实流程
	// failErr 按 stepID 注入 Orchestrate 失败（P2-② 假启动对账测试）。
	failErr map[string]error
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
	if err, ok := f.failErr[step.ID]; ok {
		f.calls = append(f.calls, step.ID)
		return nil, err
	}
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
	return f.waitForCallCount(stepID, 1, timeout)
}

// waitForCallCount polls until stepID has been dispatched at least n times
// (F2: 自动重试会产生同一 step 的多次 dispatch)。
func (f *fakeOrchestrator) waitForCallCount(stepID string, n int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		count := 0
		for _, id := range f.calls {
			if id == stepID {
				count++
			}
		}
		f.mu.Unlock()
		if count >= n {
			return true
		}
		time.Sleep(time.Millisecond)
	}
	return false
}

// fakeReposForExecutor implements executorRepos for testing.
type fakeReposForExecutor struct {
	mu         sync.Mutex
	steps      map[string]biz.PlanStep
	stages     map[string]biz.TeamStage
	board      *biz.PlanBoard
	boards     map[string]biz.PlanBoard
	graphStage *biz.GraphStage
	graphNodes map[string]biz.GraphNode
}

func newFakeReposForExecutor() *fakeReposForExecutor {
	return &fakeReposForExecutor{
		steps:      make(map[string]biz.PlanStep),
		stages:     make(map[string]biz.TeamStage),
		boards:     make(map[string]biz.PlanBoard),
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
	if f.boards == nil {
		f.boards = make(map[string]biz.PlanBoard)
	}
	f.boards[pb.ID] = pb
	return pb, nil
}

func (f *fakeReposForExecutor) GetPlanBoard(_ context.Context, id string) (biz.PlanBoard, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.boards != nil {
		if b, ok := f.boards[id]; ok {
			return b, nil
		}
	}
	if f.board != nil && f.board.ID == id {
		return *f.board, nil
	}
	return biz.PlanBoard{}, errors.New("not found")
}

func (f *fakeReposForExecutor) ListPlanBoardsByStatuses(_ context.Context, statuses []biz.PlanStatus) ([]biz.PlanBoard, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	want := make(map[biz.PlanStatus]bool, len(statuses))
	for _, s := range statuses {
		want[s] = true
	}
	out := make([]biz.PlanBoard, 0)
	seen := make(map[string]bool)
	for id, b := range f.boards {
		if want[b.Status] {
			out = append(out, b)
			seen[id] = true
		}
	}
	if f.board != nil && want[f.board.Status] && !seen[f.board.ID] {
		out = append(out, *f.board)
	}
	return out, nil
}

func (f *fakeReposForExecutor) GetPlanStep(_ context.Context, id string) (biz.PlanStep, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.steps[id]; ok {
		return s, nil
	}
	return biz.PlanStep{}, errors.New("not found")
}

func (f *fakeReposForExecutor) ListPlanStepsByPlan(_ context.Context, planID string) ([]biz.PlanStep, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]biz.PlanStep, 0)
	for _, s := range f.steps {
		if s.PlanID == planID {
			out = append(out, s)
		}
	}
	return out, nil
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
// Mirrors production: empty TeamStageID on update does not wipe an existing value.
func (f *fakeReposForExecutor) UpsertGraphNode(_ context.Context, gn biz.GraphNode) (biz.GraphNode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if existing, ok := f.graphNodes[gn.ID]; ok && gn.TeamStageID == "" {
		gn.TeamStageID = existing.TeamStageID
	}
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

// GetTeamStage returns the stored TeamStage by ID (2026-07-05 P1 #9d 补齐).
func (f *fakeReposForExecutor) GetTeamStage(_ context.Context, id string) (biz.TeamStage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.stages[id]; ok {
		return s, nil
	}
	return biz.TeamStage{}, errors.New("not found")
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

// TestPlanExecutor_FailedStepBlocksDownstream verifies that when s1 fails
// (retries exhausted), its dependent s2 is marked skipped (not dispatched to
// the orchestrator). F2（2026-09-03）：首次失败触发 1 次自动重试，重试仍败
// 才 cascade skip 下游。
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
	pe.stepRetryBackoff = 0 // 测试关闭退避

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	if !orch.waitForCall("f1", 2*time.Second) {
		t.Fatal("f1 was not dispatched")
	}
	// f2 must NOT be dispatched before f1 completes.
	if orch.waitForCall("f2", 100*time.Millisecond) {
		t.Fatal("f2 dispatched before f1 completed")
	}
	// 首次失败：触发自动重试（f1 会被再次 dispatch），下游仍不 cascade。
	orch.completeStep("f1", false, "team execution failed")
	if !orch.waitForCallCount("f1", 2, 2*time.Second) {
		t.Fatal("f1 was not auto-retried after first failure")
	}
	if orch.waitForCall("f2", 100*time.Millisecond) {
		t.Fatal("f2 dispatched while f1 retry in flight")
	}
	// 重试仍败：预算耗尽，cascade skip 生效。
	orch.completeStep("f1", false, "team execution failed again")

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

	// Verify events: f1 started ×2（含重试）+ failed ×2; f2 skipped ×1。
	kinds := countingEventKinds(seq.snapshot())
	if kinds[biz.EventKindPlanStepStarted] != 2 {
		t.Errorf("PlanStepStarted events = %d, want 2 (f1 首发+重试)", kinds[biz.EventKindPlanStepStarted])
	}
	if kinds[biz.EventKindPlanStepFailed] != 2 {
		t.Errorf("PlanStepFailed events = %d, want 2", kinds[biz.EventKindPlanStepFailed])
	}
	if kinds[biz.EventKindPlanStepSkipped] != 1 {
		t.Errorf("PlanStepSkipped events = %d, want 1", kinds[biz.EventKindPlanStepSkipped])
	}
}

// TestPlanExecutor_StepAutoRetrySucceeds F2：step 首次失败后 executor 自动
// 重试 1 次；重试成功则下游照常 dispatch，board 收敛 Completed——瞬时故障
// （如 LLM 首字节超时）不再杀死整个任务计划。
func TestPlanExecutor_StepAutoRetrySucceeds(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-retry",
		TaskID:    "task-retry",
		SessionID: "sess-retry",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "a1", PlanID: "board-retry", TaskID: "task-retry", Label: "flaky", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "a2", PlanID: "board-retry", TaskID: "task-retry", Label: "downstream", DependsOn: []string{"a1"}, Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	pe.stepRetryBackoff = 0

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	if !orch.waitForCall("a1", 2*time.Second) {
		t.Fatal("a1 was not dispatched")
	}
	orch.completeStep("a1", false, "transient: first byte timeout")
	// 自动重试：a1 第二次 dispatch。
	if !orch.waitForCallCount("a1", 2, 2*time.Second) {
		t.Fatal("a1 was not auto-retried")
	}
	orch.completeStep("a1", true, "")
	// 重试成功 → 下游 a2 照常派发。
	if !orch.waitForCall("a2", 2*time.Second) {
		t.Fatal("a2 was not dispatched after a1 retry succeeded")
	}
	orch.completeStep("a2", true, "")

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
	if got := repos.steps["a1"].Status; got != biz.PlanStepStatusCompleted {
		t.Errorf("a1 status = %s, want completed", got)
	}
	if got := repos.steps["a2"].Status; got != biz.PlanStepStatusCompleted {
		t.Errorf("a2 status = %s, want completed", got)
	}
	if repos.board == nil || repos.board.Status != biz.PlanStatusCompleted {
		t.Errorf("board status = %v, want completed", repos.board)
	}
}

// waitBoardStatus 轮询 fake repos 直到 board 进入目标状态或超时（resume 后
// DAG 在异步 goroutine 中执行）。
func waitBoardStatus(repos *fakeReposForExecutor, boardID string, want biz.PlanStatus, timeout time.Duration) biz.PlanStatus {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		repos.mu.Lock()
		b, ok := repos.boards[boardID]
		repos.mu.Unlock()
		if ok && b.Status == want {
			return b.Status
		}
		time.Sleep(time.Millisecond)
	}
	repos.mu.Lock()
	defer repos.mu.Unlock()
	return repos.boards[boardID].Status
}

// runBoardToFailed 构造两 step（b1→b2）的 board 并跑到 Failed 终态
//（b1 两次失败耗尽自动重试）。返回执行器三件套。
func runBoardToFailed(t *testing.T, boardID string) (*PlanExecutor, *fakeOrchestrator, *fakeReposForExecutor, *fakeSeq) {
	t.Helper()
	board := biz.PlanBoard{
		ID:        boardID,
		TaskID:    "task-" + boardID,
		SessionID: "sess-" + boardID,
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "b1", PlanID: boardID, TaskID: "task-" + boardID, Label: "failstep", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "b2", PlanID: boardID, TaskID: "task-" + boardID, Label: "depstep", DependsOn: []string{"b1"}, Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	pe.stepRetryBackoff = 0

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()
	if !orch.waitForCall("b1", 2*time.Second) {
		t.Fatal("b1 was not dispatched")
	}
	orch.completeStep("b1", false, "boom-1")
	if !orch.waitForCallCount("b1", 2, 2*time.Second) {
		t.Fatal("b1 was not auto-retried")
	}
	orch.completeStep("b1", false, "boom-2")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Subscribe returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}
	if got := waitBoardStatus(repos, boardID, biz.PlanStatusFailed, time.Second); got != biz.PlanStatusFailed {
		t.Fatalf("board status = %s, want failed", got)
	}
	return pe, orch, repos, seq
}

// TestPlanExecutor_ResumeFailedBoard_Retry F2：失败 board 可恢复——retry 模式
// 重跑失败 step，cascade skip 的下游复活，DAG 续跑至 Completed。
func TestPlanExecutor_ResumeFailedBoard_Retry(t *testing.T) {
	t.Parallel()
	pe, orch, repos, _ := runBoardToFailed(t, "board-resume-retry")

	if err := pe.ResumePlanBoard(context.Background(), "board-resume-retry", ResumeModeRetry); err != nil {
		t.Fatalf("ResumePlanBoard: %v", err)
	}
	// resume 重排队后 b1 第三次 dispatch（新 dagRun，重试预算重置）。
	if !orch.waitForCallCount("b1", 3, 2*time.Second) {
		t.Fatal("b1 was not redispatched after resume(retry)")
	}
	orch.completeStep("b1", true, "")
	// b1 成功 → 下游 b2 照常派发。
	if !orch.waitForCall("b2", 2*time.Second) {
		t.Fatal("b2 was not dispatched after resumed b1 completed")
	}
	orch.completeStep("b2", true, "")

	if got := waitBoardStatus(repos, "board-resume-retry", biz.PlanStatusCompleted, 5*time.Second); got != biz.PlanStatusCompleted {
		t.Fatalf("board status = %s, want completed", got)
	}
	repos.mu.Lock()
	defer repos.mu.Unlock()
	if got := repos.steps["b1"].Status; got != biz.PlanStepStatusCompleted {
		t.Errorf("b1 status = %s, want completed", got)
	}
	if got := repos.steps["b2"].Status; got != biz.PlanStepStatusCompleted {
		t.Errorf("b2 status = %s, want completed", got)
	}
}

// TestPlanExecutor_ResumeFailedBoard_Skip F2：skip 模式把失败 step 降级为
// skipped，下游依赖按「skipped 视为已满足」放行续跑，board 收敛 Completed。
func TestPlanExecutor_ResumeFailedBoard_Skip(t *testing.T) {
	t.Parallel()
	pe, orch, repos, _ := runBoardToFailed(t, "board-resume-skip")

	if err := pe.ResumePlanBoard(context.Background(), "board-resume-skip", ResumeModeSkip); err != nil {
		t.Fatalf("ResumePlanBoard: %v", err)
	}
	// b1 不再 dispatch；b2 降级放行直接 dispatch。
	repos.mu.Lock()
	b1Status := repos.steps["b1"].Status
	repos.mu.Unlock()
	if b1Status != biz.PlanStepStatusSkipped {
		t.Fatalf("b1 status = %s, want skipped", b1Status)
	}
	if !orch.waitForCall("b2", 2*time.Second) {
		t.Fatal("b2 was not dispatched under degraded skip semantics")
	}
	orch.completeStep("b2", true, "")

	if got := waitBoardStatus(repos, "board-resume-skip", biz.PlanStatusCompleted, 5*time.Second); got != biz.PlanStatusCompleted {
		t.Fatalf("board status = %s, want completed", got)
	}
	// b1 保持 skipped（两轮失败后第三次 dispatch 不得发生）。
	if orch.waitForCallCount("b1", 3, 200*time.Millisecond) {
		t.Fatal("b1 must not be redispatched in skip mode")
	}
}

// TestPlanExecutor_ResumePlanBoard_Guards F2：非 Failed board / 非法 mode /
// 空 board 拒绝恢复。
func TestPlanExecutor_ResumePlanBoard_Guards(t *testing.T) {
	t.Parallel()
	repos := newFakeReposForExecutor()
	pe := NewPlanExecutor(repos, newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())

	if err := pe.ResumePlanBoard(context.Background(), "", ResumeModeRetry); err == nil {
		t.Error("empty board id should be rejected")
	}
	if err := pe.ResumePlanBoard(context.Background(), "x", "bogus"); err == nil {
		t.Error("invalid mode should be rejected")
	}
	repos.boards["board-ok"] = biz.PlanBoard{ID: "board-ok", Status: biz.PlanStatusCompleted}
	if err := pe.ResumePlanBoard(context.Background(), "board-ok", ResumeModeRetry); err == nil {
		t.Error("completed board should be rejected")
	}
	repos.boards["board-nosteps"] = biz.PlanBoard{ID: "board-nosteps", Status: biz.PlanStatusFailed}
	if err := pe.ResumePlanBoard(context.Background(), "board-nosteps", ResumeModeRetry); err == nil {
		t.Error("failed board without failed steps should be rejected")
	}
}

// TestPlanExecutor_CancelSweepsNonTerminalSteps verifies P1-6: when the run
// ctx is canceled mid-flight, steps that never reached a terminal state
// (in-flight running + never-dispatched pending) must be swept to Skipped
// with an audit event — otherwise they stay running/pending forever in the
// DB and the UI shows stale non-terminal state after refresh.
func TestPlanExecutor_CancelSweepsNonTerminalSteps(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-cancel",
		TaskID:    "task-cancel",
		SessionID: "sess-cancel",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "c1", PlanID: "board-cancel", TaskID: "task-cancel", Label: "inflight", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "c2", PlanID: "board-cancel", TaskID: "task-cancel", Label: "neverdispatched", DependsOn: []string{"c1"}, Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(ctx, board) }()

	if !orch.waitForCall("c1", 2*time.Second) {
		t.Fatal("c1 was not dispatched")
	}
	// c1 stays in-flight (never completed); cancel the run.
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Subscribe should return ctx.Err() on cancel")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}

	repos.mu.Lock()
	defer repos.mu.Unlock()
	for _, id := range []string{"c1", "c2"} {
		step, ok := repos.steps[id]
		if !ok {
			t.Errorf("step %s not persisted", id)
			continue
		}
		if step.Status != biz.PlanStepStatusSkipped {
			t.Errorf("step %s status = %s, want %s (cancel sweep)", id, step.Status, biz.PlanStepStatusSkipped)
		}
	}
	kinds := countingEventKinds(seq.snapshot())
	if kinds[biz.EventKindPlanStepSkipped] != 2 {
		t.Errorf("PlanStepSkipped events = %d, want 2 (c1 in-flight + c2 never-dispatched)", kinds[biz.EventKindPlanStepSkipped])
	}
}

// TestPlanExecutor_GraphNodeKeepsTeamStageIDAfterComplete verifies GS-1:
// dispatch writes TeamStageID; terminal update with empty teamStageID must not wipe it.
func TestPlanExecutor_GraphNodeKeepsTeamStageIDAfterComplete(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-tsid",
		TaskID:    "task-tsid",
		SessionID: "sess-tsid",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "ts1", PlanID: "board-tsid", TaskID: "task-tsid", Label: "only", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	if !orch.waitForCall("ts1", 2*time.Second) {
		t.Fatal("ts1 was not dispatched")
	}
	wantTeamStageID := "fake-team-stage-ts1"
	// After dispatch, GraphNode should have TeamStageID.
	deadline := time.Now().Add(2 * time.Second)
	for {
		repos.mu.Lock()
		gn, ok := repos.graphNodes["ts1"]
		repos.mu.Unlock()
		if ok && gn.TeamStageID == wantTeamStageID {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("GraphNode TeamStageID not set after dispatch: ok=%v id=%q", ok, gn.TeamStageID)
		}
		time.Sleep(5 * time.Millisecond)
	}

	orch.completeStep("ts1", true, "")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}

	repos.mu.Lock()
	gn := repos.graphNodes["ts1"]
	repos.mu.Unlock()
	if gn.TeamStageID != wantTeamStageID {
		t.Errorf("TeamStageID after complete = %q, want %q (must not be wiped)", gn.TeamStageID, wantTeamStageID)
	}
	if gn.Status != biz.GraphNodeStatusCompleted {
		t.Errorf("status = %s, want completed", gn.Status)
	}
}

// TestPlanExecutor_InitGraphStageDoesNotPublishCreated verifies GS-2 / B.10.5:
// initGraphStage is Upsert-only; GraphStageCreatedEvent comes only from PublishV2Board.
func TestPlanExecutor_InitGraphStageDoesNotPublishCreated(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-nocreated",
		TaskID:    "task-nocreated",
		SessionID: "sess-nocreated",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "n1", PlanID: "board-nocreated", TaskID: "task-nocreated", Label: "n1", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "n2", PlanID: "board-nocreated", TaskID: "task-nocreated", Label: "n2", DependsOn: []string{"n1"}, Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	if !orch.waitForCall("n1", 2*time.Second) {
		t.Fatal("n1 not dispatched")
	}
	orch.completeStep("n1", true, "")
	if !orch.waitForCall("n2", 2*time.Second) {
		t.Fatal("n2 not dispatched")
	}
	orch.completeStep("n2", true, "")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}

	kinds := countingEventKinds(seq.snapshot())
	if n := kinds[biz.EventKindGraphStageCreated]; n != 0 {
		t.Errorf("GraphStageCreated events = %d, want 0 (PublishV2Board owns created)", n)
	}
	if repos.graphStage == nil || repos.graphStage.ID == "" {
		t.Fatal("expected initGraphStage to Upsert GraphStage for crash recovery")
	}
}

// ---------------------------------------------------------------------------
// P1 形式契约（B.10.15.2）：dagRun 启动时契约 advisory 验证
// ---------------------------------------------------------------------------

// collectSystemNotices subscribes to the bus and buffers SystemNoticeEvents
// until the returned stop func is called.
//
// V2Bus 契约（R-1 修复后）：cancel 只摘除订阅者，永不 close channel——
// 接收循环必须通过自己的 done channel 退出（红线 #23），禁止 range ch。
func collectSystemNotices(bus biz.EventBus) (notices func() []*biz.SystemNoticeEvent, stop func()) {
	ch, cancel := bus.Subscribe(biz.EventSubscribeOptions{})
	var mu sync.Mutex
	var buf []*biz.SystemNoticeEvent
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for {
			select {
			case <-done:
				return
			case ev := <-ch:
				if n, ok := ev.(*biz.SystemNoticeEvent); ok {
					mu.Lock()
					buf = append(buf, n)
					mu.Unlock()
				}
			}
		}
	}()
	return func() []*biz.SystemNoticeEvent {
			mu.Lock()
			defer mu.Unlock()
			out := make([]*biz.SystemNoticeEvent, len(buf))
			copy(out, buf)
			return out
		}, func() {
			cancel()
			close(done)
			<-finished
		}
}

func TestDagRun_ContractMismatch_PublishesSystemNotice(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-contract-mismatch",
		TaskID:    "task-contract",
		SessionID: "sess-contract",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "board-contract-mismatch", TaskID: "task-contract", Label: "upstream", Status: biz.PlanStepStatusPending, Version: 1,
				Deliverables: []biz.DeliverableContract{{Name: "report", Type: "document", Format: "markdown"}}},
			{ID: "s2", PlanID: "board-contract-mismatch", TaskID: "task-contract", Label: "downstream", DependsOn: []string{"s1"}, Status: biz.PlanStepStatusPending, Version: 1,
				InputContract: []biz.DeliverableContract{{Name: "data", Type: "data", Format: "json"}}},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	bus := event.NewV2Bus()
	pe.SetEventBus(bus)
	notices, stop := collectSystemNotices(bus)
	defer stop()

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	if !orch.waitForCall("s1", 2*time.Second) {
		t.Fatal("s1 was not dispatched")
	}
	orch.completeStep("s1", true, "")
	if !orch.waitForCall("s2", 2*time.Second) {
		t.Fatal("s2 was not dispatched")
	}
	orch.completeStep("s2", true, "")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}

	var mismatch *biz.SystemNoticeEvent
	for _, n := range notices() {
		if n.NoticeType == "contract_mismatch" {
			mismatch = n
			break
		}
	}
	if mismatch == nil {
		t.Fatal("expected a contract_mismatch SystemNoticeEvent, got none")
	}
	if mismatch.SpiritSessionID() != "sess-contract" {
		t.Errorf("notice session = %q, want sess-contract", mismatch.SpiritSessionID())
	}
	warnings, ok := mismatch.Meta["warnings"].([]string)
	if !ok || len(warnings) == 0 {
		t.Fatalf("notice meta warnings missing or empty: %v", mismatch.Meta)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, `"data"`) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("warnings should mention unmatched contract %q: %v", "data", warnings)
	}
}

// stubTaskPlanUpdater implements taskPlanStatusUpdater in memory (TS9-BUG-1).
type stubTaskPlanUpdater struct {
	mu   sync.Mutex
	plan *biz.TaskPlan
}

func (s *stubTaskPlanUpdater) GetByID(_ context.Context, id string) (*biz.TaskPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.plan != nil && s.plan.ID == id {
		cp := *s.plan
		return &cp, nil
	}
	return nil, errors.New("not found")
}

func (s *stubTaskPlanUpdater) Update(_ context.Context, plan *biz.TaskPlan) (*biz.TaskPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *plan
	s.plan = &cp
	return plan, nil
}

func (s *stubTaskPlanUpdater) status() biz.TaskPlanStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.plan.Status
}

func waitPlanStatus(t *testing.T, s *stubTaskPlanUpdater, want biz.TaskPlanStatus) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.status() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("plan status = %s, want %s (timed out waiting)", s.status(), want)
}

// TestPlanExecutor_TaskPlanStatusPropagation (TS9-BUG-1) verifies the TaskPlan
// lifecycle follows the PlanBoard: confirmed → executing → completed. Before
// the fix, plans stayed "confirmed" forever.
func TestPlanExecutor_TaskPlanStatusPropagation(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "pb_plan-1", // PublishV2Board 派生规则："pb_"+plan.ID
		TaskID:    "task-1",
		SessionID: "sess-1",
		Status:    biz.PlanStatusPlanning,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "pb_plan-1", TaskID: "task-1", Label: "step1", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	updater := &stubTaskPlanUpdater{plan: &biz.TaskPlan{ID: "plan-1", Status: biz.TaskPlanStatusConfirmed}}
	pe.SetTaskPlanUpdater(updater)

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	if !orch.waitForCall("s1", 2*time.Second) {
		t.Fatal("s1 was not dispatched in time")
	}
	// Board planning→executing 后 plan 必须进入 executing。
	waitPlanStatus(t, updater, biz.TaskPlanStatusExecuting)

	orch.completeStep("s1", true, "")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Subscribe returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}
	if got := updater.status(); got != biz.TaskPlanStatusCompleted {
		t.Fatalf("plan status after DAG completed = %s, want completed", got)
	}
}

// TestPlanExecutor_TaskPlanStatusPropagation_Failed (TS9-BUG-1) verifies a
// failed DAG run propagates failed to the TaskPlan.
// F2（2026-09-03）：首次失败触发自动重试，重试耗尽（第二次失败）后才收敛
// board/plan 终态。
func TestPlanExecutor_TaskPlanStatusPropagation_Failed(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "pb_plan-2",
		TaskID:    "task-2",
		SessionID: "sess-2",
		Status:    biz.PlanStatusPlanning,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "pb_plan-2", TaskID: "task-2", Label: "step1", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	pe.stepRetryBackoff = 0
	updater := &stubTaskPlanUpdater{plan: &biz.TaskPlan{ID: "plan-2", Status: biz.TaskPlanStatusConfirmed}}
	pe.SetTaskPlanUpdater(updater)

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	if !orch.waitForCall("s1", 2*time.Second) {
		t.Fatal("s1 was not dispatched in time")
	}
	waitPlanStatus(t, updater, biz.TaskPlanStatusExecuting)

	orch.completeStep("s1", false, "boom")
	// F2：自动重试→第二次 dispatch→仍败，重试预算耗尽。
	if !orch.waitForCallCount("s1", 2, 2*time.Second) {
		t.Fatal("s1 was not auto-retried after first failure")
	}
	orch.completeStep("s1", false, "boom again")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Subscribe returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}
	if got := updater.status(); got != biz.TaskPlanStatusFailed {
		t.Fatalf("plan status after DAG failed = %s, want failed", got)
	}
}

// TestPlanExecutor_TaskPlanStatusPropagation_TerminalNotOverwritten
// (TS9-BUG-1) verifies an already-terminal TaskPlan is not overwritten by a
// late board terminal event (e.g. replay).
func TestPlanExecutor_TaskPlanStatusPropagation_TerminalNotOverwritten(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	updater := &stubTaskPlanUpdater{plan: &biz.TaskPlan{ID: "plan-3", Status: biz.TaskPlanStatusCompleted}}
	pe.SetTaskPlanUpdater(updater)

	pe.propagateTaskPlanExecuting(context.Background(), "pb_plan-3")
	if got := updater.status(); got != biz.TaskPlanStatusCompleted {
		t.Fatalf("executing propagation overwrote terminal plan: %s", got)
	}
	pe.propagateTaskPlanTerminal(context.Background(), "pb_plan-3", biz.PlanStatusFailed)
	if got := updater.status(); got != biz.TaskPlanStatusCompleted {
		t.Fatalf("terminal propagation overwrote terminal plan: %s", got)
	}
}

func TestDagRun_ContractMatch_NoMismatchNotice(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-contract-match",
		TaskID:    "task-contract-ok",
		SessionID: "sess-contract-ok",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "board-contract-match", TaskID: "task-contract-ok", Label: "upstream", Status: biz.PlanStepStatusPending, Version: 1,
				Deliverables: []biz.DeliverableContract{{Name: "report", Type: "document", Format: "markdown"}}},
			{ID: "s2", PlanID: "board-contract-match", TaskID: "task-contract-ok", Label: "downstream", DependsOn: []string{"s1"}, Status: biz.PlanStepStatusPending, Version: 1,
				InputContract: []biz.DeliverableContract{{Name: "report", Type: "document", Format: "markdown"}}},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	bus := event.NewV2Bus()
	pe.SetEventBus(bus)
	notices, stop := collectSystemNotices(bus)
	defer stop()

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	if !orch.waitForCall("s1", 2*time.Second) {
		t.Fatal("s1 was not dispatched")
	}
	orch.completeStep("s1", true, "")
	if !orch.waitForCall("s2", 2*time.Second) {
		t.Fatal("s2 was not dispatched")
	}
	orch.completeStep("s2", true, "")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}

	for _, n := range notices() {
		if n.NoticeType == "contract_mismatch" {
			t.Fatalf("matching contracts should not emit contract_mismatch, got %+v", n)
		}
	}
}
