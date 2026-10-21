package biz

import (
	"fmt"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// fakeChatPendingQueue 功能性内存队列（P1 FR-7 测试）：实现
// ForceEnqueueFollowup 所需的入队/出队/满队语义子集，并实现可选
// chatPendingQueueRestored 端口（IsRestored）。
type fakeChatPendingQueue struct {
	restored bool
	queues   map[string][]PendingQueueEntry
	seq      int
}

func newFakeChatPendingQueue() *fakeChatPendingQueue {
	return &fakeChatPendingQueue{queues: make(map[string][]PendingQueueEntry)}
}

func (f *fakeChatPendingQueue) List(sessionID string) []PendingQueueEntry {
	return append([]PendingQueueEntry(nil), f.queues[sessionID]...)
}

func (f *fakeChatPendingQueue) Enqueue(sessionID, content string) string {
	if len(f.queues[sessionID]) >= ChatPendingQueueCap {
		return ""
	}
	f.seq++
	id := fmt.Sprintf("p%d", f.seq)
	f.queues[sessionID] = append(f.queues[sessionID], PendingQueueEntry{ID: id, Content: content, Status: "pending"})
	return id
}

func (f *fakeChatPendingQueue) EnqueueFollowup(sessionID, content string) string {
	return f.Enqueue(sessionID, content)
}

func (f *fakeChatPendingQueue) EnqueueInject(sessionID, content string) string {
	return f.Enqueue(sessionID, content)
}

func (f *fakeChatPendingQueue) FlushLeadingInjects(string) []PendingQueueEntry   { return nil }
func (f *fakeChatPendingQueue) DequeueLeadingInjects(string) []PendingQueueEntry { return nil }

func (f *fakeChatPendingQueue) Dequeue(sessionID string) (PendingQueueEntry, bool) {
	q := f.queues[sessionID]
	if len(q) == 0 {
		return PendingQueueEntry{}, false
	}
	head := q[0]
	f.queues[sessionID] = q[1:]
	return head, true
}

func (f *fakeChatPendingQueue) Peek(sessionID string) (PendingQueueEntry, bool) {
	q := f.queues[sessionID]
	if len(q) == 0 {
		return PendingQueueEntry{}, false
	}
	return q[0], true
}

func (f *fakeChatPendingQueue) Remove(string, string) bool          { return false }
func (f *fakeChatPendingQueue) Update(string, string, string) bool  { return false }
func (f *fakeChatPendingQueue) PromoteToFront(string, string) error { return nil }
func (f *fakeChatPendingQueue) SetPriority(string, string, int) error {
	return nil
}
func (f *fakeChatPendingQueue) Close() {}

func (f *fakeChatPendingQueue) IsRestored(sessionID string) bool { return f.restored }

func newChatUsecaseWithQueue(runs ChatRunGateway, pending ChatPendingQueue) *ChatUsecase {
	return NewChatUsecase(
		runs,
		stubChatSessionLocker{},
		pending,
		&stubChatPersister{},
		&stubChatEventPublisher{},
		loggateway.NewNoop(),
	)
}

// --- P1 FR-7：恢复队列惰性排空的强制入队与恢复集查询 ---

// TestChatUsecase_ForceEnqueueFollowupWithoutActiveRun 复现 FR-7 阻塞面：
// 「会话空闲 + 恢复行存在」时新消息必须能入队（否则 FIFO 排空无载体）。
// 对照：同一状态下常规 followup 入队必须仍被 no_active_run 拒绝。
func TestChatUsecase_ForceEnqueueFollowupWithoutActiveRun(t *testing.T) {
	pending := newFakeChatPendingQueue()
	uc := newChatUsecaseWithQueue(&stubChatRunGateway{hasActive: false}, pending)

	pid, reason, err := uc.ForceEnqueueFollowup("s-1", "新消息")
	if err != nil {
		t.Fatalf("force enqueue: %v", err)
	}
	if pid == "" || reason != "" {
		t.Fatalf("P1 FR-7: force enqueue must accept (pid=%q reason=%q)", pid, reason)
	}
	if entries := pending.List("s-1"); len(entries) != 1 || entries[0].Content != "新消息" {
		t.Fatalf("expected the new message queued, got %+v", entries)
	}

	// 对照组：常规路径维持拒绝。
	accepted, _, _, reason, err := uc.EnqueueUserMessageWithKind("s-1", "再来一条", ChatEnqueueKindFollowup, false)
	if err != nil || accepted || reason != ChatEnqueueRejectNoActiveRun {
		t.Fatalf("regular followup without active run must stay rejected, got accepted=%v reason=%q err=%v", accepted, reason, err)
	}
}

// TestChatUsecase_ForceEnqueueFollowupRespectsQueueFull：强制入队不放宽
// 容量上限——满队仍为 queue_full（冲刷对 followup 满队本就拒绝）。
func TestChatUsecase_ForceEnqueueFollowupRespectsQueueFull(t *testing.T) {
	pending := newFakeChatPendingQueue()
	for i := 0; i < ChatPendingQueueCap; i++ {
		pending.Enqueue("s-1", fmt.Sprintf("m%d", i))
	}
	uc := newChatUsecaseWithQueue(&stubChatRunGateway{hasActive: false}, pending)

	_, reason, err := uc.ForceEnqueueFollowup("s-1", "overflow")
	if err != nil {
		t.Fatalf("force enqueue: %v", err)
	}
	if reason != ChatEnqueueRejectQueueFull {
		t.Fatalf("full queue must reject with queue_full, got %q", reason)
	}
}

// TestChatUsecase_PendingQueueHasRestored：可选端口语义——队列实现支持
// IsRestored 时透传其判定；不支持时一律 false（不强迫存量桩跟进）。
func TestChatUsecase_PendingQueueHasRestored(t *testing.T) {
	restoredPending := newFakeChatPendingQueue()
	restoredPending.restored = true
	uc := newChatUsecaseWithQueue(&stubChatRunGateway{}, restoredPending)
	if !uc.PendingQueueHasRestored("s-1") {
		t.Fatal("P1 FR-7: restored marking from the queue impl must be visible")
	}

	plainUC := newChatUsecaseWithQueue(&stubChatRunGateway{}, stubChatPendingQueue{})
	if plainUC.PendingQueueHasRestored("s-1") {
		t.Fatal("queue impl without the restored port must report false")
	}
}
