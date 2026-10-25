package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// 发现#17（87-orchestration-chain-hardening Wave-2 B1）：r.canceled 的
// 无锁写改持锁写，补齐与 maybeRetryStep 持锁读之间的 happens-before。
// 本文件用例在 `go test -race` 下钉住该竞态与取消终态分类语义。

// TestDagRun_CancelDuringRetryBackoff_RaceClean transient 失败使
// maybeRetryStep 进入 5s backoff 后取消：run 返回 ctx.Canceled、board
// 终态 Failed、backoff 中处于 Running 的 step 被 sweep 成 Skipped。
// 修复前该用例在 -race 下必报 data race（run goroutine 无锁写 canceled
// vs retry 路径持锁读）。
func TestDagRun_CancelDuringRetryBackoff_RaceClean(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-cancel-retry",
		TaskID:    "task-cancel-retry",
		SessionID: "sess-cancel-retry",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "r1", PlanID: "board-cancel-retry", TaskID: "task-cancel-retry", Label: "flaky", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	// 注意：不置 pe.stepRetryBackoff——5s 退避窗口正是竞态面。

	r := newDagRun(pe, board)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.run(ctx) }()

	if !orch.waitForCall("r1", 2*time.Second) {
		t.Fatal("r1 未 dispatch")
	}
	// transient 失败 → maybeRetryStep（持锁读 canceled）预标记 Failed→Running
	// 并进入 5s backoff。
	orch.completeStep("r1", false, "connection reset by peer")
	// 等 maybeRetryStep 临界区完成（attempts 递增与预标记同锁内发生），
	// 确保取消落在 backoff 窗口内——竞态读写对被 race detector 观测到。
	deadline := time.Now().Add(2 * time.Second)
	for {
		r.mu.Lock()
		a := r.attempts["r1"]
		r.mu.Unlock()
		if a >= 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("maybeRetryStep 未在超时内预标记重试")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("run 返回 %v, want ctx.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run 超时未返回")
	}

	repos.mu.Lock()
	defer repos.mu.Unlock()
	if repos.board == nil || repos.board.Status != biz.PlanStatusFailed {
		t.Errorf("board 终态 = %v, want failed（取消分类 PlanBoardEventFail）", repos.board)
	}
	step := repos.steps["r1"]
	if step.Status != biz.PlanStepStatusSkipped {
		t.Errorf("r1 状态 = %s, want skipped（cancel sweep 兜底 backoff 中的 Running）", step.Status)
	}
}

// TestDagRun_CancelTerminalClassificationUnchanged 取消后终态分类语义
// 不变：publishPlanBoardTerminal 走 PlanBoardEventFail 分支、
// signalStartup(false, "plan board run canceled") 首发有效。
func TestDagRun_CancelTerminalClassificationUnchanged(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-cancel-classify",
		TaskID:    "task-cancel-classify",
		SessionID: "sess-cancel-classify",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "c1", PlanID: "board-cancel-classify", TaskID: "task-cancel-classify", Label: "inflight", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "c2", PlanID: "board-cancel-classify", TaskID: "task-cancel-classify", Label: "pending", DependsOn: []string{"c1"}, Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	r := newDagRun(pe, board)
	// 模拟 startBoardDAG 的启动信号登记（首发有效，chan buffered 1）。
	startupCh := make(chan startupResult, 1)
	pe.startup.Store(board.ID, startupCh)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- r.run(ctx) }()

	if !orch.waitForCall("c1", 2*time.Second) {
		t.Fatal("c1 未 dispatch")
	}
	// 首步派发成功信号已投递（chan buffered 1，首发有效），先排水——
	// 模拟 WaitBoardStartup 消费；之后取消，方能观测取消失败信号入队。
	select {
	case sig := <-startupCh:
		if !sig.ok || sig.reason != "" {
			t.Fatalf("首发信号 = %+v, want {ok:true reason:\"\"}", sig)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("首步派发成功信号未投递")
	}
	// c1 在飞（不完成），c2 未就绪——取消。
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("run 返回 %v, want ctx.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run 超时未返回")
	}

	// Fail 分支 + 取消原因首发有效。
	select {
	case sig := <-startupCh:
		if sig.ok {
			t.Error("signalStartup ok = true, want false（取消对账失败）")
		}
		if sig.reason != "plan board run canceled" {
			t.Errorf("signalStartup reason = %q, want %q", sig.reason, "plan board run canceled")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("signalStartup 未收到取消失败信号")
	}
	repos.mu.Lock()
	defer repos.mu.Unlock()
	if repos.board == nil || repos.board.Status != biz.PlanStatusFailed {
		t.Errorf("board 终态 = %v, want failed", repos.board)
	}
}
