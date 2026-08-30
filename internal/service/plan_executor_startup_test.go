package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// P2-② 假启动拦截（session-eval-20260829-r2 R4-Q1）：
// plan_and_execute 返回前必须对账看板真实启动结局。S07 事故：assembly 校验
// 失败（无效 agent_keys）→ 零 team_runs，但工具返回 running → Spirit 终复
// 谎称「编排已组建」。这些测试钉住 WaitBoardStartup 的对账语义。

// registerStartup 模拟 runBoard 的通道注册（Subscribe 直调路径不经过 runBoard）。
func registerStartup(pe *PlanExecutor, boardID string) {
	pe.startup.Store(boardID, make(chan startupResult, 1))
}

// 成功路径：首个 team 真实创建 → (true, "")。
func TestPlanExecutor_WaitBoardStartup_Success(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-startup-ok",
		TaskID:    "task-startup-ok",
		SessionID: "sess-startup-ok",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "board-startup-ok", TaskID: "task-startup-ok", Label: "s1", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	registerStartup(pe, board.ID)

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	ok, reason := pe.WaitBoardStartup(context.Background(), board.ID, 3*time.Second)
	if !ok || reason != "" {
		t.Fatalf("WaitBoardStartup=(%v,%q), want (true,\"\")", ok, reason)
	}

	// 收尾：完成步骤让 Subscribe 退出，避免 goroutine 泄漏。
	if !orch.waitForCall("s1", 2*time.Second) {
		t.Fatal("orchestrator never called for s1")
	}
	orch.completeStep("s1", true, "")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Subscribe returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}
}

// 步骤级失败路径（S07 事故形态）：orchestrate 校验失败 → failStep →
// 终态扫描 board Failed → 信号 false，reason 含真实失败原因。
// 回归守卫：此前仅 publishPlanBoardFailed（DAG 校验 fail-closed）发信号，
// 步骤级失败静默漏过对账。
func TestPlanExecutor_WaitBoardStartup_OrchestrateFailure(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-startup-fail",
		TaskID:    "task-startup-fail",
		SessionID: "sess-startup-fail",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "board-startup-fail", TaskID: "task-startup-fail", Label: "s1", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	orch.failErr = map[string]error{"s1": errors.New("agent keys not found or not active: ghost")}
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	registerStartup(pe, board.ID)

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	ok, reason := pe.WaitBoardStartup(context.Background(), board.ID, 3*time.Second)
	if ok {
		t.Fatalf("WaitBoardStartup=(true,%q), want (false, reason)", reason)
	}
	if !strings.Contains(reason, "agent keys not found") {
		t.Fatalf("reason=%q, want contains orchestrate error", reason)
	}

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}
	repos.mu.Lock()
	defer repos.mu.Unlock()
	if repos.board == nil || repos.board.Status != biz.PlanStatusFailed {
		status := "<nil>"
		if repos.board != nil {
			status = string(repos.board.Status)
		}
		t.Fatalf("board status=%s, want Failed", status)
	}
}

// DAG 校验 fail-closed 路径（环图）：publishPlanBoardFailed 信号 false。
func TestPlanExecutor_WaitBoardStartup_ValidationFailure(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-startup-cycle",
		TaskID:    "task-startup-cycle",
		SessionID: "sess-startup-cycle",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "c1", PlanID: "board-startup-cycle", TaskID: "task-startup-cycle", Label: "c1", DependsOn: []string{"c2"}, Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "c2", PlanID: "board-startup-cycle", TaskID: "task-startup-cycle", Label: "c2", DependsOn: []string{"c1"}, Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	registerStartup(pe, board.ID)

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	ok, reason := pe.WaitBoardStartup(context.Background(), board.ID, 3*time.Second)
	if ok {
		t.Fatalf("WaitBoardStartup=(true,%q), want (false, reason) for cyclic board", reason)
	}
	if !strings.Contains(reason, "cyclic") {
		t.Fatalf("reason=%q, want contains cyclic", reason)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}
}

// 超时放行：通道在但信号迟迟不到（HITL 挂起/慢创建）→ (true, "") 不阻断。
func TestPlanExecutor_WaitBoardStartup_TimeoutPassThrough(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	registerStartup(pe, "board-startup-slow")

	start := time.Now()
	ok, reason := pe.WaitBoardStartup(context.Background(), "board-startup-slow", 300*time.Millisecond)
	if !ok || reason != "" {
		t.Fatalf("WaitBoardStartup=(%v,%q), want (true,\"\") on timeout", ok, reason)
	}
	if elapsed := time.Since(start); elapsed < 250*time.Millisecond {
		t.Fatalf("returned too early (%v), timeout not honored", elapsed)
	}
}

// 终态 DB 兜底：通道缺失（进程重启/信号丢失）时按 board 终态对账。
// Completed/PartialFailure → 确有 team 落地 → true；Failed → false。
func TestPlanExecutor_WaitBoardStartup_TerminalFallback(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		status biz.PlanStatus
		wantOK bool
	}{
		{"completed", biz.PlanStatusCompleted, true},
		{"partial_failure", biz.PlanStatusPartialFailure, true},
		{"failed", biz.PlanStatusFailed, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repos := newFakeReposForExecutor()
			repos.boards["board-"+tc.name] = biz.PlanBoard{ID: "board-" + tc.name, Status: tc.status}
			pe := NewPlanExecutor(repos, newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
			ok, _ := pe.WaitBoardStartup(context.Background(), "board-"+tc.name, 500*time.Millisecond)
			if ok != tc.wantOK {
				t.Fatalf("status %s: WaitBoardStartup ok=%v, want %v", tc.status, ok, tc.wantOK)
			}
		})
	}
}
