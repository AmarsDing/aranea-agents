package service

import (
	"context"
	"sync"
	"testing"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// ── 2026-07-27 总结重复触发修复 ─────────────────────────────────────────────
// 根因：CheckAllTeamsCompleted 只查 teams 表（plan-unaware），lazy 建团下
// 首波团队完成时后续 PlanStep 尚无团队记录 → 误判全完成，每个 DAG 波次都
// 触发一次 synthesis turn（会话 d78029b9 总结重复 3 次：total_teams 3→4→5）。
// 修复：dagRun 活跃期间 checkAllTeamsCompleted 跳过 synthesis；board 终态
// 处理在释放 lease 后通过 AllTeamsCompletedNotifier 唯一触发最终总结。

// 门控：本 session 存在活跃 dagRun（lazy 建团，后续 step 还会派发新团队）时，
// 即使当前 teams 全部终态也不得触发 synthesis —— 这是「波次中点」而非
// 「编排终点」。
func TestCheckAllTeamsCompleted_SkipsWhileDagRunActive(t *testing.T) {
	controller := &stubSpiritTeamController{
		completedResult: biz.AllTeamsCompletedResult{
			AllDone: true, TeamIDs: []string{"t1", "t2", "t3"},
			TotalTeams: 3, CompletedTeams: 3,
		},
	}
	gw := &capturingTurnGateway{}
	s := newSynthesisStarter(controller, gw, &capturingEventBus{})

	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	pe.running.Store("board-active", &boardRunLease{cancel: cancel, sessionID: "sp1"})
	s.planExecutor = pe

	s.checkAllTeamsCompleted(context.Background(), "sp1")

	if got := len(gw.snapshot()); got != 0 {
		t.Fatalf("ExecuteTurn called %d times, want 0 (dagRun active → mid-wave, not orchestration end)", got)
	}
}

// 门控不误伤：活跃 dagRun 属于其他 session 时，本 session 的 synthesis 照常触发。
func TestCheckAllTeamsCompleted_FiresWhenLeaseBelongsToOtherSession(t *testing.T) {
	controller := &stubSpiritTeamController{
		completedResult: biz.AllTeamsCompletedResult{
			AllDone: true, TeamIDs: []string{"t1"},
			TotalTeams: 1, CompletedTeams: 1,
		},
	}
	gw := &capturingTurnGateway{}
	s := newSynthesisStarter(controller, gw, &capturingEventBus{})

	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	pe.running.Store("board-other", &boardRunLease{cancel: cancel, sessionID: "other-session"})
	s.planExecutor = pe

	s.checkAllTeamsCompleted(context.Background(), "sp1")

	if got := len(gw.snapshot()); got != 1 {
		t.Fatalf("ExecuteTurn called %d times, want 1 (lease belongs to another session)", got)
	}
}

// recordingCompletionNotifier 记录 NotifyAllTeamsCompleted 调用，并在调用时刻
// 检查 board lease 是否已释放（门控自身触发不得被门拦住）。
type recordingCompletionNotifier struct {
	mu               sync.Mutex
	pe               *PlanExecutor
	boardID          string
	calls            []string
	leaseAliveAtCall map[string]bool
}

func (n *recordingCompletionNotifier) NotifyAllTeamsCompleted(_ context.Context, spiritSessionID string) {
	_, alive := n.pe.running.Load(n.boardID)
	n.mu.Lock()
	defer n.mu.Unlock()
	n.calls = append(n.calls, spiritSessionID)
	n.leaseAliveAtCall[spiritSessionID] = alive
}

// 终态触发：dagRun 跑完后必须调用一次 completionNotifier（唯一触发点），
// 且调用时 lease 已经释放 —— 否则 TeamStarter 侧的门控会把这个最终触发也拦掉，
// 导致整个编排没有任何总结。
func TestDagRunTerminal_NotifiesCompletionAfterLeaseRelease(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	pe.SetEventBus(event.NewV2Bus())

	notifier := &recordingCompletionNotifier{
		pe:               pe,
		boardID:          "board-notify",
		leaseAliveAtCall: map[string]bool{},
	}
	pe.SetCompletionNotifier(notifier)

	// 模拟 StartSubscription 的 lease（StartSubscription 走事件总线，此处直接持有）。
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	pe.running.Store("board-notify", &boardRunLease{cancel: cancel, sessionID: "sess-notify"})

	board := biz.PlanBoard{
		ID:        "board-notify",
		TaskID:    "task-notify",
		SessionID: "sess-notify",
		Status:    biz.PlanStatusPlanning,
		Version:   1,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "board-notify", TaskID: "task-notify", Label: "step1", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	runDone := make(chan error, 1)
	go func() { runDone <- pe.Subscribe(context.Background(), board) }()

	if !orch.waitForCall("s1", 2*time.Second) {
		t.Fatal("expected dispatch of s1")
	}
	orch.completeStep("s1", true, "")
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Subscribe did not return after step completion")
	}

	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	if len(notifier.calls) != 1 || notifier.calls[0] != "sess-notify" {
		t.Fatalf("NotifyAllTeamsCompleted calls=%v, want exactly [sess-notify]", notifier.calls)
	}
	if notifier.leaseAliveAtCall["sess-notify"] {
		t.Fatal("lease still held when notifier fired — the synthesis gate would block the final trigger")
	}
}

// v1 兼容：planExecutor 为 nil（v1-only 部署）时门常开，行为与修复前一致。
func TestCheckAllTeamsCompleted_NilPlanExecutor_GateOpen(t *testing.T) {
	controller := &stubSpiritTeamController{
		completedResult: biz.AllTeamsCompletedResult{
			AllDone: true, TeamIDs: []string{"t1"},
			TotalTeams: 1, CompletedTeams: 1,
		},
	}
	gw := &capturingTurnGateway{}
	s := newSynthesisStarter(controller, gw, &capturingEventBus{})
	// s.planExecutor 保持 nil。

	s.checkAllTeamsCompleted(context.Background(), "sp1")

	if got := len(gw.snapshot()); got != 1 {
		t.Fatalf("ExecuteTurn called %d times, want 1 (nil planExecutor → gate open)", got)
	}
}

// ── D3-FIX-2（87-orchestration-chain-hardening）：lease 释放通知注入根 Task
// releaseLeaseAndNotifyCompletion 必须把 board.TaskID 注入通知 ctx
// （RootTaskActivityID），否则 resolveLatestUserTaskID 的 ctx 权威分支恒走
// 不到、挂靠只能依赖「DB 最新 user task」推断（重启/重排场景会错挂轮次）。

// ctxRecordingNotifier 记录每次通知时刻的 RootTaskActivityID。
type ctxRecordingNotifier struct {
	mu    sync.Mutex
	calls []string
	rtIDs []string
}

func (n *ctxRecordingNotifier) NotifyAllTeamsCompleted(ctx context.Context, spiritSessionID string) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.calls = append(n.calls, spiritSessionID)
	n.rtIDs = append(n.rtIDs, string(chatagent.RootTaskActivityIDFromCtx(ctx)))
}

func TestDagRunReleaseLeaseNotify_InjectsRootTaskID(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	notifier := &ctxRecordingNotifier{}
	pe.SetCompletionNotifier(notifier)

	board := biz.PlanBoard{ID: "board-rt", TaskID: "task-notify", SessionID: "sess-rt"}
	dr := newDagRun(pe, board)
	dr.releaseLeaseAndNotifyCompletion()

	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	if len(notifier.calls) != 1 || notifier.calls[0] != "sess-rt" {
		t.Fatalf("NotifyAllTeamsCompleted calls=%v, want [sess-rt]", notifier.calls)
	}
	if notifier.rtIDs[0] != "task-notify" {
		t.Errorf("RootTaskActivityID in notify ctx = %q, want %q (attach-integrity fix)",
			notifier.rtIDs[0], "task-notify")
	}
}

// board.TaskID 为空（孤儿兜底场景）：通知仍送达一次，ctx 不携带根 Task，
// 不 panic —— 由 service 侧 attach-integrity（parentTaskID=="" → CAS 释放）
// 守住降级。
func TestDagRunReleaseLeaseNotify_EmptyTaskIDStillNotifies(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	notifier := &ctxRecordingNotifier{}
	pe.SetCompletionNotifier(notifier)

	board := biz.PlanBoard{ID: "board-orphan", SessionID: "sess-orphan"}
	dr := newDagRun(pe, board)
	dr.releaseLeaseAndNotifyCompletion()

	notifier.mu.Lock()
	defer notifier.mu.Unlock()
	if len(notifier.calls) != 1 || notifier.calls[0] != "sess-orphan" {
		t.Fatalf("NotifyAllTeamsCompleted calls=%v, want [sess-orphan]", notifier.calls)
	}
	if notifier.rtIDs[0] != "" {
		t.Errorf("RootTaskActivityID = %q, want empty when board.TaskID empty", notifier.rtIDs[0])
	}
}
