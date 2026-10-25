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

// fakeTeamRunReader 是 biz.TeamRunV2Reader 的进程内桩（D6 轮询钉板用）。
type fakeTeamRunReader struct {
	mu   sync.Mutex
	runs map[string][]biz.TeamRun // teamStageID → runs
}

func (f *fakeTeamRunReader) set(stageID string, runs ...biz.TeamRun) {
	f.mu.Lock()
	f.runs[stageID] = runs
	f.mu.Unlock()
}

func (f *fakeTeamRunReader) GetTeamRun(_ context.Context, id string) (biz.TeamRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rs := range f.runs {
		for _, r := range rs {
			if r.ID == id {
				return r, nil
			}
		}
	}
	return biz.TeamRun{}, errors.New("not found")
}

func (f *fakeTeamRunReader) ListTeamRunsByStage(_ context.Context, stageID string) ([]biz.TeamRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]biz.TeamRun, len(f.runs[stageID]))
	copy(out, f.runs[stageID])
	return out, nil
}

// D6 核心行为钉：waiter 副本的 300ms 轮询消费「别副本只写进决议表」的
// playbook_confirm 决议——本进程 noted 图与确认卡均无该决议，唯一事实源是
// 共享决议面（mem 档端口即跨进程共享面的等价物）。断言 ≤2s 推进（验收
// ≤1s 留足调度余量）、waiter 登记清空、确认卡按批准定案、决议收敛进本
// 进程缓存。
func TestHoldPlaybookConfirmPollConsumesCrossReplicaDecision(t *testing.T) {
	t.Parallel()
	seq := &fakeSeq{}
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), seq, loggateway.NewNoop())
	port := NewInMemExecutorWaiterDecisions()
	pe.SetWaiterDecisionPort(port)
	r := newDagRun(pe, biz.PlanBoard{SessionID: "sp-1", ID: "pb-1", TaskID: "task-1", TurnID: "turn-1"})
	step := &biz.PlanStep{ID: "st-confirm", Label: "发布", ConfirmBefore: true, Status: biz.PlanStepStatusPending}

	type result struct{ approved, held bool }
	done := make(chan result, 1)
	go func() {
		approved, held := r.holdPlaybookConfirm(context.Background(), step)
		done <- result{approved, held}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !pe.HasPlaybookStageConfirm("sp-1", "st-confirm") {
		if time.Now().After(deadline) {
			t.Fatal("waiter not registered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// 副本 B 决议：只写共享决议面。
	if _, err := port.TryRecordDecision(context.Background(), biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{
			Kind:      biz.WaiterDecisionKindPlaybookConfirm,
			SessionID: "sp-1",
			ScopeID:   "st-confirm",
		},
		Approved:  true,
		Actor:     "replica-b",
		DecidedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("record cross-replica decision: %v", err)
	}
	select {
	case got := <-done:
		if !got.held || !got.approved {
			t.Fatalf("held=%v approved=%v", got.held, got.approved)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("poll did not consume cross-replica decision within budget")
	}
	if pe.HasPlaybookStageConfirm("sp-1", "st-confirm") {
		t.Fatal("settle must clear waiter registrations")
	}
	if v, ok := pe.lookupPlaybookConfirmDecision("sp-1", "st-confirm"); !ok || !v {
		t.Fatalf("decision must converge into local cache: ok=%v v=%v", ok, v)
	}
	closed := false
	for _, ev := range seq.snapshot() {
		u, ok := ev.(*biz.StepUpdatedEvent)
		if ok && u.Step.ID == biz.PlaybookConfirmActivityID("sp-1", "st-confirm") && u.Step.Status == biz.StepStatusCompleted {
			closed = true
		}
	}
	if !closed {
		t.Fatal("poll settle must finalize the confirm card as completed")
	}
}

// D6 验收钉（堵 plan_executor_dag_dispatch.go ctx 超时→abortPlaybookConfirm
// 误杀链）：waiter 挂起期间别副本批准 + 本进程 ctx 死亡 → 绝不落 abort 取消
// 路径，按批准推进（Orchestrate 被调用），确认卡绝不定为 cancelled。
func TestDispatchStepCtxDeathWithApprovedDecisionDoesNotAbort(t *testing.T) {
	t.Parallel()
	seq := &fakeSeq{}
	repos := newFakeReposForExecutor()
	orch := newFakeOrchestrator()
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	port := NewInMemExecutorWaiterDecisions()
	pe.SetWaiterDecisionPort(port)
	board := biz.PlanBoard{ID: "pb-ctx", SessionID: "sp-1", TaskID: "task-1", Status: biz.PlanStatusExecuting}
	r := newDagRun(pe, board)
	// dispatchStep 只消费预标记 Running（发现#15 断言），hold 在其之前。
	step := &biz.PlanStep{ID: "st-confirm", PlanID: board.ID, TaskID: "task-1", Label: "发布", ConfirmBefore: true, Status: biz.PlanStepStatusRunning, Version: 1}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.dispatchStep(ctx, step)
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !pe.HasPlaybookStageConfirm("sp-1", "st-confirm") {
		if time.Now().After(deadline) {
			t.Fatal("waiter not registered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// 别副本批准：只写共享决议面（本进程 chan 无人 signal）。
	if _, err := port.TryRecordDecision(context.Background(), biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{
			Kind:      biz.WaiterDecisionKindPlaybookConfirm,
			SessionID: "sp-1",
			ScopeID:   "st-confirm",
		},
		Approved:  true,
		Actor:     "replica-b",
		DecidedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("record approved decision: %v", err)
	}
	cancel()
	// 按批准推进：Orchestrate 必须被调用（不是 abort 取消）。
	if !orch.waitForCall("st-confirm", 2*time.Second) {
		t.Fatal("approved decision at ctx death must proceed, not abort")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("dispatchStep did not return after ctx death")
	}
	// 确认卡绝不因 ctx 死亡被误定为 cancelled（settle 按批准定案 completed）。
	for _, ev := range seq.snapshot() {
		if u, ok := ev.(*biz.StepUpdatedEvent); ok &&
			u.Step.ID == biz.PlaybookConfirmActivityID("sp-1", "st-confirm") &&
			u.Step.Status == biz.StepStatusCancelled {
			t.Fatal("approved decision must never finalize the card as cancelled")
		}
	}
}

// D6 验收钉（deny 侧）：ctx 死亡瞬间已有拒绝决议 → 走 skipPlaybookConfirmDenied
//（ConfirmDenied 标记阻断 resume 复活），而非 abort 的无标记取消。
func TestDispatchStepCtxDeathWithDeniedDecisionSkipsNotAbort(t *testing.T) {
	t.Parallel()
	repos := newFakeReposForExecutor()
	pe := NewPlanExecutor(repos, newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	port := NewInMemExecutorWaiterDecisions()
	pe.SetWaiterDecisionPort(port)
	board := biz.PlanBoard{ID: "pb-ctx-deny", SessionID: "sp-1", TaskID: "task-1", Status: biz.PlanStatusExecuting}
	r := newDagRun(pe, board)
	step := &biz.PlanStep{ID: "st-confirm", PlanID: board.ID, TaskID: "task-1", Label: "发布", ConfirmBefore: true, Status: biz.PlanStepStatusRunning, Version: 1}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.dispatchStep(ctx, step)
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !pe.HasPlaybookStageConfirm("sp-1", "st-confirm") {
		if time.Now().After(deadline) {
			t.Fatal("waiter not registered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := port.TryRecordDecision(context.Background(), biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{
			Kind:      biz.WaiterDecisionKindPlaybookConfirm,
			SessionID: "sp-1",
			ScopeID:   "st-confirm",
		},
		Approved:  false,
		Reason:    "no",
		Actor:     "replica-b",
		DecidedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("record denied decision: %v", err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("dispatchStep did not return after ctx death")
	}
	if step.Status != biz.PlanStepStatusSkipped {
		t.Fatalf("denied decision at ctx death must skip: status=%v", step.Status)
	}
	if step.Error == nil || step.Error.Code != biz.StepErrCodeConfirmDenied {
		t.Fatalf("deny provenance marker missing: err=%+v", step.Error)
	}
}

// D6-3 钉：team 完成通知永不落到本进程 chan（跨副本）时，team_runs_v2
// 终态行轮询推进 step——Completed 行 → step 完成。
func TestDispatchStepTeamCompletionPollerAdvances(t *testing.T) {
	t.Parallel()
	repos := newFakeReposForExecutor()
	orch := newFakeOrchestrator()
	pe := NewPlanExecutor(repos, orch, &fakeSeq{}, loggateway.NewNoop())
	reader := &fakeTeamRunReader{runs: map[string][]biz.TeamRun{}}
	pe.SetTeamRunReader(reader)
	board := biz.PlanBoard{ID: "pb-poll", SessionID: "sp-1", TaskID: "task-1", Status: biz.PlanStatusExecuting}
	r := newDagRun(pe, board)
	step := &biz.PlanStep{ID: "st-poll", PlanID: board.ID, TaskID: "task-1", Label: "执行", Status: biz.PlanStepStatusRunning, Version: 1}

	done := make(chan struct{})
	go func() {
		r.dispatchStep(context.Background(), step)
		close(done)
	}()
	if !orch.waitForCall("st-poll", 2*time.Second) {
		t.Fatal("dispatch did not reach orchestrator")
	}
	// 跨副本场景：完成通知落在别副本；本进程只靠 team_runs_v2 终态行。
	reader.set("fake-team-stage-st-poll", biz.TeamRun{
		ID: "run-1", TeamStageID: "fake-team-stage-st-poll", Seq: 1,
		Status: biz.TeamRunV2StatusCompleted,
	})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("poller did not advance the step")
	}
	if step.Status != biz.PlanStepStatusCompleted {
		t.Fatalf("step status=%v, want completed", step.Status)
	}
}

// D6-3 钉（cancel 侧）：Cancelled 终态行经轮询合成的事件必须让 L4 分类器
// 命中 cancel 类——不消耗自动重试预算（不重跑用户已取消的团队）。
func TestDispatchStepTeamCompletionPollerCancelledNoRetry(t *testing.T) {
	t.Parallel()
	repos := newFakeReposForExecutor()
	orch := newFakeOrchestrator()
	pe := NewPlanExecutor(repos, orch, &fakeSeq{}, loggateway.NewNoop())
	reader := &fakeTeamRunReader{runs: map[string][]biz.TeamRun{}}
	pe.SetTeamRunReader(reader)
	board := biz.PlanBoard{ID: "pb-poll-cancel", SessionID: "sp-1", TaskID: "task-1", Status: biz.PlanStatusExecuting}
	r := newDagRun(pe, board)
	step := &biz.PlanStep{ID: "st-cancel", PlanID: board.ID, TaskID: "task-1", Label: "执行", Status: biz.PlanStepStatusRunning, Version: 1}

	done := make(chan struct{})
	go func() {
		r.dispatchStep(context.Background(), step)
		close(done)
	}()
	if !orch.waitForCall("st-cancel", 2*time.Second) {
		t.Fatal("dispatch did not reach orchestrator")
	}
	reader.set("fake-team-stage-st-cancel", biz.TeamRun{
		ID: "run-1", TeamStageID: "fake-team-stage-st-cancel", Seq: 1,
		Status: biz.TeamRunV2StatusCancelled, // Error 为空——补 team_cancelled 分类锚
	})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("poller did not settle the cancelled run")
	}
	if step.Status != biz.PlanStepStatusFailed {
		t.Fatalf("cancelled run must fail the step: status=%v", step.Status)
	}
	if orch.waitForCallCount("st-cancel", 2, 300*time.Millisecond) {
		t.Fatal("cancelled run must not consume the retry budget")
	}
}

// D6 钉：多 run 同 stage（重试/假启动对账）时只有 Seq 最大者权威——旧 run
// 终态不得推进 step。
func TestPollTeamRunTerminalLatestSeqWins(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	reader := &fakeTeamRunReader{runs: map[string][]biz.TeamRun{}}
	pe.SetTeamRunReader(reader)
	r := newDagRun(pe, biz.PlanBoard{SessionID: "sp-1", ID: "pb-1"})
	reader.set("ts-1",
		biz.TeamRun{ID: "run-old", TeamStageID: "ts-1", Seq: 1, Status: biz.TeamRunV2StatusFailed, Error: "boom"},
		biz.TeamRun{ID: "run-new", TeamStageID: "ts-1", Seq: 2, Status: biz.TeamRunV2StatusRunning},
	)
	if _, done := r.pollTeamRunTerminal(context.Background(), "ts-1", "st-1"); done {
		t.Fatal("stale terminal run must not advance while a newer run is active")
	}
	reader.set("ts-1",
		biz.TeamRun{ID: "run-old", TeamStageID: "ts-1", Seq: 1, Status: biz.TeamRunV2StatusFailed, Error: "boom"},
		biz.TeamRun{ID: "run-new", TeamStageID: "ts-1", Seq: 2, Status: biz.TeamRunV2StatusCompleted},
	)
	ev, done := r.pollTeamRunTerminal(context.Background(), "ts-1", "st-1")
	if !done || !ev.Success || ev.TeamRunID != "run-new" {
		t.Fatalf("latest terminal run must advance: done=%v ev=%+v", done, ev)
	}
}

// D6 钉：WaitBoardStartup 慢路径——本进程无 chan 登记、无看板行时，从决议
// 表对账别副本的启动决议（失败带原因/成功放行）。
func TestWaitBoardStartupDecisionTableSlowPath(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	port := NewInMemExecutorWaiterDecisions()
	pe.SetWaiterDecisionPort(port)
	ctx := context.Background()

	if _, err := port.TryRecordDecision(ctx, biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{Kind: biz.WaiterDecisionKindBoardStartup, ScopeID: "pb-x"},
		Approved:  false,
		Reason:    "assembly failed",
		Actor:     "replica-b",
		DecidedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("record startup decision: %v", err)
	}
	ok, reason := pe.WaitBoardStartup(ctx, "pb-x", time.Second)
	if ok || reason != "assembly failed" {
		t.Fatalf("failed startup decision: ok=%v reason=%q", ok, reason)
	}

	if _, err := port.TryRecordDecision(ctx, biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{Kind: biz.WaiterDecisionKindBoardStartup, ScopeID: "pb-ok"},
		Approved:  true,
		Actor:     "replica-b",
		DecidedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("record startup decision: %v", err)
	}
	ok, reason = pe.WaitBoardStartup(ctx, "pb-ok", time.Second)
	if !ok || reason != "" {
		t.Fatalf("approved startup decision: ok=%v reason=%q", ok, reason)
	}
}

// D6 钉：signalStartup 双写决议表（CAS 与 buffered-1 chan 首发有效语义对齐）。
func TestSignalStartupRecordsDecision(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	port := NewInMemExecutorWaiterDecisions()
	pe.SetWaiterDecisionPort(port)
	pe.signalStartup("pb-s", true, "")
	dec, found, err := port.LoadDecision(context.Background(), biz.ExecutorWaiterDecisionKey{
		Kind:    biz.WaiterDecisionKindBoardStartup,
		ScopeID: "pb-s",
	})
	if err != nil || !found {
		t.Fatalf("signalStartup must record the decision: found=%v err=%v", found, err)
	}
	if !dec.Approved || dec.Actor != "plan_executor.startup" {
		t.Fatalf("decision=%+v", dec)
	}
}
