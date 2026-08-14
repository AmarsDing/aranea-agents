package service

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// TestPlanExecutor_StartSubscription_StreamingShellWaitsForSteps 覆盖 20:45
// 会话计划失败的根因：流式分解路径先发布空壳 PlanBoardCreatedEvent
// （publishV2BoardShell，Steps=nil，供前端先渲染看板），steps 就绪后由
// PublishV2Board 发布 PlanBoardUpdatedEvent（Version=2，携带完整 steps）。
// 执行器必须：
//  1. 空壳 Created：不启动 DAG、不占 lease、不 fail-closed；
//  2. steps 就绪的 Updated：启动 DAG。
func TestPlanExecutor_StartSubscription_StreamingShellWaitsForSteps(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	bus := event.NewV2Bus()
	pe.SetEventBus(bus)
	pe.StartSubscription()
	t.Cleanup(pe.Stop)
	time.Sleep(20 * time.Millisecond)

	shell := biz.PlanBoard{
		ID:        "board-stream",
		TaskID:    "task-stream",
		SessionID: "sess-stream",
		Status:    biz.PlanStatusPlanning,
		Steps:     nil, // 流式壳：steps 未就绪
	}
	bus.Publish(context.Background(), biz.NewPlanBoardCreatedEvent(shell))
	time.Sleep(100 * time.Millisecond)

	// 空壳不得触发执行，也不得 fail-closed 写库。
	if orch.waitForCall("s1", 50*time.Millisecond) {
		t.Fatal("empty shell must not trigger DAG execution")
	}
	if repos.board != nil {
		t.Fatalf("empty shell must not be fail-closed upserted, got status=%s", repos.board.Status)
	}
	// 空壳不得占用 lease（否则后续 Updated 无法启动）。
	if _, ok := pe.running.Load(shell.ID); ok {
		t.Fatal("empty shell must not hold the execution lease")
	}

	// steps 就绪：PublishV2Board 流式分支发布 Updated（planning + steps）。
	ready := shell
	ready.Version = 2
	ready.Steps = []biz.PlanStep{
		{ID: "s1", PlanID: shell.ID, TaskID: shell.TaskID, Label: "step1", Status: biz.PlanStepStatusPending, Version: 1},
	}
	bus.Publish(context.Background(), biz.NewPlanBoardUpdatedEvent(ready))

	if !orch.waitForCall("s1", 2*time.Second) {
		t.Fatal("expected DAG execution to start on steps-ready Updated event")
	}
	orch.completeStep("s1", true, "")
}

// TestPlanExecutor_StartSubscription_SkipsNonPlanningUpdated 确保执行器自身
// 发布的 Updated（markPlanBoardExecuting → executing、terminal 事件）不会
// 重新触发 DAG。
func TestPlanExecutor_StartSubscription_SkipsNonPlanningUpdated(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	bus := event.NewV2Bus()
	pe.SetEventBus(bus)
	pe.StartSubscription()
	t.Cleanup(pe.Stop)
	time.Sleep(20 * time.Millisecond)

	board := biz.PlanBoard{
		ID:        "board-exec",
		TaskID:    "task-exec",
		SessionID: "sess-exec",
		Status:    biz.PlanStatusExecuting, // 执行中事件：不得启动
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "board-exec", TaskID: "task-exec", Label: "step1", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	bus.Publish(context.Background(), biz.NewPlanBoardUpdatedEvent(board))
	time.Sleep(100 * time.Millisecond)

	if orch.waitForCall("s1", 50*time.Millisecond) {
		t.Fatal("executing-status Updated must not trigger DAG execution")
	}
	if _, ok := pe.running.Load(board.ID); ok {
		t.Fatal("executing-status Updated must not hold the lease")
	}
}

// TestPlanExecutor_StartSubscription_ShellTimeoutFails 覆盖空壳兜底：
// Plan/Allocate 中途失败（如工具超时）时 PublishV2Board 永不到达，
// 看板不能永远停在 planning——超时后强制标记 Failed。
func TestPlanExecutor_StartSubscription_ShellTimeoutFails(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	pe.shellTimeout = 50 * time.Millisecond

	bus := event.NewV2Bus()
	pe.SetEventBus(bus)
	pe.StartSubscription()
	t.Cleanup(pe.Stop)
	time.Sleep(20 * time.Millisecond)

	shell := biz.PlanBoard{
		ID:        "board-shell-timeout",
		TaskID:    "task-shell-timeout",
		SessionID: "sess-shell-timeout",
		Status:    biz.PlanStatusPlanning,
		Version:   1,
		Steps:     nil,
	}
	// 模拟 Sequencer 对 PlanBoardCreatedEvent 的异步落库。
	if _, err := repos.UpsertPlanBoard(context.Background(), shell); err != nil {
		t.Fatal(err)
	}
	bus.Publish(context.Background(), biz.NewPlanBoardCreatedEvent(shell))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		repos.mu.Lock()
		st := repos.board.Status
		repos.mu.Unlock()
		if st == biz.PlanStatusFailed {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	repos.mu.Lock()
	defer repos.mu.Unlock()
	t.Fatalf("shell timeout must fail-close the board, got status=%s", repos.board.Status)
}

// TestPlanExecutor_StartSubscription_ShellTimeoutSkippedWhenStepsArrive
// 确保 steps 在超时前到达时兜底不误伤：DAG 正常启动后，超时回调自行跳过。
func TestPlanExecutor_StartSubscription_ShellTimeoutSkippedWhenStepsArrive(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	pe.shellTimeout = 150 * time.Millisecond

	bus := event.NewV2Bus()
	pe.SetEventBus(bus)
	pe.StartSubscription()
	t.Cleanup(pe.Stop)
	time.Sleep(20 * time.Millisecond)

	shell := biz.PlanBoard{
		ID:        "board-shell-ok",
		TaskID:    "task-shell-ok",
		SessionID: "sess-shell-ok",
		Status:    biz.PlanStatusPlanning,
		Version:   1,
	}
	if _, err := repos.UpsertPlanBoard(context.Background(), shell); err != nil {
		t.Fatal(err)
	}
	bus.Publish(context.Background(), biz.NewPlanBoardCreatedEvent(shell))

	// steps 在超时前到达 → DAG 启动。
	ready := shell
	ready.Version = 2
	ready.Steps = []biz.PlanStep{
		{ID: "s1", PlanID: shell.ID, TaskID: shell.TaskID, Label: "step1", Status: biz.PlanStepStatusPending, Version: 1},
	}
	bus.Publish(context.Background(), biz.NewPlanBoardUpdatedEvent(ready))
	if !orch.waitForCall("s1", 2*time.Second) {
		t.Fatal("expected DAG execution to start")
	}
	orch.completeStep("s1", true, "")

	// 超过兜底超时后，board 不得被误标 Failed（DAG 正常完成后是 Completed）。
	time.Sleep(300 * time.Millisecond)
	repos.mu.Lock()
	st := repos.board.Status
	repos.mu.Unlock()
	if st == biz.PlanStatusFailed {
		t.Fatalf("shell timeout must not fire after steps arrived, got status=%s", st)
	}
}
