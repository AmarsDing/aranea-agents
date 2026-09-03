package service

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// 分类规则表测试已随分类器迁至 internal/biz（TestClassifyStepFailure）。

// TestPlanExecutor_CapabilityFailureNoRetry P2-3：team 执行失败但分类为能力
// 缺失（"agent keys not found"，S07 类）时不再消耗自动重试预算——step 只
// dispatch 1 次，下游立即 cascade skip，board 快速收敛 Failed。
func TestPlanExecutor_CapabilityFailureNoRetry(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-capfail",
		TaskID:    "task-capfail",
		SessionID: "sess-capfail",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "c1", PlanID: "board-capfail", TaskID: "task-capfail", Label: "capstep", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "c2", PlanID: "board-capfail", TaskID: "task-capfail", Label: "depstep", DependsOn: []string{"c1"}, Status: biz.PlanStepStatusPending, Version: 1},
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

	if !orch.waitForCall("c1", 2*time.Second) {
		t.Fatal("c1 was not dispatched")
	}
	// 能力缺失失败：直接 cascade，不重试——Subscribe 应快速返回终态。
	orch.completeStep("c1", false, "agent keys not found")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Subscribe returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}

	// c1 只 dispatch 1 次（未自动重试），c2 从未 dispatch。
	orch.mu.Lock()
	c1Calls := 0
	for _, id := range orch.calls {
		switch id {
		case "c1":
			c1Calls++
		case "c2":
			orch.mu.Unlock()
			t.Fatal("c2 was dispatched despite dependency failure")
		}
	}
	orch.mu.Unlock()
	if c1Calls != 1 {
		t.Errorf("c1 dispatch count = %d, want 1 (capability failure must not auto-retry)", c1Calls)
	}

	repos.mu.Lock()
	defer repos.mu.Unlock()
	if got := repos.steps["c1"].Status; got != biz.PlanStepStatusFailed {
		t.Errorf("c1 status = %s, want %s", got, biz.PlanStepStatusFailed)
	}
	if got := repos.steps["c2"].Status; got != biz.PlanStepStatusSkipped {
		t.Errorf("c2 status = %s, want %s", got, biz.PlanStepStatusSkipped)
	}

	// 事件：c1 started ×1（无重试）+ failed ×1；c2 skipped ×1。
	kinds := countingEventKinds(seq.snapshot())
	if kinds[biz.EventKindPlanStepStarted] != 1 {
		t.Errorf("PlanStepStarted events = %d, want 1 (no retry)", kinds[biz.EventKindPlanStepStarted])
	}
	if kinds[biz.EventKindPlanStepFailed] != 1 {
		t.Errorf("PlanStepFailed events = %d, want 1", kinds[biz.EventKindPlanStepFailed])
	}
	if kinds[biz.EventKindPlanStepSkipped] != 1 {
		t.Errorf("PlanStepSkipped events = %d, want 1", kinds[biz.EventKindPlanStepSkipped])
	}
}
