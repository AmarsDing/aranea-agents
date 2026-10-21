package service

import (
	"context"
	"testing"
	"time"

	rt "aranea-agents/internal/runtime"
)

// --- P0 FR-5：「打断并发送」无活跃 run 时退化为正常发送（消息必达） ---

// TestInterruptAndSendMessage_NoActiveRunDrainsQueue 复现 M-4：消息在活跃 run
// 期间入队后，run 已结束（或进程重启），用户点击「打断并发送」——取消落空，
// 被提升的消息无人派发、永久搁浅于 pending 队列（静默成功）。
//
// 修复期望：取消落空且队列有被提升消息时，主动触发排空，消息按正常发送
// 路径派发（gate.entered 到达 SetRunStatus、队列清空）。
// 缺陷行为：仅提升+Warn 后 return nil —— gate 信号超时。
func TestInterruptAndSendMessage_NoActiveRunDrainsQueue(t *testing.T) {
	reg := rt.NewRunRegistry()
	bus := &captureEventBus{}
	gate := &gatedRunStatusMgr{
		chatRunManager: newNoopChatRunManager(),
		entered:        make(chan struct{}),
		release:        make(chan struct{}),
	}
	const sessionID = "sess-fr5-idle"
	orch, chatUC := newPendingRegTestOrch(t, reg, bus, gate, sessionID)

	pendingID := chatUC.EnqueuePendingMessage(sessionID, "queued-then-idle")
	if pendingID == "" {
		t.Fatal("expected pending id")
	}

	// 无活跃 run：打断取消落空，但消息必须仍被派发（退化为正常发送）。
	if err := orch.InterruptAndSendMessage(context.Background(), sessionID, pendingID); err != nil {
		t.Fatalf("InterruptAndSendMessage: %v", err)
	}

	select {
	case <-gate.entered:
	case <-time.After(3 * time.Second):
		t.Fatal("P0 FR-5: interrupt-and-send on idle session left message stranded in pending queue")
	}
	if n := len(chatUC.GetPendingMessages(sessionID)); n != 0 {
		t.Fatalf("P0 FR-5: expected queue dispatched (0 remain), got %d", n)
	}
	reg.Finish(sessionID, "")
}

// TestInterruptAndSendMessage_ActiveRunKeepsInterruptSemantics 对照组：有活跃
// run 时维持原语义——取消被触发、消息提升为队首；不自行排空（取消收账的
// EXECUTE 收尾 defer 负责排空，免双循环）。
func TestInterruptAndSendMessage_ActiveRunKeepsInterruptSemantics(t *testing.T) {
	reg := rt.NewRunRegistry()
	bus := &captureEventBus{}
	gate := &gatedRunStatusMgr{
		chatRunManager: newNoopChatRunManager(),
		entered:        make(chan struct{}),
		release:        make(chan struct{}),
	}
	const sessionID = "sess-fr5-active"
	orch, chatUC := newPendingRegTestOrch(t, reg, bus, gate, sessionID)

	pendingID := chatUC.EnqueuePendingMessage(sessionID, "second-priority")

	cancelled := make(chan struct{})
	reg.StoreCancelable(sessionID, "active-run", func() { close(cancelled) })

	if err := orch.InterruptAndSendMessage(context.Background(), sessionID, pendingID); err != nil {
		t.Fatalf("InterruptAndSendMessage: %v", err)
	}

	// 取消语义：活跃 run 的 cancel 函数被调用。
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("P0 FR-5: cancel must be invoked on the active run")
	}

	// 提升语义：目标消息位于队首。
	msgs := chatUC.GetPendingMessages(sessionID)
	if len(msgs) == 0 || msgs[0].ID != pendingID {
		t.Fatalf("P0 FR-5: message must be promoted to queue front, got %+v", msgs)
	}

	// 不自行排空：mock run 永不真正收尾，稳定窗内排空未被触发、队列不被消费。
	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if gate.calls.Load() != 0 {
			t.Fatalf("P0 FR-5: self-drain must not fire while a run is active (SetRunStatus calls=%d)", gate.calls.Load())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if n := len(chatUC.GetPendingMessages(sessionID)); n != 1 {
		t.Fatalf("P0 FR-5: queue must stay intact awaiting the cancelled run's drain, got %d", n)
	}
	reg.Finish(sessionID, "")
}
