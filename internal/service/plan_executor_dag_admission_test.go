package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// 发现#15（87-orchestration-chain-hardening Wave-2 B1）：
// check-and-dispatch 原子化——「判定就绪 → 转换 Running」收进同一个 r.mu
// 临界区（admitReadySteps 锁内预标记 Pending→Running），消灭旧
// dispatchReadyPending/checkDownstream 分离实现在并发完成时的
// double-dispatch 误杀链。本文件钉住该契约：dispatchStep 是断言方不是
// 转移方；Pending→Running 的唯一调度入口是 admitReadySteps；
// Failed→Running 的唯一入口是 maybeRetryStep 的锁内预标记。

// newAdmissionDagRun 构造一个不经 Subscribe 的最小 dagRun harness：
// 直接持有 *dagRun，便于对 admitReadySteps/dispatchStep/failStepWith 做
// 白盒驱动（stepRetryBackoff 置 0 关闭退避，quotaRearmDelay 默认 2s）。
func newAdmissionDagRun(board biz.PlanBoard) (*PlanExecutor, *fakeOrchestrator, *fakeReposForExecutor, *fakeSeq, *dagRun) {
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	pe.stepRetryBackoff = 0
	r := newDagRun(pe, board)
	return pe, orch, repos, seq, r
}

// orchCallCount 返回 stepID 在 orchestrator 调用列表中的出现次数。
func orchCallCount(orch *fakeOrchestrator, stepID string) int {
	orch.mu.Lock()
	defer orch.mu.Unlock()
	n := 0
	for _, id := range orch.calls {
		if id == stepID {
			n++
		}
	}
	return n
}

// TestDagRun_ConcurrentLastDeps_DispatchOnce 菱形 A→{B,C}→D：B/C 并发完成
// 时，下游 D 恰好 dispatch 一次——旧实现两个 handleCompletion 并发进
// checkDownstream，双双判定 D 就绪并 dispatch，第二次 Transition 失败被
// failStep 升级成 PlanStepFailed 误杀链；锁内预标记后重复就绪判定被锁内
// 状态检查自然吞掉。
func TestDagRun_ConcurrentLastDeps_DispatchOnce(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-diamond",
		TaskID:    "task-diamond",
		SessionID: "sess-diamond",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "a", PlanID: "board-diamond", TaskID: "task-diamond", Label: "root", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "b", PlanID: "board-diamond", TaskID: "task-diamond", Label: "left", DependsOn: []string{"a"}, Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "c", PlanID: "board-diamond", TaskID: "task-diamond", Label: "right", DependsOn: []string{"a"}, Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "d", PlanID: "board-diamond", TaskID: "task-diamond", Label: "join", DependsOn: []string{"b", "c"}, Status: biz.PlanStepStatusPending, Version: 1},
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

	if !orch.waitForCall("a", 2*time.Second) {
		t.Fatal("a 未 dispatch")
	}
	orch.completeStep("a", true, "")
	if !orch.waitForCall("b", 2*time.Second) {
		t.Fatal("b 未在 a 完成后 dispatch")
	}
	if !orch.waitForCall("c", 2*time.Second) {
		t.Fatal("c 未在 a 完成后 dispatch")
	}
	// 并发完成两个最后依赖。
	var cwg sync.WaitGroup
	cwg.Add(2)
	go func() { defer cwg.Done(); orch.completeStep("b", true, "") }()
	go func() { defer cwg.Done(); orch.completeStep("c", true, "") }()
	cwg.Wait()

	if !orch.waitForCallCount("d", 1, 2*time.Second) {
		t.Fatal("d 未在两个依赖完成后准入")
	}
	// 短停后计数仍 ==1：无 double dispatch。
	time.Sleep(100 * time.Millisecond)
	if got := orchCallCount(orch, "d"); got != 1 {
		t.Fatalf("d dispatch 次数 = %d, want 1（锁内预标记吞掉重复就绪判定）", got)
	}
	orch.completeStep("d", true, "")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run 返回错误: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run 超时")
	}

	// 零 PlanStepFailed 误报。
	kinds := countingEventKinds(seq.snapshot())
	if got := kinds[biz.EventKindPlanStepFailed]; got != 0 {
		t.Errorf("PlanStepFailed 事件 = %d, want 0（并发完成不得误杀下游）", got)
	}
	// board 收敛 Completed。
	repos.mu.Lock()
	defer repos.mu.Unlock()
	if repos.board == nil || repos.board.Status != biz.PlanStatusCompleted {
		t.Errorf("board 终态 = %v, want completed", repos.board)
	}
}

// TestDagRun_ConcurrentAdmitReadySteps_NoDoubleDispatch 直接对同一 dagRun
// 并发调 admitReadySteps：每步骤 orchestrator 调用数恰好 1（-race 跑）。
func TestDagRun_ConcurrentAdmitReadySteps_NoDoubleDispatch(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-admit-race",
		TaskID:    "task-admit-race",
		SessionID: "sess-admit-race",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "r1", PlanID: "board-admit-race", TaskID: "task-admit-race", Label: "r1", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "r2", PlanID: "board-admit-race", TaskID: "task-admit-race", Label: "r2", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "r3", PlanID: "board-admit-race", TaskID: "task-admit-race", Label: "r3", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	_, orch, _, _, r := newAdmissionDagRun(board)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); r.admitReadySteps(ctx) }()
	}
	wg.Wait()

	for _, id := range []string{"r1", "r2", "r3"} {
		if !orch.waitForCall(id, 2*time.Second) {
			t.Fatalf("%s 未 dispatch", id)
		}
	}
	time.Sleep(100 * time.Millisecond)
	for _, id := range []string{"r1", "r2", "r3"} {
		if got := orchCallCount(orch, id); got != 1 {
			t.Errorf("%s dispatch 次数 = %d, want 1", id, got)
		}
	}
	// 收尾：完成全部，等 dispatch goroutine 退出。
	for _, id := range []string{"r1", "r2", "r3"} {
		orch.completeStep(id, true, "")
	}
	r.wg.Wait()
}

// TestDagRun_DispatchStepRejectsNonPremarked dispatchStep 改为断言式后：
// 对未预标记（非 Running）的 step 直接调用必须打日志后 return——状态保持
// Pending、零 orchestrator 调用、零 failed/skipped 事件（发现#15 的误杀
// 源头被根除：严禁再调 failStep）。
func TestDagRun_DispatchStepRejectsNonPremarked(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-reject",
		TaskID:    "task-reject",
		SessionID: "sess-reject",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "x1", PlanID: "board-reject", TaskID: "task-reject", Label: "x1", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	_, orch, _, seq, r := newAdmissionDagRun(board)
	step := r.stepsByID["x1"]

	r.dispatchStep(context.Background(), step)

	if step.Status != biz.PlanStepStatusPending {
		t.Errorf("step.Status = %s, want pending（未预标记被拒后不得变更状态）", step.Status)
	}
	if got := orchCallCount(orch, "x1"); got != 0 {
		t.Errorf("orchestrator 调用 = %d, want 0", got)
	}
	kinds := countingEventKinds(seq.snapshot())
	if got := kinds[biz.EventKindPlanStepFailed]; got != 0 {
		t.Errorf("PlanStepFailed 事件 = %d, want 0（拒绝路径严禁 failStep）", got)
	}
	if got := kinds[biz.EventKindPlanStepSkipped]; got != 0 {
		t.Errorf("PlanStepSkipped 事件 = %d, want 0", got)
	}
}

// TestDagRun_RetryPreMarksFailedToRunning 重试路径的 Running 预标记同样
// 发生在 maybeRetryStep 的锁内：transient 失败后同一 step dispatch 恰好
// 2 次、第二次可 complete、cascade 未触发、下游照常准入。
func TestDagRun_RetryPreMarksFailedToRunning(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-retry-premark",
		TaskID:    "task-retry-premark",
		SessionID: "sess-retry-premark",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "x1", PlanID: "board-retry-premark", TaskID: "task-rety-premark", Label: "flaky", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "x2", PlanID: "board-retry-premark", TaskID: "task-retry-premark", Label: "downstream", DependsOn: []string{"x1"}, Status: biz.PlanStepStatusPending, Version: 1},
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

	if !orch.waitForCall("x1", 2*time.Second) {
		t.Fatal("x1 未 dispatch")
	}
	orch.completeStep("x1", false, "transient: connection reset by peer")
	// 自动重试：x1 第二次 dispatch（Failed→Running 锁内预标记后经断言）。
	if !orch.waitForCallCount("x1", 2, 2*time.Second) {
		t.Fatal("x1 未被自动重试")
	}
	// cascade 未触发：重试在飞期间下游不得被 skipped。
	kinds := countingEventKinds(seq.snapshot())
	if got := kinds[biz.EventKindPlanStepSkipped]; got != 0 {
		t.Fatalf("重试在飞期间 PlanStepSkipped = %d, want 0（cascade 不得触发）", got)
	}
	// 第二次可 complete。
	orch.completeStep("x1", true, "")
	// 下游照常准入。
	if !orch.waitForCall("x2", 2*time.Second) {
		t.Fatal("x2 未在 x1 重试成功后准入")
	}
	orch.completeStep("x2", true, "")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run 返回错误: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run 超时")
	}

	// 同一 step dispatch 恰好 2 次。
	if got := orchCallCount(orch, "x1"); got != 2 {
		t.Errorf("x1 dispatch 次数 = %d, want 2（首发+重试）", got)
	}
	repos.mu.Lock()
	defer repos.mu.Unlock()
	if got := repos.steps["x1"].Status; got != biz.PlanStepStatusCompleted {
		t.Errorf("x1 状态 = %s, want completed", got)
	}
	if got := repos.steps["x2"].Status; got != biz.PlanStepStatusCompleted {
		t.Errorf("x2 状态 = %s, want completed", got)
	}
}
