package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// 发现#7（87-orchestration-chain-hardening Wave-2 B1）：DAG 扇出配额闸门。
// 配额维度 per-spirit-session（与 plan_and_execute 入口门同源：
// GetMaxParallelTeams + ListActiveTeams），超限执行语义=延后准入
// （不就绪失败不就绪阻塞）：就绪但无槽步骤保持 Pending，每次步骤终态
// 转移触发全局重扫，「在飞=0 且就绪延滞且仅外部占用」时由单飞重扫循环
// 兜底活性。

// stubQuotaPort 实现 planExecutorQuotaPort（内存桩）。
type stubQuotaPort struct {
	mu     sync.Mutex
	max    int
	active []biz.Team
	err    error
}

func (s *stubQuotaPort) GetMaxParallelTeams(context.Context, string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.max
}

func (s *stubQuotaPort) ListActiveTeams(context.Context, string) ([]biz.Team, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active, s.err
}

func (s *stubQuotaPort) set(max int, active []biz.Team) {
	s.mu.Lock()
	s.max = max
	s.active = active
	s.mu.Unlock()
}

func newQuotaExecutor(quota *stubQuotaPort) (*PlanExecutor, *fakeOrchestrator, *fakeReposForExecutor, *fakeSeq) {
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	pe.stepRetryBackoff = 0
	if quota != nil {
		pe.SetQuotaPort(quota)
	}
	return pe, orch, repos, seq
}

// TestDagRun_FanOutQuotaDefersExcessRoots max=1 的 3 根扇出：并发 Running
// 峰值恒为 1，超额根延后准入（保持 Pending、零事件），全部最终完成。
func TestDagRun_FanOutQuotaDefersExcessRoots(t *testing.T) {
	t.Parallel()
	quota := &stubQuotaPort{max: 1}
	pe, orch, repos, _ := newQuotaExecutor(quota)
	board := biz.PlanBoard{
		ID:        "board-quota-fanout",
		TaskID:    "task-quota-fanout",
		SessionID: "sess-quota-fanout",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "q1", PlanID: "board-quota-fanout", TaskID: "task-quota-fanout", Label: "q1", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "q2", PlanID: "board-quota-fanout", TaskID: "task-quota-fanout", Label: "q2", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "q3", PlanID: "board-quota-fanout", TaskID: "task-quota-fanout", Label: "q3", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	// 按下标序首准入 q1；q2/q3 延后（峰值 1）。
	if !orch.waitForCall("q1", 2*time.Second) {
		t.Fatal("q1 未 dispatch")
	}
	if orch.waitForCall("q2", 100*time.Millisecond) || orch.waitForCall("q3", 50*time.Millisecond) {
		t.Fatal("配额 max=1 时 q2/q3 不得并发准入")
	}
	// q1 完成 → 终态转移触发全局重扫 → q2 准入。
	orch.completeStep("q1", true, "")
	if !orch.waitForCall("q2", 2*time.Second) {
		t.Fatal("q1 完成释放槽位后 q2 未准入")
	}
	if orch.waitForCall("q3", 100*time.Millisecond) {
		t.Fatal("q2 在飞时 q3 不得并发准入（峰值 1）")
	}
	orch.completeStep("q2", true, "")
	if !orch.waitForCall("q3", 2*time.Second) {
		t.Fatal("q2 完成释放槽位后 q3 未准入")
	}
	orch.completeStep("q3", true, "")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run 返回错误: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run 超时")
	}
	repos.mu.Lock()
	defer repos.mu.Unlock()
	for _, id := range []string{"q1", "q2", "q3"} {
		if got := repos.steps[id].Status; got != biz.PlanStepStatusCompleted {
			t.Errorf("%s 状态 = %s, want completed", id, got)
		}
	}
	if repos.board == nil || repos.board.Status != biz.PlanStatusCompleted {
		t.Errorf("board 终态 = %v, want completed", repos.board)
	}
}

// TestDagRun_QuotaSlotFreedAdmitsNext 首根完成后第二根在超时内被 auto
// 准入——验证「步骤终态转移 → 全局 admitReadySteps 重扫」链路。
func TestDagRun_QuotaSlotFreedAdmitsNext(t *testing.T) {
	t.Parallel()
	quota := &stubQuotaPort{max: 1}
	pe, orch, _, _ := newQuotaExecutor(quota)
	board := biz.PlanBoard{
		ID:        "board-quota-free",
		TaskID:    "task-quota-free",
		SessionID: "sess-quota-free",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "f1", PlanID: "board-quota-free", TaskID: "task-quota-free", Label: "f1", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "f2", PlanID: "board-quota-free", TaskID: "task-quota-free", Label: "f2", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	if !orch.waitForCall("f1", 2*time.Second) {
		t.Fatal("f1 未 dispatch")
	}
	if orch.waitForCall("f2", 100*time.Millisecond) {
		t.Fatal("f1 在飞时 f2 不得准入")
	}
	orch.completeStep("f1", true, "")
	if !orch.waitForCall("f2", 2*time.Second) {
		t.Fatal("f1 完成后 f2 未被全局重扫准入")
	}
	orch.completeStep("f2", true, "")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run 返回错误: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run 超时")
	}
}

// TestDagRun_QuotaExternalActiveBlocksThenRearmAdmits 外部团队（非本 run
// 步骤）占满配额时首轮准入 0；外部完成后由单飞重扫循环兜底活性——
// 否则本 run 在飞=0 无人能唤醒，board 会带着 pending 步骤误判 Completed。
func TestDagRun_QuotaExternalActiveBlocksThenRearmAdmits(t *testing.T) {
	t.Parallel()
	quota := &stubQuotaPort{
		max:    1,
		active: []biz.Team{{ID: "external-team", Status: biz.TeamStatusRunning}},
	}
	pe, orch, _, _ := newQuotaExecutor(quota)
	board := biz.PlanBoard{
		ID:        "board-quota-rearm",
		TaskID:    "task-quota-rearm",
		SessionID: "sess-quota-rearm",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "e1", PlanID: "board-quota-rearm", TaskID: "task-quota-rearm", Label: "e1", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	// built 直接持有 dagRun，收窄重扫周期（测试钩子字段）。
	r := newDagRun(pe, board)
	r.quotaRearmDelay = 20 * time.Millisecond

	done := make(chan error, 1)
	go func() { done <- r.run(context.Background()) }()

	// 首轮准入 0：外部占满唯一槽位，e1 延滞 Pending（不产生事件）。
	if orch.waitForCall("e1", 200*time.Millisecond) {
		t.Fatal("外部团队占座时 e1 不应被准入")
	}
	// 外部团队完成（无本 run 终态转移可触发重扫）→ 单飞重扫兜底。
	quota.set(1, nil)
	if !orch.waitForCall("e1", time.Second) {
		t.Fatal("单飞重扫未在 1s 内准入 e1")
	}
	orch.completeStep("e1", true, "")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run 返回错误: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run 超时")
	}
}

// TestDagRun_QuotaFailureReleasesSlotToSibling max=1 下根 X 连挂耗尽重试
// 后，释放的槽位经「failStepWith cascade 后补 admitReadySteps」准入独立
// 根 Y；board 终态 Failed 但 Y 的 Started 事件已发。
func TestDagRun_QuotaFailureReleasesSlotToSibling(t *testing.T) {
	t.Parallel()
	quota := &stubQuotaPort{max: 1}
	pe, orch, repos, seq := newQuotaExecutor(quota)
	board := biz.PlanBoard{
		ID:        "board-quota-fail",
		TaskID:    "task-quota-fail",
		SessionID: "sess-quota-fail",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "x", PlanID: "board-quota-fail", TaskID: "task-quota-fail", Label: "x", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "y", PlanID: "board-quota-fail", TaskID: "task-quota-fail", Label: "y", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	if !orch.waitForCall("x", 2*time.Second) {
		t.Fatal("x 未 dispatch")
	}
	// transient 失败 ×2：重试期间 X 仍占槽，Y 不得准入。
	orch.completeStep("x", false, "connection reset by peer")
	if !orch.waitForCallCount("x", 2, 2*time.Second) {
		t.Fatal("x 未被自动重试")
	}
	if orch.waitForCall("y", 100*time.Millisecond) {
		t.Fatal("x 重试在飞期间槽位仍被占用，y 不得准入")
	}
	orch.completeStep("x", false, "connection reset by peer again")
	// 重试耗尽 → cascade（无下游）→ 槽位释放 → Y 被准入并完成。
	if !orch.waitForCall("y", 2*time.Second) {
		t.Fatal("x 重试耗尽后 y 未被准入")
	}
	orch.completeStep("y", true, "")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run 返回错误: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run 超时")
	}

	repos.mu.Lock()
	defer repos.mu.Unlock()
	if repos.board == nil || repos.board.Status != biz.PlanStatusFailed {
		t.Errorf("board 终态 = %v, want failed", repos.board)
	}
	if got := repos.steps["x"].Status; got != biz.PlanStepStatusFailed {
		t.Errorf("x 状态 = %s, want failed", got)
	}
	if got := repos.steps["y"].Status; got != biz.PlanStepStatusCompleted {
		t.Errorf("y 状态 = %s, want completed", got)
	}
	// Y 的 Started 事件已发（x 首发+重试+y 首发 = 3 条 Started）。
	kinds := countingEventKinds(seq.snapshot())
	if got := kinds[biz.EventKindPlanStepStarted]; got != 3 {
		t.Errorf("PlanStepStarted 事件 = %d, want 3（x×2 + y×1）", got)
	}
}

// TestDagRun_QuotaNilPortUnlimited port=nil 时配额闸门 fail-open：3 根
// 并发 Running 峰值 == 3。
func TestDagRun_QuotaNilPortUnlimited(t *testing.T) {
	t.Parallel()
	pe, orch, _, _ := newQuotaExecutor(nil)
	board := biz.PlanBoard{
		ID:        "board-quota-nil",
		TaskID:    "task-quota-nil",
		SessionID: "sess-quota-nil",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "n1", PlanID: "board-quota-nil", TaskID: "task-quota-nil", Label: "n1", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "n2", PlanID: "board-quota-nil", TaskID: "task-quota-nil", Label: "n2", Status: biz.PlanStepStatusPending, Version: 1},
			{ID: "n3", PlanID: "board-quota-nil", TaskID: "task-quota-nil", Label: "n3", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	// 全部并发准入（无需任何完成信号）。
	for _, id := range []string{"n1", "n2", "n3"} {
		if !orch.waitForCall(id, 2*time.Second) {
			t.Fatalf("%s 未并发 dispatch", id)
		}
	}
	// 峰值 3：三个都在飞（pending channel 存续）。
	orch.mu.Lock()
	inflight := len(orch.pending)
	orch.mu.Unlock()
	if inflight != 3 {
		t.Errorf("并发在飞数 = %d, want 3（nil port 不限）", inflight)
	}
	for _, id := range []string{"n1", "n2", "n3"} {
		orch.completeStep(id, true, "")
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run 返回错误: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("run 超时")
	}
}
