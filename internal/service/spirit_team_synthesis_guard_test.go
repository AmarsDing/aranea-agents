package service

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
)

// ── D3-FIX-3（87-orchestration-chain-hardening）：三态持久化守卫 + 复活重试
// 预算 + 挂靠完整性。synthesis 触发的「无法判定」与「确认未产出」必须区分：
// 前者推迟到轮询重试（释放 CAS，不烧预算、不触发），后者照常触发。

// (a)/⑥ 镜像：查询失败 = uncertain → 不触发且 CAS 已释放（可重试）。
// 与 TestCheckAllTeamsCompleted_TaskReadError_FailsOpen（翻转语义）互补，
// 直接断言内存 CAS 状态。
func TestSynthesisGuard_ReadError_UncertainReleasesCAS(t *testing.T) {
	controller := &stubSpiritTeamController{
		completedResult: biz.AllTeamsCompletedResult{
			AllDone: true, TeamIDs: []string{"t1"},
			TotalTeams: 1, CompletedTeams: 1,
		},
	}
	gw := &capturingTurnGateway{}
	s := newSynthesisStarter(controller, gw, &capturingEventBus{})
	s.taskV2Reader = &stubSynthesisTaskReader{err: errors.New("db down")}

	s.checkAllTeamsCompleted(context.Background(), "sp1")
	if got := len(gw.snapshot()); got != 0 {
		t.Fatalf("uncertain (read error): ExecuteTurn called %d times, want 0", got)
	}
	if _, loaded := s.synthesisTriggered.Load("sp1"); loaded {
		t.Fatal("uncertain must release CAS so the poller can retry after recovery")
	}
}

// (b) DB 空且无 ctx 根 = 时序间隙无法判定 → 不触发且 CAS 释放。
func TestSynthesisGuard_EmptyListNoCtx_UncertainDefers(t *testing.T) {
	controller := &stubSpiritTeamController{
		completedResult: biz.AllTeamsCompletedResult{
			AllDone: true, TeamIDs: []string{"t1"},
			TotalTeams: 1, CompletedTeams: 1,
		},
	}
	gw := &capturingTurnGateway{}
	s := newSynthesisStarter(controller, gw, &capturingEventBus{})
	s.taskV2Reader = &stubSynthesisTaskReader{tasks: nil}

	s.checkAllTeamsCompleted(context.Background(), "sp1")
	if got := len(gw.snapshot()); got != 0 {
		t.Fatalf("uncertain (empty list, no ctx root): ExecuteTurn called %d times, want 0", got)
	}
	if _, loaded := s.synthesisTriggered.Load("sp1"); loaded {
		t.Fatal("uncertain must release CAS (defer to next poller tick)")
	}
}

// (c) DB 空但 ctx 携带本轮根 Task（sequencer 异步落库间隙）→ 照常触发，
// ParentTaskID == ctx 根（挂靠完整性由 ctx 权威满足）。
func TestSynthesisGuard_EmptyListWithCtxRoot_TriggersWithCtxParent(t *testing.T) {
	controller := &stubSpiritTeamController{
		completedResult: biz.AllTeamsCompletedResult{
			AllDone: true, TeamIDs: []string{"t1"},
			TotalTeams: 1, CompletedTeams: 1,
		},
	}
	gw := &capturingTurnGateway{}
	s := newSynthesisStarter(controller, gw, &capturingEventBus{})
	s.taskV2Reader = &stubSynthesisTaskReader{tasks: nil}

	ctx := agent.ContextWithRootTaskActivityID(context.Background(), "task-rt-1")
	s.checkAllTeamsCompleted(ctx, "sp1")
	inputs := gw.snapshot()
	if len(inputs) != 1 {
		t.Fatalf("clear (ctx root authoritative): ExecuteTurn called %d times, want 1", len(inputs))
	}
	if inputs[0].ParentTaskID != "task-rt-1" {
		t.Errorf("ParentTaskID = %q, want %q (ctx 权威挂靠)", inputs[0].ParentTaskID, "task-rt-1")
	}
}

// (f) DB 最新行属于上一轮（已 Completed = 陈旧），ctx 根是本轮 → 触发，
// ParentTaskID 必须取 ctx 根而不是 DB 陈旧行（修复 stale-row 错挂）。
func TestSynthesisGuard_StaleListWithCtxRoot_AttachesToCtxRoot(t *testing.T) {
	controller := &stubSpiritTeamController{
		completedResult: biz.AllTeamsCompletedResult{
			AllDone: true, TeamIDs: []string{"t1"},
			TotalTeams: 1, CompletedTeams: 1,
		},
	}
	gw := &capturingTurnGateway{}
	s := newSynthesisStarter(controller, gw, &capturingEventBus{})
	s.taskV2Reader = &stubSynthesisTaskReader{tasks: []biz.Task{
		{ID: "task-1", SessionID: "sp1", Status: biz.TaskStatusCompleted}, // 上一轮
	}}

	ctx := agent.ContextWithRootTaskActivityID(context.Background(), "task-2")
	s.checkAllTeamsCompleted(ctx, "sp1")
	inputs := gw.snapshot()
	if len(inputs) != 1 {
		t.Fatalf("stale list + newer ctx root: ExecuteTurn called %d times, want 1", len(inputs))
	}
	if inputs[0].ParentTaskID != "task-2" {
		t.Errorf("ParentTaskID = %q, want %q (stale-row 错挂修复)", inputs[0].ParentTaskID, "task-2")
	}
}

// (d) 复活重试预算：连续同步失败 attempts<3 释放 CAS（轮询可重试）；
// 第 3 次失败预算耗尽 CAS 锁定，第 4 次调用不再触发；兜底通知仅首次 1 次。
func TestSynthesisGuard_ReviveBudgetExhaustion(t *testing.T) {
	controller := &stubSpiritTeamController{
		completedResult: biz.AllTeamsCompletedResult{
			AllDone: true, TeamIDs: []string{"t1"},
			TotalTeams: 1, CompletedTeams: 1,
		},
	}
	gw := &capturingTurnGateway{err: errors.New("turn gateway down")}
	eventBus := &capturingEventBus{}
	s := newSynthesisStarter(controller, gw, eventBus)
	s.taskV2Reader = &stubSynthesisTaskReader{tasks: []biz.Task{
		{ID: "task-1", SessionID: "sp1", Status: biz.TaskStatusRunning},
	}}

	ctx := context.Background()
	for i := 1; i <= 3; i++ {
		s.checkAllTeamsCompleted(ctx, "sp1")
		if got := len(gw.snapshot()); got != i {
			t.Fatalf("attempt %d: ExecuteTurn calls = %d, want %d (CAS released while budget remains)", i, got, i)
		}
	}
	// 第 3 次失败（attempts==3 == reviveMaxAttempts）后 CAS 锁定。
	if _, loaded := s.synthesisTriggered.Load("sp1"); !loaded {
		t.Fatal("budget exhausted: CAS must stay locked (no endless retry storm)")
	}
	// 第 4 次调用被 CAS 吸收。
	s.checkAllTeamsCompleted(ctx, "sp1")
	if got := len(gw.snapshot()); got != 3 {
		t.Fatalf("locked CAS must absorb later ticks: ExecuteTurn calls = %d, want 3", got)
	}
	// 兜底通知仅首次失败发布。
	notices := 0
	for _, ev := range eventBus.snapshot() {
		if stepEv, ok := ev.(*biz.StepCreatedEvent); ok && stepEv.Step.Kind == biz.StepKindNotice {
			notices++
		}
	}
	if notices != 1 {
		t.Fatalf("fallback notice must fire exactly once (first failure), got %d", notices)
	}
	// 预算计数留在 3。
	if v, loaded := s.synthesisAttempts.Load("sp1"); !loaded || v.(int) != 3 {
		t.Fatalf("synthesis attempts = %v (loaded=%v), want 3", v, loaded)
	}
}

// (e) StartTeamTurn 新一轮重置复活预算 + CAS：新一轮编排允许重新计数。
func TestSynthesisGuard_StartTeamTurnResetsBudget(t *testing.T) {
	writer := &stubTeamWriter{}
	team := biz.Team{ID: "team-b1", DisplayName: "团队", AutoCreated: true, Status: biz.TeamStatusPending}
	sess := biz.Session{
		ID: "team-sess-b1", TeamID: team.ID, ParentSessionID: "spirit-b1",
		Status: string(sessstatus.SessionStatusIdle),
	}
	controller := &stubSpiritTeamController{}
	runner := &capturingTeamRunner{}
	s := newStartTurnTestStarter(sess, team, runner, controller, writer)

	// 预置：上一轮 CAS 已烧、预算已耗尽。
	s.synthesisTriggered.Store("spirit-b1", true)
	s.synthesisAttempts.Store("spirit-b1", reviveMaxAttempts)

	if err := s.StartTeamTurn(context.Background(), sess.ID, "新一轮编排"); err != nil {
		t.Fatalf("StartTeamTurn: %v", err)
	}
	if _, loaded := s.synthesisTriggered.Load("spirit-b1"); loaded {
		t.Error("new round must reset synthesis CAS guard")
	}
	if _, loaded := s.synthesisAttempts.Load("spirit-b1"); loaded {
		t.Error("new round must reset revive attempt budget")
	}
}
