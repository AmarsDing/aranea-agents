package service

import (
	"testing"
	"time"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// 2026-10-29 44 实测 round-8 丢消息回归：run 活跃期提交的消息被 ACK 为
// queued（steer=true——steer 进框架队列成功），但 run 干净收官（~15s 简单
// turn，单 LLM step）后流循环再无下一迭代消费该消息，框架随 run 注销将其
// 静默丢弃。修复：框架经 RunOptions 回调把未消费消息交还，
// requeueUnconsumedQueuedMessages 以 followup 级回收进 pending 队列，由
// turn 收尾 defer 的 processPendingQueue 统一排空（与中止/失败收尾对齐）。

// TestRequeueUnconsumedQueuedMessages_EnqueuesFollowupsFIFO 验证回收本身：
// 未消费的 steer 消息按原序重新入 pending 队列（followup 级，可被排空循环
// 派发），且回收只入队不触发派发（mid-run 抢跑防护——排空统一由 turn 收尾
// defer 触发）。
func TestRequeueUnconsumedQueuedMessages_EnqueuesFollowupsFIFO(t *testing.T) {
	reg := rt.NewRunRegistry()
	bus := &captureEventBus{}
	gate := &gatedRunStatusMgr{
		chatRunManager: newNoopChatRunManager(),
		entered:        make(chan struct{}),
		release:        make(chan struct{}),
	}
	const sessionID = "sess-steer-rescue-fifo"
	orch, chatUC := newPendingRegTestOrch(t, reg, bus, gate, sessionID)

	orch.requeueUnconsumedQueuedMessages(sessionID, []trpcmodel.Message{
		trpcmodel.NewUserMessage("跟进一"),
		trpcmodel.NewUserMessage("  "),
		trpcmodel.NewUserMessage("跟进二"),
	})

	entries := chatUC.GetPendingMessages(sessionID)
	if len(entries) != 2 {
		t.Fatalf("expected 2 rescued followups queued (blank skipped), got %d", len(entries))
	}
	if entries[0].Content != "跟进一" || entries[1].Content != "跟进二" {
		t.Fatalf("rescue must preserve FIFO order, got [%q %q]", entries[0].Content, entries[1].Content)
	}
	// followup 级（空 Kind 兼容 followup）才能被排空循环唤醒派发。
	for _, e := range entries {
		if e.Kind == "inject" {
			t.Fatalf("rescued message must not be inject kind, got %q", e.Kind)
		}
	}
	// 守卫：回收本身绝不触发派发（调用点位于 run 事件循环收尾，drain 由
	// turn 收尾 defer 统一触发，防 mid-run 抢跑与重复派发）。
	if n := gate.calls.Load(); n != 0 {
		t.Fatalf("rescue must not dispatch by itself (SetRunStatus calls=%d)", n)
	}
	reg.Finish(sessionID, "")
}

// TestRequeueUnconsumedQueuedMessages_DrainedOnceAfterCleanCompletion 复现
// 干净收官丢消息全链路：steer 消息被回收后，turn 收尾 defer 的
// processPendingQueue 触发必须把它派发成新 turn（gate 命中一次），且队列
// 清空、无重复派发（double-fire 守卫）。
func TestRequeueUnconsumedQueuedMessages_DrainedOnceAfterCleanCompletion(t *testing.T) {
	reg := rt.NewRunRegistry()
	bus := &captureEventBus{}
	gate := &gatedRunStatusMgr{
		chatRunManager: newNoopChatRunManager(),
		entered:        make(chan struct{}),
		release:        make(chan struct{}), // 驻留执行窗口：观察派发恰好一次
	}
	const sessionID = "sess-steer-rescue-drain"
	orch, chatUC := newPendingRegTestOrch(t, reg, bus, gate, sessionID)

	// 干净收官：框架交还未消费的 steer 消息（run 事件循环收尾、turn defer
	// 之前），随后 turn 收尾 defer 触发排空——此处直接按同一时序编排。
	orch.requeueUnconsumedQueuedMessages(sessionID, []trpcmodel.Message{
		trpcmodel.NewUserMessage("run 活跃期被 steer 的追问"),
	})

	sess := biz.Session{ID: sessionID, AgentID: "agent-1", OwnerType: "agent"}
	ag := biz.Agent{ID: "agent-1", AgentKey: "agent-1", Provider: "p", Model: "m"}
	orch.processPendingQueue(sessionID, sess, ag, "default", "p", "m", "")

	select {
	case <-gate.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("rescued steer message was never dispatched after clean completion — message lost")
	}
	if n := len(chatUC.GetPendingMessages(sessionID)); n != 0 {
		t.Fatalf("expected queue drained after dispatch, %d message(s) remain", n)
	}
	// double-fire 守卫：稳定窗内派发恰好一次（同一消息不得被第二个排空
	// 循环或接力触发重复出队）。
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if gate.calls.Load() != 1 {
			t.Fatalf("rescued message dispatched more than once (SetRunStatus calls=%d)", gate.calls.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
	reg.Finish(sessionID, "")
}

// TestRequeueUnconsumedQueuedMessages_NoopSafety 边界守卫：空消息列表、空
// sessionID、nil 编排器/依赖时回收为安全 no-op。
func TestRequeueUnconsumedQueuedMessages_NoopSafety(t *testing.T) {
	var nilOrch *ChatOrchestrator
	nilOrch.requeueUnconsumedQueuedMessages("s1", []trpcmodel.Message{trpcmodel.NewUserMessage("x")})

	orch := &ChatOrchestrator{} // chatUC 为 nil
	orch.requeueUnconsumedQueuedMessages("s1", []trpcmodel.Message{trpcmodel.NewUserMessage("x")})

	reg := rt.NewRunRegistry()
	bus := &captureEventBus{}
	gate := &gatedRunStatusMgr{
		chatRunManager: newNoopChatRunManager(),
		entered:        make(chan struct{}),
		release:        make(chan struct{}),
	}
	const sessionID = "sess-steer-rescue-noop"
	orch2, chatUC := newPendingRegTestOrch(t, reg, bus, gate, sessionID)

	orch2.requeueUnconsumedQueuedMessages(sessionID, nil)
	orch2.requeueUnconsumedQueuedMessages("", []trpcmodel.Message{trpcmodel.NewUserMessage("x")})
	if n := len(chatUC.GetPendingMessages(sessionID)); n != 0 {
		t.Fatalf("no-op rescue must not enqueue anything, got %d", n)
	}
	reg.Finish(sessionID, "")
}
