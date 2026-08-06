package service

import (
	"context"
	"sync"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

// --- P4 取消路径 ctx 复现桩件 ---

// p4StepReader 返回一个 running 状态的 in-flight step。
type p4StepReader struct {
	biz.StepV2Reader
	step biz.Step
}

func (r *p4StepReader) ListStepsBySpiritSession(_ context.Context, _ string) ([]biz.Step, error) {
	return []biz.Step{r.step}, nil
}

// p4StepWriter 记录 UpdateStep 收到的 ctx 是否已取消。
type p4StepWriter struct {
	biz.StepV2Writer
	mu           sync.Mutex
	calls        int
	ctxCancelled bool
}

func (w *p4StepWriter) UpdateStep(ctx context.Context, s biz.Step) (biz.Step, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.calls++
	w.ctxCancelled = ctx.Err() != nil
	return s, nil
}

func (w *p4StepWriter) snapshot() (calls int, ctxCancelled bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.calls, w.ctxCancelled
}

// TestCancelActiveRun_CancelledCtxMustNotAbortStepCleanup 复现 P4：
// cancelActiveRun 为 SetRunStatus / transitionSessionStatus 精心构造了
// persistCtx（防取消），却在调用 chatactivity.CancelRunningActivityMessages
// 时传入原始（可能已取消的）ctx —— 违反自身 731-732 行的注释原则。
//
// 场景：WS/HTTP 入口传入的 ctx 已被取消（客户端断开或请求超时），用户触发
// 停止生成。此时 step 卡片清理（running → cancelled）会因 ctx.Err()!=nil
// 在 SQL 层直接失败，UI 上卡片永远停留在"执行中"。
//
// 期望行为（修复后）：UpdateStep 收到非取消 ctx，清理成功。
// 当前缺陷行为：UpdateStep 收到已取消 ctx。
func TestCancelActiveRun_CancelledCtxMustNotAbortStepCleanup(t *testing.T) {
	reg := rt.NewRunRegistry()
	reg.SetStatus("sess-p4", "run-p4", "running", "")
	reg.StoreCancelable("sess-p4", "run-p4", func() {})

	step := biz.Step{
		ID:              "step-p4",
		SessionID:       "sess-p4",
		SpiritSessionID: "sess-p4",
		Status:          biz.StepStatusRunning,
	}
	writer := &p4StepWriter{}

	bus := event.NewV2Bus()
	rStatus := newChatRunStatusTracker(reg, nil, bus, loggateway.NewNoop())
	orch := &ChatOrchestrator{
		runs: reg,
		core: chatTurnCoreDeps{
			TD:         rt.TurnDeps{},
			StepReader: &p4StepReader{step: step},
			StepWriter: writer,
		},
		runMgr: &chatRunManagerImpl{
			runStatusTracker:    rStatus,
			pendingQueueManager: noopPendingQueueManager{},
			awaitCoordinator:    noopAwaitCoordinator{},
			sessionRunLifecycle: noopSessionRunLifecycle{},
		},
		turnLC:    newNoopChatTurnLifecycle(),
		infraDeps: ChatInfraDeps{LG: loggateway.NewNoop()},
	}

	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel() // 模拟入口 ctx 已取消（客户端断开 / 请求超时）

	if !orch.cancelActiveRun(cancelledCtx, "sess-p4") {
		t.Fatal("expected cancelActiveRun to stop the active run")
	}

	calls, ctxCancelled := writer.snapshot()
	if calls != 1 {
		t.Fatalf("expected exactly 1 UpdateStep call, got %d", calls)
	}
	if ctxCancelled {
		t.Fatal("P4: UpdateStep received a cancelled ctx — step cleanup would fail at SQL layer, " +
			"leaving the card stuck in running state; cancelActiveRun must pass persistCtx")
	}
}
