package service

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// D1-F3（87-orchestration-chain-hardening Wave-2 B1）：失败分类器新增
// terminal-cancel 类——用户取消信号沿 status=cancelled + errMsg 原文
// （"context canceled" 等）到达 failStepWith 后，不得消耗自动重试预算、
// 不得触发 5s 退避重跑；下游依赖经 cascadeSkip 标 skipped
// （StepErrCodeCascadeSkip，可被 ResumePlanBoard 复活），当前 step 仍标
// Failed（取消语义由分类/错误文本承载）。

// TestDagRunFailStepWith_CancelSkipsRetryAndCascadesSkip cancel 类失败：
// attempts 不递增、零二次 dispatch、下游 pending step 级联 skipped。
func TestDagRunFailStepWith_CancelSkipsRetryAndCascadesSkip(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-cancel-class",
		TaskID:    "task-cancel-class",
		SessionID: "sess-cancel-class",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "c1", PlanID: "board-cancel-class", TaskID: "task-cancel-class", Label: "c1", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "c2", PlanID: "board-cancel-class", TaskID: "task-cancel-class", Label: "c2", DependsOn: []string{"c1"}, Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	_, orch, _, seq, r := newAdmissionDagRun(board)
	r.mu.Lock()
	// failStepWith 的真实入口只对已被调度临界区预标记为 Running 的 step
	// 调用（handleCompletion），这里模拟该前置条件。
	if err := r.stepsByID["c1"].Transition(biz.PlanStepStatusRunning); err != nil {
		r.mu.Unlock()
		t.Fatalf("预标记 Running 失败: %v", err)
	}
	step := r.stepsByID["c1"]
	r.mu.Unlock()

	r.failStepWith(context.Background(), step, "team_failed", "context canceled", true)

	// 不消耗自动重试预算。
	r.mu.Lock()
	attempts := r.attempts["c1"]
	s1 := *r.stepsByID["c1"]
	s2 := *r.stepsByID["c2"]
	r.mu.Unlock()
	if attempts != 0 {
		t.Errorf("attempts[c1] = %d, want 0（用户取消不重试）", attempts)
	}
	// 无二次 dispatch。
	if got := orchCallCount(orch, "c1"); got != 0 {
		t.Errorf("orchestrator 调用 = %d, want 0（取消不触发退避重跑）", got)
	}
	// 当前 step 标记 Failed。
	if s1.Status != biz.PlanStepStatusFailed {
		t.Errorf("c1 状态 = %s, want failed", s1.Status)
	}
	// 下游 pending step 级联 skipped（带来源码，可复活）。
	if s2.Status != biz.PlanStepStatusSkipped {
		t.Errorf("c2 状态 = %s, want skipped（cascade）", s2.Status)
	}
	if s2.Error == nil || s2.Error.Code != biz.StepErrCodeCascadeSkip {
		t.Errorf("c2.Error = %+v, want code=%s", s2.Error, biz.StepErrCodeCascadeSkip)
	}
	// 无 Started/二次 Failed 事件噪声：恰好 1 条 Failed + 1 条 Skipped。
	kinds := countingEventKinds(seq.snapshot())
	if got := kinds[biz.EventKindPlanStepStarted]; got != 0 {
		t.Errorf("PlanStepStarted 事件 = %d, want 0", got)
	}
	if got := kinds[biz.EventKindPlanStepFailed]; got != 1 {
		t.Errorf("PlanStepFailed 事件 = %d, want 1（取消在 step 失败路径上至多记账一次）", got)
	}
	if got := kinds[biz.EventKindPlanStepSkipped]; got != 1 {
		t.Errorf("PlanStepSkipped 事件 = %d, want 1", got)
	}
}

// TestDagRunFailStepWith_TransientStillRetries 对照 pin：transient 失败
// （"connection reset"）仍走 maybeRetryStep 自动重试（F2 语义不回退）。
func TestDagRunFailStepWith_TransientStillRetries(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-transient-retry",
		TaskID:    "task-transient-retry",
		SessionID: "sess-transient-retry",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "t1", PlanID: "board-transient-retry", TaskID: "task-transient-retry", Label: "t1", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	_, orch, _, _, r := newAdmissionDagRun(board)
	r.mu.Lock()
	if err := r.stepsByID["t1"].Transition(biz.PlanStepStatusRunning); err != nil {
		r.mu.Unlock()
		t.Fatalf("预标记 Running 失败: %v", err)
	}
	step := r.stepsByID["t1"]
	r.mu.Unlock()

	r.failStepWith(context.Background(), step, "team_failed", "read: connection reset by peer", true)

	// transient → 消耗 1 次重试预算并重新排队（stepRetryBackoff=0）。
	r.mu.Lock()
	attempts := r.attempts["t1"]
	r.mu.Unlock()
	if attempts != 1 {
		t.Errorf("attempts[t1] = %d, want 1（transient 仍走自动重试）", attempts)
	}
	if !orch.waitForCall("t1", 2*time.Second) {
		t.Fatal("transient 失败未触发自动重试 dispatch")
	}
	// 收尾：重试成功，等 dispatch goroutine 退出。
	orch.completeStep("t1", true, "")
	r.wg.Wait()
	r.mu.Lock()
	final := r.stepsByID["t1"].Status
	r.mu.Unlock()
	if final != biz.PlanStepStatusCompleted {
		t.Errorf("t1 终态 = %s, want completed", final)
	}
}
